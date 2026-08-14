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
	b.d.Focal = &sysview.Group{
		ID:    b.clusterID(m.system.GetRef().QName()),
		Label: m.system.GetRef().QName(),
	}
	for _, e := range m.focalParts {
		b.d.Focal.Nodes = append(b.d.Focal.Nodes, b.node(e))
	}

	// Neighboring systems shown as a single box.
	collapsed := map[string]*sysview.Item{}
	for _, dep := range m.extDeps {
		key := dep.targetSystem.GetRef().String()
		if _, ok := collapsed[key]; !ok {
			it := &sysview.Item{Node: b.node(dep.targetSystem)}
			collapsed[key] = it
			b.d.Externals = append(b.d.Externals, it)
		}
		if dep.direction == DirOutgoing {
			b.edge(dep.source, dep.targetSystem, nil, "system-link-edge")
		} else {
			b.edge(dep.targetSystem, dep.source, nil, "system-link-edge")
		}
	}

	// Neighboring systems shown with their parts.
	for _, g := range m.groups {
		group := &sysview.Group{
			ID:    b.clusterID(g.sysRef.QName()),
			Label: g.sysRef.QName(),
		}
		b.d.Externals = append(b.d.Externals, &sysview.Item{Group: group})
		seen := map[string]bool{}
		for _, dep := range g.deps {
			if key := dep.target.GetRef().String(); !seen[key] {
				seen[key] = true
				group.Nodes = append(group.Nodes, b.node(dep.target))
			}
			if dep.direction == DirOutgoing {
				b.edge(dep.source, dep.target, dep.ref, "")
			} else {
				b.edge(dep.target, dep.source, dep.ref, "")
			}
		}
	}

	return b.d, b.meta
}

type diagramBuilder struct {
	*render
	meta *dot.SVGGraphMetadata
	d    *sysview.Diagram
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
	id := fmt.Sprintf("svg-edge-%d", len(b.meta.Edges))
	e := &sysview.Edge{
		ID:    id,
		From:  src.GetRef().String(),
		To:    dst.GetRef().String(),
		Class: class,
	}
	info := &dot.EdgeInfo{From: e.From, To: e.To}
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
