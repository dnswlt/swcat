package svg

import (
	"context"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/sysview"
)

// TestExternalViewGeometry checks what the layout of an external view promises
// whatever it is given: that an arrow ends inside the box it meets, that a box
// with a side to itself is met near the middle of it, and that two arrows never
// run the same gutter at the same height. Those hold or there is a bug.
//
// How good the result looks is a different question, and not one a catalog can
// answer with a pass: a box whose ports are packed as tightly as they go leaves
// its arrows nowhere to attach but a few points off, and no rule of the layout
// can promise otherwise. So the shapes that come of that — short steps, and
// pairs of them interlocking into a squiggle — are counted and logged rather
// than asserted. Run the test over a catalog before and after a change to
// internal/sysview and compare the counts; the log names the views to look at.
//
// It runs over the example catalog by default and over whatever
// SWCAT_CATALOG_DIR points at otherwise, which is how a layout change gets
// judged on more than the example.
//
// The geometry is read back out of the rendered SVG on purpose. It is what the
// browser draws, and it is all the layout is for.
func TestExternalViewGeometry(t *testing.T) {
	dir := os.Getenv("SWCAT_CATALOG_DIR")
	if dir == "" {
		dir = "../../examples/flights"
	}
	renderer, r := externalRenderer(t, dir)
	st := sysview.DefaultStyle()

	var systems []*catalog.System
	for _, e := range r.AllEntities() {
		if sys, ok := e.(*catalog.System); ok {
			systems = append(systems, sys)
		}
	}
	if len(systems) == 0 {
		t.Fatalf("no systems in %s", dir)
	}

	details := []struct {
		name  string
		level DetailLevel
	}{{"systems", DetailSystems}, {"apis", DetailAPIs}, {"all", DetailAll}}

	views, arrows, bends, wobbles, squiggles := 0, 0, 0, 0, 0
	for _, sys := range systems {
		for _, d := range details {
			res, err := renderer.SystemExternalGraph(context.Background(), sys, NewSystemViewOptions(nil, d.level))
			if err != nil {
				t.Fatalf("%s at detail=%s: %v", sys.GetRef(), d.name, err)
			}
			view := fmt.Sprintf("%s?detail=%s", sys.GetRef().QName(), d.name)
			edges := readGeometry(string(res.SVG))
			views++
			arrows += len(edges)
			for _, e := range edges {
				if e.bends {
					bends++
				}
			}

			ports := map[string]int{} // how many arrows meet each side of each box
			for _, e := range edges {
				for _, p := range e.ends {
					if p.box != nil {
						ports[p.key()]++
					}
				}
			}

			for _, e := range edges {
				for _, p := range e.ends {
					if p.box == nil {
						t.Errorf("%s: %s has an end that meets no box at (%.1f, %.1f)",
							view, e.id, p.x, p.y)
						continue
					}
					// An arrow meets a box between its corners.
					lo, hi := p.box.y+st.PortInset, p.box.y+p.box.h-st.PortInset
					if p.y < lo-0.01 || p.y > hi+0.01 {
						t.Errorf("%s: %s meets %s at y=%.1f, outside [%.1f, %.1f]",
							view, e.id, p.box.id, p.y, lo, hi)
					}
					// A box with the side to itself is met in the middle, give
					// or take the slack that keeps two arrows apart.
					if !p.box.cluster && ports[p.key()] == 1 {
						if off := math.Abs(p.y - p.box.centerY()); off > st.PortSlack+0.01 {
							t.Errorf("%s: %s meets %s %.1fpt off its middle, more than the %.1fpt slack",
								view, e.id, p.box.id, off, st.PortSlack)
						}
					}
				}
			}

			// An arrow that has to change height should look like it meant to:
			// a step smaller than the distance between two ports reads as a
			// wobble in the line rather than as a route.
			for _, e := range edges {
				if !e.bends {
					continue
				}
				if drop := math.Abs(e.ends[0].y - e.ends[1].y); drop < st.MinPortPitch-0.01 {
					wobbles++
					t.Logf("%s: %s steps only %.1fpt", view, e.id, drop)
				}
			}

			// Two arrows that both step aside over the same stretch of gutter
			// interlock into a single squiggle: the shape that started all
			// this. One arrow stepping past another is fine; two stepping over
			// each other is not.
			for i, a := range edges {
				for _, b := range edges[i+1:] {
					if !a.bends || !b.bends || !a.overlapsX(b) {
						continue
					}
					aStep := math.Abs(a.ends[0].y - a.ends[1].y)
					bStep := math.Abs(b.ends[0].y - b.ends[1].y)
					if aStep >= st.MinPortPitch || bStep >= st.MinPortPitch {
						continue
					}
					aLo, aHi := math.Min(a.ends[0].y, a.ends[1].y), math.Max(a.ends[0].y, a.ends[1].y)
					bLo, bHi := math.Min(b.ends[0].y, b.ends[1].y), math.Max(b.ends[0].y, b.ends[1].y)
					if aHi > bLo-st.MinPortPitch && bHi > aLo-st.MinPortPitch {
						squiggles++
						t.Logf("%s: %s and %s both step aside over the same stretch, by %.1f and %.1fpt",
							view, a.id, b.id, aStep, bStep)
					}
				}
			}

			// Two arrows that run level in the same gutter have to stay apart,
			// or they read as one arrow with a head at each end.
			for i, a := range edges {
				for _, b := range edges[i+1:] {
					if !a.isLevel() || !b.isLevel() || !a.overlapsX(b) {
						continue
					}
					if gap := math.Abs(a.ends[0].y - b.ends[0].y); gap < st.MinPortPitch-0.01 {
						t.Errorf("%s: %s and %s both run the gutter %.1fpt apart, want %.1f",
							view, a.id, b.id, gap, st.MinPortPitch)
					}
				}
			}
		}
	}
	t.Logf("%s: %d views, %d arrows, %d with a bend, %d of those shorter than a port pitch, %d pairs of them interlocking",
		dir, views, arrows, bends, wobbles, squiggles)
}

