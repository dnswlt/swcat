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

// NewCustomColumn returns a CustomColumn with its Data template pre-parsed.
// Useful for programmatic construction (e.g. in tests); YAML-loaded columns
// are populated by Load.
func NewCustomColumn(header, data string) (*CustomColumn, error) {
	tmpl, err := template.New("col").Funcs(template.FuncMap{
		"join": join,
	}).Parse(data)
	if err != nil {
		return nil, err
	}
	return &CustomColumn{Header: header, Data: data, dataTemplate: tmpl}, nil
}

// CustomContent specifies how a single block of annotation-based or status-based
// content should be rendered in the UI.
type CustomContent struct {
	// Optional heading. At the group level, used as the section heading
	// (in the <summary> of the <details>). At the block level (when used
	// inside a Blocks list), rendered as a sub-heading above the block.
	Heading string `yaml:"heading"`
	// The style in which to render the content. One of "text", "list", "attrs", "json", "table".
	Style string `yaml:"style"`
	// For style "attrs", the attributes (GJSON paths, e.g. "field1", "nested.field") to display. If empty, all fields are displayed.
	Fields []string `yaml:"fields"`
	// For style "table", the columns to be rendered from the JSON list of objects.
	Columns []*CustomColumn `yaml:"columns"`
	// A GJSON path that selects the root element to be rendered.
	// If empty, the full (annotation/status) JSON is used.
	Selector string `yaml:"selector"`
}

// CustomContentSection describes one <details> section on an entity detail page.
// The section can contain multiple Blocks; for backward compatibility the
// inline CustomContent is used as a single block when Blocks is empty.
// Sections are bound to an annotation key or status observation key by the
// map they live in (UIConfig.AnnotationBasedContent / StatusBasedContent).
type CustomContentSection struct {
	// Used to order multiple sections on the same page.
	Rank int `yaml:"rank"`
	// If true, the section <details> is open on load.
	Open bool `yaml:"open"`
	// Multiple blocks to render inside the same section. If empty, the
	// inline CustomContent is rendered as a single block.
	Blocks        []*CustomContent `yaml:"blocks"`
	CustomContent `yaml:",inline"`
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
	AnnotationBasedContent map[string]*CustomContentSection `yaml:"annotationBasedContent"`
	StatusBasedContent     map[string]*CustomContentSection `yaml:"statusBasedContent"`
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

// parseCustomContentColumns parses (and replaces) each Column's Data template.
func parseCustomContentColumns(cc *CustomContent) error {
	for i, col := range cc.Columns {
		parsed, err := NewCustomColumn(col.Header, col.Data)
		if err != nil {
			return err
		}
		cc.Columns[i] = parsed
	}
	return nil
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

	// Populate and validate computed fields
	for k, abc := range bundle.UI.AnnotationBasedContent {
		if err := parseCustomContentColumns(&abc.CustomContent); err != nil {
			return nil, fmt.Errorf("invalid column template for annotationBasedContent %q: %v", k, err)
		}
		for j, block := range abc.Blocks {
			if err := parseCustomContentColumns(block); err != nil {
				return nil, fmt.Errorf("invalid column template for annotationBasedContent %q block %d: %v", k, j, err)
			}
		}
	}
	for k, sbc := range bundle.UI.StatusBasedContent {
		if err := parseCustomContentColumns(&sbc.CustomContent); err != nil {
			return nil, fmt.Errorf("invalid column template for statusBasedContent %q: %v", k, err)
		}
		for j, block := range sbc.Blocks {
			if err := parseCustomContentColumns(block); err != nil {
				return nil, fmt.Errorf("invalid column template for statusBasedContent %q block %d: %v", k, j, err)
			}
		}
	}

	return &bundle, nil
}
