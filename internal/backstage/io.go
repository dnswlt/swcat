package backstage

import (
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
	"go.yaml.in/yaml/v2"
)

var (
	kindFactories = map[string]func() api.Entity{
		"Domain":    func() api.Entity { return &api.Domain{} },
		"System":    func() api.Entity { return &api.System{} },
		"Component": func() api.Entity { return &api.Component{} },
		"Resource":  func() api.Entity { return &api.Resource{} },
		"API":       func() api.Entity { return &api.API{} },
		"Group":     func() api.Entity { return &api.Group{} },
	}
)

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
	for _, e := range entities {
		if err := enc.Encode(e); err != nil {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return fmt.Errorf("failed to encode entity: %v", err)
		}
	}
	enc.Close()
	tmpFile.Close()

	return os.Rename(tmpFile.Name(), path)
}

func ReadEntities(path string) ([]api.Entity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(f)

	var entities []api.Entity

	for i := 0; ; i++ {
		doc := map[string]any{}
		err = dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
		if _, ok := doc["kind"]; !ok {
			return nil, fmt.Errorf("entity #%d has no kind: field", i)
		}
		switch kind := doc["kind"].(type) {
		case string:
			factory, ok := kindFactories[kind]
			if !ok {
				return nil, fmt.Errorf("invalid kind in YAML entity: %s", kind)
			}

			entity := factory()
			bs, err := yaml.Marshal(doc)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal as intermediate JSON: %w", err)
			}
			if err := yaml.UnmarshalStrict(bs, entity); err != nil {
				return nil, fmt.Errorf("failed to unmarshal intermediate JSON: %w", err)
			}
			entities = append(entities, entity)
		default:
			return nil, fmt.Errorf("kind: field has wrong type: %T", doc["kind"])
		}
	}

	return entities, nil
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

// LoadRepositoryFromPath reads entities from the given catalog paths
// and returns a validated repository.
// Elements in catalogPaths must be .yml file paths.
func LoadRepositoryFromPaths(catalogPaths []string) (*Repository, error) {
	repo := NewRepository()

	for _, catalogPath := range catalogPaths {
		log.Printf("Reading catalog file %s", catalogPath)
		entities, err := ReadEntities(catalogPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read entities from %s: %v", catalogPath, err)
		}
		for _, e := range entities {
			if err := repo.AddEntity(e); err != nil {
				return nil, fmt.Errorf("failed to add entity %q to the repo: %v", e.GetQName(), err)
			}
		}
	}
	if err := repo.Validate(); err != nil {
		return nil, fmt.Errorf("repository validation failed: %v", err)
	}

	return repo, nil

}
