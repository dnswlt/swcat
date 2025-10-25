package repo

import (
	"fmt"
	"slices"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

func TestRepository_AddAndGet(t *testing.T) {
	repo := NewRepository()

	tests := []struct {
		entity catalog.Entity
	}{
		{
			entity: &catalog.Component{Metadata: &catalog.Metadata{Name: "c1"}},
		},
		{
			entity: &catalog.System{Metadata: &catalog.Metadata{Name: "s1"}},
		},
		{
			entity: &catalog.Domain{Metadata: &catalog.Metadata{Name: "d1"}},
		},
		{
			entity: &catalog.API{Metadata: &catalog.Metadata{Name: "a1"}},
		},
		{
			entity: &catalog.Resource{Metadata: &catalog.Metadata{Name: "r1"}},
		},
		{
			entity: &catalog.Group{Metadata: &catalog.Metadata{Name: "g1"}},
		},
	}

	for _, tt := range tests {
		err := repo.AddEntity(tt.entity)
		if err != nil {
			t.Fatalf("AddEntity() with %s error = %v", tt.entity.GetKind(), err)
		}
	}

	if repo.Size() != len(tests) {
		t.Errorf("repo.Size() = %d, want %d", repo.Size(), len(tests))
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("get %s", tt.entity.GetKind()), func(t *testing.T) {
			var e catalog.Entity
			switch tt.entity.(type) {
			case *catalog.Component:
				e = repo.Component(tt.entity.GetRef())
			case *catalog.System:
				e = repo.System(tt.entity.GetRef())
			case *catalog.Domain:
				e = repo.Domain(tt.entity.GetRef())
			case *catalog.API:
				e = repo.API(tt.entity.GetRef())
			case *catalog.Resource:
				e = repo.Resource(tt.entity.GetRef())
			case *catalog.Group:
				e = repo.Group(tt.entity.GetRef())
			default:
				t.Fatalf("unknown typeName: %s", tt.entity.GetKind())
			}

			if e == nil {
				t.Fatalf("%s() returned nil", tt.entity.GetKind())
			}
			if !e.GetRef().Equal(tt.entity.GetRef()) {
				t.Errorf("e.GetRef().String() = %v, want %v", tt.entity.GetRef(), e.GetRef())
			}
		})
	}

	t.Run("add duplicate", func(t *testing.T) {
		err := repo.AddEntity(&catalog.Component{Metadata: &catalog.Metadata{Name: "c1"}})
		if err == nil {
			t.Error("AddEntity() error = nil, want error")
		}
	})
}

func TestRepository_Entity(t *testing.T) {
	repo := NewRepository()

	entities := []catalog.Entity{
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c1"}},
		&catalog.System{Metadata: &catalog.Metadata{Name: "s1"}},
		&catalog.Domain{Metadata: &catalog.Metadata{Name: "d1"}},
		&catalog.API{Metadata: &catalog.Metadata{Name: "a1"}},
		&catalog.Resource{Metadata: &catalog.Metadata{Name: "r1"}},
		&catalog.Group{Metadata: &catalog.Metadata{Name: "g1"}},
	}

	for _, e := range entities {
		repo.AddEntity(e)
	}

	for _, entity := range entities {
		t.Run(entity.GetMetadata().Name, func(t *testing.T) {
			e := repo.Entity(entity.GetRef())
			if e == nil {
				t.Fatal("Entity() returned nil")
			}
			if e.GetRef().String() != entity.GetRef().String() {
				t.Errorf("Entity().GetRef().String() = %s, want %s", e.GetRef().String(), entity.GetRef().String())
			}
		})
	}

	t.Run("non-existing ref", func(t *testing.T) {
		e := repo.Entity(&catalog.Ref{Kind: "component", Name: "s1"})
		if e != nil {
			t.Error("Entity() returned non-nil for non-existing ref")
		}
	})

	t.Run("invalid ref", func(t *testing.T) {
		e := repo.Entity(&catalog.Ref{Kind: "invalid", Name: "ref"})
		if e != nil {
			t.Error("Entity() returned non-nil for invalid ref")
		}
	})

	t.Run("ref without kind", func(t *testing.T) {
		e := repo.Entity(&catalog.Ref{Name: "c1"})
		if e != nil {
			t.Error("Entity() returned non-nil for ref without kind")
		}
	})
}

