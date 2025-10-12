package svg

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/repo"
)

const (
	AnnotSterotype = "swcat/stereotype"
	AnnotFillColor = "swcat/fillcolor"
)

func entityNodeKind(e catalog.Entity) dot.NodeKind {
	switch e.(type) {
	case *catalog.Domain:
		return dot.KindDomain
	case *catalog.System:
		return dot.KindSystem
	case *catalog.Component:
		return dot.KindComponent
	case *catalog.Resource:
		return dot.KindResource
	case *catalog.API:
		return dot.KindAPI
	case *catalog.Group:
		return dot.KindGroup
	default:
		return dot.KindUnspecified
	}
}

func EntityNode(e catalog.Entity) dot.Node {
	meta := e.GetMetadata()
	// Label
	var label strings.Builder
	if st, ok := meta.Annotations[AnnotSterotype]; ok {
		label.WriteString(`&laquo;` + st + `&raquo;\n`)
	}
	if meta.Namespace != "" && meta.Namespace != catalog.DefaultNamespace {
		// Two-line label for namespaced entities.
		label.WriteString(meta.Namespace + `/\n`)
	}
	label.WriteString(meta.Name)
	// FillColor
	var fillColor string
	if c, ok := meta.Annotations[AnnotFillColor]; ok {
		fillColor = c
	}

	return dot.Node{
		ID:        e.GetRef().String(),
		Kind:      entityNodeKind(e),
		Label:     label.String(),
		FillColor: fillColor,
	}
}

func EntityEdge(from, to catalog.Entity, style dot.EdgeStyle) dot.Edge {
	return EntityEdgeLabel(from, to, "", style)
}

func EntityEdgeLabel(from, to catalog.Entity, label string, style dot.EdgeStyle) dot.Edge {
	return dot.Edge{
		From:  from.GetRef().String(),
		To:    to.GetRef().String(),
		Label: label,
		Style: style,
	}
}

type Result struct {
	// The dot-generated SVG output. Only contains the <svg> element,
	// <?xml> headers etc are stripped.
	SVG      []byte
	Metadata *dot.SVGGraphMetadata
}

func (d *Result) MetadataJSON() []byte {
	json, err := json.Marshal(d.Metadata)
	if err != nil {
		// This is truly an application bug.
		log.Fatalf("Cannot marshal MetadataJSON: %v", err)
	}
	return json
}

type DependencyDir int

const (
	DirIncoming DependencyDir = iota
	DirOutgoing
)

// extSysPartDep represents a dependency on an external system in a system diagram.
type extSysDep struct {
	source       catalog.Entity
	targetSystem *catalog.System
	direction    DependencyDir
}

// extSysPartDep represents an external dependency in a system diagram.
// In contrast to extSysDep, it points to a SystemPart of the target system,
// not the target system itself.
type extSysPartDep struct {
	source    catalog.Entity
	target    catalog.SystemPart
	label     string
	direction DependencyDir
}

func (e extSysDep) String() string {
	return fmt.Sprintf("%s -> %s / %v", e.source.GetRef().String(), e.targetSystem.GetRef().String(), e.direction)
}

func generateDomainDotSource(r *repo.Repository, domain *catalog.Domain) *dot.DotSource {
	dw := dot.New()
	dw.Start()

	dw.StartCluster(domain.GetQName())

	for _, s := range domain.GetSystems() {
		system := r.System(s)
		dw.AddNode(EntityNode(system))
	}

	dw.EndCluster()

	dw.End()

	return dw.Result()
}

// DomainGraph generates an SVG for the given domain.
func DomainGraph(ctx context.Context, runner dot.Runner, r *repo.Repository, domain *catalog.Domain) (*Result, error) {
	dotSource := generateDomainDotSource(r, domain)
	return runDot(ctx, runner, dotSource)
}

