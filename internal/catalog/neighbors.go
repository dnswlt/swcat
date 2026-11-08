package catalog

// Resolver looks up catalog entities by reference, for code that has to follow
// references through the catalog, e.g. to find the consumers and providers of
// an API. *repo.Repository satisfies this interface.
type Resolver interface {
	Entity(ref *Ref) Entity
}

// DeclaredNeighbors returns the set (keyed by ref string) of Component entities
// that share an undirected, declared edge with c: via dependsOn (in either
// direction), or through a shared API (c provides an API the other consumes, or
// consumes one it provides). c itself is never included.
//
// A relationship between c and another Component counts as declared in the
// catalog iff the other Component is in this set. The comparison is undirected
// on purpose: it answers whether the catalog models a relationship at all, not
// its direction or exact type.
func DeclaredNeighbors(c *Component, resolver Resolver) map[string]bool {
	neighbors := make(map[string]bool)
	add := func(r *Ref) {
		if r != nil && r.Kind == KindComponent {
			neighbors[r.String()] = true
		}
	}

	// Declared dependencies and their inverses (components that depend on c).
	for _, d := range c.Spec.DependsOn {
		add(d.Ref)
	}
	for _, d := range c.GetDependents() {
		add(d.Ref)
	}

	// Components reachable through a shared API.
	for _, a := range c.Spec.ProvidesAPIs {
		if api := lookupAPI(resolver, a.Ref); api != nil {
			for _, consumer := range api.GetConsumers() {
				add(consumer.Ref)
			}
		}
	}
	for _, a := range c.Spec.ConsumesAPIs {
		if api := lookupAPI(resolver, a.Ref); api != nil {
			for _, provider := range api.GetProviders() {
				add(provider.Ref)
			}
		}
	}

	delete(neighbors, c.GetRef().String()) // never treat self as a neighbor
	return neighbors
}

func lookupAPI(resolver Resolver, ref *Ref) *API {
	if ref == nil {
		return nil
	}
	if a, ok := resolver.Entity(ref).(*API); ok {
		return a
	}
	return nil
}
