// Package catalog defines the model classes that form the software catalog.
// See the api package for the types that are mashalled to / unmarshalled from YAML.
package catalog

import (
	"cmp"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
)

const (
	// The name of the (implicit) default namespace.
	// In swcat, entity references typically omit the default namespace
	// even in fully qualified form (e.g., resource:my-resource).
	DefaultNamespace = api.DefaultNamespace
)

type Kind string

const (
	KindDomain    Kind = api.KindDomain
	KindSystem    Kind = api.KindSystem
	KindComponent Kind = api.KindComponent
	KindResource  Kind = api.KindResource
	KindAPI       Kind = api.KindAPI
	KindGroup     Kind = api.KindGroup
)

// Well-known annotation and label names with defined interpretations.
const (
	AnnotRepository = "swcat/repo"
	AnnotSterotype  = "swcat/stereotype"
	AnnotFillColor  = "swcat/fillcolor"
)

type Ref struct {
	Kind      Kind
	Namespace string
	Name      string
}

type LabelRef struct {
	Ref   *Ref
	Label string
}

// Entity is the interface implemented by all entity kinds (Component, System, etc.).
type Entity interface {
	GetKind() Kind
	GetMetadata() *Metadata
	// Returns the fully qualified entity reference.
	GetRef() *Ref
	// Returns the namespace qualified name, e.g. "ns1/foo". The default namespace
	// is omitted, i.e. an entity "default/foo" is returned as "foo".
	GetQName() string

	// Returns the spec.type of the entity, if one exists and is set.
	GetType() string

	// GetSourceInfo returns internal bookkeeping data, e.g. for error logging
	// and reconstructing YAML files (retaining the exact structure including comments).
	GetSourceInfo() *api.SourceInfo
	SetSourceInfo(si *api.SourceInfo)

	// Reset creates a shallow copy of the entity with computed values (inv. relations) removed.
	Reset() Entity
}

// SystemPart is the interface implemented by all entity kinds that appear as parts
// of a System entity (Components, Resources, APIs, Systems).
type SystemPart interface {
	Entity
	// Returns the entity reference of the System that this entity is a part of.
	GetSystem() *Ref
}

// Metadata

type Link struct {
	// A url in a standard uri format.
	// [required]
	URL string
	// A user friendly display name for the link.
	// [optional]
	Title string
	// A key representing a visual icon to be displayed in the UI.
	// [optional]
	Icon string
	// An optional value to categorize links into specific groups.
	// [optional]
	Type string

	// Whether the link was auto-generated. False for user-provided links.
	IsGenerated bool
}

type Metadata struct {
	// The name of the entity. Must be unique within the catalog at any given point in time, for any given namespace + kind pair.
	// [required]
	Name string
	// The namespace that the entity belongs to. If empty, the entity is assume to live in the default namespace.
	// [optional]
	Namespace string
	// A display name of the entity, to be presented in user interfaces instead of the name property, when available.
	// [optional]
	Title string
	// A short (typically relatively few words, on one line) description of the entity.
	// [optional]
	Description string
	// Key/value pairs of identifying information attached to the entity.
	// [optional]
	Labels map[string]string
	// Key/value pairs of non-identifying auxiliary information attached to the entity.
	// Mostly used by plugins to store additional information about the entity.
	// [optional]
	Annotations map[string]string
	// A list of single-valued strings, to for example classify catalog entities in various ways.
	// [optional]
	Tags []string
	// A list of external hyperlinks related to the entity.
	// [optional]
	Links []*Link
}

// Domain

type domainInvRel struct {
	systems []*Ref
}

type DomainSpec struct {
	// An entity reference to the owner of the domain.
	// [required]
	Owner *Ref
	// An entity reference to another domain of which the domain is a part.
	// [optional]
	SubdomainOf *Ref
	// The type of domain. There is currently no enforced set of values for this field,
	// so it is left up to the adopting organization to choose a nomenclature that matches
	// their catalog hierarchy.
	// [optional]
	Type string

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv domainInvRel
}

