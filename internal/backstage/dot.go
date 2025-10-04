package backstage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"
)

func runDot(ctx context.Context, dotSource string) ([]byte, error) {
	// Command: dot -Tsvg
	log.Printf("Running 'dot -Tsvg' to generate SVG")
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
		return output, fmt.Errorf("dot failed: %w; output: %s; input: %s", err, output, dotSource)
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
}

func (n *DotNode) FillColor() string {
	switch n.Kind {
	case "component":
		return "#CBDCEB"
	case "system":
		return "#6D94C5"
	case "api":
		return "#FADA7A"
	case "resource":
		return "#B4DEBD"
	case "group":
		return "#F5EEDC"
	}
	return "#F5EEDC" // neutral beige
}

func (n *DotNode) Shape() string {
	switch n.Kind {
	case "group":
		return "ellipse"
	default:
		return "box"
	}
}

type DotEdgeStyle int

const (
	// A normal arrow pointing from source to target
	ESNormal = iota
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
)

type DotEdge struct {
	From string
	To   string
	// Only one of Label or HTMLLabel may be used. If the latter is set, it takes precedence.
	Label string
	Style DotEdgeStyle
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
	// Tell Graphviz about font sizes and (approximate) font families so it can
	// size boxes and edge labels appropriately. The ultimate font style is defined
	// via CSS (see style.css).
	dw.w.WriteString("class=\"graphviz-svg\"\n")
	dw.w.WriteString("node[shape=\"box\",fontname=\"sans-serif\",fontsize=\"11\",style=\"filled,rounded\"]\n")
	dw.w.WriteString("edge[fontname=\"sans-serif\",fontsize=\"11\",minlen=\"4\"]\n")
}

func (dw *DotWriter) end() {
	dw.w.WriteString("}\n")
}

func (dw *DotWriter) addNode(node DotNode) {
	if dw.nodes[node.QName] {
		// Ignore duplicate node definitions.
		return
	}
	fmt.Fprintf(dw.w, `"%s"[id="%s:%s",label="%s",fillcolor="%s",shape="%s",class="clickable-node"]`,
		node.QName, node.Kind, node.QName, node.Label, node.FillColor(), node.Shape())
	fmt.Fprintln(dw.w)
	dw.nodes[node.QName] = true
}

func (dw *DotWriter) addEdge(edge DotEdge) {
	attrs := map[string]string{}

	setLabel := func(s string) {
		attrs["label"] = `"` + s + `"`
	}

	if edge.Label != "" {
		setLabel(edge.Label)
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
		if attrs["label"] == "" {
			setLabel("owner")
		}
	case ESContains:
		attrs["dir"] = "back"
		if attrs["label"] == "" {
			setLabel("part-of")
		}
	default:
		// No special attrs required.
	}

	var edgeAttrs string
	if len(attrs) > 0 {
		var items []string
		for k, v := range attrs {
			items = append(items, fmt.Sprintf("%s=%s", k, v))
		}
		edgeAttrs = "[" + strings.Join(items, ",") + "]"
	}
	fmt.Fprintf(dw.w, `"%s" -> "%s"%s`, edge.From, edge.To, edgeAttrs)
	fmt.Fprintln(dw.w)
}

func (dw *DotWriter) startCluster(name string) {
	fmt.Fprintf(dw.w, "subgraph \"cluster_%s\" {\n", name)
	fmt.Fprintf(dw.w, "label=\"%s\"\n", name)
}

func (dw *DotWriter) endCluster() {
	dw.w.WriteString("}\n")
}

func (dw *DotWriter) String() string {
	return dw.w.String()
}

// GenerateDomainSVG generates an SVG for the given domain.
func GenerateDomainSVG(r *Repository, name string) ([]byte, error) {
	domain := r.Domain(name)
	if domain == nil {
		return nil, fmt.Errorf("domain %s does not exist", name)
	}

	dw := NewDotWriter()
	dw.start()

	dw.startCluster(name)

	for _, s := range domain.GetSystems() {
		system := r.System(s)
		dw.addNode(DotNode{QName: system.GetQName(), Kind: "system", Label: system.GetQName()})
	}

	dw.endCluster()

	dw.end()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runDot(ctx, dw.String())
}

