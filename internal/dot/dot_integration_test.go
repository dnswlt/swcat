//go:build integration

// Integration tests require "dot" to be installed and are skipped by default.
// Enable by running with
// go test -tags=integration ./internal/dot

package dot

import (
	"context"
	"testing"
	"time"

	"github.com/dnswlt/swcat/internal/testutil"
)

func TestDotRunner_Simple(t *testing.T) {
	r := NewRunner("dot")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dw := New()
	dw.Start()
	dw.AddNode(Node{ID: "a", Label: "A"})
	dw.AddNode(Node{ID: "b", Label: "B"})
	dw.AddEdge(Edge{From: "a", To: "b", Style: ESSystemLink})
	dw.End()

	svg, err := r.Run(ctx, dw.Result().DotSource)
	if err != nil {
		t.Fatalf("dot failed: %v\n%s", err, svg)
	}

	ids, err := testutil.ExtractSVGIDs(svg)
	if err != nil {
		t.Fatalf("failed to parse SVG: %v", err)
	}
	idSet := map[string]bool{}
	for _, id := range ids {
		idSet[id] = true
	}

	// Check that id= attributes are set.
	expectedIDs := []string{"a", "b", "svg-edge-0"} // nodes + our synthetic edge id
	var missingIDs []string
	for _, want := range expectedIDs {
		if !idSet[want] {
			missingIDs = append(missingIDs, want)
		}
	}
	if len(missingIDs) > 0 {
		t.Fatalf("missing expected ids: %v\n(all ids: %v)", missingIDs, ids)
	}

	// Check that class= attributes are set.
	classes, err := testutil.ExtractSVGClasses(svg)
	if err != nil {
		t.Fatalf("failed to parse SVG for classes: %v", err)
	}
	classSet := map[string]bool{}
	for _, c := range classes {
		classSet[c] = true
	}
	expectedClasses := []string{"clickable-node", "clickable-edge"}
	var missingClasses []string
	for _, want := range expectedClasses {
		if !classSet[want] {
			missingClasses = append(missingClasses, want)
		}
	}
	if len(missingClasses) > 0 {
		t.Fatalf("missing expected classes %v\n(all classes: %v)", missingClasses, classes)
	}
}
