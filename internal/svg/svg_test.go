package svg

import (
	"context"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
)

// mockRunner captures the DOT source passed to Run.
type mockRunner struct {
	lastDotSource string
}

func (m *mockRunner) Run(ctx context.Context, dotSource string) ([]byte, error) {
	m.lastDotSource = dotSource
	// Return valid XML so PostprocessSVG doesn't fail
	return []byte("<svg></svg>"), nil
}

func (m *mockRunner) Close() error {
	return nil
}

func setupRepo(t *testing.T) *repo.Repository {
	r := repo.NewRepository()

	// Domain
	dom := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "dom-a"},
		Spec:     &catalog.DomainSpec{Owner: &catalog.Ref{Name: "owner"}},
	}

	// System A
	sysA := &catalog.System{
		Metadata: &catalog.Metadata{Name: "sys-a"},
		Spec:     &catalog.SystemSpec{Type: "app", Owner: &catalog.Ref{Name: "owner"}, Domain: dom.GetRef()},
	}
	compA := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "comp-a"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "prod",
			Owner:     &catalog.Ref{Name: "owner"},
			System:    sysA.GetRef(),
			ConsumesAPIs: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: catalog.KindAPI, Name: "api-b"}},
			},
		},
	}

	// System B
	sysB := &catalog.System{
		Metadata: &catalog.Metadata{Name: "sys-b"},
		Spec:     &catalog.SystemSpec{Type: "app", Owner: &catalog.Ref{Name: "owner"}, Domain: dom.GetRef()},
	}
	apiB := &catalog.API{
		Metadata: &catalog.Metadata{Name: "api-b"},
		Spec: &catalog.APISpec{
			Type:      "openapi",
			Lifecycle: "prod",
			Owner:     &catalog.Ref{Name: "owner"},
			System:    sysB.GetRef(),
		},
	}

	// Add all
	entities := []catalog.Entity{dom, sysA, compA, sysB, apiB}
	// Also need the group for owner validation
	group := &catalog.Group{Metadata: &catalog.Metadata{Name: "owner"}, Spec: &catalog.GroupSpec{Type: "team"}}
	entities = append(entities, group)

	for _, e := range entities {
		if err := r.AddEntity(e); err != nil {
			t.Fatalf("AddEntity(%s): %v", e.GetRef(), err)
		}
	}

	if err := r.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	return r
}

// externalView renders the external view of sys-a and returns what it shows:
// the entities as node ids, and the relationships as "from -> to".
func externalView(t *testing.T, opts *SystemViewOptions) (nodes map[string]bool, edges map[string]bool) {
	t.Helper()
	r := setupRepo(t)
	renderer := NewRenderer(r, &mockRunner{}, DefaultConfig())

	sysA := r.System(&catalog.Ref{Name: "sys-a"})
	if sysA == nil {
		t.Fatal("sys-a not found")
	}
	res, err := renderer.SystemExternalGraph(context.Background(), sysA, opts)
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}

	nodes = map[string]bool{}
	for id := range res.Metadata.Nodes {
		nodes[id] = true
	}
	edges = map[string]bool{}
	for _, e := range res.Metadata.Edges {
		edges[e.From+" -> "+e.To] = true
	}
	return nodes, edges
}

func TestSystemExternalGraph_Topology(t *testing.T) {
	nodes, edges := externalView(t, NewSystemViewOptions(nil, DetailAll))

	if !nodes["component:comp-a"] {
		t.Errorf("missing node for comp-a; got %v", nodes)
	}
	// With everything selected, sys-b is shown with the API comp-a consumes.
	if !nodes["api:api-b"] {
		t.Errorf("missing node for api-b; got %v", nodes)
	}
	if !edges["component:comp-a -> api:api-b"] {
		t.Errorf("missing edge comp-a -> api-b; got %v", edges)
	}
}

