package query

import "github.com/dnswlt/swcat/internal/catalog"

// Resolver looks up a relationship target in the same catalog as the candidate.
type Resolver interface {
	Entity(ref *catalog.Ref) catalog.Entity
}

type relationshipAccessor func(catalog.Entity) []*catalog.Ref

// Keep reference traversal and the corresponding string attributes in sync.
// In particular, traversal must preserve kind and namespace, not search by name.
var relationshipAccessors = map[string]relationshipAccessor{
	"owner": func(e catalog.Entity) []*catalog.Ref {
		return singleRef(e.GetOwner())
	},
	"system": func(e catalog.Entity) []*catalog.Ref {
		if part, ok := e.(catalog.SystemPart); ok {
			return singleRef(part.GetSystem())
		}
		return nil
	},
	"domain": func(e catalog.Entity) []*catalog.Ref {
		if part, ok := e.(catalog.DomainPart); ok {
			return singleRef(part.GetDomain())
		}
		return nil
	},
	"consumesapis": func(e catalog.Entity) []*catalog.Ref {
		if c, ok := e.(*catalog.Component); ok && c.Spec != nil {
			return labelRefs(c.Spec.ConsumesAPIs)
		}
		return nil
	},
	"providesapis": func(e catalog.Entity) []*catalog.Ref {
		if c, ok := e.(*catalog.Component); ok && c.Spec != nil {
			return labelRefs(c.Spec.ProvidesAPIs)
		}
		return nil
	},
	"providedby": func(e catalog.Entity) []*catalog.Ref {
		if a, ok := e.(*catalog.API); ok && a.Spec != nil {
			return labelRefs(a.GetProviders())
		}
		return nil
	},
	"consumedby": func(e catalog.Entity) []*catalog.Ref {
		if a, ok := e.(*catalog.API); ok && a.Spec != nil {
			return labelRefs(a.GetConsumers())
		}
		return nil
	},
	"dependson": func(e catalog.Entity) []*catalog.Ref {
		switch v := e.(type) {
		case *catalog.Component:
			if v.Spec != nil {
				return labelRefs(v.Spec.DependsOn)
			}
		case *catalog.Resource:
			if v.Spec != nil {
				return labelRefs(v.Spec.DependsOn)
			}
		}
		return nil
	},
	"dependents": func(e catalog.Entity) []*catalog.Ref {
		switch v := e.(type) {
		case *catalog.Component:
			if v.Spec != nil {
				return labelRefs(v.GetDependents())
			}
		case *catalog.Resource:
			if v.Spec != nil {
				return labelRefs(v.GetDependents())
			}
		}
		return nil
	},
	"rel": relatedEntities,
}

func singleRef(ref *catalog.Ref) []*catalog.Ref {
	if ref == nil {
		return nil
	}
	return []*catalog.Ref{ref}
}

func labelRefs(refs []*catalog.LabelRef) []*catalog.Ref {
	var result []*catalog.Ref
	for _, ref := range refs {
		if ref != nil && ref.Ref != nil {
			result = append(result, ref.Ref)
		}
	}
	return result
}

func relationshipAttribute(name string) attributeAccessor {
	return func(e catalog.Entity) ([]string, bool) {
		refs := relationshipAccessors[name](e)
		values := make([]string, 0, len(refs))
		for _, ref := range refs {
			if name == "rel" {
				values = append(values, ref.String())
			} else {
				values = append(values, ref.QName())
			}
		}
		return values, len(values) > 0
	}
}
