package catalog

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
)

// Endpoint directions and kinds, in their stored (short, stable) string form.
const (
	EndpointInbound  = "inbound"
	EndpointOutbound = "outbound"

	EndpointKindEvent   = "event"
	EndpointKindRequest = "request"
)

// ObservedEndpoints is the catalog-domain representation of the endpoints a
// single external tool observed for a source entity: how the entity
// communicates, and with whom. It is stored as the JSON value of the source
// entity's "swcat-endpoints/<detected_by>" status observation, and is the shape
// that downstream consumers (dependency validation, the entity page) decode via
// ParseObservedEndpoints.
//
// This struct holds only the endpoint payload. The observation envelope owns
// the rest: the source entity owns the observation, the detecting tool lives in
// Observation.Producer (the detected_by value), and the observation time in
// Observation.UpdatedAt.
type ObservedEndpoints struct {
	// Endpoints are the observed endpoints of the source entity.
	Endpoints []ObservedEndpoint `json:"endpoints"`
}

// ObservedEndpoint is a single communication endpoint of the source entity,
// e.g. a topic it publishes to, and the counterparts it connects to.
type ObservedEndpoint struct {
	// Address as configured, e.g. a topic "orders/${STAGE}/created".
	Address string `json:"address"`
	// Direction is EndpointInbound or EndpointOutbound, from the source's view.
	Direction string `json:"direction"`
	// Kind is EndpointKindEvent or EndpointKindRequest.
	Kind string `json:"kind"`
	// Technology is free text, e.g. "solace" or "kafka". Optional.
	Technology string `json:"technology,omitempty"`
	// Names are tool-specific identifiers, e.g. binding names. Optional.
	Names []string `json:"names,omitempty"`
	// Counterparts are the other ends this endpoint connects to.
	Counterparts []ObservedCounterpart `json:"counterparts,omitempty"`
}

// ObservedCounterpart is the other end of an observed connection: a catalog
// entity, or an External name for one that is not in the catalog.
type ObservedCounterpart struct {
	Entity   *Ref   `json:"entity,omitempty"`
	External string `json:"external,omitempty"`
	// Address is the counterpart's own matching address, if it differs from
	// the endpoint's. Optional.
	Address string `json:"address,omitempty"`
}

// SourceDependsOnCounterparts reports whether the source entity depends on the
// counterparts of this endpoint: it consumes their events (inbound events), or
// calls them (outbound requests). Otherwise the counterparts depend on it.
func (e *ObservedEndpoint) SourceDependsOnCounterparts() bool {
	if e.Kind == EndpointKindRequest {
		return e.Direction == EndpointOutbound
	}
	return e.Direction == EndpointInbound
}

// ObservedEndpointsFromPB converts and validates the endpoint payload of a wire
// ObservedEndpoints message into its catalog-domain form (see
// validateObservedEndpoints). It does not check that the referenced entities
// exist in the catalog. Envelope fields (detected_by, observed_at, ...) are
// handled by the caller, which maps them onto the enclosing Observation.
func ObservedEndpointsFromPB(pb *catalog_pb.ObservedEndpoints) (*ObservedEndpoints, error) {
	if pb == nil {
		return nil, fmt.Errorf("nil ObservedEndpoints")
	}
	endpoints := make([]ObservedEndpoint, 0, len(pb.Endpoints))
	for i, e := range pb.Endpoints {
		ep, err := observedEndpointFromPB(e)
		if err != nil {
			return nil, fmt.Errorf("endpoints[%d]: %w", i, err)
		}
		endpoints = append(endpoints, ep)
	}
	oe := &ObservedEndpoints{Endpoints: endpoints}
	if err := validateObservedEndpoints(oe); err != nil {
		return nil, err
	}
	return oe, nil
}

