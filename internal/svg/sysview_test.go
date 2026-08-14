package svg

import (
	"context"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/testutil"
)

// nativeRenderer returns a renderer configured for the built-in system external
// layout. It needs no dot binary, so it can run as a plain unit test.
func nativeRenderer(t *testing.T, dir string) (*Renderer, *repo.Repository) {
	t.Helper()
	r, err := repo.Load(store.NewDiskStore(dir), nil, repo.Config{})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	cfg := DefaultConfig()
	cfg.SystemExternalRenderer = RendererNative
	return NewRenderer(r, nil, cfg), r
}

// The frontend resolves clicks, tooltips and routes from the SVG's ids and
// classes, so the native renderer has to produce the same shape as the
// dot-based one.
func TestNativeSystemExternalGraph_FrontendContract(t *testing.T) {
	renderer, r := nativeRenderer(t, "../../testdata/test2")
	system1 := r.System(&catalog.Ref{Name: "test-system-1"})
	system2 := r.System(&catalog.Ref{Name: "test-system-2"})
	if system1 == nil || system2 == nil {
		t.Fatal("test systems not found")
	}

	viewOpts := NewSystemViewOptions(r, system1, nil, []*catalog.Ref{system2.GetRef()}, nil)
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

// Both renderers are fed by the same traversal, so they must show the same
// entities and relationships — only their geometry differs.
func TestNativeAndDotShowTheSameContent(t *testing.T) {
	renderer, r := nativeRenderer(t, "../../testdata/test2")
	system1 := r.System(&catalog.Ref{Name: "test-system-1"})
	system2 := r.System(&catalog.Ref{Name: "test-system-2"})
	if system1 == nil || system2 == nil {
		t.Fatal("test systems not found")
	}
	viewOpts := NewSystemViewOptions(r, system1, nil, []*catalog.Ref{system2.GetRef()}, nil)

	rd := &render{Renderer: renderer, kind: DiagramSystem, focalEntity: system1}
	m := rd.collectSystemExternal(system1, viewOpts)
	_, nativeMeta := rd.buildSystemExternalDiagram(m)
	dotMeta := rd.generateSystemExternalDotSource(system1, viewOpts).Metadata

	if len(nativeMeta.Nodes) != len(dotMeta.Nodes) {
		t.Errorf("node count differs: native %d, dot %d", len(nativeMeta.Nodes), len(dotMeta.Nodes))
	}
	for id := range dotMeta.Nodes {
		if _, ok := nativeMeta.Nodes[id]; !ok {
			t.Errorf("node %q missing from the native rendering", id)
		}
	}
	if len(nativeMeta.Edges) != len(dotMeta.Edges) {
		t.Errorf("edge count differs: native %d, dot %d", len(nativeMeta.Edges), len(dotMeta.Edges))
	}
	dotLinks := map[string]bool{}
	for _, e := range dotMeta.Edges {
		dotLinks[e.From+" -> "+e.To] = true
	}
	for _, e := range nativeMeta.Edges {
		if !dotLinks[e.From+" -> "+e.To] {
			t.Errorf("edge %s -> %s is not in the dot rendering", e.From, e.To)
		}
	}
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
