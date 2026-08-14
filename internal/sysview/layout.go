package sysview

import (
	"math"
	"slices"
	"sort"
	"strings"
)

const (
	portLeft = iota
	portRight
)

// Layout computes positions and sizes for everything in d. It is idempotent
// only in the sense that it fully overwrites any previous geometry.
//
// The pipeline is a stripped-down Sugiyama: three fixed layers (left column,
// focal system, right column), crossing reduction by barycenter sweeps,
// coordinate assignment that pulls each external box level with the boxes it
// connects to, and finally orthogonal edge routing through vertical tracks.
func Layout(d *Diagram, st Style) {
	l := &layouter{d: d, st: st}
	l.index()
	// Sides come before measuring: which side of a box an edge attaches to
	// decides how many ports that side has to hold, and that is part of how
	// tall the box needs to be.
	l.assignSides()
	l.measure()
	l.orderLayers()
	l.placeY()
	l.assignPorts()
	// Boxes were placed level with the centers of the boxes they connect to,
	// which is only an approximation of where the edges actually attach. Now
	// that the ports are known, nudge the columns so the edges line up with
	// them, and re-derive the ports from the new positions.
	for range 3 {
		l.alignToPorts()
		l.assignPorts()
	}
	l.chooseLabelRuns()
	leftTracks, rightTracks := l.assignTracks()
	l.placeX(leftTracks, rightTracks)
	l.finish()
}

type layouter struct {
	d  *Diagram
	st Style

	nodeByID map[string]*Node
	itemOf   map[string]*Item // external node id -> its column item
	isFocal  map[string]bool

	left, right []*Item

	// edges attaching to a node on a given side, in port order (top to bottom).
	portEdges map[portKey][]*Edge

	// natural width of each column item and of the focal system's nodes.
	itemWidth  float64
	focalWidth float64
}

type portKey struct {
	node *Node
	side int
}

func (l *layouter) index() {
	l.nodeByID = map[string]*Node{}
	l.itemOf = map[string]*Item{}
	l.isFocal = map[string]bool{}

	if l.d.Focal != nil {
		for _, n := range l.d.Focal.Nodes {
			l.nodeByID[n.ID] = n
			l.isFocal[n.ID] = true
		}
	}
	for _, it := range l.d.Externals {
		for _, n := range it.nodes() {
			l.nodeByID[n.ID] = n
			l.itemOf[n.ID] = it
		}
	}

	// Resolve edge endpoints, dropping anything that does not connect the focal
	// system to an external one — the external view never produces those, and
	// carrying them would have no place to be drawn.
	kept := l.d.Edges[:0]
	for _, e := range l.d.Edges {
		from, okFrom := l.nodeByID[e.From]
		to, okTo := l.nodeByID[e.To]
		if !okFrom || !okTo {
			continue
		}
		if l.isFocal[e.From] == l.isFocal[e.To] {
			continue
		}
		e.srcNode, e.dstNode = from, to
		kept = append(kept, e)
	}
	l.d.Edges = kept
}

// focalNode returns the edge's endpoint inside the focal system, and the one
// outside it.
func (l *layouter) ends(e *Edge) (focal, ext *Node) {
	if l.isFocal[e.From] {
		return e.srcNode, e.dstNode
	}
	return e.dstNode, e.srcNode
}

func (l *layouter) item(e *Edge) *Item {
	_, ext := l.ends(e)
	return l.itemOf[ext.ID]
}

