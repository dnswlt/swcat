package web

import (
	"sort"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// buildGraphFixture builds a small catalog for FullyConnectedGraph tests.
//
// Forward edges:
//
//	compA --consumesApis--> apiX
//	apiX  --providedBy----> compB
//	compB --dependsOn-----> compC
//	compC --dependsOn-----> resR1
//	resR1 --dependsOn-----> resR2
//	compC --dependsOn-----> resOther   (side branch out of compC)
//	compD --dependsOn-----> compB      (side branch into compB)
//	compIso                            (no edges)
func buildGraphFixture(t *testing.T) (*repo.Repository, map[string]catalog.Entity) {
	t.Helper()

	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "team"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "dom"},
		Spec:     &catalog.DomainSpec{Owner: owner.GetRef()},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "sys"},
		Spec: &catalog.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}

	apiX := &catalog.API{
		Metadata: &catalog.Metadata{Name: "apiX"},
		Spec: &catalog.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
		},
	}

	resR2 := &catalog.Resource{
		Metadata: &catalog.Metadata{Name: "resR2"},
		Spec: &catalog.ResourceSpec{
			Type:   "database",
			Owner:  owner.GetRef(),
			System: system.GetRef(),
		},
	}
	resR1 := &catalog.Resource{
		Metadata: &catalog.Metadata{Name: "resR1"},
		Spec: &catalog.ResourceSpec{
			Type:      "database",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			DependsOn: []*catalog.LabelRef{{Ref: resR2.GetRef()}},
		},
	}
	resOther := &catalog.Resource{
		Metadata: &catalog.Metadata{Name: "resOther"},
		Spec: &catalog.ResourceSpec{
			Type:   "database",
			Owner:  owner.GetRef(),
			System: system.GetRef(),
		},
	}

	compC := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "compC"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			DependsOn: []*catalog.LabelRef{
				{Ref: resR1.GetRef()},
				{Ref: resOther.GetRef()},
			},
		},
	}
	compB := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "compB"},
		Spec: &catalog.ComponentSpec{
			Type:         "service",
			Lifecycle:    "production",
			Owner:        owner.GetRef(),
			System:       system.GetRef(),
			ProvidesAPIs: []*catalog.LabelRef{{Ref: apiX.GetRef()}},
			DependsOn:    []*catalog.LabelRef{{Ref: compC.GetRef()}},
		},
	}
	compA := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "compA"},
		Spec: &catalog.ComponentSpec{
			Type:         "service",
			Lifecycle:    "production",
			Owner:        owner.GetRef(),
			System:       system.GetRef(),
			ConsumesAPIs: []*catalog.LabelRef{{Ref: apiX.GetRef()}},
		},
	}
	compD := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "compD"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			DependsOn: []*catalog.LabelRef{{Ref: compB.GetRef()}},
		},
	}
	compIso := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "compIso"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
		},
	}

	r := repo.NewRepository()
	for _, e := range []catalog.Entity{
		owner, domain, system,
		apiX,
		resR2, resR1, resOther,
		compC, compB, compA, compD, compIso,
	} {
		if err := r.AddEntity(e); err != nil {
			t.Fatalf("AddEntity(%s): %v", e.GetRef(), err)
		}
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}

	byName := map[string]catalog.Entity{
		"apiX":     apiX,
		"resR1":    resR1,
		"resR2":    resR2,
		"resOther": resOther,
		"compA":    compA,
		"compB":    compB,
		"compC":    compC,
		"compD":    compD,
		"compIso":  compIso,
	}
	return r, byName
}

func refNames(es []catalog.Entity) []string {
	out := make([]string, 0, len(es))
	for _, e := range es {
		out = append(out, e.GetMetadata().Name)
	}
	sort.Strings(out)
	return out
}

func pickEntities(t *testing.T, byName map[string]catalog.Entity, names ...string) []catalog.Entity {
	t.Helper()
	out := make([]catalog.Entity, 0, len(names))
	for _, n := range names {
		e, ok := byName[n]
		if !ok {
			t.Fatalf("unknown entity %q in fixture", n)
		}
		out = append(out, e)
	}
	return out
}

func TestFullyConnectedGraph(t *testing.T) {
	r, byName := buildGraphFixture(t)

	tests := []struct {
		name  string
		roots []string
		want  []string
	}{
		{
			name:  "empty roots",
			roots: nil,
			want:  nil,
		},
		{
			name:  "single root returns just the root",
			roots: []string{"compA"},
			want:  []string{"compA"},
		},
		{
			name:  "isolated entity returns itself",
			roots: []string{"compIso"},
			want:  []string{"compIso"},
		},
		{
			name:  "two disconnected roots include only themselves",
			roots: []string{"compA", "compIso"},
			want:  []string{"compA", "compIso"},
		},
		{
			name:  "endpoints of a chain include intermediate nodes",
			roots: []string{"compA", "resR2"},
			want:  []string{"apiX", "compA", "compB", "compC", "resR1", "resR2"},
		},
		{
			name:  "endpoints exclude side branches off the path",
			roots: []string{"compA", "compB"},
			want:  []string{"apiX", "compA", "compB"},
		},
		{
			name:  "resource-to-resource dependsOn is traversed",
			roots: []string{"compC", "resR2"},
			want:  []string{"compC", "resR1", "resR2"},
		},
		{
			name:  "side-branch root pulls in its in-edge source",
			roots: []string{"compD", "resR2"},
			want:  []string{"compB", "compC", "compD", "resR1", "resR2"},
		},
	}

	opt := cmpopts.EquateEmpty()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			roots := pickEntities(t, byName, tc.roots...)
			got := refNames(FullyConnectedGraph(r, roots))
			sort.Strings(tc.want)
			if diff := cmp.Diff(tc.want, got, opt); diff != "" {
				t.Errorf("FullyConnectedGraph mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFullyConnectedGraph_Cycle(t *testing.T) {
	// Two components in a dependsOn cycle: compX <-> compY.
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "team"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "dom"},
		Spec:     &catalog.DomainSpec{Owner: owner.GetRef()},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "sys"},
		Spec:     &catalog.SystemSpec{Owner: owner.GetRef(), Domain: domain.GetRef()},
	}
	compX := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "compX"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
		},
	}
	compY := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "compY"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			DependsOn: []*catalog.LabelRef{{Ref: compX.GetRef()}},
		},
	}
	compX.Spec.DependsOn = []*catalog.LabelRef{{Ref: compY.GetRef()}}

	r := repo.NewRepository()
	for _, e := range []catalog.Entity{owner, domain, system, compX, compY} {
		if err := r.AddEntity(e); err != nil {
			t.Fatalf("AddEntity(%s): %v", e.GetRef(), err)
		}
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}

	got := refNames(FullyConnectedGraph(r, []catalog.Entity{compX}))
	want := []string{"compX", "compY"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("FullyConnectedGraph on cycle (-want +got):\n%s", diff)
	}
}
