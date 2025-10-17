package repo

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/query"
	"github.com/dnswlt/swcat/internal/store"
)

// Config holds repository-specific application configuration.
// It is part of the config.Bundle that gets loaded at application startup.
type Config struct {
	// Prefix used to generate links to the source repository
	// if the annotation "swcat/repo: default" is set.
	// This is useful in setups where individual components and APIs live in
	// separate repositories.
	//
	// Example:
	//   repositoryURLPrefix: https://github.com/some-user
	// In this case, each entity that has the "swcat/repo: default"
	// annotation set will have an auto-generated metadata.links entry
	// pointing to "https://github.com/example/some-user".
	RepositoryURLPrefix string `yaml:"repositoryURLPrefix"`
}

type Repository struct {
	// Maps containing the different kinds of entities in the repository.
	//
	// These maps are keyed by qnames without the kind: specifier: <namespace>/<name>
	domains    map[string]*catalog.Domain
	systems    map[string]*catalog.System
	components map[string]*catalog.Component
	resources  map[string]*catalog.Resource
	apis       map[string]*catalog.API
	groups     map[string]*catalog.Group
	// Tracks all qualified names added to this repo
	// (for duplicate detection and type-independent lookups)
	//
	// This map uses entity references including the kind: prefix: <kind>:<namespace>/<name>
	allEntities map[string]catalog.Entity

	// Repository configuration
	config Config
}

func NewRepositoryWithConfig(config Config) *Repository {
	return &Repository{
		domains:     make(map[string]*catalog.Domain),
		systems:     make(map[string]*catalog.System),
		components:  make(map[string]*catalog.Component),
		resources:   make(map[string]*catalog.Resource),
		apis:        make(map[string]*catalog.API),
		groups:      make(map[string]*catalog.Group),
		allEntities: make(map[string]catalog.Entity),
		config:      config,
	}
}

func NewRepository() *Repository {
	return NewRepositoryWithConfig(Config{})
}

func (r *Repository) Size() int {
	return len(r.allEntities)
}

func (r *Repository) setEntity(e catalog.Entity) error {
	qname := e.GetRef().QName()

	switch x := e.(type) {
	case *catalog.Domain:
		r.domains[qname] = x
	case *catalog.System:
		r.systems[qname] = x
	case *catalog.Component:
		r.components[qname] = x
	case *catalog.Resource:
		r.resources[qname] = x
	case *catalog.API:
		r.apis[qname] = x
	case *catalog.Group:
		r.groups[qname] = x
	default:
		return fmt.Errorf("invalid type: %T", e)
	}

	ref := e.GetRef().String()
	r.allEntities[ref] = e
	return nil
}

func (r *Repository) Exists(e catalog.Entity) bool {
	_, ok := r.allEntities[e.GetRef().String()]
	return ok
}

// InsertOrUpdateEntity inserts e into the repository or updates an existing version of e.
//
// This method uses a fairly heavyweight, but effective approach:
// Rebuild the repository from scratch (as a copy), validate, copy the maps back.
// It avoids having to deal with complex deletions and additions of relationships
// and their inverses. If the operation fails, the repository will be in an unchanged state.
func (r *Repository) InsertOrUpdateEntity(e catalog.Entity) error {
	r2 := NewRepositoryWithConfig(r.config)
	ref := e.GetRef()
	found := false
	for _, n := range r.allEntities {
		var toAdd catalog.Entity
		if n.GetRef().Equal(ref) {
			found = true
			toAdd = e // Replace old entity by the new one
		} else {
			toAdd = n.Reset() // Add a shallow copy with cleared computed fields
		}

		if err := r2.AddEntity(toAdd); err != nil {
			return fmt.Errorf("failed to rebuild repository: %v", err)
		}
	}
	if !found {
		// e not found in repository => insert
		if err := r2.AddEntity(e); err != nil {
			return fmt.Errorf("failed to insert new entity: %v", err)
		}
	}

	if err := r2.Validate(); err != nil {
		return fmt.Errorf("repository validation failed: %v", err)
	}

	// Copy over all data from the updated repo to the current one.
	*r = *r2

	return nil
}