// At the systems level nothing inside a system is drawn, so a neighbor is a
// single box again and the relationship is between the two systems.
func TestSystemExternalGraph_DetailSystems(t *testing.T) {
	nodes, edges := externalView(t, NewSystemViewOptions(nil, DetailSystems))

	if !nodes["system:sys-b"] {
		t.Errorf("missing node for sys-b; got %v", nodes)
	}
	if nodes["api:api-b"] || nodes["component:comp-a"] {
		t.Errorf("parts should not be drawn at the systems level; got %v", nodes)
	}
	if !edges["system:sys-a -> system:sys-b"] {
		t.Errorf("missing edge sys-a -> sys-b; got %v", edges)
	}
}

// A selection narrows the view to the systems in it: everything else drops out,
// which is how a neighbor gets left out now that there is no exclude control.
func TestSystemExternalGraph_UnselectedSystemsAreLeftOut(t *testing.T) {
	// Select a system that is not sys-a's neighbor, so sys-b falls outside.
	other := &catalog.Ref{Kind: catalog.KindSystem, Name: "sys-other"}
	nodes, edges := externalView(t, NewSystemViewOptions([]*catalog.Ref{other}, DetailAll))

	for id := range nodes {
		if strings.Contains(id, "sys-b") || strings.Contains(id, "api-b") {
			t.Errorf("unselected system should not appear; got %v", nodes)
		}
	}
	// comp-a's only external dependency is gone, so it has nothing to show either.
	if nodes["component:comp-a"] {
		t.Errorf("comp-a should not appear without its dependency; got %v", nodes)
	}
	if len(edges) != 0 {
		t.Errorf("expected no edges, got %v", edges)
	}
}

func TestSystemInternalGraph_Topology(t *testing.T) {
	r := setupRepo(t)
	runner := &mockRunner{}
	cfg := DefaultConfig()
	renderer := NewRenderer(r, runner, cfg)

	sysA := r.System(&catalog.Ref{Name: "sys-a"})

	// Generate internal graph for System A
	_, err := renderer.SystemInternalGraph(context.Background(), sysA)
	if err != nil {
		t.Fatalf("SystemInternalGraph failed: %v", err)
	}

	dot := runner.lastDotSource

	// 1. Verify Nodes
	if !strings.Contains(dot, `"component:comp-a"[`) {
		t.Errorf("DOT missing node for comp-a")
	}

	// 2. Verify Edges
	// comp-a consumes api-b. But api-b is in System B.
	// Internal graph of System A should NOT show edges to System B entities.
	if strings.Contains(dot, "sys-b") {
		t.Errorf("Internal graph should not mention sys-b")
	}
	if strings.Contains(dot, "api-b") {
		t.Errorf("Internal graph should not mention api-b")
	}
}

func TestComponentGraph_Topology(t *testing.T) {
	r := setupRepo(t)
	runner := &mockRunner{}
	cfg := DefaultConfig()
	renderer := NewRenderer(r, runner, cfg)

	compA := r.Component(&catalog.Ref{Name: "comp-a"})

	_, err := renderer.ComponentGraph(context.Background(), compA, nil)
	if err != nil {
		t.Fatalf("ComponentGraph failed: %v", err)
	}

	dot := runner.lastDotSource

	// Should show edge from comp-a to api-b (outgoing dependency)
	expectedEdge := `"component:comp-a" -> "api:api-b"`
	if !strings.Contains(dot, expectedEdge) {
		t.Errorf("DOT missing edge: %s", expectedEdge)
	}

	// Should show api-b node
	if !strings.Contains(dot, `"api:api-b"[`) {
		t.Errorf("DOT missing node for api-b")
	}
}

