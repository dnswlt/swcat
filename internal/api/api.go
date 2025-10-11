// This file contains the API classes that define a software catalog.
// The types are broadly compatible with backstage.io's types:
// https://backstage.io/docs/features/software-catalog/descriptor-format#contents
package api

import (
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// The name of the (implicit) default namespace.
	// In swcat, entity references typically omit the default namespace
	// even in fully qualified form (e.g., resource:my-resource).
	DefaultNamespace = "default"
)

type Ref struct {
	Kind      string
	Namespace string
	Name      string
}

type LabelRef struct {
	Ref   *Ref
	Label string
}

// Entity is the interface implemented by all entity kinds (Component, System, etc.).
type Entity interface {
	GetKind() string
	GetMetadata() *Metadata
	GetRef() *Ref

	// GetSourceInfo returns internal bookkeeping data, e.g. for error logging
	// and reconstructing YAML files (retaining the exact structure including comments).
	GetSourceInfo() *SourceInfo
	SetSourceInfo(si *SourceInfo)
}

// File and line information shared by all entities.
// Can be used in error messages and to reconstruct YAML
type SourceInfo struct {
	Node *yaml.Node // The raw YAML source code from which the entity was parsed.
	Path string     // The path from which the entity was read.
	Line int        // The first line number in Path where the entity was found.
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

type DomainSpec struct {
	// An entity reference to the owner of the domain.
	// [required]
	Owner *Ref `yaml:"owner,omitempty"`
	// An entity reference to another domain of which the domain is a part.
	// [optional]
	SubdomainOf *Ref `yaml:"subdomainOf,omitempty"`
	// The type of domain. There is currently no enforced set of values for this field,
	// so it is left up to the adopting organization to choose a nomenclature that matches
	// their catalog hierarchy.
	// [optional]
	Type string `yaml:"type,omitempty"`
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

type SystemSpec struct {
	// An entity reference to the owner of the system.
	// [required]
	Owner *Ref `yaml:"owner,omitempty"`
	// An entity reference to the domain that the system belongs to.
	// [optional]
	Domain *Ref `yaml:"domain,omitempty"`
	// The type of system. There is currently no enforced set of values for this field,
	// so it is left up to the adopting organization to choose a nomenclature that matches
	// their catalog hierarchy.
	// [optional]
	Type string `yaml:"type,omitempty"`
}

type System struct {
	APIVersion string      `yaml:"apiVersion,omitempty"`
	Kind       string      `yaml:"kind,omitempty"`
	Metadata   *Metadata   `yaml:"metadata,omitempty"`
	Spec       *SystemSpec `yaml:"spec,omitempty"`

	// Internal bookkeeping data, not part of the API.
	*SourceInfo `yaml:"-"`
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
	Owner *Ref `yaml:"owner,omitempty"`
	// An entity reference to the system that the component belongs to.
	// [required]
	System *Ref `yaml:"system,omitempty"`
	// An entity reference to another component of which the component is a part.
	// [optional]
	SubcomponentOf *Ref `yaml:"subcomponentOf,omitempty"`
	// An array of entity references to the APIs that are provided by the component.
	// [optional]
	ProvidesAPIs []*LabelRef `yaml:"providesApis,omitempty"`
	// An array of entity references to the APIs that are consumed by the component.
	// [optional]
	ConsumesAPIs []*LabelRef `yaml:"consumesApis,omitempty"`
	// An array of references to other entities that the component depends on to function.
	// [optional]
	DependsOn []*LabelRef `yaml:"dependsOn,omitempty"`
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

type ResourceSpec struct {
	// The type of resource.
	// [required]
	Type string `yaml:"type,omitempty"`
	// An entity reference to the owner of the resource.
	// [required]
	Owner *Ref `yaml:"owner,omitempty"`
	// An array of references to other entities that the resource depends on to function.
	// [optional]
	DependsOn []*LabelRef `yaml:"dependsOn,omitempty"`
	// An entity reference to the system that the resource belongs to.
	// [required]
	System *Ref `yaml:"system,omitempty"`
}

type Resource struct {
	APIVersion string        `yaml:"apiVersion,omitempty"`
	Kind       string        `yaml:"kind,omitempty"`
	Metadata   *Metadata     `yaml:"metadata,omitempty"`
	Spec       *ResourceSpec `yaml:"spec,omitempty"`

	// Internal bookkeeping data, not part of the API.
	*SourceInfo `yaml:"-"`
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
	Owner *Ref `yaml:"owner,omitempty"`
	// An entity reference to the system that the API belongs to.
	// [required]
	System *Ref `yaml:"system,omitempty"`
	// The definition of the API, based on the format defined by the type.
	// A required field in the backstage.io schema, but we leave it as optional.
	// [optional]
	Definition string `yaml:"definition,omitempty"`
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
	Parent *Ref `yaml:"parent,omitempty"`
	// The immediate child groups of this group in the hierarchy (whose parent field points to this group).
	// In the backstage.io schema the list must be present, but may be empty if there are no child groups.
	// [optional]
	Children []*Ref `yaml:"children"`
	// The users that are members of this group. The entries of this array are uninterpreted strings.
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

func (e *Ref) Equal(other *Ref) bool {
	return e.Kind == other.Kind && e.Namespace == other.Namespace && e.Name == other.Name
}

func (e *Ref) String() string {
	var sb strings.Builder
	if e.Kind != "" {
		sb.WriteString(e.Kind + ":")
	}
	if e.Namespace != "" && e.Namespace != DefaultNamespace {
		sb.WriteString(e.Namespace + "/")
	}
	sb.WriteString(e.Name)
	return sb.String()
}

func (e *LabelRef) String() string {
	refStr := e.Ref.String()
	if e.Label == "" {
		return refStr
	}
	return refStr + ` "` + e.Label + `"`
}

func eRef(kind string, meta *Metadata) *Ref {
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
func (c *Component) GetKind() string        { return c.Kind }
func (c *Component) GetMetadata() *Metadata { return c.Metadata }
func (c *Component) GetRef() *Ref           { return eRef("component", c.Metadata) }

func (c *Component) GetSourceInfo() *SourceInfo   { return c.SourceInfo }
func (c *Component) SetSourceInfo(si *SourceInfo) { c.SourceInfo = si }

func (s *System) GetKind() string        { return s.Kind }
func (s *System) GetMetadata() *Metadata { return s.Metadata }
func (s *System) GetRef() *Ref           { return eRef("system", s.Metadata) }

func (s *System) GetSourceInfo() *SourceInfo   { return s.SourceInfo }
func (s *System) SetSourceInfo(si *SourceInfo) { s.SourceInfo = si }

func (d *Domain) GetKind() string        { return d.Kind }
func (d *Domain) GetMetadata() *Metadata { return d.Metadata }
func (d *Domain) GetRef() *Ref           { return eRef("domain", d.Metadata) }

func (d *Domain) GetSourceInfo() *SourceInfo   { return d.SourceInfo }
func (d *Domain) SetSourceInfo(si *SourceInfo) { d.SourceInfo = si }

func (a *API) GetKind() string        { return a.Kind }
func (a *API) GetMetadata() *Metadata { return a.Metadata }
func (a *API) GetRef() *Ref           { return eRef("api", a.Metadata) }

func (a *API) GetSourceInfo() *SourceInfo   { return a.SourceInfo }
func (a *API) SetSourceInfo(si *SourceInfo) { a.SourceInfo = si }

func (r *Resource) GetKind() string        { return r.Kind }
func (r *Resource) GetMetadata() *Metadata { return r.Metadata }
func (r *Resource) GetRef() *Ref           { return eRef("resource", r.Metadata) }

func (r *Resource) GetSourceInfo() *SourceInfo   { return r.SourceInfo }
func (r *Resource) SetSourceInfo(si *SourceInfo) { r.SourceInfo = si }

func (g *Group) GetKind() string        { return g.Kind }
func (g *Group) GetMetadata() *Metadata { return g.Metadata }
func (g *Group) GetRef() *Ref           { return eRef("group", g.Metadata) }

func (g *Group) GetSourceInfo() *SourceInfo   { return g.SourceInfo }
func (g *Group) SetSourceInfo(si *SourceInfo) { g.SourceInfo = si }