// DeleteEntity removes the given entity from the repository.
// Deletions are only allowed if the given entity does not have remaining
// ingoing dependencies (i.e. references from other entities) of any kind.
// See InsertOrUpdateEntity for the procedure.
func (r *Repository) DeleteEntity(ref *catalog.Ref) error {
	refList := func(refs []*catalog.LabelRef) []string {
		result := make([]string, len(refs))
		for i, ref := range refs {
			result[i] = ref.String()
		}
		return result
	}

	// Validate that there are no inbound dependencies left.
	e := r.Entity(ref)
	if e == nil {
		return fmt.Errorf("entity %q does not exist", ref)
	}
	switch entity := e.(type) {
	case *catalog.Component:
		if len(entity.GetDependents()) != 0 {
			return fmt.Errorf("remaining ingoing dependencies: %v", refList(entity.GetDependents()))
		}
	case *catalog.Resource:
		if len(entity.GetDependents()) != 0 {
			return fmt.Errorf("remaining ingoing dependencies: %v", refList(entity.GetDependents()))
		}
	case *catalog.API:
		if len(entity.GetProviders()) != 0 {
			return fmt.Errorf("remaining API providers: %v", refList(entity.GetProviders()))
		}
		if len(entity.GetConsumers()) != 0 {
			return fmt.Errorf("remaining API providers: %v", refList(entity.GetConsumers()))
		}
	default:
		return fmt.Errorf("deleting entities of type %T is currently not supported", e)
	}

	// Rebuild repo without entity
	r2 := NewRepositoryWithConfig(r.config)
	for _, n := range r.allEntities {
		if n.GetRef().Equal(ref) {
			continue // Skip the entity to be deleted
		}
		toAdd := n.Reset()
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

// AddEntity adds an entity to the repository *during construction*.
// This method is intended to be used while a repository is constructed,
// but before it is validated and back-references etc. are built.
// See InsertOrUpdateEntity for operations on an "active" repository.
func (r *Repository) AddEntity(e catalog.Entity) error {
	if e.GetMetadata() == nil {
		return fmt.Errorf("entity metadata is nil")
	}
	if r.Exists(e) {
		return fmt.Errorf("entity %q already exists in the repository", e.GetRef())
	}
	return r.setEntity(e)
}

func getEntity[T any](m map[string]*T, ref *catalog.Ref, expectedKind catalog.Kind) *T {
	if ref.Kind != "" && ref.Kind != expectedKind {
		return nil
	}
	return m[ref.QName()]
}

func (r *Repository) Group(ref *catalog.Ref) *catalog.Group {
	return getEntity(r.groups, ref, catalog.KindGroup)
}

func (r *Repository) System(ref *catalog.Ref) *catalog.System {
	return getEntity(r.systems, ref, catalog.KindSystem)
}

func (r *Repository) Domain(ref *catalog.Ref) *catalog.Domain {
	return getEntity(r.domains, ref, catalog.KindDomain)
}

func (r *Repository) API(ref *catalog.Ref) *catalog.API {
	return getEntity(r.apis, ref, catalog.KindAPI)
}

func (r *Repository) Component(ref *catalog.Ref) *catalog.Component {
	return getEntity(r.components, ref, catalog.KindComponent)
}

func (r *Repository) Resource(ref *catalog.Ref) *catalog.Resource {
	return getEntity(r.resources, ref, catalog.KindResource)
}

// Entity returns the entity identified by the entity reference ref, if it exists.
// If the entity does not exist, it returns the nil interface.
// The entity reference must be fully qualified, i.e. <kind>:[<namespace>/]<name>
func (r *Repository) Entity(ref *catalog.Ref) catalog.Entity {
	if ref.Kind == "" {
		return nil // Entity lookup requires kind specifier
	}
	switch ref.Kind {
	case catalog.KindComponent:
		if c := r.Component(ref); c != nil {
			return c
		}
	case catalog.KindSystem:
		if s := r.System(ref); s != nil {
			return s
		}
	case catalog.KindDomain:
		if d := r.Domain(ref); d != nil {
			return d
		}
	case catalog.KindAPI:
		if a := r.API(ref); a != nil {
			return a
		}
	case catalog.KindResource:
		if res := r.Resource(ref); res != nil {
			return res
		}
	case catalog.KindGroup:
		if g := r.Group(ref); g != nil {
			return g
		}
	}
	return nil // invalid kind specifier
}

func findEntities[T catalog.Entity](q string, items map[string]T) []T {
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
		return catalog.CompareEntityByRef(c1, c2)
	})
	return result
}

