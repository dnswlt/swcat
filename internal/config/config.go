package config

import (
	"bytes"
	"fmt"

	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
	"github.com/dnswlt/swcat/internal/svg"
	"gopkg.in/yaml.v3"
)

// AnnotationBasedContent specifies how annotation-based content should be rendered in the UI.
type AnnotationBasedContent struct {
	Heading string // The heading under which to display the content.
	Style   string // The style in which to render the content. One of "text", "list", "json", "table".
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
	AnnotationBasedContent map[string]AnnotationBasedContent `yaml:"annotationBasedContent"`
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
	return &bundle, nil
}
