package backstage

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/query"
)

type Repository struct {
	// Maps containing the different kinds of entities in the repository.
	//
	// These maps are keyed by qnames without the kind: specifier: <namespace>/<name>
	domains    map[string]*api.Domain
	systems    map[string]*api.System
	components map[string]*api.Component
	resources  map[string]*api.Resource
	apis       map[string]*api.API
	groups     map[string]*api.Group
	// Tracks all qualified names added to this repo
	// (for duplicate detection and type-independent lookups)
	//
	// This map uses entity references including the kind: prefix: <kind>:<namespace>/<name>
	allEntities map[string]api.Entity
}

func NewRepository() *Repository {
	return &Repository{
		domains:     make(map[string]*api.Domain),
		systems:     make(map[string]*api.System),
		components:  make(map[string]*api.Component),
		resources:   make(map[string]*api.Resource),
		apis:        make(map[string]*api.API),
		groups:      make(map[string]*api.Group),
		allEntities: make(map[string]api.Entity),
	}
}

func (r *Repository) Size() int {
	return len(r.allEntities)
}

func (r *Repository) setEntity(e api.Entity) error {
	qname := e.GetQName()

	switch x := e.(type) {
	case *api.Domain:
		r.domains[qname] = x
	case *api.System:
		r.systems[qname] = x
	case *api.Component:
		r.components[qname] = x
	case *api.Resource:
		r.resources[qname] = x
	case *api.API:
		r.apis[qname] = x
	case *api.Group:
		r.groups[qname] = x
	default:
		return fmt.Errorf("invalid type: %T", e)
	}

	ref := e.GetRef()
	r.allEntities[ref] = e
	return nil
}

func (r *Repository) UpdateEntity(e api.Entity) error {
	// This is a fairly heavyweight, but effective approach:
	// Rebuild the repository from scratch (as a copy), validate, copy the maps back.
	// It avoids having to deal with complex deletions and additions of relationships
	// and their inverses.

	eRef := e.GetRef()
	if _, ok := r.allEntities[eRef]; !ok {
		return fmt.Errorf("entity %q does not exist in the repository", eRef)
	}

	r2 := NewRepository()
	for _, n := range r.allEntities {
		var toAdd api.Entity
		if n.GetRef() == eRef {
			toAdd = e // Replace old entity by the new one
		} else {
			toAdd = n.Reset() // Add a shallow copy with cleared computed fields
		}

		if err := r2.AddEntity(toAdd); err != nil {
			return fmt.Errorf("failed to rebuild repository: %v", err)
		}
	}

	if err := r2.Validate(); err != nil {
		return fmt.Errorf("repository validation failed: %v", err)
	}

	// Copy over all data from the updated repo to the current one.
	*r = *r2

	return nil
}

func (r *Repository) AddEntity(e api.Entity) error {
	ref := e.GetRef()
	if _, ok := r.allEntities[ref]; ok {
		return fmt.Errorf("entity %q already exists in the repository", ref)
	}
	return r.setEntity(e)
}

func getEntity[T any](m map[string]*T, ref, expectedKind string) *T {
	kind, qn, found := strings.Cut(ref, ":")
	if !found {
		return m[ref]
	}
	if kind != expectedKind {
		return nil
	}
	return m[qn]
}

func (r *Repository) Group(ref string) *api.Group {
	return getEntity(r.groups, ref, "group")
}

func (r *Repository) System(ref string) *api.System {
	return getEntity(r.systems, ref, "system")
}

func (r *Repository) Domain(ref string) *api.Domain {
	return getEntity(r.domains, ref, "domain")
}

func (r *Repository) API(ref string) *api.API {
	return getEntity(r.apis, ref, "api")
}

func (r *Repository) Component(ref string) *api.Component {
	return getEntity(r.components, ref, "component")
}

func (r *Repository) Resource(ref string) *api.Resource {
	return getEntity(r.resources, ref, "resource")
}

func (r *Repository) Entity(ref string) api.Entity {
	kind, qn, found := strings.Cut(ref, ":")
	if !found {
		return nil // Entity lookup requires kind specifier
	}
	switch kind {
	case "component":
		return r.Component(qn)
	case "system":
		return r.System(qn)
	case "domain":
		return r.Domain(qn)
	case "api":
		return r.API(qn)
	case "resource":
		return r.Resource(qn)
	case "group":
		return r.Group(qn)
	}
	return nil
}

