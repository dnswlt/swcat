package query

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
)

// PropertyProvider extends the query language with attributes that are not
// fields of the entity itself, such as lint findings.
type PropertyProvider struct {
	// Names lists the attributes the provider supplies, in lower case.
	// Queries are validated against them before any entity is evaluated.
	Names []string
	// Values returns the values of the named attribute for e. The attribute
	// applies to every entity: nil means it has no values, not that it is unknown.
	Values func(e catalog.Entity, name string) []string
}

// Evaluator holds a validated query and matches it against entities.
// It is not modified by evaluation, so it can be shared between goroutines.
type Evaluator struct {
	expr      Expression
	regexes   map[string]*regexp.Regexp
	providers map[string]PropertyProvider
}

// Compile parses q and validates all of it before any entity is evaluated:
// attribute names must be built in or supplied by one of the providers, and
// regular expressions must compile. It is the only place where a query can be
// rejected; a compiled query can be evaluated against any entity.
func Compile(q string, providers ...PropertyProvider) (*Evaluator, error) {
	expr, err := Parse(q)
	if err != nil {
		return nil, err
	}
	ev := &Evaluator{
		expr:      expr,
		regexes:   make(map[string]*regexp.Regexp),
		providers: make(map[string]PropertyProvider),
	}
	for _, p := range providers {
		for _, name := range p.Names {
			ev.providers[name] = p
		}
	}
	if err := ev.compileNode(expr); err != nil {
		return nil, err
	}
	return ev, nil
}

// compileNode validates expr. It visits every node, so that the outcome does
// not depend on the catalog's contents or on short-circuit evaluation.
func (ev *Evaluator) compileNode(expr Expression) error {
	switch v := expr.(type) {
	case *Term:
		return nil
	case *AttributeTerm:
		attr := strings.ToLower(v.Attribute)
		if _, ok := attributeAccessors[attr]; !ok {
			if _, ok := ev.providers[attr]; !ok {
				return fmt.Errorf("unknown attribute for filtering: %s", v.Attribute)
			}
		}
		if v.Operator == "~" {
			if _, ok := ev.regexes[v.Value]; !ok {
				re, err := regexp.Compile("(?i)" + v.Value) // (?i) for case-insensitivity
				if err != nil {
					return fmt.Errorf("invalid regular expression %q: %w", v.Value, err)
				}
				ev.regexes[v.Value] = re
			}
		}
		return nil
	case *RelationshipExpression:
		if _, ok := relationshipAccessors[strings.ToLower(v.Relationship)]; !ok {
			return fmt.Errorf("unknown relationship for filtering: %s", v.Relationship)
		}
		return ev.compileNode(v.Expression)
	case *NotExpression:
		return ev.compileNode(v.Expression)
	case *BinaryExpression:
		if err := ev.compileNode(v.Left); err != nil {
			return err
		}
		return ev.compileNode(v.Right)
	}
	// Parse produces no other node types.
	panic(fmt.Sprintf("query: unsupported expression type %T", expr))
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
	if status := e.GetStatus(); status != nil {
		for k, v := range status.Observations {
			values = append(values, k, string(v.Value))
		}
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
	"*":    fulltextAccessor,
	"meta": metadataAccessor,
	// Canonical kind ("Component"); matching is case-insensitive so kind:component still works.
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
	"status": func(e catalog.Entity) ([]string, bool) {
		// For status, we match against "key=value";
		// value will typically be a JSON object.
		status := e.GetStatus()
		if status == nil {
			return nil, false
		}
		var results []string
		for k, obs := range status.Observations {
			results = append(results, fmt.Sprintf("%s=%s", k, string(obs.Value)))
		}
		return results, true
	},
	"owner":  relationshipAttribute("owner"),
	"system": relationshipAttribute("system"),
	"domain": relationshipAttribute("domain"),
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
	"consumesapis":   relationshipAttribute("consumesapis"),
	"providesapis":   relationshipAttribute("providesapis"),
	"dependson":      relationshipAttribute("dependson"),
	"dependents":     relationshipAttribute("dependents"),
	"providedby":     relationshipAttribute("providedby"),
	"consumedby":     relationshipAttribute("consumedby"),
	"subcomponentof": relationshipAttribute("subcomponentof"),
	"subcomponents":  relationshipAttribute("subcomponents"),
	"components":     relationshipAttribute("components"),
	"apis":           relationshipAttribute("apis"),
	"resources":      relationshipAttribute("resources"),
	"systems":        relationshipAttribute("systems"),
	"rel":            relationshipAttribute("rel"),
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

// Matches reports whether e matches the query. The resolver looks up the
// entities that relationship predicates refer to, in the catalog that e belongs
// to; unresolved references do not supply a matching witness.
//
// Compile has validated the query, so evaluation cannot fail. A nil resolver is
// a programming error and panics, whether or not the query uses relationships,
// so that it shows up regardless of the query and the entity.
func (ev *Evaluator) Matches(e catalog.Entity, resolver Resolver) bool {
	if resolver == nil {
		panic("query: Matches requires a resolver")
	}
	return ev.evaluateNode(e, ev.expr, resolver)
}

// evaluateNode recursively walks the expression tree.
func (ev *Evaluator) evaluateNode(e catalog.Entity, expr Expression, resolver Resolver) bool {
	switch v := expr.(type) {
	case *Term:
		// A simple term matches against the entity's qualified name.
		qn := e.GetRef().QName()
		return strings.Contains(strings.ToLower(qn), strings.ToLower(v.Value))

	case *AttributeTerm:
		attr := strings.ToLower(v.Attribute)
		var values []string
		if accessor, ok := attributeAccessors[attr]; ok {
			values, ok = accessor(e)
			if !ok {
				// Attribute is not applicable to this entity kind.
				return false
			}
		} else {
			// Compile has checked that a provider supplies the attribute.
			values = ev.providers[attr].Values(e, attr)
		}

		// Check if any of the returned values match the query value.
		for _, value := range values {
			if ev.matchesOperator(value, v.Operator, v.Value) {
				return true
			}
		}
		return false

	case *RelationshipExpression:
		accessor := relationshipAccessors[strings.ToLower(v.Relationship)]
		for _, ref := range accessor(e) {
			if target := resolver.Entity(ref); target != nil && ev.evaluateNode(target, v.Expression, resolver) {
				return true
			}
		}
		return false

	case *NotExpression:
		return !ev.evaluateNode(e, v.Expression, resolver)

	case *BinaryExpression:
		switch v.Operator {
		case "AND":
			return ev.evaluateNode(e, v.Left, resolver) && ev.evaluateNode(e, v.Right, resolver)
		case "OR":
			return ev.evaluateNode(e, v.Left, resolver) || ev.evaluateNode(e, v.Right, resolver)
		}
		panic(fmt.Sprintf("query: unsupported binary operator %q", v.Operator))
	}
	panic(fmt.Sprintf("query: unsupported expression type %T", expr))
}

// matchesOperator performs the actual string comparison based on the operator.
func (ev *Evaluator) matchesOperator(entityValue, operator, queryValue string) bool {
	switch operator {
	case ":":
		return strings.Contains(strings.ToLower(entityValue), strings.ToLower(queryValue))
	case "=":
		return strings.EqualFold(entityValue, queryValue)
	case "~":
		// Compile has compiled every regular expression in the query.
		return ev.regexes[queryValue].MatchString(entityValue)
	default:
		return false
	}
}