type Domain struct {
	Metadata *Metadata
	Spec     *DomainSpec

	sourceInfo *api.SourceInfo
}

// System

type systemInvRel struct {
	components []*Ref
	apis       []*Ref
	resources  []*Ref
}

type SystemSpec struct {
	// An entity reference to the owner of the system.
	// [required]
	Owner *Ref
	// An entity reference to the domain that the system belongs to.
	// [optional]
	Domain *Ref
	// The type of system. There is currently no enforced set of values for this field,
	// so it is left up to the adopting organization to choose a nomenclature that matches
	// their catalog hierarchy.
	// [optional]
	Type string

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv systemInvRel
}

type System struct {
	Metadata *Metadata
	Spec     *SystemSpec

	sourceInfo *api.SourceInfo
}

// Component

type componentInvRel struct {
	dependents []*LabelRef
}

type ComponentSpec struct {
	// The type of component.
	// Should ideally be one of a few well-known values that are used consistently.
	// For example, ["service", "batch"].
	// [required]
	Type string
	// The lifecycle state of the component.
	// Should ideally be one of a few well-known values that are used consistently.
	// For example, ["production", "test", "dev", "experimental"].
	// [required]
	Lifecycle string
	// An entity reference to the owner of the component.
	// [required]
	Owner *Ref
	// An entity reference to the system that the component belongs to.
	// [required]
	System *Ref
	// An entity reference to another component of which the component is a part.
	// [optional]
	SubcomponentOf *Ref
	// An array of entity references to the APIs that are provided by the component.
	// [optional]
	ProvidesAPIs []*LabelRef
	// An array of entity references to the APIs that are consumed by the component.
	// [optional]
	ConsumesAPIs []*LabelRef
	// An array of references to other entities that the component depends on to function.
	// [optional]
	DependsOn []*LabelRef

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv componentInvRel
}

type Component struct {
	Metadata *Metadata
	Spec     *ComponentSpec

	sourceInfo *api.SourceInfo
}

// Resource

type resourceInvRel struct {
	dependents []*LabelRef
}

type ResourceSpec struct {
	// The type of resource.
	// [required]
	Type string
	// An entity reference to the owner of the resource.
	// [required]
	Owner *Ref
	// An array of references to other entities that the resource depends on to function.
	// [optional]
	DependsOn []*LabelRef
	// An entity reference to the system that the resource belongs to.
	// [required]
	System *Ref

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv resourceInvRel
}

type Resource struct {
	Metadata *Metadata
	Spec     *ResourceSpec

	sourceInfo *api.SourceInfo
}

// API
type apiInvRel struct {
	providers []*LabelRef
	consumers []*LabelRef
}

type APISpec struct {
	// The type of the API definition.
	// [required]
	Type string
	// The lifecycle state of the API.
	// [required]
	Lifecycle string
	// An entity reference to the owner of the API.
	// [required]
	Owner *Ref
	// An entity reference to the system that the API belongs to.
	// [required]
	System *Ref
	// The definition of the API, based on the format defined by the type.
	// A required field in the backstage.io schema, but we leave it as optional.
	// [optional]
	Definition string

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv apiInvRel
}

type API struct {
	Metadata *Metadata
	Spec     *APISpec

	sourceInfo *api.SourceInfo
}

// Group

type GroupSpecProfile struct {
	// A simple display name to present to users. Should always be set.
	DisplayName string
	// An email where the group can be reached.
	Email string
	// Optional URL of an image that represents this entity.
	Picture string
}