func findEntities[T api.Entity](q string, items map[string]T) []T {
	var result []T

	if strings.TrimSpace(q) == "" {
		// No filter, return all items
		result = make([]T, 0, len(items))
		for _, item := range items {
			result = append(result, item)
		}
	} else {
		expr, err := query.Parse(q)
		if err != nil {
			return nil // Invalid query => no results
		}
		ev := query.NewEvaluator(expr)
		for _, c := range items {
			ok, err := ev.Matches(c)
			if err != nil {
				return nil // Broken query (e.g. broken regex) => no results
			}
			if ok {
				result = append(result, c)
			}
		}
	}
	slices.SortFunc(result, func(c1, c2 T) int {
		return api.CompareEntityByName(c1, c2)
	})
	return result
}

func (r *Repository) FindComponents(q string) []*api.Component {
	return findEntities(q, r.components)
}

func (r *Repository) FindSystems(q string) []*api.System {
	return findEntities(q, r.systems)
}

func (r *Repository) FindAPIs(q string) []*api.API {
	return findEntities(q, r.apis)
}

func (r *Repository) FindResources(q string) []*api.Resource {
	return findEntities(q, r.resources)
}

func (r *Repository) FindDomains(q string) []*api.Domain {
	return findEntities(q, r.domains)
}

func (r *Repository) FindGroups(q string) []*api.Group {
	return findEntities(q, r.groups)
}

var (
	// Regexp to check for valid entity names
	validNameRE = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_-]*$")
)

func (r *Repository) validateMetadata(m *api.Metadata) error {
	if m == nil {
		return fmt.Errorf("metadata is null")
	}
	if !validNameRE.MatchString(m.Name) {
		return fmt.Errorf("invalid name: %s", m.Name)
	}
	if m.Namespace != "" && !validNameRE.MatchString(m.Namespace) {
		return fmt.Errorf("invalid namespace: %s", m.Namespace)
	}
	return nil
}

// validDependsOnRef checks if ref is a valid fully qualified entity reference.
// It must include the entity type, e.g. "component:my-namespace/foo", or "component:bar".
// For now, only component and resource dependencies are supported.
func validDependsOnRef(ref string) error {
	kind, qname, found := strings.Cut(ref, ":")
	if !found {
		return fmt.Errorf("entity kind is missing in entity ref %q", ref)
	}
	if kind != "component" && kind != "resource" {
		return fmt.Errorf("invalid entity kind %q", kind)
	}
	if validNameRE.MatchString(qname) {
		// Entity without namespace
		return nil
	}
	ns, name, found := strings.Cut(qname, "/")
	if !found {
		return fmt.Errorf("not a valid qualified entity name %q", qname)
	}
	if !validNameRE.MatchString(ns) {
		return fmt.Errorf("not a valid namespace name %q", ns)
	}
	if !validNameRE.MatchString(name) {
		return fmt.Errorf("not a valid entity name %q", name)
	}
	return nil
}

