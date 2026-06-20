package catalog

import (
	"fmt"
	"maps"

	"github.com/dnswlt/swcat/internal/api"
)

func APIRef(r *Ref) *api.Ref {
	return &api.Ref{
		Kind:      string(r.Kind),
		Namespace: r.Namespace,
		Name:      r.Name,
	}
}

// NewRefFromAPI creates a new catalog.Ref from the given api.Ref, taking the
// kind from r.Kind. An empty or unknown kind is rejected.
func NewRefFromAPI(r *api.Ref) (*Ref, error) {
	if r == nil {
		return nil, fmt.Errorf("nil reference")
	}
	if !api.IsValidKind(r.Kind) {
		return nil, fmt.Errorf("invalid kind: %q", r.Kind)
	}
	return validatedRef(Kind(r.Kind), r)
}

// NewRefFromAPIWithKind creates a new catalog.Ref from the given api.Ref using
// the supplied kind. r.Kind must be either empty or equal to kind.
func NewRefFromAPIWithKind(kind Kind, r *api.Ref) (*Ref, error) {
	if r == nil {
		return nil, fmt.Errorf("nil reference for kind %q", kind)
	}
	if !api.IsValidKind(string(kind)) {
		return nil, fmt.Errorf("invalid kind: %q", kind)
	}
	if r.Kind != "" && r.Kind != string(kind) {
		return nil, fmt.Errorf("kind mismatch in ref conversion: got %q, want %q", r.Kind, kind)
	}
	return validatedRef(kind, r)
}

// validatedRef builds a Ref with an already-resolved canonical kind, validating
// the name and defaulting/validating the namespace.
func validatedRef(kind Kind, r *api.Ref) (*Ref, error) {
	if !IsValidName(r.Name) {
		return nil, fmt.Errorf("invalid name: %q", r.Name)
	}
	namespace := r.Namespace
	if namespace == "" {
		namespace = DefaultNamespace
	}
	if !IsValidNamespace(namespace) {
		return nil, fmt.Errorf("invalid namespace: %q", namespace)
	}
	return &Ref{
		Kind:      kind,
		Namespace: namespace,
		Name:      r.Name,
	}, nil
}

func NewLabelRefFromAPIWithKind(kind Kind, r *api.LabelRef) (*LabelRef, error) {
	ref, err := NewRefFromAPIWithKind(kind, r.Ref)
	if err != nil {
		return nil, err
	}
	return &LabelRef{
		Ref:   ref,
		Label: r.Label,
		Attrs: maps.Clone(r.Attrs),
	}, nil
}

func NewLabelRefFromAPI(r *api.LabelRef) (*LabelRef, error) {
	ref, err := NewRefFromAPI(r.Ref)
	if err != nil {
		return nil, err
	}
	return &LabelRef{
		Ref:   ref,
		Label: r.Label,
		Attrs: maps.Clone(r.Attrs),
	}, nil
}

func NewLinkFromAPI(l *api.Link) (*Link, error) {
	if l == nil {
		return nil, fmt.Errorf("Link is nil")
	}
	return &Link{
		URL:   l.URL,
		Title: l.Title,
		Icon:  l.Icon,
		Type:  l.Type,
	}, nil
}

func NewMetadataFromAPI(m *api.Metadata) (*Metadata, error) {
	if m == nil {
		return nil, fmt.Errorf("Metadata is nil")
	}
	if !IsValidName(m.Name) {
		return nil, fmt.Errorf("invalid name: %q", m.Name)
	}
	namespace := DefaultNamespace
	if m.Namespace != "" {
		namespace = m.Namespace
	}
	if !IsValidNamespace(namespace) {
		return nil, fmt.Errorf("invalid namespace: %q", namespace)
	}
	meta := &Metadata{
		Name:        m.Name,
		Namespace:   namespace,
		Title:       m.Title,
		Description: m.Description,
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		Tags:        make([]string, len(m.Tags)),
		Links:       make([]*Link, len(m.Links)),
	}
	copy(meta.Tags, m.Tags)
	if m.Labels != nil {
		meta.Labels = maps.Clone(m.Labels)
	}
	if m.Annotations != nil {
		meta.Annotations = maps.Clone(m.Annotations)
	}
	for i, l := range m.Links {
		link, err := NewLinkFromAPI(l)
		if err != nil {
			return nil, err
		}
		meta.Links[i] = link
	}
	return meta, nil
}

