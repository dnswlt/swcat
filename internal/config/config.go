package config

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"text/template"

	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/svg"
	"gopkg.in/yaml.v3"
)

type CustomColumn struct {
	Header string `yaml:"header"` // The header to appear in the custom table. If empty, no header row will be added.
	Data   string `yaml:"data"`   // A Go text/template to render the <td> content of the column.
	// Cached instance of the Go template for the Data field.
	dataTemplate *template.Template
}

func (c *CustomColumn) DataTemplate() *template.Template {
	return c.dataTemplate
}

// AnnotationBasedContent specifies how annotation-based content should be rendered in the UI.
type AnnotationBasedContent struct {
	Heading string          `yaml:"heading"` // The heading under which to display the content.
	Style   string          `yaml:"style"`   // The style in which to render the content. One of "text", "list", "json", "table", "custom".
	Rank    int             `yaml:"rank"`    // Used to order multiple content blocks on the same page.
	Columns []*CustomColumn `yaml:"columns"` // For style "custom", the columns to be rendered from the JSON list of objects.
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
	AnnotationBasedContent map[string]*AnnotationBasedContent `yaml:"annotationBasedContent"`
	// An optional custom help link shown at the bottom of the UI.
	HelpLink *HelpLink `yaml:"helpLink"`
}

// Bundle is the umbrella struct for the serialized application configuration YAML.
// It bundles the package-specific configurations.
type Bundle struct {
	SVG     svg.Config  `yaml:"svg"`
	Catalog repo.Config `yaml:"catalog"`
	UI      UIConfig    `yaml:"ui"`
}

// join is a helper function used in template funcs for CustomColumn data.
func join(v any) string {
	// Fast path for string slices
	if xs, ok := v.([]string); ok {
		return strings.Join(xs, ", ")
	}

	rv := reflect.ValueOf(v)

	// Handle non-slice/array types by falling back to default string formatting
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return fmt.Sprint(v)
	}

	// Pre-allocate strings.Builder for the slow path
	var builder strings.Builder
	for i := 0; i < rv.Len(); i++ {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(fmt.Sprint(rv.Index(i).Interface()))
	}
	return builder.String()
}

func Load(st store.Store, configPath string) (*Bundle, error) {
	bs, err := st.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not read config %q: %v", configPath, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(bs))
	dec.KnownFields(true)
	var bundle Bundle
	if err := dec.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("invalid configuration YAML in %q: %v", configPath, err)
	}

	// Populate and validate computed fields
	for k, abc := range bundle.UI.AnnotationBasedContent {
		for _, col := range abc.Columns {
			tmpl, err := template.New("col").Funcs(template.FuncMap{
				"join": join,
			}).Parse(col.Data)
			if err != nil {
				return nil, fmt.Errorf("invalid column template for annotationBasedContent %q: %v", k, err)
			}
			col.dataTemplate = tmpl
		}
	}

	return &bundle, nil
}
