package backstage

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Repository struct {
	domains    map[string]*Domain
	systems    map[string]*System
	components map[string]*Component
	resources  map[string]*Resource
	apis       map[string]*API
	groups     map[string]*Group
	// Tracks all qualified names added to this repo
	// (for duplicate detection and type-independent lookups)
	allEntities map[string]Entity
}

func NewRepository() *Repository {
	return &Repository{
		domains:     make(map[string]*Domain),
		systems:     make(map[string]*System),
		components:  make(map[string]*Component),
		resources:   make(map[string]*Resource),
		apis:        make(map[string]*API),
		groups:      make(map[string]*Group),
		allEntities: make(map[string]Entity),
	}
}

func (r *Repository) Size() int {
	return len(r.allEntities)
}

func (r *Repository) AddEntity(e Entity) error {
	qname := e.GetQName()
	if _, ok := r.allEntities[qname]; ok {
		return fmt.Errorf("entity with qname %s already exists in the repository", qname)
	}

	switch x := e.(type) {
	case *Domain:
		r.domains[qname] = x
	case *System:
		r.systems[qname] = x
	case *Component:
		r.components[qname] = x
	case *Resource:
		r.resources[qname] = x
	case *API:
		r.apis[qname] = x
	case *Group:
		r.groups[qname] = x
	default:
		return fmt.Errorf("invalid type: %T", e)
	}

	r.allEntities[qname] = e

	return nil
}

func qname(name string) string {
	return name
}

func (r *Repository) Group(name string) *Group {
	qn := qname(name)
	return r.groups[qn]
}

func (r *Repository) System(name string) *System {
	qn := qname(name)
	return r.systems[qn]
}

func (r *Repository) Domain(name string) *Domain {
	qn := qname(name)
	return r.domains[qn]
}

func (r *Repository) API(name string) *API {
	qn := qname(name)
	return r.apis[qn]
}

func (r *Repository) Component(name string) *Component {
	qn := qname(name)
	return r.components[qn]
}

func (r *Repository) Resource(name string) *Resource {
	qn := qname(name)
	return r.resources[qn]
}

func (r *Repository) Entity(ref string) Entity {
	kind, name, found := strings.Cut(ref, ":")
	if !found {
		return nil // Entity lookup requires kind specifier
	}
	switch kind {
	case "component":
		return r.Component(name)
	case "system":
		return r.System(name)
	case "domain":
		return r.Domain(name)
	case "api":
		return r.API(name)
	case "resource":
		return r.Resource(name)
	case "group":
		return r.Group(name)
	}
	return nil
}

func (r *Repository) FindComponents(query string) []*Component {
	var result []*Component
	for _, c := range r.components {
		if strings.Contains(c.GetQName(), query) {
			result = append(result, c)
		}
	}
	slices.SortFunc(result, func(c1, c2 *Component) int {
		return cmp.Compare(c1.GetQName(), c2.GetQName())
	})
	return result
}

func (r *Repository) FindSystems(query string) []*System {
	var result []*System
	for _, s := range r.systems {
		if strings.Contains(s.GetQName(), query) {
			result = append(result, s)
		}
	}
	slices.SortFunc(result, func(s1, s2 *System) int {
		return cmp.Compare(s1.GetQName(), s2.GetQName())
	})
	return result
}

func (r *Repository) FindAPIs(query string) []*API {
	var result []*API
	for _, a := range r.apis {
		if strings.Contains(a.GetQName(), query) {
			result = append(result, a)
		}
	}
	slices.SortFunc(result, func(a1, a2 *API) int {
		return cmp.Compare(a1.GetQName(), a2.GetQName())
	})
	return result
}

func (r *Repository) FindResources(query string) []*Resource {
	var result []*Resource
	for _, res := range r.resources {
		if strings.Contains(res.GetQName(), query) {
			result = append(result, res)
		}
	}
	slices.SortFunc(result, func(r1, r2 *Resource) int {
		return cmp.Compare(r1.GetQName(), r2.GetQName())
	})
	return result
}

func (r *Repository) FindDomains(query string) []*Domain {
	var result []*Domain
	for _, d := range r.domains {
		if strings.Contains(d.GetQName(), query) {
			result = append(result, d)
		}
	}
	slices.SortFunc(result, func(d1, d2 *Domain) int {
		return cmp.Compare(d1.GetQName(), d2.GetQName())
	})
	return result
}

func (r *Repository) FindGroups(query string) []*Group {
	var result []*Group
	for _, g := range r.groups {
		if strings.Contains(g.GetQName(), query) {
			result = append(result, g)
		}
	}
	slices.SortFunc(result, func(g1, g2 *Group) int {
		return cmp.Compare(g1.GetQName(), g2.GetQName())
	})
	return result
}

var (
	// Regexp to check for valid entity names
	validNameRE = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_-]*$")
)

func (r *Repository) validateMetadata(m *Metadata) error {
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
		if g.Spec.Profile == nil {
			// Avoid nil checks elsewhere. Profile is optional according to the spec.
			g.Spec.Profile = &GroupSpecProfile{}
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
			if api := r.API(a); api == nil {
				return fmt.Errorf("provided API %q for component %s is undefined", a, qn)
			}
		}
		for _, a := range s.ConsumesAPIs {
			if api := r.API(a); api == nil {
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
	for qn, api := range r.apis {
		if err := r.validateMetadata(api.Metadata); err != nil {
			return fmt.Errorf("API %s has invalid metadata: %v", qn, err)
		}
		s := api.Spec
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
		if s.Definition == "" {
			return fmt.Errorf("API %s has no spec.definition (you can set it to 'missing')", qn)
		}

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

	// Validation succeeded: populated computed fields
	r.populateRelationships()
	return nil
}

// populateRelationships populates the "inverse relationship" fields of entities.
// Assumes that the repository has been validated already.
func (r *Repository) populateRelationships() {

	registerDependent := func(entityRef, depName string) {
		dep := r.Entity(depName)
		switch x := dep.(type) {
		case *Component:
			x.Spec.dependents = append(x.Spec.dependents, entityRef)
		case *Resource:
			x.Spec.dependents = append(x.Spec.dependents, entityRef)
		default:
			// Ignore: we only track dependents for components and resources for now.
		}
	}

	// Components
	for _, c := range r.components {
		qn := c.GetQName()
		// Register in APIs
		for _, a := range c.Spec.ConsumesAPIs {
			api := r.API(a)
			api.Spec.consumers = append(api.Spec.consumers, qn)
		}
		for _, a := range c.Spec.ProvidesAPIs {
			api := r.API(a)
			api.Spec.providers = append(api.Spec.providers, qn)
		}
		if s := c.Spec.System; s != "" {
			system := r.System(s)
			system.Spec.components = append(system.Spec.components, qn)
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
			system.Spec.resources = append(system.Spec.resources, qn)
		}
		// Register in "DependsOn" dependencies.
		for _, d := range res.Spec.DependsOn {
			registerDependent("resource:"+qn, d)
		}
	}

	// APIs
	for _, api := range r.apis {
		qn := api.GetQName()
		if s := api.Spec.System; s != "" {
			system := r.System(s)
			system.Spec.apis = append(system.Spec.apis, qn)
		}
	}

	// Systems
	for _, system := range r.systems {
		qn := system.GetQName()
		if d := system.Spec.Domain; d != "" {
			domain := r.Domain(d)
			domain.Spec.systems = append(domain.Spec.systems, qn)
		}
	}

}
