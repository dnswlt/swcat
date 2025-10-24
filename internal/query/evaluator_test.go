package query

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

func TestEvaluator_Matches(t *testing.T) {
	sys1 := &catalog.System{
		Metadata: &catalog.Metadata{
			Name:      "my-system",
			Namespace: "production",
			Title:     "My Production System",
			Tags:      []string{"java", "prod"},
			Labels:    map[string]string{"env": "prod", "critical": "true"},
		},
		Spec: &catalog.SystemSpec{
			Type:  "workflow",
			Owner: &catalog.Ref{Name: "team-b"},
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
		},
	}

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

		// Error Cases
		{
			name:    "unknown attribute",
			query:   "foo:bar",
			entity:  comp1,
			wantErr: true,
		},
		{
			name:    "invalid regex",
			query:   "name~[a-",
			entity:  comp1,
			wantErr: true, // This error surfaces during evaluation, not parsing
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This assumes a Parse function exists in the query package.
			// You would have this from your queryparser.
			expr, err := Parse(tt.query)
			if err != nil {
				if tt.wantErr {
					return // Expected parse error
				}
				t.Fatalf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}

			evaluator := NewEvaluator(expr)
			gotMatch, err := evaluator.Matches(tt.entity)

			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluator.Matches() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && gotMatch != tt.wantMatch {
				t.Errorf("Evaluator.Matches() = %v, want %v", gotMatch, tt.wantMatch)
			}
		})
	}
}