type extSysDep struct {
	source       Entity
	targetSystem string
	direction    string // "incoming" or "outgoing"
}

func (e extSysDep) String() string {
	return fmt.Sprintf("%s -> %s / %s", e.source.GetQName(), e.targetSystem, e.direction)
}

// GenerateSystemSVG generates an SVG for the given system.
func GenerateSystemSVG(r *Repository, name string) ([]byte, error) {
	system := r.System(name)
	if system == nil {
		return nil, fmt.Errorf("system %s does not exist", name)
	}

	dw := NewDotWriter()
	dw.start()

	var externalDeps []extSysDep

	dw.startCluster(name)

	// Components
	for _, c := range system.GetComponents() {
		comp := r.Component(c)
		dw.addNode(DotNode{QName: comp.GetQName(), Kind: "component", Label: comp.GetQName()})

		// Add links to external systems of which the component consumes APIs.
		for _, a := range comp.Spec.ConsumesAPIs {
			api := r.API(a)
			if api.GetSystem() != comp.GetSystem() {
				externalDeps = append(externalDeps, extSysDep{
					source: comp, targetSystem: api.GetSystem(), direction: "outgoing",
				})
			}
		}
		// Add links for direct dependencies of the component.
		for _, d := range comp.Spec.DependsOn {
			entity := r.Entity(d)
			if se, ok := entity.(SystemPart); ok && se.GetSystem() != comp.GetSystem() {
				externalDeps = append(externalDeps, extSysDep{
					source: comp, targetSystem: se.GetSystem(), direction: "outgoing",
				})
			}
		}
		// Add links for direct dependents of the component.
		for _, d := range comp.GetDependents() {
			entity := r.Entity(d)
			if se, ok := entity.(SystemPart); ok && se.GetSystem() != comp.GetSystem() {
				externalDeps = append(externalDeps, extSysDep{
					source: comp, targetSystem: se.GetSystem(), direction: "incoming",
				})
			}
		}
	}

	// APIs
	for _, a := range system.GetAPIs() {
		api := r.API(a)
		dw.addNode(DotNode{QName: api.GetQName(), Kind: "api", Label: api.GetQName()})

		// Add links for consumers of any API of this system.
		for _, c := range api.GetConsumers() {
			consumer := r.Component(c)
			if consumer.GetSystem() != api.GetSystem() {
				externalDeps = append(externalDeps, extSysDep{
					source: api, targetSystem: consumer.GetSystem(), direction: "incoming",
				})

			}
		}
	}

	// Resources
	for _, res := range system.GetResources() {
		resource := r.Resource(res)
		dw.addNode(DotNode{QName: resource.GetQName(), Kind: "resource", Label: resource.GetQName()})

		// Add links to external systems that the resource depends on.
		for _, d := range resource.Spec.DependsOn {
			entity := r.Entity(d)
			if se, ok := entity.(SystemPart); ok && se.GetSystem() != resource.GetSystem() {
				externalDeps = append(externalDeps, extSysDep{
					source: resource, targetSystem: se.GetSystem(), direction: "outgoing",
				})
			}
		}
		// Add links for direct dependents of the resource.
		for _, d := range resource.GetDependents() {
			entity := r.Entity(d)
			if se, ok := entity.(SystemPart); ok && se.GetSystem() != resource.GetSystem() {
				externalDeps = append(externalDeps, extSysDep{
					source: resource, targetSystem: se.GetSystem(), direction: "incoming",
				})
			}
		}
	}

	dw.endCluster()

	// Draw edges for all collected external dependencies, removing duplicates
	seenDeps := map[string]bool{}
	for _, extDep := range externalDeps {
		if seenDeps[extDep.String()] {
			continue
		}
		seenDeps[extDep.String()] = true
		dw.addNode(DotNode{QName: extDep.targetSystem, Kind: "system", Label: extDep.targetSystem})
		if extDep.direction == "outgoing" {
			dw.addEdge(DotEdge{From: extDep.source.GetQName(), To: extDep.targetSystem})
		} else {
			dw.addEdge(DotEdge{From: extDep.targetSystem, To: extDep.source.GetQName()})
		}
	}

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
		dw.addNode(DotNode{QName: ownerQn, Kind: "group", Label: ownerQn})
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
		dw.addEdge(DotEdge{From: apiQn, To: qn, Style: ESProvidedBy})
	}
	for _, d := range component.Spec.dependents {
		e := r.Entity(d)
		if e != nil {
			xQn := e.GetQName()
			dw.addNode(DotNode{QName: xQn, Kind: strings.ToLower(e.GetKind()), Label: xQn})
			dw.addEdge(DotEdge{From: xQn, To: qn, Style: ESDependsOn})
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
		if e != nil {
			xQn := e.GetQName()
			dw.addNode(DotNode{QName: xQn, Kind: strings.ToLower(e.GetKind()), Label: xQn})
			dw.addEdge(DotEdge{From: qn, To: xQn, Style: ESDependsOn})
		}
	}

	dw.end()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runDot(ctx, dw.String())

}

