package repo

import (
	"cmp"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/query"
	"github.com/dnswlt/swcat/internal/store"
)

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

// cloneEmpty returns a copy of r with all maps empty, but config etc. preserved.
func (r *Repository) cloneEmpty() *Repository {
	return NewRepositoryWithConfig(r.config)
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
// Rebuild the repository from scratch (as a copy), and validate.
// It avoids having to deal with complex deletions and additions of relationships
// and their inverses. The repository r remains unchanged in any case.
func (r *Repository) InsertOrUpdateEntity(e catalog.Entity) (*Repository, error) {
	r2 := r.cloneEmpty()
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
			return nil, fmt.Errorf("failed to rebuild repository: %v", err)
		}
	}
	if !found {
		// e not found in repository => insert
		if err := r2.AddEntity(e); err != nil {
			return nil, fmt.Errorf("failed to insert new entity: %v", err)
		}
	}

	if err := r2.Validate(); err != nil {
		return nil, fmt.Errorf("repository validation failed: %v", err)
	}

	return r2, nil
}

// DeleteEntity removes the given entity from the repository.
// Deletions are only allowed if the given entity does not have remaining
// ingoing dependencies (i.e. references from other entities) of any kind.
// See InsertOrUpdateEntity for the procedure.
func (r *Repository) DeleteEntity(ref *catalog.Ref) (*Repository, error) {
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
		return nil, fmt.Errorf("entity %q does not exist", ref)
	}
	switch entity := e.(type) {
	case *catalog.Component:
		if len(entity.GetDependents()) != 0 {
			return nil, fmt.Errorf("remaining ingoing dependencies: %v", refList(entity.GetDependents()))
		}
	case *catalog.Resource:
		if len(entity.GetDependents()) != 0 {
			return nil, fmt.Errorf("remaining ingoing dependencies: %v", refList(entity.GetDependents()))
		}
	case *catalog.API:
		if len(entity.GetProviders()) != 0 {
			return nil, fmt.Errorf("remaining API providers: %v", refList(entity.GetProviders()))
		}
		if len(entity.GetConsumers()) != 0 {
			return nil, fmt.Errorf("remaining API providers: %v", refList(entity.GetConsumers()))
		}
	default:
		return nil, fmt.Errorf("deleting entities of type %T is currently not supported", e)
	}

	// Rebuild repo without entity
	r2 := r.cloneEmpty()
	for _, n := range r.allEntities {
		if n.GetRef().Equal(ref) {
			continue // Skip the entity to be deleted
		}
		toAdd := n.Reset()
		if err := r2.AddEntity(toAdd); err != nil {
			return nil, fmt.Errorf("failed to rebuild repository: %v", err)
		}
	}
	if err := r2.Validate(); err != nil {
		return nil, fmt.Errorf("repository validation failed: %v", err)
	}

	return r2, nil
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

func (r *Repository) FindEntities(q string) []catalog.Entity {
	return findEntities(q, r.allEntities)
}

