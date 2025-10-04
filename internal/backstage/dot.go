package backstage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"time"
)

type NodeInfo struct {
	Label string `json:"label"`
}
type EdgeInfo struct {
	From  string `json:"from"`  // From entity reference (e.g. api:ns1/super-api).
	To    string `json:"to"`    // To entity reference.
	Label string `json:"label"` // Optional edge label.
}
type SVGGraphMetadata struct {
	// Maps the IDs of nodes in the generated SVG (id= attributes) to their node info.
	Nodes map[string]*NodeInfo `json:"nodes"`
	// Maps the IDs of edges in the generated SVG (id= attributes) to their edge info.
	Edges map[string]*EdgeInfo `json:"edges"`
}

type SVGResult struct {
	// The dot-generated SVG output. Only contains the <svg> element,
	// <?xml> headers etc are stripped.
	SVG      []byte
	Metadata *SVGGraphMetadata
}

func (d *SVGResult) MetadataJSON() []byte {
	json, err := json.Marshal(d.Metadata)
	if err != nil {
		// This is truly an application bug.
		log.Fatalf("Cannot marshal MetadataJSON: %v", err)
	}
	return json
}

func runDot(ctx context.Context, dotSource string) ([]byte, error) {
	// Command: dot -Tsvg
	log.Println("Running 'dot -Tsvg' to generate SVG")
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
	KindUnspecified NodeKind = iota
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
	Kind  NodeKind
	Label string
}

