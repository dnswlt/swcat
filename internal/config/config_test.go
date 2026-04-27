package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
