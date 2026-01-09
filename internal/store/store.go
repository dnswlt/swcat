package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/gitclient"
	"gopkg.in/yaml.v3"
)

const (
	YAMLIndent = 2
)

// Store is a minimal abstraction to read catalog files.
// The idea is to support both reading from a read-only in-memory Git repo branch
// as well as from disk.
type Store interface {
	// Lists all catalog (*.yml) files in the store.
	CatalogFiles() ([]string, error)
	// Reads the contents of path from the store.
	// path should be a relative path (e.g., "catalog/domain.yml").
	ReadFile(path string) ([]byte, error)
}

// diskStore is an implementation of Store that reads files from the local file system.
type diskStore struct {
	catalogRoot string // Root path of the catalog
}

func NewDiskStore(catalogRoot string) *diskStore {
	return &diskStore{
		catalogRoot: catalogRoot,
	}
}

func (d *diskStore) CatalogFiles() ([]string, error) {
	return collectYMLFilesInDir(d.catalogRoot)
}
func (d *diskStore) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// gitStore is an implementation of Store that reads from a remote Git repository.
type gitStore struct {
	client      *gitclient.Client
	catalogRoot string
	currentRef  string
}

func NewGitStore(client *gitclient.Client, catalogRoot string, currentRef string) *gitStore {
	return &gitStore{
		client:      client,
		catalogRoot: catalogRoot,
		currentRef:  currentRef,
	}
}

func (g *gitStore) WithCurrentRef(ref string) (*gitStore, error) {
	refs, err := g.client.ListReferences()
	if err != nil {
		return nil, fmt.Errorf("cannot list references: %v", err)
	}
	if !slices.Contains(refs, ref) {
		return nil, fmt.Errorf("ref %q not found", ref)
	}
	g.currentRef = ref
	return NewGitStore(g.client, g.catalogRoot, ref), nil
}

func (g *gitStore) CatalogFiles() ([]string, error) {
	files, err := g.client.ListFilesRecursive(g.currentRef, g.catalogRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %v", err)
	}
	var ymlFiles []string
	for _, f := range files {
		if strings.HasSuffix(strings.ToLower(f), ".yml") {
			ymlFiles = append(ymlFiles, filepath.Join(g.catalogRoot, f))
		}
	}
	return ymlFiles, nil
}

func (g *gitStore) ReadFile(path string) ([]byte, error) {
	return g.client.ReadFile(g.currentRef, path)
}

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

func DeleteEntity(st Store, path string, ref *api.Ref) error {
	// Only disk-based repos can currently be modified.
	if _, ok := st.(*diskStore); !ok {
		return fmt.Errorf("cannot update catalog in store of type %T", st)
	}
	entities, err := ReadEntities(st, path)
	if err != nil {
		return fmt.Errorf("failed to read entity file %s: %v", path, err)
	}

	// Remove the modified entity from the list of entities read from its path.
	remaining := make([]api.Entity, 0, len(entities))
	var found bool
	for _, e := range entities {
		if e.GetRef().Equal(ref) {
			// Replace old with new for writing back to disk
			found = true
			continue
		}
		remaining = append(remaining, e)
	}
	if !found {
		return fmt.Errorf("failed to delete entity %s from file %s", ref, path)
	}

	if err := writeEntities(path, remaining); err != nil {
		return fmt.Errorf("failed to write updated entity file %s: %v", path, err)
	}

	return nil
}

func InsertOrReplaceEntity(st Store, path string, entity api.Entity) error {
	// Only disk-based repos can currently be modified.
	if _, ok := st.(*diskStore); !ok {
		return fmt.Errorf("cannot update catalog in store of type %T", st)
	}
	entities, err := ReadEntities(st, path)
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

	if err := writeEntities(path, entities); err != nil {
		return fmt.Errorf("failed to write updated entity file %s: %v", path, err)
	}

	return nil
}

// writeEntities safely writes a slice of entities to a given path.
// It writes to a temporary file first and then atomically moves it to the final destination.
func writeEntities(path string, entities []api.Entity) error {
	// 1. Create a temporary file in the same directory as the target path.
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, "swcat-*.tmp")
	if err != nil {
		return fmt.Errorf("could not create temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	enc := yaml.NewEncoder(tmpFile)
	enc.SetIndent(YAMLIndent)
	for _, e := range entities {
		if err := enc.Encode(e.GetSourceInfo().Node); err != nil {
			tmpFile.Close()
			return fmt.Errorf("failed to encode node from line %d: %w", e.GetSourceInfo().Line, err)
		}
	}
	enc.Close()
	tmpFile.Close()
	if err := os.Chmod(tmpFile.Name(), 0644); err != nil {
		return fmt.Errorf("could not chmod temporary file: %v", err)
	}

	return os.Rename(tmpFile.Name(), path)
}

func ReadEntities(st Store, path string) ([]api.Entity, error) {
	bs, err := st.ReadFile(path)
	if err != nil {
		return nil, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(bs))
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

func CopyNode(node *yaml.Node) (*yaml.Node, error) {
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

func NewEntityFromNode(node *yaml.Node, strict bool) (api.Entity, error) {

	if len(node.Content) == 0 {
		return nil, errors.New("empty yaml document")
	}

	kind, err := findKindInNode(node)
	if err != nil {
		return nil, fmt.Errorf("error in document: %w", err)
	}

	factory, ok := kindFactories[kind]
	if !ok {
		return nil, fmt.Errorf("invalid kind '%s'", kind)
	}
	entity := factory()

	if strict {
		// See comments in ReadEntities about why this dance is necessary.
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
	entity.SetSourceInfo(&api.SourceInfo{
		Node: node,
	})

	return entity, nil
}

func NewEntityFromString(content string) (api.Entity, error) {
	var node yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(content))
	err := dec.Decode(&node)
	if err != nil {
		return nil, fmt.Errorf("failed to decode YAML node: %w", err)
	}
	return NewEntityFromNode(&node, true)
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

// SetAnnotationInNode finds the metadata.annotations map in the given YAML document node
// and sets the given key to the given value. If the key already exists, its value is
// updated. If the metadata or annotations maps do not exist, they are created.
func SetAnnotationInNode(doc *yaml.Node, key, value string) error {
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

// collectYMLFilesInDir walks root recursively up to maxDepth levels below root
// (root itself is depth 0) and returns all *.yml files it finds.
// It does NOT follow symlinks.
func collectYMLFilesInDir(root string) ([]string, error) {
	root = filepath.Clean(root)
	var out []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // propagate filesystem error
		}

		if d.IsDir() {
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
