package catalog

import (
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
)

// ToPB converts a catalog Entity to its Protobuf representation.
func ToPB(e Entity) *catalog_pb.Entity {
	if e == nil {
		return nil
	}
	switch t := e.(type) {
	case *Component:
		return componentToPB(t)
	case *System:
		return systemToPB(t)
	case *Domain:
		return domainToPB(t)
	case *Resource:
		return resourceToPB(t)
	case *API:
		return apiToPB(t)
	case *Group:
		return groupToPB(t)
	}
	return nil
}

func refToPB(r *Ref) *catalog_pb.Ref {
	if r == nil {
		return nil
	}
	return &catalog_pb.Ref{
		Kind:      string(r.Kind),
		Namespace: r.Namespace,
		Name:      r.Name,
	}
}

func labelRefToPB(r *LabelRef) *catalog_pb.LabelRef {
	if r == nil {
		return nil
	}
	return &catalog_pb.LabelRef{
		Ref:   refToPB(r.Ref),
		Label: r.Label,
		Attrs: r.Attrs,
	}
}

func linkToPB(l *Link) *catalog_pb.Link {
	if l == nil {
		return nil
	}
	return &catalog_pb.Link{
		Url:         l.URL,
		Title:       l.Title,
		Icon:        l.Icon,
		Type:        l.Type,
		IsGenerated: l.IsGenerated,
	}
}

func metadataToPB(m *Metadata) *catalog_pb.Metadata {
	if m == nil {
		return nil
	}
	pb := &catalog_pb.Metadata{
		Name:        m.Name,
		Namespace:   m.Namespace,
		Title:       m.Title,
		Description: m.Description,
		Labels:      m.Labels,
		Annotations: m.Annotations,
		Tags:        m.Tags,
	}
	for _, l := range m.Links {
		pb.Links = append(pb.Links, linkToPB(l))
	}
	return pb
}

func versionToPB(v Version) *catalog_pb.Version {
	return &catalog_pb.Version{
		RawVersion: v.RawVersion,
		Major:      int32(v.Major),
		Minor:      int32(v.Minor),
		Patch:      int32(v.Patch),
		Suffix:     v.Suffix,
	}
}

func componentToPB(c *Component) *catalog_pb.Entity {
	if c == nil {
		return nil
	}
	spec := &catalog_pb.ComponentSpec{
		Type:           c.Spec.Type,
		Lifecycle:      c.Spec.Lifecycle,
		Owner:          refToPB(c.Spec.Owner),
		System:         refToPB(c.Spec.System),
		SubcomponentOf: refToPB(c.Spec.SubcomponentOf),
	}
	for _, r := range c.Spec.ProvidesAPIs {
		spec.ProvidesApis = append(spec.ProvidesApis, labelRefToPB(r))
	}
	for _, r := range c.Spec.ConsumesAPIs {
		spec.ConsumesApis = append(spec.ConsumesApis, labelRefToPB(r))
	}
	for _, r := range c.Spec.DependsOn {
		spec.DependsOn = append(spec.DependsOn, labelRefToPB(r))
	}
	for _, r := range c.GetDependents() {
		spec.Dependents = append(spec.Dependents, labelRefToPB(r))
	}
	for _, r := range c.GetSubcomponents() {
		spec.Subcomponents = append(spec.Subcomponents, refToPB(r))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindComponent),
		Metadata: metadataToPB(c.Metadata),
		Spec:     &catalog_pb.Entity_ComponentSpec{ComponentSpec: spec},
	}
}

func systemToPB(s *System) *catalog_pb.Entity {
	if s == nil {
		return nil
	}
	spec := &catalog_pb.SystemSpec{
		Owner:  refToPB(s.Spec.Owner),
		Domain: refToPB(s.Spec.Domain),
		Type:   s.Spec.Type,
	}
	for _, r := range s.GetComponents() {
		spec.Components = append(spec.Components, refToPB(r))
	}
	for _, r := range s.GetAPIs() {
		spec.Apis = append(spec.Apis, refToPB(r))
	}
	for _, r := range s.GetResources() {
		spec.Resources = append(spec.Resources, refToPB(r))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindSystem),
		Metadata: metadataToPB(s.Metadata),
		Spec:     &catalog_pb.Entity_SystemSpec{SystemSpec: spec},
	}
}

func domainToPB(d *Domain) *catalog_pb.Entity {
	if d == nil {
		return nil
	}
	spec := &catalog_pb.DomainSpec{
		Owner:       refToPB(d.Spec.Owner),
		SubdomainOf: refToPB(d.Spec.SubdomainOf),
		Type:        d.Spec.Type,
	}
	for _, r := range d.GetSystems() {
		spec.Systems = append(spec.Systems, refToPB(r))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindDomain),
		Metadata: metadataToPB(d.Metadata),
		Spec:     &catalog_pb.Entity_DomainSpec{DomainSpec: spec},
	}
}

func resourceToPB(r *Resource) *catalog_pb.Entity {
	if r == nil {
		return nil
	}
	spec := &catalog_pb.ResourceSpec{
		Type:   r.Spec.Type,
		Owner:  refToPB(r.Spec.Owner),
		System: refToPB(r.Spec.System),
	}
	for _, d := range r.Spec.DependsOn {
		spec.DependsOn = append(spec.DependsOn, labelRefToPB(d))
	}
	for _, d := range r.GetDependents() {
		spec.Dependents = append(spec.Dependents, labelRefToPB(d))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindResource),
		Metadata: metadataToPB(r.Metadata),
		Spec:     &catalog_pb.Entity_ResourceSpec{ResourceSpec: spec},
	}
}

func apiToPB(a *API) *catalog_pb.Entity {
	if a == nil {
		return nil
	}
	spec := &catalog_pb.ApiSpec{
		Type:       a.Spec.Type,
		Lifecycle:  a.Spec.Lifecycle,
		Owner:      refToPB(a.Spec.Owner),
		System:     refToPB(a.Spec.System),
		Definition: a.Spec.Definition,
	}
	for _, v := range a.Spec.Versions {
		spec.Versions = append(spec.Versions, &catalog_pb.ApiSpecVersion{
			Version:   versionToPB(v.Version),
			Lifecycle: v.Lifecycle,
		})
	}
	for _, r := range a.GetProviders() {
		spec.Providers = append(spec.Providers, labelRefToPB(r))
	}
	for _, r := range a.GetConsumers() {
		spec.Consumers = append(spec.Consumers, labelRefToPB(r))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindAPI),
		Metadata: metadataToPB(a.Metadata),
		Spec:     &catalog_pb.Entity_ApiSpec{ApiSpec: spec},
	}
}

func groupToPB(g *Group) *catalog_pb.Entity {
	if g == nil {
		return nil
	}
	spec := &catalog_pb.GroupSpec{
		Type: g.Spec.Type,
		Profile: &catalog_pb.GroupSpecProfile{
			DisplayName: g.Spec.Profile.DisplayName,
			Email:       g.Spec.Profile.Email,
			Picture:     g.Spec.Profile.Picture,
		},
		Parent:  refToPB(g.Spec.Parent),
		Members: g.Spec.Members,
	}
	for _, c := range g.Spec.Children {
		spec.Children = append(spec.Children, refToPB(c))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindGroup),
		Metadata: metadataToPB(g.Metadata),
		Spec:     &catalog_pb.Entity_GroupSpec{GroupSpec: spec},
	}
}