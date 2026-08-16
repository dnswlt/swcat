package svg

import (
	"context"
	"fmt"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/sysview"
)

// Renderer is the stateless top-level entry point for rendering catalog SVGs.
// It is safe for concurrent use; per-call state lives in the render struct.
type Renderer struct {
	repo   *repo.Repository
	runner dot.Runner
	config Config
}

func NewRenderer(r *repo.Repository, runner dot.Runner, config Config) *Renderer {
	return &Renderer{
		repo:   r,
		runner: runner,
		config: config,
	}
}

// render is a per-call rendering context. It bundles the diagram kind and
// focal entity being rendered with the immutable Renderer (for repo/config
// access). One render is created per public *Graph call, scoped to that call
// only.
type render struct {
	*Renderer
	kind DiagramKind
	// focalEntity is the focal entity of the diagram (e.g. the component
	// for ComponentGraph). It informs labelling decisions, like whether to
	// show another entity's parent system because it crosses system boundaries.
	// nil for ad-hoc views with no single focal entity.
	focalEntity catalog.Entity
}

func (r *render) entityNode(e catalog.Entity) dot.Node {
	return dot.Node{
		ID:     e.GetRef().String(),
		Layout: r.nodeLayout(e),
	}
}

func (r *render) entityEdge(from, to catalog.Entity, style dot.EdgeStyle) dot.Edge {
	return dot.Edge{
		From: from.GetRef().String(),
		To:   to.GetRef().String(),
		Layout: dot.EdgeLayout{
			Style: style,
			// Every edge gets a tooltip title, so that hovering any of them
			// says what it connects even when it carries no label.
			TooltipTitle: from.GetQName() + " → " + to.GetQName(),
		},
	}
}

func (r *render) entityEdgeLabel(from, to catalog.Entity, ref *catalog.LabelRef, style dot.EdgeStyle) dot.Edge {
	return dot.Edge{
		From:   from.GetRef().String(),
		To:     to.GetRef().String(),
		Layout: r.edgeLabelLayout(from, to, ref, style),
	}
}

type Result struct {
	// The dot-generated SVG output. Only contains the <svg> element,
	// <?xml> headers etc are stripped.
	SVG      []byte
	Metadata *dot.SVGGraphMetadata
}

// DetailLevel says how far into the catalog's nesting a view goes. An entity of
// a kind that is not drawn is represented by whatever contains it, so its
// relationships survive at that coarser level.
//
// The ladder spans both views: a domain view offers its lower half (domains,
// systems), a system view its upper half (systems, apis, all).
type DetailLevel int

const (
	// DetailDomains draws domains only.
	DetailDomains DetailLevel = iota
	// DetailSystems adds the systems inside them.
	DetailSystems
	// DetailAPIs adds the APIs systems expose, leaving out what implements them.
	DetailAPIs
	// DetailAll adds components and resources.
	DetailAll
)

// ParseDetailLevel maps the "detail" query parameter to a level, falling back to
// the given default for anything unknown or empty. Each view has its own
// default: one level into the focal entity, since what it is made of is what its
// internal view is for.
func ParseDetailLevel(s string, fallback DetailLevel) DetailLevel {
	switch s {
	case "domains":
		return DetailDomains
	case "systems":
		return DetailSystems
	case "apis":
		return DetailAPIs
	case "all":
		return DetailAll
	default:
		return fallback
	}
}

func (d DetailLevel) String() string {
	switch d {
	case DetailDomains:
		return "domains"
	case DetailSystems:
		return "systems"
	case DetailAPIs:
		return "apis"
	default:
		return "all"
	}
}

// draws reports whether entities of the given kind get a box of their own.
// APIs are the interface layer and survive one level longer than the components
// and resources behind them.
func (d DetailLevel) draws(kind catalog.Kind) bool {
	switch kind {
	case catalog.KindSystem:
		return d >= DetailSystems
	case catalog.KindAPI:
		return d >= DetailAPIs
	default:
		return d >= DetailAll
	}
}

// SystemViewOptions configures a system's external view: which neighbors it
// covers, and how deep it goes into them.
type SystemViewOptions struct {
	// SelectedSystems narrows the view to these neighbors, shown with their
	// parts. When empty, every neighbor is shown, each as a single box.
	SelectedSystems []*catalog.Ref
	// Detail is the level of detail for the focal system and the selected ones.
	Detail DetailLevel
}