func (r *Repository) FindComponents(q string) []*catalog.Component {
	return findEntities(q, r.components)
}

func (r *Repository) FindSystems(q string) []*catalog.System {
	return findEntities(q, r.systems)
}

func (r *Repository) FindAPIs(q string) []*catalog.API {
	return findEntities(q, r.apis)
}

func (r *Repository) FindResources(q string) []*catalog.Resource {
	return findEntities(q, r.resources)
}

func (r *Repository) FindDomains(q string) []*catalog.Domain {
	return findEntities(q, r.domains)
}

func (r *Repository) FindGroups(q string) []*catalog.Group {
	return findEntities(q, r.groups)
}

func (r *Repository) validateMetadata(m *catalog.Metadata) error {
	if m == nil {
		return fmt.Errorf("metadata is null")
	}
	if !catalog.IsValidName(m.Name) {
		return fmt.Errorf("invalid name: %s", m.Name)
	}
	if m.Namespace != "" && !catalog.IsValidNamespace(m.Namespace) {
		return fmt.Errorf("invalid namespace: %s", m.Namespace)
	}
	for k, v := range m.Labels {
		if !catalog.IsValidLabel(k, v) {
			return fmt.Errorf("invalid label: \"%s: %s\"", k, v)
		}
	}
	for k, v := range m.Annotations {
		if !catalog.IsValidAnnotation(k, v) {
			return fmt.Errorf("invalid annotation: \"%s: %s\"", k, v)
		}
	}
	for _, tag := range m.Tags {
		if !catalog.IsValidTag(tag) {
			return fmt.Errorf("invalid tag: %q", tag)
		}
	}
	return nil
}

func validRef(ref *catalog.Ref) error {
	if ref == nil {
		return fmt.Errorf("nil entity reference")
	}
	if ref.Namespace != "" && !catalog.IsValidName(ref.Namespace) {
		return fmt.Errorf("not a valid namespace name %q", ref.Namespace)
	}
	if !catalog.IsValidName(ref.Name) {
		return fmt.Errorf("not a valid entity name %q", ref.Name)
	}
	return nil
}

// validDependsOnRef checks if ref is a valid fully qualified entity reference.
// It must include the entity kind, e.g. "component:my-namespace/foo", or "component:bar".
// For now, only component and resource dependencies are supported.
func validDependsOnRef(ref *catalog.Ref) error {
	if err := validRef(ref); err != nil {
		return err
	}
	if ref.Kind == "" {
		return fmt.Errorf("entity kind is missing in DependsOn entity ref %q", ref)
	}
	if ref.Kind != "component" && ref.Kind != "resource" {
		return fmt.Errorf("invalid entity kind %q for DependsOn entity ref", ref.Kind)
	}
	return nil
}

