package svg

import (
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
)

type Layouter interface {
	Node(e catalog.Entity) dot.NodeLayout
	Edge(src, dst catalog.Entity, style dot.EdgeStyle) dot.EdgeLayout
	EdgeLabel(src, dst catalog.Entity, ref *catalog.LabelRef, style dot.EdgeStyle) dot.EdgeLayout
}

type StandardLayouter struct {
	config Config
}

func NewStandardLayouter(config Config) *StandardLayouter {
	return &StandardLayouter{
		config: config,
	}
}

func (l *StandardLayouter) fillColor(e catalog.Entity) string {
	if c, ok := e.GetMetadata().Annotations[catalog.AnnotFillColor]; ok {
		// explicit annotation overrides everything
		return c
	}

	// Next priority: check colors per label.
	if len(l.config.NodeColors.Labels) > 0 {
		for k, v := range e.GetMetadata().Labels {
			if m, ok := l.config.NodeColors.Labels[k]; ok && m != nil {
				if val, ok := m[v]; ok {
					return string(val)
				}
			}
		}
	}

	// Check colors per type.
	if len(l.config.NodeColors.Types) > 0 {
		typ := e.GetType()
		if col, ok := l.config.NodeColors.Types[typ]; ok {
			return string(col)
		}
	}

	// Default colors
	switch e.GetKind() {
	case catalog.KindComponent:
		return "#D2E5EF"
		// return "#CBDCEB"
	case catalog.KindSystem:
		return "#6BABD0"
	case catalog.KindAPI:
		return "#FCE0BA"
		// return "#FADA7A"
	case catalog.KindResource:
		return "#B4DEBD"
	case catalog.KindGroup:
		// return "#F5EEDC"
		return "#F2F0EB"
	}
	return "#F5EEDC" // neutral beige
}

func (l *StandardLayouter) shape(e catalog.Entity) dot.NodeShape {
	switch e.GetKind() {
	case catalog.KindSystem:
		return dot.NSBox
	case catalog.KindGroup:
		return dot.NSEllipse
	}
	return dot.NSRoundedBox
}

func (l *StandardLayouter) stereotype(e catalog.Entity) (string, bool) {
	meta := e.GetMetadata()

	if st, ok := meta.Annotations[catalog.AnnotSterotype]; ok {
		// Explicit stereotype annotation overrides everything
		return st, true
	}

	if len(l.config.StereotypeLabels) == 0 {
		return "", false
	}

	var values []string
	for _, lbl := range l.config.StereotypeLabels {
		if val, ok := meta.Labels[lbl]; ok {
			values = append(values, val)
		}
	}
	if len(values) == 0 {
		return "", false
	}
	return strings.Join(values, ", "), true
}

func (l *StandardLayouter) label(e catalog.Entity) string {
	var label strings.Builder

	if st, ok := l.stereotype(e); ok {
		label.WriteString(PrefixNodeLabelEm + `&laquo;` + st + `&raquo;\n`)
	}

	meta := e.GetMetadata()
	if meta.Namespace != "" && meta.Namespace != catalog.DefaultNamespace {
		// Two-line label for namespaced entities.
		label.WriteString(meta.Namespace + `/\n`)
	}
	label.WriteString(meta.Name)

	if a, ok := e.(*catalog.API); ok && l.config.ShowAPIProvider {
		providers := a.GetProviders()
		if len(providers) > 0 {
			label.WriteString(`\n` + PrefixNodeLabelSmall + " " + providers[0].QName())
		}
	}

	return label.String()
}

func (l *StandardLayouter) Node(e catalog.Entity) dot.NodeLayout {

	return dot.NodeLayout{
		Label:     l.label(e),
		FillColor: l.fillColor(e),
		Shape:     l.shape(e),
	}
}

func (l *StandardLayouter) Edge(src, dst catalog.Entity, style dot.EdgeStyle) dot.EdgeLayout {
	return dot.EdgeLayout{
		Style: style,
	}
}

func (l *StandardLayouter) EdgeLabel(src, dst catalog.Entity, ref *catalog.LabelRef, style dot.EdgeStyle) dot.EdgeLayout {
	return dot.EdgeLayout{
		Label: ref.Label,
		Style: style,
	}
}

// Interface implementation assertion
var _ Layouter = (*StandardLayouter)(nil)
