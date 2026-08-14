// Package sysview lays out and renders the "external view" of a system: one
// focal system in the middle, the systems it talks to in a column left and
// right of it.
//
// That diagram is structured enough that a general-purpose layout engine like
// graphviz gives away more than it buys. Doing it directly lets us align the
// external boxes into proper columns of equal width, keep edges on horizontal
// lines wherever the endpoints allow it, and route the rest through disjoint
// vertical tracks instead of letting splines wander across the picture.
//
// The emitted SVG deliberately mirrors the DOM shape graphviz produces (node
// groups carrying the entity ref as id plus a "clickable-node" class, edge
// groups with "svg-edge-N" ids, cluster groups with "svg-cluster-N" ids), so the
// frontend's click handling, tooltips and CSS work unchanged.
package sysview

// LabelStyle controls font styling of a single label line. Values are bit flags.
type LabelStyle int

const (
	StyleEm    LabelStyle = 1 << iota // italic
	StyleSmall                        // smaller font size
	StyleLight                        // muted color
)

func (s LabelStyle) em() bool    { return s&StyleEm != 0 }
func (s LabelStyle) small() bool { return s&StyleSmall != 0 }
func (s LabelStyle) light() bool { return s&StyleLight != 0 }

// Label is one line of a node label.
type Label struct {
	Text  string
	Style LabelStyle
	Color string // optional, overrides the style's default color
}

type Shape int

const (
	ShapeRounded Shape = iota
	ShapeRect
)

// Node is a single box: a part of the focal system, a collapsed external
// system, or a part of an expanded external system.
type Node struct {
	// ID becomes the SVG element id and is what the frontend resolves to a
	// route, so it must be the entity ref (e.g. "component:ns/foo").
	ID     string
	Labels []Label
	Fill   string
	Border string
	Shape  Shape

	geom
}

// Group is a box drawn around a set of nodes: the focal system, or an external
// system shown with its parts.
type Group struct {
	// ID becomes the SVG element id; the caller assigns cluster ids so that
	// they match the metadata it ships alongside the SVG.
	ID    string
	Label string
	Nodes []*Node
	// Frame, if set, is what edges attach to that concern the system as a whole
	// rather than one of its parts — the case when components are hidden and
	// their dependencies are attributed to the system containing them. It is
	// never drawn: it borrows the group's own outline, so those edges start and
	// end on the system's border.
	Frame *Node
	// Fill and Border, when set, paint the group like a box instead of a
	// container. That is what a system with no parts to show is: a box, and it
	// should look like the other system boxes rather than like an empty frame.
	Fill   string
	Border string

	geom
}

// Item is one entry of an external column: either a collapsed system (Node) or
// a system shown in detail (Group).
type Item struct {
	Node  *Node
	Group *Group

	side Side
	geom
}

// Edge connects two nodes by their IDs. One endpoint is always a node of the
// focal system.
type Edge struct {
	// ID becomes the SVG element id and the key of the edge's metadata entry.
	ID    string
	From  string
	To    string
	Label string
	// Class is an additional CSS class set on the edge group (e.g. "system-link-edge").
	Class string

	route
	labelBox
}

// labelBox is an edge label's computed text layout: the lines it is broken
// into, and the space they need.
type labelBox struct {
	labelLines []string
	labelW     float64
	labelH     float64
	// labelAtSource selects which of the edge's two horizontal runs carries the
	// label: the one before the bend, or the one after it.
	labelAtSource bool
}

// Side is which column an external item is placed in.
type Side int

const (
	SideLeft Side = iota
	SideRight
)

// Diagram is the input to layout and rendering: the focal system, the external
// systems around it, and the edges between them. Sides, sizes and positions are
// all computed by Layout.
type Diagram struct {
	Focal     *Group
	Externals []*Item
	Edges     []*Edge

	width, height float64
}

// geom is the computed position and size of a laid out element.
type geom struct {
	x, y, w, h float64
}

func (g geom) centerY() float64 { return g.y + g.h/2 }
func (g geom) left() float64    { return g.x }
func (g geom) right() float64   { return g.x + g.w }
func (g geom) bottom() float64  { return g.y + g.h }

// route is an edge's computed geometry: the two endpoints and the vertical
// track connecting them.
type route struct {
	srcNode, dstNode *Node
	srcSide, dstSide int // portLeft or portRight of the respective node

	x1, y1 float64 // source port
	x2, y2 float64 // target port (the arrow tip)
	// track is the index of the vertical lane the edge bends through, or -1 if
	// it runs straight; trackX is that lane's x coordinate.
	track  int
	trackX float64
	// dir is +1 when the edge runs left to right, -1 when it runs right to left.
	dir float64
}

func (i *Item) nodes() []*Node {
	if i.Group != nil {
		return i.Group.Nodes
	}
	return []*Node{i.Node}
}

func (i *Item) label() string {
	if i.Group != nil {
		return i.Group.Label
	}
	return i.Node.ID
}