// observedEndpointFromPB converts a wire Endpoint. It only rejects what the
// domain form cannot represent (unknown enum values, malformed references);
// the domain invariants are checked by validateObservedEndpoints.
func observedEndpointFromPB(e *catalog_pb.Endpoint) (ObservedEndpoint, error) {
	var direction string
	switch e.GetDirection() {
	case catalog_pb.EndpointDirection_ENDPOINT_DIRECTION_INBOUND:
		direction = EndpointInbound
	case catalog_pb.EndpointDirection_ENDPOINT_DIRECTION_OUTBOUND:
		direction = EndpointOutbound
	case catalog_pb.EndpointDirection_ENDPOINT_DIRECTION_UNSPECIFIED:
		return ObservedEndpoint{}, fmt.Errorf("'direction' is required")
	default:
		return ObservedEndpoint{}, fmt.Errorf("unknown 'direction' %d", e.GetDirection())
	}
	var kind string
	switch e.GetKind() {
	case catalog_pb.EndpointKind_ENDPOINT_KIND_UNSPECIFIED, catalog_pb.EndpointKind_ENDPOINT_KIND_EVENT:
		kind = EndpointKindEvent
	case catalog_pb.EndpointKind_ENDPOINT_KIND_REQUEST:
		kind = EndpointKindRequest
	default:
		return ObservedEndpoint{}, fmt.Errorf("unknown 'kind' %d", e.GetKind())
	}
	counterparts := make([]ObservedCounterpart, 0, len(e.GetCounterparts()))
	for j, c := range e.GetCounterparts() {
		cp := ObservedCounterpart{
			External: strings.TrimSpace(c.GetExternal()),
			Address:  strings.TrimSpace(c.GetAddress()),
		}
		if c.GetEntity() != nil {
			ref, err := RefFromPB(c.GetEntity())
			if err != nil {
				return ObservedEndpoint{}, fmt.Errorf("counterparts[%d]: invalid entity reference: %w", j, err)
			}
			cp.Entity = ref
		}
		counterparts = append(counterparts, cp)
	}
	return ObservedEndpoint{
		Address:      strings.TrimSpace(e.GetAddress()),
		Direction:    direction,
		Kind:         kind,
		Technology:   strings.TrimSpace(e.GetTechnology()),
		Names:        e.GetNames(),
		Counterparts: counterparts,
	}, nil
}

// ParseObservedEndpoints decodes and validates the JSON value of a
// "swcat-endpoints/*" status observation into its catalog-domain form. It is
// the inverse of marshalling an ObservedEndpoints as a status observation
// value. Stored values are validated too, because the generic observations
// API accepts any JSON under any key.
func ParseObservedEndpoints(value json.RawMessage) (*ObservedEndpoints, error) {
	var oe ObservedEndpoints
	if err := json.Unmarshal(value, &oe); err != nil {
		return nil, fmt.Errorf("invalid observed endpoints value: %w", err)
	}
	if err := validateObservedEndpoints(&oe); err != nil {
		return nil, fmt.Errorf("invalid observed endpoints value: %w", err)
	}
	return &oe, nil
}

// validateObservedEndpoints checks the domain invariants of oe: every
// endpoint has an address, a known direction and kind, and every counterpart
// is exactly one of a valid entity reference or an external name.
func validateObservedEndpoints(oe *ObservedEndpoints) error {
	for i, ep := range oe.Endpoints {
		if err := validateObservedEndpoint(&ep); err != nil {
			return fmt.Errorf("endpoints[%d]: %w", i, err)
		}
	}
	return nil
}

func validateObservedEndpoint(ep *ObservedEndpoint) error {
	if strings.TrimSpace(ep.Address) == "" {
		return fmt.Errorf("'address' is required")
	}
	if ep.Direction != EndpointInbound && ep.Direction != EndpointOutbound {
		return fmt.Errorf("invalid 'direction' %q", ep.Direction)
	}
	if ep.Kind != EndpointKindEvent && ep.Kind != EndpointKindRequest {
		return fmt.Errorf("invalid 'kind' %q", ep.Kind)
	}
	for j, cp := range ep.Counterparts {
		if (cp.Entity == nil) == (cp.External == "") {
			return fmt.Errorf("counterparts[%d]: exactly one of 'entity' and 'external' must be set", j)
		}
		if cp.Entity != nil {
			if _, err := NewRefFromAPI(&api.Ref{
				Kind: string(cp.Entity.Kind), Namespace: cp.Entity.Namespace, Name: cp.Entity.Name,
			}); err != nil {
				return fmt.Errorf("counterparts[%d]: invalid entity reference: %w", j, err)
			}
		}
	}
	return nil
}
