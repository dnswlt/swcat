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

type LayouterConfig struct {
	// Labels that should be displayed as stereotypes in nodes.
	StereotypeLabels []string
	// Maps label keys and label values to node colors.
	LabelColorMap map[string]map[string]string
}

type StandardLayouter struct {
	config LayouterConfig
}

func NewStandardLayouter(config LayouterConfig) *StandardLayouter {
	return &StandardLayouter{
		config: config,
	}
}

func (l *StandardLayouter) fillColor(e catalog.Entity) string {
	if len(l.config.LabelColorMap) > 0 {
		for k, v := range e.GetMetadata().Labels {
			if m, ok := l.config.LabelColorMap[k]; ok && m != nil {
				if val, ok := m[v]; ok {
					return val
				}
			}
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
	if len(l.config.StereotypeLabels) == 0 {
		return "", false
	}
	meta := e.GetMetadata()

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

func (l *StandardLayouter) Node(e catalog.Entity) dot.NodeLayout {
	meta := e.GetMetadata()
	// Label
	var label strings.Builder
	if st, ok := meta.Annotations[AnnotSterotype]; ok {
		// Explicit stereotype annotation
		label.WriteString(`&laquo;` + st + `&raquo;\n`)
	} else if st, ok := l.stereotype(e); ok {
		label.WriteString(`&laquo;` + st + `&raquo;\n`)
	}
	if meta.Namespace != "" && meta.Namespace != catalog.DefaultNamespace {
		// Two-line label for namespaced entities.
		label.WriteString(meta.Namespace + `/\n`)
	}
	label.WriteString(meta.Name)

	var fillColor string
	if c, ok := meta.Annotations[AnnotFillColor]; ok {
		// explicit annotation overrides everything
		fillColor = c
	} else {
		fillColor = l.fillColor(e)
	}

	return dot.NodeLayout{
		Label:     label.String(),
		FillColor: fillColor,
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
