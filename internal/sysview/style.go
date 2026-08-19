package sysview

// Style holds every tunable of the layout. Distances are in the SVG's user
// units, which — as with graphviz's output — are emitted as points and thus
// scaled by 4/3 in the browser. Keeping the same unit system means the existing
// CSS (font family, hover stroke widths) applies unchanged.
type Style struct {
	FontSize      float64 // node label text
	SmallFontSize float64 // "«stereotype»" and parent-domain lines
	EdgeFontSize  float64 // edge labels
	GroupFontSize float64 // cluster titles

	LineHeight      float64 // baseline distance for FontSize lines
	SmallLineHeight float64 // baseline distance for SmallFontSize lines

	NodePadX      float64 // horizontal padding between text and node border
	NodePadY      float64 // vertical padding between text and node border
	NodeMinHeight float64
	NodeMinWidth  float64
	NodeVGap      float64 // vertical gap between stacked nodes
	NodeRadius    float64 // corner radius of rounded nodes
	PortInset     float64 // distance between a box's edge and its outermost port
	MinPortPitch  float64 // smallest vertical distance between two ports on a box

	GroupPadX      float64
	GroupPadTop    float64 // includes room for the cluster title
	GroupPadBottom float64
	GroupVGap      float64 // vertical gap between stacked external items

	// ShowEdgeLabels draws the labels of labelled edges. Off by default: on a
	// large diagram they clutter the picture and, since gutters have to be wide
	// enough to fit them, widen it as well. The frontend shows them on hover
	// instead, from the metadata that ships with the SVG.
	ShowEdgeLabels      bool
	EdgeLabelLineHeight float64 // baseline distance between edge label lines
	EdgeLabelPadX       float64 // clearance between an edge label and the boxes it runs between
	EdgeLabelMaxWidth   float64 // label lines wider than this are wrapped at spaces

	ColumnGap   float64 // minimum width of the gutter between two columns
	TrackGap    float64 // horizontal distance between two vertical edge tracks
	TrackInset  float64 // clearance between a column and the nearest track
	Margin      float64 // padding around the whole drawing
	CornerRad   float64 // corner radius of orthogonal edge bends
	SnapY       float64 // endpoints closer than this vertically are made collinear
	ArrowLength float64
	ArrowWidth  float64
	// EdgeHitWidth is the width of the invisible stroke that makes an edge easy
	// to hover and click; the drawn line is far too thin to hit. Keep it below
	// MinPortPitch: edges converging on a box are that far apart, and hit areas
	// wider than the gap between them would steal each other's pointer events.
	EdgeHitWidth float64

	EdgeColor      string
	EdgeLabelColor string
	GroupFill      string
	GroupBorder    string
	GroupLabel     string
	Background     string
	SmallColor     string // color of StyleLight label lines
}

// DefaultStyle returns the style used for external views. Font sizes and
// paddings match what graphviz produces, so these diagrams are no denser than
// the ones it still draws.
func DefaultStyle() Style {
	return Style{
		FontSize:      11,
		SmallFontSize: 9,
		EdgeFontSize:  10,
		GroupFontSize: 11,

		LineHeight:      14.6,
		SmallLineHeight: 12,

		NodePadX:      8,
		NodePadY:      8,
		NodeMinHeight: 36,
		NodeMinWidth:  72,
		NodeVGap:      20,
		NodeRadius:    6,
		PortInset:     8,
		MinPortPitch:  10,

		GroupPadX:      12,
		GroupPadTop:    30,
		GroupPadBottom: 14,
		GroupVGap:      24,

		EdgeLabelLineHeight: 12,
		EdgeLabelPadX:       8,
		EdgeLabelMaxWidth:   130,

		ColumnGap:    72,
		TrackGap:     14,
		TrackInset:   16,
		Margin:       6,
		CornerRad:    8,
		SnapY:        4,
		ArrowLength:  10,
		ArrowWidth:   7,
		EdgeHitWidth: 8,

		EdgeColor:      "#8493A5",
		EdgeLabelColor: "#374151",
		GroupFill:      "#F3F4F6",
		GroupBorder:    "#C9CED7",
		GroupLabel:     "#000000",
		Background:     "white",
		SmallColor:     "#6B7280",
	}
}

// lineHeight is the vertical space one label line occupies.
func (st Style) lineHeight(l Label) float64 {
	if l.Style.small() {
		return st.SmallLineHeight
	}
	return st.LineHeight
}

// fontSize is the font size one label line renders at.
func (st Style) fontSize(l Label) float64 {
	if l.Style.small() {
		return st.SmallFontSize
	}
	return st.FontSize
}
