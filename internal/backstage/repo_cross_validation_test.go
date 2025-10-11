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
	domain := &api.Domain{
		Kind:     "Domain",
		Metadata: &api.Metadata{Name: "my-domain"},
		Spec:     &api.DomainSpec{Owner: owner.GetRef()},
	}
	system := &api.System{
		Kind:     "System",
		Metadata: &api.Metadata{Name: "my-system"},
		Spec: &api.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	ap := &api.API{
		Kind:     "API",
		Metadata: &api.Metadata{Name: "my-api"},
		Spec: &api.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
		},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			ConsumesAPIs: []*api.LabelRef{
				{Ref: ap.GetRef()},
			},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(domain); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(system); err != nil {
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
	if !ap.GetConsumers()[0].Ref.Equal(component.GetRef()) {
		t.Errorf("api.GetConsumers()[0] = %q, want %q", ap.GetConsumers()[0], component.GetRef())
	}
}

func TestComponentConsumesApiQualified(t *testing.T) {
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "my-team"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	domain := &api.Domain{
		Kind:     "Domain",
		Metadata: &api.Metadata{Name: "my-domain"},
		Spec:     &api.DomainSpec{Owner: owner.GetRef()},
	}
	system := &api.System{
		Kind:     "System",
		Metadata: &api.Metadata{Name: "my-system"},
		Spec: &api.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	ap := &api.API{
		Kind:     "API",
		Metadata: &api.Metadata{Name: "my-api", Namespace: "ns"},
		Spec: &api.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
		},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			ConsumesAPIs: []*api.LabelRef{
				{Ref: ap.GetRef()},
			},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(domain); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(system); err != nil {
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
	if !ap.GetConsumers()[0].Ref.Equal(component.GetRef()) {
		t.Errorf("api.GetConsumers()[0] = %q, want %q", ap.GetConsumers()[0], component.GetRef())
	}
}

func TestComponentProvidesApi(t *testing.T) {
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "my-team"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	domain := &api.Domain{
		Kind:     "Domain",
		Metadata: &api.Metadata{Name: "my-domain"},
		Spec:     &api.DomainSpec{Owner: owner.GetRef()},
	}
	system := &api.System{
		Kind:     "System",
		Metadata: &api.Metadata{Name: "my-system"},
		Spec: &api.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	ap := &api.API{
		Kind:     "API",
		Metadata: &api.Metadata{Name: "my-api"},
		Spec: &api.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
		},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			ProvidesAPIs: []*api.LabelRef{
				{Ref: ap.GetRef()},
			},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(domain); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(system); err != nil {
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
	if !ap.GetProviders()[0].Ref.Equal(component.GetRef()) {
		t.Errorf("api.GetProviders()[0] = %q, want %q", ap.GetProviders()[0], component.GetRef())
	}
}

func TestComponentProvidesApiInvalid(t *testing.T) {
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "my-team"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	domain := &api.Domain{
		Kind:     "Domain",
		Metadata: &api.Metadata{Name: "my-domain"},
		Spec:     &api.DomainSpec{Owner: owner.GetRef()},
	}
	system := &api.System{
		Kind:     "System",
		Metadata: &api.Metadata{Name: "my-system"},
		Spec: &api.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			ProvidesAPIs: []*api.LabelRef{
				{Ref: &api.Ref{Kind: "api", Name: "no-such-api"}},
			},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(domain); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(system); err != nil {
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
	domain := &api.Domain{
		Kind:     "Domain",
		Metadata: &api.Metadata{Name: "my-domain"},
		Spec:     &api.DomainSpec{Owner: owner.GetRef()},
	}
	system := &api.System{
		Kind:     "System",
		Metadata: &api.Metadata{Name: "my-system"},
		Spec: &api.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	resource := &api.Resource{
		Kind:     "Resource",
		Metadata: &api.Metadata{Name: "my-resource"},
		Spec: &api.ResourceSpec{
			Type:   "database",
			Owner:  owner.GetRef(),
			System: system.GetRef(),
		},
	}
	component := &api.Component{
		Kind:     "Component",
		Metadata: &api.Metadata{Name: "my-component"},
		Spec: &api.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			DependsOn: []*api.LabelRef{
				{Ref: resource.GetRef()},
			},
		},
	}

	r := NewRepository()
	if err := r.AddEntity(owner); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(domain); err != nil {
		t.Fatal(err)
	}
	if err := r.AddEntity(system); err != nil {
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
	if !resource.GetDependents()[0].Ref.Equal(component.GetRef()) {
		t.Errorf("resource.GetDependents()[0] = %q, want %q", resource.GetDependents()[0], component.GetRef())
	}
}
