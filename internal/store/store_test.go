package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dnswlt/swcat/internal/api"
)

func TestReadEntities(t *testing.T) {
	t.Run("valid entities", func(t *testing.T) {
		content := `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: my-component
spec:
  type: service
  owner: my-group
  lifecycle: experimental
---
apiVersion: swcat/v1alpha1
kind: Group
metadata:
  name: my-group
spec:
  type: team
`
		tmpfile := writeTempFile(t, "entities.yaml", content)
		defer os.Remove(tmpfile)

		entities, err := ReadEntities(tmpfile)
		if err != nil {
			t.Fatalf("ReadEntities() error = %v, wantErr %v", err, false)
		}
		if len(entities) != 2 {
			t.Fatalf("len(entities) = %d, want %d", len(entities), 2)
		}

		component, ok := entities[0].(*api.Component)
		if !ok {
			t.Fatalf("entities[0] is not a *Component")
		}
		if component.Metadata.Name != "my-component" {
			t.Errorf("component.Metadata.Name = %s, want %s", component.Metadata.Name, "my-component")
		}

		group, ok := entities[1].(*api.Group)
		if !ok {
			t.Fatalf("entities[1] is not a *Group")
		}
		if group.Metadata.Name != "my-group" {
			t.Errorf("group.Metadata.Name = %s, want %s", group.Metadata.Name, "my-group")
		}
	})

	t.Run("empty file", func(t *testing.T) {
		tmpfile := writeTempFile(t, "empty.yaml", "")
		defer os.Remove(tmpfile)

		entities, err := ReadEntities(tmpfile)
		if err != nil {
			t.Fatalf("ReadEntities() error = %v, wantErr %v", err, false)
		}
		if len(entities) != 0 {
			t.Errorf("len(entities) = %d, want %d", len(entities), 0)
		}
	})

	t.Run("no kind", func(t *testing.T) {
		content := `
apiVersion: swcat/v1alpha1
metadata:
  name: no-kind
`
		tmpfile := writeTempFile(t, "no-kind.yaml", content)
		defer os.Remove(tmpfile)

		_, err := ReadEntities(tmpfile)
		if err == nil {
			t.Errorf("ReadEntities() error = %v, wantErr %v", err, true)
		}
	})

	t.Run("invalid kind", func(t *testing.T) {
		content := `
apiVersion: swcat/v1alpha1
kind: InvalidKind
metadata:
  name: invalid-kind
`
		tmpfile := writeTempFile(t, "invalid-kind.yaml", content)
		defer os.Remove(tmpfile)

		_, err := ReadEntities(tmpfile)
		if err == nil {
			t.Errorf("ReadEntities() error = %v, wantErr %v", err, true)
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := ReadEntities("non-existent-file.yaml")
		if err == nil {
			t.Errorf("ReadEntities() error = %v, wantErr %v", err, true)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		content := `
invalid: yaml: here
`
		tmpfile := writeTempFile(t, "invalid.yaml", content)
		defer os.Remove(tmpfile)

		_, err := ReadEntities(tmpfile)
		if err == nil {
			t.Errorf("ReadEntities() error = %v, wantErr %v", err, true)
		}
	})
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	tmpfile := filepath.Join(dir, name)
	err := os.WriteFile(tmpfile, []byte(content), 0666)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	return tmpfile
}

func TestReadEntityFromString(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		content := `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: my-component
spec:
  type: service
  owner: my-group
  lifecycle: experimental
`
		entity, err := ReadEntityFromString(content)
		if err != nil {
			t.Fatalf("ReadEntityFromString() error = %v, wantErr %v", err, false)
		}
		if entity == nil {
			t.Fatal("entity is nil")
		}
		if component, ok := entity.(*api.Component); !ok {
			t.Fatalf("entity is not a *Component")
		} else {
			if component.Metadata.Name != "my-component" {
				t.Errorf("component.Metadata.Name = %s, want %s", component.Metadata.Name, "my-component")
			}
			if component.Spec.Type != "service" {
				t.Errorf("component.Spec.Type = %s, want %s", component.Spec.Type, "service")
			}
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		content := `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: my-component
spec:
  type: service
  owner: my-group
  lifecycle: experimental
  foo: bar
`
		_, err := ReadEntityFromString(content)
		if err == nil {
			t.Errorf("ReadEntityFromString() error = %v, wantErr %v", err, true)
		}
	})
}
