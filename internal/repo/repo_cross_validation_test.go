package repo

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

func TestComponentConsumesApi(t *testing.T) {
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "my-team"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "my-domain"},
		Spec:     &catalog.DomainSpec{Owner: owner.GetRef()},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "my-system"},
		Spec: &catalog.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	ap := &catalog.API{
		Metadata: &catalog.Metadata{Name: "my-api"},
		Spec: &catalog.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
		},
	}
	component := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "my-component"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			ConsumesAPIs: []*catalog.LabelRef{
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
		t.Fatalf("len(catalog.GetConsumers()) = %d, want 1", len(ap.GetConsumers()))
	}
	if !ap.GetConsumers()[0].Ref.Equal(component.GetRef()) {
		t.Errorf("catalog.GetConsumers()[0] = %q, want %q", ap.GetConsumers()[0], component.GetRef())
	}
}

func TestComponentConsumesApiQualified(t *testing.T) {
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "my-team"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "my-domain"},
		Spec:     &catalog.DomainSpec{Owner: owner.GetRef()},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "my-system"},
		Spec: &catalog.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	ap := &catalog.API{
		Metadata: &catalog.Metadata{Name: "my-api", Namespace: "ns"},
		Spec: &catalog.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
		},
	}
	component := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "my-component"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			ConsumesAPIs: []*catalog.LabelRef{
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
		t.Fatalf("len(catalog.GetConsumers()) = %d, want 1", len(ap.GetConsumers()))
	}
	if !ap.GetConsumers()[0].Ref.Equal(component.GetRef()) {
		t.Errorf("catalog.GetConsumers()[0] = %q, want %q", ap.GetConsumers()[0], component.GetRef())
	}
}

func TestComponentProvidesApi(t *testing.T) {
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "my-team"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "my-domain"},
		Spec:     &catalog.DomainSpec{Owner: owner.GetRef()},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "my-system"},
		Spec: &catalog.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	ap := &catalog.API{
		Metadata: &catalog.Metadata{Name: "my-api"},
		Spec: &catalog.APISpec{
			Type:      "openapi",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
		},
	}
	component := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "my-component"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			ProvidesAPIs: []*catalog.LabelRef{
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
		t.Fatalf("len(catalog.GetProviders()) = %d, want 1", len(ap.GetProviders()))
	}
	if !ap.GetProviders()[0].Ref.Equal(component.GetRef()) {
		t.Errorf("catalog.GetProviders()[0] = %q, want %q", ap.GetProviders()[0], component.GetRef())
	}
}

func TestComponentProvidesApiInvalid(t *testing.T) {
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "my-team"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "my-domain"},
		Spec:     &catalog.DomainSpec{Owner: owner.GetRef()},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "my-system"},
		Spec: &catalog.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	component := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "my-component"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			ProvidesAPIs: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: "api", Name: "no-such-api"}},
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
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "my-team"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "my-domain"},
		Spec:     &catalog.DomainSpec{Owner: owner.GetRef()},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "my-system"},
		Spec: &catalog.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	resource := &catalog.Resource{
		Metadata: &catalog.Metadata{Name: "my-resource"},
		Spec: &catalog.ResourceSpec{
			Type:   "database",
			Owner:  owner.GetRef(),
			System: system.GetRef(),
		},
	}
	component := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "my-component"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     owner.GetRef(),
			System:    system.GetRef(),
			DependsOn: []*catalog.LabelRef{
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
