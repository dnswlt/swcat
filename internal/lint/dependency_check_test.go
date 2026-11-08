package lint

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

// fakeResolver resolves refs from a fixed map keyed by ref string.
type fakeResolver map[string]catalog.Entity

func (f fakeResolver) Entity(ref *catalog.Ref) catalog.Entity { return f[ref.String()] }

// setObservedDeps attaches an "swcat-endpoints/<tool>" observation to c in
// which c depends on each of targets (given as ref strings): one inbound event
// endpoint per target, with the target as its counterpart.
func setObservedDeps(t *testing.T, c *catalog.Component, tool string, targets ...string) {
	t.Helper()
	eps := make([]catalog.ObservedEndpoint, len(targets))
	for i, target := range targets {
		eps[i] = inbound("topic/"+target, catalog.MustParseRef(target))
	}
	setObservedEndpoints(t, c, tool, eps...)
}

// setObservedEndpoints attaches an "swcat-endpoints/<tool>" observation with
// fully specified endpoints to c.
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

// endpoint returns an observed endpoint with the given counterpart entities.
func endpoint(address, direction, kind string, counterparts ...*catalog.Ref) catalog.ObservedEndpoint {
	ep := catalog.ObservedEndpoint{Address: address, Direction: direction, Kind: kind}
	for _, r := range counterparts {
		ep.Counterparts = append(ep.Counterparts, catalog.ObservedCounterpart{Entity: r})
	}
	return ep
}

// inbound returns an inbound event endpoint: the source consumes the
// counterparts' events, so it depends on them.
func inbound(address string, counterparts ...*catalog.Ref) catalog.ObservedEndpoint {
	return endpoint(address, catalog.EndpointInbound, catalog.EndpointKindEvent, counterparts...)
}

func ref(s string) *catalog.Ref { return catalog.MustParseRef(s) }

// buildComponentGraph returns a component "c" with one declared dependency
// (dep1), one declared dependent (dep2), one component reachable via a provided
// API (cons1), and one reachable via a consumed API (prov1), along with a
// resolver that knows the two APIs.
func buildComponentGraph() (*catalog.Component, fakeResolver) {
	a1 := &catalog.API{Metadata: &catalog.Metadata{Name: "a1"}, Spec: &catalog.APISpec{}}
	a1.AddConsumer(&catalog.LabelRef{Ref: ref("component:default/cons1")})

	a2 := &catalog.API{Metadata: &catalog.Metadata{Name: "a2"}, Spec: &catalog.APISpec{}}
	a2.AddProvider(&catalog.LabelRef{Ref: ref("component:default/prov1")})

	c := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "c"},
		Spec: &catalog.ComponentSpec{
			DependsOn:    []*catalog.LabelRef{{Ref: ref("component:default/dep1")}},
			ProvidesAPIs: []*catalog.LabelRef{{Ref: ref("api:default/a1")}},
			ConsumesAPIs: []*catalog.LabelRef{{Ref: ref("api:default/a2")}},
		},
	}
	c.AddDependent(&catalog.LabelRef{Ref: ref("component:default/dep2")})

	resolver := fakeResolver{
		"api:a1": a1,
		"api:a2": a2,
	}
	return c, resolver
}

func messages(findings []Finding) string {
	var sb strings.Builder
	for _, f := range findings {
		sb.WriteString(f.Message)
		sb.WriteString("\n")
	}
	return sb.String()
}

func TestCheckDependencyCandidates_FlagsOnlyUnreflected(t *testing.T) {
	c, resolver := buildComponentGraph()
	setObservedDeps(t, c, "test-tool",
		"component:default/dep1",    // declared dependency -> reflected
		"component:default/dep2",    // declared dependent  -> reflected
		"component:default/cons1",   // via provided API    -> reflected
		"component:default/prov1",   // via consumed API    -> reflected
		"component:default/missing", // not modelled        -> finding
		"resource:default/res1",     // non-component       -> ignored
		"component:default/c",       // self                -> ignored
	)

	findings := checkDependencyCandidates(c, resolver)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1:\n%s", len(findings), messages(findings))
	}
	f := findings[0]
	if f.RuleName != DependencyCandidateRuleName {
		t.Errorf("RuleName = %q, want %q", f.RuleName, DependencyCandidateRuleName)
	}
	if f.Severity != SeverityWarn {
		t.Errorf("Severity = %q, want %q", f.Severity, SeverityWarn)
	}
	if !strings.Contains(f.Message, "component:missing") {
		t.Errorf("message does not mention the missing target: %q", f.Message)
	}
	if !strings.Contains(f.Message, "test-tool") {
		t.Errorf("message does not mention the detecting tool: %q", f.Message)
	}
}

func TestCheckDependencyCandidates_IncludesEvidence(t *testing.T) {
	c, resolver := buildComponentGraph()
	setObservedEndpoints(t, c, "traffic-scanner",
		inbound("flights/events", ref("component:default/missing")),
		endpoint("POST /internal/pay", catalog.EndpointOutbound, catalog.EndpointKindRequest,
			ref("component:default/missing")),
	)

	findings := checkDependencyCandidates(c, resolver)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1:\n%s", len(findings), messages(findings))
	}
	msg := findings[0].Message
	for _, want := range []string{"flights/events", "POST /internal/pay", "traffic-scanner"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q: %q", want, msg)
		}
	}
	// Evidence should lead, the detecting tool follows.
	if strings.Index(msg, "evidence:") > strings.Index(msg, "detected by") {
		t.Errorf("expected evidence before detecting tool: %q", msg)
	}
}

