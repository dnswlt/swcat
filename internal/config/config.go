package config

import (
	"bytes"
	"fmt"
	"html/template"
	"path"
	"reflect"
	"strings"

	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/svg"
	"gopkg.in/yaml.v3"
)

// CustomContent describes one <details> section on an entity detail page,
// rendered from an annotation value or a status observation. Sections are
// bound to an annotation/observation key by the map they live in
// (UIConfig.AnnotationBasedContent / StatusBasedContent).
//
// If neither Template nor TemplateFile is set, the value is rendered as
// pretty-printed JSON in a read-only viewer. Otherwise, the template is
// executed against the parsed JSON (or the raw string, if the value isn't
// valid JSON) and its output is inserted as HTML into a
// <div class="custom-content"> wrapper.
type CustomContent struct {
	// Heading appears in the <summary> of the section.
	Heading string `yaml:"heading"`
	// Open: if true, the section is expanded on load.
	Open bool `yaml:"open"`
	// Rank orders multiple sections on the same page (lower first).
	Rank int `yaml:"rank"`
	// Template is an optional Go html/template. The template's . is the
	// JSON-decoded annotation/observation value (any), or the raw string
	// when the value isn't valid JSON. Output should be semantic HTML.
	// Mutually exclusive with TemplateFile.
	Template string `yaml:"template"`
	// TemplateFile is a path to a file (relative to the config file's
	// directory) whose contents are used as the Template. Useful for
	// keeping larger HTML templates out of the YAML config.
	// Mutually exclusive with Template.
	TemplateFile string `yaml:"templateFile"`

	tmpl *template.Template // parsed template; nil if no template configured
}

// Tmpl returns the parsed template (nil if no Template was configured).
func (c *CustomContent) Tmpl() *template.Template { return c.tmpl }

// NewCustomContent returns a CustomContent with its Template pre-parsed.
// Useful for programmatic construction (e.g. in tests); YAML-loaded
// CustomContents are populated by Load.
func NewCustomContent(template string) (*CustomContent, error) {
	c := &CustomContent{Template: template}
	if template == "" {
		return c, nil
	}
	t, err := parseCustomContentTemplate("custom-content", template)
	if err != nil {
		return nil, err
	}
	c.tmpl = t
	return c, nil
}

// HelpLink is a custom link shown in the footer.
type HelpLink struct {
	Title string `yaml:"title"`
	URL   string `yaml:"url"`
}

// UIConfig has configuration that only affects the UI.
// We cannot put it into the web package as that would generate
// a cyclic dependency.
type UIConfig struct {
	AnnotationBasedContent map[string]*CustomContent `yaml:"annotationBasedContent"`
	StatusBasedContent     map[string]*CustomContent `yaml:"statusBasedContent"`
	// An optional custom help link shown at the bottom of the UI.
	// DEPRECATED: Use HelpLinks instead.
	HelpLink *HelpLink `yaml:"helpLink"`
	// An optional list of custom help links shown at the bottom of the UI.
	HelpLinks []HelpLink `yaml:"helpLinks"`
}

// Bundle is the umbrella struct for the serialized application configuration YAML.
// It bundles the package-specific configurations.
type Bundle struct {
	SVG     svg.Config  `yaml:"svg"`
	Catalog repo.Config `yaml:"catalog"`
	UI      UIConfig    `yaml:"ui"`
}

// customContentFuncs are the template helpers exposed to user-defined
// CustomContent templates.
var customContentFuncs = template.FuncMap{
	"join": join,
}

// join formats a slice/array as a comma-separated string. Strings are used
// as-is; other values fall back to fmt.Sprint.
func join(v any) string {
	if xs, ok := v.([]string); ok {
		return strings.Join(xs, ", ")
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return fmt.Sprint(v)
	}
	var b strings.Builder
	for i := 0; i < rv.Len(); i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprint(&b, rv.Index(i).Interface())
	}
	return b.String()
}

func parseCustomContentTemplate(name, src string) (*template.Template, error) {
	return template.New(name).Funcs(customContentFuncs).Parse(src)
}

func Load(st store.Store, configPath string) (*Bundle, error) {
	bs, err := st.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not read config %q: %w", configPath, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(bs))
	dec.KnownFields(true)
	var bundle Bundle
	if err := dec.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("invalid configuration YAML in %q: %w", configPath, err)
	}

	// Resolve and pre-parse user templates. TemplateFile paths are
	// relative to the directory containing the config file.
	configDir := path.Dir(configPath)
	for k, c := range bundle.UI.AnnotationBasedContent {
		if err := loadCustomContentTemplate(st, configDir, k, c); err != nil {
			return nil, fmt.Errorf("annotationBasedContent %q: %v", k, err)
		}
	}
	for k, c := range bundle.UI.StatusBasedContent {
		if err := loadCustomContentTemplate(st, configDir, k, c); err != nil {
			return nil, fmt.Errorf("statusBasedContent %q: %v", k, err)
		}
	}

	return &bundle, nil
}

// loadCustomContentTemplate validates the template/templateFile combination,
// reads the templateFile from the store if needed, and parses the resulting
// template. The template is stored on c in-place.
func loadCustomContentTemplate(st store.Store, configDir, name string, c *CustomContent) error {
	if c.Template != "" && c.TemplateFile != "" {
		return fmt.Errorf("template and templateFile are mutually exclusive")
	}
	if c.TemplateFile != "" {
		bs, err := st.ReadFile(path.Join(configDir, c.TemplateFile))
		if err != nil {
			return fmt.Errorf("could not read templateFile %q: %v", c.TemplateFile, err)
		}
		c.Template = string(bs)
	}
	if c.Template == "" {
		return nil
	}
	t, err := parseCustomContentTemplate(name, c.Template)
	if err != nil {
		return fmt.Errorf("invalid template: %v", err)
	}
	c.tmpl = t
	return nil
}