// measure computes the size of every box. All external items end up the same
// width, and so do all nodes of the focal system: the columns are the whole
// point of this layout, and ragged boxes would undo them.
func (l *layouter) measure() {
	st := l.st

	// A box has to be tall enough for its text, and tall enough that the edges
	// arriving at one of its sides stay far enough apart to be told apart.
	ports := l.portCounts()
	nodeSize := func(n *Node) (w, h float64) {
		for _, lbl := range n.Labels {
			w = max(w, TextWidth(lbl.Text, st.fontSize(lbl)))
			h += st.lineHeight(lbl)
		}
		busiest := max(ports[portKey{n, portLeft}], ports[portKey{n, portRight}])
		portsH := float64(busiest)*st.MinPortPitch + 2*st.PortInset
		return w + 2*st.NodePadX, max(max(h+2*st.NodePadY, st.NodeMinHeight), portsH)
	}

	// Focal system: one width for all its nodes.
	if l.d.Focal != nil {
		for _, n := range l.d.Focal.Nodes {
			w, h := nodeSize(n)
			n.w, n.h = w, h
			l.focalWidth = max(l.focalWidth, w)
		}
		l.focalWidth = max(l.focalWidth, st.NodeMinWidth)
		titleW := TextWidth(l.d.Focal.Label, st.GroupFontSize)
		l.d.Focal.w = max(l.focalWidth+2*st.GroupPadX, titleW+2*st.GroupPadX)
		l.focalWidth = l.d.Focal.w - 2*st.GroupPadX
		for _, n := range l.d.Focal.Nodes {
			n.w = l.focalWidth
		}
	}

	// External items: one width for every item in both columns, so that the
	// left and right column line up as blocks.
	for _, it := range l.d.Externals {
		var w float64
		for _, n := range it.nodes() {
			nw, nh := nodeSize(n)
			n.w, n.h = nw, nh
			w = max(w, nw)
		}
		if it.Group != nil {
			w += 2 * st.GroupPadX
			w = max(w, TextWidth(it.Group.Label, st.GroupFontSize)+2*st.GroupPadX)
		}
		l.itemWidth = max(l.itemWidth, w)
	}
	l.itemWidth = max(l.itemWidth, st.NodeMinWidth)

	for _, it := range l.d.Externals {
		it.w = l.itemWidth
		inner := l.itemWidth
		if it.Group != nil {
			inner -= 2 * st.GroupPadX
			it.Group.w = l.itemWidth
		}
		for _, n := range it.nodes() {
			n.w = inner
		}
		it.h = l.itemHeight(it)
	}

	for _, e := range l.d.Edges {
		e.labelLines = wrapLabel(e.Label, st)
		for _, line := range e.labelLines {
			e.labelW = max(e.labelW, TextWidth(line, st.EdgeFontSize))
		}
		e.labelH = float64(len(e.labelLines)) * st.EdgeLabelLineHeight
	}
}

// wrapLabel breaks a label into the lines it is drawn as. Explicit newlines are
// honored, and any line still too wide is wrapped at spaces — a label is only
// worth reserving space for if that space stays within reason.
func wrapLabel(label string, st Style) []string {
	if label == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(label, "\n") {
		words := strings.Fields(part)
		if len(words) == 0 {
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if TextWidth(line+" "+w, st.EdgeFontSize) > st.EdgeLabelMaxWidth {
				out = append(out, line)
				line = w
				continue
			}
			line += " " + w
		}
		out = append(out, line)
	}
	return out
}

func (l *layouter) itemHeight(it *Item) float64 {
	if it.Group == nil {
		return it.Node.h
	}
	h := l.st.GroupPadTop + l.st.GroupPadBottom
	for i, n := range it.Group.Nodes {
		if i > 0 {
			h += l.st.NodeVGap
		}
		h += n.h
	}
	it.Group.h = h
	return h
}

// assignSides puts every external system in the column that its edges argue
// for: consumers of the focal system on the left, systems the focal system
// calls on the right. A system that does both goes to the side with more
// edges — ties to the left — and the minority edges simply leave from the near
// side of their box, so no arrow has to travel around the focal system.
func (l *layouter) assignSides() {
	incoming := map[*Item]int{}
	outgoing := map[*Item]int{}
	for _, e := range l.d.Edges {
		it := l.item(e)
		if l.isFocal[e.To] {
			incoming[it]++
		} else {
			outgoing[it]++
		}
	}
	for _, it := range l.d.Externals {
		if incoming[it] >= outgoing[it] {
			it.side = SideLeft
			l.left = append(l.left, it)
		} else {
			it.side = SideRight
			l.right = append(l.right, it)
		}
	}
}

