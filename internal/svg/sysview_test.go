package svg

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/testutil"
)

// externalRenderer returns a renderer for the system external view. That view
// is laid out in process, so it needs no dot binary and these run as plain unit
// tests.
func externalRenderer(t *testing.T, dir string) (*Renderer, *repo.Repository) {
	t.Helper()
	r, err := repo.Load(store.NewDiskStore(dir), nil, repo.Config{})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	return NewRenderer(r, nil, DefaultConfig()), r
}

// The frontend resolves clicks, tooltips and routes from the SVG's ids and
// classes, which have to keep the shape graphviz produces for the other views.
func TestSystemExternalGraph_FrontendContract(t *testing.T) {
	renderer, r := externalRenderer(t, "../../testdata/test2")
	system1 := r.System(&catalog.Ref{Name: "test-system-1"})
	system2 := r.System(&catalog.Ref{Name: "test-system-2"})
	if system1 == nil || system2 == nil {
		t.Fatal("test systems not found")
	}

	viewOpts := NewSystemViewOptions([]*catalog.Ref{system2.GetRef()}, DetailAll)
	res, err := renderer.SystemExternalGraph(context.Background(), system1, viewOpts)
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}

	svg := string(res.SVG)
	if !strings.HasPrefix(svg, "<svg") {
		t.Fatalf("output does not start with <svg>: %.80s", svg)
	}
	classes, err := testutil.ExtractSVGClasses(res.SVG)
	if err != nil {
		t.Fatalf("ExtractSVGClasses: %v", err)
	}
	for _, want := range []string{"graphviz-svg", "clickable-node", "edge", "cluster"} {
		if !containsClass(classes, want) {
			t.Errorf("class %q missing from SVG; got %v", want, classes)
		}
	}

	ids, err := testutil.ExtractSVGIDs(res.SVG)
	if err != nil {
		t.Fatalf("ExtractSVGIDs: %v", err)
	}
	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	if !idSet["component:test-component"] {
		t.Errorf("expected node id %q in SVG; got %v", "component:test-component", ids)
	}

	// Every element the metadata describes must exist in the SVG, since the
	// frontend looks metadata up by element id.
	if len(res.Metadata.Nodes) == 0 || len(res.Metadata.Edges) == 0 {
		t.Fatalf("metadata is incomplete: %d nodes, %d edges",
			len(res.Metadata.Nodes), len(res.Metadata.Edges))
	}
	for id := range res.Metadata.Nodes {
		if !idSet[id] {
			t.Errorf("metadata node %q has no element in the SVG", id)
		}
	}
	for id := range res.Metadata.Edges {
		if !idSet[id] {
			t.Errorf("metadata edge %q has no element in the SVG", id)
		}
	}
	for id := range res.Metadata.Clusters {
		if !idSet[id] {
			t.Errorf("metadata cluster %q has no element in the SVG", id)
		}
	}
}

