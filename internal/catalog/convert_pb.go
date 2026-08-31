package catalog

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dnswlt/swcat/internal/api"
	catalog_pb "github.com/dnswlt/swcat/internal/catalog/pb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
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

// RefToPB converts an entity reference to its protobuf form.
func RefToPB(r *Ref) *catalog_pb.Ref {
	if r == nil {
		return nil
	}
	return &catalog_pb.Ref{
		Kind:      string(r.Kind),
		Namespace: r.Namespace,
		Name:      r.Name,
	}
}

// RefFromPB converts a Protobuf Ref into a catalog Ref, validating the kind,
// name and namespace. The kind must be a canonical kind (e.g. "Component").
// An empty namespace defaults to the default namespace.
func RefFromPB(r *catalog_pb.Ref) (*Ref, error) {
	if r == nil {
		return nil, fmt.Errorf("nil reference")
	}
	return NewRefFromAPI(&api.Ref{
		Kind:      r.Kind,
		Namespace: r.Namespace,
		Name:      r.Name,
	})
}

// DependencyRelationLabel returns the short, stable string form of a
// DependencyRelation, suitable for storing and displaying. An unspecified
// relation degrades to the generic "dependency".
func DependencyRelationLabel(r catalog_pb.DependencyRelation) string {
	switch r {
	case catalog_pb.DependencyRelation_DEPENDENCY_RELATION_CALLS:
		return "calls"
	case catalog_pb.DependencyRelation_DEPENDENCY_RELATION_PRODUCES:
		return "produces"
	case catalog_pb.DependencyRelation_DEPENDENCY_RELATION_CONSUMES:
		return "consumes"
	default:
		return "dependency"
	}
}

func labelRefToPB(r *LabelRef) *catalog_pb.LabelRef {
	if r == nil {
		return nil
	}
	return &catalog_pb.LabelRef{
		Ref:   RefToPB(r.Ref),
		Label: r.Label,
		Attrs: r.Attrs,
	}
}

func linkGroupInfoToPB(g *LinkGroupInfo) *catalog_pb.LinkGroupInfo {
	if g == nil {
		return nil
	}
	return &catalog_pb.LinkGroupInfo{
		Group: g.Group,
		Label: g.Label,
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
		GroupInfo:   linkGroupInfoToPB(l.GroupInfo),
	}
}

// observationValueToPB parses raw (which is expected to be valid JSON) into a
// structpb.Value so that protojson renders it as native JSON. If parsing
// fails, the raw bytes are preserved as a JSON string instead of being lost.
func observationValueToPB(raw json.RawMessage) *structpb.Value {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return structpb.NewStringValue(string(raw))
	}
	pv, err := structpb.NewValue(v)
	if err != nil {
		return structpb.NewStringValue(string(raw))
	}
	return pv
}

func observationToPB(o Observation) *catalog_pb.Observation {
	return &catalog_pb.Observation{
		Value:     observationValueToPB(o.Value),
		Producer:  o.Producer,
		UpdatedAt: timestamppb.New(o.UpdatedAt),
		Version:   o.Version,
		Meta:      o.Meta,
	}
}

