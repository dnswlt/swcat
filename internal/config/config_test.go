package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	catalogrepo "github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
)

// writeFiles writes the given path→content map under root, creating
// intermediate directories as needed.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for p, content := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

func TestLoad_TemplateFile(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"swcat.yml": `
ui:
  annotationBasedContent:
    my.org/data:
      heading: Data
      templateFile: custom/data.html
`,
		"custom/data.html": `<p>{{ .name }}</p>`,
	})

	st := store.NewDiskStore(root)
	b, err := Load(st, "swcat.yml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := b.UI.AnnotationBasedContent["my.org/data"]
	if c == nil {
		t.Fatalf("missing annotation entry")
	}
	if c.Tmpl() == nil {
		t.Fatalf("expected template to be parsed from templateFile")
	}
	if !strings.Contains(c.Template, "{{ .name }}") {
		t.Errorf("Template not populated from file: %q", c.Template)
	}
}

func TestLoad_TemplateFile_RelativeToConfigDir(t *testing.T) {
	// Config sits in a subdirectory; templateFile should resolve relative
	// to that subdirectory, not the store root.
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"conf/swcat.yml": `
ui:
  statusBasedContent:
    my.org/status:
      heading: Status
      templateFile: tmpl/status.html
`,
		"conf/tmpl/status.html": `<p>{{ . }}</p>`,
	})

	st := store.NewDiskStore(root)
	b, err := Load(st, "conf/swcat.yml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := b.UI.StatusBasedContent["my.org/status"]
	if c == nil || c.Tmpl() == nil {
		t.Fatalf("templateFile not loaded: %#v", c)
	}
}

func TestLoad_TemplateAndTemplateFile_MutuallyExclusive(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"swcat.yml": `
ui:
  annotationBasedContent:
    my.org/data:
      heading: Data
      template: "<p>inline</p>"
      templateFile: custom/data.html
`,
		"custom/data.html": `<p>file</p>`,
	})

	st := store.NewDiskStore(root)
	_, err := Load(st, "swcat.yml")
	if err == nil {
		t.Fatalf("expected error for both template and templateFile")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error %q does not mention mutual exclusion", err)
	}
}

func TestLoad_TemplateFile_Missing(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"swcat.yml": `
ui:
  annotationBasedContent:
    my.org/data:
      heading: Data
      templateFile: missing.html
`,
	})

	st := store.NewDiskStore(root)
	_, err := Load(st, "swcat.yml")
	if err == nil {
		t.Fatalf("expected error for missing templateFile")
	}
	if !strings.Contains(err.Error(), "templateFile") {
		t.Errorf("error %q does not reference templateFile", err)
	}
}

func TestLoad_TemplateFile_InvalidTemplate(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"swcat.yml": `
ui:
  annotationBasedContent:
    my.org/data:
      heading: Data
      templateFile: broken.html
`,
		"broken.html": `{{ .unterminated`,
	})

	st := store.NewDiskStore(root)
	_, err := Load(st, "swcat.yml")
	if err == nil {
		t.Fatalf("expected error for invalid template content")
	}
}

func TestLoad_StarlarkLinkFile_RelativeToConfigDir(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"conf/swcat.yml": `
catalog:
  starlarkLinks:
    - filter: kind=component
      file: links/components.star
`,
		"conf/links/components.star": `def links(entity): return []`,
	})

	st := store.NewDiskStore(root)
	b, err := Load(st, "conf/swcat.yml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(b.Catalog.StarlarkLinks) != 1 {
		t.Fatalf("len(StarlarkLinks) = %d, want 1", len(b.Catalog.StarlarkLinks))
	}
	if got := b.Catalog.StarlarkLinks[0].File; got != "links/components.star" {
		t.Fatalf("File = %q, want %q", got, "links/components.star")
	}
}

func TestLoad_StarlarkLinkFileErrors(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		files   map[string]string
		wantErr string
	}{
		{
			name: "missing file",
			config: `
catalog:
  starlarkLinks:
    - filter: kind=component
      file: missing.star
`,
			wantErr: `could not read file "missing.star"`,
		},
		{
			name: "syntax error",
			config: `
catalog:
  starlarkLinks:
    - filter: kind=component
      file: broken.star
`,
			files:   map[string]string{"broken.star": `def links(:`},
			wantErr: `compile "broken.star"`,
		},
		{
			name: "empty filter",
			config: `
catalog:
  starlarkLinks:
    - file: links.star
`,
			files:   map[string]string{"links.star": `def links(entity): return []`},
			wantErr: "empty filter",
		},
		{
			name: "empty file",
			config: `
catalog:
  starlarkLinks:
    - filter: kind=component
`,
			wantErr: "empty file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{"swcat.yml": tt.config}
			for path, contents := range tt.files {
				files[path] = contents
			}
			writeFiles(t, root, files)
			_, err := Load(store.NewDiskStore(root), "swcat.yml")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoad_StarlarkLinksEndToEnd(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"swcat.yml": `
catalog:
  starlarkLinks:
    - filter: kind=component
      file: links/components.star
`,
		"links/components.star": `
def links(entity):
    return [link(
        url="https://deployments.example.com/{name}".format(
            name=entity["metadata"]["name"],
        ),
        title="Deployment",
    )]
`,
		"catalog/catalog.yml": `
apiVersion: swcat/v1
kind: Group
metadata:
  name: team
spec:
  type: team
  profile:
    displayName: Team
---
apiVersion: swcat/v1
kind: Domain
metadata:
  name: payments
spec:
  type: business
  owner: team
---
apiVersion: swcat/v1
kind: System
metadata:
  name: checkout
spec:
  type: service
  owner: team
  domain: payments
---
apiVersion: swcat/v1
kind: Component
metadata:
  name: checkout-api
spec:
  type: service
  lifecycle: production
  owner: team
  system: checkout
`,
	})

	st := store.NewDiskStore(root)
	bundle, err := Load(st, "swcat.yml")
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}
	repository, err := catalogrepo.Load(st, nil, bundle.Catalog)
	if err != nil {
		t.Fatalf("Load repository: %v", err)
	}
	entity := repository.Entity(&catalog.Ref{
		Kind:      catalog.KindComponent,
		Namespace: "default",
		Name:      "checkout-api",
	})
	if entity == nil {
		t.Fatal("checkout-api component not found")
	}
	links := entity.GetMetadata().Links
	if len(links) != 1 || links[0].URL != "https://deployments.example.com/checkout-api" || !links[0].IsGenerated {
		t.Fatalf("generated links = %#v", links)
	}
}