func NewResourceSpecFromAPI(r *api.ResourceSpec) (*ResourceSpec, error) {
	if r == nil {
		return nil, fmt.Errorf("ResourceSpec is nil")
	}
	owner, err := NewRefFromAPIWithKind(KindGroup, r.Owner)
	if err != nil {
		return nil, fmt.Errorf("invalid owner ref: %v", err)
	}
	system, err := NewRefFromAPIWithKind(KindSystem, r.System)
	if err != nil {
		return nil, fmt.Errorf("invalid system ref: %v", err)
	}
	spec := &ResourceSpec{
		Owner:  owner,
		System: system,
		Type:   r.Type,
	}
	for _, d := range r.DependsOn {
		dep, err := NewLabelRefFromAPI(d)
		if err != nil {
			return nil, fmt.Errorf("invalid dependsOn ref: %v", err)
		}
		spec.DependsOn = append(spec.DependsOn, dep)
	}
	return spec, nil
}

func NewResourceFromAPI(r *api.Resource) (*Resource, error) {
	if r == nil {
		return nil, fmt.Errorf("Resource is nil")
	}
	meta, err := NewMetadataFromAPI(r.Metadata)
	if err != nil {
		return nil, err
	}
	spec, err := NewResourceSpecFromAPI(r.Spec)
	if err != nil {
		return nil, err
	}

	return &Resource{
		Metadata:   meta,
		Spec:       spec,
		sourceInfo: r.SourceInfo,
	}, nil
}

func NewAPISpecVersionFromAPI(v *api.APISpecVersion) (*APISpecVersion, error) {
	ver := &v.Version
	if ver.RawVersion == "" {
		return nil, fmt.Errorf("empty version")
	}
	if ver.Major < 0 || ver.Minor < 0 || ver.Patch < 0 {
		return nil, fmt.Errorf("negative version: %s", ver)
	}
	return &APISpecVersion{
		Version: Version{
			RawVersion: ver.RawVersion,
			Major:      ver.Major,
			Minor:      ver.Minor,
			Patch:      ver.Patch,
			Suffix:     ver.Suffix,
		},
		Lifecycle: v.Lifecycle,
	}, nil
}

func NewAPISpecFromAPI(a *api.APISpec) (*APISpec, error) {
	if a == nil {
		return nil, fmt.Errorf("APISpec is nil")
	}
	owner, err := NewRefFromAPIWithKind(KindGroup, a.Owner)
	if err != nil {
		return nil, fmt.Errorf("invalid owner ref: %v", err)
	}
	system, err := NewRefFromAPIWithKind(KindSystem, a.System)
	if err != nil {
		return nil, fmt.Errorf("invalid system ref: %v", err)
	}
	spec := &APISpec{
		Owner:      owner,
		System:     system,
		Type:       a.Type,
		Lifecycle:  a.Lifecycle,
		Definition: a.Definition,
		Versions:   make([]*APISpecVersion, len(a.Versions)),
	}
	for i, v := range a.Versions {
		ver, err := NewAPISpecVersionFromAPI(v)
		if err != nil {
			return nil, err
		}
		spec.Versions[i] = ver
	}
	return spec, nil
}

func NewAPIFromAPI(a *api.API) (*API, error) {
	if a == nil {
		return nil, fmt.Errorf("API is nil")
	}
	meta, err := NewMetadataFromAPI(a.Metadata)
	if err != nil {
		return nil, err
	}
	spec, err := NewAPISpecFromAPI(a.Spec)
	if err != nil {
		return nil, err
	}

	return &API{
		Metadata:   meta,
		Spec:       spec,
		sourceInfo: a.SourceInfo,
	}, nil
}

