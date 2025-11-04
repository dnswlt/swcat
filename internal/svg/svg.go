package svg

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/repo"
)

type Renderer struct {
	repo     *repo.Repository
	runner   dot.Runner
	layouter Layouter
}

func NewRenderer(r *repo.Repository, runner dot.Runner, layouter Layouter) *Renderer {
	return &Renderer{
		repo:     r,
		runner:   runner,
		layouter: layouter,
	}
}

func (r *Renderer) entityNode(e catalog.Entity) dot.Node {
	return dot.Node{
		ID:     e.GetRef().String(),
		Layout: r.layouter.Node(e),
	}
}

func (r *Renderer) entityNodeContext(e, contextEntity catalog.Entity) dot.Node {
	return dot.Node{
		ID:     e.GetRef().String(),
		Layout: r.layouter.NodeContext(e, contextEntity),
	}
}

func (r *Renderer) entityEdge(from, to catalog.Entity, style dot.EdgeStyle) dot.Edge {
	return dot.Edge{
		From:   from.GetRef().String(),
		To:     to.GetRef().String(),
		Layout: r.layouter.Edge(from, to, style),
	}
}

func (r *Renderer) entityEdgeLabel(from, to catalog.Entity, ref *catalog.LabelRef, style dot.EdgeStyle) dot.Edge {
	return dot.Edge{
		From:   from.GetRef().String(),
		To:     to.GetRef().String(),
		Layout: r.layouter.EdgeLabel(from, to, ref, style),
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
		panic(fmt.Sprintf("Cannot marshal MetadataJSON: %v", err))
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
	ref       *catalog.LabelRef
	direction DependencyDir
}

func (e extSysDep) String() string {
	return fmt.Sprintf("%s -> %s / %v", e.source.GetRef().String(), e.targetSystem.GetRef().String(), e.direction)
}

func (r *Renderer) generateDomainDotSource(domain *catalog.Domain) *dot.DotSource {
	dw := dot.New()
	dw.Start()

	dw.StartCluster(domain.GetQName())

	for _, s := range domain.GetSystems() {
		system := r.repo.System(s)
		dw.AddNode(r.entityNode(system))
	}

	dw.EndCluster()

	// Find relationships of components and resources in this domain to systems of other domains.
	type sysLink struct {
		src *catalog.System
		tgt *catalog.System
	}
	var systemLinks []sysLink
	seenLinks := make(map[string]bool)
	for _, ref := range domain.GetSystems() {
		s := r.repo.System(ref)

		var outgoing []*catalog.LabelRef
		var incoming []*catalog.LabelRef
		for _, cRef := range s.GetComponents() {
			c := r.repo.Component(cRef)
			outgoing = append(outgoing, c.Spec.DependsOn...)
			outgoing = append(outgoing, c.Spec.ConsumesAPIs...)
			incoming = append(incoming, c.GetDependents()...)
		}
		for _, aRef := range s.GetAPIs() {
			a := r.repo.API(aRef)
			incoming = append(incoming, a.GetConsumers()...)
		}
		for _, rRef := range s.GetResources() {
			res := r.repo.Resource(rRef)
			incoming = append(incoming, res.GetDependents()...)
		}
		addLink := func(ref *catalog.Ref, incoming bool) {
			e := r.repo.Entity(ref)
			if sp, ok := e.(catalog.SystemPart); ok {
				s2 := r.repo.System(sp.GetSystem())
				if s == s2 {
					return // Don't create self-links
				}
				var src, tgt *catalog.System
				if incoming {
					src, tgt = s2, s
				} else {
					src, tgt = s, s2
				}
				link := fmt.Sprintf("%s -> %s", src.GetRef(), tgt.GetRef())
				if _, ok := seenLinks[link]; !ok {
					systemLinks = append(systemLinks, sysLink{src: src, tgt: tgt})
				}
				seenLinks[link] = true
			}

		}
		for _, dRef := range outgoing {
			addLink(dRef.Ref, false)
		}
		for _, dRef := range incoming {
			addLink(dRef.Ref, true)
		}

	}
	for _, link := range systemLinks {
		dw.AddNode(r.entityNode(link.src))
		dw.AddNode(r.entityNode(link.tgt))
		dw.AddEdge(r.entityEdge(link.src, link.tgt, dot.ESNormal))
	}

	dw.End()

	return dw.Result()
}

// DomainGraph generates an SVG for the given domain.
func (r *Renderer) DomainGraph(ctx context.Context, domain *catalog.Domain) (*Result, error) {
	dotSource := r.generateDomainDotSource(domain)
	return runDot(ctx, r.runner, dotSource)
}

func (r *Renderer) generateSystemExternalDotSource(system *catalog.System, contextSystems []*catalog.System) *dot.DotSource {
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
	addExtDep := func(src, dst catalog.SystemPart, ref *catalog.LabelRef, dir DependencyDir) bool {
		if dst.GetSystem().Equal(src.GetSystem()) {
			return false
		}
		dstSys := r.repo.System(dst.GetSystem())
		if _, ok := ctxSysMap[dstSys.GetRef().QName()]; ok {
			// dst is part of a system for which we want to show full context.
			extSPDeps[dstSys.GetRef().QName()] = append(extSPDeps[dstSys.GetRef().QName()],
				extSysPartDep{source: src, target: dst, ref: ref, direction: dir},
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
		comp := r.repo.Component(c)
		hasEdges := false
		// Add links to external systems of which the component consumes APIs.
		for _, ref := range comp.Spec.ConsumesAPIs {
			ap := r.repo.API(ref.Ref)
			if addExtDep(comp, ap, ref, DirOutgoing) {
				hasEdges = true
			}
		}
		// Add links for direct dependencies of the component.
		for _, ref := range comp.Spec.DependsOn {
			entity := r.repo.Entity(ref.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				if addExtDep(comp, sp, ref, DirOutgoing) {
					hasEdges = true
				}
			}
		}
		// Add links for direct dependents of the component.
		for _, ref := range comp.GetDependents() {
			entity := r.repo.Entity(ref.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				hasEdges = hasEdges || addExtDep(comp, sp, ref, DirIncoming)
			}
		}
		if hasEdges {
			dw.AddNode(r.entityNode(comp))
		}
	}

	// APIs
	for _, a := range system.GetAPIs() {
		ap := r.repo.API(a)
		hasEdges := false
		// Add links for consumers of any API of this system.
		for _, c := range ap.GetConsumers() {
			consumer := r.repo.Component(c.Ref)
			if addExtDep(ap, consumer, c, DirIncoming) {
				hasEdges = true
			}
		}
		if hasEdges {
			dw.AddNode(r.entityNode(ap))
		}
	}

	// Resources
	for _, res := range system.GetResources() {
		resource := r.repo.Resource(res)
		hasEdges := false

		// Add links to external systems that the resource depends on.
		for _, d := range resource.Spec.DependsOn {
			entity := r.repo.Entity(d.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				if addExtDep(resource, sp, d, DirOutgoing) {
					hasEdges = true
				}
			}
		}
		// Add links for direct dependents of the resource.
		for _, d := range resource.GetDependents() {
			entity := r.repo.Entity(d.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				if addExtDep(resource, sp, d, DirIncoming) {
					hasEdges = true
				}
			}
		}
		if hasEdges {
			dw.AddNode(r.entityNode(resource))
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
		dw.AddNode(r.entityNode(dep.targetSystem))
		if dep.direction == DirOutgoing {
			dw.AddEdge(r.entityEdge(dep.source, dep.targetSystem, dot.ESSystemLink))
		} else {
			dw.AddEdge(r.entityEdge(dep.targetSystem, dep.source, dot.ESSystemLink))
		}
	}

	for systemID, deps := range extSPDeps {
		dw.StartCluster(systemID)
		for _, dep := range deps {
			dw.AddNode(r.entityNode(dep.target))
			if dep.direction == DirOutgoing {
				dw.AddEdge(r.entityEdgeLabel(dep.source, dep.target, dep.ref, dot.ESNormal))
			} else {
				dw.AddEdge(r.entityEdgeLabel(dep.target, dep.source, dep.ref, dot.ESNormal))
			}
		}
		dw.EndCluster()
	}
	dw.End()
	return dw.Result()
}

func (r *Renderer) generateSystemInternalDotSource(system *catalog.System) *dot.DotSource {
	dw := dot.NewWithConfig(dot.WriterConfig{
		EdgeMinLen: 1, // The internal view will have many nodes. Draw it as compactly as possible.
	})
	dw.Start()

	// Add nodes to the system cluster first to avoid any surprises with dot's rendering.
	// Edges are defined below, outside the cluster.
	dw.StartCluster(system.GetRef().QName())

	// Components
	for _, c := range system.GetComponents() {
		comp := r.repo.Component(c)
		dw.AddNode(r.entityNode(comp))
	}

	// APIs
	for _, a := range system.GetAPIs() {
		ap := r.repo.API(a)
		dw.AddNode(r.entityNode(ap))
	}

	// Resources
	for _, res := range system.GetResources() {
		resource := r.repo.Resource(res)
		dw.AddNode(r.entityNode(resource))
	}

	dw.EndCluster()

	// Convenience helper to add an internal dependency edge.
	addInternalDep := func(src, dst catalog.SystemPart, ref *catalog.LabelRef, style dot.EdgeStyle) {
		if !src.GetSystem().Equal(dst.GetSystem()) {
			return
		}
		dw.AddEdge(r.entityEdgeLabel(src, dst, ref, style))
	}

	// Components
	for _, c := range system.GetComponents() {
		comp := r.repo.Component(c)
		// API links
		for _, ref := range comp.Spec.ConsumesAPIs {
			ap := r.repo.API(ref.Ref)
			addInternalDep(comp, ap, ref, dot.ESNormal)
		}
		for _, ref := range comp.Spec.ProvidesAPIs {
			ap := r.repo.API(ref.Ref)
			addInternalDep(ap, comp, ref, dot.ESProvidedBy)
		}
		// Dependency links
		for _, ref := range comp.Spec.DependsOn {
			entity := r.repo.Entity(ref.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				addInternalDep(comp, sp, ref, dot.ESDependsOn)
			}
		}
	}

	// APIs don't have outgoing references.

	// Resources
	for _, res := range system.GetResources() {
		resource := r.repo.Resource(res)
		// Dependency links
		for _, d := range resource.Spec.DependsOn {
			entity := r.repo.Entity(d.Ref)
			if sp, ok := entity.(catalog.SystemPart); ok {
				addInternalDep(resource, sp, d, dot.ESDependsOn)
			}
		}
	}

	dw.End()

	return dw.Result()
}

// SystemExternalGraph generates an SVG for an "external" view of the given system.
// contextSystems are systems that should be expanded in the view. Other systems will be shown
// as opaque single nodes.
func (r *Renderer) SystemExternalGraph(ctx context.Context, system *catalog.System, contextSystems []*catalog.System) (*Result, error) {
	dotSource := r.generateSystemExternalDotSource(system, contextSystems)
	return runDot(ctx, r.runner, dotSource)
}

// SystemInternalGraph generates an SVG for an "internal" view of the given system.
// Only entities that are part of the system and their relationships are shown.
func (r *Renderer) SystemInternalGraph(ctx context.Context, system *catalog.System) (*Result, error) {
	dotSource := r.generateSystemInternalDotSource(system)
	return runDot(ctx, r.runner, dotSource)
}

func (r *Renderer) generateComponentDotSource(component *catalog.Component) *dot.DotSource {
	dw := dot.New()
	dw.Start()

	// Component
	dw.AddNode(r.entityNode(component))

	// "Incoming" dependencies
	// - Owner
	// - System
	// - Provided APIs
	// - Other entities with a DependsOn relationship to this entity
	owner := r.repo.Group(component.Spec.Owner)
	if owner != nil {
		dw.AddNode(r.entityNode(owner))
		dw.AddEdge(r.entityEdge(owner, component, dot.ESOwner))
	}
	system := r.repo.System(component.Spec.System)
	if system != nil {
		dw.AddNode(r.entityNode(system))
		dw.AddEdge(r.entityEdge(system, component, dot.ESContains))
	}
	for _, a := range component.Spec.ProvidesAPIs {
		ap := r.repo.API(a.Ref)
		dw.AddNode(r.entityNodeContext(ap, component))
		dw.AddEdge(r.entityEdgeLabel(ap, component, a, dot.ESProvidedBy))
	}
	for _, d := range component.GetDependents() {
		e := r.repo.Entity(d.Ref)
		if e != nil {
			dw.AddNode(r.entityNodeContext(e, component))
			dw.AddEdge(r.entityEdgeLabel(e, component, d, dot.ESDependsOn))
		}
	}

	// "Outgoing" dependencies
	// - Consumed APIs
	// - DependsOn relationships of this entity
	for _, a := range component.Spec.ConsumesAPIs {
		ap := r.repo.API(a.Ref)
		dw.AddNode(r.entityNodeContext(ap, component))
		dw.AddEdge(r.entityEdgeLabel(component, ap, a, dot.ESNormal))
	}
	for _, d := range component.Spec.DependsOn {
		e := r.repo.Entity(d.Ref)
		if e != nil {
			dw.AddNode(r.entityNodeContext(e, component))
			dw.AddEdge(r.entityEdgeLabel(component, e, d, dot.ESDependsOn))
		}
	}

	dw.End()
	return dw.Result()
}

// ComponentGraph generates an SVG for the given component.
func (r *Renderer) ComponentGraph(ctx context.Context, component *catalog.Component) (*Result, error) {
	dotSource := r.generateComponentDotSource(component)
	return runDot(ctx, r.runner, dotSource)
}

func (r *Renderer) generateAPIDotSource(api *catalog.API) *dot.DotSource {
	dw := dot.New()
	dw.Start()

	// API
	dw.AddNode(r.entityNode(api))

	// Owner
	owner := r.repo.Group(api.Spec.Owner)
	if owner != nil {
		dw.AddNode(r.entityNode(owner))
		dw.AddEdge(r.entityEdge(owner, api, dot.ESOwner))
	}
	// System containing the API
	system := r.repo.System(api.Spec.System)
	if system != nil {
		dw.AddNode(r.entityNode(system))
		dw.AddEdge(r.entityEdge(system, api, dot.ESContains))
	}

	// Providers
	for _, p := range api.GetProviders() {
		provider := r.repo.Component(p.Ref)
		if provider != nil {
			dw.AddNode(r.entityNodeContext(provider, api))
			dw.AddEdge(r.entityEdgeLabel(api, provider, p, dot.ESProvidedBy))
		}
	}

	// Consumers
	for _, c := range api.GetConsumers() {
		consumer := r.repo.Component(c.Ref)
		if consumer != nil {
			dw.AddNode(r.entityNodeContext(consumer, api))
			dw.AddEdge(r.entityEdgeLabel(consumer, api, c, dot.ESNormal))
		}
	}

	dw.End()
	return dw.Result()
}

// APIGraph generates an SVG for the given API.
func (r *Renderer) APIGraph(ctx context.Context, api *catalog.API) (*Result, error) {
	dotSource := r.generateAPIDotSource(api)
	return runDot(ctx, r.runner, dotSource)
}

func (r *Renderer) generateResourceDotSource(resource *catalog.Resource) *dot.DotSource {
	dw := dot.New()
	dw.Start()

	// Resource
	dw.AddNode(r.entityNode(resource))

	// Owner
	owner := r.repo.Group(resource.Spec.Owner)
	if owner != nil {
		dw.AddNode(r.entityNode(owner))
		dw.AddEdge(r.entityEdge(owner, resource, dot.ESOwner))
	}
	// System containing the API
	system := r.repo.System(resource.Spec.System)
	if system != nil {
		dw.AddNode(r.entityNode(system))
		dw.AddEdge(r.entityEdge(system, resource, dot.ESContains))
	}

	// Dependents
	for _, d := range resource.GetDependents() {
		dependent := r.repo.Entity(d.Ref)
		if dependent != nil {
			dw.AddNode(r.entityNode(dependent))
			dw.AddEdge(r.entityEdgeLabel(dependent, resource, d, dot.ESDependsOn))
		}
	}

	dw.End()
	return dw.Result()
}

// ResourceGraph generates an SVG for the given resource.
func (r *Renderer) ResourceGraph(ctx context.Context, resource *catalog.Resource) (*Result, error) {
	dotSource := r.generateResourceDotSource(resource)
	return runDot(ctx, r.runner, dotSource)
}

func runDot(ctx context.Context, runner dot.Runner, ds *dot.DotSource) (*Result, error) {
	svg, err := runner.Run(ctx, ds.DotSource)
	if err != nil {
		return nil, fmt.Errorf("running dot failed: %w", err)
	}
	svg, err = PostprocessSVG(svg)
	if err != nil {
		return nil, fmt.Errorf("postprocessing failed: %w", err)
	}

	return &Result{
		SVG:      svg,
		Metadata: ds.Metadata,
	}, nil
}
