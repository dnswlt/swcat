package dot

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestWriter_AddNode_DuplicateIgnored(t *testing.T) {
	dw := New()
	dw.Start()
	dw.AddNode(Node{ID: "x", Kind: KindComponent, Label: "X"})
	dw.AddNode(Node{ID: "x", Kind: KindSystem, Label: "X2"}) // should be ignored
	dw.End()
	result := dw.Result()
	ds := result.DotSource
	if strings.Count(ds, `"x"[`) != 1 {
		t.Fatalf("expected one node definition for x, got:\n%s", ds)
	}
	if len(result.Metadata.Nodes) != 1 {
		t.Fatalf("expected one node definition for x in metadata, got %d", len(result.Metadata.Nodes))
	}
}

func TestWriter_Golden_Simple(t *testing.T) {
	dw := New()
	dw.Start()

	// cluster sys1
	dw.StartCluster("sys1")
	dw.AddNode(Node{ID: "c1", Kind: KindComponent, Label: "c1"})
	dw.AddNode(Node{ID: "api1", Kind: KindAPI, Label: "api1"})
	dw.EndCluster()

	// outside node
	dw.AddNode(Node{ID: "s1", Kind: KindSystem, Label: "s1"})

	// edges (root-level)
	dw.AddEdge(Edge{From: "s1", To: "api1", Style: ESSystemLink})
	dw.AddEdge(Edge{From: "c1", To: "api1", Style: ESDependsOn})

	dw.End()

	got := dw.Result()

	wantDOT := strings.TrimSpace(`
digraph {
rankdir="LR"
fontname="sans-serif"
splines="spline"
class="graphviz-svg"
node[shape="box",fontname="sans-serif",fontsize="11",style="filled,rounded"]
edge[fontname="sans-serif",fontsize="11",minlen="4"]
subgraph "cluster_sys1" {
label="sys1"
style=filled
fillcolor="#f3f4f6"
"c1"[id="c1",label="c1",fillcolor="#CBDCEB",shape="box",class="clickable-node"]
"api1"[id="api1",label="api1",fillcolor="#FADA7A",shape="box",class="clickable-node"]
}
"s1"[id="s1",label="s1",fillcolor="#6D94C5",shape="box",class="clickable-node"]
"s1" -> "api1"[class="clickable-edge system-link-edge",id="svg-edge-0"]
"c1" -> "api1"[id="svg-edge-1",style="dashed"]
}
`)

	gotDOT := strings.TrimSpace(got.DotSource)
	if diff := cmp.Diff(wantDOT, gotDOT); diff != "" {
		t.Fatalf("DOT mismatch (-want +got):\n%s", diff)
	}

	// Sidecar metadata checks
	if got.Metadata == nil {
		t.Fatalf("metadata nil")
	}
	if got.Metadata.Nodes["c1"] == nil || got.Metadata.Nodes["api1"] == nil || got.Metadata.Nodes["s1"] == nil {
		t.Fatalf("missing node metadata: %+v", got.Metadata.Nodes)
	}

	e0 := got.Metadata.Edges["svg-edge-0"]
	if e0 == nil || e0.From != "s1" || e0.To != "api1" {
		t.Fatalf("edge svg-edge-0 metadata mismatch: %+v", e0)
	}
	e1 := got.Metadata.Edges["svg-edge-1"]
	if e1 == nil || e1.From != "c1" || e1.To != "api1" || e1.Label != "" {
		t.Fatalf("edge svg-edge-1 metadata mismatch: %+v", e1)
	}
}
