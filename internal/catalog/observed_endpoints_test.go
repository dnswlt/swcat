package catalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
)

func TestObservedEndpointsFromPB(t *testing.T) {
	pb := &catalog_pb.ObservedEndpoints{
		Endpoints: []*catalog_pb.Endpoint{
			{
				Address:    " orders/${STAGE}/created ",
				Direction:  catalog_pb.EndpointDirection_ENDPOINT_DIRECTION_OUTBOUND,
				Technology: "solace",
				Names:      []string{"sendOrder-out-0"},
				Counterparts: []*catalog_pb.Counterpart{
					{Entity: &catalog_pb.Ref{Kind: "Component", Name: "billing"}, Address: "orders/*/created"},
					{External: "com.example:outside"},
				},
			},
			{
				Address:   "orders/${STAGE}/request",
				Direction: catalog_pb.EndpointDirection_ENDPOINT_DIRECTION_INBOUND,
				Kind:      catalog_pb.EndpointKind_ENDPOINT_KIND_REQUEST,
			},
		},
	}
	got, err := ObservedEndpointsFromPB(pb)
	if err != nil {
		t.Fatalf("ObservedEndpointsFromPB: %v", err)
	}
	want := &ObservedEndpoints{Endpoints: []ObservedEndpoint{
		{
			Address:    "orders/${STAGE}/created",
			Direction:  EndpointOutbound,
			Kind:       EndpointKindEvent, // unspecified defaults to event
			Technology: "solace",
			Names:      []string{"sendOrder-out-0"},
			Counterparts: []ObservedCounterpart{
				{Entity: MustParseRef("component:billing"), Address: "orders/*/created"},
				{External: "com.example:outside"},
			},
		},
		{
			Address:      "orders/${STAGE}/request",
			Direction:    EndpointInbound,
			Kind:         EndpointKindRequest,
			Counterparts: []ObservedCounterpart{},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		t.Errorf("got %s", gotJSON)
	}
}

func TestObservedEndpointsFromPB_Invalid(t *testing.T) {
	valid := func() *catalog_pb.Endpoint {
		return &catalog_pb.Endpoint{
			Address:   "a/topic",
			Direction: catalog_pb.EndpointDirection_ENDPOINT_DIRECTION_INBOUND,
		}
	}
	tests := []struct {
		name    string
		mutate  func(e *catalog_pb.Endpoint)
		wantErr string
	}{
		{"missing address", func(e *catalog_pb.Endpoint) { e.Address = " " }, "'address' is required"},
		{"missing direction", func(e *catalog_pb.Endpoint) { e.Direction = 0 }, "'direction' is required"},
		{"counterpart with neither entity nor external", func(e *catalog_pb.Endpoint) {
			e.Counterparts = []*catalog_pb.Counterpart{{Address: "x"}}
		}, "exactly one of"},
		{"counterpart with both entity and external", func(e *catalog_pb.Endpoint) {
			e.Counterparts = []*catalog_pb.Counterpart{{Entity: &catalog_pb.Ref{Kind: "Component", Name: "c"}, External: "x"}}
		}, "exactly one of"},
		{"counterpart with invalid entity ref", func(e *catalog_pb.Endpoint) {
			e.Counterparts = []*catalog_pb.Counterpart{{Entity: &catalog_pb.Ref{Kind: "Component", Name: "Not Valid!"}}}
		}, "invalid entity reference"},
		// protojson accepts unknown numeric enum values.
		{"unknown direction", func(e *catalog_pb.Endpoint) { e.Direction = 99 }, "unknown 'direction'"},
		{"unknown kind", func(e *catalog_pb.Endpoint) { e.Kind = 99 }, "unknown 'kind'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := valid()
			tt.mutate(e)
			_, err := ObservedEndpointsFromPB(&catalog_pb.ObservedEndpoints{Endpoints: []*catalog_pb.Endpoint{e}})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

// Stored values are validated too: the generic observations API accepts any JSON
// under a "swcat-endpoints/*" key.
func TestParseObservedEndpoints_Invalid(t *testing.T) {
	tests := []struct {
		name, value, wantErr string
	}{
		{"not JSON", `{`, "invalid observed endpoints value"},
		{"missing address", `{"endpoints":[{"direction":"inbound","kind":"event"}]}`, "'address' is required"},
		{"unknown direction", `{"endpoints":[{"address":"a","direction":"sideways","kind":"event"}]}`, "invalid 'direction'"},
		{"missing kind", `{"endpoints":[{"address":"a","direction":"inbound"}]}`, "invalid 'kind'"},
		{"counterpart with neither", `{"endpoints":[{"address":"a","direction":"inbound","kind":"event",
			"counterparts":[{"address":"x"}]}]}`, "exactly one of"},
		{"counterpart with unknown entity kind", `{"endpoints":[{"address":"a","direction":"inbound","kind":"event",
			"counterparts":[{"entity":{"kind":"Banana","name":"target"}}]}]}`, "invalid entity reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseObservedEndpoints(json.RawMessage(tt.value))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseObservedEndpoints_RoundTrip(t *testing.T) {
	in := &ObservedEndpoints{Endpoints: []ObservedEndpoint{{
		Address: "a", Direction: EndpointOutbound, Kind: EndpointKindRequest,
		Counterparts: []ObservedCounterpart{{Entity: MustParseRef("component:b")}, {External: "g:x"}},
	}}}
	value, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseObservedEndpoints(value)
	if err != nil {
		t.Fatalf("ParseObservedEndpoints: %v", err)
	}
	if !reflect.DeepEqual(got, in) {
		t.Errorf("got %+v, want %+v", got, in)
	}
}

func TestObservedEndpointSourceDependsOnCounterparts(t *testing.T) {
	tests := []struct {
		direction, kind string
		want            bool
	}{
		{EndpointInbound, EndpointKindEvent, true},    // consumes their events
		{EndpointOutbound, EndpointKindRequest, true}, // calls them
		{EndpointOutbound, EndpointKindEvent, false},  // they consume our events
		{EndpointInbound, EndpointKindRequest, false}, // they call us
	}
	for _, tt := range tests {
		e := &ObservedEndpoint{Direction: tt.direction, Kind: tt.kind}
		if got := e.SourceDependsOnCounterparts(); got != tt.want {
			t.Errorf("%s %s: got %v, want %v", tt.direction, tt.kind, got, tt.want)
		}
	}
}
