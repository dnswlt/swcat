package backstage

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/dot"
)

func entityNodeKind(e api.Entity) dot.NodeKind {
	switch e.(type) {
	case *api.Domain:
		return dot.KindDomain
	case *api.System:
		return dot.KindSystem
	case *api.Component:
		return dot.KindComponent
	case *api.Resource:
		return dot.KindResource
	case *api.API:
		return dot.KindAPI
	case *api.Group:
		return dot.KindGroup
	default:
		return dot.KindUnspecified
	}
}

func EntityNode(e api.Entity) dot.Node {
	meta := e.GetMetadata()
	label := meta.Name
	if meta.Namespace != "" && meta.Namespace != api.DefaultNamespace {
		// Two-line label for namespaced entities.
		label = meta.Namespace + `/\n` + meta.Name
	}
	return dot.Node{
		ID:    e.GetRef(),
		Kind:  entityNodeKind(e),
		Label: label,
	}
}

func EntityEdge(from, to api.Entity, style dot.EdgeStyle) dot.Edge {
	return dot.Edge{
		From:  from.GetRef(),
		To:    to.GetRef(),
		Style: style,
	}
}

type SVGResult struct {
	// The dot-generated SVG output. Only contains the <svg> element,
	// <?xml> headers etc are stripped.
	SVG      []byte
	Metadata *dot.SVGGraphMetadata
}

func (d *SVGResult) MetadataJSON() []byte {
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
	source       api.Entity
	targetSystem *api.System
	direction    DependencyDir
}

// extSysPartDep represents an external dependency in a system diagram.
// In contrast to extSysDep, it points to a SystemPart of the target system,
// not the target system itself.
type extSysPartDep struct {
	source    api.Entity
	target    api.SystemPart
	direction DependencyDir
}

func (e extSysDep) String() string {
	return fmt.Sprintf("%s -> %s / %v", e.source.GetQName(), e.targetSystem.GetQName(), e.direction)
}

func generateDomainDotSource(r *Repository, domain *api.Domain) *dot.DotSource {
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

// GenerateDomainSVG generates an SVG for the given domain.
func GenerateDomainSVG(ctx context.Context, runner dot.Runner, r *Repository, domain *api.Domain) (*SVGResult, error) {
	dotSource := generateDomainDotSource(r, domain)
	return runDot(ctx, runner, dotSource)
}

func generateSystemDotSource(r *Repository, system *api.System, contextSystems []*api.System) *dot.DotSource {
	ctxSysMap := map[string]*api.System{}
	for _, ctxSys := range contextSystems {
		ctxSysMap[ctxSys.GetQName()] = ctxSys
	}

	dw := dot.New()
	dw.Start()

	var extDeps []extSysDep
	extSPDeps := map[string][]extSysPartDep{}
	// Adds the src->dst dependency to either extDeps or extSPDeps, depending on whether
	// full context was requested for dst.
	addDep := func(src api.SystemPart, dst api.SystemPart, dir DependencyDir) {
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
			ap := r.API(a)
			addDep(comp, ap, DirOutgoing)
		}
		// Add links for direct dependencies of the component.
		for _, d := range comp.Spec.DependsOn {
			entity := r.Entity(d)
			if sp, ok := entity.(api.SystemPart); ok && sp.GetSystem() != comp.GetSystem() {
				addDep(comp, sp, DirOutgoing)
			}
		}
		// Add links for direct dependents of the component.
		for _, d := range comp.GetDependents() {
			entity := r.Entity(d)
			if sp, ok := entity.(api.SystemPart); ok && sp.GetSystem() != comp.GetSystem() {
				addDep(comp, sp, DirIncoming)
			}
		}
	}

	// APIs
	for _, a := range system.GetAPIs() {
		ap := r.API(a)
		dw.AddNode(EntityNode(ap))

		// Add links for consumers of any API of this system.
		for _, c := range ap.GetConsumers() {
			consumer := r.Component(c)
			if consumer.GetSystem() != ap.GetSystem() {
				addDep(ap, consumer, DirIncoming)
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
			if sp, ok := entity.(api.SystemPart); ok && sp.GetSystem() != resource.GetSystem() {
				addDep(resource, sp, DirOutgoing)
			}
		}
		// Add links for direct dependents of the resource.
		for _, d := range resource.GetDependents() {
			entity := r.Entity(d)
			if sp, ok := entity.(api.SystemPart); ok && sp.GetSystem() != resource.GetSystem() {
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
				dw.AddEdge(EntityEdge(dep.source, dep.target, dot.ESNormal))
			} else {
				dw.AddEdge(EntityEdge(dep.target, dep.source, dot.ESNormal))
			}
		}
		dw.EndCluster()
	}
	dw.End()
	return dw.Result()
}

// GenerateSystemSVG generates an SVG for the given system.
// contextSystems are systems that should be expanded in the view. Other systems will be shown
// as opaque single nodes.
func GenerateSystemSVG(ctx context.Context, runner dot.Runner, r *Repository, system *api.System, contextSystems []*api.System) (*SVGResult, error) {
	dotSource := generateSystemDotSource(r, system, contextSystems)
	return runDot(ctx, runner, dotSource)
}

func generateComponentDotSource(r *Repository, component *api.Component) *dot.DotSource {
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
		ap := r.API(a)
		dw.AddNode(EntityNode(ap))
		dw.AddEdge(EntityEdge(ap, component, dot.ESProvidedBy))
	}
	for _, d := range component.GetDependents() {
		e := r.Entity(d)
		if e != nil {
			dw.AddNode(EntityNode(e))
			dw.AddEdge(EntityEdge(e, component, dot.ESDependsOn))
		}
	}

	// "Outgoing" dependencies
	// - Consumed APIs
	// - DependsOn relationships of this entity
	for _, a := range component.Spec.ConsumesAPIs {
		ap := r.API(a)
		dw.AddNode(EntityNode(ap))
		dw.AddEdge(EntityEdge(component, ap, dot.ESNormal))
	}
	for _, d := range component.Spec.DependsOn {
		e := r.Entity(d)
		if e != nil {
			dw.AddNode(EntityNode(e))
			dw.AddEdge(EntityEdge(component, e, dot.ESDependsOn))
		}
	}

	dw.End()
	return dw.Result()
}

