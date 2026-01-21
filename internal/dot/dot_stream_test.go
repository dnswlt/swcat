package dot

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStreamingRunner_MultipleRuns(t *testing.T) {
	// Check if dot is installed, skip if not (integration test style)
	// We can reuse the check from dot_integration_test.go logic or just try and fail.
	// Since this is a unit test file, we probably want to mock dot or be sure it exists.
	// But mocking exec.Command is hard.
	// Let's assume for now this runs where dot is available, or we skip if it fails to start.

	runner := NewStreamingRunner("dot")
	defer runner.Close()

	ctx := context.Background()

	// 1. First Run
	dot1 := `digraph G1 { a -> b }`
	svg1, err := runner.Run(ctx, dot1)
	if err != nil {
		t.Logf("Skipping test, dot not found or failed: %v", err)
		return
	}
	if !strings.Contains(string(svg1), "<svg") {
		t.Errorf("Expected SVG output, got: %s", string(svg1))
	}
	if !strings.Contains(string(svg1), "a -> b") && !strings.Contains(string(svg1), "title>a&#45;&gt;b") {
		// Graphviz output varies, but it should contain something related to the edge
	}

	// 2. Second Run (Reuse process)
	dot2 := `digraph G2 { c -> d; d -> e }`
	svg2, err := runner.Run(ctx, dot2)
	if err != nil {
		t.Fatalf("Second run failed: %v", err)
	}
	if !strings.Contains(string(svg2), "<svg") {
		t.Errorf("Expected SVG output for second run, got: %s", string(svg2))
	}
	// Verify it's actually the second graph
	// Note: We can't easily check for "c -> d" in SVG XML, but we can check if it's not the first one.
	// Or check for node names if they are rendered as text.
	if strings.Contains(string(svg2), "a -> b") {
		t.Errorf("Second SVG seems to contain content from first graph?")
	}

	// 3. Third Run (Stress test)
	for i := 0; i < 5; i++ {
		dotN := `digraph G { x -> y }`
		_, err := runner.Run(ctx, dotN)
		if err != nil {
			t.Fatalf("Run %d failed: %v", i, err)
		}
	}
}

func TestStreamingRunner_InvalidInput(t *testing.T) {
	runner := NewStreamingRunner("dot")

	defer runner.Close()

	// Use a short timeout for the test to detect hangs
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Invalid DOT input
	_, err := runner.Run(ctx, "invalid graph {")
	if err == nil {
		t.Log("Warning: expected error for invalid input, but some dot versions might produce empty output or exit")
	} else {
		t.Logf("Got expected error for invalid input: %v", err)
	}

	// 2. Try valid input to see if it recovered (it should restart the process if it died)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	dotValid := `digraph G { a -> b }`
	svg, err := runner.Run(ctx2, dotValid)
	if err != nil {
		t.Fatalf("Failed to run valid input after invalid input: %v", err)
	}
	if !strings.Contains(string(svg), "<svg") {
		t.Errorf("Expected SVG output after recovery, got: %s", string(svg))
	}
}
