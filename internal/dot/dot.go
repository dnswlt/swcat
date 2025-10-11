package dot

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sort"
	"strings"
)

type NodeInfo struct {
	Label string `json:"label"`
}
type EdgeInfo struct {
	From  string `json:"from"`  // From entity reference (e.g. api:ns1/super-api).
	To    string `json:"to"`    // To entity reference.
	Label string `json:"label"` // Optional edge label.
}
type ClusterInfo struct {
	Label string `json:"label"`
}

// SVGGraphMetadata provides "sidecar" metadata for nodes and edges that are hard or impossible
// to encode in the SVG itself. It is marshalled as JSON and embedded in HTML pages along with
// the SVG.
type SVGGraphMetadata struct {
	// Maps the IDs of nodes in the generated SVG (id= attributes) to their node info.
	Nodes map[string]*NodeInfo `json:"nodes"`
	// Maps the IDs of edges in the generated SVG (id= attributes) to their edge info.
	Edges map[string]*EdgeInfo `json:"edges"`
	// Maps the IDs of clusters in the generated SVG (id= attributes) to their cluster info.
	Clusters map[string]*ClusterInfo `json:"clusters"`
}

type DotSource struct {
	DotSource string
	Metadata  *SVGGraphMetadata
}

// Runner is an interface that wraps executions of the Graphviz dot tool.
// It is mostly used as an abstraction layer for testing.
type Runner interface {
	Run(ctx context.Context, dotSource string) ([]byte, error)
}

func NewRunner(dotPath string) Runner {
	return &dotRunner{
		dotPath: dotPath,
	}
}

// dotRunner is the implementation of the Runner interface that
// executes the dot program.
type dotRunner struct {
	dotPath string
}

func (r *dotRunner) Run(ctx context.Context, dotSource string) ([]byte, error) {
	// Command: dot -Tsvg
	log.Printf("Running '%s -Tsvg' to generate the SVG", r.dotPath)
	cmd := exec.CommandContext(ctx, r.dotPath, "-Tsvg")

	// Provide the DOT source on stdin and capture stdout/stderr
	// Use CombinedOutput to get useful error messages in case dot fails.
	cmd.Stdin = nil // we'll set via a pipe below
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("creating stdin pipe: %w", err)
	}

	go func() {
		defer stdin.Close()
		io.WriteString(stdin, dotSource)
	}()

	output, err := cmd.CombinedOutput() // will wait until process exits
	if err != nil {
		// CombinedOutput returns output (stdout+stderr) even on error - include it for debugging.
		return output, fmt.Errorf("dot failed: %w; output: %s; input: %s", err, output, dotSource)
	}

	// Cut off <?xml ?> header and only return the <svg>
	if idx := bytes.Index(output, []byte("<svg")); idx > 0 {
		output = output[idx:]
	}

	return output, nil
}

type NodeKind int

const (
	KindUnspecified NodeKind = iota
	KindDomain
	KindSystem
	KindComponent
	KindResource
	KindAPI
	KindGroup
)

type Node struct {
	ID    string // ID of this node in the dot graph.
	Kind  NodeKind
	Label string
}

func (n *Node) FillColor() string {
	switch n.Kind {
	case KindComponent:
		return "#CBDCEB"
	case KindSystem:
		return "#6D94C5"
	case KindAPI:
		return "#FADA7A"
	case KindResource:
		return "#B4DEBD"
	case KindGroup:
		return "#F5EEDC"
	}
	return "#F5EEDC" // neutral beige
}

func (n *Node) Shape() string {
	switch n.Kind {
	case KindGroup:
		return "ellipse"
	default:
		return "box"
	}
}

type EdgeStyle int

const (
	// A normal arrow pointing from source to target
	ESNormal EdgeStyle = iota
	// An arrow poinging from target to source.
	ESBackward
	// An arrow pointing from target (the API provder) to source (the API),
	// with an empty arrowhead (like a UML implementation arrow).
	ESProvidedBy
	// A dashed line pointing from source to target.
	ESDependsOn
	// Used for arrows from owner groups to their owned entities.
	ESOwner
	// Used for arrows from a System to its constituent entities (components, apis, resources).
	ESContains
	// Used for edges on the System overview page to link from the system of interest to its neighbors.
	ESSystemLink
)

type Edge struct {
	From  string
	To    string
	Label string
	Style EdgeStyle
}

type Writer struct {
	w           *strings.Builder
	nodeInfo    map[string]*NodeInfo
	edgeInfo    map[string]*EdgeInfo
	clusterInfo map[string]*ClusterInfo
}

