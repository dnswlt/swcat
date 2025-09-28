package backstage

const (
	DefaultNamespace = "default"
)

// Metadata

type Link struct {
	URL   string `yaml:"url"`
	Title string `yaml:"title"`
	Icon  string `yaml:"icon"`
	Type  string `yaml:"type"`
}

type Metadata struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
	Tags        []string          `yaml:"tags"`
	Links       []*Link           `yaml:"links"`
}

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
	Type           string   `yaml:"type"`
	Lifecycle      string   `yaml:"lifecycle"`
	Owner          string   `yaml:"owner"`
	System         string   `yaml:"system"`
	SubcomponentOf string   `yaml:"subcomponentOf"`
	ProvidesAPIs   []string `yaml:"providesApis"`
	ConsumesAPIs   []string `yaml:"consumesApis"`
	DependsOn      []string `yaml:"dependsOn"`

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
	Owner  string `yaml:"owner"`
	Domain string `yaml:"domain"`
	Type   string `yaml:"type"`

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
	Owner       string `yaml:"owner"`
	SubdomainOf string `yaml:"subdomainOf"`
	Type        string `yaml:"type"`
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

// API

type APISpec struct {
	Type       string `yaml:"type"`
	Lifecycle  string `yaml:"lifecycle"`
	Owner      string `yaml:"owner"`
	System     string `yaml:"system"`
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
	Type      string   `yaml:"type"`
	Owner     string   `yaml:"owner"`
	DependsOn []string `yaml:"dependsOn"`
	System    string   `yaml:"system"`

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
func (r *Resource) Dependents() []string { return r.Spec.dependents }

// Group

type GroupSpecProfile struct {
	DisplayName string `yaml:"displayName"`
	Email       string `yaml:"email"`
	Picture     string `yaml:"picture"`
}

type GroupSpec struct {
	Type     string            `yaml:"type"`
	Profile  *GroupSpecProfile `yaml:"profile"`
	Parent   string            `yaml:"parent"`
	Children []string          `yaml:"children"`
	Members  []string          `yaml:"members"`
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