// orderLayers reduces edge crossings by alternating barycenter sweeps between
// the focal system's nodes and the two external columns, keeping the best
// arrangement seen. The graphs here are small (tens of nodes), so a handful of
// sweeps is both cheap and close to optimal.
func (l *layouter) orderLayers() {
	if l.d.Focal == nil {
		return
	}
	focal := l.d.Focal.Nodes

	type snapshot struct {
		focal       []*Node
		left, right []*Item
	}
	best := snapshot{slices.Clone(focal), slices.Clone(l.left), slices.Clone(l.right)}
	bestCrossings := l.countCrossings(best.focal, best.left, best.right)

	for range 4 {
		l.sortItemsByBarycenter(l.left, focal)
		l.sortItemsByBarycenter(l.right, focal)
		l.sortFocalByBarycenter(focal)
		if c := l.countCrossings(focal, l.left, l.right); c < bestCrossings {
			bestCrossings = c
			best = snapshot{slices.Clone(focal), slices.Clone(l.left), slices.Clone(l.right)}
		}
	}
	copy(focal, best.focal)
	copy(l.left, best.left)
	copy(l.right, best.right)

	// Within an expanded external system, order the parts by the focal nodes
	// they connect to, for the same reason.
	for _, it := range l.d.Externals {
		if it.Group == nil {
			continue
		}
		pos := indexOf(focal)
		sortStable(it.Group.Nodes, func(n *Node) float64 {
			return l.barycenter(l.partnersOf(n), pos)
		})
	}
}

// partnersOf returns the nodes on the other end of n's edges.
func (l *layouter) partnersOf(n *Node) []*Node {
	var out []*Node
	for _, e := range l.d.Edges {
		switch n {
		case e.srcNode:
			out = append(out, e.dstNode)
		case e.dstNode:
			out = append(out, e.srcNode)
		}
	}
	return out
}

func (l *layouter) sortItemsByBarycenter(items []*Item, focal []*Node) {
	pos := indexOf(focal)
	sortStable(items, func(it *Item) float64 {
		var partners []*Node
		for _, n := range it.nodes() {
			partners = append(partners, l.partnersOf(n)...)
		}
		return l.barycenter(partners, pos)
	})
}

func (l *layouter) sortFocalByBarycenter(focal []*Node) {
	// Both columns contribute; positions are normalized to [0,1] so that a long
	// column does not outweigh a short one.
	posLeft := normalizedIndex(l.left)
	posRight := normalizedIndex(l.right)
	sortStable(focal, func(n *Node) float64 {
		var sum float64
		var cnt int
		for _, p := range l.partnersOf(n) {
			it := l.itemOf[p.ID]
			if v, ok := posLeft[it]; ok {
				sum, cnt = sum+v, cnt+1
			} else if v, ok := posRight[it]; ok {
				sum, cnt = sum+v, cnt+1
			}
		}
		if cnt == 0 {
			return math.Inf(1) // unconnected nodes sink to the bottom
		}
		return sum / float64(cnt)
	})
}

func (l *layouter) barycenter(partners []*Node, pos map[*Node]float64) float64 {
	var sum float64
	var cnt int
	for _, p := range partners {
		if v, ok := pos[p]; ok {
			sum, cnt = sum+v, cnt+1
		}
	}
	if cnt == 0 {
		return math.Inf(1)
	}
	return sum / float64(cnt)
}

func (l *layouter) countCrossings(focal []*Node, left, right []*Item) int {
	pos := indexOf(focal)
	count := func(items []*Item) int {
		itemPos := map[*Item]int{}
		for i, it := range items {
			itemPos[it] = i
		}
		type pair struct{ a, b float64 }
		var pairs []pair
		for _, e := range l.d.Edges {
			f, _ := l.ends(e)
			it := l.item(e)
			ip, ok := itemPos[it]
			if !ok {
				continue
			}
			pairs = append(pairs, pair{float64(ip), pos[f]})
		}
		n := 0
		for i := range pairs {
			for j := i + 1; j < len(pairs); j++ {
				if (pairs[i].a-pairs[j].a)*(pairs[i].b-pairs[j].b) < 0 {
					n++
				}
			}
		}
		return n
	}
	return count(left) + count(right)
}

// placeY stacks the focal system's nodes at a fixed pitch and then floats each
// external box to the average height of the boxes it connects to, so that most
// edges come out as straight horizontal lines. Overlaps are resolved by
// relaxation, which keeps well-connected boxes closest to their ideal height.
func (l *layouter) placeY() {
	st := l.st
	if l.d.Focal != nil {
		y := st.GroupPadTop
		for _, n := range l.d.Focal.Nodes {
			n.y = y
			y += n.h + st.NodeVGap
		}
		if len(l.d.Focal.Nodes) > 0 {
			y -= st.NodeVGap
		}
		l.d.Focal.y = 0
		l.d.Focal.h = y + st.GroupPadBottom
	}

	l.placeColumnY(l.left)
	l.placeColumnY(l.right)
	l.normalizeY()
}