func EntityNode(e Entity) DotNode {
	meta := e.GetMetadata()
	label := meta.Name
	if meta.Namespace != "" && meta.Namespace != DefaultNamespace {
		// Two-line label for namespaced entities.
		label = meta.Namespace + `/\n` + meta.Name
	}
	return DotNode{
		ID:    e.GetRef(),
		Kind:  entityNodeKind(e),
		Label: label,
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

type DotEdge struct {
	From  string
	To    string
	Label string
	Style EdgeStyle
}

func EntityEdge(from, to Entity, style EdgeStyle) DotEdge {
	return DotEdge{
		From:  from.GetRef(),
		To:    to.GetRef(),
		Style: style,
	}
}

type DotWriter struct {
	w        *strings.Builder
	nodeInfo map[string]*NodeInfo
	edgeInfo map[string]*EdgeInfo
}

func NewDotWriter() *DotWriter {
	return &DotWriter{
		w:        &strings.Builder{},
		nodeInfo: make(map[string]*NodeInfo),
		edgeInfo: make(map[string]*EdgeInfo),
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

func (dw *DotWriter) AddEdge(edge DotEdge) {
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
		var items []string
		for k, v := range attrs {
			items = append(items, fmt.Sprintf("%s=\"%s\"", k, v))
		}
		edgeAttrs = "[" + strings.Join(items, ",") + "]"
	}

	fmt.Fprintf(dw.w, `"%s" -> "%s"%s`, edge.From, edge.To, edgeAttrs)
	fmt.Fprintln(dw.w)
}

func (dw *DotWriter) StartCluster(name string) {
	fmt.Fprintf(dw.w, "subgraph \"cluster_%s\" {\n", name)
	fmt.Fprintf(dw.w, "label=\"%s\"\n", name)
	fmt.Fprintf(dw.w, "style=filled\n")
	fmt.Fprintf(dw.w, "fillcolor=\"#f3f4f6\"\n")
}

func (dw *DotWriter) EndCluster() {
	dw.w.WriteString("}\n")
}

func (dw *DotWriter) String() string {
	return dw.w.String()
}

func (dw *DotWriter) Metadata() *SVGGraphMetadata {
	return &SVGGraphMetadata{
		Nodes: dw.nodeInfo,
		Edges: dw.edgeInfo,
	}
}

func generateSVGResult(ctx context.Context, dw *DotWriter) (*SVGResult, error) {
	svg, err := runDot(ctx, dw.String())
	if err != nil {
		return nil, err
	}
	return &SVGResult{
		SVG:      svg,
		Metadata: dw.Metadata(),
	}, nil
}

// GenerateDomainSVG generates an SVG for the given domain.
func GenerateDomainSVG(r *Repository, domain *Domain) (*SVGResult, error) {
	dw := NewDotWriter()
	dw.Start()

	dw.StartCluster(domain.GetQName())

	for _, s := range domain.GetSystems() {
		system := r.System(s)
		dw.AddNode(EntityNode(system))
	}

	dw.EndCluster()

	dw.End()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return generateSVGResult(ctx, dw)
}

type DependencyDir int

const (
	DirIncoming DependencyDir = iota
	DirOutgoing
)

// extSysPartDep represents a dependency on an external system in a system diagram.
type extSysDep struct {
	source       Entity
	targetSystem *System
	direction    DependencyDir
}

// extSysPartDep represents an external dependency in a system diagram.
// In contrast to extSysDep, it points to a SystemPart of the target system,
// not the target system itself.
type extSysPartDep struct {
	source    Entity
	target    SystemPart
	direction DependencyDir
}

func (e extSysDep) String() string {
	return fmt.Sprintf("%s -> %s / %s", e.source.GetQName(), e.targetSystem.GetQName(), e.direction)
}

// GenerateSystemSVG generates an SVG for the given system.
// contextSystems are systems that should be expanded in the view. Other systems will be shown
// as opaque single nodes.
func GenerateSystemSVG(r *Repository, system *System, contextSystems []*System) (*SVGResult, error) {
	ctxSysMap := map[string]*System{}
	for _, ctxSys := range contextSystems {
		ctxSysMap[ctxSys.GetQName()] = ctxSys
	}

	dw := NewDotWriter()
	dw.Start()

	var extDeps []extSysDep
	extSPDeps := map[string][]extSysPartDep{}
	// Adds the src->dst dependency to either extDeps or extSPDeps, depending on whether
	// full context was requested for dst.
	addDep := func(src SystemPart, dst SystemPart, dir DependencyDir) {
		if dst.GetSystem() != src.GetSystem() {
			dstSys := r.System(dst.GetSystem())
			if _, ok := ctxSysMap[dstSys.GetQName()]; ok {
				// dst is part of a system for which we want to show full context
				extSPDeps[dstSys.GetQName()] = append(extSPDeps[dstSys.GetQName()],
					extSysPartDep{source: src, target: dst, direction: dir},
				)
			} else {
				extDeps = append(extDeps, extSysDep{
					source: src, targetSystem: dstSys, direction: dir,
				})
			}
		}
	}

	dw.StartCluster(system.GetQName())

	// Components
	for _, c := range system.GetComponents() {
		comp := r.Component(c)
		dw.AddNode(EntityNode(comp))

		// Add links to external systems of which the component consumes APIs.
		for _, a := range comp.Spec.ConsumesAPIs {
			api := r.API(a)
			addDep(comp, api, DirOutgoing)
		}
		// Add links for direct dependencies of the component.
		for _, d := range comp.Spec.DependsOn {
			entity := r.Entity(d)
			if sp, ok := entity.(SystemPart); ok && sp.GetSystem() != comp.GetSystem() {
				addDep(comp, sp, DirOutgoing)
			}
		}
		// Add links for direct dependents of the component.
		for _, d := range comp.GetDependents() {
			entity := r.Entity(d)
			if sp, ok := entity.(SystemPart); ok && sp.GetSystem() != comp.GetSystem() {
				addDep(comp, sp, DirIncoming)
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
				addDep(api, consumer, DirIncoming)
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
				addDep(resource, sp, DirOutgoing)
			}
		}
		// Add links for direct dependents of the resource.
		for _, d := range resource.GetDependents() {
			entity := r.Entity(d)
			if sp, ok := entity.(SystemPart); ok && sp.GetSystem() != resource.GetSystem() {
				addDep(resource, sp, DirIncoming)
			}
		}
	}

	dw.EndCluster()

	// Draw edges for all collected external dependencies, removing duplicates
	seenDeps := map[string]bool{}
	for _, dep := range extDeps {
		if seenDeps[dep.String()] {
			continue
		}
		seenDeps[dep.String()] = true
		dw.AddNode(EntityNode(dep.targetSystem))
		if dep.direction == DirOutgoing {
			dw.AddEdge(EntityEdge(dep.source, dep.targetSystem, ESSystemLink))
		} else {
			dw.AddEdge(EntityEdge(dep.targetSystem, dep.source, ESSystemLink))
		}
	}

	for systemID, deps := range extSPDeps {
		dw.StartCluster(systemID)
		for _, dep := range deps {
			dw.AddNode(EntityNode(dep.target))
			// TODO: use proper edge styles, not always normal.
			if dep.direction == DirOutgoing {
				dw.AddEdge(EntityEdge(dep.source, dep.target, ESNormal))
			} else {
				dw.AddEdge(EntityEdge(dep.target, dep.source, ESNormal))
			}
		}
		dw.EndCluster()
	}
	dw.End()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return generateSVGResult(ctx, dw)
}

// GenerateComponentSVG generates an SVG for the given component.
func GenerateComponentSVG(r *Repository, component *Component) (*SVGResult, error) {
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
		dw.AddNode(EntityNode(owner))
		dw.AddEdge(EntityEdge(owner, component, ESOwner))
	}
	system := r.System(component.Spec.System)
	if system != nil {
		dw.AddNode(EntityNode(system))
		dw.AddEdge(EntityEdge(system, component, ESContains))
	}
	for _, a := range component.Spec.ProvidesAPIs {
		api := r.API(a)
		dw.AddNode(EntityNode(api))
		dw.AddEdge(EntityEdge(api, component, ESProvidedBy))
	}
	for _, d := range component.Spec.dependents {
		e := r.Entity(d)
		if e != nil {
			dw.AddNode(EntityNode(e))
			dw.AddEdge(EntityEdge(e, component, ESDependsOn))
		}
	}

	// "Outgoing" dependencies
	// - Consumed APIs
	// - DependsOn relationships of this entity
	for _, a := range component.Spec.ConsumesAPIs {
		api := r.API(a)
		dw.AddNode(EntityNode(api))
		dw.AddEdge(EntityEdge(component, api, ESNormal))
	}
	for _, d := range component.Spec.DependsOn {
		e := r.Entity(d)
		if e != nil {
			dw.AddNode(EntityNode(e))
			dw.AddEdge(EntityEdge(component, e, ESDependsOn))
		}
	}

	dw.End()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return generateSVGResult(ctx, dw)
}

// GenerateAPISVG generates an SVG for the given API.
func GenerateAPISVG(r *Repository, api *API) (*SVGResult, error) {
	dw := NewDotWriter()
	dw.Start()

	// API
	dw.AddNode(EntityNode(api))

	// Owner
	owner := r.Group(api.Spec.Owner)
	if owner != nil {
		dw.AddNode(EntityNode(owner))
		dw.AddEdge(EntityEdge(owner, api, ESOwner))
	}
	// System containing the API
	system := r.System(api.Spec.System)
	if system != nil {
		dw.AddNode(EntityNode(system))
		dw.AddEdge(EntityEdge(system, api, ESContains))
	}

	// Providers
	for _, p := range api.GetProviders() {
		provider := r.Component(p)
		if provider != nil {
			dw.AddNode(EntityNode(provider))
			dw.AddEdge(EntityEdge(api, provider, ESProvidedBy))
		}
	}

	// Consumers
	for _, c := range api.GetConsumers() {
		consumer := r.Component(c)
		if consumer != nil {
			dw.AddNode(EntityNode(consumer))
			dw.AddEdge(EntityEdge(consumer, api, ESNormal))
		}
	}

	dw.End()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return generateSVGResult(ctx, dw)
}

// GenerateResourceSVG generates an SVG for the given resource.
func GenerateResourceSVG(r *Repository, resource *Resource) (*SVGResult, error) {
	dw := NewDotWriter()
	dw.Start()

	// Resource
	dw.AddNode(EntityNode(resource))

	// Owner
	owner := r.Group(resource.Spec.Owner)
	if owner != nil {
		dw.AddNode(EntityNode(owner))
		dw.AddEdge(EntityEdge(owner, resource, ESOwner))
	}
	// System containing the API
	system := r.System(resource.Spec.System)
	if system != nil {
		dw.AddNode(EntityNode(system))
		dw.AddEdge(EntityEdge(system, resource, ESContains))
	}

	// Dependents
	for _, d := range resource.GetDependents() {
		dependent := r.Entity(d)
		if dependent != nil {
			dw.AddNode(EntityNode(dependent))
			dw.AddEdge(EntityEdge(dependent, resource, ESDependsOn))
		}
	}

	dw.End()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return generateSVGResult(ctx, dw)
}
