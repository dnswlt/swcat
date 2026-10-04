package query

import (
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

type entityResolver map[string]catalog.Entity

func (r entityResolver) Entity(ref *catalog.Ref) catalog.Entity {
	return r[ref.String()]
}

func TestRelationshipEvaluation(t *testing.T) {
	api := &catalog.API{Metadata: &catalog.Metadata{Name: "api"}, Spec: &catalog.APISpec{}}
	component := &catalog.Component{Metadata: &catalog.Metadata{Name: "consumer"}, Spec: &catalog.ComponentSpec{
		ConsumesAPIs: []*catalog.LabelRef{
			{Ref: catalog.MustParseRef("api:missing")},
			{Ref: api.GetRef()},
		},
	}}
	resolved := entityResolver{api.GetRef().String(): api}
	tests := []struct {
		query    string
		resolver Resolver
		want     bool
	}{
		{"consumesApis[name=api]", resolved, true},
		// The missing API is unresolved, so it is no witness for a negated condition either.
		{"consumesApis[!name=api]", resolved, false},
		{"consumesApis[name=api]", entityResolver{}, false},
		{"!consumesApis[name=api]", entityResolver{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			evaluator, err := Compile(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			if got := evaluator.Matches(component, tt.resolver); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// mustPanic fails the test unless f panics with a message containing want.
func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("got panic %v, want one containing %q", r, want)
		}
	}()
	f()
}

// A nil resolver is a programming error. It panics on every query, not only
// on those whose evaluation reaches a relationship predicate.
func TestMatchesRequiresResolver(t *testing.T) {
	component := &catalog.Component{Metadata: &catalog.Metadata{Name: "c"}, Spec: &catalog.ComponentSpec{}}
	for _, q := range []string{
		"name=c",
		"consumesApis[name=x]",
		// Short-circuited: evaluation would never reach the relationship.
		"name=nomatch AND consumesApis[name=x]",
		"name=c OR consumesApis[name=x]",
	} {
		t.Run(q, func(t *testing.T) {
			evaluator, err := Compile(q)
			if err != nil {
				t.Fatal(err)
			}
			mustPanic(t, "requires a resolver", func() { evaluator.Matches(component, nil) })
		})
	}
}

// unknownExpr is an expression type that Parse never produces.
type unknownExpr struct{}

func (unknownExpr) String() string { return "unknown" }

// Expression types and operators that Parse never produces are internal
// invariant violations: they panic instead of silently not matching.
func TestUnsupportedExpressionsPanic(t *testing.T) {
	component := &catalog.Component{Metadata: &catalog.Metadata{Name: "c"}, Spec: &catalog.ComponentSpec{}}
	t.Run("evaluate unknown type", func(t *testing.T) {
		ev := &Evaluator{expr: unknownExpr{}}
		mustPanic(t, "unsupported expression type", func() { ev.Matches(component, entityResolver{}) })
	})
	t.Run("evaluate unknown operator", func(t *testing.T) {
		ev := &Evaluator{expr: &BinaryExpression{Left: &Term{Value: "c"}, Operator: "XOR", Right: &Term{Value: "c"}}}
		mustPanic(t, "unsupported binary operator", func() { ev.Matches(component, entityResolver{}) })
	})
	t.Run("compile unknown type", func(t *testing.T) {
		ev := &Evaluator{}
		mustPanic(t, "unsupported expression type", func() { ev.compileNode(&NotExpression{Expression: unknownExpr{}}) })
	})
}

func TestEvaluator_Matches(t *testing.T) {
	api1 := &catalog.API{
		Metadata: &catalog.Metadata{
			Name: "my-api",
		},
	}
	dom1 := &catalog.Domain{
		Metadata: &catalog.Metadata{
			Name: "my-domain",
		},
	}
	sys1 := &catalog.System{
		Metadata: &catalog.Metadata{
			Name:      "my-system",
			Namespace: "production",
			Title:     "My Production System",
			Tags:      []string{"java", "prod"},
			Labels:    map[string]string{"env": "prod", "critical": "true"},
		},
		Spec: &catalog.SystemSpec{
			Type:   "workflow",
			Owner:  &catalog.Ref{Name: "team-b"},
			Domain: dom1.GetRef(),
		},
	}
	comp1 := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name:        "test-component",
			Namespace:   "default",
			Title:       "Test Component",
			Description: "Super duper component",
			Tags:        []string{"go", "test"},
			Labels:      map[string]string{"env": "dev", "team": "a"},
		},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "experimental",
			Owner:     &catalog.Ref{Name: "team-a"},
			System:    sys1.GetRef(),
			ProvidesAPIs: []*catalog.LabelRef{
				{Ref: api1.GetRef()},
			},
		},
	}
	comp1.SetDomain(dom1.GetRef())
	catalog.MergeObservations(comp1, map[string]catalog.Observation{
		"swcat-plugins/test": {
			Value: []byte(`{"foo": "bar"}`),
		},
	})

	tests := []struct {
		name      string
		query     string
		entity    catalog.Entity
		wantMatch bool
		wantErr   bool
	}{
		// Simple Term Matching
		{
			name:      "simple term match",
			query:     "test-component",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "simple term partial match",
			query:     "component",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "simple term no match",
			query:     "my-system",
			entity:    comp1,
			wantMatch: false,
			wantErr:   false,
		},
		// Equality Matching (Operator '=')
		{
			name:      "exact match name",
			query:     "name=my-system",
			entity:    sys1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "exact match name case-insensitive",
			query:     "name=My-System",
			entity:    sys1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "exact match name no match",
			query:     "name=my-system-nope",
			entity:    sys1,
			wantMatch: false,
			wantErr:   false,
		},
		{
			name:      "exact match owner",
			query:     "owner=team-b",
			entity:    sys1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "exact match lifecycle no match",
			query:     "lifecycle=production",
			entity:    comp1,
			wantMatch: false,
			wantErr:   false,
		},

		// Attribute Matching (Operator ':')
		{
			name:      "exact attribute match",
			query:     "description:'super duper'",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "exact attribute match",
			query:     "owner:team-a",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "contains attribute match",
			query:     "owner:team",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "system attribute match",
			query:     "system:my-system",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "domain attribute match",
			query:     "domain:my-domain",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "domain attribute match (system)",
			query:     "domain:my-domain",
			entity:    sys1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "case-insensitive contains match",
			query:     "owner:TEAM-A",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "tag match",
			query:     "tag:go",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "label value match",
			query:     "label:dev",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "label key match",
			query:     "label:team",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "status key match",
			query:     "status:swcat-plugin",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "status regex match",
			query:     "status~'swcat-plugins/test=.*\"bar\"'",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "attribute no match",
			query:     "owner:team-b",
			entity:    comp1,
			wantMatch: false,
			wantErr:   false,
		},

		// Regex Matching (Operator '~')
		{
			name:      "regex match",
			query:     "name~test-.*",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "regex no match",
			query:     "owner~^team-b$",
			entity:    comp1,
			wantMatch: false,
			wantErr:   false,
		},

		// Logical Operators
		{
			name:      "AND match",
			query:     "owner:team-a AND type:service",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "AND no match",
			query:     "owner:team-a AND type:website",
			entity:    comp1,
			wantMatch: false,
			wantErr:   false,
		},
		{
			name:      "OR match",
			query:     "owner:team-b OR type:service",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "NOT match",
			query:     "!owner:team-b",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "complex query with parentheses",
			query:     "tag:go AND (owner:team-b OR lifecycle:experimental)",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},

		// Entity Kind Specifics
		{
			name:      "system tag match",
			query:     "tag:java",
			entity:    sys1,
			wantMatch: true,
			wantErr:   false,
		},
		{
			name:      "attribute not applicable to kind",
			query:     "lifecycle:production",
			entity:    sys1,
			wantMatch: false,
			wantErr:   false,
		},
		{
			name:      "component consumesApis",
			query:     "consumesApis:my-api",
			entity:    comp1,
			wantMatch: false,
			wantErr:   false,
		},
		{
			name:      "component consumesApis",
			query:     "providesApis:my-api",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},
		// Full text search with *:
		{
			name:      "full text search",
			query:     "*:'super duper' AND *:swcat-plugins",
			entity:    comp1,
			wantMatch: true,
			wantErr:   false,
		},

		// Error Cases
		{
			name:    "unknown attribute",
			query:   "foo:bar",
			entity:  comp1,
			wantErr: true,
		},
		{
			name:    "invalid regex",
			query:   "name~'[a-'",
			entity:  comp1,
			wantErr: true, // Parses, but Compile rejects it
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evaluator, err := Compile(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("Compile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if gotMatch := evaluator.Matches(tt.entity, entityResolver{}); gotMatch != tt.wantMatch {
				t.Errorf("Evaluator.Matches() = %v, want %v", gotMatch, tt.wantMatch)
			}
		})
	}
}