// normalizeY shifts the whole drawing so its topmost element sits at the margin.
func (l *layouter) normalizeY() {
	minY := math.Inf(1)
	if l.d.Focal != nil {
		minY = l.d.Focal.y
	}
	for _, it := range l.d.Externals {
		minY = min(minY, it.y)
	}
	if math.IsInf(minY, 1) {
		return
	}
	dy := l.st.Margin - minY
	if l.d.Focal != nil {
		l.d.Focal.y += dy
		for _, n := range l.d.Focal.Nodes {
			n.y += dy
		}
	}
	for _, it := range l.d.Externals {
		it.y += dy
		l.layoutItemNodesY(it)
	}
}

// placeColumnY gives each external box a first vertical position: level with
// the average of the boxes it connects to.
func (l *layouter) placeColumnY(items []*Item) {
	desired := make([]float64, len(items))
	weight := make([]int, len(items))
	for i, it := range items {
		var sum float64
		var cnt int
		for _, n := range it.nodes() {
			for _, p := range l.partnersOf(n) {
				if l.isFocal[p.ID] {
					sum += p.centerY()
					cnt++
				}
			}
		}
		weight[i] = cnt
		if cnt == 0 {
			desired[i] = 0
		} else {
			desired[i] = sum/float64(cnt) - it.h/2
		}
	}
	l.arrangeColumn(items, desired, weight)
	for _, it := range items {
		l.layoutItemNodesY(it)
	}
}

// alignToPorts moves each external box by the average offset between its own
// ports and the ports they connect to, which turns near misses into properly
// horizontal edges.
func (l *layouter) alignToPorts() {
	for _, items := range [][]*Item{l.left, l.right} {
		desired := make([]float64, len(items))
		weight := make([]int, len(items))
		for i, it := range items {
			var sum float64
			var cnt int
			for _, e := range l.d.Edges {
				if l.item(e) != it {
					continue
				}
				own, partner := e.y2, e.y1 // external end is the edge's target
				if l.isFocal[e.To] {
					own, partner = e.y1, e.y2
				}
				sum += partner - own
				cnt++
			}
			weight[i] = cnt
			desired[i] = it.y
			if cnt > 0 {
				desired[i] += sum / float64(cnt)
			}
		}
		l.arrangeColumn(items, desired, weight)
	}
	l.normalizeY()
}

// arrangeColumn places items as close to their desired positions as the
// required vertical spacing allows, giving priority to the best connected ones.
func (l *layouter) arrangeColumn(items []*Item, desired []float64, weight []int) {
	if len(items) == 0 {
		return
	}
	st := l.st

	// Initial feasible placement: top down, never overlapping.
	y := make([]float64, len(items))
	for i := range items {
		y[i] = desired[i]
		if i > 0 {
			y[i] = max(y[i], y[i-1]+items[i-1].h+st.GroupVGap)
		}
	}

	// Relax towards the desired positions, best-connected boxes first.
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return weight[order[a]] > weight[order[b]] })
	for range 8 {
		for _, i := range order {
			lo := math.Inf(-1)
			hi := math.Inf(1)
			if i > 0 {
				lo = y[i-1] + items[i-1].h + st.GroupVGap
			}
			if i < len(items)-1 {
				hi = y[i+1] - items[i].h - st.GroupVGap
			}
			if weight[i] == 0 {
				continue
			}
			y[i] = math.Min(math.Max(desired[i], lo), math.Max(lo, hi))
		}
	}
	for i, it := range items {
		it.y = y[i]
	}
}

func (l *layouter) layoutItemNodesY(it *Item) {
	if it.Group == nil {
		it.Node.y = it.y
		return
	}
	it.Group.y = it.y
	y := it.y + l.st.GroupPadTop
	for _, n := range it.Group.Nodes {
		n.y = y
		y += n.h + l.st.NodeVGap
	}
}

