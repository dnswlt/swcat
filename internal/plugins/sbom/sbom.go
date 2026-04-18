package sbom

import (
	"fmt"
	"regexp"
	"strings"

	cdx "github.com/CycloneDX/cyclonedx-go"
)

// ComponentsFilter defines which items of an SBOM to include in the MiniBOM.
type ComponentsFilter struct {
	// Defines the types of SBOM components to include in the MiniBOM, e.g., "library".
	Types []string `yaml:"types"`
	// A regex to filter SBOM components to include in the MiniBOM
	NamePattern string `yaml:"namePattern"`
}

// MiniBOM is a minimal representation of a CycloneDX SBOM.
type MiniBOM struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	Components []string `json:"components"`
}

func Parse(input string) (*cdx.BOM, error) {
	var bom cdx.BOM
	dec := cdx.NewBOMDecoder(strings.NewReader(input), cdx.BOMFileFormatJSON)
	err := dec.Decode(&bom)
	if err != nil {
		return nil, fmt.Errorf("failed to decode BOM: %w", err)
	}
	return &bom, nil
}

// FilterComponents uses the given filter to build a MiniBOM from the given bom.
func FilterComponents(bom *cdx.BOM, filter ComponentsFilter) (*MiniBOM, error) {
	var nameRE *regexp.Regexp
	if filter.NamePattern != "" {
		r, err := regexp.Compile(filter.NamePattern)
		if err != nil {
			return nil, fmt.Errorf("invalid namePattern: %q: %w", filter.NamePattern, err)
		}
		nameRE = r
	}
	types := make(map[string]bool)
	for _, t := range filter.Types {
		types[t] = true
	}
	if bom.Components == nil {
		return nil, nil
	}
	var components []string
	for _, comp := range *bom.Components {
		if nameRE != nil && !nameRE.MatchString(comp.Name) {
			continue
		}
		if len(types) > 0 && !types[string(comp.Type)] {
			continue
		}
		components = append(components, comp.Name+":"+comp.Version)
	}

	var name, version string
	if bom.Metadata != nil && bom.Metadata.Component != nil {
		c := bom.Metadata.Component
		name = c.Name
		version = c.Version
	}

	return &MiniBOM{
		Name:       name,
		Version:    version,
		Components: components,
	}, nil
}