func generateSystemExternalDotSource(r *repo.Repository, system *catalog.System, contextSystems []*catalog.System) *dot.DotSource {
	// Potential neighboring systems for which a detailed view is requested.
	ctxSysMap := map[string]*catalog.System{}
	for _, ctxSys := range contextSystems {
		ctxSysMap[ctxSys.GetRef().QName()] = ctxSys
	}

	dw := dot.New()
	dw.Start()

	var extDeps []extSysDep
	extSPDeps := map[string][]extSysPartDep{}
	// Adds the src->dst dependency to either extDeps or extSPDeps, depending on whether
	// full context was requested for dst.
	// Ignores intra-system dependencies.
	// Returns true if the dependency was added.
	addExtDep := func(src, dst catalog.SystemPart, label string, dir DependencyDir) bool {
		if dst.GetSystem().Equal(src.GetSystem()) {
			return false
		}
		dstSys := r.System(dst.GetSystem())
		if _, ok := ctxSysMap[dstSys.GetRef().QName()]; ok {
			// dst is part of a system for which we want to show full context.
			extSPDeps[dstSys.GetRef().QName()] = append(extSPDeps[dstSys.GetRef().QName()],
				extSysPartDep{source: src, target: dst, label: label, direction: dir},
			)
		} else {
			// dst is part of a system for which no context was requested.
			extDeps = append(extDeps, extSysDep{
				source: src, targetSystem: dstSys, direction: dir,
			})
		}
		return true
	}

	dw.StartCluster(system.GetRef().QName())

	// Components
	for _, c := range system.GetComponents() {
		comp := r.Component(c)
		hasEdges := false
		// Add links to external systems of which the component consumes APIs.
		for _, ref := range comp.Spec.ConsumesAPIs {
			ap := r.API(ref.Ref)
			if addExtDep(comp, ap, ref.Label, DirOutgoing) {
				hasEdges = true
			}
		}
		// Add links for direct dependencies of the component.
		for _, ref := range comp.Spec.DependsOn {
			entity := r.Entity(ref.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				if addExtDep(comp, sp, ref.Label, DirOutgoing) {
					hasEdges = true
				}
			}
		}
		// Add links for direct dependents of the component.
		for _, ref := range comp.GetDependents() {
			entity := r.Entity(ref.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				hasEdges = hasEdges || addExtDep(comp, sp, ref.Label, DirIncoming)
			}
		}
		if hasEdges {
			dw.AddNode(EntityNode(comp))
		}
	}

	// APIs
	for _, a := range system.GetAPIs() {
		ap := r.API(a)
		hasEdges := false
		// Add links for consumers of any API of this system.
		for _, c := range ap.GetConsumers() {
			consumer := r.Component(c.Ref)
			if addExtDep(ap, consumer, c.Label, DirIncoming) {
				hasEdges = true
			}
		}
		if hasEdges {
			dw.AddNode(EntityNode(ap))
		}
	}

	// Resources
	for _, res := range system.GetResources() {
		resource := r.Resource(res)
		hasEdges := false

		// Add links to external systems that the resource depends on.
		for _, d := range resource.Spec.DependsOn {
			entity := r.Entity(d.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				if addExtDep(resource, sp, d.Label, DirOutgoing) {
					hasEdges = true
				}
			}
		}
		// Add links for direct dependents of the resource.
		for _, d := range resource.GetDependents() {
			entity := r.Entity(d.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				if addExtDep(resource, sp, d.Label, DirIncoming) {
					hasEdges = true
				}
			}
		}
		if hasEdges {
			dw.AddNode(EntityNode(resource))
		}
	}

	dw.EndCluster()

	// Draw edges for all collected external dependencies, removing duplicates.
	seenDeps := map[string]bool{}
	for _, dep := range extDeps {
		if seenDeps[dep.String()] {
			continue
		}
		seenDeps[dep.String()] = true
		dw.AddNode(EntityNode(dep.targetSystem))
		if dep.direction == DirOutgoing {
			dw.AddEdge(EntityEdge(dep.source, dep.targetSystem, dot.ESSystemLink))
		} else {
			dw.AddEdge(EntityEdge(dep.targetSystem, dep.source, dot.ESSystemLink))
		}
	}

	for systemID, deps := range extSPDeps {
		dw.StartCluster(systemID)
		for _, dep := range deps {
			dw.AddNode(EntityNode(dep.target))
			if dep.direction == DirOutgoing {
				dw.AddEdge(EntityEdgeLabel(dep.source, dep.target, dep.label, dot.ESNormal))
			} else {
				dw.AddEdge(EntityEdgeLabel(dep.target, dep.source, dep.label, dot.ESNormal))
			}
		}
		dw.EndCluster()
	}
	dw.End()
	return dw.Result()
}

