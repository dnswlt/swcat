package sbom

import (
	"fmt"
	"regexp"
	"strings"

	cdx "github.com/CycloneDX/cyclonedx-go"
)

type ComponentFilter struct {
	Types       []string `yaml:"types"`
	NamePattern string   `yaml:"namePattern"`
}

type Component struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version"`
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

func FilterComponents(bom *cdx.BOM, filter ComponentFilter) ([]Component, error) {
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
	var result []Component
	for _, comp := range *bom.Components {
		if nameRE != nil && !nameRE.MatchString(comp.Name) {
			continue
		}
		if len(types) > 0 && !types[string(comp.Type)] {
			continue
		}
		result = append(result, Component{
			Type:    string(comp.Type),
			Name:    comp.Name,
			Version: comp.Version,
		})
	}
	return result, nil
}
