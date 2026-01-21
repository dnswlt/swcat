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
	"time"
	"unicode"
)

type NodeInfo struct {
	Label string `json:"label"`
}
type EdgeInfo struct {
	From         string        `json:"from"`                   // From entity reference (e.g. api:ns1/super-api).
	To           string        `json:"to"`                     // To entity reference.
	Label        string        `json:"label"`                  // Optional edge label.
	TooltipAttrs []TooltipAttr `json:"tooltipAttrs,omitempty"` // Optional key/value pairs to be displayed in tooltips.
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
	Close() error
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
	started := time.Now()
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
		return output, fmt.Errorf("Running %s failed: %w; output: %s; input: %s", r.dotPath, err, output, dotSource)
	}

	elapsed := time.Since(started).Milliseconds()
	log.Printf("Generated SVG using dot in %d ms", elapsed)

	// Cut off <?xml ?> header and only return the <svg>
	if idx := bytes.Index(output, []byte("<svg")); idx > 0 {
		output = output[idx:]
	}

	return output, nil
}

func (r *dotRunner) Close() error {
	// Nothing to do for a dotRunner
	return nil
}

type NodeLayout struct {
	Label     string // Multi-line labels should use Graphviz \n \l \r sequences.
	FillColor string // Either hex ("#ff00aa") or a well-known color name ("red").
	Shape     NodeShape
}

type NodeShape int

const (
	NSRoundedBox NodeShape = iota
	NSBox
	NSEllipse
)

type Node struct {
	ID     string // ID of this node in the dot graph.
	Layout NodeLayout
}

func (n *Node) Label() string {
	return n.Layout.Label
}

func (n *Node) FillColor() string {
	if n.Layout.FillColor != "" {
		return n.Layout.FillColor
	}
	return "#FFFFFF"
}

func (n *Node) dotShape() string {
	switch n.Layout.Shape {
	case NSEllipse:
		return "ellipse"
	}
	return "box"
}

func (n *Node) dotStyle() string {
	switch n.Layout.Shape {
	case NSRoundedBox:
		return "filled,rounded"
	}
	return "filled"
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
	// Used for arrows from a Component to its subcomponents.
	ESSubcomponent
	// Used for edges on the System overview page to link from the system of interest to its neighbors.
	ESSystemLink
)

type TooltipAttr struct {
	Key   string
	Value string
}

type EdgeLayout struct {
	Label        string
	Style        EdgeStyle
	TooltipAttrs []TooltipAttr
}

type Edge struct {
	From   string
	To     string
	Layout EdgeLayout
}

type WriterConfig struct {
	EdgeMinLen int
}

type Writer struct {
	w           *strings.Builder
	nodeInfo    map[string]*NodeInfo
	edgeInfo    map[string]*EdgeInfo
	clusterInfo map[string]*ClusterInfo
	config      WriterConfig
}

func DefaultConfig() WriterConfig {
	return WriterConfig{
		EdgeMinLen: 3,
	}
}

func New() *Writer {
	return NewWithConfig(DefaultConfig())
}

func NewWithConfig(config WriterConfig) *Writer {
	return &Writer{
		w:           &strings.Builder{},
		nodeInfo:    make(map[string]*NodeInfo),
		edgeInfo:    make(map[string]*EdgeInfo),
		clusterInfo: make(map[string]*ClusterInfo),
		config:      config,
	}
}

func (dw *Writer) Start() {
	dw.w.WriteString("digraph {\n")
	dw.w.WriteString("charset=\"UTF-8\"\n")
	dw.w.WriteString("rankdir=\"LR\"\n")
	dw.w.WriteString("fontname=\"sans-serif\"\n")
	dw.w.WriteString("splines=\"spline\"\n")
	// Tell Graphviz about font sizes and (approximate) font families so it can
	// size boxes and edge labels appropriately. The ultimate font style is defined
	// via CSS (see style.css).
	dw.w.WriteString("class=\"graphviz-svg\"\n")
	dw.w.WriteString("node[shape=\"box\",fontname=\"sans-serif\",fontsize=\"11\",style=\"filled\"]\n")
	// We would like to add tooltip="" here to prevent tooltips on edges (given that we render rich tooltips).
	// https://forum.graphviz.org/t/svg-without-tooltips/425/3 indicates that this does not work
	// (and indeed it didn't). So we remove the <title> element in postprocessing instead.
	fmt.Fprintf(dw.w, "edge[fontname=\"sans-serif\",fontsize=\"11\",minlen=\"%d\"]\n", dw.config.EdgeMinLen)
}

func (dw *Writer) End() {
	dw.w.WriteString("}\n")
}

func (dw *Writer) AddNode(node Node) {
	if _, ok := dw.nodeInfo[node.ID]; ok {
		// Ignore duplicate node definitions.
		return
	}
	label := escLabel(node.Label())
	fillColor := escLabel(node.FillColor())

	fmt.Fprintf(dw.w, `"%s"[id="%s",label="%s",fillcolor="%s",shape="%s",style="%s",class="clickable-node"]`,
		node.ID, node.ID, label, fillColor, node.dotShape(), node.dotStyle())
	fmt.Fprintln(dw.w)
	dw.nodeInfo[node.ID] = &NodeInfo{
		Label: node.Label(),
	}
}

// escLabel escapes the given label string for safe use as a graphviz/dot label.
// Escape sequences that dot understands are passed through unchanged, to allow
// users to e.g. specify multi-line labels using dot's "\n" "\r" or "\l" character sequences.
// Only Latin1 characters are supported, other characters are replaced by "?".
func escLabel(label string) string {
	rs := []rune(label)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			if i+1 < len(rs) && (rs[i+1] == 'n' || rs[i+1] == 'l' || rs[i+1] == 'r') {
				b.WriteRune('\\')
				b.WriteRune(rs[i+1])
				i++
			} else {
				b.WriteString(`\\`)
			}
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			// ignore CRs
		default:
			r := rs[i]
			// allow only printable Latin-1; normalize spaces
			if unicode.IsSpace(r) {
				b.WriteRune(' ')
			} else if unicode.IsPrint(r) {
				b.WriteRune(r)
			} else {
				// Replace non-printable ASCII by ?
				b.WriteRune('?')
			}
		}
	}
	return b.String()
}

func (dw *Writer) AddEdge(edge Edge) {
	attrs := map[string]string{}

	// Use a synthetic ID. The frontend will get its information about this
	// edge from its associated metadata JSON.
	edgeID := fmt.Sprintf("svg-edge-%d", len(dw.edgeInfo))
	attrs["id"] = edgeID
	dw.edgeInfo[edgeID] = &EdgeInfo{
		From:         edge.From,
		To:           edge.To,
		Label:        edge.Layout.Label,
		TooltipAttrs: edge.Layout.TooltipAttrs,
	}

	if edge.Layout.Label != "" {
		attrs["label"] = escLabel(edge.Layout.Label)
	}
	switch edge.Layout.Style {
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
	case ESSubcomponent:
		attrs["dir"] = "back"
		attrs["label"] = "sub-of"
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
	fmt.Fprintf(dw.w, "fontsize=\"11\"\n")
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
