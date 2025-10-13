package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
	"gopkg.in/yaml.v3"
)

const (
	YAMLIndent = 2
)

var (
	kindFactories = map[string]func() api.Entity{
		api.YAMLKindDomain:    func() api.Entity { return &api.Domain{} },
		api.YAMLKindSystem:    func() api.Entity { return &api.System{} },
		api.YAMLKindComponent: func() api.Entity { return &api.Component{} },
		api.YAMLKindResource:  func() api.Entity { return &api.Resource{} },
		api.YAMLKindAPI:       func() api.Entity { return &api.API{} },
		api.YAMLKindGroup:     func() api.Entity { return &api.Group{} },
	}
)

func InsertOrReplace(path string, entity api.Entity) error {
	entities, err := ReadEntities(path)
	if err != nil {
		return fmt.Errorf("failed to read entity file %s: %v", path, err)
	}

	// Find and replace the modified entity in the list of entities read from its path.
	var found bool
	ref := entity.GetRef()
	for i, e := range entities {
		if e.GetRef().Equal(ref) {
			// Replace old with new for writing back to disk
			entities[i] = entity
			found = true
			break
		}
	}
	if !found {
		// New entity: append
		entities = append(entities, entity)
	}

	if err := WriteEntities(path, entities); err != nil {
		return fmt.Errorf("failed to write updated entity file %s: %v", path, err)
	}

	return nil
}

// WriteEntities safely writes a slice of entities to a given path.
// It writes to a temporary file first and then atomically moves it to the final destination.
func WriteEntities(path string, entities []api.Entity) error {
	// 1. Create a temporary file in the same directory as the target path.
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, "swcat-*.tmp")
	if err != nil {
		return fmt.Errorf("could not create temporary file: %v", err)
	}

	enc := yaml.NewEncoder(tmpFile)
	enc.SetIndent(YAMLIndent)
	for _, e := range entities {
		if err := enc.Encode(e.GetSourceInfo().Node); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return fmt.Errorf("failed to encode node from line %d: %w", e.GetSourceInfo().Line, err)
		}
	}
	enc.Close()
	tmpFile.Close()

	return os.Rename(tmpFile.Name(), path)
}

// WriteEntitiesByPath groups entities by their source path, sorts them by line number,
// and writes them back to their original files.
func WriteEntitiesByPath(allEntities []api.Entity) error {
	// Group entities by their source file path.
	entitiesByPath := make(map[string][]api.Entity)
	for _, e := range allEntities {
		path := e.GetSourceInfo().Path
		entitiesByPath[path] = append(entitiesByPath[path], e)
	}

	// Process each file.
	for path, entities := range entitiesByPath {
		// Sort the entities for this path by their original line number.
		sort.Slice(entities, func(i, j int) bool {
			return entities[i].GetSourceInfo().Line < entities[j].GetSourceInfo().Line
		})

		// Use the safe, atomic write pattern.
		err := WriteEntities(path, entities)
		if err != nil {
			return fmt.Errorf("failed to write entities to %s: %w", path, err)
		}
		log.Printf("Successfully wrote %d entities to %s\n", len(entities), path)
	}

	return nil
}

func ReadEntities(path string) ([]api.Entity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true) // We want to be strict and error out on any unknown field

	var entities []api.Entity

	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to decode YAML node: %w", err)
		}

		// node.Content will be empty for blank documents (e.g., just "---")
		if len(node.Content) == 0 {
			continue
		}

		// Find the 'kind' field to use the factory
		kind, err := findKindInNode(&node)
		if err != nil {
			return nil, fmt.Errorf("error in document starting at line %d: %v", node.Line, err)
		}

		factory, ok := kindFactories[kind]
		if !ok {
			return nil, fmt.Errorf("invalid kind '%s' in document at line %d", kind, node.Line)
		}
		entity := factory()

		// Re-encode the YAML document to then decode it strictly into the target type.
		// This is a necessary dance to make parsing strict - there is no equivalent "strict mode"
		// when decoding from the yaml.Node to the target struct directly :(
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		if err := enc.Encode(&node); err != nil {
			return nil, fmt.Errorf("failed to re-encode node: %v", err)
		}
		// Decode into the final typed struct
		strictDec := yaml.NewDecoder(&buf)
		strictDec.KnownFields(true)
		if err := strictDec.Decode(entity); err != nil {
			return nil, fmt.Errorf("failed to decode node into struct at line %d: %v", node.Line, err)
		}

		entity.SetSourceInfo(&api.SourceInfo{
			Path: path,
			Line: node.Line,
			Node: &node,
		})

		entities = append(entities, entity)
	}

	return entities, nil
}

func ReadEntityFromString(content string) (api.Entity, error) {
	var node yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(content))
	err := dec.Decode(&node)
	if err != nil {
		return nil, fmt.Errorf("failed to decode YAML node: %w", err)
	}

	if len(node.Content) == 0 {
		return nil, errors.New("empty yaml document")
	}

	kind, err := findKindInNode(&node)
	if err != nil {
		return nil, fmt.Errorf("error in document: %w", err)
	}

	factory, ok := kindFactories[kind]
	if !ok {
		return nil, fmt.Errorf("invalid kind '%s'", kind)
	}
	entity := factory()

	// See comments in ReadEntities about why this dance is necessary.
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(&node); err != nil {
		return nil, fmt.Errorf("failed to re-encode node: %v", err)
	}
	// Decode into the final typed struct
	strictDec := yaml.NewDecoder(&buf)
	strictDec.KnownFields(true)
	if err := strictDec.Decode(entity); err != nil {
		return nil, fmt.Errorf("failed to decode node into struct: %v", err)
	}

	entity.SetSourceInfo(&api.SourceInfo{
		Node: &node,
	})

	return entity, nil
}

// findKindInNode is a helper to extract the 'kind' value from a yaml.Node
func findKindInNode(doc *yaml.Node) (string, error) {
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

// collectYMLFilesInDir walks root recursively up to maxDepth levels below root
// (root itself is depth 0) and returns all *.yml files it finds.
// It does NOT follow symlinks. It skips directories deeper than maxDepth.
func collectYMLFilesInDir(root string, maxDepth int) ([]string, error) {
	root = filepath.Clean(root)
	var out []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // propagate filesystem error
		}

		if d.IsDir() {
			// Compute depth relative to root (root=0, its children=1, etc.)
			if path == root {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			depth := strings.Count(rel, string(os.PathSeparator)) + 1
			if depth > maxDepth {
				return fs.SkipDir
			}
			return nil
		}

		// Match *.yml (case-insensitive)
		if strings.HasSuffix(strings.ToLower(d.Name()), ".yml") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(out) // deterministic order
	return out, nil
}

func CollectYMLFiles(args []string, maxDepth int) ([]string, error) {
	var allFiles []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("failed to stat %s: %v", arg, err)
		}

		if info.IsDir() {
			// Collect files recursively, up to maxDepth levels deep
			files, err := collectYMLFilesInDir(arg, maxDepth)
			if err != nil {
				return nil, fmt.Errorf("failed to walk dir %s: %v", arg, err)
			}
			allFiles = append(allFiles, files...)
		} else {
			allFiles = append(allFiles, arg)
		}
	}
	return allFiles, nil

}
