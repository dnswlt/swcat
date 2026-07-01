package catalog

import (
	"encoding/json"
	"fmt"

	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
)

// ObservedDependencies is the catalog-domain representation of the dependencies
// a single external tool observed for a source entity. It is stored as the JSON
// value of the source entity's "swcat-deps/<detected_by>" status observation,
// and is the shape that downstream consumers (e.g. dependency validation)
// decode via ParseObservedDependencies.
//
// This struct holds only the dependency payload. The observation envelope owns
// the rest: the source entity owns the observation, the detecting tool lives in
// Observation.Producer (the detected_by value), and the observation time in
// Observation.UpdatedAt.
type ObservedDependencies struct {
	// Dependencies are the observed dependencies of the source entity.
	Dependencies []ObservedDependency `json:"dependencies"`
}

// ObservedDependency is a single machine-detected dependency from a source
// entity to Target.
type ObservedDependency struct {
	// Target is the entity that the source entity depends on.
	Target *Ref `json:"target"`
	// Relation classifies the dependency: one of "dependency" (generic),
	// "calls", "produces" or "consumes".
	Relation string `json:"relation"`
	// Evidence is the detail on which the detection was based, e.g. topic names
	// or an RPC Service.Method name.
	Evidence []string `json:"evidence,omitempty"`
}

// ObservedDependenciesFromPB converts and validates the dependency payload of a
// wire ObservedDependencies message into its catalog-domain form. It parses and
// validates every target reference (kind/name/namespace), but does not check
// that the referenced entities exist in the catalog. Envelope fields
// (detected_by, observed_at) are handled by the caller, which maps them onto the
// enclosing Observation.
func ObservedDependenciesFromPB(pb *catalog_pb.ObservedDependencies) (*ObservedDependencies, error) {
	if pb == nil {
		return nil, fmt.Errorf("nil ObservedDependencies")
	}
	deps := make([]ObservedDependency, 0, len(pb.Dependencies))
	for i, d := range pb.Dependencies {
		target, err := RefFromPB(d.Target)
		if err != nil {
			return nil, fmt.Errorf("dependencies[%d]: invalid target reference: %w", i, err)
		}
		deps = append(deps, ObservedDependency{
			Target:   target,
			Relation: DependencyRelationLabel(d.Relation),
			Evidence: d.Evidence,
		})
	}
	return &ObservedDependencies{
		Dependencies: deps,
	}, nil
}

// ParseObservedDependencies decodes the JSON value of a "swcat-deps/*" status
// observation into its catalog-domain form. It is the inverse of marshalling an
// ObservedDependencies as a status observation value.
func ParseObservedDependencies(value json.RawMessage) (*ObservedDependencies, error) {
	var od ObservedDependencies
	if err := json.Unmarshal(value, &od); err != nil {
		return nil, fmt.Errorf("invalid observed dependencies value: %w", err)
	}
	return &od, nil
}
