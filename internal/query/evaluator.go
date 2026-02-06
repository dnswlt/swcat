package query

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
)

// PropertyProvider is a function that retrieves external properties for an entity.
// It returns a slice of string values and a boolean indicating if the property was found.
type PropertyProvider func(e catalog.Entity, prop string) ([]string, bool)

// Evaluator holds a compiled query expression and provides methods to match it against entities.
// It caches compiled regular expressions for performance.
type Evaluator struct {
	expr       Expression
	regexCache map[string]*regexp.Regexp
	providers  []PropertyProvider
}

// NewEvaluator creates a new Evaluator for the given expression AST.
func NewEvaluator(expr Expression, providers ...PropertyProvider) *Evaluator {
	return &Evaluator{
		expr:       expr,
		regexCache: make(map[string]*regexp.Regexp),
		providers:  providers,
	}
}

// fulltextAccessor collects all relevant searchable text from an entity.
func fulltextAccessor(e catalog.Entity) ([]string, bool) {
	values, _ := metadataAccessor(e)

	// 2. Spec (kind-specific scalar fields)
	switch v := e.(type) {
	case *catalog.Component:
		if v.Spec != nil {
			values = append(values, v.Spec.Type, v.Spec.Lifecycle)
		}
	case *catalog.API:
		if v.Spec != nil {
			values = append(values, v.Spec.Type, v.Spec.Lifecycle, v.Spec.Definition)
		}
	case *catalog.Resource:
		if v.Spec != nil {
			values = append(values, v.Spec.Type)
		}
	case *catalog.System:
		if v.Spec != nil {
			values = append(values, v.Spec.Type)
		}
	case *catalog.Domain:
		if v.Spec != nil {
			values = append(values, v.Spec.Type)
		}
	case *catalog.Group:
		if v.Spec != nil {
			values = append(values, v.Spec.Type)
			if v.Spec.Profile != nil {
				values = append(values, v.Spec.Profile.DisplayName, v.Spec.Profile.Email)
			}
			values = append(values, v.Spec.Members...)
		}
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
		// For labels, we match against "key=value"
		var results []string
		for k, v := range e.GetMetadata().Labels {
			results = append(results, fmt.Sprintf("%s=%s", k, v))
		}
		return results, true
	},
	"annotation": func(e catalog.Entity) ([]string, bool) {
		// For annotations, we match against "key=value"
		var results []string
		for k, v := range e.GetMetadata().Annotations {
			results = append(results, fmt.Sprintf("%s=%s", k, v))
		}
		return results, true
	},
	"owner": func(e catalog.Entity) ([]string, bool) {
		if o := e.GetOwner(); o != nil {
			return []string{o.QName()}, true
		}
		return nil, false // No owner
	},
	"system": func(e catalog.Entity) ([]string, bool) {
		sp, ok := e.(catalog.SystemPart)
		if !ok {
			return nil, false
		}
		return []string{sp.GetSystem().QName()}, true
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
	"consumesapis": func(e catalog.Entity) ([]string, bool) {
		switch v := e.(type) {
		case *catalog.Component:
			if v.Spec == nil {
				return nil, false
			}
			var results []string
			for _, a := range v.Spec.ConsumesAPIs {
				results = append(results, a.QName())
			}
			return results, true
		default:
			return nil, false
		}
	},
	"providesapis": func(e catalog.Entity) ([]string, bool) {
		switch v := e.(type) {
		case *catalog.Component:
			if v.Spec == nil {
				return nil, false
			}
			var results []string
			for _, a := range v.Spec.ProvidesAPIs {
				results = append(results, a.QName())
			}
			return results, true
		default:
			return nil, false
		}
	},
	"dependson": func(e catalog.Entity) ([]string, bool) {
		var deps []*catalog.LabelRef
		switch v := e.(type) {
		case *catalog.Component:
			if v.Spec != nil {
				deps = v.Spec.DependsOn
			}
		case *catalog.Resource:
			if v.Spec != nil {
				deps = v.Spec.DependsOn
			}
		default:
			return nil, false
		}
		var results []string
		for _, d := range deps {
			results = append(results, d.Ref.QName())
		}
		return results, true
	},
	"dependents": func(e catalog.Entity) ([]string, bool) {
		var deps []*catalog.LabelRef
		switch v := e.(type) {
		case *catalog.Component:
			deps = v.GetDependents()
		case *catalog.Resource:
			deps = v.GetDependents()
		default:
			return nil, false
		}
		var results []string
		for _, d := range deps {
			results = append(results, d.Ref.QName())
		}
		return results, true
	},
	"providedby": func(e catalog.Entity) ([]string, bool) {
		switch v := e.(type) {
		case *catalog.API:
			var results []string
			for _, lr := range v.GetProviders() {
				results = append(results, lr.Ref.QName())
			}
			return results, true
		default:
			return nil, false
		}
	},
	"consumedby": func(e catalog.Entity) ([]string, bool) {
		switch v := e.(type) {
		case *catalog.API:
			var results []string
			for _, lr := range v.GetConsumers() {
				results = append(results, lr.Ref.QName())
			}
			return results, true
		default:
			return nil, false
		}
	},
	"rel": func(e catalog.Entity) ([]string, bool) {
		refs := relatedEntities(e)
		if len(refs) == 0 {
			return nil, false
		}
		var results []string
		for _, r := range refs {
			results = append(results, r.String())
		}
		return results, true
	},
}

// relatedEntities returns a slice of references to all entities that are directly
// related to the given entity (both incoming and outgoing).
func relatedEntities(e catalog.Entity) []*catalog.Ref {
	var refs []*catalog.Ref
	seen := make(map[string]bool)
	self := e.GetRef()
	seen[self.String()] = true

	add := func(r *catalog.Ref) {
		if r == nil {
			return
		}
		s := r.String()
		if !seen[s] {
			seen[s] = true
			refs = append(refs, r)
		}
	}
	addLRs := func(lrs []*catalog.LabelRef) {
		for _, lr := range lrs {
			if lr != nil {
				add(lr.Ref)
			}
		}
	}
	addRefs := func(rs []*catalog.Ref) {
		for _, r := range rs {
			add(r)
		}
	}

	add(e.GetOwner())
	if sp, ok := e.(catalog.SystemPart); ok {
		add(sp.GetSystem())
	}

	switch v := e.(type) {
	case *catalog.Component:
		if v.Spec != nil {
			add(v.Spec.SubcomponentOf)
			addLRs(v.Spec.ProvidesAPIs)
			addLRs(v.Spec.ConsumesAPIs)
			addLRs(v.Spec.DependsOn)
			addLRs(v.GetDependents())
			addRefs(v.GetSubcomponents())
		}
	case *catalog.API:
		if v.Spec != nil {
			addLRs(v.GetProviders())
			addLRs(v.GetConsumers())
		}
	case *catalog.Resource:
		if v.Spec != nil {
			addLRs(v.Spec.DependsOn)
			addLRs(v.GetDependents())
		}
	case *catalog.System:
		if v.Spec != nil {
			add(v.Spec.Domain)
			addRefs(v.GetComponents())
			addRefs(v.GetAPIs())
			addRefs(v.GetResources())
		}
	case *catalog.Domain:
		if v.Spec != nil {
			add(v.Spec.SubdomainOf)
			addRefs(v.GetSystems())
		}
	case *catalog.Group:
		if v.Spec != nil {
			addRefs(v.Spec.Children)
		}
	}
	return refs
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
		var values []string
		if ok {
			values, ok = accessor(e)
		} else {
			// Try external providers
			for _, p := range ev.providers {
				values, ok = p(e, attr)
				if ok {
					break
				}
			}
			if !ok {
				return false, fmt.Errorf("unknown attribute for filtering: %s", v.Attribute)
			}
		}
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
	case "=":
		return strings.EqualFold(entityValue, queryValue), nil
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
