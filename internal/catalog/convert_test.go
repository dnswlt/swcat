package catalog

import (
	"testing"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"gopkg.in/yaml.v3"
)

var commonOpts = []cmp.Option{
	cmp.AllowUnexported(
		Component{}, ComponentSpec{}, componentInvRel{},
		API{}, APISpec{}, apiInvRel{},
		System{}, SystemSpec{}, systemInvRel{},
		Domain{}, DomainSpec{}, domainInvRel{},
		Resource{}, ResourceSpec{}, resourceInvRel{},
		Group{}, GroupSpec{},
	),
	cmpopts.EquateEmpty(),
}

func TestCloneEntityFromAPI_Component(t *testing.T) {
	input := `
kind: Component
metadata:
  name: yankee
spec:
  type: service
  lifecycle: prod
  owner: team-x
  system: system	
`
	var node yaml.Node
	err := yaml.Unmarshal([]byte(input), &node)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	var component api.Component
	if err := node.Decode(&component); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	component.SetSourceInfo(&api.SourceInfo{
		Node: &node,
	})
	c, err := NewEntityFromAPI(&component)
	if err != nil {
		t.Fatalf("NewEntityFromAPI failed: %v", err)
	}
	cpy, err := cloneEntityFromAPI[*api.Component](c)
	if err != nil {
		t.Fatalf("CloneEntityFromAPI failed: %v", err)
	}
	if !cpy.GetRef().Equal(c.GetRef()) {
		t.Errorf("Refs differ: got: %s want: %s", cpy.GetRef(), c.GetRef())
	}
	if cpy.GetSourceInfo() != c.GetSourceInfo() {
		t.Error("SourceInfo pointers differ")
	}
}

