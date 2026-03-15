package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
	"gopkg.in/yaml.v3"
)

const (
	YAMLIndent = 2

	CatalogDir   = "catalog"
	DocumentsDir = "documents"
	ConfigFile   = "swcat.yml"
	PluginsFile  = "plugins.yml"
	LintFile     = "lint.yml"
	KubeFile     = "kube.yml"
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

func DeleteEntity(st Store, path string, ref *api.Ref) error {
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
			return nil, fmt.Errorf("failed to decode YAML node in %q: %w", path, err)
		}

		// node.Content will be empty for blank documents (e.g., just "---")
		if len(node.Content) == 0 {
			continue
		}

		entity, err := api.NewEntityFromNode(&node, true)
		if err != nil {
			return nil, fmt.Errorf("error in document %q starting at line %d: %v", path, node.Line, err)
		}
		entity.GetSourceInfo().Path = path

		entities = append(entities, entity)
	}

	return entities, nil
}

// ExtensionFile returns the ".ext.json" sidecar file for the given file (which is typically a .yml file).
func ExtensionFile(file string) string {
	ext := filepath.Ext(file)
	lowExt := strings.ToLower(ext)

	if lowExt == ".yml" || lowExt == ".yaml" {
		return strings.TrimSuffix(file, ext) + ".ext.json"
	}

	return file + ".ext.json"
}

func ReadExtensions(st Store, path string) (*api.CatalogExtensions, error) {
	bs, err := st.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ext api.CatalogExtensions
	err = json.Unmarshal(bs, &ext)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s as JSON: %v", path, err)
	}
	return &ext, nil
}

func WriteExtensions(st Store, path string, ext *api.CatalogExtensions) error {
	bs, err := json.MarshalIndent(ext, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal extensions: %w", err)
	}
	return st.WriteFile(path, bs)
}

func MergeExtensions(st Store, path string, newExts *api.CatalogExtensions) error {
	extPath := ExtensionFile(path)
	existingExts, err := ReadExtensions(st, extPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read extensions: %w", err)
	}
	if existingExts == nil {
		existingExts = &api.CatalogExtensions{}
	}
	existingExts.Merge(newExts)

	if err := WriteExtensions(st, extPath, existingExts); err != nil {
		return fmt.Errorf("write extensions: %w", err)
	}
	return nil
}

// SetExtensionAnnotation sets a single annotation key/value for the given entity ref
// in the sidecar extension file for path. Other annotations and entities in the
// sidecar are preserved.
func SetExtensionAnnotation(st Store, path string, entityRef string, key string, value any) error {
	extPath := ExtensionFile(path)
	exts, err := ReadExtensions(st, extPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read extensions: %w", err)
	}
	if exts == nil {
		exts = api.NewCatalogExtensions()
	}
	entityExts := exts.Entities[entityRef]
	if entityExts == nil {
		entityExts = &api.MetadataExtensions{Annotations: map[string]any{}}
		exts.Entities[entityRef] = entityExts
	}
	if value == nil {
		delete(entityExts.Annotations, key)
	} else {
		if entityExts.Annotations == nil {
			entityExts.Annotations = map[string]any{}
		}
		entityExts.Annotations[key] = value
	}
	return WriteExtensions(st, extPath, exts)
}

// CatalogFiles lists all *.yml files under the default catalog directory.
func CatalogFiles(st Store) ([]string, error) {
	allFiles, err := st.ListFiles(CatalogDir)
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
