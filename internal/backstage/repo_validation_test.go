package backstage

import (
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/api"
)

func TestValidateMandatoryComponentFields(t *testing.T) {
	cases := []struct {
		name      string
		component *api.Component
		owner     *api.Group
		system    *api.System
		domain    *api.Domain
		wantErr   string
	}{
		{
			name: "no metadata",
			component: &api.Component{
				Kind: "Component",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.type",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec:     &api.ComponentSpec{},
			},
			wantErr: "has no spec.type",
		},
		{
			name: "no spec.lifecycle",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					Type: "service",
				},
			},
			wantErr: "has no spec.lifecycle",
		},
		{
			name: "no spec.owner",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
				},
			},
			wantErr: `owner "" for component test is undefined`,
		},
		{
			name: "invalid spec.owner",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     "foo",
				},
			},
			wantErr: `owner "foo" for component test is undefined`,
		},
		{
			name: "invalid spec.system",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     "foo",
					System:    "bar",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
			wantErr: `system "bar" for component test is undefined`,
		},
		{
			name: "valid",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     "foo",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
		},
		{
			name: "valid with system",
			component: &api.Component{
				Kind:     "Component",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     "foo",
					System:    "bar",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
			system: &api.System{
				Kind:     "System",
				Metadata: &api.Metadata{Name: "bar"},
				Spec: &api.SystemSpec{
					Owner:  "foo",
					Domain: "test-domain",
				},
			},
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: &api.Metadata{Name: "test-domain"},
				Spec: &api.DomainSpec{
					Owner: "foo",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			if tc.owner != nil {
				if err := r.AddEntity(tc.owner); err != nil {
					t.Fatal(err)
				}
			}
			if tc.domain != nil {
				if err := r.AddEntity(tc.domain); err != nil {
					t.Fatal(err)
				}
			}
			if tc.system != nil {
				if err := r.AddEntity(tc.system); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.AddEntity(tc.component); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Validate() error = %q, wantErr %q", err, tc.wantErr)
				}
			}
		})
	}
}

func TestValidateMandatoryDomainFields(t *testing.T) {
	cases := []struct {
		name    string
		domain  *api.Domain
		owner   *api.Group
		wantErr string
	}{
		{
			name: "no metadata",
			domain: &api.Domain{
				Kind: "Domain",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: &api.Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.owner",
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: &api.Metadata{Name: "test"},
				Spec:     &api.DomainSpec{},
			},
			wantErr: `owner "" for domain test is undefined`,
		},
		{
			name: "invalid spec.owner",
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.DomainSpec{
					Owner: "foo",
				},
			},
			wantErr: `owner "foo" for domain test is undefined`,
		},
		{
			name: "valid",
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.DomainSpec{
					Owner: "foo",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			if tc.owner != nil {
				if err := r.AddEntity(tc.owner); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.AddEntity(tc.domain); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Validate() error = %q, wantErr %q", err, tc.wantErr)
				}
			}
		})
	}
}