// At the APIs level no component or resource is drawn: their relationships are
// attributed to the system containing them, and duplicates collapse.
func TestDetailAPIsHidesComponentsBehindTheirSystem(t *testing.T) {
	renderer, r := externalRenderer(t, "../../testdata/test2")
	system1 := r.System(&catalog.Ref{Name: "test-system-1"})
	system2 := r.System(&catalog.Ref{Name: "test-system-2"})
	if system1 == nil || system2 == nil {
		t.Fatal("test systems not found")
	}

	opts := NewSystemViewOptions([]*catalog.Ref{system2.GetRef()}, DetailAll)
	withComponents, err := renderer.SystemExternalGraph(context.Background(), system1, opts)
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}
	if !hasComponent(withComponents.Metadata.Nodes) {
		t.Fatal("expected components to be drawn at the default level")
	}

	opts.Detail = DetailAPIs
	res, err := renderer.SystemExternalGraph(context.Background(), system1, opts)
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}
	if hasComponent(res.Metadata.Nodes) {
		t.Errorf("components are still drawn: %v", res.Metadata.Nodes)
	}
	for _, prefix := range []string{`id="component:`, `id="resource:`} {
		if strings.Contains(string(res.SVG), prefix) {
			t.Errorf("SVG still contains a %s element", strings.TrimSuffix(prefix, ":`"))
		}
	}
	// The relationships survive, attributed to a system.
	if len(res.Metadata.Edges) == 0 {
		t.Fatal("hiding components dropped every edge")
	}
	sawSystemEnd := false
	for _, e := range res.Metadata.Edges {
		if strings.HasPrefix(e.From, "component:") || strings.HasPrefix(e.To, "component:") {
			t.Errorf("edge %s -> %s still ends at a component", e.From, e.To)
		}
		if strings.HasPrefix(e.From, "system:") || strings.HasPrefix(e.To, "system:") {
			sawSystemEnd = true
		}
	}
	if !sawSystemEnd {
		t.Error("expected at least one edge to end at a system")
	}
	// Frames are layout anchors, not entities, so they get no metadata entry of
	// their own — every metadata node still has to exist in the SVG.
	ids, err := testutil.ExtractSVGIDs(res.SVG)
	if err != nil {
		t.Fatalf("ExtractSVGIDs: %v", err)
	}
	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}
	for id := range res.Metadata.Nodes {
		if !idSet[id] {
			t.Errorf("metadata node %q has no element in the SVG", id)
		}
	}
}

func hasComponent(nodes map[string]*dot.NodeInfo) bool {
	for id := range nodes {
		if strings.HasPrefix(id, "component:") {
			return true
		}
	}
	return false
}

func containsClass(classes []string, want string) bool {
	for _, c := range classes {
		for _, part := range strings.Fields(c) {
			if part == want {
				return true
			}
		}
	}
	return false
}

// Two boxes are connected by one arrow, whatever number of relationships it
// stands for: at a coarser level the entities that told them apart are not
// drawn, so several arrows between one pair would be indistinguishable.
func TestArrowsAggregatePerBoxPair(t *testing.T) {
	r := flightsRepo(t)
	sys := r.System(&catalog.Ref{Name: "flights-tickets"})
	renderer := NewRenderer(r, nil, DefaultConfig())

	for _, detail := range []DetailLevel{DetailSystems, DetailAPIs, DetailAll} {
		res, err := renderer.SystemExternalGraph(context.Background(), sys,
			NewSystemViewOptions(nil, detail))
		if err != nil {
			t.Fatalf("SystemExternalGraph failed: %v", err)
		}
		pairs := map[string]int{}
		for _, e := range res.Metadata.Edges {
			pairs[e.From+" -> "+e.To]++
		}
		for pair, n := range pairs {
			if n > 1 {
				t.Errorf("detail=%v: %s is drawn %d times, want once", detail, pair, n)
			}
		}
	}
}

// A label describes a relationship between two particular entities. It belongs
// on the arrow while those entities are the ones drawn, and nowhere else: at a
// coarser level the arrow connects something the label was never about.
func TestLabelsOnlyWhereTheirEntitiesAreDrawn(t *testing.T) {
	r := flightsRepo(t)
	sys := r.System(&catalog.Ref{Name: "flights-tickets"})
	renderer := NewRenderer(r, nil, DefaultConfig())

	for _, detail := range []DetailLevel{DetailSystems, DetailAPIs} {
		res, err := renderer.SystemExternalGraph(context.Background(), sys,
			NewSystemViewOptions(nil, detail))
		if err != nil {
			t.Fatalf("SystemExternalGraph failed: %v", err)
		}
		for _, e := range res.Metadata.Edges {
			if e.Label != "" {
				t.Errorf("detail=%v: arrow %s -> %s carries the label %q, "+
					"but the entities it describes are not drawn",
					detail, e.From, e.To, e.Label)
			}
			// Neither do the label's attributes.
			for _, a := range e.TooltipAttrs {
				t.Errorf("detail=%v: arrow %s -> %s carries %q: %q",
					detail, e.From, e.To, a.Key, a.Value)
			}
		}
	}
}

