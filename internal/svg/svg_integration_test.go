//go:build integration

package svg

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/testutil"
)

func TestGenerateComponentSVG_WithDot(t *testing.T) {
	repo, err := repo.Load(store.NewDiskStore("../../testdata/catalog"), repo.Config{}, "")
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}

	comp := repo.Component(&catalog.Ref{Name: "test-component"})
	if comp == nil {
		t.Fatalf("test-component not found in repo")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	renderer := NewRenderer(repo, dot.NewRunner("dot"), NewStandardLayouter(Config{}))
	res, err := renderer.ComponentGraph(ctx, comp)
	if err != nil {
		t.Fatalf("GenerateComponentSVG failed: %v", err)
	}
	if !bytes.Contains(res.SVG, []byte("<svg")) {
		t.Fatalf("SVG output missing <svg> tag:\n%s", string(res.SVG))
	}

	// Structural checks on the produced SVG
	ids, err := testutil.ExtractSVGIDs(res.SVG)
	if err != nil {
		t.Fatalf("extractIDs: %v", err)
	}

	// Expect the component node id to be present
	foundComp := slices.Contains(ids, "component:test-component")
	if !foundComp {
		t.Fatalf("expected node id %q not found; all ids: %v", "test-component", ids)
	}

	// Expect at least one edge id like svg-edge-*
	foundEdge := false
	for _, id := range ids {
		if strings.HasPrefix(id, "svg-edge-") {
			foundEdge = true
			break
		}
	}
	if !foundEdge {
		t.Fatalf("expected at least one edge id (svg-edge-*) in SVG; ids: %v", ids)
	}

	classes, err := testutil.ExtractSVGClasses(res.SVG)
	if err != nil {
		t.Fatalf("extractClasses: %v", err)
	}
	hasClickableNode := slices.Contains(classes, "clickable-node")
	if !hasClickableNode {
		t.Fatalf("expected class %q not found; classes: %v", "clickable-node", classes)
	}
}

func TestSystemExternalGraph_WithDot(t *testing.T) {
	repo, err := repo.Load(store.NewDiskStore("../../testdata/catalog2"), repo.Config{}, "")
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}

	system1 := repo.System(&catalog.Ref{Name: "test-system-1"})
	if system1 == nil {
		t.Fatalf("test-system-1 not found in repo")
	}
	system2 := repo.System(&catalog.Ref{Name: "test-system-2"})
	if system1 == nil {
		t.Fatalf("test-system-2 not found in repo")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	renderer := NewRenderer(repo, dot.NewRunner("dot"), NewStandardLayouter(Config{}))
	res, err := renderer.SystemExternalGraph(ctx, system1, []*catalog.System{system2})
	if err != nil {
		t.Fatalf("GenerateSystemSVG failed: %v", err)
	}
	if !bytes.Contains(res.SVG, []byte("<svg")) {
		t.Fatalf("SVG output missing <svg> tag:\n%s", string(res.SVG))
	}

	// Structural checks on the produced SVG
	ids, err := testutil.ExtractSVGIDs(res.SVG)
	if err != nil {
		t.Fatalf("extractIDs: %v", err)
	}

	// Expect the component node id to be present
	// NOTE: the system itself is not present, since it is represented as a "cluster",
	// not a node.
	foundComp := slices.Contains(ids, "component:test-component")
	if !foundComp {
		t.Fatalf("expected node id %q not found; all ids: %v", "test-component", ids)
	}

}

func TestSystemInternalGraph_WithDot(t *testing.T) {
	repo, err := repo.Load(store.NewDiskStore("../../testdata/catalog"), repo.Config{}, "")
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}

	system := repo.System(&catalog.Ref{Name: "test-system"})
	if system == nil {
		t.Fatalf("test-system not found in repo")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	renderer := NewRenderer(repo, dot.NewRunner("dot"), NewStandardLayouter(Config{}))
	res, err := renderer.SystemInternalGraph(ctx, system)
	if err != nil {
		t.Fatalf("GenerateSystemSVG failed: %v", err)
	}
	if !bytes.Contains(res.SVG, []byte("<svg")) {
		t.Fatalf("SVG output missing <svg> tag:\n%s", string(res.SVG))
	}

	// Structural checks on the produced SVG
	ids, err := testutil.ExtractSVGIDs(res.SVG)
	if err != nil {
		t.Fatalf("extractIDs: %v", err)
	}

	// Expect the component node id to be present
	// NOTE: the system itself is not present, since it is represented as a "cluster",
	// not a node.
	foundComp := slices.Contains(ids, "component:test-component")
	if !foundComp {
		t.Fatalf("expected node id %q not found; all ids: %v", "test-component", ids)
	}

}