type GroupSpec struct {
	// The type of group. There is currently no enforced set of values for this field,
	// so it is left up to the adopting organization to choose a nomenclature that matches their org hierarchy.
	// [required]
	Type string
	// Optional profile information about the group, mainly for display purposes.
	// [optional]
	Profile *GroupSpecProfile
	// The immediate parent group in the hierarchy, if any.
	// [optional]
	Parent *Ref
	// The immediate child groups of this group in the hierarchy (whose parent field points to this group).
	// In the backstage.io schema the list must be present, but may be empty if there are no child groups.
	// [optional]
	Children []*Ref
	// The users that are members of this group. The entries of this array are uninterpreted strings.
	// [optional]
	Members []string
}

type Group struct {
	Metadata *Metadata
	Spec     *GroupSpec

	sourceInfo *api.SourceInfo
}

// Interface implementations and helpers.
func (m *Metadata) QName() string {
	if m.Namespace != "" && m.Namespace != DefaultNamespace {
		return m.Namespace + "/" + m.Name
	}
	return m.Name
}

func (e *Ref) QName() string {
	if e.Namespace != "" && e.Namespace != DefaultNamespace {
		return e.Namespace + "/" + e.Name
	}
	return e.Name
}

func (e *Ref) Equal(other *Ref) bool {
	return e.Kind == other.Kind && e.Namespace == other.Namespace && e.Name == other.Name
}

func (e *Ref) String() string {
	var sb strings.Builder
	if e.Kind != "" {
		sb.WriteString(string(e.Kind) + ":")
	}
	if e.Namespace != "" && e.Namespace != DefaultNamespace {
		sb.WriteString(e.Namespace + "/")
	}
	sb.WriteString(e.Name)
	return sb.String()
}

func (e *LabelRef) QName() string {
	return e.Ref.QName()
}

func (e *LabelRef) Equal(other *LabelRef) bool {
	return e.Ref.Equal(other.Ref) && e.Label == other.Label
}

func (e *LabelRef) String() string {
	refStr := e.Ref.String()
	if e.Label == "" {
		return refStr
	}
	return refStr + ` "` + e.Label + `"`
}

func newRef(kind Kind, meta *Metadata) *Ref {
	namespace := meta.Namespace
	if namespace == "" {
		namespace = DefaultNamespace
	}
	return &Ref{
		Kind:      kind,
		Namespace: namespace,
		Name:      meta.Name,
	}
}

// Compare compares two Refs lexicographically by (kind, namespace, name).
func (r *Ref) Compare(s *Ref) int {
	// kind
	if c := cmp.Compare(r.Kind, s.Kind); c != 0 {
		return c
	}
	// namespace
	rDef := r.Namespace == DefaultNamespace
	sDef := s.Namespace == DefaultNamespace
	if rDef != sDef {
		if rDef {
			return -1
		}
		return 1
	}
	if c := cmp.Compare(r.Namespace, s.Namespace); c != 0 {
		return c
	}
	// name
	return cmp.Compare(r.Name, s.Name)
}

func (r *LabelRef) Compare(s *LabelRef) int {
	if c := r.Ref.Compare(s.Ref); c != 0 {
		return c
	}
	return cmp.Compare(r.Label, s.Label)
}

// CompareEntityByRef compares two entities lexicographically by (namespace, name).
func CompareEntityByRef(a, b Entity) int {
	return a.GetRef().Compare(b.GetRef())
}

func (c *Component) GetKind() Kind              { return KindComponent }
func (c *Component) GetMetadata() *Metadata     { return c.Metadata }
func (c *Component) GetRef() *Ref               { return newRef(KindComponent, c.Metadata) }
func (c *Component) GetQName() string           { return c.Metadata.QName() }
func (c *Component) GetType() string            { return c.Spec.Type }
func (c *Component) GetOwner() *Ref             { return c.Spec.Owner }
func (c *Component) GetLifecycle() string       { return c.Spec.Lifecycle }
func (c *Component) GetSystem() *Ref            { return c.Spec.System }
func (c *Component) GetDependents() []*LabelRef { return c.Spec.inv.dependents }
func (c *Component) AddDependent(d *LabelRef) {
	c.Spec.inv.dependents = append(c.Spec.inv.dependents, d)
}
func (c *Component) GetSourceInfo() *api.SourceInfo   { return c.sourceInfo }
func (c *Component) SetSourceInfo(si *api.SourceInfo) { c.sourceInfo = si }
func (c *Component) Reset() Entity {
	clone := *c
	spec := *c.Spec
	clone.Spec = &spec
	clone.Spec.inv = componentInvRel{}
	return &clone
}

