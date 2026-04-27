package config

import (
	"bytes"
	"fmt"
	"html/template"
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
// If Template is empty, the value is rendered as pretty-printed JSON in a
// read-only viewer. Otherwise, Template is executed against the parsed JSON
// (or the raw string, if the value isn't valid JSON) and its output is
// inserted as HTML into a <div class="custom-content"> wrapper.
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
	Template string `yaml:"template"`

	tmpl *template.Template // parsed Template; nil if Template is empty
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

	// Pre-parse user templates.
	for k, c := range bundle.UI.AnnotationBasedContent {
		if c.Template == "" {
			continue
		}
		t, err := parseCustomContentTemplate(k, c.Template)
		if err != nil {
			return nil, fmt.Errorf("invalid template for annotationBasedContent %q: %v", k, err)
		}
		c.tmpl = t
	}
	for k, c := range bundle.UI.StatusBasedContent {
		if c.Template == "" {
			continue
		}
		t, err := parseCustomContentTemplate(k, c.Template)
		if err != nil {
			return nil, fmt.Errorf("invalid template for statusBasedContent %q: %v", k, err)
		}
		c.tmpl = t
	}

	return &bundle, nil
}
