package sysview

import (
	"math"
	"testing"
)

// Widths graphviz computed for the same strings at font size 11 when rendering
// the flights example (derived from node box widths minus 8pt padding per side).
// They are a cross-check that the embedded metrics are sane, not an exact
// contract: graphviz measures with its own copy of Noto Sans and applies kerning.
func TestTextWidthMatchesGraphvizWithinTolerance(t *testing.T) {
	tests := []struct {
		s    string
		want float64
	}{
		{"availability-aggregator", 115.5},
		{"internal-payment-handler", 132.0},
		{"flights-availability-api", 109.5},
		{"external/vendor-alpha", 114.0},
	}
	for _, tc := range tests {
		got := TextWidth(tc.s, 11)
		if rel := math.Abs(got-tc.want) / tc.want; rel > 0.05 {
			t.Errorf("TextWidth(%q, 11) = %.2f, graphviz measured %.2f (%.1f%% off)",
				tc.s, got, tc.want, rel*100)
		}
	}
}

func TestTextWidthCoversASCII(t *testing.T) {
	m := fontMetrics()
	if len(m) == 0 {
		t.Fatal("no font metrics loaded from embedded WOFF files")
	}
	for r := rune(' '); r <= '~'; r++ {
		if _, ok := m[r]; !ok {
			t.Errorf("no advance width for %q", r)
		}
	}
	// Latin-ext coverage, e.g. for names with umlauts.
	for _, r := range "äöüßéèçñ" {
		if _, ok := m[r]; !ok {
			t.Errorf("no advance width for %q", r)
		}
	}
}