// GenerateAPISVG generates an SVG for the given API.
func GenerateAPISVG(r *Repository, name string) ([]byte, error) {
	api := r.API(name)
	if api == nil {
		return nil, fmt.Errorf("API %s does not exist", name)
	}
	qn := api.GetQName()

	dw := NewDotWriter()
	dw.start()

	// API
	dw.addNode(DotNode{QName: qn, Kind: "api", Label: qn})

	// Owner
	owner := r.Group(api.Spec.Owner)
	if owner != nil {
		ownerQn := owner.GetQName()
		dw.addNode(DotNode{QName: ownerQn, Kind: "group", Label: ownerQn})
		dw.addEdge(DotEdge{From: ownerQn, To: qn, Style: ESOwner})
	}
	// System containing the API
	system := r.System(api.Spec.System)
	if system != nil {
		systemQn := system.GetQName()
		dw.addNode(DotNode{QName: systemQn, Kind: "system", Label: systemQn})
		dw.addEdge(DotEdge{From: systemQn, To: qn, Style: ESContains})
	}

	// Providers
	for _, p := range api.GetProviders() {
		provider := r.Component(p)
		if provider != nil {
			providerQn := provider.GetQName()
			dw.addNode(DotNode{QName: providerQn, Kind: "component", Label: providerQn})
			dw.addEdge(DotEdge{From: qn, To: providerQn, Style: ESProvidedBy})
		}
	}

	// Consumers
	for _, c := range api.GetConsumers() {
		consumer := r.Component(c)
		if consumer != nil {
			consumerQn := consumer.GetQName()
			dw.addNode(DotNode{QName: consumerQn, Kind: "component", Label: consumerQn})
			dw.addEdge(DotEdge{From: consumerQn, To: qn})
		}
	}

	dw.end()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runDot(ctx, dw.String())
}

// GenerateResourceSVG generates an SVG for the given resource.
func GenerateResourceSVG(r *Repository, name string) ([]byte, error) {
	resource := r.Resource(name)
	if resource == nil {
		return nil, fmt.Errorf("resource %s does not exist", name)
	}
	qn := resource.GetQName()

	dw := NewDotWriter()
	dw.start()

	// Resource
	dw.addNode(DotNode{QName: qn, Kind: "resource", Label: qn})

	// Owner
	owner := r.Group(resource.Spec.Owner)
	if owner != nil {
		ownerQn := owner.GetQName()
		dw.addNode(DotNode{QName: ownerQn, Kind: "group", Label: ownerQn})
		dw.addEdge(DotEdge{From: ownerQn, To: qn, Style: ESOwner})
	}
	// System containing the API
	system := r.System(resource.Spec.System)
	if system != nil {
		systemQn := system.GetQName()
		dw.addNode(DotNode{QName: systemQn, Kind: "system", Label: systemQn})
		dw.addEdge(DotEdge{From: systemQn, To: qn, Style: ESContains})
	}

	// Dependents
	for _, d := range resource.GetDependents() {
		dependent := r.Entity(d)
		if dependent != nil {
			dependentQn := dependent.GetQName()
			dw.addNode(DotNode{QName: dependentQn, Kind: strings.ToLower(dependent.GetKind()), Label: dependentQn})
			dw.addEdge(DotEdge{From: dependentQn, To: qn, Style: ESDependsOn})
		}
	}

	dw.end()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runDot(ctx, dw.String())
}
