package svg

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
	"github.com/dnswlt/swcat/internal/sysview"
)

// buildExternalDiagram translates a collected external view into the input of
// the native renderer, together with the sidecar metadata the frontend needs. It
// serves both the system and the domain view: the two differ in what the boxes
// are, not in how they are arranged.
//
// Ids follow the same conventions as the dot output ("svg-cluster-N",
// "svg-edge-N", entity refs for nodes), so both renderers are interchangeable
// from the browser's point of view.
func (r *render) buildExternalDiagram(m *externalModel) (*sysview.Diagram, *dot.SVGGraphMetadata) {
	b := &diagramBuilder{
		render: r,
		meta: &dot.SVGGraphMetadata{
			Nodes:    map[string]*dot.NodeInfo{},
			Edges:    map[string]*dot.EdgeInfo{},
			Clusters: map[string]*dot.ClusterInfo{},
		},
		d: &sysview.Diagram{},
	}

	// The focal entity.
	b.focalRef = m.focal.GetRef().String()
	b.d.Focal = &sysview.Group{
		ID:    b.clusterID(m.focal.GetRef().QName()),
		Label: m.focal.GetRef().QName(),
	}
	for _, e := range m.focalParts {
		b.d.Focal.Nodes = append(b.d.Focal.Nodes, b.node(e))
	}
	if len(b.d.Focal.Nodes) == 0 {
		// Nothing to frame: at this detail level the focal system is a box like
		// every other system in the picture, so it is painted like one.
		layout := b.nodeLayout(m.focal)
		b.d.Focal.Fill, b.d.Focal.Border = layout.FillColor, layout.BorderColor
	}

	// Neighbors: a frame with parts inside, or a single box when the detail
	// level left them nothing to show.
	for _, g := range m.groups {
		item := &sysview.Item{}
		class := ""
		if groupHasParts(g) {
			item.Group = &sysview.Group{
				ID:    b.clusterID(g.container.GetRef().QName()),
				Label: g.container.GetRef().QName(),
			}
		} else {
			item.Node = b.node(g.container)
			class = "system-link-edge"
		}
		b.d.Externals = append(b.d.Externals, item)
		seen := map[string]bool{}
		for _, dep := range g.deps {
			if key := dep.target.GetRef().String(); !seen[key] && item.Group != nil {
				seen[key] = true
				b.anchor(item.Group, g.container, dep.target)
			}
			if dep.direction == DirOutgoing {
				b.edge(dep.source, dep.target, dep, class)
			} else {
				b.edge(dep.target, dep.source, dep, class)
			}
		}
	}

	return b.d, b.meta
}

// groupHasParts reports whether a neighbor still has anything to show inside a
// frame. It does not once all of its relationships are with the neighbor as a
// whole, which is what happens when the detail level leaves its parts out.
func groupHasParts(g *externalGroup) bool {
	for _, dep := range g.deps {
		if dep.target.GetRef().String() != g.container.GetRef().String() {
			return true
		}
	}
	return false
}

// focalAnchor gives the focal entity a frame if this edge end is that entity
// itself rather than one of its parts.
func (b *diagramBuilder) focalAnchor(e catalog.Entity) {
	if b.d.Focal == nil || b.d.Focal.Frame != nil {
		return
	}
	if e.GetRef().String() == b.focalRef {
		b.d.Focal.Frame = &sysview.Node{ID: b.focalRef}
	}
}

// anchor gives the group whatever the edge end e needs to attach to: a box for
// a part, or the group's frame when the end is the neighbor itself — which is
// how the relationships of parts that are not drawn end up being shown.
func (b *diagramBuilder) anchor(group *sysview.Group, container, e catalog.Entity) {
	if e.GetRef().String() != container.GetRef().String() {
		group.Nodes = append(group.Nodes, b.node(e))
		return
	}
	if group.Frame == nil {
		group.Frame = &sysview.Node{ID: container.GetRef().String()}
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

// edge adds the arrow for dep, drawn from src to dst.
func (b *diagramBuilder) edge(src, dst catalog.Entity, dep *extSysPartDep, class string) {
	b.focalAnchor(src)
	b.focalAnchor(dst)

	id := fmt.Sprintf("svg-edge-%d", len(b.meta.Edges))
	e := &sysview.Edge{
		ID:    id,
		From:  src.GetRef().String(),
		To:    dst.GetRef().String(),
		Class: class,
	}
	// The title makes every arrow hoverable, whether or not it says more.
	info := &dot.EdgeInfo{
		From:  e.From,
		To:    e.To,
		Title: src.GetQName() + " → " + dst.GetQName(),
	}

	// A label describes a relationship between two particular entities, so it
	// belongs on the arrow only while those entities are the ones drawn.
	if rel, ok := dep.singleDrawnRel(); ok && rel.ref != nil {
		layout := b.edgeLabelLayout(src, dst, rel.ref, dot.ESNormal)
		e.Label = layout.Label
		info.Label = layout.Label
		info.Title = layout.TooltipTitle
		info.TooltipAttrs = layout.TooltipAttrs
	} else {
		// The arrow stands for relationships between entities that are not
		// drawn, so it reports how many of them it covers.
		info.TooltipAttrs = []dot.TooltipAttr{
			{Key: "relationships", Value: strconv.Itoa(len(dep.rels))},
		}
	}

	b.meta.Edges[id] = info
	b.d.Edges = append(b.d.Edges, e)
}

// singleDrawnRel returns the relationship this arrow stands for, if it stands
// for exactly one and that one is between the very entities the arrow is drawn
// between.
func (d *extSysPartDep) singleDrawnRel() (relationship, bool) {
	if len(d.rels) != 1 {
		return relationship{}, false
	}
	rel := d.rels[0]
	if !rel.src.GetRef().Equal(d.source.GetRef()) || !rel.dst.GetRef().Equal(d.target.GetRef()) {
		return relationship{}, false
	}
	return rel, true
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