func compareRef(a, b *Ref) int {
	return a.Compare(b)
}
func compareLabelRef(a, b *LabelRef) int {
	return a.Compare(b)
}

func (c *Component) SortRefs() {
	slices.SortFunc(c.Spec.ConsumesAPIs, compareLabelRef)
	slices.SortFunc(c.Spec.ProvidesAPIs, compareLabelRef)
	slices.SortFunc(c.Spec.DependsOn, compareLabelRef)
	slices.SortFunc(c.Spec.inv.dependents, compareLabelRef)
}

func (s *System) GetKind() Kind                    { return KindSystem }
func (s *System) GetMetadata() *Metadata           { return s.Metadata }
func (s *System) GetRef() *Ref                     { return newRef(KindSystem, s.Metadata) }
func (s *System) GetQName() string                 { return s.Metadata.QName() }
func (s *System) GetType() string                  { return s.Spec.Type }
func (s *System) GetComponents() []*Ref            { return s.Spec.inv.components }
func (s *System) GetAPIs() []*Ref                  { return s.Spec.inv.apis }
func (s *System) GetResources() []*Ref             { return s.Spec.inv.resources }
func (s *System) GetSystem() *Ref                  { return s.GetRef() }
func (s *System) AddAPI(a *Ref)                    { s.Spec.inv.apis = append(s.Spec.inv.apis, a) }
func (s *System) AddComponent(c *Ref)              { s.Spec.inv.components = append(s.Spec.inv.components, c) }
func (s *System) AddResource(r *Ref)               { s.Spec.inv.resources = append(s.Spec.inv.resources, r) }
func (s *System) GetSourceInfo() *api.SourceInfo   { return s.sourceInfo }
func (s *System) SetSourceInfo(si *api.SourceInfo) { s.sourceInfo = si }
func (s *System) Reset() Entity {
	clone := *s
	spec := *s.Spec
	clone.Spec = &spec
	clone.Spec.inv = systemInvRel{}
	return &clone
}
func (s *System) SortRefs() {
	slices.SortFunc(s.Spec.inv.apis, compareRef)
	slices.SortFunc(s.Spec.inv.components, compareRef)
	slices.SortFunc(s.Spec.inv.resources, compareRef)
}

func (d *Domain) GetKind() Kind                    { return KindDomain }
func (d *Domain) GetMetadata() *Metadata           { return d.Metadata }
func (d *Domain) GetRef() *Ref                     { return newRef(KindDomain, d.Metadata) }
func (d *Domain) GetQName() string                 { return d.Metadata.QName() }
func (d *Domain) GetType() string                  { return d.Spec.Type }
func (d *Domain) GetSystems() []*Ref               { return d.Spec.inv.systems }
func (d *Domain) AddSystem(s *Ref)                 { d.Spec.inv.systems = append(d.Spec.inv.systems, s) }
func (d *Domain) GetSourceInfo() *api.SourceInfo   { return d.sourceInfo }
func (d *Domain) SetSourceInfo(si *api.SourceInfo) { d.sourceInfo = si }
func (d *Domain) Reset() Entity {
	clone := *d
	spec := *d.Spec
	clone.Spec = &spec
	clone.Spec.inv = domainInvRel{}
	return &clone
}
func (d *Domain) SortRefs() {
	slices.SortFunc(d.Spec.inv.systems, compareRef)
}