type geomBox struct {
	id         string
	x, y, w, h float64
	cluster    bool
}

func (b geomBox) centerY() float64 { return b.y + b.h/2 }

type geomPort struct {
	box  *geomBox
	side int
	x, y float64
}

func (p geomPort) key() string { return fmt.Sprintf("%s/%d", p.box.id, p.side) }

type geomEdge struct {
	id    string
	bends bool
	ends  [2]geomPort
}

func (e geomEdge) isLevel() bool { return !e.bends && math.Abs(e.ends[0].y-e.ends[1].y) < 0.01 }

func (e geomEdge) overlapsX(o geomEdge) bool {
	lo := math.Max(math.Min(e.ends[0].x, e.ends[1].x), math.Min(o.ends[0].x, o.ends[1].x))
	hi := math.Min(math.Max(e.ends[0].x, e.ends[1].x), math.Max(o.ends[0].x, o.ends[1].x))
	return hi-lo > 1
}

var (
	reGeomBox   = regexp.MustCompile(`(?s)<g id="([^"]+)" class="(node clickable-node|cluster [^"]*)">\s*<rect[^>]*x="([-0-9.]+)" y="([-0-9.]+)" width="([-0-9.]+)" height="([-0-9.]+)"`)
	reGeomEdge  = regexp.MustCompile(`(?s)<g id="(svg-edge-\d+)" class="edge[^"]*">\s*<path class="edge-hit"[^>]*d="([^"]+)"`)
	reGeomPoint = regexp.MustCompile(`([-0-9.]+),([-0-9.]+)`)
)

// readGeometry recovers the arrows of a rendered view, each end matched to the
// box it meets.
func readGeometry(svg string) []geomEdge {
	var boxes []*geomBox
	for _, m := range reGeomBox.FindAllStringSubmatch(svg, -1) {
		boxes = append(boxes, &geomBox{
			id: m[1], cluster: strings.HasPrefix(m[2], "cluster"),
			x: geomNum(m[3]), y: geomNum(m[4]), w: geomNum(m[5]), h: geomNum(m[6]),
		})
	}
	var edges []geomEdge
	for _, m := range reGeomEdge.FindAllStringSubmatch(svg, -1) {
		pts := reGeomPoint.FindAllStringSubmatch(m[2], -1)
		if len(pts) < 2 {
			continue
		}
		e := geomEdge{id: m[1], bends: strings.Contains(m[2], "Q")}
		// The drawn line stops short of the box it points into: the arrow head
		// covers the last stretch.
		reach := sysview.DefaultStyle().ArrowLength + 0.01
		for i, pt := range [][2]string{{pts[0][1], pts[0][2]}, {pts[len(pts)-1][1], pts[len(pts)-1][2]}} {
			x, y := geomNum(pt[0]), geomNum(pt[1])
			p := geomPort{x: x, y: y, side: -1}
			// Boxes inside a cluster sit on top of it, so a node wins over the
			// cluster around it when both have a side within reach.
			best := math.Inf(1)
			for _, b := range boxes {
				if y < b.y-0.01 || y > b.y+b.h+0.01 {
					continue
				}
				for side, edgeX := range []float64{b.x, b.x + b.w} {
					d := math.Abs(x - edgeX)
					if d > reach {
						continue
					}
					if p.box != nil && b.cluster && !p.box.cluster {
						continue
					}
					if d < best || (p.box != nil && p.box.cluster && !b.cluster) {
						p.box, p.side, best = b, side, d
					}
				}
			}
			e.ends[i] = p
		}
		edges = append(edges, e)
	}
	return edges
}

func geomNum(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
