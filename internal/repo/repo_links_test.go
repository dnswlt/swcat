package repo

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

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
				t.Errorf("Wrong number of generators: want 1, got %d", l)
			}
			gen := tmpls[0]
			if gen.annotation != "test" {
				t.Fatalf("Expected generator with annotation 'test', got %q", gen.annotation)
			}
			if gen.url == nil {
				t.Errorf("url template is nil")
			}
			if gen.title == nil {
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

func TestAddGeneratedLinks_AutomaticLinks(t *testing.T) {
	repo := NewRepositoryWithConfig(Config{
		AutomaticLinks: []*AutomaticLink{
			{
				Filter: "kind=component AND type=service",
				URL:    "https://grafana.example.com/{{ .Metadata.Name }}",
				Title:  "Monitoring",
			},
		},
	})
	c1 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "service-1"},
		Spec:     &catalog.ComponentSpec{Type: "service"},
	}
	c2 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "library-1"},
		Spec:     &catalog.ComponentSpec{Type: "library"},
	}
	repo.AddEntity(c1)
	repo.AddEntity(c2)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	if len(c1.Metadata.Links) != 1 {
		t.Errorf("len(c1.links) = %d, want 1", len(c1.Metadata.Links))
	} else {
		link := c1.Metadata.Links[0]
		if link.URL != "https://grafana.example.com/service-1" {
			t.Errorf("link.URL = %q, want %q", link.URL, "https://grafana.example.com/service-1")
		}
	}

	if len(c2.Metadata.Links) != 0 {
		t.Errorf("len(c2.links) = %d, want 0", len(c2.Metadata.Links))
	}
}

func TestAddGeneratedLinks_AutomaticLinks_FirstFunc(t *testing.T) {
	repo := NewRepositoryWithConfig(Config{
		AutomaticLinks: []*AutomaticLink{
			{
				Filter: "kind=component",
				URL:    `https://grafana.example.com/{{ first (index .Metadata.Annotations "hexz.me/monitoring") .Metadata.Name }}`,
				Title:  "Monitoring",
			},
		},
	})
	c1 := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "service-1",
			Annotations: map[string]string{
				"hexz.me/monitoring": "dashboard-abc",
			},
		},
		Spec: &catalog.ComponentSpec{},
	}
	c2 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "service-2"},
		Spec:     &catalog.ComponentSpec{},
	}
	repo.AddEntity(c1)
	repo.AddEntity(c2)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	link1 := c1.Metadata.Links[0]
	if link1.URL != "https://grafana.example.com/dashboard-abc" {
		t.Errorf("link1.URL = %q, want %q", link1.URL, "https://grafana.example.com/dashboard-abc")
	}

	link2 := c2.Metadata.Links[0]
	if link2.URL != "https://grafana.example.com/service-2" {
		t.Errorf("link2.URL = %q, want %q", link2.URL, "https://grafana.example.com/service-2")
	}
}