// Validate validates the repository (cross references exist, etc.).
func (r *Repository) Validate() error {
	// Groups
	for _, g := range r.groups {
		qn := g.GetRef().QName()
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
			g.Spec.Profile = &catalog.GroupSpecProfile{}
		}
	}

	// Components
	for _, c := range r.components {
		qn := c.GetRef().QName()
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
		if s.Owner == nil {
			return fmt.Errorf("component %s has no owner", qn)
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for component %s is undefined", s.Owner, qn)
		}

		if s.System == nil {
			return fmt.Errorf("component %s has no system reference", qn)
		}
		if system := r.System(s.System); system == nil {
			return fmt.Errorf("system %q for component %s is undefined", s.System, qn)
		}

		for _, a := range s.ProvidesAPIs {
			if ap := r.API(a.Ref); ap == nil {
				return fmt.Errorf("provided API %q for component %s is undefined", a, qn)
			}
		}
		for _, a := range s.ConsumesAPIs {
			if ap := r.API(a.Ref); ap == nil {
				return fmt.Errorf("consumed API %q for component %s is undefined", a, qn)
			}
		}
		for _, a := range s.DependsOn {
			if err := validDependsOnRef(a.Ref); err != nil {
				return fmt.Errorf("invalid entity reference in dependency %q for component %s: %v ", a, qn, err)
			}
			if e := r.Entity(a.Ref); e == nil {
				return fmt.Errorf("dependency %q for component %s is undefined", a, qn)
			}
		}
	}

	// APIs
	for _, ap := range r.apis {
		qn := ap.GetRef().QName()
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
		if s.System == nil {
			return fmt.Errorf("API %s has no system reference", qn)
		}
		if system := r.System(s.System); system == nil {
			return fmt.Errorf("system %q for API %s is undefined", s.System, qn)
		}
		if s.Owner == nil {
			return fmt.Errorf("API %s has no owner", qn)
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for API %s is undefined", s.Owner, qn)
		}
		// Allow an empty Definition.
		// It is mandatory in the swcat/v1alpha1 schema, but this just
		// forces us to set it to "n/a" all over the place.
	}

	// Resources
	for _, res := range r.resources {
		qn := res.GetRef().QName()
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
		if s.System == nil {
			return fmt.Errorf("resource %s has no system reference", qn)
		}
		if r.System(s.System) == nil {
			return fmt.Errorf("system %q for resource %s is undefined", s.System, qn)
		}
		if s.Owner == nil {
			return fmt.Errorf("resource %s has no owner", qn)
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for resource %s is undefined", s.Owner, qn)
		}
		for _, a := range s.DependsOn {
			if err := validDependsOnRef(a.Ref); err != nil {
				return fmt.Errorf("invalid entity reference in dependency %q for component %s: %v ", a, qn, err)
			}
			if e := r.Entity(a.Ref); e == nil {
				return fmt.Errorf("dependency %q for resource %s is undefined", a, qn)
			}
		}
	}

	// Systems
	for _, system := range r.systems {
		qn := system.GetRef().QName()
		if err := r.validateMetadata(system.Metadata); err != nil {
			return fmt.Errorf("system %s has invalid metadata: %v", qn, err)
		}
		s := system.Spec
		if s == nil {
			return fmt.Errorf("system %s has no spec", qn)
		}
		if s.Owner == nil {
			return fmt.Errorf("system %s has no owner", qn)
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for system %s is undefined", s.Owner, qn)
		}
		if s.Domain == nil {
			return fmt.Errorf("system %s has no domain reference", qn)
		}
		if d := r.Domain(s.Domain); d == nil {
			return fmt.Errorf("domain %q for system %s is undefined", s.Domain, qn)
		}
	}

	// Domains
	for _, dom := range r.domains {
		qn := dom.GetRef().QName()
		if err := r.validateMetadata(dom.Metadata); err != nil {
			return fmt.Errorf("domain %s has invalid metadata: %v", qn, err)
		}
		s := dom.Spec
		if s == nil {
			return fmt.Errorf("domain %s has no spec", qn)
		}
		if s.Owner == nil {
			return fmt.Errorf("domain %s has no owner", qn)
		}
		if g := r.Group(s.Owner); g == nil {
			return fmt.Errorf("owner %q for domain %s is undefined", s.Owner, qn)
		}
	}

	// Validation succeeded: postprocess entities.
	r.populateRelationships()
	r.sortReferences()
	r.addGeneratedLinks()

	return nil
}