// Compile validates the whole expression up front, so whether a query is valid
// does not depend on which parts evaluation would reach.
func TestCompileValidatesWholeQuery(t *testing.T) {
	lint := PropertyProvider{
		Names:  []string{"lint"},
		Values: func(catalog.Entity, string) []string { return nil },
	}
	tests := []struct {
		query string
		err   string // empty: compiles
	}{
		{"name=a", ""},
		{"lint=warn", ""},
		{"LINT=warn", ""},
		{"consumesApis[lint=warn]", ""},
		{"unknown=x", "unknown attribute for filtering: unknown"},
		{"!unknown=x", "unknown attribute"},
		{"consumesApis[unknown=x]", "unknown attribute"},
		{"name=a AND unknown=x", "unknown attribute"},
		{"name~'[a-'", "invalid regular expression"},
		{"name=a OR name~'[a-'", "invalid regular expression"},
		{"consumesApis[providedBy[name~'(']]", "invalid regular expression"},
		{"consumesApis[name=x", "expected ']'"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			_, err := Compile(tt.query, lint)
			if tt.err == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Fatalf("got error %v, want %q", err, tt.err)
			}
		})
	}
	// Provider attributes are known only to evaluators compiled with the provider.
	if _, err := Compile("lint=warn"); err == nil || !strings.Contains(err.Error(), "unknown attribute") {
		t.Fatalf("Compile without provider: got %v, want unknown attribute", err)
	}
}