// portSides returns which side of the source box and of the target box an edge
// attaches to. Both endpoints face each other: for a system in the left column
// that means the external box uses its right side and the focal node its left.
func (l *layouter) portSides(e *Edge) (srcSide, dstSide int) {
	focalSide, extSide := portLeft, portRight
	if l.item(e).side == SideRight {
		focalSide, extSide = portRight, portLeft
	}
	if focal, _ := l.ends(e); focal == e.srcNode {
		return focalSide, extSide
	}
	return extSide, focalSide
}

// portCounts returns how many edges attach to each side of each box.
func (l *layouter) portCounts() map[portKey]int {
	counts := map[portKey]int{}
	for _, e := range l.d.Edges {
		srcSide, dstSide := l.portSides(e)
		counts[portKey{e.srcNode, srcSide}]++
		counts[portKey{e.dstNode, dstSide}]++
	}
	return counts
}

// assignPorts decides where on a box each edge attaches. Edges always leave and
// enter through the side facing the other box, and several edges on the same
// side are spread over the box's height — which measure() already made tall
// enough to keep them apart.
func (l *layouter) assignPorts() {
	l.portEdges = map[portKey][]*Edge{}

	for _, e := range l.d.Edges {
		e.srcSide, e.dstSide = l.portSides(e)
		l.portEdges[portKey{e.srcNode, e.srcSide}] = append(l.portEdges[portKey{e.srcNode, e.srcSide}], e)
		l.portEdges[portKey{e.dstNode, e.dstSide}] = append(l.portEdges[portKey{e.dstNode, e.dstSide}], e)
	}

	// Order the ports of each side by where the other end sits, then spread
	// them over the box, keeping clear of its rounded corners.
	for key, edges := range l.portEdges {
		sort.SliceStable(edges, func(a, b int) bool {
			return l.otherEnd(edges[a], key.node).centerY() < l.otherEnd(edges[b], key.node).centerY()
		})
		n := len(edges)
		span := math.Max(key.node.h-2*l.st.PortInset, 0)
		for i, e := range edges {
			y := key.node.centerY()
			if n > 1 {
				y += span * ((float64(i)+0.5)/float64(n) - 0.5)
			}
			if e.srcNode == key.node && e.srcSide == key.side {
				e.y1 = y
			} else {
				e.y2 = y
			}
		}
	}

	// Where both ends are the only edge on their side, nudge them onto a common
	// height if they are nearly level: a straight line reads better than a
	// barely visible jog.
	for _, e := range l.d.Edges {
		if len(l.portEdges[portKey{e.srcNode, e.srcSide}]) != 1 ||
			len(l.portEdges[portKey{e.dstNode, e.dstSide}]) != 1 {
			continue
		}
		if math.Abs(e.y1-e.y2) <= l.st.SnapY {
			mid := (e.y1 + e.y2) / 2
			e.y1, e.y2 = mid, mid
		}
	}
}

// chooseLabelRuns decides, for each labelled edge, whether its label goes on
// the run before the bend or the one after it. A label is drawn above its line,
// so what matters is how much room there is up there: at an end where several
// edges fan into the same box the lines sit a few points apart and a label
// would lie across them, while at an end where the edge is on its own it has
// the whole gap to itself.
func (l *layouter) chooseLabelRuns() {
	for _, e := range l.d.Edges {
		if len(e.labelLines) == 0 {
			continue
		}
		atSource := l.portClearance(e.srcNode, e.srcSide, e.y1)
		atTarget := l.portClearance(e.dstNode, e.dstSide, e.y2)
		e.labelAtSource = atSource >= atTarget
	}
}

// portClearance returns the vertical distance from a port to the next port
// above it on the same side of the same box, which is the room a label drawn
// above that edge's line has. There is no limit above the topmost port.
func (l *layouter) portClearance(n *Node, side int, y float64) float64 {
	clearance := math.Inf(1)
	for _, e := range l.portEdges[portKey{n, side}] {
		other := e.y2
		if e.srcNode == n && e.srcSide == side {
			other = e.y1
		}
		if other < y {
			clearance = math.Min(clearance, y-other)
		}
	}
	return clearance
}

func (l *layouter) otherEnd(e *Edge, n *Node) *Node {
	if e.srcNode == n {
		return e.dstNode
	}
	return e.srcNode
}