func TestValidateMandatorySystemFields(t *testing.T) {
	cases := []struct {
		name    string
		system  *api.System
		owner   *api.Group
		domain  *api.Domain
		wantErr string
	}{
		{
			name: "no metadata",
			system: &api.System{
				Kind: "System",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			system: &api.System{
				Kind:     "System",
				Metadata: &api.Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.owner",
			system: &api.System{
				Kind:     "System",
				Metadata: &api.Metadata{Name: "test"},
				Spec:     &api.SystemSpec{},
			},
			wantErr: `owner "" for system test is undefined`,
		},
		{
			name: "no spec.domain",
			system: &api.System{
				Kind:     "System",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.SystemSpec{
					Owner: "foo",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
			wantErr: `domain "" for system test is undefined`,
		},
		{
			name: "invalid spec.domain",
			system: &api.System{
				Kind:     "System",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.SystemSpec{
					Owner:  "foo",
					Domain: "bar",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
			wantErr: `domain "bar" for system test is undefined`,
		},
		{
			name: "valid",
			system: &api.System{
				Kind:     "System",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.SystemSpec{
					Owner:  "foo",
					Domain: "bar",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: &api.Metadata{Name: "bar"},
				Spec: &api.DomainSpec{
					Owner: "foo",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			if tc.owner != nil {
				if err := r.AddEntity(tc.owner); err != nil {
					t.Fatal(err)
				}
			}
			if tc.domain != nil {
				if err := r.AddEntity(tc.domain); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.AddEntity(tc.system); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Validate() error = %q, wantErr %q", err, tc.wantErr)
				}
			}
		})
	}
}

func TestValidateMandatoryApiFields(t *testing.T) {
	cases := []struct {
		name    string
		api     *api.API
		owner   *api.Group
		system  *api.System
		domain  *api.Domain
		wantErr string
	}{
		{
			name: "no metadata",
			api: &api.API{
				Kind: "API",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			api: &api.API{
				Kind:     "API",
				Metadata: &api.Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.type",
			api: &api.API{
				Kind:     "API",
				Metadata: &api.Metadata{Name: "test"},
				Spec:     &api.APISpec{},
			},
			wantErr: "has no spec.type",
		},
		{
			name: "no spec.lifecycle",
			api: &api.API{
				Kind:     "API",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.APISpec{
					Type: "openapi",
				},
			},
			wantErr: "has no spec.lifecycle",
		},
		{
			name: "no spec.owner",
			api: &api.API{
				Kind:     "API",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
				},
			},
			wantErr: `owner "" for API test is undefined`,
		},
		{
			name: "invalid spec.owner",
			api: &api.API{
				Kind:     "API",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     "foo",
				},
			},
			wantErr: `owner "foo" for API test is undefined`,
		},
		{
			name: "invalid spec.system",
			api: &api.API{
				Kind:     "API",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     "foo",
					System:    "bar",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
			wantErr: `system "bar" for API test is undefined`,
		},
		{
			name: "valid",
			api: &api.API{
				Kind:     "API",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     "foo",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
		},
		{
			name: "valid with system",
			api: &api.API{
				Kind:     "API",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     "foo",
					System:    "bar",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
			system: &api.System{
				Kind:     "System",
				Metadata: &api.Metadata{Name: "bar"},
				Spec: &api.SystemSpec{
					Owner:  "foo",
					Domain: "test-domain",
				},
			},
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: &api.Metadata{Name: "test-domain"},
				Spec: &api.DomainSpec{
					Owner: "foo",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			if tc.owner != nil {
				if err := r.AddEntity(tc.owner); err != nil {
					t.Fatal(err)
				}
			}
			if tc.domain != nil {
				if err := r.AddEntity(tc.domain); err != nil {
					t.Fatal(err)
				}
			}
			if tc.system != nil {
				if err := r.AddEntity(tc.system); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.AddEntity(tc.api); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Validate() error = %q, wantErr %q", err, tc.wantErr)
				}
			}
		})
	}
}

func TestValidateMandatoryResourceFields(t *testing.T) {
	cases := []struct {
		name     string
		resource *api.Resource
		owner    *api.Group
		system   *api.System
		domain   *api.Domain
		wantErr  string
	}{
		{
			name: "no metadata",
			resource: &api.Resource{
				Kind: "Resource",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: &api.Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.type",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: &api.Metadata{Name: "test"},
				Spec:     &api.ResourceSpec{},
			},
			wantErr: "has no spec.type",
		},
		{
			name: "no spec.owner",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ResourceSpec{
					Type: "database",
				},
			},
			wantErr: `owner "" for resource test is undefined`,
		},
		{
			name: "invalid spec.owner",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ResourceSpec{
					Type:  "database",
					Owner: "foo",
				},
			},
			wantErr: `owner "foo" for resource test is undefined`,
		},
		{
			name: "invalid spec.system",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ResourceSpec{
					Type:   "database",
					Owner:  "foo",
					System: "bar",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
			wantErr: `system "bar" for resource test is undefined`,
		},
		{
			name: "valid",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ResourceSpec{
					Type:  "database",
					Owner: "foo",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
		},
		{
			name: "valid with system",
			resource: &api.Resource{
				Kind:     "Resource",
				Metadata: &api.Metadata{Name: "test"},
				Spec: &api.ResourceSpec{
					Type:   "database",
					Owner:  "foo",
					System: "bar",
				},
			},
			owner: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "foo"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
			system: &api.System{
				Kind:     "System",
				Metadata: &api.Metadata{Name: "bar"},
				Spec: &api.SystemSpec{
					Owner:  "foo",
					Domain: "test-domain",
				},
			},
			domain: &api.Domain{
				Kind:     "Domain",
				Metadata: &api.Metadata{Name: "test-domain"},
				Spec: &api.DomainSpec{
					Owner: "foo",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			if tc.owner != nil {
				if err := r.AddEntity(tc.owner); err != nil {
					t.Fatal(err)
				}
			}
			if tc.domain != nil {
				if err := r.AddEntity(tc.domain); err != nil {
					t.Fatal(err)
				}
			}
			if tc.system != nil {
				if err := r.AddEntity(tc.system); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.AddEntity(tc.resource); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Validate() error = %q, wantErr %q", err, tc.wantErr)
				}
			}
		})
	}
}

func TestValidateMandatoryGroupFields(t *testing.T) {
	cases := []struct {
		name    string
		group   *api.Group
		wantErr string
	}{
		{
			name: "no metadata",
			group: &api.Group{
				Kind: "Group",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			group: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.type",
			group: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "test"},
				Spec:     &api.GroupSpec{},
			},
			wantErr: "has no spec.type",
		},
		{
			name: "valid",
			group: &api.Group{
				Kind:     "Group",
				Metadata: &api.Metadata{Name: "test"},
				Spec:     &api.GroupSpec{Type: "team"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRepository()
			if err := r.AddEntity(tc.group); err != nil {
				t.Fatal(err)
			}

			err := r.Validate()

			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("Validate() error = %q, wantErr %q", err, tc.wantErr)
				}
			}
		})
	}
}