func TestAddGeneratedLinks_AnnotationBasedMultiLinks(t *testing.T) {
	repo := NewRepositoryWithConfig(Config{
		AnnotationBasedLinks: map[string]*AnnotationBasedLink{
			"example.com/app-name": {
				URL:   "https://{{ .MultiLink.Value }}.example.com/apps/{{ .Annotation.Value }}",
				Title: "Monitoring",
				Icon:  "dashboard",
				MultiLinks: []MultiLinkEntry{
					{Label: "dev", Value: "dev"},
					{Label: "staging", Value: "staging"},
					{Label: "prod", Value: "prod"},
				},
			},
		},
	})
	c := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "my-service",
			Annotations: map[string]string{
				"example.com/app-name": "my-service",
			},
		},
		Spec: &catalog.ComponentSpec{},
	}
	repo.AddEntity(c)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	if len(c.Metadata.Links) != 3 {
		t.Fatalf("len(links) = %d, want 3", len(c.Metadata.Links))
	}

	// Links are sorted by title; all share the same group title so the sort
	// falls through to URL: dev < prod < staging alphabetically.
	want := []struct {
		name  string
		url   string
		title string
	}{
		{"dev", "https://dev.example.com/apps/my-service", "Monitoring (dev)"},
		{"prod", "https://prod.example.com/apps/my-service", "Monitoring (prod)"},
		{"staging", "https://staging.example.com/apps/my-service", "Monitoring (staging)"},
	}
	for i, w := range want {
		l := c.Metadata.Links[i]
		if !l.IsGenerated {
			t.Errorf("links[%d].IsGenerated = false, want true", i)
		}
		if l.URL != w.url {
			t.Errorf("links[%d].URL = %q, want %q", i, l.URL, w.url)
		}
		if l.Title != w.title {
			t.Errorf("links[%d].Title = %q, want %q", i, l.Title, w.title)
		}
		if l.Icon != "dashboard" {
			t.Errorf("links[%d].Icon = %q, want %q", i, l.Icon, "dashboard")
		}
		if l.GroupInfo == nil {
			t.Fatalf("links[%d].GroupInfo is nil", i)
		}
		if l.GroupInfo.Group != "Monitoring" {
			t.Errorf("links[%d].GroupInfo.Group = %q, want %q", i, l.GroupInfo.Group, "Monitoring")
		}
		if l.GroupInfo.Label != w.name {
			t.Errorf("links[%d].GroupInfo.Name = %q, want %q", i, l.GroupInfo.Label, w.name)
		}
	}
}

func TestAddGeneratedLinks_AutomaticMultiLinks(t *testing.T) {
	repo := NewRepositoryWithConfig(Config{
		AutomaticLinks: []*AutomaticLink{
			{
				Filter: "kind=component",
				URL:    "https://{{ .MultiLink.Value }}.example.com/apps/{{ .Metadata.Name }}",
				Title:  "Monitoring",
				MultiLinks: []MultiLinkEntry{
					{Label: "dev", Value: "dev"},
					{Label: "prod", Value: "prod"},
				},
			},
		},
	})
	c := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "my-service"},
		Spec:     &catalog.ComponentSpec{},
	}
	repo.AddEntity(c)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	if len(c.Metadata.Links) != 2 {
		t.Fatalf("len(links) = %d, want 2", len(c.Metadata.Links))
	}

	// Sorted by title then URL: "Monitoring (dev)" < "Monitoring (prod)"
	want := []struct {
		name  string
		url   string
		title string
	}{
		{"dev", "https://dev.example.com/apps/my-service", "Monitoring (dev)"},
		{"prod", "https://prod.example.com/apps/my-service", "Monitoring (prod)"},
	}
	for i, w := range want {
		l := c.Metadata.Links[i]
		if !l.IsGenerated {
			t.Errorf("links[%d].IsGenerated = false, want true", i)
		}
		if l.URL != w.url {
			t.Errorf("links[%d].URL = %q, want %q", i, l.URL, w.url)
		}
		if l.Title != w.title {
			t.Errorf("links[%d].Title = %q, want %q", i, l.Title, w.title)
		}
		if l.GroupInfo == nil {
			t.Fatalf("links[%d].GroupInfo is nil", i)
		}
		if l.GroupInfo.Group != "Monitoring" {
			t.Errorf("links[%d].GroupInfo.Group = %q, want %q", i, l.GroupInfo.Group, "Monitoring")
		}
		if l.GroupInfo.Label != w.name {
			t.Errorf("links[%d].GroupInfo.Name = %q, want %q", i, l.GroupInfo.Label, w.name)
		}
	}
}