func labelKeys[T catalog.Entity](items map[string]T) []string {
	keySet := map[string]bool{}
	for _, item := range items {
		for k := range item.GetMetadata().Labels {
			keySet[k] = true
		}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	return keys
}

func annnotationKeys[T catalog.Entity](items map[string]T) []string {
	keySet := map[string]bool{}
	for _, item := range items {
		for k := range item.GetMetadata().Annotations {
			keySet[k] = true
		}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	return keys
}

func (r *Repository) AnnotationKeys(kind catalog.Kind) []string {
	switch kind {
	case catalog.KindDomain:
		return annnotationKeys(r.domains)
	case catalog.KindSystem:
		return annnotationKeys(r.systems)
	case catalog.KindComponent:
		return annnotationKeys(r.components)
	case catalog.KindResource:
		return annnotationKeys(r.resources)
	case catalog.KindAPI:
		return annnotationKeys(r.apis)
	case catalog.KindGroup:
		return annnotationKeys(r.groups)
	}
	return nil
}

func (r *Repository) LabelKeys(kind catalog.Kind) []string {
	switch kind {
	case catalog.KindDomain:
		return labelKeys(r.domains)
	case catalog.KindSystem:
		return labelKeys(r.systems)
	case catalog.KindComponent:
		return labelKeys(r.components)
	case catalog.KindResource:
		return labelKeys(r.resources)
	case catalog.KindAPI:
		return labelKeys(r.apis)
	case catalog.KindGroup:
		return labelKeys(r.groups)
	}
	return nil
}

func collectSpecValues[T any](items map[string]T, extractor func(T) string) []string {
	valueSet := map[string]bool{}
	for _, item := range items {
		if v := extractor(item); v != "" {
			valueSet[v] = true
		}
	}
	values := make([]string, 0, len(valueSet))
	for v := range valueSet {
		values = append(values, v)
	}
	return values
}

func (r *Repository) SpecFieldValues(kind catalog.Kind, field string) ([]string, error) {
	switch kind {
	case catalog.KindComponent:
		switch field {
		case "type":
			return collectSpecValues(r.components, func(x *catalog.Component) string { return x.Spec.Type }), nil
		case "lifecycle":
			return collectSpecValues(r.components, func(x *catalog.Component) string { return x.Spec.Lifecycle }), nil
		}
	case catalog.KindAPI:
		switch field {
		case "type":
			return collectSpecValues(r.apis, func(x *catalog.API) string { return x.Spec.Type }), nil
		case "lifecycle":
			return collectSpecValues(r.apis, func(x *catalog.API) string { return x.Spec.Lifecycle }), nil
		}
	case catalog.KindResource:
		switch field {
		case "type":
			return collectSpecValues(r.resources, func(x *catalog.Resource) string { return x.Spec.Type }), nil
		}
	case catalog.KindSystem:
		switch field {
		case "type":
			return collectSpecValues(r.systems, func(x *catalog.System) string { return x.Spec.Type }), nil
		}
	case catalog.KindDomain:
		switch field {
		case "type":
			return collectSpecValues(r.domains, func(x *catalog.Domain) string { return x.Spec.Type }), nil
		}
	case catalog.KindGroup:
		switch field {
		case "type":
			return collectSpecValues(r.groups, func(x *catalog.Group) string { return x.Spec.Type }), nil
		}
	}
	return nil, fmt.Errorf("field %q not supported for kind %q", field, kind)
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
	// Validate against configured rules, if present
	if v := r.config.Validation; v != nil {
		for _, e := range r.allEntities {
			if err := v.Accept(e); err != nil {
				return fmt.Errorf("entity %s failed validation of configured rules: %v", e.GetRef(), err)
			}
		}
	}

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

		if s.SubcomponentOf != nil {
			if s.SubcomponentOf.Kind != catalog.KindComponent {
				return fmt.Errorf("subcomponentOf %q must be of kind Component for component %q", s.SubcomponentOf, qn)
			}
			if parent := r.Component(s.SubcomponentOf); parent == nil {
				return fmt.Errorf("subcomponentOf %q is undefined for component %q", s.SubcomponentOf, qn)
			}
		}
		for _, a := range s.ProvidesAPIs {
			ap := r.API(a.Ref)
			if ap == nil {
				return fmt.Errorf("provided API %q for component %s is undefined", a, qn)
			}
			if val, ok := a.GetAttr(catalog.VersionAttrKey); ok {
				// Check that referenced API version exists
				if !slices.ContainsFunc(ap.Spec.Versions, func(v *catalog.APISpecVersion) bool {
					return v.Version.RawVersion == val
				}) {
					return fmt.Errorf("provided API %q for component %s does not exist in version %q", a, qn, val)
				}
			}
		}
		for _, a := range s.ConsumesAPIs {
			ap := r.API(a.Ref)
			if ap == nil {
				return fmt.Errorf("consumed API %q for component %s is undefined", a, qn)
			}
			if val, ok := a.GetAttr(catalog.VersionAttrKey); ok {
				// Check that referenced API version exists
				if !slices.ContainsFunc(ap.Spec.Versions, func(v *catalog.APISpecVersion) bool {
					return v.Version.RawVersion == val
				}) {
					return fmt.Errorf("consumed API %q for component %s does not exist in version %q", a, qn, val)
				}
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
		if len(s.Versions) > 0 {
			matchesAPILifecycle := false
			for _, v := range s.Versions {
				if v.Version.RawVersion == "" {
					return fmt.Errorf("version name is empty for API %s", qn)
				}
				if v.Lifecycle == "" {
					return fmt.Errorf("version lifecycle is empty for API %s", qn)
				}
				if v.Lifecycle == ap.Spec.Lifecycle {
					matchesAPILifecycle = true
				}
			}
			if !matchesAPILifecycle {
				return fmt.Errorf("none of the version lifecycles matches the API's own lifecycle for API %s", qn)
			}
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

	if err := r.addGeneratedLinks(); err != nil {
		return fmt.Errorf("error generating annotation-based links: %w", err)
	}

	return nil
}

// populateRelationships populates the "inverse relationship" fields of entities.
// Assumes that the repository has been validated already.
func (r *Repository) populateRelationships() {

	registerDependent := func(entityRef *catalog.Ref, depRef *catalog.LabelRef) {
		dep := r.Entity(depRef.Ref)
		switch x := dep.(type) {
		case *catalog.Component:
			x.AddDependent(&catalog.LabelRef{Ref: entityRef, Label: depRef.Label, Attrs: depRef.Attrs})
		case *catalog.Resource:
			x.AddDependent(&catalog.LabelRef{Ref: entityRef, Label: depRef.Label, Attrs: depRef.Attrs})
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
			ap.AddConsumer(&catalog.LabelRef{Ref: ref, Label: a.Label, Attrs: a.Attrs})
		}
		for _, a := range c.Spec.ProvidesAPIs {
			ap := r.API(a.Ref)
			ap.AddProvider(&catalog.LabelRef{Ref: ref, Label: a.Label, Attrs: a.Attrs})
		}
		if s := c.Spec.System; s != nil {
			system := r.System(s)
			system.AddComponent(ref)
		}
		// Register in parent component
		if p := c.Spec.SubcomponentOf; p != nil {
			parent := r.Component(p)
			parent.AddSubcomponent(ref)
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

// isValidAbsoluteURL checks if a string is a valid, absolute URL
// with a scheme (like "http" or "https") and a host.
func isValidAbsoluteURL(s string) bool {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

// linkTemplates
type linkTemplates struct {
	url        *template.Template
	title      *template.Template
	hasVersion bool // Indicates whether a {{ Version[.*] }} annotation is present in url or title.
}

// prepareLinkTemplates compiles all url and title templates found in the config.
func (r *Repository) prepareLinkTemplates() (map[string]linkTemplates, error) {
	tmpls := map[string]linkTemplates{}
	versionPlaceholderRE := regexp.MustCompile(`\{\{\s*\.Version\b`)

	for annot, abl := range r.config.AnnotationBasedLinks {
		if abl == nil {
			return nil, fmt.Errorf("annotation-based label for %q is nil", annot)
		}
		if strings.TrimSpace(abl.URL) == "" {
			return nil, fmt.Errorf("annotation-based label for %q has an empty URL", annot)
		}
		urlTmpl, err := template.New("url").Parse(abl.URL)
		if err != nil {
			return nil, fmt.Errorf("invalid URL template for annotation %q: %v", annot, err)
		}
		urlTmpl.Option("missingkey=error")
		titleTmpl, err := template.New("title").Parse(abl.Title)
		if err != nil {
			return nil, fmt.Errorf("invalid title template for annotation %q: %v", annot, err)
		}
		titleTmpl.Option("missingkey=error")
		// If either template references .Version, make this a per-version template.
		hasVersion := versionPlaceholderRE.MatchString(abl.URL + " " + abl.Title)
		tmpls[annot] = linkTemplates{url: urlTmpl, title: titleTmpl, hasVersion: hasVersion}
	}

	return tmpls, nil
}

func (r *Repository) addGeneratedLinks() error {
	tmpls, err := r.prepareLinkTemplates()
	if err != nil {
		return err
	}

	// Process entities
	for _, e := range r.allEntities {
		meta := e.GetMetadata()
		// Check that no generated links already exist (that would be a programming error)
		if slices.ContainsFunc(meta.Links, func(l *catalog.Link) bool {
			return l.IsGenerated
		}) {
			panic(fmt.Sprintf("addGeneratedLinks called on entity %s that already has generated links", e.GetRef()))
		}
		// Generate new links
		var links []*catalog.Link
		for annot, t := range tmpls {
			if value, ok := meta.Annotations[annot]; ok && value != "" {
				makeLink := func(version *catalog.Version) (*catalog.Link, error) {
					data := map[string]any{
						"Annotation": map[string]string{
							"Key":   annot,
							"Value": value,
						},
						"Metadata": meta,
					}
					if version != nil {
						data["Version"] = version
					}
					var urlSb strings.Builder
					if err := t.url.Execute(&urlSb, data); err != nil {
						return nil, fmt.Errorf("failed to execute URL template for annotation %v in entity %v: %v", annot, e.GetRef(), err)
					}
					url := urlSb.String()
					if !isValidAbsoluteURL(url) {
						return nil, fmt.Errorf("invalid url for annotation %v in entity %v: %q", annot, e.GetRef(), url)
					}
					var titleSb strings.Builder
					if err := t.title.Execute(&titleSb, data); err != nil {
						return nil, fmt.Errorf("failed to execute title template for annotation %v in entity %v: %v", annot, e.GetRef(), err)
					}
					return &catalog.Link{
						Title:       titleSb.String(),
						URL:         url,
						Type:        r.config.AnnotationBasedLinks[annot].Type,
						Icon:        r.config.AnnotationBasedLinks[annot].Icon,
						IsGenerated: true,
					}, nil
				}
				// For versioned APIs and templates referencing a version, generate links for each version.
				if ap, ok := e.(*catalog.API); ok && t.hasVersion && len(ap.Spec.Versions) > 0 {
					for _, ver := range ap.Spec.Versions {
						link, err := makeLink(&ver.Version)
						if err != nil {
							return err
						}
						links = append(links, link)
					}
				} else {
					// No {{ .Version }} placeholder or no versions, generate a single link.
					link, err := makeLink(nil)
					if err != nil {
						return err
					}
					links = append(links, link)
				}
			}
		}
		meta.Links = append(meta.Links, links...)
		slices.SortFunc(meta.Links, func(a, b *catalog.Link) int {
			if c := cmp.Compare(a.Title, b.Title); c != 0 {
				return c
			}
			return cmp.Compare(a.URL, b.URL)
		})
	}
	return nil
}

// Load reads entities from the given catalog paths
// and returns a validated repository.
// Elements in catalogPaths must be .yml file paths.
func Load(st store.Store, config Config, catalogDir string) (*Repository, error) {
	repo := NewRepositoryWithConfig(config)
	err := repo.initialize(st, catalogDir)
	if err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *Repository) initialize(st store.Store, catalogDir string) error {
	if r.Size() != 0 {
		return fmt.Errorf("initialize called on a non-empty repo (size: %d)", r.Size())
	}
	catalogPaths, err := store.CatalogFiles(st, catalogDir)
	if err != nil {
		return fmt.Errorf("initialize: cannot retrieve catalog files :%v", err)
	}
	for _, catalogPath := range catalogPaths {
		log.Printf("Reading catalog file %s", catalogPath)
		entities, err := store.ReadEntities(st, catalogPath)
		if err != nil {
			return fmt.Errorf("failed to read entities from %s: %v", catalogPath, err)
		}
		for _, e := range entities {
			entity, err := catalog.NewEntityFromAPI(e)
			if err != nil {
				return fmt.Errorf("failed to convert api entity %s:%s/%s (source: %s:%d) to catalog entity: %v",
					e.GetKind(), e.GetMetadata().Namespace, e.GetMetadata().Namespace,
					e.GetSourceInfo().Path, e.GetSourceInfo().Line, err)
			}
			if err := r.AddEntity(entity); err != nil {
				return fmt.Errorf("failed to add entity %q to the repo: %v", entity.GetRef(), err)
			}
		}
	}
	if err := r.Validate(); err != nil {
		return fmt.Errorf("repository validation failed: %v", err)
	}

	return nil
}