// NewSystemViewOptions creates the options for a system's external view.
func NewSystemViewOptions(selectedSystems []*catalog.Ref, detail DetailLevel) *SystemViewOptions {
	return &SystemViewOptions{
		SelectedSystems: selectedSystems,
		Detail:          detail,
	}
}

type DependencyDir int

const (
	DirIncoming DependencyDir = iota
	DirOutgoing
)

// extSysPartDep represents one relationship between the focal system and a
// neighboring one.
//
// source and target are the entities the edge is drawn between. They are the
// system parts involved, except where the detail level leaves a part out, in
// which case they are the system containing it.
type extSysPartDep struct {
	source    catalog.Entity
	target    catalog.Entity
	ref       *catalog.LabelRef
	direction DependencyDir
}

// key identifies the relationship as drawn. Two dependencies that end up
// between the same entities, in the same direction and with the same label are
// one arrow in the diagram.
func (e extSysPartDep) key() string {
	var label string
	if e.ref != nil {
		label = e.ref.Label
	}
	return fmt.Sprintf("%s -> %s / %v / %s",
		e.source.GetRef(), e.target.GetRef(), e.direction, label)
}

// externalGroup collects the relationships with one neighboring container: the
// system a part belongs to in a system view, the domain a system belongs to in
// a domain view.
type externalGroup struct {
	container catalog.Entity
	deps      []extSysPartDep
}

// externalModel is what an "external" view consists of, independent of how it is
// drawn: the focal entity, the parts of it that have relationships reaching
// outside, and the neighbors those relationships lead to.
//
// It describes a system's view of the systems around it, and a domain's view of
// the domains around it, which are the same picture one level apart. Whether a
// neighbor ends up as a box or as a frame with parts inside is left to the
// renderer: it depends on whether the detail level left it anything to show.
type externalModel struct {
	focal catalog.Entity
	// focalParts are the parts of the focal entity that have at least one
	// external relationship, in the order they are drawn.
	focalParts []catalog.Entity
	// groups are the neighbors, in the order they were encountered.
	groups []*externalGroup
}

