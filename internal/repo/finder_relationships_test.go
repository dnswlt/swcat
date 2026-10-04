package repo

import (
	"slices"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

func TestFinderRelationshipPredicates(t *testing.T) {
	r := NewRepository()
	team := &catalog.Group{Metadata: &catalog.Metadata{Name: "team"}, Spec: &catalog.GroupSpec{Type: "team"}}
	x := &catalog.Domain{Metadata: &catalog.Metadata{Name: "x"}, Spec: &catalog.DomainSpec{Owner: team.GetRef()}}
	y := &catalog.Domain{Metadata: &catalog.Metadata{Name: "y"}, Spec: &catalog.DomainSpec{Owner: team.GetRef()}}
	sx := &catalog.System{Metadata: &catalog.Metadata{Name: "sx"}, Spec: &catalog.SystemSpec{Owner: team.GetRef(), Domain: x.GetRef()}}
	sy := &catalog.System{Metadata: &catalog.Metadata{Name: "sy"}, Spec: &catalog.SystemSpec{Owner: team.GetRef(), Domain: y.GetRef()}}
	ax := &catalog.API{Metadata: &catalog.Metadata{Name: "shared"}, Spec: &catalog.APISpec{
		Type: "openapi", Lifecycle: "experimental", Owner: team.GetRef(), System: sx.GetRef(),
	}}
	ay := &catalog.API{Metadata: &catalog.Metadata{Name: "shared", Namespace: "other", Tags: []string{"production"}}, Spec: &catalog.APISpec{
		Type: "openapi", Lifecycle: "production", Owner: team.GetRef(), System: sy.GetRef(),
	}}
	component := func(name string, system *catalog.System) *catalog.Component {
		return &catalog.Component{Metadata: &catalog.Metadata{Name: name}, Spec: &catalog.ComponentSpec{
			Type: "service", Lifecycle: "production", Owner: team.GetRef(), System: system.GetRef(),
		}}
	}
	refs := func(entities ...catalog.Entity) []*catalog.LabelRef {
		var result []*catalog.LabelRef
		for _, entity := range entities {
			result = append(result, &catalog.LabelRef{Ref: entity.GetRef()})
		}
		return result
	}
	provider := component("provider", sx)
	provider.Spec.ProvidesAPIs = refs(ax)
	external := component("external", sy)
	external.Metadata.Tags = []string{"client"}
	external.Spec.ConsumesAPIs = refs(ax, ay)
	internal := component("internal", sx)
	internal.Spec.ConsumesAPIs = refs(ax)
	internal.Spec.SubcomponentOf = provider.GetRef()
	other := component("other", sy)
	other.Spec.ConsumesAPIs = refs(ay)
	empty := component("empty", sy)
	db := &catalog.Resource{Metadata: &catalog.Metadata{Name: "db"}, Spec: &catalog.ResourceSpec{
		Type: "database", Owner: team.GetRef(), System: sy.GetRef(), DependsOn: refs(provider),
	}}
	external.Spec.DependsOn = refs(db)
	for _, entity := range []catalog.Entity{team, x, y, sx, sy, ax, ay, provider, external, internal, other, empty, db} {
		if err := r.AddEntity(entity); err != nil {
			t.Fatal(err)
		}
	}
	// Validate populates inherited domains and inverse relationships as in a real catalog.
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	finder := NewFinder(func(e catalog.Entity, property string) ([]string, bool) {
		if property != "lint" {
			return nil, false
		}
		if e == ax {
			return []string{"warn"}, true
		}
		return nil, true
	})
	tests := []struct {
		query string
		want  []string
	}{
		{"kind=component !domain=x consumesApis[domain=x]", []string{"component:external"}},
		{"kind=component !domain=x consumesApis[domain=x AND tag=production]", nil},
		{"kind=component !domain=x consumesApis[domain=x] consumesApis[tag=production]", []string{"component:external"}},
		{"consumesApis[providedBy[owner[name=team]]]", []string{"component:external", "component:internal"}},
		{"consumesApis[domain[name=x]]", []string{"component:external", "component:internal"}},
		{"kind=component system[domain[name=x]]", []string{"component:internal", "component:provider"}},
		{"kind=component !consumesApis[domain=x]", []string{"component:empty", "component:other", "component:provider"}},
		{"consumesApis[!domain=x]", []string{"component:external", "component:other"}},
		{"consumesApis[domain=x OR tag=production]", []string{"component:external", "component:internal", "component:other"}},
		{"consumesApis[shared] tag=client", []string{"component:external"}},
		{"consumesApis[namespace=other]", []string{"component:external", "component:other"}},
		{"CONSUMESAPIS[domain=X]", []string{"component:external", "component:internal"}},
		{"consumesApis[lint=warn]", []string{"component:external", "component:internal"}},
		{"providesApis[consumedBy[domain=y]]", []string{"component:provider"}},
		{"kind=api consumedBy[!domain=x]", []string{"api:other/shared", "api:shared"}},
		{"providedBy[owner=team]", []string{"api:shared"}},
		{"dependsOn[dependsOn[providesApis[domain=x]]]", []string{"component:external"}},
		{"kind=resource dependents[consumesApis[domain=x]]", []string{"resource:db"}},
		{"kind=component rel[kind=resource type=database]", []string{"component:external", "component:provider"}},
		{"subcomponentOf=provider", []string{"component:internal"}},
		{"subcomponentOf[providesApis=shared]", []string{"component:internal"}},
		{"subcomponents[consumesApis[domain=x]]", []string{"component:provider"}},
		{"components=internal", []string{"system:sx"}},
		{"components[consumesApis[namespace=other]]", []string{"system:sy"}},
		{"apis[tag=production]", []string{"system:sy"}},
		{"resources[type=database]", []string{"system:sy"}},
		{"systems[components[name=internal]]", []string{"domain:x"}},
		{"kind=domain !systems[apis[lifecycle=production]]", []string{"domain:x"}},
		// Following a cycle is bounded by the query's explicit nesting depth.
		{"name=external consumesApis[consumedBy[consumesApis[domain=x]]]", []string{"component:external"}},
		// A known relationship on an inapplicable entity kind has no witnesses.
		{"kind=group consumesApis[name=shared]", nil},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			var got []string
			for _, entity := range finder.FindEntities(r, tt.query) {
				got = append(got, entity.GetRef().String())
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	// Kind-specific list searches must also pass the repository to the evaluator.
	got := finder.FindComponents(r, "!domain=x consumesApis[domain=x]")
	if len(got) != 1 || got[0] != external {
		t.Fatalf("FindComponents returned %v, want external", got)
	}
	apis := finder.FindAPIs(r, "consumedBy[name=external]")
	if len(apis) != 2 {
		t.Fatalf("FindAPIs returned %d APIs, want 2", len(apis))
	}
}
