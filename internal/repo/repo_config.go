package repo

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	starlarkinterp "github.com/dnswlt/swcat/internal/starlark"
	"github.com/dnswlt/swcat/internal/store"
	"gopkg.in/yaml.v3"
)

// ValueRegexp is a wrapper around regexp.Regexp to allow for custom YAML unmarshaling.
type ValueRegexp regexp.Regexp

// ValueRule defines a validation rule for a string value.
// It can enforce a specific list of values or a set of regular expressions.
type ValueRule struct {
	Values  []string       `yaml:"values"`
	Matches []*ValueRegexp `yaml:"matches"`
}

type DomainValidationRules struct {
	Type *ValueRule `yaml:"type"`
}
type SystemValidationRules struct {
	Type *ValueRule `yaml:"type"`
}
type ComponentValidationRules struct {
	Type      *ValueRule `yaml:"type"`
	Lifecycle *ValueRule `yaml:"lifecycle"`
}
type ResourceValidationRules struct {
	Type *ValueRule `yaml:"type"`
}
type APIValidationRules struct {
	Type      *ValueRule `yaml:"type"`
	Lifecycle *ValueRule `yaml:"lifecycle"`
}

type CatalogValidationRules struct {
	Domain    *DomainValidationRules    `yaml:"domain"`
	System    *SystemValidationRules    `yaml:"system"`
	Component *ComponentValidationRules `yaml:"component"`
	Resource  *ResourceValidationRules  `yaml:"resource"`
	API       *APIValidationRules       `yaml:"api"`
}

type AnnotationBasedLink struct {
	// The URL to which the annotation-based link should point.
	// May use {{ .Annotation.Value }} template placeholders.
	// [required]
	URL string `yaml:"url,omitempty"`
	// A user friendly display name for the link.
	// May use {{ .Annotation.Value }} template placeholders.
	// [optional]
	Title string `yaml:"title,omitempty"`
	// A key representing a visual icon to be displayed in the UI.
	// [optional]
	Icon string `yaml:"icon,omitempty"`
	// An optional value to categorize links into specific groups.
	// [optional]
	Type string `yaml:"type,omitempty"`
	// Deprecated: use StarlarkLinks for generating multiple links. This remains
	// supported so historical catalog revisions keep loading.
	MultiLinks []MultiLinkEntry `yaml:"multiLinks,omitempty"`
	// Deprecated: use StarlarkLinks with iannotation and json.decode. This
	// remains supported so historical catalog revisions keep loading.
	MultiLinkData string `yaml:"multiLinkData,omitempty"`
}

// MultiLinkEntry defines a single named value used in multiLinks expansion.
// The Label is the per-link label (e.g. "dev") and Value is substituted into
// URL/title templates via {{ .MultiLink.Value }}.
//
// Deprecated: retained for compatibility with historical catalog revisions.
type MultiLinkEntry struct {
	Label string `yaml:"label" json:"label"`
	Value any    `yaml:"value" json:"value"`
}

// AutomaticLink is the legacy template-based generated-link configuration.
//
// Deprecated: use StarlarkLink. This remains supported so historical catalog
// revisions keep loading.
type AutomaticLink struct {
	// A filter expression that determines which entities this link applies to.
	// [required]
	Filter string `yaml:"filter"`
	// The URL to which the link should point.
	// May use {{ .Metadata.Name }} etc. template placeholders.
	// [required]
	URL string `yaml:"url"`
	// A user friendly display name for the link.
	// [optional]
	Title string `yaml:"title,omitempty"`
	// A key representing a visual icon to be displayed in the UI.
	// [optional]
	Icon string `yaml:"icon,omitempty"`
	// An optional value to categorize links into specific groups.
	// [optional]
	Type string `yaml:"type,omitempty"`
	// Deprecated: retained for compatibility with historical catalog revisions.
	MultiLinks []MultiLinkEntry `yaml:"multiLinks,omitempty"`
	// Deprecated: retained for compatibility with historical catalog revisions.
	MultiLinkData string `yaml:"multiLinkData,omitempty"`
}

// StarlarkLink generates links by executing a Starlark file for every entity
// matching Filter.
type StarlarkLink struct {
	// A filter expression that determines which entities this script applies to.
	// [required]
	Filter string `yaml:"filter"`
	// A Starlark file relative to swcat.yml. It must define links(entity).
	// [required]
	File string `yaml:"file"`

	program *starlarkinterp.Program
}

// Config holds repository-specific application configuration.
type Config struct {
	AnnotationBasedLinks map[string]*AnnotationBasedLink `yaml:"annotationBasedLinks"`
	// Deprecated: use StarlarkLinks. Retained for historical catalog revisions.
	AutomaticLinks []*AutomaticLink        `yaml:"automaticLinks"`
	StarlarkLinks  []*StarlarkLink         `yaml:"starlarkLinks"`
	Validation     *CatalogValidationRules `yaml:"validation"`
}