// collectSystemExternal walks the focal system's parts and gathers everything
// the external view shows. Both the dot-based and the native renderer build on
// it, so that they always show the same content.
func (r *render) collectSystemExternal(system *catalog.System, opts *SystemViewOptions) *externalModel {
	// The selected neighbors are the ones the view covers, each shown with its
	// parts. Selecting nothing selects everything.
	selected := map[string]bool{}
	for _, sel := range opts.SelectedSystems {
		selected[sel.QName()] = true
	}
	allSelected := len(selected) == 0

	m := &externalModel{focal: system}

	// endpoint returns the entity an edge end is drawn as: the part itself, or
	// the system containing it when the detail level leaves that kind of part
	// out. That is what turns a pile of process-level links into a handful of
	// system-level ones.
	endpoint := func(sp catalog.SystemPart) catalog.Entity {
		if opts.Detail.draws(sp.GetKind()) {
			return sp
		}
		if sys := r.repo.System(sp.GetSystem()); sys != nil {
			return sys
		}
		return sp
	}

	extSPDeps := map[string]*externalGroup{}
	// seenDeps drops dependencies that have become indistinguishable: several
	// components of the focal system talking to the same neighbor collapse into
	// one relationship as soon as the components themselves are not drawn.
	seenDeps := map[string]bool{}

	// Adds the src->dst dependency to the group of dst's system.
	// Ignores intra-system dependencies and unselected systems.
	// Returns true if the dependency was added.
	addExtDep := func(src, dst catalog.SystemPart, ref *catalog.LabelRef, dir DependencyDir) bool {
		if dst.GetSystem().Equal(src.GetSystem()) {
			return false
		}
		dstSysRef := dst.GetSystem()
		if !allSelected && !selected[dstSysRef.QName()] {
			// A selection narrows the view to the systems in it.
			return false
		}

		dep := extSysPartDep{
			source: endpoint(src), target: endpoint(dst), ref: ref, direction: dir,
		}
		if seenDeps[dep.key()] {
			return true
		}
		seenDeps[dep.key()] = true

		g, ok := extSPDeps[dstSysRef.QName()]
		if !ok {
			g = &externalGroup{container: r.repo.System(dstSysRef)}
			extSPDeps[dstSysRef.QName()] = g
			m.groups = append(m.groups, g)
		}
		g.deps = append(g.deps, dep)
		return true
	}

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
		// A part the detail level leaves out gets no box; its relationships were
		// already attributed to this system by endpoint().
		if hasEdges && opts.Detail.draws(comp.GetKind()) {
			m.focalParts = append(m.focalParts, comp)
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
		if hasEdges && opts.Detail.draws(ap.GetKind()) {
			m.focalParts = append(m.focalParts, ap)
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
		if hasEdges && opts.Detail.draws(resource.GetKind()) {
			m.focalParts = append(m.focalParts, resource)
		}
	}

	return m
}

func (r *render) generateSystemInternalDotSource(system *catalog.System) *dot.DotSource {
	dw := dot.New(dot.WriterConfig{
		EdgeMinLen: r.config.CompactEdgeMinLen,
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

// SystemExternalGraph generates an SVG for an "external" view of the given
// system: the systems around it and how they relate to it. opts configures
// which of them the view covers and how far into them it goes.
//
// Unlike the other views this one is laid out in process (see internal/sysview)
// rather than by graphviz, which is why it needs no context.
func (r *Renderer) SystemExternalGraph(_ context.Context, system *catalog.System, opts *SystemViewOptions) (*Result, error) {
	rd := &render{Renderer: r, kind: DiagramSystem, focalEntity: system}
	d, meta := rd.buildExternalDiagram(rd.collectSystemExternal(system, opts))
	return &Result{
		SVG:      sysview.Render(d, sysview.DefaultStyle()),
		Metadata: meta,
	}, nil
}

// SystemInternalGraph generates an SVG for an "internal" view of the given system.
// Only entities that are part of the system and their relationships are shown.
func (r *Renderer) SystemInternalGraph(ctx context.Context, system *catalog.System) (*Result, error) {
	rd := &render{Renderer: r, kind: DiagramSystem, focalEntity: system}
	return runDot(ctx, r.runner, rd.generateSystemInternalDotSource(system))
}

// ComponentViewOptions configures which APIs to expand in a component detail view.
type ComponentViewOptions struct {
	// ExpandedAPIs: show consumers for provided APIs and providers for consumed APIs.
	ExpandedAPIs []*catalog.Ref
}

func (r *render) generateComponentDotSource(component *catalog.Component, opts *ComponentViewOptions) *dot.DotSource {
	dw := dot.New(dot.WriterConfig{EdgeMinLen: r.config.NormalEdgeMinLen})
	dw.Start()

	expandedAPIs := map[string]bool{}
	if opts != nil {
		for _, ref := range opts.ExpandedAPIs {
			expandedAPIs[ref.String()] = true
		}
	}

	// Component
	dw.AddNode(r.entityNode(component))

	// "Incoming" dependencies
	// - Owner
	// - System
	// - Parent component
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
	if component.Spec.SubcomponentOf != nil {
		parent := r.repo.Component(component.Spec.SubcomponentOf)
		if parent != nil {
			dw.AddNode(r.entityNode(parent))
			dw.AddEdge(r.entityEdge(parent, component, dot.ESSubcomponent))
		}
	}
	for _, a := range component.Spec.ProvidesAPIs {
		ap := r.repo.API(a.Ref)
		dw.AddNode(r.entityNode(ap))
		dw.AddEdge(r.entityEdgeLabel(ap, component, a, dot.ESProvidedBy))
		if expandedAPIs[a.Ref.String()] {
			for _, c := range ap.GetConsumers() {
				consumer := r.repo.Component(c.Ref)
				if consumer != nil {
					dw.AddNode(r.entityNode(consumer))
					dw.AddEdge(r.entityEdgeLabel(consumer, ap, c, dot.ESNormal))
				}
			}
		}
	}
	for _, d := range component.GetDependents() {
		e := r.repo.Entity(d.Ref)
		if e != nil {
			dw.AddNode(r.entityNode(e))
			dw.AddEdge(r.entityEdgeLabel(e, component, d, dot.ESDependsOn))
		}
	}

	// "Outgoing" dependencies
	// - Consumed APIs
	// - Subcomponents
	// - DependsOn relationships of this entity
	for _, a := range component.Spec.ConsumesAPIs {
		ap := r.repo.API(a.Ref)
		dw.AddNode(r.entityNode(ap))
		dw.AddEdge(r.entityEdgeLabel(component, ap, a, dot.ESNormal))
		if expandedAPIs[a.Ref.String()] {
			for _, p := range ap.GetProviders() {
				provider := r.repo.Component(p.Ref)
				if provider != nil {
					dw.AddNode(r.entityNode(provider))
					dw.AddEdge(r.entityEdgeLabel(ap, provider, p, dot.ESProvidedBy))
				}
			}
		}
	}
	for _, s := range component.GetSubcomponents() {
		sc := r.repo.Component(s)
		dw.AddNode(r.entityNode(sc))
		dw.AddEdge(r.entityEdge(component, sc, dot.ESSubcomponent))
	}
	for _, d := range component.Spec.DependsOn {
		e := r.repo.Entity(d.Ref)
		if e != nil {
			dw.AddNode(r.entityNode(e))
			dw.AddEdge(r.entityEdgeLabel(component, e, d, dot.ESDependsOn))
		}
	}

	dw.End()
	return dw.Result()
}

// ComponentGraph generates an SVG for the given component.
// opts may be nil to use default rendering with no expanded APIs.
func (r *Renderer) ComponentGraph(ctx context.Context, component *catalog.Component, opts *ComponentViewOptions) (*Result, error) {
	rd := &render{Renderer: r, kind: DiagramComponent, focalEntity: component}
	return runDot(ctx, r.runner, rd.generateComponentDotSource(component, opts))
}

func (r *render) generateAPIDotSource(api *catalog.API) *dot.DotSource {
	dw := dot.New(dot.WriterConfig{EdgeMinLen: r.config.NormalEdgeMinLen})
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
			dw.AddNode(r.entityNode(provider))
			dw.AddEdge(r.entityEdgeLabel(api, provider, p, dot.ESProvidedBy))
		}
	}

	// Consumers
	for _, c := range api.GetConsumers() {
		consumer := r.repo.Component(c.Ref)
		if consumer != nil {
			dw.AddNode(r.entityNode(consumer))
			dw.AddEdge(r.entityEdgeLabel(consumer, api, c, dot.ESNormal))
		}
	}

	dw.End()
	return dw.Result()
}

// APIGraph generates an SVG for the given API.
func (r *Renderer) APIGraph(ctx context.Context, api *catalog.API) (*Result, error) {
	rd := &render{Renderer: r, kind: DiagramAPI, focalEntity: api}
	return runDot(ctx, r.runner, rd.generateAPIDotSource(api))
}

func (r *render) generateResourceDotSource(resource *catalog.Resource) *dot.DotSource {
	dw := dot.New(dot.WriterConfig{EdgeMinLen: r.config.NormalEdgeMinLen})
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
	rd := &render{Renderer: r, kind: DiagramResource, focalEntity: resource}
	return runDot(ctx, r.runner, rd.generateResourceDotSource(resource))
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

func (r *render) generateGraphDotSource(entities []catalog.Entity, opts GraphOptions) *dot.DotSource {
	dw := dot.New(dot.WriterConfig{
		EdgeMinLen: r.config.CompactEdgeMinLen,
	})
	dw.Start()

	included := make(map[string]bool)
	for _, e := range entities {
		included[e.GetRef().String()] = true
	}

	// clusters tracks System entities that are rendered as dot clusters
	// rather than as nodes. Edges involving clustered systems are skipped, since
	// containment is conveyed visually by the cluster boundary instead.
	type clusterGroup struct {
		sysRef   *catalog.Ref
		children []catalog.Entity
	}
	sysClusters := make(map[string]*clusterGroup)

	if opts.SystemsAsClusters {
		// Group entities by their system cluster.
		// - *catalog.System entities register a (possibly empty) cluster for themselves.
		// - SystemPart entities (Component, API, Resource) are collected as children
		//   of their system's cluster.
		// - Everything else is treated as a standalone node.

		ensureCluster := func(sysRef *catalog.Ref) *clusterGroup {
			key := sysRef.String()
			if g, ok := sysClusters[key]; ok {
				return g
			}
			g := &clusterGroup{sysRef: sysRef}
			sysClusters[key] = g
			return g
		}

		for _, e := range entities {
			if v, ok := e.(*catalog.System); ok {
				ensureCluster(v.GetRef())
			} else if sp, ok := e.(catalog.SystemPart); ok {
				sysRef := sp.GetSystem()
				ensureCluster(sysRef).children = append(ensureCluster(sysRef).children, e)
			} else {
				// standalone entity
				dw.AddNode(r.entityNode(e))
			}
		}

		for _, g := range sysClusters {
			sys := r.repo.System(g.sysRef)
			dw.StartCluster(sys.GetRef().QName())
			for _, child := range g.children {
				dw.AddNode(r.entityNode(child))
			}
			dw.EndCluster()
		}
	} else {
		for _, e := range entities {
			dw.AddNode(r.entityNode(e))
		}
	}

	for _, e := range entities {
		// Clustered systems are rendered as dot subgraphs, not nodes; skip all edges.
		if _, ok := sysClusters[e.GetRef().String()]; ok {
			continue
		}

		// Owner edge (all entity kinds).
		if ownerRef := e.GetOwner(); ownerRef != nil && included[ownerRef.String()] {
			if owner := r.repo.Group(ownerRef); owner != nil {
				dw.AddEdge(r.entityEdge(owner, e, dot.ESOwner))
			}
		}

		switch v := e.(type) {
		case *catalog.System:
			if domRef := v.Spec.Domain; domRef != nil && included[domRef.String()] {
				if dom := r.repo.Domain(domRef); dom != nil {
					dw.AddEdge(r.entityEdge(dom, v, dot.ESContains))
				}
			}

		case *catalog.Component:
			if sysRef := v.GetSystem(); included[sysRef.String()] && sysClusters[sysRef.String()] == nil {
				dw.AddEdge(r.entityEdge(r.repo.System(sysRef), v, dot.ESContains))
			}
			if v.Spec.SubcomponentOf != nil && included[v.Spec.SubcomponentOf.String()] {
				if parent := r.repo.Component(v.Spec.SubcomponentOf); parent != nil {
					dw.AddEdge(r.entityEdge(parent, v, dot.ESSubcomponent))
				}
			}
			for _, ref := range v.Spec.ProvidesAPIs {
				if included[ref.Ref.String()] {
					if ap := r.repo.API(ref.Ref); ap != nil {
						dw.AddEdge(r.entityEdgeLabel(ap, v, ref, dot.ESProvidedBy))
					}
				}
			}
			for _, ref := range v.Spec.ConsumesAPIs {
				if included[ref.Ref.String()] {
					if ap := r.repo.API(ref.Ref); ap != nil {
						dw.AddEdge(r.entityEdgeLabel(v, ap, ref, dot.ESNormal))
					}
				}
			}
			for _, ref := range v.Spec.DependsOn {
				if included[ref.Ref.String()] {
					if target := r.repo.Entity(ref.Ref); target != nil {
						dw.AddEdge(r.entityEdgeLabel(v, target, ref, dot.ESDependsOn))
					}
				}
			}

		case *catalog.API:
			if sysRef := v.GetSystem(); included[sysRef.String()] && sysClusters[sysRef.String()] == nil {
				dw.AddEdge(r.entityEdge(r.repo.System(sysRef), v, dot.ESContains))
			}

		case *catalog.Resource:
			if sysRef := v.GetSystem(); included[sysRef.String()] && sysClusters[sysRef.String()] == nil {
				dw.AddEdge(r.entityEdge(r.repo.System(sysRef), v, dot.ESContains))
			}
			for _, ref := range v.Spec.DependsOn {
				if included[ref.Ref.String()] {
					if target := r.repo.Entity(ref.Ref); target != nil {
						dw.AddEdge(r.entityEdgeLabel(v, target, ref, dot.ESDependsOn))
					}
				}
			}
		}
	}

	dw.End()
	return dw.Result()
}

type GraphOptions struct {
	// If true, draws System entities as dot clusters instead of as simple nodes.
	SystemsAsClusters bool
}

// Graph generates an SVG for the given list of entities.
func (r *Renderer) Graph(ctx context.Context, entities []catalog.Entity, opts GraphOptions) (*Result, error) {
	rd := &render{Renderer: r, kind: DiagramAdHoc}
	return runDot(ctx, r.runner, rd.generateGraphDotSource(entities, opts))
}
