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

// NewEntityFromNodeWithAnnotation creates a new entity from the given node,
// adding `key: value` as an additional metadata.annotation.
func NewEntityFromNodeWithAnnotation(node *yaml.Node, key, value string) (Entity, error) {
	copy, err := copyNode(node)
	if err != nil {
		return nil, fmt.Errorf("failed to create a copy to set annotation: %v", err)
	}
	if err := setAnnotationInNode(copy, key, value); err != nil {
		return nil, fmt.Errorf("failed to set annotation: %v", err)
	}
	return NewEntityFromNode(copy, false)
}

// setAnnotationInNode finds the metadata.annotations map in the given YAML document node
// and sets the given key to the given value. If the key already exists, its value is
// updated. If the metadata or annotations maps do not exist, they are created.
func setAnnotationInNode(doc *yaml.Node, key, value string) error {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return errors.New("expected a YAML document with a top-level map")
	}
	rootMap := doc.Content[0]

	// 1. Find or create 'metadata'
	var metadataNode *yaml.Node
	for i := 0; i < len(rootMap.Content); i += 2 {
		if rootMap.Content[i].Value == "metadata" {
			metadataNode = rootMap.Content[i+1]
			break
		}
	}
	if metadataNode == nil {
		// Not found, create it
		metadataKeyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "metadata"}
		metadataNode = &yaml.Node{Kind: yaml.MappingNode}
		rootMap.Content = append(rootMap.Content, metadataKeyNode, metadataNode)
	}
	if metadataNode.Kind != yaml.MappingNode {
		return errors.New("'metadata' field is not a map")
	}

	// 2. Find or create 'annotations'
	var annotationsNode *yaml.Node
	for i := 0; i < len(metadataNode.Content); i += 2 {
		if metadataNode.Content[i].Value == "annotations" {
			annotationsNode = metadataNode.Content[i+1]
			break
		}
	}
	if annotationsNode == nil {
		// Not found, create it
		annotationsKeyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "annotations"}
		annotationsNode = &yaml.Node{Kind: yaml.MappingNode}
		metadataNode.Content = append(metadataNode.Content, annotationsKeyNode, annotationsNode)
	}
	if annotationsNode.Kind != yaml.MappingNode {
		return errors.New("'annotations' field is not a map")
	}

	// 3. Find and update or create the annotation key
	var found bool
	for i := 0; i < len(annotationsNode.Content); i += 2 {
		if annotationsNode.Content[i].Value == key {
			// Found it, update the value
			annotationsNode.Content[i+1].Value = value
			annotationsNode.Content[i+1].Tag = "!!str" // Ensure it's a string
			found = true
			break
		}
	}

	if !found {
		// Not found, append it
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
		valueNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
		annotationsNode.Content = append(annotationsNode.Content, keyNode, valueNode)
	}

	return nil
}

// copyNode returns a deep copy of the given node.
func copyNode(node *yaml.Node) (*yaml.Node, error) {
	if node == nil {
		return nil, nil
	}
	data, err := yaml.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("failed to encode node: %v", err)
	}
	var copiedNode yaml.Node
	err = yaml.Unmarshal(data, &copiedNode)
	if err != nil {
		return nil, fmt.Errorf("failed to decode node: %v", err)
	}
	return &copiedNode, nil
}
