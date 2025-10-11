package repo

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

func TestValidateMandatoryComponentFields(t *testing.T) {
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "group"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "domain"},
		Spec: &catalog.DomainSpec{
			Owner: owner.GetRef(),
		},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "system"},
		Spec: &catalog.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	metadata := &catalog.Metadata{
		Name: "component",
	}
	spec := &catalog.ComponentSpec{
		System:    system.GetRef(),
		Owner:     owner.GetRef(),
		Type:      "service",
		Lifecycle: "production",
	}
	cases := []struct {
		name      string
		component *catalog.Component
		wantErr   bool
	}{
		{
			name: "valid component",
			component: &catalog.Component{
				Metadata: metadata,
				Spec:     spec,
			},
			wantErr: false,
		},
		{
			name: "no spec",
			component: &catalog.Component{
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.type",
			component: &catalog.Component{
				Metadata: &catalog.Metadata{Name: "test"},
				Spec: &catalog.ComponentSpec{
					System:    system.GetRef(),
					Owner:     owner.GetRef(),
					Lifecycle: "production",
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.lifecycle",
			component: &catalog.Component{
				Metadata: &catalog.Metadata{Name: "test"},
				Spec: &catalog.ComponentSpec{
					System: system.GetRef(),
					Owner:  owner.GetRef(),
					Type:   "service",
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			component: &catalog.Component{
				Metadata: &catalog.Metadata{Name: "test"},
				Spec: &catalog.ComponentSpec{
					System:    system.GetRef(),
					Type:      "service",
					Lifecycle: "production",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			component: &catalog.Component{
				Metadata: &catalog.Metadata{Name: "test"},
				Spec: &catalog.ComponentSpec{
					System:    system.GetRef(),
					Owner:     &catalog.Ref{Kind: "group", Name: "foo"},
					Type:      "service",
					Lifecycle: "production",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.system",
			component: &catalog.Component{
				Metadata: &catalog.Metadata{Name: "test"},
				Spec: &catalog.ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     owner.GetRef(),
					System:    &catalog.Ref{Kind: "system", Name: "bar"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			for _, e := range []catalog.Entity{owner, domain, system} {
				if err := r.AddEntity(e); err != nil {
					t.Fatalf("r.AddEntity(%v): %v", e.GetRef(), err)
				}
			}

			if err := r.AddEntity(tc.component); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if !tc.wantErr {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else if err == nil {
				t.Errorf("Validate() no error, but wantErr %v", tc.wantErr)
			}
		})
	}
}

func TestValidateMandatoryDomainFields(t *testing.T) {
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "group"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	metadata := &catalog.Metadata{
		Name: "domain",
	}
	spec := &catalog.DomainSpec{
		Owner: owner.GetRef(),
	}
	cases := []struct {
		name    string
		domain  *catalog.Domain
		wantErr bool
	}{
		{
			name: "valid domain",
			domain: &catalog.Domain{
				Metadata: metadata,
				Spec:     spec,
			},
			wantErr: false,
		},
		{
			name: "no spec",
			domain: &catalog.Domain{
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			domain: &catalog.Domain{
				Metadata: metadata,
				Spec:     &catalog.DomainSpec{},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			domain: &catalog.Domain{
				Metadata: metadata,
				Spec: &catalog.DomainSpec{
					Owner: &catalog.Ref{Kind: "group", Name: "foo"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			if err := r.AddEntity(owner); err != nil {
				t.Fatalf("r.AddEntity(owner): %v", err)
			}
			if err := r.AddEntity(tc.domain); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if !tc.wantErr {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else if err == nil {
				t.Errorf("Validate() no error, but wantErr %v", tc.wantErr)
			}
		})
	}
}

func TestValidateMandatorySystemFields(t *testing.T) {
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "group"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "domain"},
		Spec: &catalog.DomainSpec{
			Owner: owner.GetRef(),
		},
	}
	metadata := &catalog.Metadata{
		Name: "system",
	}
	spec := &catalog.SystemSpec{
		Owner:  owner.GetRef(),
		Domain: domain.GetRef(),
	}
	cases := []struct {
		name    string
		system  *catalog.System
		wantErr bool
	}{
		{
			name: "valid system",
			system: &catalog.System{
				Metadata: metadata,
				Spec:     spec,
			},
			wantErr: false,
		},
		{
			name: "no spec",
			system: &catalog.System{
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			system: &catalog.System{
				Metadata: metadata,
				Spec: &catalog.SystemSpec{
					Domain: domain.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			system: &catalog.System{
				Metadata: metadata,
				Spec: &catalog.SystemSpec{
					Owner:  &catalog.Ref{Kind: "group", Name: "foo"},
					Domain: domain.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.domain",
			system: &catalog.System{
				Metadata: metadata,
				Spec: &catalog.SystemSpec{
					Owner: owner.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.domain",
			system: &catalog.System{
				Metadata: metadata,
				Spec: &catalog.SystemSpec{
					Owner:  owner.GetRef(),
					Domain: &catalog.Ref{Kind: "domain", Name: "foo"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			for _, e := range []catalog.Entity{owner, domain} {
				if err := r.AddEntity(e); err != nil {
					t.Fatalf("r.AddEntity(%v): %v", e.GetRef(), err)
				}
			}

			if err := r.AddEntity(tc.system); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if !tc.wantErr {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else if err == nil {
				t.Errorf("Validate() no error, but wantErr %v", tc.wantErr)
			}
		})
	}
}

func TestValidateMandatoryApiFields(t *testing.T) {
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "group"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "domain"},
		Spec: &catalog.DomainSpec{
			Owner: owner.GetRef(),
		},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "system"},
		Spec: &catalog.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	metadata := &catalog.Metadata{
		Name: "api",
	}
	spec := &catalog.APISpec{
		Type:      "openapi",
		Lifecycle: "production",
		Owner:     owner.GetRef(),
		System:    system.GetRef(),
	}
	cases := []struct {
		name    string
		api     *catalog.API
		wantErr bool
	}{
		{
			name: "valid api",
			api: &catalog.API{
				Metadata: metadata,
				Spec:     spec,
			},
		},
		{
			name: "no spec",
			api: &catalog.API{
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.type",
			api: &catalog.API{
				Metadata: metadata,
				Spec: &catalog.APISpec{
					Lifecycle: "production",
					Owner:     owner.GetRef(),
					System:    system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.lifecycle",
			api: &catalog.API{
				Metadata: metadata,
				Spec: &catalog.APISpec{
					Type:   "openapi",
					Owner:  owner.GetRef(),
					System: system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			api: &catalog.API{
				Metadata: metadata,
				Spec: &catalog.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					System:    system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			api: &catalog.API{
				Metadata: metadata,
				Spec: &catalog.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     &catalog.Ref{Kind: "group", Name: "foo"},
					System:    system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.system",
			api: &catalog.API{
				Metadata: metadata,
				Spec: &catalog.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     owner.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.system",
			api: &catalog.API{
				Metadata: metadata,
				Spec: &catalog.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     owner.GetRef(),
					System:    &catalog.Ref{Kind: "system", Name: "foo"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			for _, e := range []catalog.Entity{owner, domain, system} {
				if err := r.AddEntity(e); err != nil {
					t.Fatalf("r.AddEntity(%v): %v", e.GetRef(), err)
				}
			}

			if err := r.AddEntity(tc.api); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if !tc.wantErr {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else if err == nil {
				t.Errorf("Validate() no error, but wantErr %v", tc.wantErr)
			}
		})
	}
}

func TestValidateMandatoryResourceFields(t *testing.T) {
	owner := &catalog.Group{
		Metadata: &catalog.Metadata{Name: "group"},
		Spec:     &catalog.GroupSpec{Type: "team"},
	}
	domain := &catalog.Domain{
		Metadata: &catalog.Metadata{Name: "domain"},
		Spec: &catalog.DomainSpec{
			Owner: owner.GetRef(),
		},
	}
	system := &catalog.System{
		Metadata: &catalog.Metadata{Name: "system"},
		Spec: &catalog.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	metadata := &catalog.Metadata{
		Name: "resource",
	}
	spec := &catalog.ResourceSpec{
		Type:   "database",
		Owner:  owner.GetRef(),
		System: system.GetRef(),
	}
	cases := []struct {
		name     string
		resource *catalog.Resource
		wantErr  bool
	}{
		{
			name: "valid resource",
			resource: &catalog.Resource{
				Metadata: metadata,
				Spec:     spec,
			},
		},
		{
			name: "no spec",
			resource: &catalog.Resource{
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.type",
			resource: &catalog.Resource{
				Metadata: metadata,
				Spec: &catalog.ResourceSpec{
					Owner:  owner.GetRef(),
					System: system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			resource: &catalog.Resource{
				Metadata: metadata,
				Spec: &catalog.ResourceSpec{
					Type:   "database",
					System: system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			resource: &catalog.Resource{
				Metadata: metadata,
				Spec: &catalog.ResourceSpec{
					Type:   "database",
					Owner:  &catalog.Ref{Kind: "group", Name: "foo"},
					System: system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.system",
			resource: &catalog.Resource{
				Metadata: metadata,
				Spec: &catalog.ResourceSpec{
					Type:  "database",
					Owner: owner.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.system",
			resource: &catalog.Resource{
				Metadata: metadata,
				Spec: &catalog.ResourceSpec{
					Type:   "database",
					Owner:  owner.GetRef(),
					System: &catalog.Ref{Kind: "system", Name: "foo"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			for _, e := range []catalog.Entity{owner, domain, system} {
				if err := r.AddEntity(e); err != nil {
					t.Fatalf("r.AddEntity(%v): %v", e.GetRef(), err)
				}
			}

			if err := r.AddEntity(tc.resource); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if !tc.wantErr {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else if err == nil {
				t.Errorf("Validate() no error, but wantErr %v", tc.wantErr)
			}
		})
	}
}

func TestValidateMandatoryGroupFields(t *testing.T) {
	metadata := &catalog.Metadata{
		Name: "group",
	}
	spec := &catalog.GroupSpec{
		Type: "team",
	}
	cases := []struct {
		name    string
		group   *catalog.Group
		wantErr bool
	}{
		{
			name: "valid group",
			group: &catalog.Group{
				Metadata: metadata,
				Spec:     spec,
			},
		},
		{
			name: "no spec",
			group: &catalog.Group{
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.type",
			group: &catalog.Group{
				Metadata: metadata,
				Spec:     &catalog.GroupSpec{},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			if err := r.AddEntity(tc.group); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if !tc.wantErr {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else if err == nil {
				t.Errorf("Validate() no error, but wantErr %v", tc.wantErr)
			}
		})
	}
}
