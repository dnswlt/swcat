package sysview

import (
	"bytes"
	"fmt"
	"html"
	"math"
	"strings"
)

// Render lays out the diagram and returns its SVG representation. The output
// contains only the <svg> element, matching what the dot-based renderer
// returns, and uses the same ids and CSS classes so the frontend can treat both
// interchangeably.
func Render(d *Diagram, st Style) []byte {
	Layout(d, st)

	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%spt" height="%spt" viewBox="0 0 %s %s">`,
		num(d.width), num(d.height), num(d.width), num(d.height))
	b.WriteString("\n")
	// The inner <g> mirrors graphviz's "graph0": .graphviz-svg is what pins the
	// SVG's font to Noto Sans in the stylesheet.
	b.WriteString(`<g id="graph0" class="graph graphviz-svg">` + "\n")
	fmt.Fprintf(&b, `<rect fill="%s" stroke="none" x="0" y="0" width="%s" height="%s"/>`+"\n",
		st.Background, num(d.width), num(d.height))

	r := &renderer{b: &b, st: st}
	if d.Focal != nil {
		r.group(d.Focal)
	}
	for _, it := range d.Externals {
		if it.Group != nil {
			r.group(it.Group)
		}
	}
	if d.Focal != nil {
		for _, n := range d.Focal.Nodes {
			r.node(n)
		}
	}
	for _, it := range d.Externals {
		for _, n := range it.nodes() {
			r.node(n)
		}
	}
	for _, e := range d.Edges {
		r.edge(e)
	}

	b.WriteString("</g>\n</svg>\n")
	return b.Bytes()
}

type renderer struct {
	b  *bytes.Buffer
	st Style
}

func (r *renderer) group(g *Group) {
	fmt.Fprintf(r.b, `<g id="%s" class="cluster graphviz-svg">`+"\n", html.EscapeString(g.ID))
	fill, border := r.st.GroupFill, r.st.GroupBorder
	if g.Fill != "" {
		fill, border = g.Fill, g.Border
	}
	fmt.Fprintf(r.b, `<rect fill="%s" stroke="%s" x="%s" y="%s" width="%s" height="%s"/>`+"\n",
		fill, border, num(g.x), num(g.y), num(g.w), num(g.h))
	if g.Label != "" {
		// Baseline sits inside the top padding, centered over the box. A group
		// with nothing in it — every relationship is with the system as a whole
		// — is all title, so that gets centered instead.
		baseline := g.y + r.st.GroupPadTop - 14
		if len(g.Nodes) == 0 {
			baseline = g.centerY() + r.st.GroupFontSize*0.35
		}
		fmt.Fprintf(r.b, `<text text-anchor="middle" x="%s" y="%s" font-size="%s" fill="%s">%s</text>`+"\n",
			num(g.x+g.w/2), num(baseline), num(r.st.GroupFontSize), r.st.GroupLabel,
			html.EscapeString(g.Label))
	}
	r.b.WriteString("</g>\n")
}

func (r *renderer) node(n *Node) {
	fmt.Fprintf(r.b, `<g id="%s" class="node clickable-node">`+"\n", html.EscapeString(n.ID))
	rad := 0.0
	if n.Shape == ShapeRounded {
		rad = r.st.NodeRadius
	}
	fill, border := n.Fill, n.Border
	if fill == "" {
		fill = "#FFFFFF"
	}
	if border == "" {
		border = "#000000"
	}
	fmt.Fprintf(r.b, `<rect fill="%s" stroke="%s" x="%s" y="%s" width="%s" height="%s" rx="%s" ry="%s"/>`+"\n",
		fill, border, num(n.x), num(n.y), num(n.w), num(n.h), num(rad), num(rad))

	// Label lines are centered as a block within the node.
	var textH float64
	for _, lbl := range n.Labels {
		textH += r.st.lineHeight(lbl)
	}
	y := n.centerY() - textH/2
	for _, lbl := range n.Labels {
		lh := r.st.lineHeight(lbl)
		// Baseline roughly at 78% of the line box, which centers x-height well
		// enough for the two sizes in play.
		baseline := y + lh*0.78
		var attrs []string
		attrs = append(attrs, fmt.Sprintf(`font-size="%s"`, num(r.st.fontSize(lbl))))
		if c := r.labelColor(lbl); c != "" {
			attrs = append(attrs, fmt.Sprintf(`fill="%s"`, c))
		}
		if lbl.Style.em() {
			attrs = append(attrs, `font-style="italic"`)
		}
		fmt.Fprintf(r.b, `<text text-anchor="middle" x="%s" y="%s" %s>%s</text>`+"\n",
			num(n.x+n.w/2), num(baseline), strings.Join(attrs, " "),
			html.EscapeString(lbl.Text))
		y += lh
	}
	r.b.WriteString("</g>\n")
}

