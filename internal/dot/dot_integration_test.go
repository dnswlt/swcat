// Integration tests require "dot" to be installed and are skipped by default.
// Enable by running with
// go test ./internal/dot -integration

package dot

import (
	"bytes"
	"context"
	"encoding/xml"
	"flag"
	"io"
	"sort"
	"strings"
	"testing"
	"time"
)

var integration = flag.Bool("integration", false, "run integration tests")

func extractIDs(svg []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(svg))
	var ids []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return ids, nil
		}
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			for _, a := range se.Attr {
				if a.Name.Local == "id" {
					// xml.Decoder already entity-decodes (e.g., &#45; -> -)
					ids = append(ids, a.Value)
				}
			}
		}
	}
}

func extractClasses(svg []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(svg))
	set := make(map[string]struct{})
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			for _, a := range se.Attr {
				if a.Name.Local == "class" {
					for _, c := range strings.Fields(a.Value) { // splits by any whitespace
						if c != "" {
							set[c] = struct{}{}
						}
					}
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

func TestDotRunner_Simple(t *testing.T) {
	if !*integration {
		t.Skip("pass -integration to run integration tests")
	}

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

	ids, err := extractIDs(svg)
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
	classes, err := extractClasses(svg)
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