// With the parts themselves on screen, a labelled relationship describes itself.
func TestSingleRelationshipKeepsItsLabel(t *testing.T) {
	r := flightsRepo(t)
	sys := r.System(&catalog.Ref{Name: "flights-tickets"})
	renderer := NewRenderer(r, nil, DefaultConfig())

	res, err := renderer.SystemExternalGraph(context.Background(), sys,
		NewSystemViewOptions(nil, DetailAll))
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}
	for _, e := range res.Metadata.Edges {
		if e.Label != "" {
			return // a labelled arrow survived aggregation
		}
	}
	t.Error("no arrow kept its label")
}

// An arrow whose relationships are not drawn names the APIs they use, so that
// collapsing parts into their system does not lose which interfaces are
// involved.
func TestCollapsedArrowsNameTheirAPIs(t *testing.T) {
	r := flightsRepo(t)
	sys := r.System(&catalog.Ref{Name: "flights-tickets"})
	renderer := NewRenderer(r, nil, DefaultConfig())

	res, err := renderer.SystemExternalGraph(context.Background(), sys,
		NewSystemViewOptions(nil, DetailSystems))
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}
	got := map[string][]string{}
	for _, e := range res.Metadata.Edges {
		got[e.Title] = sectionItems(e.Sections)
	}
	want := map[string][]string{
		"flights-tickets → iam-system":       {"APIs: user-management-api"},
		"flights-search → flights-tickets":   {"APIs: flights-availability-api"},
		"flights-frontend → flights-tickets": {"APIs: flights-purchase-api"},
	}
	for title, items := range want {
		if !slices.Equal(got[title], items) {
			t.Errorf("%s lists %v, want %v", title, got[title], items)
		}
	}
}