// observationValueFromPB renders a structpb.Value back into its native JSON
// form. It is the inverse of observationValueToPB. A nil value yields nil.
func observationValueFromPB(v *structpb.Value) json.RawMessage {
	if v == nil {
		return nil
	}
	b, err := protojson.Marshal(v)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

// ObservationFromPB converts a Protobuf Observation into its catalog
// representation. A nil UpdatedAt yields the zero time, which callers may
// replace with a default (e.g. time.Now()).
func ObservationFromPB(o *catalog_pb.Observation) Observation {
	if o == nil {
		return Observation{}
	}
	var updatedAt time.Time
	if o.UpdatedAt != nil {
		updatedAt = o.UpdatedAt.AsTime()
	}
	return Observation{
		Value:     observationValueFromPB(o.Value),
		Producer:  o.Producer,
		UpdatedAt: updatedAt,
		Version:   o.Version,
		Meta:      o.Meta,
	}
}

func statusToPB(s *Status) *catalog_pb.Status {
	if s == nil {
		return nil
	}
	pb := &catalog_pb.Status{}
	if len(s.Observations) > 0 {
		pb.Observations = make(map[string]*catalog_pb.Observation, len(s.Observations))
		for k, v := range s.Observations {
			pb.Observations[k] = observationToPB(v)
		}
	}
	return pb
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
		Owner:          RefToPB(c.Spec.Owner),
		System:         RefToPB(c.Spec.System),
		Domain:         RefToPB(c.GetDomain()),
		SubcomponentOf: RefToPB(c.Spec.SubcomponentOf),
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
		spec.Subcomponents = append(spec.Subcomponents, RefToPB(r))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindComponent),
		Metadata: metadataToPB(c.Metadata),
		Spec:     &catalog_pb.Entity_ComponentSpec{ComponentSpec: spec},
		Status:   statusToPB(c.GetStatus()),
	}
}

func systemToPB(s *System) *catalog_pb.Entity {
	if s == nil {
		return nil
	}
	spec := &catalog_pb.SystemSpec{
		Owner:  RefToPB(s.Spec.Owner),
		Domain: RefToPB(s.Spec.Domain),
		Type:   s.Spec.Type,
	}
	for _, r := range s.GetComponents() {
		spec.Components = append(spec.Components, RefToPB(r))
	}
	for _, r := range s.GetAPIs() {
		spec.Apis = append(spec.Apis, RefToPB(r))
	}
	for _, r := range s.GetResources() {
		spec.Resources = append(spec.Resources, RefToPB(r))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindSystem),
		Metadata: metadataToPB(s.Metadata),
		Spec:     &catalog_pb.Entity_SystemSpec{SystemSpec: spec},
		Status:   statusToPB(s.GetStatus()),
	}
}

func domainToPB(d *Domain) *catalog_pb.Entity {
	if d == nil {
		return nil
	}
	spec := &catalog_pb.DomainSpec{
		Owner:       RefToPB(d.Spec.Owner),
		SubdomainOf: RefToPB(d.Spec.SubdomainOf),
		Type:        d.Spec.Type,
	}
	for _, r := range d.GetSystems() {
		spec.Systems = append(spec.Systems, RefToPB(r))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindDomain),
		Metadata: metadataToPB(d.Metadata),
		Spec:     &catalog_pb.Entity_DomainSpec{DomainSpec: spec},
		Status:   statusToPB(d.GetStatus()),
	}
}

func resourceToPB(r *Resource) *catalog_pb.Entity {
	if r == nil {
		return nil
	}
	spec := &catalog_pb.ResourceSpec{
		Type:   r.Spec.Type,
		Owner:  RefToPB(r.Spec.Owner),
		System: RefToPB(r.Spec.System),
		Domain: RefToPB(r.GetDomain()),
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
		Status:   statusToPB(r.GetStatus()),
	}
}

func apiToPB(a *API) *catalog_pb.Entity {
	if a == nil {
		return nil
	}
	spec := &catalog_pb.ApiSpec{
		Type:       a.Spec.Type,
		Lifecycle:  a.Spec.Lifecycle,
		Owner:      RefToPB(a.Spec.Owner),
		System:     RefToPB(a.Spec.System),
		Domain:     RefToPB(a.GetDomain()),
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
		Status:   statusToPB(a.GetStatus()),
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
		Parent:  RefToPB(g.Spec.Parent),
		Members: g.Spec.Members,
	}
	for _, c := range g.Spec.Children {
		spec.Children = append(spec.Children, RefToPB(c))
	}
	return &catalog_pb.Entity{
		Kind:     string(KindGroup),
		Metadata: metadataToPB(g.Metadata),
		Spec:     &catalog_pb.Entity_GroupSpec{GroupSpec: spec},
	}
}
