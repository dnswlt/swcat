package backstage

import (
	"testing"
)

func TestComponentConsumesApi(t *testing.T) {
	owner := &Group{
		Kind:     "Group",
		Metadata: &Metadata{Name: "my-team"},
		Spec:     &GroupSpec{Type: "team"},
	}
	api := &API{
		Kind:     "API",
		Metadata: &Metadata{Name: "my-api"},
		Spec: &APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     "my-team",
		},
	}
	component := &Component{
		Kind:     "Component",
		Metadata: &Metadata{Name: "my-component"},
		Spec: &ComponentSpec{
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
	if err := r.AddEntity(api); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(component); err != nil {
		t.Fatal(err)
	}

	if err := r.Validate(); err != nil {
		t.Errorf("Validate() error = %v, wantErr nil", err)
	}

	if len(api.Spec.consumers) != 1 {
		t.Fatalf("len(api.Spec.consumers) = %d, want 1", len(api.Spec.consumers))
	}
	if api.Spec.consumers[0] != "my-component" {
		t.Errorf("api.Spec.consumers[0] = %q, want %q", api.Spec.consumers[0], "my-component")
	}
}

func TestComponentConsumesApiQualified(t *testing.T) {
	owner := &Group{
		Kind:     "Group",
		Metadata: &Metadata{Name: "my-team"},
		Spec:     &GroupSpec{Type: "team"},
	}
	api := &API{
		Kind:     "API",
		Metadata: &Metadata{Name: "my-api", Namespace: "ns"},
		Spec: &APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     "my-team",
		},
	}
	component := &Component{
		Kind:     "Component",
		Metadata: &Metadata{Name: "my-component"},
		Spec: &ComponentSpec{
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
	if err := r.AddEntity(api); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(component); err != nil {
		t.Fatal(err)
	}

	if err := r.Validate(); err != nil {
		t.Errorf("Validate() error = %v, wantErr nil", err)
	}

	if len(api.Spec.consumers) != 1 {
		t.Fatalf("len(api.Spec.consumers) = %d, want 1", len(api.Spec.consumers))
	}
	if api.Spec.consumers[0] != "my-component" {
		t.Errorf("api.Spec.consumers[0] = %q, want %q", api.Spec.consumers[0], "my-component")
	}
}

func TestComponentProvidesApi(t *testing.T) {
	owner := &Group{
		Kind:     "Group",
		Metadata: &Metadata{Name: "my-team"},
		Spec:     &GroupSpec{Type: "team"},
	}
	api := &API{
		Kind:     "API",
		Metadata: &Metadata{Name: "my-api"},
		Spec: &APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     "my-team",
		},
	}
	component := &Component{
		Kind:     "Component",
		Metadata: &Metadata{Name: "my-component"},
		Spec: &ComponentSpec{
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
	if err := r.AddEntity(api); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(component); err != nil {
		t.Fatal(err)
	}

	if err := r.Validate(); err != nil {
		t.Errorf("Validate() error = %v, wantErr nil", err)
	}

	if len(api.Spec.providers) != 1 {
		t.Fatalf("len(api.Spec.providers) = %d, want 1", len(api.Spec.providers))
	}
	if api.Spec.providers[0] != "my-component" {
		t.Errorf("api.Spec.providers[0] = %q, want %q", api.Spec.providers[0], "my-component")
	}
}

func TestComponentProvidesApiInvalid(t *testing.T) {
	owner := &Group{
		Kind:     "Group",
		Metadata: &Metadata{Name: "my-team"},
		Spec:     &GroupSpec{Type: "team"},
	}
	component := &Component{
		Kind:     "Component",
		Metadata: &Metadata{Name: "my-component"},
		Spec: &ComponentSpec{
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
	owner := &Group{
		Kind:     "Group",
		Metadata: &Metadata{Name: "my-team"},
		Spec:     &GroupSpec{Type: "team"},
	}
	resource := &Resource{
		Kind:     "Resource",
		Metadata: &Metadata{Name: "my-resource"},
		Spec: &ResourceSpec{
			Type:  "database",
			Owner: "my-team",
		},
	}
	component := &Component{
		Kind:     "Component",
		Metadata: &Metadata{Name: "my-component"},
		Spec: &ComponentSpec{
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

	if len(resource.Spec.dependents) != 1 {
		t.Fatalf("len(resource.Spec.dependents) = %d, want 1", len(resource.Spec.dependents))
	}
	if resource.Spec.dependents[0] != "component:my-component" {
		t.Errorf("resource.Spec.dependents[0] = %q, want %q", resource.Spec.dependents[0], "component:my-component")
	}
}