// populateRelationships populates the "inverse relationship" fields of entities.
// Assumes that the repository has been validated already.
func (r *Repository) populateRelationships() {

	registerDependent := func(entityRef *catalog.Ref, depRef *catalog.LabelRef) {
		dep := r.Entity(depRef.Ref)
		switch x := dep.(type) {
		case *catalog.Component:
			x.AddDependent(&catalog.LabelRef{Ref: entityRef, Label: depRef.Label})
		case *catalog.Resource:
			x.AddDependent(&catalog.LabelRef{Ref: entityRef, Label: depRef.Label})
		default:
			panic(fmt.Sprintf("Invalid dependency: %s", depRef.String()))
		}
	}

	// Components
	for _, c := range r.components {
		ref := c.GetRef()
		// Register in APIs
		for _, a := range c.Spec.ConsumesAPIs {
			ap := r.API(a.Ref)
			ap.AddConsumer(&catalog.LabelRef{Ref: ref, Label: a.Label})
		}
		for _, a := range c.Spec.ProvidesAPIs {
			ap := r.API(a.Ref)
			ap.AddProvider(&catalog.LabelRef{Ref: ref, Label: a.Label})
		}
		if s := c.Spec.System; s != nil {
			system := r.System(s)
			system.AddComponent(ref)
		}

		// Register in "DependsOn" dependencies.
		for _, d := range c.Spec.DependsOn {
			registerDependent(ref, d)
		}
	}

	// Resources
	for _, res := range r.resources {
		ref := res.GetRef()
		if s := res.Spec.System; s != nil {
			system := r.System(s)
			system.AddResource(ref)
		}
		// Register in "DependsOn" dependencies.
		for _, d := range res.Spec.DependsOn {
			registerDependent(ref, d)
		}
	}

	// APIs
	for _, ap := range r.apis {
		ref := ap.GetRef()
		if s := ap.Spec.System; s != nil {
			system := r.System(s)
			system.AddAPI(ref)
		}
	}

	// Systems
	for _, system := range r.systems {
		ref := system.GetRef()
		if d := system.Spec.Domain; d != nil {
			domain := r.Domain(d)
			domain.AddSystem(ref)
		}
	}

}

func (r *Repository) sortReferences() {

	// Components
	for _, c := range r.components {
		c.SortRefs()
	}

	// Resources
	for _, res := range r.resources {
		res.SortRefs()
	}

	// APIs
	for _, ap := range r.apis {
		ap.SortRefs()
	}

	// Systems
	for _, system := range r.systems {
		system.SortRefs()
	}

}

func (r *Repository) addGeneratedLinks() {
	defaultRepoPrefix := strings.TrimSuffix(r.config.RepositoryURLPrefix, "/")

	for _, e := range r.allEntities {
		m := e.GetMetadata()
		// Check that no generated links already exist (that would be a programming error)
		if slices.ContainsFunc(m.Links, func(l *catalog.Link) bool {
			return l.IsGenerated
		}) {
			panic(fmt.Sprintf("addGeneratedLinks called on entity %s that already has generated links", e.GetRef()))
		}
		// Generate new links
		var links []*catalog.Link
		if r, ok := m.Annotations[catalog.AnnotRepository]; ok && r != "" {
			var baseUrl string
			if r == "default" {
				baseUrl = defaultRepoPrefix
			} else {
				baseUrl = strings.TrimSuffix(r, "/")
			}
			link := &catalog.Link{
				Title:       "Source",
				URL:         baseUrl + "/" + m.Name,
				Type:        "repository",
				Icon:        "code",
				IsGenerated: true,
			}
			links = append(links, link)
		}
		m.Links = append(m.Links, links...)
	}
}

// LoadRepositoryFromPath reads entities from the given catalog paths
// and returns a validated repository.
// Elements in catalogPaths must be .yml file paths.
func LoadRepositoryFromPaths(config Config, catalogPaths []string) (*Repository, error) {
	repo := NewRepositoryWithConfig(config)

	for _, catalogPath := range catalogPaths {
		log.Printf("Reading catalog file %s", catalogPath)
		entities, err := store.ReadEntities(catalogPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read entities from %s: %v", catalogPath, err)
		}
		for _, e := range entities {
			entity, err := catalog.NewEntityFromAPI(e)
			if err != nil {
				return nil, fmt.Errorf("failed to convert api entity %s:%s/%s (source: %s:%d) to catalog entity: %v",
					e.GetKind(), e.GetMetadata().Namespace, e.GetMetadata().Namespace,
					e.GetSourceInfo().Path, e.GetSourceInfo().Line, err)
			}
			if err := repo.AddEntity(entity); err != nil {
				return nil, fmt.Errorf("failed to add entity %q to the repo: %v", entity.GetRef(), err)
			}
		}
	}
	if err := repo.Validate(); err != nil {
		return nil, fmt.Errorf("repository validation failed: %v", err)
	}

	return repo, nil

}