// assignTracks gives every edge that has to change height its own vertical lane
// in the gutter between the columns. Lanes are reused as soon as two edges
// cannot collide, which keeps the gutters narrow.
func (l *layouter) assignTracks() (leftTracks, rightTracks int) {
	assign := func(side Side) int {
		var edges []*Edge
		for _, e := range l.d.Edges {
			if l.item(e).side != side {
				continue
			}
			if math.Abs(e.y1-e.y2) < 0.5 {
				e.track = -1 // straight line, no lane needed
				continue
			}
			edges = append(edges, e)
		}
		// Lanes fill up from the source column outwards, longest edge first.
		// That is what makes a fan of edges nest: an edge reaching further has
		// to turn earlier, or it would cut across the ones that stop short of
		// it. Within a lane, edges that cannot collide share it.
		sort.SliceStable(edges, func(a, b int) bool {
			return math.Abs(edges[a].y1-edges[a].y2) > math.Abs(edges[b].y1-edges[b].y2)
		})
		type lane struct{ lo, hi float64 }
		var lanes []lane
		for _, e := range edges {
			lo, hi := math.Min(e.y1, e.y2), math.Max(e.y1, e.y2)
			placed := false
			for i, used := range lanes {
				if hi+l.st.CornerRad <= used.lo || lo >= used.hi+l.st.CornerRad {
					lanes[i] = lane{math.Min(used.lo, lo), math.Max(used.hi, hi)}
					e.track = i
					placed = true
					break
				}
			}
			if !placed {
				lanes = append(lanes, lane{lo, hi})
				e.track = len(lanes) - 1
			}
		}
		return len(lanes)
	}
	return assign(SideLeft), assign(SideRight)
}

// placeX turns the column structure into x coordinates: the gutters are made
// just wide enough for the lanes they have to carry.
func (l *layouter) placeX(leftTracks, rightTracks int) {
	st := l.st
	gutterL, laneOffsetL := l.gutterLayout(SideLeft, leftTracks)
	gutterR, laneOffsetR := l.gutterLayout(SideRight, rightTracks)

	x := st.Margin
	var leftX, focalX, rightX float64
	if len(l.left) > 0 {
		leftX = x
		x += l.itemWidth + gutterL
	}
	focalX = x
	if l.d.Focal != nil {
		x += l.d.Focal.w
	}
	if len(l.right) > 0 {
		x += gutterR
		rightX = x
		x += l.itemWidth
	}
	l.d.width = x + st.Margin

	if l.d.Focal != nil {
		l.d.Focal.x = focalX
		for _, n := range l.d.Focal.Nodes {
			n.x = focalX + st.GroupPadX
		}
	}
	for _, it := range l.d.Externals {
		it.x = leftX
		if it.side == SideRight {
			it.x = rightX
		}
		if it.Group != nil {
			it.Group.x = it.x
		}
		for _, n := range it.nodes() {
			n.x = it.x
			if it.Group != nil {
				n.x += st.GroupPadX
			}
		}
	}

	laneX := func(gutterStart, laneOffset float64) func(int) float64 {
		return func(i int) float64 {
			return gutterStart + laneOffset + float64(i)*st.TrackGap
		}
	}
	leftLane := laneX(leftX+l.itemWidth, laneOffsetL)
	rightLane := laneX(focalX+focalWidthOf(l.d), laneOffsetR)

	for _, e := range l.d.Edges {
		e.x1 = portX(e.srcNode, e.srcSide)
		e.x2 = portX(e.dstNode, e.dstSide)
		e.dir = 1
		if e.x2 < e.x1 {
			e.dir = -1
		}
		if e.track >= 0 {
			if l.item(e).side == SideLeft {
				e.trackX = leftLane(e.track)
			} else {
				e.trackX = rightLane(e.track)
			}
		}
	}
}

