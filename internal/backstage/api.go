// This file contains the API classes that define a software catalog.
// The types are compatible with backstage.io's types:
// https://backstage.io/docs/features/software-catalog/descriptor-format#contents
package backstage

const (
	// The name of the (implicit) default namespace.
	DefaultNamespace = "default"
)

// Metadata

type Link struct {
	// A url in a standard uri format.
	URL string `yaml:"url"`
	// A user friendly display name for the link.
	Title string `yaml:"title"`
	// A key representing a visual icon to be displayed in the UI.
	Icon string `yaml:"icon"`
	// An optional value to categorize links into specific groups.
	Type string `yaml:"type"`
}

type Metadata struct {
	// The name of the entity. Must be unique within the catalog at any given point in time, for any given namespace + kind pair.
	Name string `yaml:"name"`
	// The namespace that the entity belongs to. If empty, the entity is assume to live in the default namespace.
	Namespace string `yaml:"namespace"`
	// A display name of the entity, to be presented in user interfaces instead of the name property, when available.
	Title string `yaml:"title"`
	// A short (typically relatively few words, on one line) description of the entity.
	Description string `yaml:"description"`
	// Key/value pairs of identifying information attached to the entity.
	Labels map[string]string `yaml:"labels"`
	// Key/value pairs of non-identifying auxiliary information attached to the entity.
	// Mostly used by plugins to store additional information about the entity.
	Annotations map[string]string `yaml:"annotations"`
	// A list of single-valued strings, to for example classify catalog entities in various ways.
	Tags []string `yaml:"tags"`
	// A list of external hyperlinks related to the entity.
	Links []*Link `yaml:"links"`
}

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

type Entity interface {
	GetKind() string
	GetMetadata() *Metadata
	GetQName() string
}

// Component

type ComponentSpec struct {
	// The type of component.
	// Should ideally be one of a few well-known values that are used consistently.
	// For example, ["service", "batch"].
	Type string `yaml:"type"`
	// The lifecycle state of the component.
	// Should ideally be one of a few well-known values that are used consistently.
	// For example, ["production", "test", "dev", "experimental"].
	Lifecycle string `yaml:"lifecycle"`
	// An entity reference to the owner of the component.
	Owner string `yaml:"owner"`
	// An entity reference to the system that the component belongs to.
	System string `yaml:"system"`
	// An entity reference to another component of which the component is a part.
	SubcomponentOf string `yaml:"subcomponentOf"`
	// An array of entity references to the APIs that are provided by the component.
	ProvidesAPIs []string `yaml:"providesApis"`
	// An array of entity references to the APIs that are consumed by the component.
	ConsumesAPIs []string `yaml:"consumesApis"`
	// An array of references to other entities that the component depends on to function.
	DependsOn []string `yaml:"dependsOn"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	dependents []string
}

type Component struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   *Metadata      `yaml:"metadata"`
	Spec       *ComponentSpec `yaml:"spec"`
}

func (c *Component) GetKind() string        { return c.Kind }
func (c *Component) GetMetadata() *Metadata { return c.Metadata }
func (c *Component) GetQName() string       { return c.Metadata.GetQName() }

// System

type SystemSpec struct {
	// An entity reference to the owner of the system.
	Owner string `yaml:"owner"`
	// An entity reference to the domain that the system belongs to.
	Domain string `yaml:"domain"`
	// The type of system. There is currently no enforced set of values for this field, so it is left up to the adopting organization to choose a nomenclature that matches their catalog hierarchy.
	Type string `yaml:"type"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	components []string
	apis       []string
	resources  []string
}

type System struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   *Metadata   `yaml:"metadata"`
	Spec       *SystemSpec `yaml:"spec"`
}

func (s *System) GetKind() string        { return s.Kind }
func (s *System) GetMetadata() *Metadata { return s.Metadata }
func (s *System) GetQName() string       { return s.Metadata.GetQName() }
func (s *System) Components() []string   { return s.Spec.components }
func (s *System) APIs() []string         { return s.Spec.apis }
func (s *System) Resources() []string    { return s.Spec.resources }

