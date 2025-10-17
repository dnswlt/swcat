package config

import (
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/svg"
)

// Bundle is the umbrella struct for the serialized application configuration YAML.
// It bundles the package-specific configurations.
type Bundle struct {
	SVG     svg.Config  `yaml:"svg"`
	Catalog repo.Config `yaml:"catalog"`
}