// A domain arrow stands for every system pair between two domains, and lists
// the APIs of all of them, each once and by name.
func TestDomainArrowsNameTheirAPIs(t *testing.T) {
	r := flightsRepo(t)
	dom := r.Domain(&catalog.Ref{Name: "flights"})
	renderer := NewRenderer(r, nil, DefaultConfig())

	res, err := renderer.DomainExternalGraph(context.Background(), dom,
		NewDomainViewOptions(nil, DetailDomains))
	if err != nil {
		t.Fatalf("DomainExternalGraph failed: %v", err)
	}
	var got []string
	for _, e := range res.Metadata.Edges {
		if e.Title == "flights-tickets → external/ticket-vendors" {
			got = sectionItems(e.Sections)
		}
	}
	want := []string{
		"APIs: external/vendor-alpha-api",
		"APIs: external/vendor-beta-api",
		"APIs: external/vendor-gamma-api",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// What the diagram already draws is not listed again: an arrow that ends at an
// API box does not name that API in its tooltip.
func TestDrawnEndsAreNotListed(t *testing.T) {
	r := flightsRepo(t)
	sys := r.System(&catalog.Ref{Name: "flights-tickets"})
	renderer := NewRenderer(r, nil, DefaultConfig())

	res, err := renderer.SystemExternalGraph(context.Background(), sys,
		NewSystemViewOptions(nil, DetailAPIs))
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}
	for _, e := range res.Metadata.Edges {
		for _, sec := range e.Sections {
			for _, item := range sec.Items {
				if item.Ref == e.From || item.Ref == e.To {
					t.Errorf("arrow %s -> %s lists its own end %s", e.From, e.To, item.Ref)
				}
			}
		}
	}
}

// sectionItems flattens tooltip sections into "Title: label" strings.
func sectionItems(sections []dot.TooltipSection) []string {
	var out []string
	for _, sec := range sections {
		for _, item := range sec.Items {
			out = append(out, sec.Title+": "+item.Label)
		}
	}
	return out
}

// Two systems that depend on each other are connected by an arrow in each
// direction, and each arrow lists what is used in its own direction only:
// APIs, then components, then resources, each entity once however many parts
// depend on it. Incoming dependencies list the focal side's entities.
func TestMutualDependencyHasAnArrowPerDirection(t *testing.T) {
	renderer, r := externalRenderer(t, "../../testdata/mutual")
	aUses := []string{"APIs: b-api", "Components: b-store", "Resources: b-db"}
	bUses := []string{"APIs: a-api", "Components: a-cache", "Resources: a-db"}

	views := []struct {
		name   string
		render func() (*Result, error)
		want   map[string][]string
	}{
		{
			name: "system a, detail=systems",
			render: func() (*Result, error) {
				return renderer.SystemExternalGraph(context.Background(),
					r.System(&catalog.Ref{Name: "a"}), NewSystemViewOptions(nil, DetailSystems))
			},
			want: map[string][]string{"system:a -> system:b": aUses, "system:b -> system:a": bUses},
		},
		{
			name: "domain left, detail=domains",
			render: func() (*Result, error) {
				return renderer.DomainExternalGraph(context.Background(),
					r.Domain(&catalog.Ref{Name: "left"}), NewDomainViewOptions(nil, DetailDomains))
			},
			want: map[string][]string{"system:a -> domain:right": aUses, "domain:right -> system:a": bUses},
		},
		{
			name: "domain left, detail=systems",
			render: func() (*Result, error) {
				return renderer.DomainExternalGraph(context.Background(),
					r.Domain(&catalog.Ref{Name: "left"}), NewDomainViewOptions(nil, DetailSystems))
			},
			want: map[string][]string{"system:a -> system:b": aUses, "system:b -> system:a": bUses},
		},
	}
	for _, v := range views {
		res, err := v.render()
		if err != nil {
			t.Fatalf("%s: rendering failed: %v", v.name, err)
		}
		got := map[string][]string{}
		for _, e := range res.Metadata.Edges {
			got[e.From+" -> "+e.To] = sectionItems(e.Sections)
		}
		if len(got) != len(v.want) {
			t.Errorf("%s: got arrows %v, want %v", v.name, got, v.want)
		}
		for arrow, items := range v.want {
			if !slices.Equal(got[arrow], items) {
				t.Errorf("%s: %s lists %v, want %v", v.name, arrow, got[arrow], items)
			}
		}
	}
}

// A component with expanded APIs is drawn with the compact edge length; the
// plain view, and a view whose e= names none of its APIs, keep the normal one.
func TestComponentEdgeLengthFollowsExpansion(t *testing.T) {
	_, r := externalRenderer(t, "../../testdata/mutual")
	comp := r.Component(&catalog.Ref{Name: "a-service"})
	cfg := DefaultConfig()
	cfg.NormalEdgeMinLen, cfg.CompactEdgeMinLen = 3, 1
	rd := &render{Renderer: NewRenderer(r, nil, cfg), kind: DiagramComponent, focalEntity: comp}

	for _, tc := range []struct {
		name     string
		expanded []*catalog.Ref
		want     string
	}{
		{"plain", nil, `minlen="3"`},
		{"expanded", []*catalog.Ref{{Kind: catalog.KindAPI, Name: "b-api"}}, `minlen="1"`},
		{"unrelated ref", []*catalog.Ref{{Kind: catalog.KindAPI, Name: "elsewhere"}}, `minlen="3"`},
	} {
		src := rd.generateComponentDotSource(comp, &ComponentViewOptions{ExpandedAPIs: tc.expanded}).DotSource
		if !strings.Contains(src, tc.want) {
			t.Errorf("%s: dot source lacks %s:\n%s", tc.name, tc.want, src)
		}
	}
}
