package query

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"gopkg.in/yaml.v3"
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

// fulltextAccessor collects and returns all leaf values of the YAML from which e was built.
// For convenience, metadata label and annotation keys are also included.
func fulltextAccessor(e catalog.Entity) ([]string, bool) {
	if e.GetSourceInfo() == nil {
		return nil, false
	}
	node := e.GetSourceInfo().Node
	if node == nil {
		return nil, false
	}
	values := collectLeafValues(node)
	// Collect metadata label and annotation keys as well.
	m := e.GetMetadata()
	if e == nil {
		return values, true
	}
	for k := range m.Labels {
		values = append(values, k)
	}
	for k := range m.Annotations {
		values = append(values, k)
	}
	return values, true
}

// metadataAccessor returns all values of e's metadata.
func metadataAccessor(e catalog.Entity) ([]string, bool) {
	m := e.GetMetadata()
	if m == nil {
		return nil, false
	}
	values := []string{
		m.Name,
		m.Namespace,
		m.Title,
		m.Description,
	}
	for k, v := range m.Labels {
		values = append(values, k, v)
	}
	for k, v := range m.Annotations {
		values = append(values, k, v)
	}
	values = append(values, m.Tags...)
	for _, l := range m.Links {
		values = append(values, l.Title, l.URL)
	}
	return values, true
}

// attributeAccessor defines a function that extracts specific string attribute values from an entity.
// It returns a slice of strings and a boolean indicating if the attribute is applicable.
type attributeAccessor func(e catalog.Entity) (values []string, ok bool)

// attributeAccessors maps query attribute names to functions that can retrieve them from an entity.
var attributeAccessors = map[string]attributeAccessor{
	"*":           fulltextAccessor,
	"meta":        metadataAccessor,
	"kind":        func(e catalog.Entity) ([]string, bool) { return []string{string(e.GetKind())}, true },
	"name":        func(e catalog.Entity) ([]string, bool) { return []string{e.GetMetadata().Name}, true },
	"namespace":   func(e catalog.Entity) ([]string, bool) { return []string{e.GetMetadata().Namespace}, true },
	"title":       func(e catalog.Entity) ([]string, bool) { return []string{e.GetMetadata().Title}, true },
	"description": func(e catalog.Entity) ([]string, bool) { return []string{e.GetMetadata().Description}, true },
	"tag":         func(e catalog.Entity) ([]string, bool) { return e.GetMetadata().Tags, true },
	"label": func(e catalog.Entity) ([]string, bool) {
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
	"owner": func(e catalog.Entity) ([]string, bool) {
		if o := e.GetOwner(); o != nil {
			return []string{o.QName()}, true
		}
		return nil, false // No owner
	},
	"type": func(e catalog.Entity) ([]string, bool) {
		if t := e.GetType(); t != "" {
			return []string{t}, true
		}
		return nil, false
	},
	"lifecycle": func(e catalog.Entity) ([]string, bool) {
		switch v := e.(type) {
		case *catalog.Component:
			if v.Spec == nil {
				return nil, false
			}
			return []string{v.Spec.Lifecycle}, true
		case *catalog.API:
			if v.Spec == nil {
				return nil, false
			}
			return []string{v.Spec.Lifecycle}, true
		default:
			return nil, false
		}
	},
}

// Matches returns true if the entity matches the expression held by the Evaluator.
func (ev *Evaluator) Matches(e catalog.Entity) (bool, error) {
	return ev.evaluateNode(e, ev.expr)
}

// evaluateNode recursively walks the expression tree.
func (ev *Evaluator) evaluateNode(e catalog.Entity, expr Expression) (bool, error) {
	switch v := expr.(type) {
	case *Term:
		// A simple term matches against the entity's qualified name.
		qn := e.GetRef().QName()
		return strings.Contains(strings.ToLower(qn), strings.ToLower(v.Value)), nil

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

// CollectLeafValues walks a YAML node tree and returns all scalar "values"
// (i.e., leaf nodes). Mapping keys are ignored; only mapping values are traversed.
// Aliases are followed (cycle-safe). Null scalars are skipped.
func collectLeafValues(root *yaml.Node) []string {
	out := make([]string, 0, 16)
	visited := make(map[*yaml.Node]struct{})

	var walk func(*yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		if _, seen := visited[n]; seen {
			return
		}
		visited[n] = struct{}{}

		switch n.Kind {
		case yaml.DocumentNode:
			for _, c := range n.Content {
				walk(c)
			}
		case yaml.MappingNode:
			// Content is [k0, v0, k1, v1, ...]; collect only values.
			for i := 0; i+1 < len(n.Content); i += 2 {
				walk(n.Content[i+1])
			}
		case yaml.SequenceNode:
			for _, c := range n.Content {
				walk(c)
			}
		case yaml.AliasNode:
			walk(n.Alias)
		case yaml.ScalarNode:
			// Skip nulls; include other scalar types as their string value.
			if n.Tag != "!!null" {
				out = append(out, n.Value)
			}
		}
	}

	walk(root)
	return out
}
