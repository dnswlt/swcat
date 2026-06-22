package catalog

import (
	"encoding/json"
	"fmt"
	"time"

	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
)

// ObservedDependencies is the catalog-domain representation of the dependencies
// a single external tool observed for a source entity. It is stored as the JSON
// value of the source entity's "swcat-deps/<detected_by>" status observation,
// and is the shape that downstream consumers (e.g. dependency validation)
// decode via ParseObservedDependencies.
//
// The source entity is implicit (it owns the observation) and is therefore not
// part of this struct.
type ObservedDependencies struct {
	// DetectedBy is the label of the external tool that detected the
	// dependencies. It also forms the observation key "swcat-deps/<DetectedBy>".
	DetectedBy string `json:"detectedBy"`
	// ObservedAt is when the dependencies were observed.
	ObservedAt time.Time `json:"observedAt"`
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

// ObservedDependenciesFromPB converts and validates a wire ObservedDependencies
// message into its catalog-domain form. It parses and validates every target
// reference (kind/name/namespace), but does not check that the referenced
// entities exist in the catalog. ObservedAt is left at its zero value when the
// message omits it; callers may default it (e.g. to time.Now()).
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
	var observedAt time.Time
	if pb.ObservedAt != nil {
		observedAt = pb.ObservedAt.AsTime()
	}
	return &ObservedDependencies{
		DetectedBy:   pb.DetectedBy,
		ObservedAt:   observedAt,
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
