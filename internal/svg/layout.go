package svg

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/dot"
)

type NodeOptions struct {
	IncludeSystemPart bool
}

type Layouter interface {
	Node(e catalog.Entity) dot.NodeLayout
	// NodeContext lays out the given entity e as a node in the context of contextEntity.
	// The context is used to determine how to lay out e. For example, a component
	// might include the containing system's name in its label only if it is rendered
	// in a context of an entity belonging to a different system.
	NodeContext(e, contextEntity catalog.Entity) dot.NodeLayout
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

func sameSystem(e1, e2 catalog.Entity) bool {
	sp1, ok1 := e1.(catalog.SystemPart)
	if !ok1 {
		return false
	}
	sp2, ok2 := e2.(catalog.SystemPart)
	if !ok2 {
		return false
	}
	return sp1.GetSystem().Equal(sp2.GetSystem())
}

func (l *StandardLayouter) label(e, contextEntity catalog.Entity) string {
	var label strings.Builder

	// <<Stereotypes>>
	if st, ok := l.stereotype(e); ok {
		label.WriteString(PrefixNodeLabelEm + `&laquo;` + st + `&raquo;\n`)
	}

	// Parent system, if applicable (e is not a system itself, and the contextEntity is from another system).
	systemName := func() (string, bool) {
		sp, isSystemPart := e.(catalog.SystemPart)
		if !isSystemPart {
			return "", false
		}
		if !l.config.ShowParentSystem || e.GetKind() == catalog.KindSystem {
			return "", false
		}
		if contextEntity == nil || sameSystem(e, contextEntity) {
			return "", false
		}
		return sp.GetSystem().QName(), true
	}
	sysName, addSystemName := systemName()
	if addSystemName {
		label.WriteString(PrefixNodeLabelSmall + sysName + `\n`)
	}

	// Qualified entity name
	meta := e.GetMetadata()
	if meta.Namespace != "" && meta.Namespace != catalog.DefaultNamespace {
		// Wrap line between namespace and name if label gets too long.
		label.WriteString(meta.Namespace + "/")
		if len(meta.Namespace+"/"+meta.Name) > 30 {
			label.WriteString(`\n`)
		}
	}
	label.WriteString(meta.Name)

	// API provider component, if applicable and if parent system isn't already shown (to avoid visual overload).
	if a, ok := e.(*catalog.API); ok && l.config.ShowAPIProvider && !addSystemName {
		providers := a.GetProviders()
		if l := len(providers); l > 0 {
			label.WriteString(`\n` + PrefixNodeLabelSmall + providers[0].QName())
			if l > 1 {
				fmt.Fprintf(&label, "+%d", l-1)
			}
		}
	}

	return label.String()
}

func (l *StandardLayouter) Node(e catalog.Entity) dot.NodeLayout {
	return l.NodeContext(e, nil)
}

func (l *StandardLayouter) NodeContext(e, contextEntity catalog.Entity) dot.NodeLayout {

	return dot.NodeLayout{
		Label:     l.label(e, contextEntity),
		FillColor: l.fillColor(e),
		Shape:     l.shape(e),
	}
}

func (l *StandardLayouter) Edge(src, dst catalog.Entity, style dot.EdgeStyle) dot.EdgeLayout {
	return dot.EdgeLayout{
		Style: style,
	}
}

// EdgeLabel generates a dot.EdgeLayout with an edge label.
// The label is built from the ref's label and attributes:
//
// The full format is
// <version> · <label>
//
//	<key1>: <value1>
//	...
func (l *StandardLayouter) EdgeLabel(src, dst catalog.Entity, ref *catalog.LabelRef, style dot.EdgeStyle) dot.EdgeLayout {
	var label strings.Builder
	// Version
	if version, ok := ref.GetAttr(catalog.VersionAttrKey); ok && l.config.ShowVersionAsLabel {
		label.WriteString(version)
	}
	// Label
	if ref.Label != "" {
		if label.Len() > 0 {
			label.WriteString(" · ")
		}
		label.WriteString(ref.Label)
	}
	// Attrs, sorted alphabetically
	keys := make([]string, 0, len(ref.Attrs))
	for k := range ref.Attrs {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if k == catalog.VersionAttrKey {
			continue // version has already been added
		}
		if label.Len() > 0 {
			label.WriteString("\\n")
		}
		label.WriteString(fmt.Sprintf("%s: %s", k, ref.Attrs[k]))
	}
	return dot.EdgeLayout{
		Label: label.String(),
		Style: style,
	}
}

// Interface implementation assertion
var _ Layouter = (*StandardLayouter)(nil)