// Domain

type DomainSpec struct {
	// An entity reference to the owner of the domain.
	Owner string `yaml:"owner"`
	// An entity reference to another domain of which the domain is a part.
	SubdomainOf string `yaml:"subdomainOf"`
	// The type of domain. There is currently no enforced set of values for this field, so it is left up to the adopting organization to choose a nomenclature that matches their catalog hierarchy.
	Type string `yaml:"type"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	systems []string
}

type Domain struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   *Metadata   `yaml:"metadata"`
	Spec       *DomainSpec `yaml:"spec"`
}

func (d *Domain) GetKind() string        { return d.Kind }
func (d *Domain) GetMetadata() *Metadata { return d.Metadata }
func (d *Domain) GetQName() string       { return d.Metadata.GetQName() }
func (d *Domain) Systems() []string      { return d.Spec.systems }

// API

type APISpec struct {
	// The type of the API definition.
	Type string `yaml:"type"`
	// The lifecycle state of the API.
	Lifecycle string `yaml:"lifecycle"`
	// An entity reference to the owner of the API.
	Owner string `yaml:"owner"`
	// An entity reference to the system that the API belongs to.
	System string `yaml:"system"`
	// The definition of the API, based on the format defined by the type.
	Definition string `yaml:"definition"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	providers []string
	consumers []string
}

type API struct {
	APIVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   *Metadata `yaml:"metadata"`
	Spec       *APISpec  `yaml:"spec"`
}

func (a *API) GetKind() string        { return a.Kind }
func (a *API) GetMetadata() *Metadata { return a.Metadata }
func (a *API) GetQName() string       { return a.Metadata.GetQName() }
func (a *API) Providers() []string    { return a.Spec.providers }
func (a *API) Consumers() []string    { return a.Spec.consumers }

// Resource

type ResourceSpec struct {
	// The type of resource.
	Type string `yaml:"type"`
	// An entity reference to the owner of the resource.
	Owner string `yaml:"owner"`
	// An array of references to other entities that the resource depends on to function.
	DependsOn []string `yaml:"dependsOn"`
	// An entity reference to the system that the resource belongs to.
	System string `yaml:"system"`

	// These fields are not part of the Backstage API.
	// They are populated on demand to make "reverse navigation" easier.
	dependents []string
}

type Resource struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   *Metadata     `yaml:"metadata"`
	Spec       *ResourceSpec `yaml:"spec"`
}

func (r *Resource) GetKind() string        { return r.Kind }
func (r *Resource) GetMetadata() *Metadata { return r.Metadata }
func (r *Resource) GetQName() string       { return r.Metadata.GetQName() }
func (r *Resource) Dependents() []string   { return r.Spec.dependents }

// Group

type GroupSpecProfile struct {
	// A simple display name to present to users. Should always be set.
	DisplayName string `yaml:"displayName"`
	// An email where the group can be reached.
	Email string `yaml:"email"`
	// Optional URL of an image that represents this entity.
	Picture string `yaml:"picture"`
}

type GroupSpec struct {
	// The type of group. There is currently no enforced set of values for this field,
	// so it is left up to the adopting organization to choose a nomenclature that matches their org hierarchy.
	Type string `yaml:"type"`
	// Optional profile information about the group, mainly for display purposes.
	Profile *GroupSpecProfile `yaml:"profile"`
	// The immediate parent group in the hierarchy, if any.
	Parent string `yaml:"parent"`
	// The immediate child groups of this group in the hierarchy (whose parent field points to this group).
	// The list must be present, but may be empty if there are no child groups.
	Children []string `yaml:"children"`
	// The users that are members of this group. The entries of this array are entity references.
	Members []string `yaml:"members"`
}

type Group struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   *Metadata  `yaml:"metadata"`
	Spec       *GroupSpec `yaml:"spec"`
}

func (g *Group) GetKind() string        { return g.Kind }
func (g *Group) GetMetadata() *Metadata { return g.Metadata }
func (g *Group) GetQName() string       { return g.Metadata.GetQName() }