func NewSystemSpecFromAPI(s *api.SystemSpec) (*SystemSpec, error) {
	if s == nil {
		return nil, fmt.Errorf("SystemSpec is nil")
	}
	owner, err := NewRefFromAPIWithKind(KindGroup, s.Owner)
	if err != nil {
		return nil, fmt.Errorf("invalid owner ref: %v", err)
	}
	domain, err := NewRefFromAPIWithKind(KindDomain, s.Domain)
	if err != nil {
		return nil, fmt.Errorf("invalid domain ref: %v", err)
	}
	spec := &SystemSpec{
		Owner:  owner,
		Domain: domain,
		Type:   s.Type,
	}
	return spec, nil
}

func NewSystemFromAPI(s *api.System) (*System, error) {
	if s == nil {
		return nil, fmt.Errorf("System is nil")
	}
	meta, err := NewMetadataFromAPI(s.Metadata)
	if err != nil {
		return nil, err
	}
	spec, err := NewSystemSpecFromAPI(s.Spec)
	if err != nil {
		return nil, err
	}

	return &System{
		Metadata:   meta,
		Spec:       spec,
		sourceInfo: s.SourceInfo,
	}, nil
}

func NewComponentSpecFromAPI(c *api.ComponentSpec) (*ComponentSpec, error) {
	if c == nil {
		return nil, fmt.Errorf("ComponentSpec is nil")
	}
	owner, err := NewRefFromAPIWithKind(KindGroup, c.Owner)
	if err != nil {
		return nil, fmt.Errorf("invalid owner ref: %v", err)
	}
	system, err := NewRefFromAPIWithKind(KindSystem, c.System)
	if err != nil {
		return nil, fmt.Errorf("invalid system ref: %v", err)
	}
	spec := &ComponentSpec{
		Owner:     owner,
		System:    system,
		Type:      c.Type,
		Lifecycle: c.Lifecycle,
	}
	if c.SubcomponentOf != nil {
		parent, err := NewRefFromAPIWithKind(KindComponent, c.SubcomponentOf)
		if err != nil {
			return nil, fmt.Errorf("invalid subcomponentof ref: %v", err)
		}
		spec.SubcomponentOf = parent
	}
	for _, r := range c.ProvidesAPIs {
		ref, err := NewLabelRefFromAPIWithKind(KindAPI, r)
		if err != nil {
			return nil, fmt.Errorf("invalid providesApis ref: %v", err)
		}
		spec.ProvidesAPIs = append(spec.ProvidesAPIs, ref)
	}
	for _, r := range c.ConsumesAPIs {
		ref, err := NewLabelRefFromAPIWithKind(KindAPI, r)
		if err != nil {
			return nil, fmt.Errorf("invalid consumesApis ref: %v", err)
		}
		spec.ConsumesAPIs = append(spec.ConsumesAPIs, ref)
	}
	for _, r := range c.DependsOn {
		ref, err := NewLabelRefFromAPI(r)
		if err != nil {
			return nil, fmt.Errorf("invalid dependsOn ref: %v", err)
		}
		spec.DependsOn = append(spec.DependsOn, ref)
	}
	return spec, nil
}

func NewComponentFromAPI(c *api.Component) (*Component, error) {
	if c == nil {
		return nil, fmt.Errorf("Component is nil")
	}
	meta, err := NewMetadataFromAPI(c.Metadata)
	if err != nil {
		return nil, err
	}
	spec, err := NewComponentSpecFromAPI(c.Spec)
	if err != nil {
		return nil, err
	}

	return &Component{
		Metadata:   meta,
		Spec:       spec,
		sourceInfo: c.SourceInfo,
	}, nil
}

func NewDomainSpecFromAPI(d *api.DomainSpec) (*DomainSpec, error) {
	if d == nil {
		return nil, fmt.Errorf("DomainSpec is nil")
	}
	owner, err := NewRefFromAPIWithKind(KindGroup, d.Owner)
	if err != nil {
		return nil, fmt.Errorf("invalid owner ref: %v", err)
	}
	spec := &DomainSpec{
		Owner: owner,
		Type:  d.Type,
	}
	if d.SubdomainOf != nil {
		parent, err := NewRefFromAPIWithKind(KindDomain, d.SubdomainOf)
		if err != nil {
			return nil, fmt.Errorf("invalid subdomainof ref: %v", err)
		}
		spec.SubdomainOf = parent
	}
	return spec, nil
}

