package backstage

import (
	"testing"

	"github.com/dnswlt/swcat/internal/api"
)

func TestComponentConsumesApi(t *testing.T) {
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "my-team"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	ap := &api.API{
		Kind:     "API",
		Metadata: &api.Metadata{Name: "my-api"},
		Spec: &api.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     "my-team",
		},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:         "service",
			Lifecycle:    "production",
			Owner:        "my-team",
			ConsumesAPIs: []string{"my-api"},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(ap); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(component); err != nil {
		t.Fatal(err)
	}

	if err := r.Validate(); err != nil {
		t.Errorf("Validate() error = %v, wantErr nil", err)
	}

	if len(ap.GetConsumers()) != 1 {
		t.Fatalf("len(api.GetConsumers()) = %d, want 1", len(ap.GetConsumers()))
	}
	if ap.GetConsumers()[0] != "my-component" {
		t.Errorf("api.GetConsumers()[0] = %q, want %q", ap.GetConsumers()[0], "my-component")
	}
}

func TestComponentConsumesApiQualified(t *testing.T) {
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "my-team"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	ap := &api.API{
		Kind:     "API",
		Metadata: &api.Metadata{Name: "my-api", Namespace: "ns"},
		Spec: &api.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     "my-team",
		},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:         "service",
			Lifecycle:    "production",
			Owner:        "my-team",
			ConsumesAPIs: []string{"api:ns/my-api"},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(ap); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(component); err != nil {
		t.Fatal(err)
	}

	if err := r.Validate(); err != nil {
		t.Errorf("Validate() error = %v, wantErr nil", err)
	}

	if len(ap.GetConsumers()) != 1 {
		t.Fatalf("len(api.GetConsumers()) = %d, want 1", len(ap.GetConsumers()))
	}
	if ap.GetConsumers()[0] != "my-component" {
		t.Errorf("api.GetConsumers()[0] = %q, want %q", ap.GetConsumers()[0], "my-component")
	}
}

func TestComponentProvidesApi(t *testing.T) {
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "my-team"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	ap := &api.API{
		Kind:     "API",
		Metadata: &api.Metadata{Name: "my-api"},
		Spec: &api.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     "my-team",
		},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:         "service",
			Lifecycle:    "production",
			Owner:        "my-team",
			ProvidesAPIs: []string{"my-api"},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(ap); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(component); err != nil {
		t.Fatal(err)
	}

	if err := r.Validate(); err != nil {
		t.Errorf("Validate() error = %v, wantErr nil", err)
	}

	if len(ap.GetProviders()) != 1 {
		t.Fatalf("len(api.GetProviders()) = %d, want 1", len(ap.GetProviders()))
	}
	if ap.GetProviders()[0] != "my-component" {
		t.Errorf("api.GetProviders()[0] = %q, want %q", ap.GetProviders()[0], "my-component")
	}
}

func TestComponentProvidesApiInvalid(t *testing.T) {
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "my-team"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:         "service",
			Lifecycle:    "production",
			Owner:        "my-team",
			ProvidesAPIs: []string{"no-such-api"},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(component); err != nil {
		t.Fatal(err)
	}

	err := r.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil, wantErr not nil")
	}
}

func TestComponentDependsOnResource(t *testing.T) {
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "my-team"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	resource := &api.Resource{
		Kind:     "Resource",
		Metadata: &api.Metadata{Name: "my-resource"},
		Spec: &api.ResourceSpec{
			Type:  "database",
			Owner: "my-team",
		},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     "my-team",
			DependsOn: []string{"resource:my-resource"},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(resource); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(component); err != nil {
		t.Fatal(err)
	}

	if err := r.Validate(); err != nil {
		t.Errorf("Validate() error = %v, wantErr nil", err)
	}

	if len(resource.GetDependents()) != 1 {
		t.Fatalf("len(resource.GetDependents()) = %d, want 1", len(resource.GetDependents()))
	}
	if resource.GetDependents()[0] != "component:my-component" {
		t.Errorf("resource.GetDependents()[0] = %q, want %q", resource.GetDependents()[0], "component:my-component")
	}
}