func New() *Writer {
	return &Writer{
		w:           &strings.Builder{},
		nodeInfo:    make(map[string]*NodeInfo),
		edgeInfo:    make(map[string]*EdgeInfo),
		clusterInfo: make(map[string]*ClusterInfo),
	}
}

func (dw *Writer) Start() {
	dw.w.WriteString("digraph {\n")
	dw.w.WriteString("rankdir=\"LR\"\n")
	dw.w.WriteString("fontname=\"sans-serif\"\n")
	dw.w.WriteString("splines=\"spline\"\n")
	// Tell Graphviz about font sizes and (approximate) font families so it can
	// size boxes and edge labels appropriately. The ultimate font style is defined
	// via CSS (see style.css).
	dw.w.WriteString("class=\"graphviz-svg\"\n")
	dw.w.WriteString("node[shape=\"box\",fontname=\"sans-serif\",fontsize=\"11\",style=\"filled,rounded\"]\n")
	dw.w.WriteString("edge[fontname=\"sans-serif\",fontsize=\"11\",minlen=\"4\"]\n")
}

func (dw *Writer) End() {
	dw.w.WriteString("}\n")
}

func (dw *Writer) AddNode(node Node) {
	if _, ok := dw.nodeInfo[node.ID]; ok {
		// Ignore duplicate node definitions.
		return
	}
	fmt.Fprintf(dw.w, `"%s"[id="%s",label="%s",fillcolor="%s",shape="%s",class="clickable-node"]`,
		node.ID, node.ID, node.Label, node.FillColor(), node.Shape())
	fmt.Fprintln(dw.w)
	dw.nodeInfo[node.ID] = &NodeInfo{
		Label: node.Label,
	}
}

func (dw *Writer) AddEdge(edge Edge) {
	attrs := map[string]string{}

	// Use a synthetic ID. The frontend will get its information about this
	// edge from its associated metadata JSON.
	edgeID := fmt.Sprintf("svg-edge-%d", len(dw.edgeInfo))
	attrs["id"] = edgeID
	dw.edgeInfo[edgeID] = &EdgeInfo{
		From:  edge.From,
		To:    edge.To,
		Label: edge.Label,
	}

	if edge.Label != "" {
		attrs["label"] = edge.Label
	}
	switch edge.Style {
	case ESBackward:
		attrs["dir"] = "back"
	case ESProvidedBy:
		// Draw "provides" relationships backwards, so the API appears on the left-hand side
		// of the implementing component (with an empty arrowhead pointing at the API).
		attrs["dir"] = "back"
		attrs["arrowtail"] = "empty"
	case ESDependsOn:
		attrs["style"] = "dashed"
	case ESOwner:
		attrs["dir"] = "back"
		attrs["label"] = "owner"
	case ESContains:
		attrs["dir"] = "back"
		attrs["label"] = "part-of"
	case ESSystemLink:
		attrs["class"] = "clickable-edge system-link-edge"
	default:
		// No special attrs required.
	}

	var edgeAttrs string
	if len(attrs) > 0 {
		// Sort keys for deterministic output
		keys := make([]string, 0, len(attrs))
		for k := range attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		items := make([]string, 0, len(keys))
		for _, k := range keys {
			items = append(items, fmt.Sprintf("%s=\"%s\"", k, attrs[k]))
		}
		edgeAttrs = "[" + strings.Join(items, ",") + "]"
	}

	fmt.Fprintf(dw.w, `"%s" -> "%s"%s`, edge.From, edge.To, edgeAttrs)
	fmt.Fprintln(dw.w)
}

func (dw *Writer) StartCluster(label string) {
	clusterID := fmt.Sprintf("svg-cluster-%d", len(dw.clusterInfo))
	dw.clusterInfo[clusterID] = &ClusterInfo{
		Label: label,
	}
	fmt.Fprintf(dw.w, "subgraph \"cluster_%s\" {\n", clusterID)
	fmt.Fprintf(dw.w, "id=\"%s\"\n", clusterID)
	fmt.Fprintf(dw.w, "label=\"%s\"\n", label)
	fmt.Fprintf(dw.w, "style=filled\n")
	fmt.Fprintf(dw.w, "fillcolor=\"#f3f4f6\"\n")
}

func (dw *Writer) EndCluster() {
	dw.w.WriteString("}\n")
}

func (dw *Writer) Result() *DotSource {
	return &DotSource{
		DotSource: dw.w.String(),
		Metadata: &SVGGraphMetadata{
			Nodes: dw.nodeInfo,
			Edges: dw.edgeInfo,
		},
	}
}
