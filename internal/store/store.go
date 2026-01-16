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
	"strings"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/gitclient"
	"gopkg.in/yaml.v3"
)

const (
	YAMLIndent = 2
)

var (
	ErrReadOnly  = errors.New("store is read-only")
	ErrNoSuchRef = errors.New("no such ref")
)

// Source is the abstraction over different types of storage layers,
// in particular local disk (non-versioned) and a Git repo (read-only).
type Source interface {
	// Refresh updates the internal state of the source (e.g., via git fetch).
	// For a disk store, this might be a no-op.
	Refresh() error
	// Store returns a handle to a store at the given ref.
	// For non-versioned disk-based stores, ref must be "".
	Store(ref string) (Store, error)
}

// Store is a minimal abstraction to list, read, and write files.
// It is the common interface for disk-based and git-repo-based stores.
type Store interface {
	// ListFiles lists all files in dir (recursively).
	// The resulting paths will all be relative to the store's root directory,
	// so they can be passed to ReadFile and WriteFile unmodified.
	ListFiles(dir string) ([]string, error)
	// ReadFile reads the contents of path from the store.
	// path should be a relative path (e.g., "catalog/domain.yml").
	ReadFile(path string) ([]byte, error)
	// WriteFile write the given contents to path in the store.
	// Stores that do not support writing should return ErrReadOnly.
	WriteFile(path string, contents []byte) error
}

// DiskStore is an implementation of Source and Store that reads files from the local file system.
type DiskStore struct {
	rootDir string // Root path of the catalog
}

// Asserts that DiskStore implements both Source and Store.
var _ Source = (*DiskStore)(nil)
var _ Store = (*DiskStore)(nil)

func NewDiskStore(rootDir string) *DiskStore {
	return &DiskStore{
		rootDir: rootDir,
	}
}

func (d *DiskStore) Refresh() error {
	// Nothing to do for a disk-based store.
	return nil
}

func (d *DiskStore) Store(ref string) (Store, error) {
	if ref != "" {
		return nil, fmt.Errorf("invalid ref %q: %w", ref, ErrNoSuchRef)
	}
	return d, nil
}

func (d *DiskStore) ListFiles(dir string) ([]string, error) {
	return listFilesRecursively(d.rootDir, dir)
}

func resolveRelPath(root, subpath string) (string, error) {
	fullPath := filepath.Join(root, subpath)

	// Verify ancestry by calculating the relative path from the root.
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", fmt.Errorf("not a relative path: %v", err) // e.g. paths on different volumes
	}

	// A relative path escaping the root will start with ".."
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q escapes root directory", subpath)
	}

	return fullPath, nil
}

func (d *DiskStore) ReadFile(path string) ([]byte, error) {
	fullPath, err := resolveRelPath(d.rootDir, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(fullPath)
}

func (d *DiskStore) WriteFile(path string, contents []byte) error {
	fullPath, err := resolveRelPath(d.rootDir, path)
	if err != nil {
		return err
	}
	return os.WriteFile(fullPath, contents, 0644)
}

// GitSource is an implementation of Source that reads from a remote Git repository.
type GitSource struct {
	client     *gitclient.Client
	defaultRef string // ref to use if the empty ref ("") is requested
}

// gitStore is a "view" over a single revision in a GitSource.
type gitStore struct {
	client *gitclient.Client
	ref    string
}

var _ Source = (*GitSource)(nil)
var _ Store = (*gitStore)(nil)

func NewGitSource(client *gitclient.Client, defaultRef string) *GitSource {
	return &GitSource{
		client:     client,
		defaultRef: defaultRef,
	}
}

func (g *GitSource) Refresh() error {
	return g.client.Update()
}

func (g *GitSource) Store(ref string) (Store, error) {
	if ref == "" {
		ref = g.defaultRef
	}
	refs, err := g.client.ListReferences()
	if err != nil {
		return nil, fmt.Errorf("cannot list references: %v", err)
	}
	if !slices.Contains(refs, ref) {
		return nil, ErrNoSuchRef
	}
	return &gitStore{
		client: g.client,
		ref:    ref,
	}, nil
}

func (g *gitStore) ListFiles(dir string) ([]string, error) {
	files, err := g.client.ListFilesRecursive(g.ref, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %v", err)
	}
	// Make relative to gitStore root.
	result := make([]string, len(files))
	for i, f := range files {
		result[i] = filepath.Join(dir, f)
	}
	return result, nil
}

func (g *gitStore) ReadFile(path string) ([]byte, error) {
	return g.client.ReadFile(g.ref, path)
}

func (g *gitStore) WriteFile(path string, contents []byte) error {
	return ErrReadOnly
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
	if _, ok := st.(*DiskStore); !ok {
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

	if err := writeEntities(st, path, remaining); err != nil {
		return fmt.Errorf("failed to write updated entity file %s: %v", path, err)
	}

	return nil
}

func InsertOrReplaceEntity(st Store, path string, entity api.Entity) error {
	// Only disk-based repos can currently be modified.
	if _, ok := st.(*DiskStore); !ok {
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

	if err := writeEntities(st, path, entities); err != nil {
		return fmt.Errorf("failed to write updated entity file %s: %v", path, err)
	}

	return nil
}

// writeEntities writes a slice of entities to a given path.
func writeEntities(st Store, path string, entities []api.Entity) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(YAMLIndent)
	for _, e := range entities {
		if err := enc.Encode(e.GetSourceInfo().Node); err != nil {
			return fmt.Errorf("failed to encode node from line %d: %w", e.GetSourceInfo().Line, err)
		}
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("failed to close encoder: %w", err)
	}
	return st.WriteFile(path, buf.Bytes())
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

// listFilesRecursively lists all files in subDir, which must
// be a relative path specifying a sub-directory of rootDir.
// The resulting paths will all be relative to rootDir.
//
// Example:
// with rootDir "/foo/bar" and subDir "baz/quz", all files under
// "/foo/bar/baz/quz" will be returned, relative to "/foo/bar", such as
// ["baz/quz/yankee.yml"].
func listFilesRecursively(rootDir, subDir string) ([]string, error) {
	var files []string

	startDir := filepath.Join(rootDir, subDir)
	err := filepath.WalkDir(startDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Handle errors accessing a path (e.g. permission denied)
			return err
		}

		// If it's a directory, we just continue (it will automatically recurse)
		if d.IsDir() {
			return nil
		}

		// Add the relative file path to our list
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}

		files = append(files, relPath)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// CatalogFiles lists all *.yml files under catalogRoot, which must be
// a relative path (relative to the store's root).
func CatalogFiles(st Store, catalogRoot string) ([]string, error) {
	allFiles, err := st.ListFiles(catalogRoot)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, f := range allFiles {
		if strings.HasSuffix(strings.ToLower(f), ".yml") {
			result = append(result, f)
		}
	}

	return result, nil
}