// gutterLayout returns how wide the gap between two columns has to be, and how
// far from its left edge the first lane sits.
//
// Without labels the lanes sit centered, which looks balanced. A label, though,
// is drawn on one of the edge's two runs only — so widening the gutter evenly
// would stretch the other run for nothing. Instead the lanes move aside by
// exactly what the labels before them need, and the width covers what the
// labels after them need.
func (l *layouter) gutterLayout(side Side, tracks int) (width, laneOffset float64) {
	st := l.st
	var spread float64
	if tracks > 0 {
		spread = float64(tracks-1) * st.TrackGap
	}
	base := max(st.ColumnGap, spread+2*st.TrackInset)

	var leftOfLanes, rightOfLanes, straight float64
	for _, e := range l.d.Edges {
		if e.labelW == 0 || l.item(e).side != side {
			continue
		}
		need := e.labelW + 2*st.EdgeLabelPadX
		switch {
		case e.track < 0:
			// A straight edge has a single run: the whole gutter.
			straight = max(straight, need)
		case labelRunIsLeftOfLanes(e):
			leftOfLanes = max(leftOfLanes, need)
		default:
			rightOfLanes = max(rightOfLanes, need)
		}
	}
	if leftOfLanes == 0 && rightOfLanes == 0 && straight == 0 {
		return base, (base - spread) / 2
	}
	laneOffset = max(st.TrackInset, leftOfLanes)
	width = max(max(base, straight), laneOffset+spread+max(st.TrackInset, rightOfLanes))
	return width, laneOffset
}

// labelRunIsLeftOfLanes reports on which side of the lanes an edge's labelled
// run lies. Most edges run left to right, so their run before the bend is the
// left one — but an edge back to a system placed on the far side runs the other
// way, and then it is the run after the bend that sits on the left.
func labelRunIsLeftOfLanes(e *Edge) bool {
	runsRightward := e.srcSide == portRight
	return runsRightward == e.labelAtSource
}

func focalWidthOf(d *Diagram) float64 {
	if d.Focal == nil {
		return 0
	}
	return d.Focal.w
}

func portX(n *Node, side int) float64 {
	if side == portLeft {
		return n.x
	}
	return n.right()
}

// finish sizes the drawing around everything that has been placed. Edge labels
// sit above their line and can reach over the topmost box, so they get a say in
// where the top edge is.
func (l *layouter) finish() {
	top := math.Inf(1)
	bottom := 0.0
	if l.d.Focal != nil {
		top = min(top, l.d.Focal.y)
		bottom = max(bottom, l.d.Focal.bottom())
	}
	for _, it := range l.d.Externals {
		top = min(top, it.y)
		bottom = max(bottom, it.y+it.h)
	}
	for _, e := range l.d.Edges {
		if len(e.labelLines) == 0 {
			continue
		}
		labelTop := math.Min(e.y1, e.y2) - 5 - e.labelH
		top = min(top, labelTop)
	}
	if dy := l.st.Margin - top; dy > 0 {
		l.translateY(dy)
		bottom += dy
	}
	l.d.height = bottom + l.st.Margin
}

// translateY moves the whole drawing, routed edges included.
func (l *layouter) translateY(dy float64) {
	if l.d.Focal != nil {
		l.d.Focal.y += dy
		for _, n := range l.d.Focal.Nodes {
			n.y += dy
		}
	}
	for _, it := range l.d.Externals {
		it.y += dy
		l.layoutItemNodesY(it)
	}
	for _, e := range l.d.Edges {
		e.y1 += dy
		e.y2 += dy
	}
}

// indexOf maps each node to its position in the slice.
func indexOf(nodes []*Node) map[*Node]float64 {
	m := make(map[*Node]float64, len(nodes))
	for i, n := range nodes {
		m[n] = float64(i)
	}
	return m
}

// normalizedIndex maps each item to its position in [0,1].
func normalizedIndex(items []*Item) map[*Item]float64 {
	m := make(map[*Item]float64, len(items))
	for i, it := range items {
		v := 0.5
		if len(items) > 1 {
			v = float64(i) / float64(len(items)-1)
		}
		m[it] = v
	}
	return m
}

// sortStable sorts by a key function, leaving elements without a key (+Inf) in
// their original relative order at the end.
func sortStable[T any](s []T, key func(T) float64) {
	keys := make(map[int]float64, len(s))
	idx := make([]int, len(s))
	orig := slices.Clone(s)
	for i, v := range s {
		keys[i] = key(v)
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return keys[idx[a]] < keys[idx[b]] })
	for i, j := range idx {
		s[i] = orig[j]
	}
}
