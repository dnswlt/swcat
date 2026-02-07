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

func TestSystemExternalGraph_Topology(t *testing.T) {
	r := setupRepo(t)
	runner := &mockRunner{}
	renderer := NewRenderer(r, runner, NewStandardLayouter(Config{}))

	sysA := r.System(&catalog.Ref{Name: "sys-a"})
	if sysA == nil {
		t.Fatal("sys-a not found")
	}

	// Generate graph for System A
	_, err := renderer.SystemExternalGraph(context.Background(), sysA, nil, nil)
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}

	dot := runner.lastDotSource

	// 1. Verify Nodes
	// Should contain comp-a
	if !strings.Contains(dot, `"component:comp-a"[`) {
		t.Errorf("DOT missing node for comp-a")
	}
	// Should contain sys-b (collapsed node)
	if !strings.Contains(dot, `"system:sys-b"[`) {
		t.Errorf("DOT missing node for sys-b")
	}

	// 2. Verify Edges
	expectedEdge := `"component:comp-a" -> "system:sys-b"`
	if !strings.Contains(dot, expectedEdge) {
		t.Errorf("DOT missing edge: %s", expectedEdge)
	}
}

func TestSystemExternalGraph_Excluded(t *testing.T) {
	r := setupRepo(t)
	runner := &mockRunner{}
	renderer := NewRenderer(r, runner, NewStandardLayouter(Config{}))

	sysA := r.System(&catalog.Ref{Name: "sys-a"})
	sysB := r.System(&catalog.Ref{Name: "sys-b"})

	// Generate graph for System A, but exclude System B
	_, err := renderer.SystemExternalGraph(context.Background(), sysA, nil, []*catalog.Ref{sysB.GetRef()})
	if err != nil {
		t.Fatalf("SystemExternalGraph failed: %v", err)
	}

	dot := runner.lastDotSource

	// Should NOT contain comp-a because its only external dependency is excluded
	if strings.Contains(dot, `"component:comp-a"[`) {
		t.Errorf("DOT should not contain node for comp-a")
	}
	// Should NOT contain sys-b
	if strings.Contains(dot, `"system:sys-b"[`) {
		t.Errorf("DOT should not contain node for sys-b")
	}
	// Should NOT contain edge to sys-b
	if strings.Contains(dot, `"system:sys-b"`) {
		t.Errorf("DOT should not mention sys-b")
	}
}

func TestSystemInternalGraph_Topology(t *testing.T) {
	r := setupRepo(t)
	runner := &mockRunner{}
	renderer := NewRenderer(r, runner, NewStandardLayouter(Config{}))

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
	renderer := NewRenderer(r, runner, NewStandardLayouter(Config{}))

	compA := r.Component(&catalog.Ref{Name: "comp-a"})

	_, err := renderer.ComponentGraph(context.Background(), compA)
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

func TestGraph_Topology(t *testing.T) {
	r := setupRepo(t)
	runner := &mockRunner{}
	renderer := NewRenderer(r, runner, NewStandardLayouter(Config{}))

	domA := r.Domain(&catalog.Ref{Kind: catalog.KindDomain, Name: "dom-a"})
	sysA := r.System(&catalog.Ref{Kind: catalog.KindSystem, Name: "sys-a"})
	compA := r.Component(&catalog.Ref{Kind: catalog.KindComponent, Name: "comp-a"})
	apiB := r.API(&catalog.Ref{Kind: catalog.KindAPI, Name: "api-b"})

	entities := []catalog.Entity{domA, sysA, compA, apiB}

	_, err := renderer.Graph(context.Background(), entities)
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