func TestRepository_Finders(t *testing.T) {
	repo := NewRepository()

	entities := []catalog.Entity{
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c2", Namespace: "ns1"}, Spec: &catalog.ComponentSpec{}}, // Add in different order
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c1", Namespace: "ns1"}, Spec: &catalog.ComponentSpec{}},
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c3", Namespace: "ns2"}, Spec: &catalog.ComponentSpec{}},
		&catalog.Component{Metadata: &catalog.Metadata{Name: "c4", Namespace: "ns3"}, Spec: &catalog.ComponentSpec{
			Owner: &catalog.Ref{Name: "o4"}, Lifecycle: "production",
		}},
		&catalog.System{Metadata: &catalog.Metadata{Name: "s2"}, Spec: &catalog.SystemSpec{}},
		&catalog.System{Metadata: &catalog.Metadata{Name: "s1"}, Spec: &catalog.SystemSpec{}},
		&catalog.Domain{Metadata: &catalog.Metadata{Name: "d1"}, Spec: &catalog.DomainSpec{}},
		&catalog.API{Metadata: &catalog.Metadata{Name: "a1"}, Spec: &catalog.APISpec{}},
		&catalog.Resource{Metadata: &catalog.Metadata{Name: "r1"}, Spec: &catalog.ResourceSpec{}},
		&catalog.Group{Metadata: &catalog.Metadata{Name: "g2"}, Spec: &catalog.GroupSpec{}},
		&catalog.Group{Metadata: &catalog.Metadata{Name: "g1"}, Spec: &catalog.GroupSpec{}},
	}

	for _, e := range entities {
		repo.AddEntity(e)
	}

	type finderTest struct {
		query     string
		wantNames []string
	}

	testFinder := func(t *testing.T, finder func(string) []catalog.Entity, tests []finderTest) {
		for _, tt := range tests {
			t.Run(tt.query, func(t *testing.T) {
				results := finder(tt.query)
				if len(results) != len(tt.wantNames) {
					t.Errorf("len(results) = %d, want %d", len(results), len(tt.wantNames))
				}

				var gotNames []string
				for _, r := range results {
					gotNames = append(gotNames, r.GetRef().QName())
				}

				if !slices.Equal(gotNames, tt.wantNames) {
					t.Errorf("results = %v, want %v", gotNames, tt.wantNames)
				}
			})
		}
	}

	t.Run("FindComponents", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindComponents(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"ns1", []string{"ns1/c1", "ns1/c2"}},
			{"namespace:ns1 AND name:c1", []string{"ns1/c1"}},
			{"c1", []string{"ns1/c1"}},
			{"c3", []string{"ns2/c3"}},
			{"owner:o4 OR lifecycle:production", []string{"ns3/c4"}},
			{"notfound", nil},
			{"", []string{"ns1/c1", "ns1/c2", "ns2/c3", "ns3/c4"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindSystems", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindSystems(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"s", []string{"s1", "s2"}},
			{"s1", []string{"s1"}},
			{"", []string{"s1", "s2"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindDomains", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindDomains(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"d", []string{"d1"}},
			{"", []string{"d1"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindAPIs", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindAPIs(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"a", []string{"a1"}},
			{"", []string{"a1"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindResources", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindResources(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"r", []string{"r1"}},
			{"", []string{"r1"}},
		}
		testFinder(t, finder, tests)
	})

	t.Run("FindGroups", func(t *testing.T) {
		finder := func(q string) []catalog.Entity {
			var entities []catalog.Entity
			for _, e := range repo.FindGroups(q) {
				entities = append(entities, e)
			}
			return entities
		}
		tests := []finderTest{
			{"g", []string{"g1", "g2"}},
			{"g1", []string{"g1"}},
			{"", []string{"g1", "g2"}},
		}
		testFinder(t, finder, tests)
	})
}

func TestPrepareLinkTemplates(t *testing.T) {
	tests := []struct {
		name    string
		link    *AnnotationBasedLink
		wantErr bool
	}{
		{
			name: "no annotations",
			link: &AnnotationBasedLink{
				URL:   "foo",
				Title: "bar",
			},
			wantErr: false,
		},
		{
			name: "valid template",
			link: &AnnotationBasedLink{
				URL: "https://example.com/{{ .Metadata.Name }}/{{ .Annotation.Value }}",
			},
			wantErr: false,
		},
		{
			name: "empty url",
			link: &AnnotationBasedLink{
				URL:   "",
				Title: "Yankee",
			},
			wantErr: true,
		},
		{
			name: "invalid template",
			link: &AnnotationBasedLink{
				URL: "https://example.com/{{ .Metadata.Name",
			},
			wantErr: true,
		},
		{
			name: "unknown function",
			link: &AnnotationBasedLink{
				URL:   "https://example.com/{{ .Metadata.Name }}",
				Title: "Super {{ .Metadata.Name | tocamelcase }}",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewRepository()
			repo.config.AnnotationBasedLinks = map[string]*AnnotationBasedLink{
				"test": tt.link,
			}
			tmpls, err := repo.prepareLinkTemplates()
			if (err != nil) != tt.wantErr {
				t.Fatalf("prepareLinkTemplates() error: %v, wantErr: %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if l := len(tmpls); l != 1 {
				t.Errorf("Wrong number of templates: want 1, got %d", l)
			}
			tmpl, ok := tmpls["test"]
			if !ok {
				t.Fatal("Expected template with key 'test' was not prepared")
			}
			if tmpl.url == nil {
				t.Errorf("url template is nil")
			}
			if tmpl.title == nil {
				t.Errorf("title template is nil")
			}
		})
	}
}

func TestAddGeneratedLinks_Component(t *testing.T) {
	repo := NewRepositoryWithConfig(Config{
		AnnotationBasedLinks: map[string]*AnnotationBasedLink{
			"example.com/foobar": {
				URL:   "https://example.com/{{ .Annotation.Value }}",
				Title: "FooBar for {{ .Metadata.Name }}",
				Type:  "dashboard",
				Icon:  "dashboard-icon",
			},
		},
	})
	c := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "my-component",
			Annotations: map[string]string{
				"example.com/foobar": "abc-123",
			},
		},
	}
	repo.AddEntity(c)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	if len(c.Metadata.Links) != 1 {
		t.Fatalf("len(links) = %d, want 1", len(c.Metadata.Links))
	}
	link := c.Metadata.Links[0]
	if !link.IsGenerated {
		t.Error("link.IsGenerated = false, want true")
	}
	if link.URL != "https://example.com/abc-123" {
		t.Errorf("link.URL = %q, want %q", link.URL, "https://example.com/abc-123")
	}
	if link.Title != "FooBar for my-component" {
		t.Errorf("link.Title = %q, want %q", link.Title, "FooBar for my-component")
	}
	if link.Type != "dashboard" {
		t.Errorf("link.Type = %q, want %q", link.Type, "dashboard")
	}
	if link.Icon != "dashboard-icon" {
		t.Errorf("link.Icon = %q, want %q", link.Icon, "dashboard")
	}

}
func TestAddGeneratedLinks_VersionedAPI(t *testing.T) {

	repo := NewRepositoryWithConfig(Config{
		AnnotationBasedLinks: map[string]*AnnotationBasedLink{
			"example.com/docs": {
				URL:   "https://example.com/docs/{{ .Annotation.Value }}/{{ .Version.RawVersion }}",
				Title: "Docs for {{ .Metadata.Name }} ({{ .Version.RawVersion }})",
			},
		},
	})
	api := &catalog.API{
		Metadata: &catalog.Metadata{
			Name: "my-api",
			Annotations: map[string]string{
				"example.com/docs": "my-api-docs",
			},
		},
		Spec: &catalog.APISpec{
			Versions: []*catalog.APISpecVersion{
				{Version: catalog.Version{RawVersion: "v1"}},
				{Version: catalog.Version{RawVersion: "v2.1"}},
			},
		},
	}
	repo.AddEntity(api)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	if len(api.Metadata.Links) != 2 {
		t.Fatalf("len(links) = %d, want 2", len(api.Metadata.Links))
	}

	// Links are sorted by title
	link1 := api.Metadata.Links[0]
	if link1.URL != "https://example.com/docs/my-api-docs/v1" {
		t.Errorf("link1.URL = %q, want %q", link1.URL, "https://example.com/docs/my-api-docs/v1")
	}
	if link1.Title != "Docs for my-api (v1)" {
		t.Errorf("link1.Title = %q, want %q", link1.Title, "Docs for my-api (v1)")
	}

	link2 := api.Metadata.Links[1]
	if link2.URL != "https://example.com/docs/my-api-docs/v2.1" {
		t.Errorf("link2.URL = %q, want %q", link2.URL, "https://example.com/docs/my-api-docs/v2.1")
	}
	if link2.Title != "Docs for my-api (v2.1)" {
		t.Errorf("link2.Title = %q, want %q", link2.Title, "Docs for my-api (v2.1)")
	}

}
func TestAddGeneratedLinks_MixedEntities(t *testing.T) {

	repo := NewRepositoryWithConfig(Config{
		AnnotationBasedLinks: map[string]*AnnotationBasedLink{
			"example.com/foobar": {
				URL:   "https://example.com/{{ .Annotation.Value }}",
				Title: "FooBar for {{ .Metadata.Name }}",
			},
		},
	})
	c1 := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "component-with-annotation",
			Annotations: map[string]string{
				"example.com/foobar": "abc-123",
			},
		},
	}
	c2 := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "component-without-annotation",
		},
	}
	repo.AddEntity(c1)
	repo.AddEntity(c2)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	if len(c1.Metadata.Links) != 1 {
		t.Errorf("len(c1.links) = %d, want 1", len(c1.Metadata.Links))
	}
	if len(c2.Metadata.Links) != 0 {
		t.Errorf("len(c2.links) = %d, want 0", len(c2.Metadata.Links))
	}
}
