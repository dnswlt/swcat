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

type NodeKind int

const (
	KindUnspecified = iota
	KindDomain
	KindSystem
	KindComponent
	KindResource
	KindAPI
	KindGroup
)

func entityNodeKind(e Entity) NodeKind {
	switch e.(type) {
	case *Domain:
		return KindDomain
	case *System:
		return KindSystem
	case *Component:
		return KindComponent
	case *Resource:
		return KindResource
	case *API:
		return KindAPI
	case *Group:
		return KindGroup
	default:
		return KindUnspecified
	}
}

type DotNode struct {
	ID    string // ID of this node in the dot graph.
	SVGID string // ID to use as the SVG id= attribute.
	Kind  NodeKind
	Label string
}

func EntityNode(e Entity) DotNode {
	return DotNode{
		ID:    e.GetQName(),
		SVGID: e.GetRef(),
		Kind:  entityNodeKind(e),
		Label: e.GetQName(),
	}
}

func (n *DotNode) FillColor() string {
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

func (n *DotNode) Shape() string {
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
	Style EdgeStyle
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

func (dw *DotWriter) Start() {
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

func (dw *DotWriter) End() {
	dw.w.WriteString("}\n")
}

func (dw *DotWriter) AddNode(node DotNode) {
	if dw.nodes[node.ID] {
		// Ignore duplicate node definitions.
		return
	}
	fmt.Fprintf(dw.w, `"%s"[id="%s",label="%s",fillcolor="%s",shape="%s",class="clickable-node"]`,
		node.ID, node.SVGID, node.Label, node.FillColor(), node.Shape())
	fmt.Fprintln(dw.w)
	dw.nodes[node.ID] = true
}

func (dw *DotWriter) AddEdge(edge DotEdge) {
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

func (dw *DotWriter) StartCluster(name string) {
	fmt.Fprintf(dw.w, "subgraph \"cluster_%s\" {\n", name)
	fmt.Fprintf(dw.w, "label=\"%s\"\n", name)
}

func (dw *DotWriter) EndCluster() {
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
	dw.Start()

	dw.StartCluster(name)

	for _, s := range domain.GetSystems() {
		system := r.System(s)
		dw.AddNode(EntityNode(system))
	}

	dw.EndCluster()

	dw.End()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runDot(ctx, dw.String())
}

type extSysDep struct {
	source       Entity
	targetSystem *System
	direction    string // "incoming" or "outgoing"
}

func (e extSysDep) String() string {
	return fmt.Sprintf("%s -> %s / %s", e.source.GetQName(), e.targetSystem.GetQName(), e.direction)
}

// GenerateSystemSVG generates an SVG for the given system.
func GenerateSystemSVG(r *Repository, name string) ([]byte, error) {
	system := r.System(name)
	if system == nil {
		return nil, fmt.Errorf("system %s does not exist", name)
	}

	dw := NewDotWriter()
	dw.Start()

	var externalDeps []extSysDep

	dw.StartCluster(name)

	// Components
	for _, c := range system.GetComponents() {
		comp := r.Component(c)
		dw.AddNode(EntityNode(comp))

		// Add links to external systems of which the component consumes APIs.
		for _, a := range comp.Spec.ConsumesAPIs {
			api := r.API(a)
			if api.GetSystem() != comp.GetSystem() {
				apiSystem := r.System(api.GetSystem())
				externalDeps = append(externalDeps, extSysDep{
					source: comp, targetSystem: apiSystem, direction: "outgoing",
				})
			}
		}
		// Add links for direct dependencies of the component.
		for _, d := range comp.Spec.DependsOn {
			entity := r.Entity(d)
			if sp, ok := entity.(SystemPart); ok && sp.GetSystem() != comp.GetSystem() {
				spSystem := r.System(sp.GetSystem())
				externalDeps = append(externalDeps, extSysDep{
					source: comp, targetSystem: spSystem, direction: "outgoing",
				})
			}
		}
		// Add links for direct dependents of the component.
		for _, d := range comp.GetDependents() {
			entity := r.Entity(d)
			if sp, ok := entity.(SystemPart); ok && sp.GetSystem() != comp.GetSystem() {
				spSystem := r.System(sp.GetSystem())
				externalDeps = append(externalDeps, extSysDep{
					source: comp, targetSystem: spSystem, direction: "incoming",
				})
			}
		}
	}

	// APIs
	for _, a := range system.GetAPIs() {
		api := r.API(a)
		dw.AddNode(EntityNode(api))

		// Add links for consumers of any API of this system.
		for _, c := range api.GetConsumers() {
			consumer := r.Component(c)
			if consumer.GetSystem() != api.GetSystem() {
				consumerSystem := r.System(consumer.GetSystem())

				externalDeps = append(externalDeps, extSysDep{
					source: api, targetSystem: consumerSystem, direction: "incoming",
				})

			}
		}
	}

	// Resources
	for _, res := range system.GetResources() {
		resource := r.Resource(res)
		dw.AddNode(EntityNode(resource))

		// Add links to external systems that the resource depends on.
		for _, d := range resource.Spec.DependsOn {
			entity := r.Entity(d)
			if sp, ok := entity.(SystemPart); ok && sp.GetSystem() != resource.GetSystem() {
				spSystem := r.System(sp.GetSystem())
				externalDeps = append(externalDeps, extSysDep{
					source: resource, targetSystem: spSystem, direction: "outgoing",
				})
			}
		}
		// Add links for direct dependents of the resource.
		for _, d := range resource.GetDependents() {
			entity := r.Entity(d)
			if sp, ok := entity.(SystemPart); ok && sp.GetSystem() != resource.GetSystem() {
				spSystem := r.System(sp.GetSystem())
				externalDeps = append(externalDeps, extSysDep{
					source: resource, targetSystem: spSystem, direction: "incoming",
				})
			}
		}
	}

	dw.EndCluster()

	// Draw edges for all collected external dependencies, removing duplicates
	seenDeps := map[string]bool{}
	for _, extDep := range externalDeps {
		if seenDeps[extDep.String()] {
			continue
		}
		seenDeps[extDep.String()] = true
		dw.AddNode(EntityNode(extDep.targetSystem))
		if extDep.direction == "outgoing" {
			dw.AddEdge(DotEdge{From: extDep.source.GetQName(), To: extDep.targetSystem.GetQName()})
		} else {
			dw.AddEdge(DotEdge{From: extDep.targetSystem.GetQName(), To: extDep.source.GetQName()})
		}
	}

	dw.End()

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
	dw.Start()

	// Component
	dw.AddNode(EntityNode(component))

	// "Incoming" dependencies
	// - Owner
	// - System
	// - Provided APIs
	// - Other entities with a DependsOn relationship to this entity
	owner := r.Group(component.Spec.Owner)
	if owner != nil {
		ownerQn := owner.GetQName()
		dw.AddNode(EntityNode(owner))
		dw.AddEdge(DotEdge{From: ownerQn, To: qn})
	}
	system := r.System(component.Spec.System)
	if system != nil {
		systemQn := system.GetQName()
		dw.AddNode(EntityNode(system))
		dw.AddEdge(DotEdge{From: systemQn, To: qn})
	}
	for _, a := range component.Spec.ProvidesAPIs {
		api := r.API(a)
		apiQn := api.GetQName()
		dw.AddNode(EntityNode(api))
		dw.AddEdge(DotEdge{From: apiQn, To: qn, Style: ESProvidedBy})
	}
	for _, d := range component.Spec.dependents {
		e := r.Entity(d)
		if e != nil {
			xQn := e.GetQName()
			dw.AddNode(EntityNode(e))
			dw.AddEdge(DotEdge{From: xQn, To: qn, Style: ESDependsOn})
		}
	}

	// "Outgoing" dependencies
	// - Consumed APIs
	// - DependsOn relationships of this entity
	for _, a := range component.Spec.ConsumesAPIs {
		api := r.API(a)
		apiQn := api.GetQName()
		dw.AddNode(EntityNode(api))
		dw.AddEdge(DotEdge{From: qn, To: apiQn})
	}
	for _, d := range component.Spec.DependsOn {
		e := r.Entity(d)
		if e != nil {
			xQn := e.GetQName()
			dw.AddNode(EntityNode(e))
			dw.AddEdge(DotEdge{From: qn, To: xQn, Style: ESDependsOn})
		}
	}

	dw.End()

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
	dw.Start()

	// API
	dw.AddNode(EntityNode(api))

	// Owner
	owner := r.Group(api.Spec.Owner)
	if owner != nil {
		ownerQn := owner.GetQName()
		dw.AddNode(EntityNode(owner))
		dw.AddEdge(DotEdge{From: ownerQn, To: qn, Style: ESOwner})
	}
	// System containing the API
	system := r.System(api.Spec.System)
	if system != nil {
		systemQn := system.GetQName()
		dw.AddNode(EntityNode(system))
		dw.AddEdge(DotEdge{From: systemQn, To: qn, Style: ESContains})
	}

	// Providers
	for _, p := range api.GetProviders() {
		provider := r.Component(p)
		if provider != nil {
			providerQn := provider.GetQName()
			dw.AddNode(EntityNode(provider))
			dw.AddEdge(DotEdge{From: qn, To: providerQn, Style: ESProvidedBy})
		}
	}

	// Consumers
	for _, c := range api.GetConsumers() {
		consumer := r.Component(c)
		if consumer != nil {
			consumerQn := consumer.GetQName()
			dw.AddNode(EntityNode(consumer))
			dw.AddEdge(DotEdge{From: consumerQn, To: qn})
		}
	}

	dw.End()

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
	dw.Start()

	// Resource
	dw.AddNode(EntityNode(resource))

	// Owner
	owner := r.Group(resource.Spec.Owner)
	if owner != nil {
		ownerQn := owner.GetQName()
		dw.AddNode(EntityNode(owner))
		dw.AddEdge(DotEdge{From: ownerQn, To: qn, Style: ESOwner})
	}
	// System containing the API
	system := r.System(resource.Spec.System)
	if system != nil {
		systemQn := system.GetQName()
		dw.AddNode(EntityNode(system))
		dw.AddEdge(DotEdge{From: systemQn, To: qn, Style: ESContains})
	}

	// Dependents
	for _, d := range resource.GetDependents() {
		dependent := r.Entity(d)
		if dependent != nil {
			dependentQn := dependent.GetQName()
			dw.AddNode(EntityNode(dependent))
			dw.AddEdge(DotEdge{From: dependentQn, To: qn, Style: ESDependsOn})
		}
	}

	dw.End()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return runDot(ctx, dw.String())
}
