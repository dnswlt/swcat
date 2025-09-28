package backstage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

func runDot(ctx context.Context, dotSource string) ([]byte, error) {
	// Command: dot -Tsvg
	cmd := exec.CommandContext(ctx, "dot", "-Tsvg")

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
		return output, fmt.Errorf("dot failed: %w; output: %s", err, output)
	}

	// Cut off <?xml ?> header and only return the <svg>
	if idx := bytes.Index(output, []byte("<svg")); idx > 0 {
		output = output[idx:]
	}

	return output, nil
}

type DotNode struct {
	QName string
	Kind  string
	Label string
	Shape string
}

func (n *DotNode) FillColor() string {
	switch n.Kind {
	case "component":
		return "lightblue"
	case "system":
		return "lightsteelblue"
	case "api":
		return "plum"
	case "resource":
		return "azure"
	case "group":
		return "sandybrown"
	}
	return "lightgray"
}

type DotEdge struct {
	From  string
	To    string
	Style string
}

type DotWriter struct {
	w     *strings.Builder
	nodes map[string]bool
}

func NewDotWriter() *DotWriter {
	return &DotWriter{
		w:     &strings.Builder{},
		nodes: make(map[string]bool),
	}
}

func (dw *DotWriter) start() {
	dw.w.WriteString("digraph {\n")
	dw.w.WriteString("rankdir=\"LR\"\n")
	dw.w.WriteString("fontname=\"sans-serif\"\n")
	dw.w.WriteString("splines=\"spline\"\n")
	dw.w.WriteString("fontsize=\"11\"\n")
	dw.w.WriteString("node[shape=\"box\",fontname=\"sans-serif\",fontsize=\"11\",style=\"filled,rounded\"]\n")
	dw.w.WriteString("edge[fontname=\"sans-serif\",fontsize=\"11\",minlen=\"4\"]\n")
}

func (dw *DotWriter) end() {
	dw.w.WriteString("}\n")
}

func (dw *DotWriter) String() string {
	return dw.w.String()
}

func (dw *DotWriter) addNode(node DotNode) {
	if dw.nodes[node.QName] {
		return
	}
	if node.Shape == "" {
		node.Shape = "box"
	}
	fmt.Fprintf(dw.w, `"%s"[id="%s:%s",label="%s",fillcolor="%s",shape="%s",class="clickable-node"]`,
		node.QName, node.Kind, node.QName, node.Label, node.FillColor(), node.Shape)
	fmt.Fprintln(dw.w)
	dw.nodes[node.QName] = true
}

func (dw *DotWriter) addEdge(edge DotEdge) {
	switch edge.Style {
	case "provides":
		fmt.Fprintf(dw.w, `"%s" -> "%s"[dir="back",arrowtail="empty"]`, edge.From, edge.To)
	default:
		fmt.Fprintf(dw.w, `"%s" -> "%s"`, edge.From, edge.To)
	}
	fmt.Fprintln(dw.w)
}

func (dw *DotWriter) startCluster(name string) {
	fmt.Fprintf(dw.w, "subgraph \"cluster_%s\" {\n", name)
	fmt.Fprintf(dw.w, "label=\"%s\"\n", name)
}

func (dw *DotWriter) endCluster() {
	dw.w.WriteString("}\n")
}

// GenerateSystemSVG generates an SVG for the given system.
func GenerateSystemSVG(r *Repository, name string) ([]byte, error) {
	system := r.System(name)
	if system == nil {
		return nil, fmt.Errorf("system %s does not exist", name)
	}

	dw := NewDotWriter()
	dw.start()

	dw.startCluster(name)

	for _, c := range system.Components() {
		comp := r.Component(c)
		dw.addNode(DotNode{QName: comp.GetQName(), Kind: "component", Label: comp.GetQName()})
	}
	for _, a := range system.APIs() {
		api := r.API(a)
		dw.addNode(DotNode{QName: api.GetQName(), Kind: "api", Label: api.GetQName()})
	}
	for _, res := range system.Resources() {
		resource := r.Resource(res)
		dw.addNode(DotNode{QName: resource.GetQName(), Kind: "resource", Label: resource.GetQName()})
	}

	dw.endCluster()

	dw.end()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runDot(ctx, dw.String())
}

// GenerateComponentSVG generates an SVG for the given component.
func GenerateComponentSVG(r *Repository, name string) ([]byte, error) {
	component := r.Component(name)
	if component == nil {
		return nil, fmt.Errorf("component %s does not exist", name)
	}
	qn := component.GetQName()

	dw := NewDotWriter()
	dw.start()

	// Component
	dw.addNode(DotNode{QName: qn, Kind: "component", Label: qn})

	// "Incoming" dependencies
	// - Owner
	// - System
	// - Provided APIs
	// - Other entities with a DependsOn relationship to this entity
	owner := r.Group(component.Spec.Owner)
	if owner != nil {
		ownerQn := owner.GetQName()
		dw.addNode(DotNode{QName: ownerQn, Kind: "group", Label: ownerQn, Shape: "ellipse"})
		dw.addEdge(DotEdge{From: ownerQn, To: qn})
	}
	system := r.System(component.Spec.System)
	if system != nil {
		systemQn := system.GetQName()
		dw.addNode(DotNode{QName: systemQn, Kind: "system", Label: systemQn})
		dw.addEdge(DotEdge{From: systemQn, To: qn})
	}
	for _, a := range component.Spec.ProvidesAPIs {
		api := r.API(a)
		apiQn := api.GetQName()
		dw.addNode(DotNode{QName: apiQn, Kind: "api", Label: apiQn})
		dw.addEdge(DotEdge{From: apiQn, To: qn, Style: "provides"})
	}
	for _, d := range component.Spec.dependents {
		e := r.Entity(d)
		switch x := e.(type) {
		case *Component:
			xQn := x.GetQName()
			dw.addNode(DotNode{QName: xQn, Kind: "component", Label: xQn})
			dw.addEdge(DotEdge{From: xQn, To: qn})
		case *Resource:
			xQn := x.GetQName()
			dw.addNode(DotNode{QName: xQn, Kind: "resource", Label: xQn})
			dw.addEdge(DotEdge{From: xQn, To: qn})
		}
	}

	// "Outgoing" dependencies
	// - Consumed APIs
	// - DependsOn relationships of this entity
	for _, a := range component.Spec.ConsumesAPIs {
		api := r.API(a)
		apiQn := api.GetQName()
		dw.addNode(DotNode{QName: apiQn, Kind: "api", Label: apiQn})
		dw.addEdge(DotEdge{From: qn, To: apiQn})
	}
	for _, d := range component.Spec.DependsOn {
		e := r.Entity(d)
		switch x := e.(type) {
		case *Component:
			xQn := x.GetQName()
			dw.addNode(DotNode{QName: xQn, Kind: "component", Label: xQn})
			dw.addEdge(DotEdge{From: qn, To: xQn})
		case *Resource:
			xQn := x.GetQName()
			dw.addNode(DotNode{QName: xQn, Kind: "resource", Label: xQn})
			dw.addEdge(DotEdge{From: qn, To: xQn})
		}
	}

	dw.end()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runDot(ctx, dw.String())

}
