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
	_, err := renderer.SystemExternalGraph(context.Background(), sysA, nil)
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
	// Should NOT contain api-b node (it's inside sys-b which is collapsed)
	if strings.Contains(dot, `"api:api-b"[`) {
		t.Errorf("DOT should not contain node for api-b")
	}

	// 2. Verify Edges
	// Edge from comp-a to sys-b
	// The edge generation logic in svg.go uses:
	// dw.AddEdge(r.entityEdge(dep.source, dep.targetSystem, dot.ESSystemLink))
	// So: "component:comp-a" -> "system:sys-b"
	expectedEdge := `"component:comp-a" -> "system:sys-b"`
	if !strings.Contains(dot, expectedEdge) {
		t.Errorf("DOT missing edge: %s", expectedEdge)
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
