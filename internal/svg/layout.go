package svg

import (
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
		return "#A8CCDF"
	case catalog.KindAPI:
		return "#FCE0BA"
		// return "#FADA7A"
	case catalog.KindResource:
		return "#D5E8D4"
		// return "#B4DEBD"
	case catalog.KindGroup:
		return "#F5EEDC"
		// return "#F2F0EB"
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

func (l *StandardLayouter) labels(e, contextEntity catalog.Entity) []dot.NodeLabel {
	var labels []dot.NodeLabel

	// <<Stereotypes>>
	if st, ok := l.stereotype(e); ok {
		labels = append(labels, dot.NodeLabel{
			Text:  "«" + st + "»",
			Style: dot.LSEm | dot.LSSmall | dot.LSLight,
		})
	}

	// Domain name for system entities.
	if s, ok := e.(*catalog.System); ok {
		if d := s.GetDomain(); d != nil && d.QName() != "" {
			labels = append(labels, dot.NodeLabel{
				Text:  d.QName(),
				Style: dot.LSSmall | dot.LSLight,
			})
		}
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
	if sysName, ok := systemName(); ok {
		labels = append(labels, dot.NodeLabel{
			Text:  sysName,
			Style: dot.LSSmall | dot.LSLight,
		})
	}

	// Qualified entity name (optionally split across two lines if too long).
	meta := e.GetMetadata()
	if meta.Namespace != "" && meta.Namespace != catalog.DefaultNamespace {
		nsPrefix := meta.Namespace + "/"
		if len(nsPrefix+meta.Name) > 30 {
			labels = append(labels, dot.NodeLabel{Text: nsPrefix})
			labels = append(labels, dot.NodeLabel{Text: meta.Name})
		} else {
			labels = append(labels, dot.NodeLabel{Text: nsPrefix + meta.Name})
		}
	} else {
		labels = append(labels, dot.NodeLabel{Text: meta.Name})
	}

	return labels
}

// nodeTooltipAttrs builds the tooltip attributes for a node. Currently this only
// surfaces the API's provider component(s); other entity kinds get no tooltip.
func (l *StandardLayouter) nodeTooltipAttrs(e catalog.Entity) []dot.TooltipAttr {
	a, ok := e.(*catalog.API)
	if !ok {
		return nil
	}
	providers := a.GetProviders()
	if len(providers) == 0 {
		return nil
	}
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.QName()
	}
	return []dot.TooltipAttr{
		{Key: "provided by", Value: strings.Join(names, ", ")},
	}
}

func (l *StandardLayouter) Node(e catalog.Entity) dot.NodeLayout {
	return l.NodeContext(e, nil)
}

func (l *StandardLayouter) NodeContext(e, contextEntity catalog.Entity) dot.NodeLayout {

	fillColor := l.fillColor(e)
	borderColor := "#000000"
	if strings.HasPrefix(fillColor, "#") {
		// Use slightly darkened version of the fill color for borders.
		if c, err := AdjustLightness(fillColor, 0.85); err == nil {
			borderColor = c
		}
	}
	return dot.NodeLayout{
		Labels:       l.labels(e, contextEntity),
		FillColor:    fillColor,
		BorderColor:  borderColor,
		Shape:        l.shape(e),
		TooltipAttrs: l.nodeTooltipAttrs(e),
	}
}

func (l *StandardLayouter) Edge(src, dst catalog.Entity, style dot.EdgeStyle) dot.EdgeLayout {
	return dot.EdgeLayout{
		Style: style,
	}
}

// tooltipAttrs builds the list of tooltip attributes that should be displayed.
// It also returns the list of attr keys (all except "version"), sorted alphabetically.
func (l *StandardLayouter) tooltipAttrs(src, dst catalog.Entity, ref *catalog.LabelRef) (otherKeys []string, tooltipAttrs []dot.TooltipAttr) {
	otherKeys = make([]string, 0, len(ref.Attrs))
	for k := range ref.Attrs {
		if k == catalog.VersionAttrKey {
			continue
		}
		otherKeys = append(otherKeys, k)
	}
	slices.Sort(otherKeys)
	tooltipAttrs = []dot.TooltipAttr{
		{Key: "", Value: src.GetQName() + " → " + dst.GetQName()},
	}
	if ref.Label != "" {
		tooltipAttrs = append(tooltipAttrs, dot.TooltipAttr{Key: "label", Value: ref.Label})
	}
	if val, ok := ref.Attrs[catalog.VersionAttrKey]; ok {
		tooltipAttrs = append(tooltipAttrs, dot.TooltipAttr{
			Key: "version", Value: val,
		})
	}
	for _, k := range otherKeys {
		tooltipAttrs = append(tooltipAttrs, dot.TooltipAttr{
			Key: k, Value: ref.Attrs[k],
		})
	}
	return otherKeys, tooltipAttrs
}

// EdgeLabel generates a dot.EdgeLayout with an edge label.
// The label and tooltip are built from the ref's label and attributes.
func (l *StandardLayouter) EdgeLabel(src, dst catalog.Entity, ref *catalog.LabelRef, style dot.EdgeStyle) dot.EdgeLayout {
	var labelParts []string
	// Version
	if version, ok := ref.GetAttr(catalog.VersionAttrKey); ok && l.config.ShowVersionAsLabel {
		labelParts = append(labelParts, version)
	}
	// Label
	if ref.Label != "" {
		labelParts = append(labelParts, ref.Label)
	}
	// Attrs keys
	otherKeys, tooltipAttrs := l.tooltipAttrs(src, dst, ref)
	if len(otherKeys) > 0 {
		labelParts = append(labelParts, strings.Join(otherKeys, "/"))
	}
	return dot.EdgeLayout{
		Label:        joinWrap(labelParts, " · ", 20),
		Style:        style,
		TooltipAttrs: tooltipAttrs,
	}
}

// Interface implementation assertion
var _ Layouter = (*StandardLayouter)(nil)
