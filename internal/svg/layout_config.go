package svg

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// ColorString represents valid strings for color names ("white") or hex codes ("#f7f7f7").
type ColorString string

type NodeColorsConfig struct {
	Labels map[string]map[string]ColorString `yaml:"labels"`
	Types  map[string]ColorString            `yaml:"types"`
}

type Config struct {
	// Labels whose values should be displayed as <<stereotypes>> in node labels.
	StereotypeLabels []string `yaml:"stereotypeLabels"`
	// Maps label keys and label values to node colors.
	// Can be used to override the default node colors per label value.
	NodeColors NodeColorsConfig `yaml:"nodeColors"`
	// Include the API provider (component) in labels of API entities.
	ShowAPIProvider bool `yaml:"showAPIProvider"`
}

var (
	colorStringRegex = regexp.MustCompile(`(?i)^(#[0-9a-f]{6}|[a-z]+)$`)
)

// UnmarshalYAML implements the yaml.Unmarshaler interface for ColorString.
// It supports simple color strings (a-z+) and hex codes ("#f7f7f7").
func (s *ColorString) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("color string must be a string scalar, but got %s", value.Tag)
	}

	if !colorStringRegex.MatchString(value.Value) {
		return fmt.Errorf("invalid color string '%s'", value.Value)
	}

	*s = ColorString(value.Value)
	return nil
}
