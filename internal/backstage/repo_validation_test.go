package backstage

import (
	"strings"
	"testing"
)

func TestValidateMandatoryComponentFields(t *testing.T) {
	cases := []struct {
		name      string
		component *Component
		owner     *Group
		system    *System
		domain    *Domain
		wantErr   string
	}{
		{
			name: "no metadata",
			component: &Component{
				Kind: "Component",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			component: &Component{
				Kind:     "Component",
				Metadata: &Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.type",
			component: &Component{
				Kind:     "Component",
				Metadata: &Metadata{Name: "test"},
				Spec:     &ComponentSpec{},
			},
			wantErr: "has no spec.type",
		},
		{
			name: "no spec.lifecycle",
			component: &Component{
				Kind:     "Component",
				Metadata: &Metadata{Name: "test"},
				Spec: &ComponentSpec{
					Type: "service",
				},
			},
			wantErr: "has no spec.lifecycle",
		},
		{
			name: "no spec.owner",
			component: &Component{
				Kind:     "Component",
				Metadata: &Metadata{Name: "test"},
				Spec: &ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
				},
			},
			wantErr: `owner "" for component test is undefined`,
		},
		{
			name: "invalid spec.owner",
			component: &Component{
				Kind:     "Component",
				Metadata: &Metadata{Name: "test"},
				Spec: &ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     "foo",
				},
			},
			wantErr: `owner "foo" for component test is undefined`,
		},
		{
			name: "invalid spec.system",
			component: &Component{
				Kind:     "Component",
				Metadata: &Metadata{Name: "test"},
				Spec: &ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     "foo",
					System:    "bar",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
			wantErr: `system "bar" for component test is undefined`,
		},
		{
			name: "valid",
			component: &Component{
				Kind:     "Component",
				Metadata: &Metadata{Name: "test"},
				Spec: &ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     "foo",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
		},
		{
			name: "valid with system",
			component: &Component{
				Kind:     "Component",
				Metadata: &Metadata{Name: "test"},
				Spec: &ComponentSpec{
					Type:      "service",
					Lifecycle: "production",
					Owner:     "foo",
					System:    "bar",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
			system: &System{
				Kind:     "System",
				Metadata: &Metadata{Name: "bar"},
				Spec: &SystemSpec{
					Owner:  "foo",
					Domain: "test-domain",
				},
			},
			domain: &Domain{
				Kind:     "Domain",
				Metadata: &Metadata{Name: "test-domain"},
				Spec: &DomainSpec{
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
		domain  *Domain
		owner   *Group
		wantErr string
	}{
		{
			name: "no metadata",
			domain: &Domain{
				Kind: "Domain",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			domain: &Domain{
				Kind:     "Domain",
				Metadata: &Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.owner",
			domain: &Domain{
				Kind:     "Domain",
				Metadata: &Metadata{Name: "test"},
				Spec:     &DomainSpec{},
			},
			wantErr: `owner "" for domain test is undefined`,
		},
		{
			name: "invalid spec.owner",
			domain: &Domain{
				Kind:     "Domain",
				Metadata: &Metadata{Name: "test"},
				Spec: &DomainSpec{
					Owner: "foo",
				},
			},
			wantErr: `owner "foo" for domain test is undefined`,
		},
		{
			name: "valid",
			domain: &Domain{
				Kind:     "Domain",
				Metadata: &Metadata{Name: "test"},
				Spec: &DomainSpec{
					Owner: "foo",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
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
		system  *System
		owner   *Group
		domain  *Domain
		wantErr string
	}{
		{
			name: "no metadata",
			system: &System{
				Kind: "System",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			system: &System{
				Kind:     "System",
				Metadata: &Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.owner",
			system: &System{
				Kind:     "System",
				Metadata: &Metadata{Name: "test"},
				Spec:     &SystemSpec{},
			},
			wantErr: `owner "" for system test is undefined`,
		},
		{
			name: "no spec.domain",
			system: &System{
				Kind:     "System",
				Metadata: &Metadata{Name: "test"},
				Spec: &SystemSpec{
					Owner: "foo",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
			wantErr: `domain "" for system test is undefined`,
		},
		{
			name: "invalid spec.domain",
			system: &System{
				Kind:     "System",
				Metadata: &Metadata{Name: "test"},
				Spec: &SystemSpec{
					Owner:  "foo",
					Domain: "bar",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
			wantErr: `domain "bar" for system test is undefined`,
		},
		{
			name: "valid",
			system: &System{
				Kind:     "System",
				Metadata: &Metadata{Name: "test"},
				Spec: &SystemSpec{
					Owner:  "foo",
					Domain: "bar",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
			domain: &Domain{
				Kind:     "Domain",
				Metadata: &Metadata{Name: "bar"},
				Spec: &DomainSpec{
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
		api     *API
		owner   *Group
		system  *System
		domain  *Domain
		wantErr string
	}{
		{
			name: "no metadata",
			api: &API{
				Kind: "API",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			api: &API{
				Kind:     "API",
				Metadata: &Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.type",
			api: &API{
				Kind:     "API",
				Metadata: &Metadata{Name: "test"},
				Spec:     &APISpec{},
			},
			wantErr: "has no spec.type",
		},
		{
			name: "no spec.lifecycle",
			api: &API{
				Kind:     "API",
				Metadata: &Metadata{Name: "test"},
				Spec: &APISpec{
					Type: "openapi",
				},
			},
			wantErr: "has no spec.lifecycle",
		},
		{
			name: "no spec.owner",
			api: &API{
				Kind:     "API",
				Metadata: &Metadata{Name: "test"},
				Spec: &APISpec{
					Type:      "openapi",
					Lifecycle: "production",
				},
			},
			wantErr: `owner "" for API test is undefined`,
		},
		{
			name: "invalid spec.owner",
			api: &API{
				Kind:     "API",
				Metadata: &Metadata{Name: "test"},
				Spec: &APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     "foo",
				},
			},
			wantErr: `owner "foo" for API test is undefined`,
		},
		{
			name: "invalid spec.system",
			api: &API{
				Kind:     "API",
				Metadata: &Metadata{Name: "test"},
				Spec: &APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     "foo",
					System:    "bar",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
			wantErr: `system "bar" for API test is undefined`,
		},
		{
			name: "valid",
			api: &API{
				Kind:     "API",
				Metadata: &Metadata{Name: "test"},
				Spec: &APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     "foo",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
		},
		{
			name: "valid with system",
			api: &API{
				Kind:     "API",
				Metadata: &Metadata{Name: "test"},
				Spec: &APISpec{
					Type:      "openapi",
					Lifecycle: "production",
					Owner:     "foo",
					System:    "bar",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
			system: &System{
				Kind:     "System",
				Metadata: &Metadata{Name: "bar"},
				Spec: &SystemSpec{
					Owner:  "foo",
					Domain: "test-domain",
				},
			},
			domain: &Domain{
				Kind:     "Domain",
				Metadata: &Metadata{Name: "test-domain"},
				Spec: &DomainSpec{
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
		resource *Resource
		owner    *Group
		system   *System
		domain   *Domain
		wantErr  string
	}{
		{
			name: "no metadata",
			resource: &Resource{
				Kind: "Resource",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			resource: &Resource{
				Kind:     "Resource",
				Metadata: &Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.type",
			resource: &Resource{
				Kind:     "Resource",
				Metadata: &Metadata{Name: "test"},
				Spec:     &ResourceSpec{},
			},
			wantErr: "has no spec.type",
		},
		{
			name: "no spec.owner",
			resource: &Resource{
				Kind:     "Resource",
				Metadata: &Metadata{Name: "test"},
				Spec: &ResourceSpec{
					Type: "database",
				},
			},
			wantErr: `owner "" for resource test is undefined`,
		},
		{
			name: "invalid spec.owner",
			resource: &Resource{
				Kind:     "Resource",
				Metadata: &Metadata{Name: "test"},
				Spec: &ResourceSpec{
					Type:  "database",
					Owner: "foo",
				},
			},
			wantErr: `owner "foo" for resource test is undefined`,
		},
		{
			name: "invalid spec.system",
			resource: &Resource{
				Kind:     "Resource",
				Metadata: &Metadata{Name: "test"},
				Spec: &ResourceSpec{
					Type:   "database",
					Owner:  "foo",
					System: "bar",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
			wantErr: `system "bar" for resource test is undefined`,
		},
		{
			name: "valid",
			resource: &Resource{
				Kind:     "Resource",
				Metadata: &Metadata{Name: "test"},
				Spec: &ResourceSpec{
					Type:  "database",
					Owner: "foo",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
		},
		{
			name: "valid with system",
			resource: &Resource{
				Kind:     "Resource",
				Metadata: &Metadata{Name: "test"},
				Spec: &ResourceSpec{
					Type:   "database",
					Owner:  "foo",
					System: "bar",
				},
			},
			owner: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "foo"},
				Spec:     &GroupSpec{Type: "team"},
			},
			system: &System{
				Kind:     "System",
				Metadata: &Metadata{Name: "bar"},
				Spec: &SystemSpec{
					Owner:  "foo",
					Domain: "test-domain",
				},
			},
			domain: &Domain{
				Kind:     "Domain",
				Metadata: &Metadata{Name: "test-domain"},
				Spec: &DomainSpec{
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
		group   *Group
		wantErr string
	}{
		{
			name: "no metadata",
			group: &Group{
				Kind: "Group",
			},
			wantErr: "metadata is null",
		},
		{
			name: "no spec",
			group: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "test"},
			},
			wantErr: "has no spec",
		},
		{
			name: "no spec.type",
			group: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "test"},
				Spec:     &GroupSpec{},
			},
			wantErr: "has no spec.type",
		},
		{
			name: "valid",
			group: &Group{
				Kind:     "Group",
				Metadata: &Metadata{Name: "test"},
				Spec:     &GroupSpec{Type: "team"},
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