// Validate validates the repository (cross references exist, etc.).
func (r *Repository) Validate() error {
	// Groups
	for qn, g := range r.groups {
		if err := r.validateMetadata(g.Metadata); err != nil {
			return fmt.Errorf("group %s has invalid metadata: %v", qn, err)
		}
		if g.Spec == nil {
			return fmt.Errorf("group %s has no spec", qn)
		}
		if g.Spec.Type == "" {
			return fmt.Errorf("group %s has no spec.type", qn)
		}
		if g.Spec.Profile == nil {
			// Avoid nil checks elsewhere. Profile is optional according to the spec.
			g.Spec.Profile = &api.GroupSpecProfile{}
		}
	}

	// Components
	for qn, c := range r.components {
		if err := r.validateMetadata(c.Metadata); err != nil {
			return fmt.Errorf("component %s has invalid metadata: %v", qn, err)
		}
		s := c.Spec
		if s == nil {
			return fmt.Errorf("component %s has no spec", qn)
		}
		if s.Type == "" {
			return fmt.Errorf("component %s has no spec.type", qn)
		}
		if s.Lifecycle == "" {
			return fmt.Errorf("component %s has no spec.lifecycle", qn)
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for component %s is undefined", s.Owner, qn)
		}

		if s.System != "" {
			if system := r.System(s.System); system == nil {
				return fmt.Errorf("system %q for component %s is undefined", s.System, qn)
			}
		}

		for _, a := range s.ProvidesAPIs {
			if ap := r.API(a); ap == nil {
				return fmt.Errorf("provided API %q for component %s is undefined", a, qn)
			}
		}
		for _, a := range s.ConsumesAPIs {
			if ap := r.API(a); ap == nil {
				return fmt.Errorf("consumed API %q for component %s is undefined", a, qn)
			}
		}
		for _, a := range s.DependsOn {
			if err := validDependsOnRef(a); err != nil {
				return fmt.Errorf("invalid entity reference in dependency %q for component %s: %v ", a, qn, err)
			}
			if e := r.Entity(a); e == nil {
				return fmt.Errorf("dependency %q for component %s is undefined", a, qn)
			}
		}
	}

	// APIs
	for qn, ap := range r.apis {
		if err := r.validateMetadata(ap.Metadata); err != nil {
			return fmt.Errorf("API %s has invalid metadata: %v", qn, err)
		}
		s := ap.Spec
		if s == nil {
			return fmt.Errorf("API %s has no spec", qn)
		}
		if s.Type == "" {
			return fmt.Errorf("API %s has no spec.type", qn)
		}
		if s.Lifecycle == "" {
			return fmt.Errorf("API %s has no spec.lifecycle", qn)
		}
		if s.System != "" {
			if system := r.System(s.System); system == nil {
				return fmt.Errorf("system %q for API %s is undefined", s.System, qn)
			}
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for API %s is undefined", s.Owner, qn)
		}
		// Allow an empty Definition.
		// It is mandatory in the backstage.io/v1alpha1 schema, but this just
		// forces us to set it to "n/a" all over the place.
	}

	// Resources
	for qn, res := range r.resources {
		if err := r.validateMetadata(res.Metadata); err != nil {
			return fmt.Errorf("resource %s has invalid metadata: %v", qn, err)
		}
		s := res.Spec
		if s == nil {
			return fmt.Errorf("resource %s has no spec", qn)
		}
		if s.Type == "" {
			return fmt.Errorf("resource %s has no spec.type", qn)
		}
		if s.System != "" && r.System(s.System) == nil {
			return fmt.Errorf("system %q for resource %s is undefined", s.System, qn)
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for resource %s is undefined", s.Owner, qn)
		}
		for _, a := range s.DependsOn {
			if err := validDependsOnRef(a); err != nil {
				return fmt.Errorf("invalid entity reference in dependency %q for component %s: %v ", a, qn, err)
			}
			if e := r.Entity(a); e == nil {
				return fmt.Errorf("dependency %q for resource %s is undefined", a, qn)
			}
		}
	}

	// Systems
	for qn, system := range r.systems {
		if err := r.validateMetadata(system.Metadata); err != nil {
			return fmt.Errorf("system %s has invalid metadata: %v", qn, err)
		}
		s := system.Spec
		if s == nil {
			return fmt.Errorf("system %s has no spec", qn)
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for system %s is undefined", s.Owner, qn)
		}
		if d := r.Domain(s.Domain); d == nil {
			return fmt.Errorf("domain %q for system %s is undefined", s.Domain, qn)
		}
	}

	// Domains
	for qn, dom := range r.domains {
		if err := r.validateMetadata(dom.Metadata); err != nil {
			return fmt.Errorf("domain %s has invalid metadata: %v", qn, err)
		}
		s := dom.Spec
		if s == nil {
			return fmt.Errorf("domain %s has no spec", qn)
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for domain %s is undefined", s.Owner, qn)
		}
	}

	// Validation succeeded: populate computed fields
	r.populateRelationships()
	return nil
}

// populateRelationships populates the "inverse relationship" fields of entities.
// Assumes that the repository has been validated already.
func (r *Repository) populateRelationships() {

	registerDependent := func(entityRef, depName string) {
		dep := r.Entity(depName)
		switch x := dep.(type) {
		case *api.Component:
			x.AddDependent(entityRef)
		case *api.Resource:
			x.AddDependent(entityRef)
		default:
			// Ignore: we only track dependents for components and resources for now.
		}
	}

	// Components
	for _, c := range r.components {
		qn := c.GetQName()
		// Register in APIs
		for _, a := range c.Spec.ConsumesAPIs {
			ap := r.API(a)
			ap.AddConsumer(qn)
		}
		for _, a := range c.Spec.ProvidesAPIs {
			ap := r.API(a)
			ap.AddProvider(qn)
		}
		if s := c.Spec.System; s != "" {
			system := r.System(s)
			system.AddComponent(qn)
		}

		// Register in "DependsOn" dependencies.
		for _, d := range c.Spec.DependsOn {
			registerDependent("component:"+qn, d)
		}
	}

	// Resources
	for _, res := range r.resources {
		qn := res.GetQName()
		if s := res.Spec.System; s != "" {
			system := r.System(s)
			system.AddResource(qn)
		}
		// Register in "DependsOn" dependencies.
		for _, d := range res.Spec.DependsOn {
			registerDependent("resource:"+qn, d)
		}
	}

	// APIs
	for _, ap := range r.apis {
		qn := ap.GetQName()
		if s := ap.Spec.System; s != "" {
			system := r.System(s)
			system.AddAPI(qn)
		}
	}

	// Systems
	for _, system := range r.systems {
		qn := system.GetQName()
		if d := system.Spec.Domain; d != "" {
			domain := r.Domain(d)
			domain.AddSystem(qn)
		}
	}

}
