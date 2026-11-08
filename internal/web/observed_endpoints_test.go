package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/store"
)

type mapResolver map[string]catalog.Entity

func (m mapResolver) Entity(ref *catalog.Ref) catalog.Entity { return m[ref.String()] }

func setObservedEndpoints(t *testing.T, c *catalog.Component, tool string, eps ...catalog.ObservedEndpoint) {
	t.Helper()
	val, err := json.Marshal(catalog.ObservedEndpoints{Endpoints: eps})
	if err != nil {
		t.Fatalf("marshal observed endpoints: %v", err)
	}
	if !catalog.MergeObservations(c, map[string]catalog.Observation{
		catalog.ObservedEndpointsKeyPrefix + "/" + tool: {Value: val, Producer: tool},
	}) {
		t.Fatal("MergeObservations returned false")
	}
}

func counterpart(ref string) catalog.ObservedCounterpart {
	return catalog.ObservedCounterpart{Entity: catalog.MustParseRef(ref)}
}

func TestObservedEndpointsSections(t *testing.T) {
	c := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "c"},
		Spec: &catalog.ComponentSpec{
			DependsOn: []*catalog.LabelRef{{Ref: catalog.MustParseRef("component:declared")}},
		},
	}
	setObservedEndpoints(t, c, "solace-graph",
		catalog.ObservedEndpoint{Address: "b/out", Direction: catalog.EndpointOutbound, Kind: catalog.EndpointKindEvent,
			Counterparts: []catalog.ObservedCounterpart{
				{External: "aaa.external:first-by-name"}, counterpart("component:zeta"), counterpart("component:declared"),
			}},
		catalog.ObservedEndpoint{Address: "a/request", Direction: catalog.EndpointInbound, Kind: catalog.EndpointKindRequest,
			Counterparts: []catalog.ObservedCounterpart{{External: "com.example:outside"}}},
		catalog.ObservedEndpoint{Address: "b/out", Direction: catalog.EndpointInbound, Kind: catalog.EndpointKindEvent,
			Counterparts: []catalog.ObservedCounterpart{
				{Entity: catalog.MustParseRef("component:zeta"), Address: "b/*"},
				{Entity: catalog.MustParseRef("component:zeta"), Address: "b/+"},
				counterpart("resource:db"),
				counterpart("resource:zeta"), // same name as component:zeta, a different counterpart
				counterpart("component:c"),   // c itself: needs no declaration
			}},
		catalog.ObservedEndpoint{Address: "c/unmatched", Direction: catalog.EndpointOutbound, Kind: catalog.EndpointKindRequest},
	)

	sections := observedEndpointsSections(c, mapResolver{})
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	sec := sections[0]
	if sec.Tool != "solace-graph" {
		t.Errorf("Tool = %q", sec.Tool)
	}

	// Rows sorted by address, then inbound before outbound.
	var rows []string
	for _, r := range sec.Rows {
		var cps []string
		for _, cp := range r.Counterparts {
			name := cp.displayName()
			if cp.Undeclared {
				name += "!"
			}
			cps = append(cps, name)
		}
		rows = append(rows, r.Address+" "+r.Mode+" ["+strings.Join(cps, ", ")+"]")
		if r.ModeTitle == "" {
			t.Errorf("row %s has no ModeTitle", r.Address)
		}
	}
	want := []string{
		// "!" marks undeclared counterparts.
		"a/request reply [com.example:outside]",
		// component:zeta was reported twice (two matching addresses of its
		// own): listed once. resource:zeta is a different counterpart.
		"b/out in [c, db, zeta!, zeta]",
		// Catalog entities first, external ones last, whatever their names.
		"b/out out [declared, zeta!, aaa.external:first-by-name]",
		"c/unmatched request []",
	}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Errorf("rows:\n%s\nwant:\n%s", strings.Join(rows, "\n"), strings.Join(want, "\n"))
	}
	if sec.Undeclared != 2 {
		t.Errorf("Undeclared = %d, want 2", sec.Undeclared)
	}
	if !sec.Open {
		t.Error("section with undeclared counterparts should be open")
	}
}

func TestObservedEndpointsSections_LargeDeclaredTableCollapsed(t *testing.T) {
	c := &catalog.Component{Metadata: &catalog.Metadata{Name: "c"}, Spec: &catalog.ComponentSpec{}}
	var eps []catalog.ObservedEndpoint
	for i := 0; i <= observedEndpointsOpenMaxRows; i++ {
		eps = append(eps, catalog.ObservedEndpoint{
			Address: "topic/" + string(rune('a'+i)), Direction: catalog.EndpointOutbound, Kind: catalog.EndpointKindEvent,
		})
	}
	setObservedEndpoints(t, c, "solace-graph", eps...)
	if sec := observedEndpointsSections(c, mapResolver{})[0]; sec.Open {
		t.Errorf("table with %d rows and no undeclared counterparts should start collapsed", len(sec.Rows))
	}
}

func TestComponentDetail_ShowsObservedEndpoints(t *testing.T) {
	s := newTestServer(t, store.NewDiskStore("../../testdata/test1"))
	data, err := s.loadStoreData("")
	if err != nil {
		t.Fatalf("loadStoreData: %v", err)
	}
	c := data.repo.Component(catalog.MustParseRef("component:test-component"))
	if c == nil {
		t.Fatal("component test-component not found")
	}
	setObservedEndpoints(t, c, "solace-graph",
		catalog.ObservedEndpoint{Address: "orders/${STAGE}/created", Direction: catalog.EndpointOutbound,
			Kind: catalog.EndpointKindEvent, Counterparts: []catalog.ObservedCounterpart{counterpart("component:billing")}},
		catalog.ObservedEndpoint{Address: "audit/events", Direction: catalog.EndpointInbound,
			Kind: catalog.EndpointKindEvent, Counterparts: []catalog.ObservedCounterpart{{External: "com.example:outside"}}},
	)

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ui/components/test-component", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	body := rr.Body.String()
	for _, want := range []string{"Observed endpoints", "detected by solace-graph", "1 not declared",
		"orders/${STAGE}/created", "billing", "com.example:outside"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	// Sorted by address: audit/... before orders/...
	if strings.Index(body, "audit/events") > strings.Index(body, "orders/${STAGE}/created") {
		t.Error("rows are not sorted by address")
	}
}

// A malformed stored value (the generic observations API accepts any JSON)
// shows as the section's error, not as a broken page.
func TestComponentDetail_MalformedObservedEndpoints(t *testing.T) {
	s := newTestServer(t, store.NewDiskStore("../../testdata/test1"))
	data, err := s.loadStoreData("")
	if err != nil {
		t.Fatalf("loadStoreData: %v", err)
	}
	c := data.repo.Component(catalog.MustParseRef("component:test-component"))
	if c == nil {
		t.Fatal("component test-component not found")
	}
	value := `{"endpoints":[{"address":"a","direction":"inbound","kind":"event",
		"counterparts":[{"entity":{"kind":"Banana","name":"target"}}]}]}`
	if !catalog.MergeObservations(c, map[string]catalog.Observation{
		catalog.ObservedEndpointsKeyPrefix + "/solace-graph": {Value: json.RawMessage(value), Producer: "solace-graph"},
	}) {
		t.Fatal("MergeObservations returned false")
	}

	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ui/components/test-component", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if body := rr.Body.String(); !strings.Contains(body, "invalid entity reference") {
		t.Error("page does not show the decoding error")
	}
}
