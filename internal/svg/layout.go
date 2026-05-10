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

// DiagramKind identifies the kind of diagram being rendered. Layout methods
// consult it to tailor labels and tooltips (e.g. surface the API provider only
// on system views, where many APIs from various systems appear side by side).
type DiagramKind int

const (
	DiagramAdHoc DiagramKind = iota
	DiagramDomain
	DiagramSystem
	DiagramComponent
	DiagramAPI
	DiagramResource
)

func (r *render) fillColor(e catalog.Entity) string {
	if c, ok := e.GetMetadata().Annotations[catalog.AnnotFillColor]; ok {
		// explicit annotation overrides everything
		return c
	}

	// Next priority: check colors per label.
	if len(r.config.NodeColors.Labels) > 0 {
		for k, v := range e.GetMetadata().Labels {
			if m, ok := r.config.NodeColors.Labels[k]; ok && m != nil {
				if val, ok := m[v]; ok {
					return string(val)
				}
			}
		}
	}

	// Check colors per type.
	if len(r.config.NodeColors.Types) > 0 {
		typ := e.GetType()
		if col, ok := r.config.NodeColors.Types[typ]; ok {
			return string(col)
		}
	}

	// Default colors
	switch e.GetKind() {
	case catalog.KindComponent:
		return "#D2E5EF"
	case catalog.KindSystem:
		return "#A8CCDF"
	case catalog.KindAPI:
		return "#FCE0BA"
	case catalog.KindResource:
		return "#D5E8D4"
	case catalog.KindGroup:
		return "#F5EEDC"
	}
	return "#F5EEDC" // neutral beige
}

func (r *render) shape(e catalog.Entity) dot.NodeShape {
	switch e.GetKind() {
	case catalog.KindSystem:
		return dot.NSBox
	case catalog.KindGroup:
		return dot.NSEllipse
	}
	return dot.NSRoundedBox
}

func (r *render) stereotype(e catalog.Entity) (string, bool) {
	meta := e.GetMetadata()

	if st, ok := meta.Annotations[catalog.AnnotSterotype]; ok {
		// Explicit stereotype annotation overrides everything
		return st, true
	}

	if len(r.config.StereotypeLabels) == 0 {
		return "", false
	}

	var values []string
	for _, lbl := range r.config.StereotypeLabels {
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

func (r *render) labels(e, contextEntity catalog.Entity) []dot.NodeLabel {
	var labels []dot.NodeLabel

	// <<Stereotypes>>
	if st, ok := r.stereotype(e); ok {
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
		if !r.config.ShowParentSystem || e.GetKind() == catalog.KindSystem {
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

// nodeTooltipAttrs builds the tooltip attributes for a node. Tooltip content is
// tailored to the diagram kind: e.g. on a system view, API nodes get a
// "provided by" entry, since seeing the implementing component is useful when
// many APIs from different systems appear together.
func (r *render) nodeTooltipAttrs(e catalog.Entity) []dot.TooltipAttr {
	if r.kind != DiagramSystem {
		return nil
	}
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

func (r *render) nodeLayout(e, contextEntity catalog.Entity) dot.NodeLayout {
	fillColor := r.fillColor(e)
	borderColor := "#000000"
	if strings.HasPrefix(fillColor, "#") {
		// Use slightly darkened version of the fill color for borders.
		if c, err := AdjustLightness(fillColor, 0.85); err == nil {
			borderColor = c
		}
	}
	return dot.NodeLayout{
		Labels:       r.labels(e, contextEntity),
		FillColor:    fillColor,
		BorderColor:  borderColor,
		Shape:        r.shape(e),
		TooltipAttrs: r.nodeTooltipAttrs(e),
	}
}

func (r *render) edgeLayout(src, dst catalog.Entity, style dot.EdgeStyle) dot.EdgeLayout {
	return dot.EdgeLayout{
		Style: style,
	}
}

// edgeLabelTooltipAttrs builds the list of tooltip attributes for a labelled edge.
// It also returns the list of attr keys (all except "version"), sorted alphabetically.
func (r *render) edgeLabelTooltipAttrs(src, dst catalog.Entity, ref *catalog.LabelRef) (otherKeys []string, tooltipAttrs []dot.TooltipAttr) {
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

// edgeLabelLayout builds a dot.EdgeLayout for a labelled edge. The label and
// tooltip are built from the ref's label and attributes.
func (r *render) edgeLabelLayout(src, dst catalog.Entity, ref *catalog.LabelRef, style dot.EdgeStyle) dot.EdgeLayout {
	var labelParts []string
	// Version
	if version, ok := ref.GetAttr(catalog.VersionAttrKey); ok && r.config.ShowVersionAsLabel {
		labelParts = append(labelParts, version)
	}
	// Label
	if ref.Label != "" {
		labelParts = append(labelParts, ref.Label)
	}
	// Attrs keys
	otherKeys, tooltipAttrs := r.edgeLabelTooltipAttrs(src, dst, ref)
	if len(otherKeys) > 0 {
		labelParts = append(labelParts, strings.Join(otherKeys, "/"))
	}
	return dot.EdgeLayout{
		Label:        joinWrap(labelParts, " · ", 20),
		Style:        style,
		TooltipAttrs: tooltipAttrs,
	}
}
