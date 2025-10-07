// This file contains the API classes that define a software catalog.
// The types are broadly compatible with backstage.io's types:
// https://backstage.io/docs/features/software-catalog/descriptor-format#contents
package api

import (
	"cmp"

	"gopkg.in/yaml.v3"
)

const (
	// The name of the (implicit) default namespace.
	// In swcat, entity references typically omit the default namespace
	// even in fully qualified form (e.g., resource:my-resource).
	DefaultNamespace = "default"
)

// Entity is the interface implemented by all entity kinds (Component, System, etc.).
type Entity interface {
	GetKind() string
	GetMetadata() *Metadata
	// Returns the qualified entity name in the format
	// <namespace>/<name>
	// This form can presented to the user in cases where the entity kind is obvious or irrelevant.
	GetQName() string
	// Returns the fully qualified entity reference in the format
	// <kind>:<namespace>/<name>
	GetRef() string

	// GetSourceInfo returns internal bookkeeping data, e.g. for error logging
	// and reconstructing YAML files (retaining the exact structure including comments).
	GetSourceInfo() *SourceInfo
	SetSourceInfo(si *SourceInfo)

	// Reset creates a shallow copy of the entity with computed values (inv. relations) removed.
	Reset() Entity
}

// File and line information shared by all entities.
// Can be used in error messages and to reconstruct YAML
type SourceInfo struct {
	Node *yaml.Node // The raw YAML source code from which the entity was parsed.
	Path string     // The path from which the entity was read.
	Line int        // The first line number in Path where the entity was found.
}

// SystemPart is the interface implemented by all entity kinds that appear as parts
// of a System entity (Components, Resources, APIs, Systems).
type SystemPart interface {
	Entity
	// Returns the entity reference of the System that this entity is a part of.
	GetSystem() string
}

// Metadata

type Link struct {
	// A url in a standard uri format.
	// [required]
	URL string `yaml:"url,omitempty"`
	// A user friendly display name for the link.
	// [optional]
	Title string `yaml:"title,omitempty"`
	// A key representing a visual icon to be displayed in the UI.
	// [optional]
	Icon string `yaml:"icon,omitempty"`
	// An optional value to categorize links into specific groups.
	// [optional]
	Type string `yaml:"type,omitempty"`
}

type Metadata struct {
	// The name of the entity. Must be unique within the catalog at any given point in time, for any given namespace + kind pair.
	// [required]
	Name string `yaml:"name,omitempty"`
	// The namespace that the entity belongs to. If empty, the entity is assume to live in the default namespace.
	// [optional]
	Namespace string `yaml:"namespace,omitempty"`
	// A display name of the entity, to be presented in user interfaces instead of the name property, when available.
	// [optional]
	Title string `yaml:"title,omitempty"`
	// A short (typically relatively few words, on one line) description of the entity.
	// [optional]
	Description string `yaml:"description,omitempty"`
	// Key/value pairs of identifying information attached to the entity.
	// [optional]
	Labels map[string]string `yaml:"labels,omitempty"`
	// Key/value pairs of non-identifying auxiliary information attached to the entity.
	// Mostly used by plugins to store additional information about the entity.
	// [optional]
	Annotations map[string]string `yaml:"annotations,omitempty"`
	// A list of single-valued strings, to for example classify catalog entities in various ways.
	// [optional]
	Tags []string `yaml:"tags,omitempty"`
	// A list of external hyperlinks related to the entity.
	// [optional]
	Links []*Link `yaml:"links,omitempty"`
}

// Domain

type domainInvRel struct {
	systems []string
}

type DomainSpec struct {
	// An entity reference to the owner of the domain.
	// [required]
	Owner string `yaml:"owner,omitempty"`
	// An entity reference to another domain of which the domain is a part.
	// [optional]
	SubdomainOf string `yaml:"subdomainOf,omitempty"`
	// The type of domain. There is currently no enforced set of values for this field,
	// so it is left up to the adopting organization to choose a nomenclature that matches
	// their catalog hierarchy.
	// [optional]
	Type string `yaml:"type,omitempty"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv domainInvRel
}

type Domain struct {
	APIVersion string      `yaml:"apiVersion,omitempty"`
	Kind       string      `yaml:"kind,omitempty"`
	Metadata   *Metadata   `yaml:"metadata,omitempty"`
	Spec       *DomainSpec `yaml:"spec,omitempty"`

	// Internal data, not part of the API.
	*SourceInfo `yaml:"-"`
}

// System

type systemInvRel struct {
	components []string
	apis       []string
	resources  []string
}

type SystemSpec struct {
	// An entity reference to the owner of the system.
	// [required]
	Owner string `yaml:"owner,omitempty"`
	// An entity reference to the domain that the system belongs to.
	// [optional]
	Domain string `yaml:"domain,omitempty"`
	// The type of system. There is currently no enforced set of values for this field,
	// so it is left up to the adopting organization to choose a nomenclature that matches
	// their catalog hierarchy.
	// [optional]
	Type string `yaml:"type,omitempty"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv systemInvRel
}