func TestNewComponentFromAPI(t *testing.T) {
	si := &api.SourceInfo{Line: 1}
	apiComp := &api.Component{
		Kind: "Component",
		Metadata: &api.Metadata{
			Name:        "comp",
			Namespace:   "ns",
			Title:       "Title",
			Description: "Desc",
			Labels:      map[string]string{"l": "v"},
			Annotations: map[string]string{"a": "v"},
			Tags:        []string{"t1"},
			Links: []*api.Link{
				{URL: "http://url", Title: "Link", Icon: "icon", Type: "type"},
			},
		},
		Spec: &api.ComponentSpec{
			Type:           "service",
			Lifecycle:      "prod",
			Owner:          &api.Ref{Name: "owner", Kind: "group"},
			System:         &api.Ref{Name: "sys", Kind: "system"},
			SubcomponentOf: &api.Ref{Name: "parent", Kind: "component"},
			ProvidesAPIs: []*api.LabelRef{
				{Ref: &api.Ref{Name: "p1", Kind: "api"}, Label: "lbl1"},
			},
			ConsumesAPIs: []*api.LabelRef{
				{Ref: &api.Ref{Name: "c1", Kind: "api"}, Attrs: map[string]string{"v": "1"}},
			},
			DependsOn: []*api.LabelRef{
				{Ref: &api.Ref{Name: "d1", Kind: "resource"}},
			},
		},
		SourceInfo: si,
	}

	got, err := NewComponentFromAPI(apiComp)
	if err != nil {
		t.Fatalf("NewComponentFromAPI() error = %v", err)
	}

	want := &Component{
		Metadata: &Metadata{
			Name:        "comp",
			Namespace:   "ns",
			Title:       "Title",
			Description: "Desc",
			Labels:      map[string]string{"l": "v"},
			Annotations: map[string]string{"a": "v"},
			Tags:        []string{"t1"},
			Links: []*Link{
				{URL: "http://url", Title: "Link", Icon: "icon", Type: "type"},
			},
		},
		Spec: &ComponentSpec{
			Type:      "service",
			Lifecycle: "prod",
			Owner:     &Ref{Name: "owner", Namespace: "default", Kind: KindGroup},
			System:    &Ref{Name: "sys", Namespace: "default", Kind: KindSystem},
			SubcomponentOf: &Ref{Name: "parent", Namespace: "default", Kind: KindComponent},
			ProvidesAPIs: []*LabelRef{
				{Ref: &Ref{Name: "p1", Namespace: "default", Kind: KindAPI}, Label: "lbl1", Attrs: map[string]string{}},
			},
			ConsumesAPIs: []*LabelRef{
				{Ref: &Ref{Name: "c1", Namespace: "default", Kind: KindAPI}, Attrs: map[string]string{"v": "1"}},
			},
			DependsOn: []*LabelRef{
				{Ref: &Ref{Name: "d1", Namespace: "default", Kind: KindResource}, Attrs: map[string]string{}},
			},
		},
		sourceInfo: si,
	}

	if diff := cmp.Diff(want, got, commonOpts...); diff != "" {
		t.Errorf("NewComponentFromAPI() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewAPIFromAPI(t *testing.T) {
	si := &api.SourceInfo{Line: 2}
	apiEntity := &api.API{
		Kind: "API",
		Metadata: &api.Metadata{
			Name:      "my-api",
			Namespace: "default",
		},
		Spec: &api.APISpec{
			Type:       "openapi",
			Lifecycle:  "stable",
			Owner:      &api.Ref{Name: "owner", Kind: "group"},
			System:     &api.Ref{Name: "sys", Kind: "system"},
			Definition: "raw-def",
			Versions: []*api.APISpecVersion{
				{Version: api.Version{RawVersion: "v1"}, Lifecycle: "stable"},
			},
		},
		SourceInfo: si,
	}

	got, err := NewAPIFromAPI(apiEntity)
	if err != nil {
		t.Fatalf("NewAPIFromAPI() error = %v", err)
	}

	want := &API{
		Metadata: &Metadata{
			Name:        "my-api",
			Namespace:   "default",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: &APISpec{
			Type:       "openapi",
			Lifecycle:  "stable",
			Owner:      &Ref{Name: "owner", Namespace: "default", Kind: KindGroup},
			System:     &Ref{Name: "sys", Namespace: "default", Kind: KindSystem},
			Definition: "raw-def",
			Versions: []*APISpecVersion{
				{Version: Version{RawVersion: "v1"}, Lifecycle: "stable"},
			},
		},
		sourceInfo: si,
	}

	if diff := cmp.Diff(want, got, commonOpts...); diff != "" {
		t.Errorf("NewAPIFromAPI() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewSystemFromAPI(t *testing.T) {
	si := &api.SourceInfo{Line: 3}
	apiSystem := &api.System{
		Kind: "System",
		Metadata: &api.Metadata{
			Name: "my-system",
		},
		Spec: &api.SystemSpec{
			Type:   "app",
			Owner:  &api.Ref{Name: "owner", Kind: "group"},
			Domain: &api.Ref{Name: "domain", Kind: "domain"},
		},
		SourceInfo: si,
	}

	got, err := NewSystemFromAPI(apiSystem)
	if err != nil {
		t.Fatalf("NewSystemFromAPI() error = %v", err)
	}

	want := &System{
		Metadata: &Metadata{
			Name:        "my-system",
			Namespace:   "default",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: &SystemSpec{
			Type:   "app",
			Owner:  &Ref{Name: "owner", Namespace: "default", Kind: KindGroup},
			Domain: &Ref{Name: "domain", Namespace: "default", Kind: KindDomain},
		},
		sourceInfo: si,
	}

	if diff := cmp.Diff(want, got, commonOpts...); diff != "" {
		t.Errorf("NewSystemFromAPI() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewDomainFromAPI(t *testing.T) {
	si := &api.SourceInfo{Line: 4}
	apiDomain := &api.Domain{
		Kind: "Domain",
		Metadata: &api.Metadata{
			Name: "my-domain",
		},
		Spec: &api.DomainSpec{
			Type:        "business",
			Owner:       &api.Ref{Name: "owner", Kind: "group"},
			SubdomainOf: &api.Ref{Name: "parent", Kind: "domain"},
		},
		SourceInfo: si,
	}

	got, err := NewDomainFromAPI(apiDomain)
	if err != nil {
		t.Fatalf("NewDomainFromAPI() error = %v", err)
	}

	want := &Domain{
		Metadata: &Metadata{
			Name:        "my-domain",
			Namespace:   "default",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: &DomainSpec{
			Type:        "business",
			Owner:       &Ref{Name: "owner", Namespace: "default", Kind: KindGroup},
			SubdomainOf: &Ref{Name: "parent", Namespace: "default", Kind: KindDomain},
		},
		sourceInfo: si,
	}

	if diff := cmp.Diff(want, got, commonOpts...); diff != "" {
		t.Errorf("NewDomainFromAPI() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewResourceFromAPI(t *testing.T) {
	si := &api.SourceInfo{Line: 5}
	apiResource := &api.Resource{
		Kind: "Resource",
		Metadata: &api.Metadata{
			Name: "my-db",
		},
		Spec: &api.ResourceSpec{
			Type:   "database",
			Owner:  &api.Ref{Name: "owner", Kind: "group"},
			System: &api.Ref{Name: "sys", Kind: "system"},
			DependsOn: []*api.LabelRef{
				{Ref: &api.Ref{Name: "other-db", Kind: "resource"}},
			},
		},
		SourceInfo: si,
	}

	got, err := NewResourceFromAPI(apiResource)
	if err != nil {
		t.Fatalf("NewResourceFromAPI() error = %v", err)
	}

	want := &Resource{
		Metadata: &Metadata{
			Name:        "my-db",
			Namespace:   "default",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: &ResourceSpec{
			Type:   "database",
			Owner:  &Ref{Name: "owner", Namespace: "default", Kind: KindGroup},
			System: &Ref{Name: "sys", Namespace: "default", Kind: KindSystem},
			DependsOn: []*LabelRef{
				{Ref: &Ref{Name: "other-db", Namespace: "default", Kind: KindResource}, Attrs: map[string]string{}},
			},
		},
		sourceInfo: si,
	}

	if diff := cmp.Diff(want, got, commonOpts...); diff != "" {
		t.Errorf("NewResourceFromAPI() mismatch (-want +got):\n%s", diff)
	}
}

func TestNewGroupFromAPI(t *testing.T) {
	si := &api.SourceInfo{Line: 6}
	apiGroup := &api.Group{
		Kind: "Group",
		Metadata: &api.Metadata{
			Name: "my-group",
		},
		Spec: &api.GroupSpec{
			Type: "team",
			Profile: &api.GroupSpecProfile{
				DisplayName: "My Group",
				Email:       "g@example.com",
				Picture:     "pic.png",
			},
			Parent: &api.Ref{Name: "parent", Kind: "group"},
			Children: []*api.Ref{
				{Name: "child", Kind: "group"},
			},
			Members: []string{"user1", "user2"},
		},
		SourceInfo: si,
	}

	got, err := NewGroupFromAPI(apiGroup)
	if err != nil {
		t.Fatalf("NewGroupFromAPI() error = %v", err)
	}

	want := &Group{
		Metadata: &Metadata{
			Name:        "my-group",
			Namespace:   "default",
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: &GroupSpec{
			Type: "team",
			Profile: &GroupSpecProfile{
				DisplayName: "My Group",
				Email:       "g@example.com",
				Picture:     "pic.png",
			},
			Parent: &Ref{Name: "parent", Namespace: "default", Kind: KindGroup},
			Children: []*Ref{
				{Name: "child", Namespace: "default", Kind: KindGroup},
			},
			Members: []string{"user1", "user2"},
		},
		sourceInfo: si,
	}

	if diff := cmp.Diff(want, got, commonOpts...); diff != "" {
		t.Errorf("NewGroupFromAPI() mismatch (-want +got):\n%s", diff)
	}
}