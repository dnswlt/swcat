package lint

import (
	"encoding/json"
	"fmt"

	"github.com/dnswlt/swcat/internal/api"
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
)

var (
	KnownCustomChecks = map[string]CustomCheckFunc{
		"alwaysFail":     CheckAlwaysFail,
		"lintAnnotation": CheckLintAnnotation,
	}
)

// CheckLintAnnotation is a custom lint rule that checks for the existence of a
// specific annotation on an entity and returns its content as a lint finding.
// The rule expects an "annotation" parameter in its configuration. If the
// annotation's value is a JSON-encoded api.LintFinding, its message is used;
// otherwise, the raw annotation value is returned as the finding message.
func CheckLintAnnotation(rule CustomRule, e *catalog_pb.Entity) []Finding {
	const annotationKey = "annotation"
	if rule.Params == nil || rule.Params[annotationKey] == "" {
		return []Finding{
			{
				RuleName: rule.Name,
				Severity: SeverityError,
				Message:  fmt.Sprintf("Config error: missing %q parameter", annotationKey),
			},
		}
	}
	annotation := rule.Params[annotationKey]
	value, ok := e.GetMetadata().Annotations[annotation]
	if !ok {
		return nil
	}
	var finding api.LintFinding
	err := json.Unmarshal([]byte(value), &finding)
	if err != nil {
		// Not a structured finding message: return whole annotation text as message.
		finding.Message = value
	}
	return []Finding{
		{
			RuleName: rule.Name,
			Severity: rule.Severity,
			Message:  finding.Message,
		},
	}
}

func CheckAlwaysFail(rule CustomRule, e *catalog_pb.Entity) []Finding {
	return []Finding{
		{
			RuleName: rule.Name,
			Severity: rule.Severity,
			Message:  "This check always fails. Use it only for testing your linting config.",
		},
	}
}
