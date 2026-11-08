package lint

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
)

// DependencyCandidateRuleName is the rule name used for findings produced by the
// dependency-candidate check.
const DependencyCandidateRuleName = "dependency-candidate"

// maxEvidenceSamples caps how many evidence entries a single finding lists, so
// the message stays readable when many were reported for one dependency.
const maxEvidenceSamples = 3

// checkDependencyCandidates reports observed dependencies of a Component that
// are not reflected in the catalog.
//
// For the given component C it builds the undirected set of its declared
// Component neighbors (see catalog.DeclaredNeighbors). It then walks C's observed
// endpoints (the "swcat-endpoints/*" status observations) and emits a finding
// for every Component counterpart C depends on that is not among those
// neighbors. C depends on the counterparts of its inbound event endpoints (it
// consumes their events) and of its outbound request endpoints (it calls
// them); the counterparts of its other endpoints depend on C, and are reported
// on their own page, so each missing link is reported once, at the dependent.
//
// The comparison is undirected on purpose: an observed C->T dependency is
// considered reflected as long as there is any declared edge between C and T (in
// either direction, or via an API), so a declared dependency pointing the
// opposite way does not produce a false finding.
//
// Only Component entities are checked; for any other kind it returns nil.
func checkDependencyCandidates(e catalog.Entity, resolver catalog.Resolver) []Finding {
	c, ok := e.(*catalog.Component)
	if !ok {
		return nil
	}
	status := c.GetStatus()
	if status == nil {
		return nil
	}

	neighbors := catalog.DeclaredNeighbors(c, resolver)
	self := c.GetRef().String()

	// Collect missing observed targets, deduped by target ref. For each we
	// gather the supporting evidence (the endpoint addresses that link the two
	// components) and the tools that reported it as deduped sets.
	type missingDep struct {
		ref      *catalog.Ref
		evidence map[string]bool
		tools    map[string]bool
	}
	missing := make(map[string]*missingDep)

	for name, obs := range status.Observations {
		if !strings.HasPrefix(name, catalog.ObservedEndpointsKeyPrefix+"/") {
			continue
		}
		oe, err := catalog.ParseObservedEndpoints(obs.Value)
		if err != nil {
			continue
		}
		for _, ep := range oe.Endpoints {
			if !ep.SourceDependsOnCounterparts() {
				continue
			}
			for _, cp := range ep.Counterparts {
				if cp.Entity == nil || cp.Entity.Kind != catalog.KindComponent {
					continue
				}
				target := cp.Entity.String()
				if target == self || neighbors[target] {
					continue
				}
				m := missing[target]
				if m == nil {
					m = &missingDep{
						ref:      cp.Entity,
						evidence: make(map[string]bool),
						tools:    make(map[string]bool),
					}
					missing[target] = m
				}
				addNonEmpty(m.evidence, ep.Address)
				addNonEmpty(m.tools, obs.Producer)
			}
		}
	}

	// Emit findings in a stable (target-sorted) order.
	targets := sortedKeys(missing)
	findings := make([]Finding, 0, len(targets))
	for _, target := range targets {
		m := missing[target]
		msg := fmt.Sprintf(
			"Observed dependency on %s is not reflected in declared dependencies or API usage",
			m.ref)
		// Lead with the evidence (the most actionable part); name the
		// detecting tools as secondary context.
		var clauses []string
		if c := evidenceClause(m.evidence); c != "" {
			clauses = append(clauses, c)
		}
		if tools := sortedKeys(m.tools); len(tools) > 0 {
			clauses = append(clauses, "detected by "+strings.Join(tools, ", "))
		}
		if len(clauses) > 0 {
			msg += " (" + strings.Join(clauses, "; ") + ")"
		}
		findings = append(findings, Finding{
			RuleName: DependencyCandidateRuleName,
			Severity: SeverityWarn,
			Message:  msg,
		})
	}
	return findings
}

// addNonEmpty adds each non-empty item to the set.
func addNonEmpty(set map[string]bool, items ...string) {
	for _, it := range items {
		if it != "" {
			set[it] = true
		}
	}
}

// sortedKeys returns the keys of m in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// evidenceClause renders the evidence set as a message clause, sorted and capped
// at maxEvidenceSamples with a "(+N more)" suffix when truncated. Returns "" for
// an empty set.
func evidenceClause(set map[string]bool) string {
	ev := sortedKeys(set)
	if len(ev) == 0 {
		return ""
	}
	shown := ev
	suffix := ""
	if len(ev) > maxEvidenceSamples {
		shown = ev[:maxEvidenceSamples]
		suffix = fmt.Sprintf(" (+%d more)", len(ev)-maxEvidenceSamples)
	}
	return "evidence: " + strings.Join(shown, ", ") + suffix
}
