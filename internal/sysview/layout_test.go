package sysview

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
)

// diagram builds a small diagram: focal is the focal system with the given
// parts, externals are collapsed neighbor systems, and edges are given as
// "a->b" over the short names.
func diagram(focalParts []string, externals []string, edges []string) *Diagram {
	d := &Diagram{Focal: &Group{ID: "svg-cluster-0", Label: "focal"}}
	for _, p := range focalParts {
		d.Focal.Nodes = append(d.Focal.Nodes, &Node{ID: p, Labels: []Label{{Text: p}}})
	}
	for _, e := range externals {
		d.Externals = append(d.Externals, &Item{
			Node: &Node{ID: e, Labels: []Label{{Text: e}}},
		})
	}
	for i, spec := range edges {
		from, to, _ := strings.Cut(spec, "->")
		d.Edges = append(d.Edges, &Edge{ID: fmt.Sprintf("svg-edge-%d", i), From: from, To: to})
	}
	return d
}

func itemByID(d *Diagram, id string) *Item {
	for _, it := range d.Externals {
		for _, n := range it.nodes() {
			if n.ID == id {
				return it
			}
		}
	}
	return nil
}

func edgeByID(d *Diagram, id string) *Edge {
	for _, e := range d.Edges {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func TestSidesFollowEdgeDirection(t *testing.T) {
	d := diagram(
		[]string{"api", "comp"},
		[]string{"consumer", "provider"},
		[]string{"consumer->api", "comp->provider"},
	)
	Layout(d, DefaultStyle())

	if got := itemByID(d, "consumer").side; got != SideLeft {
		t.Errorf("consumer side = %v, want SideLeft", got)
	}
	if got := itemByID(d, "provider").side; got != SideRight {
		t.Errorf("provider side = %v, want SideRight", got)
	}
	if l, r := itemByID(d, "consumer"), itemByID(d, "provider"); l.x >= r.x {
		t.Errorf("consumer (x=%.1f) should be left of provider (x=%.1f)", l.x, r.x)
	}
}

func TestBothWaySystemGoesToDominantSideAndTiesLeft(t *testing.T) {
	// "both" sends two edges into the focal system and receives one, so it
	// belongs on the left; "tied" has one of each and goes left as well.
	d := diagram(
		[]string{"api", "comp"},
		[]string{"both", "tied"},
		[]string{"both->api", "both->comp", "comp->both", "tied->api", "comp->tied"},
	)
	Layout(d, DefaultStyle())

	for _, id := range []string{"both", "tied"} {
		if got := itemByID(d, id).side; got != SideLeft {
			t.Errorf("%s side = %v, want SideLeft", id, got)
		}
	}
}

// The minority edge of a two-way system must not travel around the focal
// system: it leaves from the near side of both boxes and simply points the
// other way.
func TestReverseEdgeStaysInItsGutter(t *testing.T) {
	d := diagram(
		[]string{"api", "comp"},
		[]string{"both"},
		[]string{"both->api", "both->comp", "comp->both"},
	)
	Layout(d, DefaultStyle())

	reverse := edgeByID(d, "svg-edge-2") // comp -> both, pointing left
	if reverse.dir != -1 {
		t.Errorf("reverse edge dir = %v, want -1", reverse.dir)
	}
	if reverse.srcSide != portLeft {
		t.Error("reverse edge should leave the focal node on its left side")
	}
	if reverse.dstSide != portRight {
		t.Error("reverse edge should enter the external node on its right side")
	}
	// The edge stays between the external box and the focal node it starts
	// from — it never wraps around the focal system.
	item := itemByID(d, "both")
	var comp *Node
	for _, n := range d.Focal.Nodes {
		if n.ID == "comp" {
			comp = n
		}
	}
	for _, x := range []float64{reverse.x1, reverse.x2, reverse.trackX} {
		if x < item.right()-0.01 || x > comp.left()+0.01 {
			t.Errorf("reverse edge x=%.1f leaves the gutter [%.1f, %.1f]",
				x, item.right(), comp.left())
		}
	}
}

func TestColumnsHaveUniformWidth(t *testing.T) {
	d := diagram(
		[]string{"a-short-part", "a-considerably-longer-part-name"},
		[]string{"tiny", "a-very-long-external-system-name", "mid-length-name"},
		[]string{"tiny->a-short-part", "a-short-part->a-very-long-external-system-name",
			"mid-length-name->a-short-part"},
	)
	Layout(d, DefaultStyle())

	width := d.Externals[0].w
	for _, it := range d.Externals {
		if it.w != width {
			t.Errorf("external %q width = %.1f, want %.1f (all columns equal)", it.label(), it.w, width)
		}
	}
	// The widest label still has to fit.
	if want := TextWidth("a-very-long-external-system-name", DefaultStyle().FontSize); width < want {
		t.Errorf("column width %.1f is too narrow for its widest label (%.1f)", width, want)
	}
	for _, n := range d.Focal.Nodes {
		if n.w != d.Focal.Nodes[0].w {
			t.Error("nodes of the focal system should all have the same width")
		}
	}
}

// A box with a single connection is placed level with it, so the edge comes out
// as a straight horizontal line.
func TestSingleEdgesComeOutStraight(t *testing.T) {
	d := diagram(
		[]string{"api", "comp"},
		[]string{"consumer", "target"},
		[]string{"consumer->api", "comp->target"},
	)
	Layout(d, DefaultStyle())

	for _, e := range d.Edges {
		if math.Abs(e.y1-e.y2) > 0.01 {
			t.Errorf("edge %s is not horizontal: y1=%.2f y2=%.2f", e.ID, e.y1, e.y2)
		}
		if e.track != -1 {
			t.Errorf("edge %s should not need a track", e.ID)
		}
	}
}

// A box grows to fit the edges arriving at it: with many of them crammed into a
// fixed height the arrow heads would merge into one wedge.
func TestBoxesGrowWithTheirPortCount(t *testing.T) {
	st := DefaultStyle()
	var sources []string
	var edges []string
	for i := range 12 {
		name := fmt.Sprintf("src%02d", i)
		sources = append(sources, name)
		edges = append(edges, name+"->target")
	}
	d := diagram(sources, []string{"target"}, edges)
	Layout(d, st)

	target := itemByID(d, "target")
	if want := 12*st.MinPortPitch + 2*st.PortInset; target.h < want {
		t.Errorf("box with 12 ports is %.1f tall, needs %.1f", target.h, want)
	}

	ys := make([]float64, 0, len(d.Edges))
	for _, e := range d.Edges {
		ys = append(ys, e.y2)
	}
	slices.Sort(ys)
	for i := 1; i < len(ys); i++ {
		if gap := ys[i] - ys[i-1]; gap < st.MinPortPitch-0.01 {
			t.Errorf("ports %.1f and %.1f are only %.1f apart, want %.1f",
				ys[i-1], ys[i], gap, st.MinPortPitch)
		}
	}

	// Boxes with few connections are unaffected.
	plain := diagram([]string{"a"}, []string{"b"}, []string{"a->b"})
	Layout(plain, st)
	if got := itemByID(plain, "b").h; got != st.NodeMinHeight {
		t.Errorf("box with one port is %.1f tall, want the minimum %.1f", got, st.NodeMinHeight)
	}
}

func TestEdgesToTheSameBoxUseDistinctPorts(t *testing.T) {
	d := diagram(
		[]string{"a", "b", "c"},
		[]string{"target"},
		[]string{"a->target", "b->target", "c->target"},
	)
	Layout(d, DefaultStyle())

	seen := map[float64]bool{}
	item := itemByID(d, "target")
	for _, e := range d.Edges {
		if seen[e.y2] {
			t.Errorf("two edges enter %q at the same height %.2f", item.label(), e.y2)
		}
		seen[e.y2] = true
		if e.y2 < item.y || e.y2 > item.bottom() {
			t.Errorf("port y=%.2f lies outside the box [%.2f, %.2f]", e.y2, item.y, item.bottom())
		}
	}
}

// Edges that overlap vertically must not share a track, or they would be drawn
// on top of each other.
func TestOverlappingEdgesGetSeparateTracks(t *testing.T) {
	d := diagram(
		[]string{"src"},
		[]string{"t1", "t2", "t3", "t4"},
		[]string{"src->t1", "src->t2", "src->t3", "src->t4"},
	)
	Layout(d, DefaultStyle())

	for i, a := range d.Edges {
		for _, b := range d.Edges[i+1:] {
			if a.track < 0 || b.track < 0 || a.track != b.track {
				continue
			}
			aLo, aHi := math.Min(a.y1, a.y2), math.Max(a.y1, a.y2)
			bLo, bHi := math.Min(b.y1, b.y2), math.Max(b.y1, b.y2)
			if aLo < bHi && bLo < aHi {
				t.Errorf("edges %s and %s share track %d with overlapping spans", a.ID, b.ID, a.track)
			}
		}
	}
}

// An edge label must fit on the horizontal run it is drawn on, so the gutter
// between the columns has to be sized for it.
func TestEdgeLabelsGetRoomOnTheirRun(t *testing.T) {
	st := DefaultStyle()
	for _, label := range []string{
		"short",
		"With a veryveryvery long label",
		"v2.1 · fetch user details · read/write",
	} {
		d := diagram(
			[]string{"comp", "other"},
			[]string{"target", "second"},
			[]string{"comp->target", "other->second", "comp->second"},
		)
		d.Edges[0].Label = label
		Layout(d, st)

		e := d.Edges[0]
		run := math.Abs(e.x2 - e.x1)
		if e.track >= 0 {
			// A bent edge is labelled on the run the layout picked for it.
			run = math.Abs(e.x2 - e.trackX)
			if e.labelAtSource {
				run = math.Abs(e.trackX - e.x1)
			}
		}
		if want := e.labelW + 2*st.EdgeLabelPadX; run < want {
			t.Errorf("label %q: run is %.1f wide, needs %.1f", label, run, want)
		}
		for _, line := range e.labelLines {
			if w := TextWidth(line, st.EdgeFontSize); w > st.EdgeLabelMaxWidth {
				t.Errorf("label line %q is %.1f wide, over the %.1f wrap width",
					line, w, st.EdgeLabelMaxWidth)
			}
		}
	}
}

// A label goes on the end of the edge that has room above the line: where
// several edges fan into one box, a label would otherwise lie across them.
func TestLabelsAvoidTheCrowdedEndOfTheEdge(t *testing.T) {
	// Three edges converge on one box; the labelled one starts at a node of
	// its own, so its label belongs at the source end.
	crowdedTarget := diagram(
		[]string{"a", "b", "lonely"},
		[]string{"target"},
		[]string{"a->target", "b->target", "lonely->target"},
	)
	crowdedTarget.Edges[2].Label = "with a label"
	Layout(crowdedTarget, DefaultStyle())
	if !crowdedTarget.Edges[2].labelAtSource {
		t.Error("label should sit at the uncrowded source end")
	}

	// The other way round: one node fans out to three boxes, so the source end
	// is the crowded one.
	crowdedSource := diagram(
		[]string{"hub"},
		[]string{"t1", "t2", "t3"},
		[]string{"hub->t1", "hub->t2", "hub->t3"},
	)
	crowdedSource.Edges[2].Label = "with a label"
	Layout(crowdedSource, DefaultStyle())
	if crowdedSource.Edges[2].labelAtSource {
		t.Error("label should sit at the uncrowded target end")
	}
}

// An edge back to a system on the far side runs right to left, so its labelled
// run sits on the other side of the lanes. The gutter has to make room there,
// not on the side a left-to-right edge would use.
func TestReverseEdgeLabelGetsRoomOnItsOwnSide(t *testing.T) {
	st := DefaultStyle()
	d := diagram(
		[]string{"api", "comp"},
		[]string{"both", "other"},
		[]string{"both->api", "both->comp", "comp->both", "other->api"},
	)
	reverse := d.Edges[2] // comp -> both, pointing back to the left column
	reverse.Label = "/api/IHPT/ws/data/v2/stammdaten/"
	Layout(d, st)

	if reverse.dir != -1 {
		t.Fatalf("expected a right-to-left edge, got dir %v", reverse.dir)
	}
	run := math.Abs(reverse.x2 - reverse.trackX)
	if reverse.labelAtSource {
		run = math.Abs(reverse.trackX - reverse.x1)
	}
	if want := reverse.labelW + 2*st.EdgeLabelPadX; run < want {
		t.Errorf("labelled run of the reverse edge is %.1f wide, needs %.1f", run, want)
	}
}

func TestLabelsWrapOnExplicitNewlinesAndSpaces(t *testing.T) {
	st := DefaultStyle()
	// joinWrap puts explicit newlines between the parts of a reference label.
	got := wrapLabel("v2.1 · fetch user details\nread/write", st)
	if len(got) != 2 || got[0] != "v2.1 · fetch user details" || got[1] != "read/write" {
		t.Errorf("wrapLabel kept the wrong lines: %q", got)
	}
	// A single line that is too wide is broken at spaces.
	long := wrapLabel("With a veryveryvery long label that keeps going on", st)
	if len(long) < 2 {
		t.Errorf("expected an over-wide label to wrap, got %q", long)
	}
	if wrapLabel("", st) != nil {
		t.Error("an empty label should produce no lines")
	}
}

// Labels are drawn above their line, so they must not stick out of the top of
// the drawing.
func TestLabelsStayInsideTheDrawing(t *testing.T) {
	d := diagram(
		[]string{"comp"},
		[]string{"target"},
		[]string{"comp->target"},
	)
	d.Edges[0].Label = "a label on the topmost edge"
	Layout(d, DefaultStyle())

	e := d.Edges[0]
	if top := math.Min(e.y1, e.y2) - 5 - e.labelH; top < 0 {
		t.Errorf("label reaches y=%.1f, above the top of the drawing", top)
	}
}

func TestLayoutIsDeterministic(t *testing.T) {
	build := func() *Diagram {
		return diagram(
			[]string{"api", "comp", "res"},
			[]string{"x", "y", "z"},
			[]string{"x->api", "comp->y", "comp->z", "z->res", "y->api"},
		)
	}
	first := string(Render(build(), DefaultStyle()))
	for range 5 {
		if got := string(Render(build(), DefaultStyle())); got != first {
			t.Fatal("Render is not deterministic across runs")
		}
	}
}
