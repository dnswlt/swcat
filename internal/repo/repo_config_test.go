package repo

import (
	"regexp"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

// Helper to create ValueRegexp for tests. It mimics the UnmarshalYAML logic
// by wrapping the pattern with anchors to enforce a full match.
func mustValueRegexp(s string) *ValueRegexp {
	re := regexp.MustCompile("^(?:" + s + ")$")
	return (*ValueRegexp)(re)
}

func TestCatalogValidationRules_Accept(t *testing.T) {
	testCases := []struct {
		name    string
		rules   *CatalogValidationRules
		entity  catalog.Entity
		wantErr bool
	}{
		// --- API Tests ---
		{
			name: "API: valid type (regex) and lifecycle (value)",
			rules: &CatalogValidationRules{
				API: &APIValidationRules{
					Type: &ValueRule{
						Matches: []*ValueRegexp{
							mustValueRegexp("openapi"),
							mustValueRegexp("grpc-.+"),
						},
					},
					Lifecycle: &ValueRule{
						Values: []string{"experimental", "production"},
					},
				},
			},
			entity: &catalog.API{
				Spec: &catalog.APISpec{Type: "openapi", Lifecycle: "production"},
			},
			wantErr: false,
		},
		{
			name: "API: valid type (second regex)",
			rules: &CatalogValidationRules{
				API: &APIValidationRules{
					Type: &ValueRule{
						Matches: []*ValueRegexp{mustValueRegexp("grpc-.+")},
					},
				},
			},
			entity: &catalog.API{
				Spec: &catalog.APISpec{Type: "grpc-customer-v1"},
			},
			wantErr: false,
		},
		{
			name: "API: invalid type (partial match rejected)",
			rules: &CatalogValidationRules{
				API: &APIValidationRules{
					Type: &ValueRule{Matches: []*ValueRegexp{mustValueRegexp("openapi")}},
				},
			},
			entity: &catalog.API{
				Spec: &catalog.APISpec{Type: "my-openapi-spec", Lifecycle: "production"},
			},
			wantErr: true,
		},
		{
			name: "API: invalid type (no match)",
			rules: &CatalogValidationRules{
				API: &APIValidationRules{
					Type: &ValueRule{Matches: []*ValueRegexp{mustValueRegexp("openapi")}},
				},
			},
			entity: &catalog.API{
				Spec: &catalog.APISpec{Type: "soap", Lifecycle: "production"},
			},
			wantErr: true,
		},
		{
			name: "API: invalid lifecycle",
			rules: &CatalogValidationRules{
				API: &APIValidationRules{
					Lifecycle: &ValueRule{Values: []string{"production"}},
				},
			},
			entity: &catalog.API{
				Spec: &catalog.APISpec{Type: "openapi", Lifecycle: "deprecated"},
			},
			wantErr: true,
		},
		{
			name: "API: invalid empty type",
			rules: &CatalogValidationRules{
				API: &APIValidationRules{
					Type: &ValueRule{Matches: []*ValueRegexp{mustValueRegexp("openapi")}},
				},
			},
			entity: &catalog.API{
				Spec: &catalog.APISpec{Type: "", Lifecycle: "production"},
			},
			wantErr: true,
		},

		// --- Component Tests ---
		{
			name: "Component: valid type and any lifecycle (nil rule)",
			rules: &CatalogValidationRules{
				Component: &ComponentValidationRules{
					Type:      &ValueRule{Values: []string{"service", "library"}},
					Lifecycle: nil, // Any lifecycle is accepted
				},
			},
			entity: &catalog.Component{
				Spec: &catalog.ComponentSpec{Type: "service", Lifecycle: "beta"},
			},
			wantErr: false,
		},
		{
			name: "Component: valid with no lifecycle value and nil rule",
			rules: &CatalogValidationRules{
				Component: &ComponentValidationRules{
					Type:      &ValueRule{Values: []string{"service", "library"}},
					Lifecycle: nil,
				},
			},
			entity: &catalog.Component{
				Spec: &catalog.ComponentSpec{Type: "library", Lifecycle: ""},
			},
			wantErr: false,
		},
		{
			name: "Component: invalid type",
			rules: &CatalogValidationRules{
				Component: &ComponentValidationRules{
					Type: &ValueRule{Values: []string{"service", "library"}},
				},
			},
			entity: &catalog.Component{
				Spec: &catalog.ComponentSpec{Type: "website", Lifecycle: "production"},
			},
			wantErr: true,
		},

		// --- Resource Tests ---
		{
			name: "Resource: valid type",
			rules: &CatalogValidationRules{
				Resource: &ResourceValidationRules{
					Type: &ValueRule{Values: []string{"database", "cache"}},
				},
			},
			entity: &catalog.Resource{
				Spec: &catalog.ResourceSpec{Type: "database"},
			},
			wantErr: false,
		},
		{
			name: "Resource: invalid type",
			rules: &CatalogValidationRules{
				Resource: &ResourceValidationRules{
					Type: &ValueRule{Values: []string{"database", "cache"}},
				},
			},
			entity: &catalog.Resource{
				Spec: &catalog.ResourceSpec{Type: "blob-storage"},
			},
			wantErr: true,
		},

		// --- Tests for kinds with no rules ---
		{
			name:  "Domain: valid because no rules are defined for it",
			rules: &CatalogValidationRules{
				// No domain rules
			},
			entity: &catalog.Domain{
				Spec: &catalog.DomainSpec{Type: "customer-data"},
			},
			wantErr: false,
		},
		{
			name:  "System: valid because no rules are defined for it",
			rules: &CatalogValidationRules{
				// No system rules
			},
			entity: &catalog.System{
				Spec: &catalog.SystemSpec{Type: "billing-system"},
			},
			wantErr: false,
		},

		// --- Test with empty ruleset ---
		{
			name:  "API: valid because the top-level rules object is empty",
			rules: &CatalogValidationRules{},
			entity: &catalog.API{
				Spec: &catalog.APISpec{Type: "any", Lifecycle: "any"},
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.rules.Accept(tc.entity); (err != nil) != tc.wantErr {
				t.Errorf("Accept() error %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
