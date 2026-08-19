package svg

import (
	"context"
	"strconv"
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
			// A count of what it covers is not a claim about what it is.
			for _, a := range e.TooltipAttrs {
				if a.Key != "relationships" {
					t.Errorf("detail=%v: arrow %s -> %s carries %q: %q",
						detail, e.From, e.To, a.Key, a.Value)
				}
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

// Every arrow whose relationships are not drawn reports how many it stands for,
// one included: an arrow that said nothing could not be told apart from one that
// is the relationship it draws.
func TestCollapsedArrowsAlwaysSayHowMany(t *testing.T) {
	r := flightsRepo(t)
	sys := r.System(&catalog.Ref{Name: "flights-tickets"})
	renderer := NewRenderer(r, nil, DefaultConfig())

	res, err := renderer.SystemExternalGraph(context.Background(), sys,
		NewSystemViewOptions(nil, DetailAPIs))
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}

	ones, more := 0, 0
	for _, e := range res.Metadata.Edges {
		if len(e.TooltipAttrs) != 1 || e.TooltipAttrs[0].Key != "relationships" {
			t.Fatalf("arrow %s -> %s reports %v, want a relationship count",
				e.From, e.To, e.TooltipAttrs)
		}
		n, err := strconv.Atoi(e.TooltipAttrs[0].Value)
		if err != nil || n < 1 {
			t.Errorf("arrow %s -> %s reports %q relationships", e.From, e.To, e.TooltipAttrs[0].Value)
		}
		if n == 1 {
			ones++
		} else {
			more++
		}
	}
	if ones == 0 || more == 0 {
		t.Errorf("expected both kinds of arrow at this level, got %d single and %d collapsed",
			ones, more)
	}
}