// LoadStarlarkLinks reads and compiles all configured Starlark link files.
// File paths are relative to the directory containing swcat.yml.
func (c *Config) LoadStarlarkLinks(st store.Store, configDir string) error {
	for i, link := range c.StarlarkLinks {
		if link == nil {
			return fmt.Errorf("starlarkLinks[%d] is null", i)
		}
		if strings.TrimSpace(link.Filter) == "" {
			return fmt.Errorf("starlarkLinks[%d] has an empty filter", i)
		}
		if strings.TrimSpace(link.File) == "" {
			return fmt.Errorf("starlarkLinks[%d] has an empty file", i)
		}

		filename := path.Join(configDir, link.File)
		source, err := st.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("starlarkLinks[%d]: could not read file %q: %w", i, link.File, err)
		}
		program, err := starlarkinterp.Compile(filename, source)
		if err != nil {
			return fmt.Errorf("starlarkLinks[%d]: compile %q: %w", i, link.File, err)
		}
		link.program = program
	}
	return nil
}

func (r *CatalogValidationRules) Accept(e catalog.Entity) error {
	switch v := e.(type) {
	case *catalog.Domain:
		if r.Domain != nil {
			if !r.Domain.Type.Accept(v.GetType()) {
				return fmt.Errorf("invalid type %q (allowed: %s)", v.GetType(), r.Domain.Type.Describe())
			}
		}
	case *catalog.System:
		if r.System != nil {
			if !r.System.Type.Accept(v.GetType()) {
				return fmt.Errorf("invalid type %q (allowed: %s)", v.GetType(), r.System.Type.Describe())
			}
		}
	case *catalog.Component:
		if r.Component != nil {
			if !r.Component.Type.Accept(v.GetType()) {
				return fmt.Errorf("invalid type %q (allowed: %s)", v.GetType(), r.Component.Type.Describe())
			}
			if !r.Component.Lifecycle.Accept(v.GetLifecycle()) {
				return fmt.Errorf("invalid lifecycle %q (allowed: %s)", v.GetLifecycle(), r.Component.Lifecycle.Describe())
			}
		}
	case *catalog.Resource:
		if r.Resource != nil {
			if !r.Resource.Type.Accept(v.GetType()) {
				return fmt.Errorf("invalid type %q (allowed: %s)", v.GetType(), r.Resource.Type.Describe())
			}
		}
	case *catalog.API:
		if r.API != nil {
			if !r.API.Type.Accept(v.GetType()) {
				return fmt.Errorf("invalid type %q (allowed: %s)", v.GetType(), r.API.Type.Describe())
			}
			if !r.API.Lifecycle.Accept(v.GetLifecycle()) {
				return fmt.Errorf("invalid lifecycle %q (allowed: %s)", v.GetLifecycle(), r.API.Lifecycle.Describe())
			}
			for _, ver := range v.Spec.Versions {
				if !r.API.Lifecycle.Accept(ver.Lifecycle) {
					return fmt.Errorf("invalid lifecycle %q (allowed: %s)", ver.Lifecycle, r.API.Lifecycle.Describe())
				}
			}
		}
	}
	// If no specific rules failed, the entity is considered valid.
	return nil
}

// Describe returns a human-readable description of the allowed values.
func (r *ValueRule) Describe() string {
	if r == nil {
		return "any value"
	}
	if len(r.Values) > 0 {
		// e.g. "one of [service, library]"
		return fmt.Sprintf("one of [%s]", strings.Join(r.Values, ", "))
	}
	if len(r.Matches) > 0 {
		// e.g. "matching patterns [^[a-z]+$, ^[0-9]+$]"
		patterns := make([]string, len(r.Matches))
		for i, re := range r.Matches {
			patterns[i] = (*regexp.Regexp)(re).String()
		}
		if len(patterns) == 1 {
			return fmt.Sprintf("matching pattern %s", patterns[0])
		}
		return fmt.Sprintf("matching any of patterns [%s]", strings.Join(patterns, ", "))
	}
	return "any value"
}

// Accept checks if a given value is valid according to the rule.
func (r *ValueRule) Accept(val string) bool {
	if r == nil {
		// If no rule is defined, all values are accepted.
		return true
	}
	if r.Values != nil {
		// If an explicit list of values is provided, check against it.
		return slices.Contains(r.Values, val)
	}
	if r.Matches != nil {
		// If regex patterns are provided, check if any of them match.
		for _, re := range r.Matches {
			// Cast the *ValueRegexp back to a *regexp.Regexp to access its methods.
			if (*regexp.Regexp)(re).MatchString(val) {
				return true
			}
		}
		// If there are regexes but none matched, the value is not accepted.
		return false
	}
	// If the rule is empty (e.g., "type:"), all values are accepted.
	return true
}

// UnmarshalYAML implements the yaml.Unmarshaler interface for ValueRegexp.
// This allows converting a string from a YAML file directly into a compiled regexp.
func (vr *ValueRegexp) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	if s == "" {
		return fmt.Errorf("regexp pattern in validation rule cannot be empty")
	}

	fullMatchPattern := "^(?:" + s + ")$"

	re, err := regexp.Compile(fullMatchPattern)
	if err != nil {
		// Return a more informative error message.
		return fmt.Errorf("failed to compile validation regexp %q: %w", s, err)
	}

	// Assign the compiled regexp to the ValueRegexp.
	*vr = ValueRegexp(*re)
	return nil
}