type System struct {
	APIVersion string      `yaml:"apiVersion,omitempty"`
	Kind       string      `yaml:"kind,omitempty"`
	Metadata   *Metadata   `yaml:"metadata,omitempty"`
	Spec       *SystemSpec `yaml:"spec,omitempty"`

	// Internal bookkeeping data, not part of the API.
	*SourceInfo `yaml:"-"`
}

// Component
type componentInvRel struct {
	dependents []string
}

type ComponentSpec struct {
	// The type of component.
	// Should ideally be one of a few well-known values that are used consistently.
	// For example, ["service", "batch"].
	// [required]
	Type string `yaml:"type,omitempty"`
	// The lifecycle state of the component.
	// Should ideally be one of a few well-known values that are used consistently.
	// For example, ["production", "test", "dev", "experimental"].
	// [required]
	Lifecycle string `yaml:"lifecycle,omitempty"`
	// An entity reference to the owner of the component.
	// [required]
	Owner string `yaml:"owner,omitempty"`
	// An entity reference to the system that the component belongs to.
	// [optional]
	System string `yaml:"system,omitempty"`
	// An entity reference to another component of which the component is a part.
	// [optional]
	SubcomponentOf string `yaml:"subcomponentOf,omitempty"`
	// An array of entity references to the APIs that are provided by the component.
	// [optional]
	ProvidesAPIs []string `yaml:"providesApis,omitempty"`
	// An array of entity references to the APIs that are consumed by the component.
	// [optional]
	ConsumesAPIs []string `yaml:"consumesApis,omitempty"`
	// An array of references to other entities that the component depends on to function.
	// [optional]
	DependsOn []string `yaml:"dependsOn,omitempty"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv componentInvRel
}

type Component struct {
	APIVersion string         `yaml:"apiVersion,omitempty"`
	Kind       string         `yaml:"kind,omitempty"`
	Metadata   *Metadata      `yaml:"metadata,omitempty"`
	Spec       *ComponentSpec `yaml:"spec,omitempty"`

	// Internal bookkeeping data, not part of the API.
	*SourceInfo `yaml:"-"`
}

// Resource

type resourceInvRel struct {
	dependents []string
}

type ResourceSpec struct {
	// The type of resource.
	// [required]
	Type string `yaml:"type,omitempty"`
	// An entity reference to the owner of the resource.
	// [required]
	Owner string `yaml:"owner,omitempty"`
	// An array of references to other entities that the resource depends on to function.
	// [optional]
	DependsOn []string `yaml:"dependsOn,omitempty"`
	// An entity reference to the system that the resource belongs to.
	// [optional]
	System string `yaml:"system,omitempty"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv resourceInvRel
}

type Resource struct {
	APIVersion string        `yaml:"apiVersion,omitempty"`
	Kind       string        `yaml:"kind,omitempty"`
	Metadata   *Metadata     `yaml:"metadata,omitempty"`
	Spec       *ResourceSpec `yaml:"spec,omitempty"`

	// Internal bookkeeping data, not part of the API.
	*SourceInfo `yaml:"-"`
}

// API
type apiInvRel struct {
	providers []string
	consumers []string
}