func TestAddGeneratedLinks_VersionedMultiLinks(t *testing.T) {
	repo := NewRepositoryWithConfig(Config{
		AnnotationBasedLinks: map[string]*AnnotationBasedLink{
			"example.com/app-name": {
				URL:   "https://{{ .MultiLink.Value }}.example.com/{{ .Annotation.Value }}/{{ .Version.RawVersion }}",
				Title: "Docs ({{ .Version.RawVersion }})",
				MultiLinks: []MultiLinkEntry{
					{Label: "dev", Value: "dev"},
					{Label: "prod", Value: "prod"},
				},
			},
		},
	})
	api := &catalog.API{
		Metadata: &catalog.Metadata{
			Name: "my-api",
			Annotations: map[string]string{
				"example.com/app-name": "my-api",
			},
		},
		Spec: &catalog.APISpec{
			Versions: []*catalog.APISpecVersion{
				{Version: catalog.Version{RawVersion: "v1"}},
				{Version: catalog.Version{RawVersion: "v2"}},
			},
		},
	}
	repo.AddEntity(api)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	// 2 versions × 2 multiLinks = 4 links, sorted by title then URL.
	// Titles: "Docs (v1) (dev)", "Docs (v1) (prod)", "Docs (v2) (dev)", "Docs (v2) (prod)"
	if len(api.Metadata.Links) != 4 {
		t.Fatalf("len(links) = %d, want 4", len(api.Metadata.Links))
	}
	want := []struct {
		title string
		url   string
		group string
		label string
	}{
		{"Docs (v1) (dev)", "https://dev.example.com/my-api/v1", "Docs (v1)", "dev"},
		{"Docs (v1) (prod)", "https://prod.example.com/my-api/v1", "Docs (v1)", "prod"},
		{"Docs (v2) (dev)", "https://dev.example.com/my-api/v2", "Docs (v2)", "dev"},
		{"Docs (v2) (prod)", "https://prod.example.com/my-api/v2", "Docs (v2)", "prod"},
	}
	for i, w := range want {
		l := api.Metadata.Links[i]
		if l.Title != w.title {
			t.Errorf("links[%d].Title = %q, want %q", i, l.Title, w.title)
		}
		if l.URL != w.url {
			t.Errorf("links[%d].URL = %q, want %q", i, l.URL, w.url)
		}
		if l.GroupInfo == nil {
			t.Fatalf("links[%d].GroupInfo is nil", i)
		}
		if l.GroupInfo.Group != w.group {
			t.Errorf("links[%d].GroupInfo.Group = %q, want %q", i, l.GroupInfo.Group, w.group)
		}
		if l.GroupInfo.Label != w.label {
			t.Errorf("links[%d].GroupInfo.Label = %q, want %q", i, l.GroupInfo.Label, w.label)
		}
	}
}

func TestAddGeneratedLinks_AnnotationBasedMultiLinkData(t *testing.T) {
	repo := NewRepositoryWithConfig(Config{
		AnnotationBasedLinks: map[string]*AnnotationBasedLink{
			"example.com/logging": {
				URL:           "https://logs.example.com/{{ .MultiLink.Value }}/{{ .Metadata.Name }}",
				Title:         "Logs",
				Icon:          "logs",
				MultiLinkData: "environments",
			},
		},
	})
	c := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "my-service",
			Annotations: map[string]string{
				"example.com/logging": "true",
				"swcat/data-environments": `[
					{"label": "dev", "value": "development"},
					{"label": "prod", "value": "production"}
				]`,
			},
		},
		Spec: &catalog.ComponentSpec{},
	}
	repo.AddEntity(c)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	if len(c.Metadata.Links) != 2 {
		t.Fatalf("len(links) = %d, want 2", len(c.Metadata.Links))
	}

	// Sorted by title then URL: "Logs (dev)" < "Logs (prod)"
	want := []struct {
		label string
		url   string
		title string
	}{
		{"dev", "https://logs.example.com/development/my-service", "Logs (dev)"},
		{"prod", "https://logs.example.com/production/my-service", "Logs (prod)"},
	}
	for i, w := range want {
		l := c.Metadata.Links[i]
		if !l.IsGenerated {
			t.Errorf("links[%d].IsGenerated = false, want true", i)
		}
		if l.URL != w.url {
			t.Errorf("links[%d].URL = %q, want %q", i, l.URL, w.url)
		}
		if l.Title != w.title {
			t.Errorf("links[%d].Title = %q, want %q", i, l.Title, w.title)
		}
		if l.Icon != "logs" {
			t.Errorf("links[%d].Icon = %q, want %q", i, l.Icon, "logs")
		}
		if l.GroupInfo == nil {
			t.Fatalf("links[%d].GroupInfo is nil", i)
		}
		if l.GroupInfo.Group != "Logs" {
			t.Errorf("links[%d].GroupInfo.Group = %q, want %q", i, l.GroupInfo.Group, "Logs")
		}
		if l.GroupInfo.Label != w.label {
			t.Errorf("links[%d].GroupInfo.Label = %q, want %q", i, l.GroupInfo.Label, w.label)
		}
	}
}