func generateSystemInternalDotSource(r *repo.Repository, system *catalog.System) *dot.DotSource {
	dw := dot.NewWithConfig(dot.WriterConfig{
		EdgeMinLen: 1, // The internal view will have many nodes. Draw it as compactly as possible.
	})
	dw.Start()

	// Add nodes to the system cluster first to avoid any surprises with dot's rendering.
	// Edges are defined below, outside the cluster.
	dw.StartCluster(system.GetRef().QName())

	// Components
	for _, c := range system.GetComponents() {
		comp := r.Component(c)
		dw.AddNode(EntityNode(comp))
	}

	// APIs
	for _, a := range system.GetAPIs() {
		ap := r.API(a)
		dw.AddNode(EntityNode(ap))
	}

	// Resources
	for _, res := range system.GetResources() {
		resource := r.Resource(res)
		dw.AddNode(EntityNode(resource))
	}

	dw.EndCluster()

	// Convenience helper to add an internal dependency edge.
	addInternalDep := func(src, dst catalog.SystemPart, label string, style dot.EdgeStyle) {
		if !src.GetSystem().Equal(dst.GetSystem()) {
			return
		}
		dw.AddEdge(EntityEdgeLabel(src, dst, label, style))
	}

	// Components
	for _, c := range system.GetComponents() {
		comp := r.Component(c)
		// API links
		for _, ref := range comp.Spec.ConsumesAPIs {
			ap := r.API(ref.Ref)
			addInternalDep(comp, ap, ref.Label, dot.ESNormal)
		}
		for _, ref := range comp.Spec.ProvidesAPIs {
			ap := r.API(ref.Ref)
			addInternalDep(ap, comp, ref.Label, dot.ESProvidedBy)
		}
		// Dependency links
		for _, ref := range comp.Spec.DependsOn {
			entity := r.Entity(ref.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				addInternalDep(comp, sp, ref.Label, dot.ESDependsOn)
			}
		}
	}

	// APIs don't have outgoing references.

	// Resources
	for _, res := range system.GetResources() {
		resource := r.Resource(res)
		// Dependency links
		for _, d := range resource.Spec.DependsOn {
			entity := r.Entity(d.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				addInternalDep(resource, sp, d.Label, dot.ESDependsOn)
			}
		}
	}

	dw.End()

	return dw.Result()
}

// SystemExternalGraph generates an SVG for an "external" view of the given system.
// contextSystems are systems that should be expanded in the view. Other systems will be shown
// as opaque single nodes.
func SystemExternalGraph(ctx context.Context, runner dot.Runner, r *repo.Repository, system *catalog.System, contextSystems []*catalog.System) (*Result, error) {
	dotSource := generateSystemExternalDotSource(r, system, contextSystems)
	return runDot(ctx, runner, dotSource)
}

// SystemInternalGraph generates an SVG for an "internal" view of the given system.
// Only entities that are part of the system and their relationships are shown.
func SystemInternalGraph(ctx context.Context, runner dot.Runner, r *repo.Repository, system *catalog.System) (*Result, error) {
	dotSource := generateSystemInternalDotSource(r, system)
	return runDot(ctx, runner, dotSource)
}

func generateComponentDotSource(r *repo.Repository, component *catalog.Component) *dot.DotSource {
	dw := dot.New()
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
		dw.AddEdge(EntityEdge(owner, component, dot.ESOwner))
	}
	system := r.System(component.Spec.System)
	if system != nil {
		dw.AddNode(EntityNode(system))
		dw.AddEdge(EntityEdge(system, component, dot.ESContains))
	}
	for _, a := range component.Spec.ProvidesAPIs {
		ap := r.API(a.Ref)
		dw.AddNode(EntityNode(ap))
		dw.AddEdge(EntityEdgeLabel(ap, component, a.Label, dot.ESProvidedBy))
	}
	for _, d := range component.GetDependents() {
		e := r.Entity(d.Ref)
		if e != nil {
			dw.AddNode(EntityNode(e))
			dw.AddEdge(EntityEdgeLabel(e, component, d.Label, dot.ESDependsOn))
		}
	}

	// "Outgoing" dependencies
	// - Consumed APIs
	// - DependsOn relationships of this entity
	for _, a := range component.Spec.ConsumesAPIs {
		ap := r.API(a.Ref)
		dw.AddNode(EntityNode(ap))
		dw.AddEdge(EntityEdgeLabel(component, ap, a.Label, dot.ESNormal))
	}
	for _, d := range component.Spec.DependsOn {
		e := r.Entity(d.Ref)
		if e != nil {
			dw.AddNode(EntityNode(e))
			dw.AddEdge(EntityEdgeLabel(component, e, d.Label, dot.ESDependsOn))
		}
	}

	dw.End()
	return dw.Result()
}