func TestCheckDependencyCandidates_CapsEvidenceSamples(t *testing.T) {
	c, resolver := buildComponentGraph()
	var eps []catalog.ObservedEndpoint
	for _, address := range []string{"ev-1", "ev-2", "ev-3", "ev-4", "ev-5"} {
		eps = append(eps, inbound(address, ref("component:default/missing")))
	}
	setObservedEndpoints(t, c, "traffic-scanner", eps...)

	findings := checkDependencyCandidates(c, resolver)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1:\n%s", len(findings), messages(findings))
	}
	msg := findings[0].Message
	// First three (sorted) shown, the rest summarized.
	for _, want := range []string{"ev-1", "ev-2", "ev-3", "(+2 more)"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q: %q", want, msg)
		}
	}
	for _, notWant := range []string{"ev-4", "ev-5"} {
		if strings.Contains(msg, notWant) {
			t.Errorf("message should not list %q (capped): %q", notWant, msg)
		}
	}
}

func TestCheckDependencyCandidates_OnlyWhereSourceDepends(t *testing.T) {
	c, resolver := buildComponentGraph()
	setObservedEndpoints(t, c, "test-tool",
		// c depends on these counterparts: findings.
		endpoint("in/event", catalog.EndpointInbound, catalog.EndpointKindEvent, ref("component:default/producer")),
		endpoint("out/request", catalog.EndpointOutbound, catalog.EndpointKindRequest, ref("component:default/replier")),
		// These counterparts depend on c: reported on their own page, not here.
		endpoint("out/event", catalog.EndpointOutbound, catalog.EndpointKindEvent, ref("component:default/consumer")),
		endpoint("in/request", catalog.EndpointInbound, catalog.EndpointKindRequest, ref("component:default/requester")),
	)

	findings := checkDependencyCandidates(c, resolver)
	msg := messages(findings)
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2:\n%s", len(findings), msg)
	}
	for _, want := range []string{"component:producer", "component:replier"} {
		if !strings.Contains(msg, want) {
			t.Errorf("findings do not mention %q:\n%s", want, msg)
		}
	}
}

func TestCheckDependencyCandidates_ExternalCounterpartsIgnored(t *testing.T) {
	c, resolver := buildComponentGraph()
	setObservedEndpoints(t, c, "test-tool", catalog.ObservedEndpoint{
		Address:      "in/event",
		Direction:    catalog.EndpointInbound,
		Kind:         catalog.EndpointKindEvent,
		Counterparts: []catalog.ObservedCounterpart{{External: "com.example:outside"}},
	})
	if findings := checkDependencyCandidates(c, resolver); len(findings) != 0 {
		t.Fatalf("got %d findings, want 0:\n%s", len(findings), messages(findings))
	}
}

func TestCheckDependencyCandidates_AllReflected(t *testing.T) {
	c, resolver := buildComponentGraph()
	setObservedDeps(t, c, "test-tool",
		"component:default/dep1",
		"component:default/cons1",
	)
	if findings := checkDependencyCandidates(c, resolver); len(findings) != 0 {
		t.Fatalf("got %d findings, want 0:\n%s", len(findings), messages(findings))
	}
}

func TestCheckDependencyCandidates_DedupesAcrossTools(t *testing.T) {
	c, resolver := buildComponentGraph()
	setObservedDeps(t, c, "tool-a", "component:default/missing")
	setObservedDeps(t, c, "tool-b", "component:default/missing")

	findings := checkDependencyCandidates(c, resolver)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1 (deduped):\n%s", len(findings), messages(findings))
	}
	// Both reporting tools should be named.
	for _, tool := range []string{"tool-a", "tool-b"} {
		if !strings.Contains(findings[0].Message, tool) {
			t.Errorf("message does not mention %q: %q", tool, findings[0].Message)
		}
	}
}

func TestCheckDependencyCandidates_NonComponentIgnored(t *testing.T) {
	resource := &catalog.Resource{Metadata: &catalog.Metadata{Name: "r"}, Spec: &catalog.ResourceSpec{}}
	if findings := checkDependencyCandidates(resource, fakeResolver{}); findings != nil {
		t.Fatalf("expected nil findings for non-component, got %v", findings)
	}
}

func TestLintWithResolver_GatesOnConfigAndResolver(t *testing.T) {
	c, resolver := buildComponentGraph()
	setObservedDeps(t, c, "test-tool", "component:default/missing")

	// Disabled by default.
	off, err := NewLinter(&Config{})
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	if findings := off.LintWithResolver(c, resolver); len(findings) != 0 {
		t.Errorf("check ran while disabled: %s", messages(findings))
	}

	on, err := NewLinter(&Config{CheckDependencyCandidates: true})
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	// Enabled but no resolver: skipped.
	if findings := on.Lint(c); len(findings) != 0 {
		t.Errorf("check ran without a resolver: %s", messages(findings))
	}
	// Enabled with resolver: runs.
	if findings := on.LintWithResolver(c, resolver); len(findings) != 1 {
		t.Errorf("got %d findings, want 1:\n%s", len(findings), messages(findings))
	}
}
