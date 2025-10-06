package query

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
)

// Evaluator holds a compiled query expression and provides methods to match it against entities.
// It caches compiled regular expressions for performance.
type Evaluator struct {
	expr       Expression
	regexCache map[string]*regexp.Regexp
}

// NewEvaluator creates a new Evaluator for the given expression AST.
func NewEvaluator(expr Expression) *Evaluator {
	return &Evaluator{
		expr:       expr,
		regexCache: make(map[string]*regexp.Regexp),
	}
}

// attributeAccessor defines a function that extracts specific string attribute values from an entity.
// It returns a slice of strings and a boolean indicating if the attribute is applicable.
type attributeAccessor func(e api.Entity) (values []string, ok bool)

// attributeAccessors maps query attribute names to functions that can retrieve them from an entity.
var attributeAccessors = map[string]attributeAccessor{
	"kind":      func(e api.Entity) ([]string, bool) { return []string{e.GetKind()}, true },
	"name":      func(e api.Entity) ([]string, bool) { return []string{e.GetMetadata().Name}, true },
	"namespace": func(e api.Entity) ([]string, bool) { return []string{e.GetMetadata().Namespace}, true },
	"title":     func(e api.Entity) ([]string, bool) { return []string{e.GetMetadata().Title}, true },
	"tag":       func(e api.Entity) ([]string, bool) { return e.GetMetadata().Tags, true },
	"label": func(e api.Entity) ([]string, bool) {
		if e.GetMetadata().Labels == nil {
			return nil, true
		}
		// For labels, we match against either key or value
		var results []string
		for k, v := range e.GetMetadata().Labels {
			results = append(results, k, v)
		}
		return results, true
	},
	"owner": func(e api.Entity) ([]string, bool) {
		switch v := e.(type) {
		case *api.Component:
			return []string{v.Spec.Owner}, true
		case *api.System:
			return []string{v.Spec.Owner}, true
		case *api.Domain:
			return []string{v.Spec.Owner}, true
		case *api.Resource:
			return []string{v.Spec.Owner}, true
		case *api.API:
			return []string{v.Spec.Owner}, true
		default:
			return nil, false // Group and other types don't have an owner
		}
	},
	"type": func(e api.Entity) ([]string, bool) {
		switch v := e.(type) {
		case *api.Component:
			return []string{v.Spec.Type}, true
		case *api.System:
			return []string{v.Spec.Type}, true
		case *api.Domain:
			return []string{v.Spec.Type}, true
		case *api.Resource:
			return []string{v.Spec.Type}, true
		case *api.API:
			return []string{v.Spec.Type}, true
		case *api.Group:
			return []string{v.Spec.Type}, true
		default:
			return nil, false
		}
	},
	"lifecycle": func(e api.Entity) ([]string, bool) {
		switch v := e.(type) {
		case *api.Component:
			return []string{v.Spec.Lifecycle}, true
		case *api.API:
			return []string{v.Spec.Lifecycle}, true
		default:
			return nil, false
		}
	},
}

// Matches returns true if the entity matches the expression held by the Evaluator.
func (ev *Evaluator) Matches(e api.Entity) (bool, error) {
	return ev.evaluateNode(e, ev.expr)
}

// evaluateNode recursively walks the expression tree.
func (ev *Evaluator) evaluateNode(e api.Entity, expr Expression) (bool, error) {
	switch v := expr.(type) {
	case *Term:
		// A simple term matches against the entity's name.
		m := e.GetMetadata()
		return strings.Contains(strings.ToLower(m.Name), strings.ToLower(v.Value)), nil

	case *AttributeTerm:
		attr := strings.ToLower(v.Attribute)
		accessor, ok := attributeAccessors[attr]
		if !ok {
			return false, fmt.Errorf("unknown attribute for filtering: %s", v.Attribute)
		}

		values, ok := accessor(e)
		if !ok {
			// Attribute is not applicable to this entity kind.
			return false, nil
		}

		// Check if any of the returned values match the query value.
		for _, value := range values {
			matches, err := ev.matchesOperator(value, v.Operator, v.Value)
			if err != nil {
				return false, err
			}
			if matches {
				return true, nil
			}
		}
		return false, nil

	case *NotExpression:
		matches, err := ev.evaluateNode(e, v.Expression)
		if err != nil {
			return false, err
		}
		return !matches, nil

	case *BinaryExpression:
		leftMatches, err := ev.evaluateNode(e, v.Left)
		if err != nil {
			return false, err
		}

		if v.Operator == "AND" {
			if !leftMatches {
				return false, nil
			}
			return ev.evaluateNode(e, v.Right)
		}

		if v.Operator == "OR" {
			if leftMatches {
				return true, nil
			}
			return ev.evaluateNode(e, v.Right)
		}
	}

	return false, fmt.Errorf("unsupported expression type")
}

// matchesOperator performs the actual string comparison based on the operator.
func (ev *Evaluator) matchesOperator(entityValue, operator, queryValue string) (bool, error) {
	switch operator {
	case ":":
		return strings.Contains(strings.ToLower(entityValue), strings.ToLower(queryValue)), nil
	case "~":
		re, found := ev.regexCache[queryValue]
		if !found {
			var err error
			re, err = regexp.Compile("(?i)" + queryValue) // (?i) for case-insensitivity
			if err != nil {
				return false, fmt.Errorf("invalid regular expression %q: %w", queryValue, err)
			}
			ev.regexCache[queryValue] = re
		}

		return re.MatchString(entityValue), nil
	default:
		return false, nil
	}
}