// ComponentGraph generates an SVG for the given component.
func ComponentGraph(ctx context.Context, runner dot.Runner, r *repo.Repository, component *catalog.Component) (*Result, error) {
	dotSource := generateComponentDotSource(r, component)
	return runDot(ctx, runner, dotSource)
}

func generateAPIDotSource(r *repo.Repository, api *catalog.API) *dot.DotSource {
	dw := dot.New()
	dw.Start()

	// API
	dw.AddNode(EntityNode(api))

	// Owner
	owner := r.Group(api.Spec.Owner)
	if owner != nil {
		dw.AddNode(EntityNode(owner))
		dw.AddEdge(EntityEdge(owner, api, dot.ESOwner))
	}
	// System containing the API
	system := r.System(api.Spec.System)
	if system != nil {
		dw.AddNode(EntityNode(system))
		dw.AddEdge(EntityEdge(system, api, dot.ESContains))
	}

	// Providers
	for _, p := range api.GetProviders() {
		provider := r.Component(p.Ref)
		if provider != nil {
			dw.AddNode(EntityNode(provider))
			dw.AddEdge(EntityEdgeLabel(api, provider, p.Label, dot.ESProvidedBy))
		}
	}

	// Consumers
	for _, c := range api.GetConsumers() {
		consumer := r.Component(c.Ref)
		if consumer != nil {
			dw.AddNode(EntityNode(consumer))
			dw.AddEdge(EntityEdgeLabel(consumer, api, c.Label, dot.ESNormal))
		}
	}

	dw.End()
	return dw.Result()
}

// APIGraph generates an SVG for the given API.
func APIGraph(ctx context.Context, runner dot.Runner, r *repo.Repository, api *catalog.API) (*Result, error) {
	dotSource := generateAPIDotSource(r, api)
	return runDot(ctx, runner, dotSource)
}

func generateResourceDotSource(r *repo.Repository, resource *catalog.Resource) *dot.DotSource {
	dw := dot.New()
	dw.Start()

	// Resource
	dw.AddNode(EntityNode(resource))

	// Owner
	owner := r.Group(resource.Spec.Owner)
	if owner != nil {
		dw.AddNode(EntityNode(owner))
		dw.AddEdge(EntityEdge(owner, resource, dot.ESOwner))
	}
	// System containing the API
	system := r.System(resource.Spec.System)
	if system != nil {
		dw.AddNode(EntityNode(system))
		dw.AddEdge(EntityEdge(system, resource, dot.ESContains))
	}

	// Dependents
	for _, d := range resource.GetDependents() {
		dependent := r.Entity(d.Ref)
		if dependent != nil {
			dw.AddNode(EntityNode(dependent))
			dw.AddEdge(EntityEdgeLabel(dependent, resource, d.Label, dot.ESDependsOn))
		}
	}

	dw.End()
	return dw.Result()
}

// ResourceGraph generates an SVG for the given resource.
func ResourceGraph(ctx context.Context, runner dot.Runner, r *repo.Repository, resource *catalog.Resource) (*Result, error) {
	dotSource := generateResourceDotSource(r, resource)
	return runDot(ctx, runner, dotSource)
}

func runDot(ctx context.Context, runner dot.Runner, ds *dot.DotSource) (*Result, error) {
	svg, err := runner.Run(ctx, ds.DotSource)
	if err != nil {
		return nil, fmt.Errorf("running dot failed: %w", err)
	}
	return &Result{
		SVG:      svg,
		Metadata: ds.Metadata,
	}, nil
}
