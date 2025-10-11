//go:build integration

package backstage

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/testutil"
)

func TestGenerateComponentSVG_WithDot(t *testing.T) {
	repo, err := LoadRepositoryFromPaths([]string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}

	comp := repo.Component(&api.Ref{Name: "test-component"})
	if comp == nil {
		t.Fatalf("test-component not found in repo")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := GenerateComponentSVG(ctx, dot.NewRunner("dot"), repo, comp)
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

func TestGenerateSystemSVG_WithDot(t *testing.T) {
	repo, err := LoadRepositoryFromPaths([]string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}

	system := repo.System(&api.Ref{Name: "test-system"})
	if system == nil {
		t.Fatalf("test-system not found in repo")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := GenerateSystemSVG(ctx, dot.NewRunner("dot"), repo, system, nil)
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
