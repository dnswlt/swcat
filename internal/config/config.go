package config

import (
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/svg"
)

// AnnotationBasedContent specifies how annotation-based content should be rendered in the UI.
type AnnotationBasedContent struct {
	Heading string // The heading under which to display the content.
	Style   string // The style in which to render the content. One of "text", "list", "json".
}

// UIConfig has configuration that only affects the UI.
// We cannot put it into the web package as that would generate
// a cyclic dependency.
type UIConfig struct {
	AnnotationBasedContent map[string]AnnotationBasedContent `yaml:"annotationBasedContent"`
}

// Bundle is the umbrella struct for the serialized application configuration YAML.
// It bundles the package-specific configurations.
type Bundle struct {
	SVG     svg.Config  `yaml:"svg"`
	Catalog repo.Config `yaml:"catalog"`
	UI      UIConfig    `yaml:"ui"`
}