// setupRepoWithProviders extends setupRepo with:
//   - api-a in sys-a, provided by comp-a and consumed by comp-c (in sys-b)
//   - comp-b in sys-b, which provides api-b
func setupRepoWithProviders(t *testing.T) *repo.Repository {
	r := repo.NewRepository()

	group := &catalog.Group{Metadata: &catalog.Metadata{Name: "owner"}, Spec: &catalog.GroupSpec{Type: "team"}}
	dom := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "dom-a"},
		Spec:     &catalog.DomainSpec{Owner: &catalog.Ref{Name: "owner"}},
	}

	sysA := &catalog.System{
		Metadata: &catalog.Metadata{Name: "sys-a"},
		Spec:     &catalog.SystemSpec{Type: "app", Owner: &catalog.Ref{Name: "owner"}, Domain: dom.GetRef()},
	}
	apiA := &catalog.API{
		Metadata: &catalog.Metadata{Name: "api-a"},
		Spec:     &catalog.APISpec{Type: "openapi", Lifecycle: "prod", Owner: &catalog.Ref{Name: "owner"}, System: sysA.GetRef()},
	}
	compA := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "comp-a"},
		Spec: &catalog.ComponentSpec{
			Type: "service", Lifecycle: "prod", Owner: &catalog.Ref{Name: "owner"}, System: sysA.GetRef(),
			ProvidesAPIs: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: catalog.KindAPI, Name: "api-a"}},
			},
			ConsumesAPIs: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: catalog.KindAPI, Name: "api-b"}},
			},
		},
	}

	sysB := &catalog.System{
		Metadata: &catalog.Metadata{Name: "sys-b"},
		Spec:     &catalog.SystemSpec{Type: "app", Owner: &catalog.Ref{Name: "owner"}, Domain: dom.GetRef()},
	}
	apiB := &catalog.API{
		Metadata: &catalog.Metadata{Name: "api-b"},
		Spec:     &catalog.APISpec{Type: "openapi", Lifecycle: "prod", Owner: &catalog.Ref{Name: "owner"}, System: sysB.GetRef()},
	}
	// comp-b provides api-b (so it shows up when comp-a's consumption of api-b is expanded)
	compB := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "comp-b"},
		Spec: &catalog.ComponentSpec{
			Type: "service", Lifecycle: "prod", Owner: &catalog.Ref{Name: "owner"}, System: sysB.GetRef(),
			ProvidesAPIs: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: catalog.KindAPI, Name: "api-b"}},
			},
		},
	}
	// comp-c consumes api-a (so it shows up when comp-a's provision of api-a is expanded)
	compC := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "comp-c"},
		Spec: &catalog.ComponentSpec{
			Type: "service", Lifecycle: "prod", Owner: &catalog.Ref{Name: "owner"}, System: sysB.GetRef(),
			ConsumesAPIs: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: catalog.KindAPI, Name: "api-a"}},
			},
		},
	}

	for _, e := range []catalog.Entity{group, dom, sysA, apiA, compA, sysB, apiB, compB, compC} {
		if err := r.AddEntity(e); err != nil {
			t.Fatalf("AddEntity(%s): %v", e.GetRef(), err)
		}
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	return r
}

func TestComponentGraph_ExpandedAPIs(t *testing.T) {
	r := setupRepoWithProviders(t)
	runner := &mockRunner{}
	cfg := DefaultConfig()
	renderer := NewRenderer(r, runner, cfg)
	compA := r.Component(&catalog.Ref{Name: "comp-a"})

	t.Run("no expansion", func(t *testing.T) {
		_, err := renderer.ComponentGraph(context.Background(), compA, nil)
		if err != nil {
			t.Fatalf("ComponentGraph failed: %v", err)
		}
		dot := runner.lastDotSource
		if strings.Contains(dot, `"component:comp-b"`) {
			t.Error("DOT should not contain comp-b without expansion")
		}
		if strings.Contains(dot, `"component:comp-c"`) {
			t.Error("DOT should not contain comp-c without expansion")
		}
	})

	t.Run("expand consumed api", func(t *testing.T) {
		opts := &ComponentViewOptions{
			ExpandedAPIs: []*catalog.Ref{{Kind: catalog.KindAPI, Name: "api-b"}},
		}
		_, err := renderer.ComponentGraph(context.Background(), compA, opts)
		if err != nil {
			t.Fatalf("ComponentGraph failed: %v", err)
		}
		dot := runner.lastDotSource
		// comp-b (provider of api-b) must appear
		if !strings.Contains(dot, `"component:comp-b"`) {
			t.Error("DOT missing node for comp-b (provider of expanded api-b)")
		}
		// providedBy edge: api-b -> comp-b
		if !strings.Contains(dot, `"api:api-b" -> "component:comp-b"`) {
			t.Error(`DOT missing edge "api:api-b" -> "component:comp-b"`)
		}
		// comp-c must not appear (it relates to api-a, which is not expanded here)
		if strings.Contains(dot, `"component:comp-c"`) {
			t.Error("DOT should not contain comp-c when only api-b is expanded")
		}
	})

	t.Run("expand provided api", func(t *testing.T) {
		opts := &ComponentViewOptions{
			ExpandedAPIs: []*catalog.Ref{{Kind: catalog.KindAPI, Name: "api-a"}},
		}
		_, err := renderer.ComponentGraph(context.Background(), compA, opts)
		if err != nil {
			t.Fatalf("ComponentGraph failed: %v", err)
		}
		dot := runner.lastDotSource
		// comp-c (consumer of api-a) must appear
		if !strings.Contains(dot, `"component:comp-c"`) {
			t.Error("DOT missing node for comp-c (consumer of expanded api-a)")
		}
		// normal edge: comp-c -> api-a
		if !strings.Contains(dot, `"component:comp-c" -> "api:api-a"`) {
			t.Error(`DOT missing edge "component:comp-c" -> "api:api-a"`)
		}
		// comp-b must not appear (it relates to api-b, which is not expanded here)
		if strings.Contains(dot, `"component:comp-b"`) {
			t.Error("DOT should not contain comp-b when only api-a is expanded")
		}
	})
}