// GenerateComponentSVG generates an SVG for the given component.
func GenerateComponentSVG(ctx context.Context, runner dot.Runner, r *Repository, component *api.Component) (*SVGResult, error) {
	dotSource := generateComponentDotSource(r, component)
	return runDot(ctx, runner, dotSource)
}

func generateAPIDotSource(r *Repository, api *api.API) *dot.DotSource {
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
		provider := r.Component(p)
		if provider != nil {
			dw.AddNode(EntityNode(provider))
			dw.AddEdge(EntityEdge(api, provider, dot.ESProvidedBy))
		}
	}

	// Consumers
	for _, c := range api.GetConsumers() {
		consumer := r.Component(c)
		if consumer != nil {
			dw.AddNode(EntityNode(consumer))
			dw.AddEdge(EntityEdge(consumer, api, dot.ESNormal))
		}
	}

	dw.End()
	return dw.Result()
}

// GenerateAPISVG generates an SVG for the given API.
func GenerateAPISVG(ctx context.Context, runner dot.Runner, r *Repository, api *api.API) (*SVGResult, error) {
	dotSource := generateAPIDotSource(r, api)
	return runDot(ctx, runner, dotSource)
}

func generateResourceDotSource(r *Repository, resource *api.Resource) *dot.DotSource {
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
		dependent := r.Entity(d)
		if dependent != nil {
			dw.AddNode(EntityNode(dependent))
			dw.AddEdge(EntityEdge(dependent, resource, dot.ESDependsOn))
		}
	}

	dw.End()
	return dw.Result()
}

// GenerateResourceSVG generates an SVG for the given resource.
func GenerateResourceSVG(ctx context.Context, runner dot.Runner, r *Repository, resource *api.Resource) (*SVGResult, error) {
	dotSource := generateResourceDotSource(r, resource)
	return runDot(ctx, runner, dotSource)
}

func runDot(ctx context.Context, runner dot.Runner, ds *dot.DotSource) (*SVGResult, error) {
	svg, err := runner.Run(ctx, ds.DotSource)
	if err != nil {
		return nil, fmt.Errorf("running dot failed: %w", err)
	}
	return &SVGResult{
		SVG:      svg,
		Metadata: ds.Metadata,
	}, nil
}
