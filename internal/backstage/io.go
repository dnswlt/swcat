package backstage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v2"
)

var (
	kindFactories = map[string]func() Entity{
		"Domain":    func() Entity { return &Domain{} },
		"System":    func() Entity { return &System{} },
		"Component": func() Entity { return &Component{} },
		"Resource":  func() Entity { return &Resource{} },
		"API":       func() Entity { return &API{} },
		"Group":     func() Entity { return &Group{} },
	}
)

// WriteEntities safely writes a slice of entities to a given path.
// It writes to a temporary file first and then atomically moves it to the final destination.
func WriteEntities(path string, entities []Entity) error {
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

func ReadEntities(path string) ([]Entity, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(f)

	var entities []Entity

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