type APISpec struct {
	// The type of the API definition.
	// [required]
	Type string `yaml:"type,omitempty"`
	// The lifecycle state of the API.
	// [required]
	Lifecycle string `yaml:"lifecycle,omitempty"`
	// An entity reference to the owner of the API.
	// [required]
	Owner string `yaml:"owner,omitempty"`
	// An entity reference to the system that the API belongs to.
	// [optional]
	System string `yaml:"system,omitempty"`
	// The definition of the API, based on the format defined by the type.
	// A required field in the backstage.io schema, but we leave it as optional.
	// [optional]
	Definition string `yaml:"definition,omitempty"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	inv apiInvRel
}

type API struct {
	APIVersion string    `yaml:"apiVersion,omitempty"`
	Kind       string    `yaml:"kind,omitempty"`
	Metadata   *Metadata `yaml:"metadata,omitempty"`
	Spec       *APISpec  `yaml:"spec,omitempty"`

	// Internal bookkeeping data, not part of the API.
	*SourceInfo `yaml:"-"`
}

// Group

type GroupSpecProfile struct {
	// A simple display name to present to users. Should always be set.
	DisplayName string `yaml:"displayName,omitempty"`
	// An email where the group can be reached.
	Email string `yaml:"email,omitempty"`
	// Optional URL of an image that represents this entity.
	Picture string `yaml:"picture,omitempty"`
}

type GroupSpec struct {
	// The type of group. There is currently no enforced set of values for this field,
	// so it is left up to the adopting organization to choose a nomenclature that matches their org hierarchy.
	// [required]
	Type string `yaml:"type,omitempty"`
	// Optional profile information about the group, mainly for display purposes.
	// [optional]
	Profile *GroupSpecProfile `yaml:"profile,omitempty"`
	// The immediate parent group in the hierarchy, if any.
	// [optional]
	Parent string `yaml:"parent,omitempty"`
	// The immediate child groups of this group in the hierarchy (whose parent field points to this group).
	// In the backstage.io schema the list must be present, but may be empty if there are no child groups.
	// [optional]
	Children []string `yaml:"children"`
	// The users that are members of this group. The entries of this array are entity references.
	// [optional]
	Members []string `yaml:"members,omitempty"`
}

type Group struct {
	APIVersion string     `yaml:"apiVersion,omitempty"`
	Kind       string     `yaml:"kind,omitempty"`
	Metadata   *Metadata  `yaml:"metadata,omitempty"`
	Spec       *GroupSpec `yaml:"spec,omitempty"`

	// Internal bookkeeping data, not part of the API.
	*SourceInfo `yaml:"-"`
}

//
// Interface implementations and helpers.
//

// GetQName returns the qualified name of the entity.
func (m *Metadata) GetQName() string {
	if m == nil {
		return ""
	}
	if m.Namespace == "" || m.Namespace == DefaultNamespace {
		return m.Name
	}
	return m.Namespace + "/" + m.Name
}

// CompareEntityByName compares two entities lexicographically by (namespace, name).
func CompareEntityByName(a, b Entity) int {
	if c := cmp.Compare(a.GetMetadata().Namespace, b.GetMetadata().Namespace); c != 0 {
		return c
	}
	return cmp.Compare(a.GetMetadata().Name, b.GetMetadata().Name)
}

func (c *Component) GetKind() string              { return c.Kind }
func (c *Component) GetMetadata() *Metadata       { return c.Metadata }
func (c *Component) GetQName() string             { return c.Metadata.GetQName() }
func (c *Component) GetRef() string               { return "component:" + c.GetQName() }
func (c *Component) GetOwner() string             { return c.Spec.Owner }
func (c *Component) GetLifecycle() string         { return c.Spec.Lifecycle }
func (c *Component) GetSystem() string            { return c.Spec.System }
func (c *Component) GetDependents() []string      { return c.Spec.inv.dependents }
func (c *Component) AddDependent(d string)        { c.Spec.inv.dependents = append(c.Spec.inv.dependents, d) }
func (c *Component) GetSourceInfo() *SourceInfo   { return c.SourceInfo }
func (c *Component) SetSourceInfo(si *SourceInfo) { c.SourceInfo = si }
func (c *Component) Reset() Entity {
	clone := *c
	spec := *c.Spec
	clone.Spec = &spec
	clone.Spec.inv = componentInvRel{}
	return &clone
}

func (s *System) GetKind() string              { return s.Kind }
func (s *System) GetMetadata() *Metadata       { return s.Metadata }
func (s *System) GetQName() string             { return s.Metadata.GetQName() }
func (s *System) GetRef() string               { return "system:" + s.GetQName() }
func (s *System) GetComponents() []string      { return s.Spec.inv.components }
func (s *System) GetAPIs() []string            { return s.Spec.inv.apis }
func (s *System) GetResources() []string       { return s.Spec.inv.resources }
func (s *System) GetSystem() string            { return s.GetQName() }
func (s *System) AddAPI(a string)              { s.Spec.inv.apis = append(s.Spec.inv.apis, a) }
func (s *System) AddComponent(c string)        { s.Spec.inv.components = append(s.Spec.inv.components, c) }
func (s *System) AddResource(r string)         { s.Spec.inv.resources = append(s.Spec.inv.resources, r) }
func (s *System) GetSourceInfo() *SourceInfo   { return s.SourceInfo }
func (s *System) SetSourceInfo(si *SourceInfo) { s.SourceInfo = si }
func (s *System) Reset() Entity {
	clone := *s
	spec := *s.Spec
	clone.Spec = &spec
	clone.Spec.inv = systemInvRel{}
	return &clone
}

func (d *Domain) GetKind() string              { return d.Kind }
func (d *Domain) GetMetadata() *Metadata       { return d.Metadata }
func (d *Domain) GetQName() string             { return d.Metadata.GetQName() }
func (d *Domain) GetRef() string               { return "domain:" + d.GetQName() }
func (d *Domain) GetSystems() []string         { return d.Spec.inv.systems }
func (d *Domain) AddSystem(s string)           { d.Spec.inv.systems = append(d.Spec.inv.systems, s) }
func (d *Domain) GetSourceInfo() *SourceInfo   { return d.SourceInfo }
func (d *Domain) SetSourceInfo(si *SourceInfo) { d.SourceInfo = si }
func (d *Domain) Reset() Entity {
	clone := *d
	spec := *d.Spec
	clone.Spec = &spec
	clone.Spec.inv = domainInvRel{}
	return &clone
}

func (a *API) GetKind() string              { return a.Kind }
func (a *API) GetMetadata() *Metadata       { return a.Metadata }
func (a *API) GetQName() string             { return a.Metadata.GetQName() }
func (a *API) GetRef() string               { return "api:" + a.GetQName() }
func (a *API) GetProviders() []string       { return a.Spec.inv.providers }
func (a *API) GetConsumers() []string       { return a.Spec.inv.consumers }
func (a *API) GetSystem() string            { return a.Spec.System }
func (a *API) AddProvider(p string)         { a.Spec.inv.providers = append(a.Spec.inv.providers, p) }
func (a *API) AddConsumer(c string)         { a.Spec.inv.consumers = append(a.Spec.inv.consumers, c) }
func (a *API) GetSourceInfo() *SourceInfo   { return a.SourceInfo }
func (a *API) SetSourceInfo(si *SourceInfo) { a.SourceInfo = si }
func (a *API) Reset() Entity {
	clone := *a
	spec := *a.Spec
	clone.Spec = &spec
	clone.Spec.inv = apiInvRel{}
	return &clone
}

func (r *Resource) GetKind() string              { return r.Kind }
func (r *Resource) GetMetadata() *Metadata       { return r.Metadata }
func (r *Resource) GetQName() string             { return r.Metadata.GetQName() }
func (r *Resource) GetRef() string               { return "resource:" + r.GetQName() }
func (r *Resource) GetDependents() []string      { return r.Spec.inv.dependents }
func (r *Resource) GetSystem() string            { return r.Spec.System }
func (r *Resource) AddDependent(d string)        { r.Spec.inv.dependents = append(r.Spec.inv.dependents, d) }
func (r *Resource) GetSourceInfo() *SourceInfo   { return r.SourceInfo }
func (r *Resource) SetSourceInfo(si *SourceInfo) { r.SourceInfo = si }
func (r *Resource) Reset() Entity {
	clone := *r
	spec := *r.Spec
	clone.Spec = &spec
	clone.Spec.inv = resourceInvRel{}
	return &clone
}

func (g *Group) GetKind() string              { return g.Kind }
func (g *Group) GetMetadata() *Metadata       { return g.Metadata }
func (g *Group) GetQName() string             { return g.Metadata.GetQName() }
func (g *Group) GetRef() string               { return "group:" + g.GetQName() }
func (g *Group) GetDisplayName() string       { return g.Spec.Profile.DisplayName }
func (g *Group) GetSourceInfo() *SourceInfo   { return g.SourceInfo }
func (g *Group) SetSourceInfo(si *SourceInfo) { g.SourceInfo = si }
func (g *Group) Reset() Entity {
	clone := *g
	spec := *g.Spec
	clone.Spec = &spec
	return &clone
}