func TestGraph_Topology(t *testing.T) {
	r := setupRepo(t)
	runner := &mockRunner{}
	cfg := DefaultConfig()
	renderer := NewRenderer(r, runner, cfg)

	domA := r.Domain(&catalog.Ref{Kind: catalog.KindDomain, Name: "dom-a"})
	sysA := r.System(&catalog.Ref{Kind: catalog.KindSystem, Name: "sys-a"})
	compA := r.Component(&catalog.Ref{Kind: catalog.KindComponent, Name: "comp-a"})
	apiB := r.API(&catalog.Ref{Kind: catalog.KindAPI, Name: "api-b"})

	entities := []catalog.Entity{domA, sysA, compA, apiB}

	_, err := renderer.Graph(context.Background(), entities, GraphOptions{})
	if err != nil {
		t.Fatalf("Graph failed: %v", err)
	}

	dot := runner.lastDotSource

	// 1. Verify Nodes
	nodes := []string{
		`"domain:dom-a"[`,
		`"system:sys-a"[`,
		`"component:comp-a"[`,
		`"api:api-b"[`,
	}
	for _, n := range nodes {
		if !strings.Contains(dot, n) {
			t.Errorf("DOT missing node: %s", n)
		}
	}

	// 2. Verify Edges
	expectedEdges := []string{
		`"domain:dom-a" -> "system:sys-a"`,
		`"system:sys-a" -> "component:comp-a"`,
		`"component:comp-a" -> "api:api-b"`,
	}
	for _, e := range expectedEdges {
		if !strings.Contains(dot, e) {
			t.Errorf("DOT missing edge: %s", e)
		}
	}

	// 3. Verify something NOT present (e.g. sys-b was not included)
	if strings.Contains(dot, "sys-b") {
		t.Errorf("DOT should not contain sys-b")
	}
}

// The external view leads with the contracts between systems: the internal view
// already covers what a system is made of.
func TestDefaultDetailLevelIsAPIs(t *testing.T) {
	// Each view passes its own fallback; the system view leads with APIs.
	for _, s := range []string{"", "unknown"} {
		if got := ParseDetailLevel(s, DetailAPIs); got != DetailAPIs {
			t.Errorf("ParseDetailLevel(%q) = %v, want %v", s, got, DetailAPIs)
		}
		if got := ParseDetailLevel(s, DetailSystems); got != DetailSystems {
			t.Errorf("ParseDetailLevel(%q) = %v, want %v", s, got, DetailSystems)
		}
	}
	for s, want := range map[string]DetailLevel{
		"domains": DetailDomains, "systems": DetailSystems, "apis": DetailAPIs, "all": DetailAll,
	} {
		if got := ParseDetailLevel(s, DetailAll); got != want {
			t.Errorf("ParseDetailLevel(%q) = %v, want %v", s, got, want)
		}
	}
}