func NewDomainFromAPI(d *api.Domain) (*Domain, error) {
	if d == nil {
		return nil, fmt.Errorf("Domain is nil")
	}
	meta, err := NewMetadataFromAPI(d.Metadata)
	if err != nil {
		return nil, err
	}
	spec, err := NewDomainSpecFromAPI(d.Spec)
	if err != nil {
		return nil, err
	}

	return &Domain{
		Metadata:   meta,
		Spec:       spec,
		sourceInfo: d.SourceInfo,
	}, nil
}

func NewGroupSpecFromAPI(g *api.GroupSpec) (*GroupSpec, error) {
	if g == nil {
		return nil, fmt.Errorf("GroupSpec is nil")
	}
	if g.Profile == nil {
		return nil, fmt.Errorf("GroupSpecProfile is nil")
	}
	spec := &GroupSpec{
		Type: g.Type,
		Profile: &GroupSpecProfile{
			DisplayName: g.Profile.DisplayName,
			Email:       g.Profile.Email,
			Picture:     g.Profile.Picture,
		},
		Members: make([]string, len(g.Members)),
	}
	copy(spec.Members, g.Members)

	if g.Parent != nil {
		parent, err := NewRefFromAPIWithKind(KindGroup, g.Parent)
		if err != nil {
			return nil, fmt.Errorf("invalid parent ref: %v", err)
		}
		spec.Parent = parent
	}
	for _, c := range g.Children {
		child, err := NewRefFromAPIWithKind(KindGroup, c)
		if err != nil {
			return nil, fmt.Errorf("invalid child ref: %v", err)
		}
		spec.Children = append(spec.Children, child)
	}
	return spec, nil
}

func NewGroupFromAPI(g *api.Group) (*Group, error) {
	if g == nil {
		return nil, fmt.Errorf("Group is nil")
	}
	meta, err := NewMetadataFromAPI(g.Metadata)
	if err != nil {
		return nil, err
	}
	spec, err := NewGroupSpecFromAPI(g.Spec)
	if err != nil {
		return nil, err
	}

	return &Group{
		Metadata:   meta,
		Spec:       spec,
		sourceInfo: g.SourceInfo,
	}, nil
}

// cloneEntityFromAPI creates a clone (deep copy) of an entity by re-decoding its api.Entity node.
// All computed fields of the catalog entity will be missing in the cloned result.
func cloneEntityFromAPI[T api.Entity](e Entity) (Entity, error) {
	var apiVal T
	si := e.GetSourceInfo()
	if si == nil {
		return nil, fmt.Errorf("missing source info")
	}
	err := e.GetSourceInfo().Node.Decode(&apiVal)
	if err != nil {
		return nil, fmt.Errorf("CloneEntityFromAPI(): Could not decode api.API %s: %v", e.GetRef(), err)
	}
	// Copy over source info, which is not part of the decoded node.
	apiVal.SetSourceInfo(e.GetSourceInfo())
	cpy, err := NewEntityFromAPI(apiVal)
	if err != nil {
		return nil, fmt.Errorf("CloneEntityFromAPI(): Could not convert api.API %s: %v", e.GetRef(), err)
	}
	return cpy, nil
}

func NewEntityFromAPI(e api.Entity) (Entity, error) {
	switch t := e.(type) {
	case *api.Domain:
		return NewDomainFromAPI(t)
	case *api.System:
		return NewSystemFromAPI(t)
	case *api.Component:
		return NewComponentFromAPI(t)
	case *api.Resource:
		return NewResourceFromAPI(t)
	case *api.API:
		return NewAPIFromAPI(t)
	case *api.Group:
		return NewGroupFromAPI(t)
	}
	return nil, fmt.Errorf("unsupported entity type: %T", e)
}
