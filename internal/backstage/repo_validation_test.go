package backstage

import (
	"testing"

	"github.com/dnswlt/swcat/internal/api"
)

func TestValidateMandatoryComponentFields(t *testing.T) {
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "group"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	domain := &api.Domain{
		Kind:     "Domain",
		Metadata: &api.Metadata{Name: "domain"},
		Spec: &api.DomainSpec{
			Owner: owner.GetRef(),
		},
	}
	system := &api.System{
		Kind:     "System",
		Metadata: &api.Metadata{Name: "system"},
		Spec: &api.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	metadata := &api.Metadata{
		Name: "component",
	}
	spec := &api.ComponentSpec{
		System:    system.GetRef(),
		Owner:     owner.GetRef(),
		Type:      "service",
		Lifecycle: "production",
	}
	cases := []struct {
		name      string
		component *api.Component
		wantErr   bool
	}{
		{
			name: "valid component",
			component: &api.Component{
				Kind:     "Component",
				Metadata: metadata,
				Spec:     spec,
			},
			wantErr: false,
		},
		{
			name: "no spec",
			component: &api.Component{
				Kind:     "Component",
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.type",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					System:    system.GetRef(),
					Owner:     owner.GetRef(),
					Lifecycle: "production",
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.lifecycle",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					System: system.GetRef(),
					Owner:  owner.GetRef(),
					Type:   "service",
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					System:    system.GetRef(),
					Type:      "service",
					Lifecycle: "production",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					System:    system.GetRef(),
					Owner:     &api.Ref{Kind: "group", Name: "foo"},
					Type:      "service",
					Lifecycle: "production",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.system",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     owner.GetRef(),
					System:    &api.Ref{Kind: "system", Name: "bar"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			for _, e := range []api.Entity{owner, domain, system} {
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
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "group"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	metadata := &api.Metadata{
		Name: "domain",
	}
	spec := &api.DomainSpec{
		Owner: owner.GetRef(),
	}
	cases := []struct {
		name    string
		domain  *api.Domain
		wantErr bool
	}{
		{
			name: "valid domain",
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: metadata,
				Spec:     spec,
			},
			wantErr: false,
		},
		{
			name: "no spec",
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: metadata,
				Spec:     &api.DomainSpec{},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: metadata,
				Spec: &api.DomainSpec{
					Owner: &api.Ref{Kind: "group", Name: "foo"},
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
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "group"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	domain := &api.Domain{
		Kind:     "Domain",
		Metadata: &api.Metadata{Name: "domain"},
		Spec: &api.DomainSpec{
			Owner: owner.GetRef(),
		},
	}
	metadata := &api.Metadata{
		Name: "system",
	}
	spec := &api.SystemSpec{
		Owner:  owner.GetRef(),
		Domain: domain.GetRef(),
	}
	cases := []struct {
		name    string
		system  *api.System
		wantErr bool
	}{
		{
			name: "valid system",
			system: &api.System{
				Kind:     "System",
				Metadata: metadata,
				Spec:     spec,
			},
			wantErr: false,
		},
		{
			name: "no spec",
			system: &api.System{
				Kind:     "System",
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			system: &api.System{
				Kind:     "System",
				Metadata: metadata,
				Spec: &api.SystemSpec{
					Domain: domain.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			system: &api.System{
				Kind:     "System",
				Metadata: metadata,
				Spec: &api.SystemSpec{
					Owner:  &api.Ref{Kind: "group", Name: "foo"},
					Domain: domain.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.domain",
			system: &api.System{
				Kind:     "System",
				Metadata: metadata,
				Spec: &api.SystemSpec{
					Owner: owner.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.domain",
			system: &api.System{
				Kind:     "System",
				Metadata: metadata,
				Spec: &api.SystemSpec{
					Owner:  owner.GetRef(),
					Domain: &api.Ref{Kind: "domain", Name: "foo"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			for _, e := range []api.Entity{owner, domain} {
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
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "group"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	domain := &api.Domain{
		Kind:     "Domain",
		Metadata: &api.Metadata{Name: "domain"},
		Spec: &api.DomainSpec{
			Owner: owner.GetRef(),
		},
	}
	system := &api.System{
		Kind:     "System",
		Metadata: &api.Metadata{Name: "system"},
		Spec: &api.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	metadata := &api.Metadata{
		Name: "api",
	}
	spec := &api.APISpec{
		Type:      "openapi",
		Lifecycle: "production",
		Owner:     owner.GetRef(),
		System:    system.GetRef(),
	}
	cases := []struct {
		name    string
		api     *api.API
		wantErr bool
	}{
		{
			name: "valid api",
			api: &api.API{
				Kind:     "API",
				Metadata: metadata,
				Spec:     spec,
			},
		},
		{
			name: "no spec",
			api: &api.API{
				Kind:     "API",
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.type",
			api: &api.API{
				Kind:     "API",
				Metadata: metadata,
				Spec: &api.APISpec{
					Lifecycle: "production",
					Owner:     owner.GetRef(),
					System:    system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.lifecycle",
			api: &api.API{
				Kind:     "API",
				Metadata: metadata,
				Spec: &api.APISpec{
					Type:   "openapi",
					Owner:  owner.GetRef(),
					System: system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			api: &api.API{
				Kind:     "API",
				Metadata: metadata,
				Spec: &api.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					System:    system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			api: &api.API{
				Kind:     "API",
				Metadata: metadata,
				Spec: &api.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     &api.Ref{Kind: "group", Name: "foo"},
					System:    system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.system",
			api: &api.API{
				Kind:     "API",
				Metadata: metadata,
				Spec: &api.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     owner.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.system",
			api: &api.API{
				Kind:     "API",
				Metadata: metadata,
				Spec: &api.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     owner.GetRef(),
					System:    &api.Ref{Kind: "system", Name: "foo"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			for _, e := range []api.Entity{owner, domain, system} {
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
	owner := &api.Group{
		Kind:     "Group",
		Metadata: &api.Metadata{Name: "group"},
		Spec:     &api.GroupSpec{Type: "team"},
	}
	domain := &api.Domain{
		Kind:     "Domain",
		Metadata: &api.Metadata{Name: "domain"},
		Spec: &api.DomainSpec{
			Owner: owner.GetRef(),
		},
	}
	system := &api.System{
		Kind:     "System",
		Metadata: &api.Metadata{Name: "system"},
		Spec: &api.SystemSpec{
			Owner:  owner.GetRef(),
			Domain: domain.GetRef(),
		},
	}
	metadata := &api.Metadata{
		Name: "resource",
	}
	spec := &api.ResourceSpec{
		Type:   "database",
		Owner:  owner.GetRef(),
		System: system.GetRef(),
	}
	cases := []struct {
		name     string
		resource *api.Resource
		wantErr  bool
	}{
		{
			name: "valid resource",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: metadata,
				Spec:     spec,
			},
		},
		{
			name: "no spec",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.type",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: metadata,
				Spec: &api.ResourceSpec{
					Owner:  owner.GetRef(),
					System: system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.owner",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: metadata,
				Spec: &api.ResourceSpec{
					Type:   "database",
					System: system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.owner",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: metadata,
				Spec: &api.ResourceSpec{
					Type:   "database",
					Owner:  &api.Ref{Kind: "group", Name: "foo"},
					System: system.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "no spec.system",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: metadata,
				Spec: &api.ResourceSpec{
					Type:  "database",
					Owner: owner.GetRef(),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid spec.system",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: metadata,
				Spec: &api.ResourceSpec{
					Type:   "database",
					Owner:  owner.GetRef(),
					System: &api.Ref{Kind: "system", Name: "foo"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			for _, e := range []api.Entity{owner, domain, system} {
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
	metadata := &api.Metadata{
		Name: "group",
	}
	spec := &api.GroupSpec{
		Type: "team",
	}
	cases := []struct {
		name    string
		group   *api.Group
		wantErr bool
	}{
		{
			name: "valid group",
			group: &api.Group{
				Kind:     "Group",
				Metadata: metadata,
				Spec:     spec,
			},
		},
		{
			name: "no spec",
			group: &api.Group{
				Kind:     "Group",
				Metadata: metadata,
			},
			wantErr: true,
		},
		{
			name: "no spec.type",
			group: &api.Group{
				Kind:     "Group",
				Metadata: metadata,
				Spec:     &api.GroupSpec{},
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
