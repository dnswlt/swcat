package svg

import (
	"context"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/sysview"
)

// DomainViewOptions configures a domain's external view, mirroring
// SystemViewOptions one level up: which neighboring domains it covers, and
// whether their systems are drawn individually.
type DomainViewOptions struct {
	// SelectedDomains narrows the view to these neighbors. When empty, every
	// neighbor is covered.
	SelectedDomains []*catalog.Ref
	// Detail is DetailSystems to draw the neighbors' systems, DetailDomains to
	// represent each neighbor by its domain alone.
	Detail DetailLevel
}

func NewDomainViewOptions(selectedDomains []*catalog.Ref, detail DetailLevel) *DomainViewOptions {
	return &DomainViewOptions{
		SelectedDomains: selectedDomains,
		Detail:          detail,
	}
}

// systemNeighbor is a system another system exchanges data with.
type systemNeighbor struct {
	system    *catalog.System
	direction DependencyDir
}

// systemNeighbors returns the systems that the given system exchanges data
// with. Relationships are recorded on parts, so this walks the system's
// components, APIs and resources and lifts what it finds to the level of
// systems. The result is sorted, so that a diagram comes out the same on every
// render.
func (r *render) systemNeighbors(s *catalog.System) []systemNeighbor {
	out := map[*catalog.System]DependencyDir{}
	add := func(ref *catalog.Ref, dir DependencyDir) {
		sp, ok := r.repo.Entity(ref).(catalog.SystemPart)
		if !ok {
			return
		}
		other := r.repo.System(sp.GetSystem())
		if other == nil || other.GetRef().Equal(s.GetRef()) {
			return
		}
		// A pair related in both directions is recorded once, as outgoing: the
		// layout picks a side for it anyway.
		if prev, seen := out[other]; !seen || prev == dir {
			out[other] = dir
		} else {
			out[other] = DirOutgoing
		}
	}
	for _, cRef := range s.GetComponents() {
		c := r.repo.Component(cRef)
		for _, ref := range c.Spec.DependsOn {
			add(ref.Ref, DirOutgoing)
		}
		for _, ref := range c.Spec.ConsumesAPIs {
			add(ref.Ref, DirOutgoing)
		}
		for _, ref := range c.GetDependents() {
			add(ref.Ref, DirIncoming)
		}
	}
	for _, aRef := range s.GetAPIs() {
		for _, ref := range r.repo.API(aRef).GetConsumers() {
			add(ref.Ref, DirIncoming)
		}
	}
	for _, rRef := range s.GetResources() {
		res := r.repo.Resource(rRef)
		for _, ref := range res.Spec.DependsOn {
			add(ref.Ref, DirOutgoing)
		}
		for _, ref := range res.GetDependents() {
			add(ref.Ref, DirIncoming)
		}
	}

	neighbors := make([]systemNeighbor, 0, len(out))
	for sys, dir := range out {
		neighbors = append(neighbors, systemNeighbor{system: sys, direction: dir})
	}
	slices.SortFunc(neighbors, func(a, b systemNeighbor) int {
		return strings.Compare(a.system.GetQName(), b.system.GetQName())
	})
	return neighbors
}

// collectDomainExternal gathers how a domain relates to the rest of the
// catalog: its own systems that reach outside it, and the neighbors they reach.
// Relationships within the domain belong to its internal view and are left out.
func (r *render) collectDomainExternal(domain *catalog.Domain, opts *DomainViewOptions) *externalModel {
	selected := map[string]bool{}
	for _, sel := range opts.SelectedDomains {
		selected[sel.QName()] = true
	}
	allSelected := len(selected) == 0

	own := map[string]bool{}
	for _, sRef := range domain.GetSystems() {
		own[sRef.String()] = true
	}

	m := &externalModel{focal: domain}
	groups := map[string]*externalGroup{}
	seenDeps := map[string]bool{}

	for _, sRef := range domain.GetSystems() {
		s := r.repo.System(sRef)
		if s == nil {
			continue
		}
		hasEdges := false
		for _, n := range r.systemNeighbors(s) {
			other, dir := n.system, n.direction
			if own[other.GetRef().String()] {
				continue // internal to the domain
			}
			// The container is the neighbor's domain when it has one; a system
			// without a domain stands for itself. It gets no chip either — the
			// chips list domains — so it shows up in the unnarrowed view only.
			container := catalog.Entity(other)
			if domRef := other.GetDomain(); domRef != nil {
				if d := r.repo.Domain(domRef); d != nil {
					container = d
				}
			}
			if !allSelected && !selected[container.GetRef().QName()] {
				continue
			}
			hasEdges = true

			// At the domains level the neighbor is represented by its domain.
			target := catalog.Entity(other)
			if !opts.Detail.draws(catalog.KindSystem) {
				target = container
			}
			dep := extSysPartDep{source: s, target: target, direction: dir}
			if seenDeps[dep.key()] {
				continue
			}
			seenDeps[dep.key()] = true

			key := container.GetRef().String()
			g, ok := groups[key]
			if !ok {
				g = &externalGroup{container: container}
				groups[key] = g
				m.groups = append(m.groups, g)
			}
			g.deps = append(g.deps, dep)
		}
		if hasEdges {
			m.focalParts = append(m.focalParts, s)
		}
	}
	return m
}

// DomainExternalGraph generates an SVG showing how a domain relates to the
// domains around it. Like the system external view, it is laid out in process.
func (r *Renderer) DomainExternalGraph(_ context.Context, domain *catalog.Domain, opts *DomainViewOptions) (*Result, error) {
	rd := &render{Renderer: r, kind: DiagramDomain, focalEntity: domain}
	d, meta := rd.buildExternalDiagram(rd.collectDomainExternal(domain, opts))
	return &Result{
		SVG:      sysview.Render(d, sysview.DefaultStyle()),
		Metadata: meta,
	}, nil
}

// generateDomainInternalDotSource draws the domain's own systems and how they
// relate to each other. That is a general graph rather than two columns of
// neighbors, so it stays with graphviz.
func (r *render) generateDomainInternalDotSource(domain *catalog.Domain) *dot.DotSource {
	dw := dot.New(dot.WriterConfig{EdgeMinLen: r.config.CompactEdgeMinLen})
	dw.Start()

	own := map[string]bool{}
	dw.StartCluster(domain.GetQName())
	for _, sRef := range domain.GetSystems() {
		if s := r.repo.System(sRef); s != nil {
			own[sRef.String()] = true
			dw.AddNode(r.entityNode(s))
		}
	}
	dw.EndCluster()

	seen := map[string]bool{}
	for _, sRef := range domain.GetSystems() {
		s := r.repo.System(sRef)
		if s == nil {
			continue
		}
		for _, n := range r.systemNeighbors(s) {
			other, dir := n.system, n.direction
			if !own[other.GetRef().String()] {
				continue // belongs to the external view
			}
			src, dst := s, other
			if dir == DirIncoming {
				src, dst = other, s
			}
			key := src.GetRef().String() + " -> " + dst.GetRef().String()
			if seen[key] {
				continue
			}
			seen[key] = true
			dw.AddEdge(r.entityEdge(src, dst, dot.ESNormal))
		}
	}

	dw.End()
	return dw.Result()
}

// DomainInternalGraph generates an SVG of the domain's systems and their
// relationships among each other.
func (r *Renderer) DomainInternalGraph(ctx context.Context, domain *catalog.Domain) (*Result, error) {
	rd := &render{Renderer: r, kind: DiagramDomain, focalEntity: domain}
	return runDot(ctx, r.runner, rd.generateDomainInternalDotSource(domain))
}