func (a *API) GetKind() Kind                    { return KindAPI }
func (a *API) GetMetadata() *Metadata           { return a.Metadata }
func (a *API) GetRef() *Ref                     { return newRef(KindAPI, a.Metadata) }
func (a *API) GetQName() string                 { return a.Metadata.QName() }
func (a *API) GetType() string                  { return a.Spec.Type }
func (a *API) GetProviders() []*LabelRef        { return a.Spec.inv.providers }
func (a *API) GetConsumers() []*LabelRef        { return a.Spec.inv.consumers }
func (a *API) GetSystem() *Ref                  { return a.Spec.System }
func (a *API) AddProvider(p *LabelRef)          { a.Spec.inv.providers = append(a.Spec.inv.providers, p) }
func (a *API) AddConsumer(c *LabelRef)          { a.Spec.inv.consumers = append(a.Spec.inv.consumers, c) }
func (a *API) GetSourceInfo() *api.SourceInfo   { return a.sourceInfo }
func (a *API) SetSourceInfo(si *api.SourceInfo) { a.sourceInfo = si }
func (a *API) Reset() Entity {
	clone := *a
	spec := *a.Spec
	clone.Spec = &spec
	clone.Spec.inv = apiInvRel{}
	return &clone
}
func (a *API) SortRefs() {
	slices.SortFunc(a.Spec.inv.consumers, compareLabelRef)
	slices.SortFunc(a.Spec.inv.providers, compareLabelRef)
}

func (r *Resource) GetKind() Kind              { return KindResource }
func (r *Resource) GetMetadata() *Metadata     { return r.Metadata }
func (r *Resource) GetRef() *Ref               { return newRef(KindResource, r.Metadata) }
func (r *Resource) GetQName() string           { return r.Metadata.QName() }
func (r *Resource) GetType() string            { return r.Spec.Type }
func (r *Resource) GetDependents() []*LabelRef { return r.Spec.inv.dependents }
func (r *Resource) GetSystem() *Ref            { return r.Spec.System }
func (r *Resource) AddDependent(d *LabelRef) {
	r.Spec.inv.dependents = append(r.Spec.inv.dependents, d)
}
func (r *Resource) GetSourceInfo() *api.SourceInfo   { return r.sourceInfo }
func (r *Resource) SetSourceInfo(si *api.SourceInfo) { r.sourceInfo = si }
func (r *Resource) Reset() Entity {
	clone := *r
	spec := *r.Spec
	clone.Spec = &spec
	clone.Spec.inv = resourceInvRel{}
	return &clone
}
func (r *Resource) SortRefs() {
	slices.SortFunc(r.Spec.DependsOn, compareLabelRef)
	slices.SortFunc(r.Spec.inv.dependents, compareLabelRef)
}

func (g *Group) GetKind() Kind                    { return KindGroup }
func (g *Group) GetMetadata() *Metadata           { return g.Metadata }
func (g *Group) GetRef() *Ref                     { return newRef(KindGroup, g.Metadata) }
func (g *Group) GetQName() string                 { return g.Metadata.QName() }
func (g *Group) GetType() string                  { return g.Spec.Type }
func (g *Group) GetDisplayName() string           { return g.Spec.Profile.DisplayName }
func (g *Group) GetSourceInfo() *api.SourceInfo   { return g.sourceInfo }
func (g *Group) SetSourceInfo(si *api.SourceInfo) { g.sourceInfo = si }
func (g *Group) Reset() Entity {
	clone := *g
	spec := *g.Spec
	clone.Spec = &spec
	return &clone
}

func ParseRef(s string) (*Ref, error) {
	r, err := api.ParseRef(s)
	if err != nil {
		return nil, err
	}
	return NewRefFromAPI(r)
}

func ParseRefAs(kind Kind, s string) (*Ref, error) {
	r, err := api.ParseRef(s)
	if err != nil {
		return nil, err
	}
	return NewRefFromAPIWithKind(kind, r)
}