func (r *renderer) labelColor(l Label) string {
	if l.Color != "" {
		return l.Color
	}
	if l.Style.light() {
		return r.st.SmallColor
	}
	return ""
}

func (r *renderer) edge(e *Edge) {
	class := "edge"
	if e.Class != "" {
		class += " " + e.Class
	}
	fmt.Fprintf(r.b, `<g id="%s" class="%s">`+"\n", html.EscapeString(e.ID), class)

	// The arrow head occupies the last stretch of the path, so the line stops
	// short of the port.
	tipX := e.x2
	lineEndX := tipX - e.dir*r.st.ArrowLength
	fmt.Fprintf(r.b, `<path fill="none" stroke="%s" d="%s"/>`+"\n",
		r.st.EdgeColor, r.path(e, lineEndX))
	fmt.Fprintf(r.b, `<polygon fill="%s" stroke="%s" points="%s"/>`+"\n",
		r.st.EdgeColor, r.st.EdgeColor, r.arrow(tipX, e.y2, e.dir))

	// The label sits above the horizontal run the layout reserved space for,
	// with its last line closest to the line.
	x, y := r.labelAnchor(e, lineEndX)
	baseline := y - 5 - float64(len(e.labelLines)-1)*r.st.EdgeLabelLineHeight
	for _, line := range e.labelLines {
		// Labels are drawn on top of the routing, so they get a halo in the
		// background color: painting the stroke first keeps the glyphs crisp.
		fmt.Fprintf(r.b, `<text text-anchor="middle" x="%s" y="%s" font-size="%s" fill="%s" `+
			`stroke="%s" stroke-width="3" paint-order="stroke">%s</text>`+"\n",
			num(x), num(baseline), num(r.st.EdgeFontSize), r.st.EdgeLabelColor,
			r.st.Background, html.EscapeString(line))
		baseline += r.st.EdgeLabelLineHeight
	}
	r.b.WriteString("</g>\n")
}

// path builds the edge's outline: a straight line when the ports are level,
// otherwise two right angles through the edge's lane, with the corners rounded
// off by quadratic curves.
func (r *renderer) path(e *Edge, endX float64) string {
	if e.track < 0 || math.Abs(e.y1-e.y2) < 0.5 {
		return fmt.Sprintf("M%s,%s L%s,%s", num(e.x1), num(e.y1), num(endX), num(e.y1))
	}
	xt := e.trackX
	sy := 1.0
	if e.y2 < e.y1 {
		sy = -1
	}
	// Never round more than the available run in either direction.
	rad := math.Min(r.st.CornerRad, math.Abs(e.y2-e.y1)/2)
	rad = math.Min(rad, math.Abs(xt-e.x1))
	rad = math.Min(rad, math.Abs(endX-xt))

	var b strings.Builder
	fmt.Fprintf(&b, "M%s,%s", num(e.x1), num(e.y1))
	fmt.Fprintf(&b, " L%s,%s", num(xt-e.dir*rad), num(e.y1))
	fmt.Fprintf(&b, " Q%s,%s %s,%s", num(xt), num(e.y1), num(xt), num(e.y1+sy*rad))
	fmt.Fprintf(&b, " L%s,%s", num(xt), num(e.y2-sy*rad))
	fmt.Fprintf(&b, " Q%s,%s %s,%s", num(xt), num(e.y2), num(xt+e.dir*rad), num(e.y2))
	fmt.Fprintf(&b, " L%s,%s", num(endX), num(e.y2))
	return b.String()
}

// arrow returns the triangle drawn at the edge's target port.
func (r *renderer) arrow(tipX, tipY, dir float64) string {
	backX := tipX - dir*r.st.ArrowLength
	half := r.st.ArrowWidth / 2
	return fmt.Sprintf("%s,%s %s,%s %s,%s",
		num(tipX), num(tipY),
		num(backX), num(tipY-half),
		num(backX), num(tipY+half))
}

// labelAnchor returns the middle of the run the layout picked for the label and
// sized the gutter for.
func (r *renderer) labelAnchor(e *Edge, endX float64) (x, y float64) {
	if e.track < 0 || math.Abs(e.y1-e.y2) < 0.5 {
		return (e.x1 + endX) / 2, e.y1
	}
	if e.labelAtSource {
		return (e.x1 + e.trackX) / 2, e.y1
	}
	return (e.trackX + endX) / 2, e.y2
}

// num formats a coordinate compactly, dropping trailing zeros.
func num(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}
