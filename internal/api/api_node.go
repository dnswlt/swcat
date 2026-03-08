package api

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	kindFactories = map[string]func() Entity{
		YAMLKindDomain:    func() Entity { return &Domain{} },
		YAMLKindSystem:    func() Entity { return &System{} },
		YAMLKindComponent: func() Entity { return &Component{} },
		YAMLKindResource:  func() Entity { return &Resource{} },
		YAMLKindAPI:       func() Entity { return &API{} },
		YAMLKindGroup:     func() Entity { return &Group{} },
	}
)

// FindKindInNode is a helper to extract the 'kind' value from a yaml.Node
func FindKindInNode(doc *yaml.Node) (string, error) {
	// The top-level node is a DocumentNode, its content is a MappingNode
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return "", errors.New("expected a YAML document with a top-level map")
	}

	nodes := doc.Content[0].Content
	for i := 0; i < len(nodes); i += 2 {
		keyNode := nodes[i]
		if keyNode.Value == "kind" {
			valueNode := nodes[i+1]
			if valueNode.Kind != yaml.ScalarNode {
				return "", fmt.Errorf("'kind' field is not a string (type: %v)", valueNode.Tag)
			}
			return valueNode.Value, nil
		}
	}
	return "", errors.New("no 'kind' field found")
}

func NewEntityFromNode(node *yaml.Node, strict bool) (Entity, error) {

	if len(node.Content) == 0 {
		return nil, errors.New("empty yaml document")
	}

	kind, err := FindKindInNode(node)
	if err != nil {
		return nil, fmt.Errorf("error in document: %w", err)
	}

	factory, ok := kindFactories[kind]
	if !ok {
		return nil, fmt.Errorf("invalid kind '%s'", kind)
	}
	entity := factory()

	if strict {
		// Re-encode the YAML document to then decode it strictly into the target type.
		// This is a necessary dance to make parsing strict - there is no equivalent "strict mode"
		// when decoding from the yaml.Node to the target struct directly :(
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		if err := enc.Encode(node); err != nil {
			return nil, fmt.Errorf("failed to re-encode node: %v", err)
		}
		// Decode into the final typed struct
		strictDec := yaml.NewDecoder(&buf)
		strictDec.KnownFields(true)
		if err := strictDec.Decode(entity); err != nil {
			return nil, fmt.Errorf("failed to decode node into struct: %v", err)
		}
	} else {
		// Non-strict: decode directly into entity
		if err := node.Decode(entity); err != nil {
			return nil, fmt.Errorf("failed to decode node into struct: %v", err)
		}
	}
	entity.SetSourceInfo(&SourceInfo{
		Node: node,
		Line: node.Line,
	})

	return entity, nil
}

func NewEntityFromString(content string) (Entity, error) {
	var node yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(content))
	err := dec.Decode(&node)
	if err != nil {
		return nil, fmt.Errorf("failed to decode YAML node: %w", err)
	}
	return NewEntityFromNode(&node, true)
}
