package web

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
)

// observedEndpointsOpenMaxRows is the largest table that is expanded on page
// load even if all its counterparts are declared. Larger tables start
// collapsed, unless they contain an undeclared counterpart.
const observedEndpointsOpenMaxRows = 20

// observedEndpointsSection is the view model of the endpoints one tool observed
// for an entity: one table on the entity page.
type observedEndpointsSection struct {
	Tool      string
	UpdatedAt time.Time
	Version   string
	// Rows are sorted by address, so the table reads as an a-z list of what
	// the entity uses.
	Rows []*observedEndpointRow
	// Undeclared counts the undeclared counterparts.
	Undeclared int
	Open       bool
	// Err is set if the observation could not be decoded; Rows is empty then.
	Err string
}

type observedEndpointRow struct {
	Address string
	// Mode is how the entity uses the endpoint, one of the endpointMode*
	// constants; the table shows it as an icon.
	Mode string
	// ModeTitle explains Mode, as the icon's tooltip.
	ModeTitle    string
	Technology   string
	Names        []string
	Counterparts []*observedCounterpartView
}

// How an entity uses an endpoint, combining direction and kind.
const (
	endpointModeIn      = "in"      // subscribes to events
	endpointModeOut     = "out"     // publishes events
	endpointModeRequest = "request" // sends requests, awaits replies
	endpointModeReply   = "reply"   // serves requests
)

var endpointModeTitles = map[string]string{
	endpointModeIn:      "Inbound: receives events published to this address",
	endpointModeOut:     "Outbound: publishes events to this address",
	endpointModeRequest: "Outbound request: sends requests to this address and awaits the replies",
	endpointModeReply:   "Inbound request: serves requests sent to this address",
}

// endpointModeRank orders the modes for the same address: inbound first.
var endpointModeRank = map[string]int{
	endpointModeIn: 0, endpointModeReply: 1, endpointModeOut: 2, endpointModeRequest: 3,
}

type observedCounterpartView struct {
	// Ref is the catalog entity, or nil for an external counterpart.
	Ref *catalog.Ref
	// External is the display name of a counterpart outside the catalog.
	External string
	// Undeclared is set for a Component the catalog declares no relationship
	// with (see catalog.DeclaredNeighbors).
	Undeclared bool
}

// observedEndpointsSections builds one section per tool that reported observed
// endpoints for c, sorted by tool name. The status of each counterpart is
// derived from the catalog via resolver.
func observedEndpointsSections(c *catalog.Component, resolver catalog.Resolver) []*observedEndpointsSection {
	status := c.GetStatus()
	if status == nil {
		return nil
	}
	self := c.GetRef().String()
	var neighbors map[string]bool // computed lazily, only if there is a section
	var sections []*observedEndpointsSection
	for key, obs := range status.Observations {
		if !strings.HasPrefix(key, catalog.ObservedEndpointsKeyPrefix+"/") {
			continue
		}
		sec := &observedEndpointsSection{
			Tool:      strings.TrimPrefix(key, catalog.ObservedEndpointsKeyPrefix+"/"),
			UpdatedAt: obs.UpdatedAt,
			Version:   obs.Version,
		}
		sections = append(sections, sec)
		oe, err := catalog.ParseObservedEndpoints(obs.Value)
		if err != nil {
			sec.Err = err.Error()
			continue
		}
		if neighbors == nil {
			neighbors = catalog.DeclaredNeighbors(c, resolver)
		}
		for _, ep := range oe.Endpoints {
			row := &observedEndpointRow{
				Address:    ep.Address,
				Mode:       endpointMode(&ep),
				Technology: ep.Technology,
				Names:      ep.Names,
			}
			row.ModeTitle = endpointModeTitles[row.Mode]
			for _, cp := range ep.Counterparts {
				// Only Components can be declared neighbors (see
				// catalog.DeclaredNeighbors), and talking to itself needs no
				// declaration (cf. the lint).
				undeclared := cp.Entity != nil && cp.Entity.Kind == catalog.KindComponent &&
					cp.Entity.String() != self && !neighbors[cp.Entity.String()]
				row.Counterparts = append(row.Counterparts,
					&observedCounterpartView{Ref: cp.Entity, External: cp.External, Undeclared: undeclared})
			}
			// Catalog entities first, by the name shown; external ones last.
			// The same counterpart may be reported once per matching address
			// of its own, which the table does not show: list it once.
			slices.SortFunc(row.Counterparts, func(a, b *observedCounterpartView) int {
				return cmp.Or(
					cmp.Compare(boolRank(a.Ref == nil), boolRank(b.Ref == nil)),
					cmp.Compare(a.displayName(), b.displayName()),
					cmp.Compare(a.key(), b.key()), // same name, different kinds
				)
			})
			row.Counterparts = slices.CompactFunc(row.Counterparts, func(a, b *observedCounterpartView) bool {
				return a.key() == b.key()
			})
			for _, v := range row.Counterparts {
				if v.Undeclared {
					sec.Undeclared++
				}
			}
			sec.Rows = append(sec.Rows, row)
		}
		slices.SortFunc(sec.Rows, func(a, b *observedEndpointRow) int {
			return cmp.Or(
				cmp.Compare(a.Address, b.Address),
				cmp.Compare(endpointModeRank[a.Mode], endpointModeRank[b.Mode]),
			)
		})
		sec.Open = sec.Undeclared > 0 || len(sec.Rows) <= observedEndpointsOpenMaxRows
	}
	slices.SortFunc(sections, func(a, b *observedEndpointsSection) int {
		return cmp.Compare(a.Tool, b.Tool)
	})
	return sections
}

// endpointMode combines the endpoint's direction and kind into one of the
// endpointMode* constants.
func endpointMode(ep *catalog.ObservedEndpoint) string {
	inbound := ep.Direction == catalog.EndpointInbound
	switch {
	case ep.Kind == catalog.EndpointKindRequest && inbound:
		return endpointModeReply
	case ep.Kind == catalog.EndpointKindRequest:
		return endpointModeRequest
	case inbound:
		return endpointModeIn
	default:
		return endpointModeOut
	}
}

// displayName is the name the table shows for the counterpart.
func (v *observedCounterpartView) displayName() string {
	if v.Ref != nil {
		return v.Ref.QName()
	}
	return v.External
}

// key identifies the counterpart: the full entity reference (including its
// kind, which displayName omits), or the external name.
func (v *observedCounterpartView) key() string {
	if v.Ref != nil {
		return v.Ref.String()
	}
	return "external:" + v.External
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}