func TestAddGeneratedLinks_AutomaticMultiLinkData(t *testing.T) {
	// The multiLinkData annotation is on the parent system, not the component itself.
	// IAnnotation should traverse upward to find it.
	repo := NewRepositoryWithConfig(Config{
		AutomaticLinks: []*AutomaticLink{
			{
				Filter:        "kind=component",
				URL:           "https://logs.example.com/{{ .MultiLink.Value }}/{{ .Metadata.Name }}",
				Title:         "Logs",
				Icon:          "logs",
				MultiLinkData: "environments",
			},
		},
	})
	sys := &catalog.System{
		Metadata: &catalog.Metadata{
			Name: "my-system",
			Annotations: map[string]string{
				"swcat/data-environments": `[
					{"label": "dev", "value": "development"},
					{"label": "prod", "value": "production"}
				]`,
			},
		},
		Spec: &catalog.SystemSpec{},
	}
	c := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "my-service"},
		Spec: &catalog.ComponentSpec{
			System: &catalog.Ref{Kind: catalog.KindSystem, Name: "my-system"},
		},
	}
	repo.AddEntity(sys)
	repo.AddEntity(c)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	if len(c.Metadata.Links) != 2 {
		t.Fatalf("len(links) = %d, want 2", len(c.Metadata.Links))
	}

	// Sorted by title then URL: "Logs (dev)" < "Logs (prod)"
	want := []struct {
		label string
		url   string
		title string
	}{
		{"dev", "https://logs.example.com/development/my-service", "Logs (dev)"},
		{"prod", "https://logs.example.com/production/my-service", "Logs (prod)"},
	}
	for i, w := range want {
		l := c.Metadata.Links[i]
		if !l.IsGenerated {
			t.Errorf("links[%d].IsGenerated = false, want true", i)
		}
		if l.URL != w.url {
			t.Errorf("links[%d].URL = %q, want %q", i, l.URL, w.url)
		}
		if l.Title != w.title {
			t.Errorf("links[%d].Title = %q, want %q", i, l.Title, w.title)
		}
		if l.GroupInfo == nil {
			t.Fatalf("links[%d].GroupInfo is nil", i)
		}
		if l.GroupInfo.Group != "Logs" {
			t.Errorf("links[%d].GroupInfo.Group = %q, want %q", i, l.GroupInfo.Group, "Logs")
		}
		if l.GroupInfo.Label != w.label {
			t.Errorf("links[%d].GroupInfo.Label = %q, want %q", i, l.GroupInfo.Label, w.label)
		}
	}
}

func TestAddGeneratedLinks_AddQueryParams(t *testing.T) {
	repo := NewRepositoryWithConfig(Config{
		AnnotationBasedLinks: map[string]*AnnotationBasedLink{
			"example.com/link": {
				URL:   `{{ addQueryParams "https://example.com" (queryParams "q" .Annotation.Value "foo" "bar baz") }}`,
				Title: "Link",
			},
		},
	})
	c := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "my-service",
			Annotations: map[string]string{
				"example.com/link": "hello world",
			},
		},
		Spec: &catalog.ComponentSpec{},
	}
	repo.AddEntity(c)

	if err := repo.addGeneratedLinks(); err != nil {
		t.Fatalf("addGeneratedLinks() error = %v", err)
	}

	if len(c.Metadata.Links) != 1 {
		t.Fatalf("len(links) = %d, want 1", len(c.Metadata.Links))
	}

	link := c.Metadata.Links[0]
	// Expected URL should have query params correctly encoded.
	// Space in "hello world" -> "hello+world" or "%20" (url.Values.Encode uses "+")
	// Space in "bar baz" -> "bar+baz"
	wantURL := "https://example.com?foo=bar+baz&q=hello+world"
	if link.URL != wantURL {
		t.Errorf("link.URL = %q, want %q", link.URL, wantURL)
	}
}
