package svg

import (
	"fmt"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/sysview"
)

// buildSystemExternalDiagram translates a collected external view into the
// input of the native renderer, together with the sidecar metadata the frontend
// needs. Ids follow the same conventions as the dot output ("svg-cluster-N",
// "svg-edge-N", entity refs for nodes), so both renderers are interchangeable
// from the browser's point of view.
func (r *render) buildSystemExternalDiagram(m *systemExternalModel) (*sysview.Diagram, *dot.SVGGraphMetadata) {
	b := &diagramBuilder{
		render: r,
		meta: &dot.SVGGraphMetadata{
			Nodes:    map[string]*dot.NodeInfo{},
			Edges:    map[string]*dot.EdgeInfo{},
			Clusters: map[string]*dot.ClusterInfo{},
		},
		d: &sysview.Diagram{},
	}

	// The focal system.
	b.focalRef = m.system.GetRef().String()
	b.d.Focal = &sysview.Group{
		ID:    b.clusterID(m.system.GetRef().QName()),
		Label: m.system.GetRef().QName(),
	}
	for _, e := range m.focalParts {
		b.d.Focal.Nodes = append(b.d.Focal.Nodes, b.node(e))
	}
	if len(b.d.Focal.Nodes) == 0 {
		// Nothing to frame: at this detail level the focal system is a box like
		// every other system in the picture, so it is painted like one.
		layout := b.nodeLayout(m.system)
		b.d.Focal.Fill, b.d.Focal.Border = layout.FillColor, layout.BorderColor
	}

	// Neighboring systems: a frame with parts inside, or a single box when the
	// detail level left them nothing to show.
	for _, g := range m.groups {
		item := &sysview.Item{}
		class := ""
		if groupHasParts(g) {
			item.Group = &sysview.Group{
				ID:    b.clusterID(g.sysRef.QName()),
				Label: g.sysRef.QName(),
			}
		} else {
			item.Node = b.node(b.repo.System(g.sysRef))
			class = "system-link-edge"
		}
		b.d.Externals = append(b.d.Externals, item)
		seen := map[string]bool{}
		for _, dep := range g.deps {
			if key := dep.target.GetRef().String(); !seen[key] && item.Group != nil {
				seen[key] = true
				b.anchor(item.Group, dep.target)
			}
			if dep.direction == DirOutgoing {
				b.edge(dep.source, dep.target, dep.ref, class)
			} else {
				b.edge(dep.target, dep.source, dep.ref, class)
			}
		}
	}

	return b.d, b.meta
}

// groupHasParts reports whether an expanded system still has anything to show
// inside its frame. It does not once all of its relationships are with the
// system as a whole, which is what hiding components does to a system whose
// only involvement is through them.
func groupHasParts(g *externalSystemGroup) bool {
	for _, dep := range g.deps {
		if _, isSystem := dep.target.(*catalog.System); !isSystem {
			return true
		}
	}
	return false
}

// focalAnchor gives the focal system a frame if this edge end is the system
// itself rather than one of its parts.
func (b *diagramBuilder) focalAnchor(e catalog.Entity) {
	sys, isSystem := e.(*catalog.System)
	if !isSystem || b.d.Focal == nil || b.d.Focal.Frame != nil {
		return
	}
	if sys.GetRef().String() == b.focalRef {
		b.d.Focal.Frame = &sysview.Node{ID: b.focalRef}
	}
}

// anchor gives the group whatever the edge end e needs to attach to: a box for
// a part, or the group's frame when the end is the system itself — which is how
// a hidden component's dependencies are drawn.
func (b *diagramBuilder) anchor(group *sysview.Group, e catalog.Entity) {
	sys, isSystem := e.(*catalog.System)
	if !isSystem {
		group.Nodes = append(group.Nodes, b.node(e))
		return
	}
	if group.Frame == nil {
		group.Frame = &sysview.Node{ID: sys.GetRef().String()}
	}
}

type diagramBuilder struct {
	*render
	meta *dot.SVGGraphMetadata
	d    *sysview.Diagram
	// focalRef is the ref of the system the view is about, used to recognize
	// edge ends that are the focal system itself rather than one of its parts.
	focalRef string
}

func (b *diagramBuilder) clusterID(label string) string {
	id := fmt.Sprintf("svg-cluster-%d", len(b.meta.Clusters))
	b.meta.Clusters[id] = &dot.ClusterInfo{Label: label}
	return id
}

// node converts an entity into a renderable box, reusing the styling rules that
// also drive the dot output (colors, shapes, label lines, tooltips).
func (b *diagramBuilder) node(e catalog.Entity) *sysview.Node {
	layout := b.nodeLayout(e)
	id := e.GetRef().String()

	n := &sysview.Node{
		ID:     id,
		Fill:   layout.FillColor,
		Border: layout.BorderColor,
		Shape:  sysview.ShapeRounded,
	}
	if layout.Shape == dot.NSBox {
		n.Shape = sysview.ShapeRect
	}
	texts := make([]string, 0, len(layout.Labels))
	for _, l := range layout.Labels {
		n.Labels = append(n.Labels, sysview.Label{
			Text:  l.Text,
			Style: labelStyle(l.Style),
			Color: l.Color,
		})
		texts = append(texts, l.Text)
	}

	b.meta.Nodes[id] = &dot.NodeInfo{
		Label:        strings.Join(texts, "\n"),
		Title:        layout.TooltipTitle,
		TooltipAttrs: layout.TooltipAttrs,
	}
	return n
}

// edge adds an edge from src to dst. ref may be nil for unlabelled links
// between the focal system and a collapsed neighbor.
func (b *diagramBuilder) edge(src, dst catalog.Entity, ref *catalog.LabelRef, class string) {
	b.focalAnchor(src)
	b.focalAnchor(dst)

	id := fmt.Sprintf("svg-edge-%d", len(b.meta.Edges))
	e := &sysview.Edge{
		ID:    id,
		From:  src.GetRef().String(),
		To:    dst.GetRef().String(),
		Class: class,
	}
	// The title makes every edge hoverable, labelled or not; a labelled one adds
	// its label and attributes below it.
	info := &dot.EdgeInfo{
		From:  e.From,
		To:    e.To,
		Title: src.GetQName() + " → " + dst.GetQName(),
	}
	if ref != nil {
		layout := b.edgeLabelLayout(src, dst, ref, dot.ESNormal)
		e.Label = layout.Label
		info.Label = layout.Label
		info.Title = layout.TooltipTitle
		info.TooltipAttrs = layout.TooltipAttrs
	}
	b.meta.Edges[id] = info
	b.d.Edges = append(b.d.Edges, e)
}

func labelStyle(s dot.LabelStyle) sysview.LabelStyle {
	var out sysview.LabelStyle
	if s.Em() {
		out |= sysview.StyleEm
	}
	if s.Small() {
		out |= sysview.StyleSmall
	}
	if s.Light() {
		out |= sysview.StyleLight
	}
	return out
}
