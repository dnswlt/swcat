package lint

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

func TestCELDemo(t *testing.T) {
	// This test case serves as a "demo" of CEL's capabilities and how
	// to apply them to entities (either for selecting entities or for validating them).

	// All CEL expressions below are evaluated against this Component:
	demoComponent := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name:        "super-service",
			Namespace:   "billing",
			Title:       "Super Billing Service",
			Description: "Handles all billing operations",
			Labels: map[string]string{
				"tier":       "critical",
				"createTime": "2024-01-01T00:00:00+01:00",
				"updateTime": "2024-02-09T08:00:00.123Z",
			},
			Annotations: map[string]string{
				"deployment": "kubernetes",
				"priority":   "100",
			},
			Tags: []string{"java", "spring-boot"},
		},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     catalog.MustParseRef("group:billing-team"),
			System:    catalog.MustParseRef("system:payment-platform"),
		},
	}
	pb := catalog.ToPB(demoComponent)

	// All expressions in this list evaluate to true when evaluated on demoComponent.
	// See https://github.com/google/cel-spec/blob/master/doc/langdef.md for a detailed
	// documentation of the CEL lanaguage.
	celExamples := []string{
		// Case matters:
		`kind == 'component'`,
		`kind != 'Component'`,
		// Refs are structured objects in CEL
		`spec.owner.kind == "group" && spec.owner.namespace == "default" && spec.owner.name == "billing-team"`,
		// Access labels (or annotations)
		`metadata.labels['tier'] == 'critical'`,
		// Check for presence of a map key
		`!("foo" in metadata.labels)`,
		`!has(metadata.labels.foo)`,
		// Check for presence of a tag
		`"java" in metadata.tags`,
		// CEL macros are useful for predicates over certain elements of a list or map
		`metadata.labels
			.filter(k, k.endsWith("Time"))
			.all(k, timestamp(metadata.labels[k]).getFullYear() > 2020)`,
		`metadata.annotations.exists_one(e, e.startsWith("deploy"))`, // Exactly one match (use .exists for 1..N)
		// Implications: if there is a certain tag, the spec.type must have a certain value:
		// (Uses the fact that {A ==> B} is the same as {!A || B}.)
		`!("spring-boot" in metadata.tags) || spec.type == "service"`,
		// Operations on timestamps
		// https://github.com/google/cel-spec/blob/master/doc/langdef.md#datetime-functions
		`timestamp(metadata.labels["updateTime"]).getFullYear() == 2024`,
		// string -> int conversion
		`int(metadata.annotations["priority"]) == 100`,
		// String operations
		`spec.type.startsWith("ser")`,
		`spec.type.endsWith("vice")`,
		`spec.type.size() == 7`,
		`metadata.title.contains("Bill")`,
		// Regular expressions
		`matches(metadata.name, "^\\w+(-\\w+)*$")`,
		`matches(metadata.name, "-")`, // "matches" is actually a search, not a full match.

	}

	// Run each CEL from celExamples as a Rule against a Linter.
	// This is only done to verify that the above expressions evaluate to true.
	for _, expr := range celExamples {
		t.Run(expr, func(t *testing.T) {
			config := &Config{
				CELRules: []Rule{
					{
						Name:    "demo-rule",
						Check:   expr,
						Message: "Validation failed",
					},
				},
			}
			linter, err := NewLinter(config, nil)
			if err != nil {
				t.Fatalf("NewLinter: %v", err)
			}

			findings := linter.Lint(pb)
			if len(findings) > 0 {
				t.Errorf("CEL expression %q failed: %v", expr, findings[0].Message)
			}
		})
	}
}
