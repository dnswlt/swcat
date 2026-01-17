package store

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dnswlt/swcat/internal/api"
)

func TestInsertOrReplaceEntity(t *testing.T) {
	// 1. Setup: Load initial data
	initialData, err := os.ReadFile("../../testdata/catalog/catalog.yml")
	if err != nil {
		t.Fatalf("Failed to read testdata: %v", err)
	}

	// 2. Setup: Create temp DiskStore
	tmpDir := t.TempDir()
	store := NewDiskStore(tmpDir)
	filename := "catalog.yml"
	if err := store.WriteFile(filename, initialData); err != nil {
		t.Fatalf("Failed to write initial data: %v", err)
	}

	// Helper to find entity by name in a slice
	findEntity := func(entities []api.Entity, kind, name string) api.Entity {
		for _, e := range entities {
			if e.GetKind() == kind && e.GetMetadata().Name == name {
				return e
			}
		}
		return nil
	}

	// Calculate initial count
	initialEntities, err := ReadEntities(store, filename)
	if err != nil {
		t.Fatalf("Failed to read initial entities: %v", err)
	}
	initialCount := len(initialEntities)

	t.Run("InsertNewEntity", func(t *testing.T) {
		newEntityYAML := `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: new-component
spec:
  type: service
  owner: test-group
  system: test-system
  lifecycle: experimental
`
		newEntity, err := api.NewEntityFromString(newEntityYAML)
		if err != nil {
			t.Fatalf("Failed to parse new entity: %v", err)
		}

		if err := InsertOrReplaceEntity(store, filename, newEntity); err != nil {
			t.Fatalf("InsertOrReplaceEntity failed: %v", err)
		}

		// Verify
		entities, err := ReadEntities(store, filename)
		if err != nil {
			t.Fatalf("ReadEntities failed: %v", err)
		}

		// Should have original entities + 1
		if len(entities) != initialCount+1 {
			t.Errorf("Expected %d entities, got %d", initialCount+1, len(entities))
		}

		inserted := findEntity(entities, "Component", "new-component")
		if inserted == nil {
			t.Fatal("Inserted entity not found")
		}
		comp := inserted.(*api.Component)
		if comp.Spec.Lifecycle != "experimental" {
			t.Errorf("Expected lifecycle 'experimental', got '%s'", comp.Spec.Lifecycle)
		}
	})

	t.Run("ReplaceExistingEntity", func(t *testing.T) {
		// We will modify 'test-component' which is already in the file.
		// NOTE: In the previous step we added 'new-component'. The file now has initialCount + 1 entities.

		replaceEntityYAML := `
apiVersion: swcat/v1alpha1
kind: Component
metadata:
  name: test-component
  title: Modified Component
  labels:
    foobar.dev/language: go
spec:
  type: service
  lifecycle: deprecated
  owner: test-group
  system: test-system
`
		replaceEntity, err := api.NewEntityFromString(replaceEntityYAML)
		if err != nil {
			t.Fatalf("Failed to parse replacement entity: %v", err)
		}

		if err := InsertOrReplaceEntity(store, filename, replaceEntity); err != nil {
			t.Fatalf("InsertOrReplaceEntity failed: %v", err)
		}

		// Verify
		entities, err := ReadEntities(store, filename)
		if err != nil {
			t.Fatalf("ReadEntities failed: %v", err)
		}

		if len(entities) != initialCount+1 {
			t.Errorf("Expected %d entities (count should not change on replace), got %d", initialCount+1, len(entities))
		}

		updated := findEntity(entities, "Component", "test-component")
		if updated == nil {
			t.Fatal("Updated entity not found")
		}
		comp := updated.(*api.Component)
		if comp.Spec.Lifecycle != "deprecated" {
			t.Errorf("Expected lifecycle 'deprecated', got '%s'", comp.Spec.Lifecycle)
		}
		if comp.Metadata.Title != "Modified Component" {
			t.Errorf("Expected title 'Modified Component', got '%s'", comp.Metadata.Title)
		}

		// Verify another entity is untouched
		domain := findEntity(entities, "Domain", "test-domain")
		if domain == nil {
			t.Error("Original domain entity is missing")
		}
	})
}

func TestDeleteEntity(t *testing.T) {
	// 1. Setup: Load initial data
	initialData, err := os.ReadFile("../../testdata/catalog/catalog.yml")
	if err != nil {
		t.Fatalf("Failed to read testdata: %v", err)
	}

	// 2. Setup: Create temp DiskStore
	tmpDir := t.TempDir()
	store := NewDiskStore(tmpDir)
	filename := "catalog.yml"
	if err := store.WriteFile(filename, initialData); err != nil {
		t.Fatalf("Failed to write initial data: %v", err)
	}

	// Helper to find entity by name in a slice
	findEntity := func(entities []api.Entity, kind, name string) api.Entity {
		for _, e := range entities {
			if e.GetKind() == kind && e.GetMetadata().Name == name {
				return e
			}
		}
		return nil
	}

	// Calculate initial count
	initialEntities, err := ReadEntities(store, filename)
	if err != nil {
		t.Fatalf("Failed to read initial entities: %v", err)
	}
	initialCount := len(initialEntities)

	t.Run("DeleteExistingEntity", func(t *testing.T) {
		// Identify entity to delete: Component:test-component
		target := findEntity(initialEntities, "Component", "test-component")
		if target == nil {
			t.Fatal("Target entity 'test-component' not found in initial data")
		}

		if err := DeleteEntity(store, filename, target.GetRef()); err != nil {
			t.Fatalf("DeleteEntity failed: %v", err)
		}

		// Verify
		entities, err := ReadEntities(store, filename)
		if err != nil {
			t.Fatalf("ReadEntities failed: %v", err)
		}

		if len(entities) != initialCount-1 {
			t.Errorf("Expected %d entities, got %d", initialCount-1, len(entities))
		}

		deleted := findEntity(entities, "Component", "test-component")
		if deleted != nil {
			t.Error("Entity 'test-component' should have been deleted")
		}

		// Verify another entity is untouched
		domain := findEntity(entities, "Domain", "test-domain")
		if domain == nil {
			t.Error("Other entity 'test-domain' is missing")
		}
	})

	t.Run("DeleteNonExistentEntity", func(t *testing.T) {
		// Try to delete an entity that doesn't exist
		ref := &api.Ref{Kind: "Component", Name: "non-existent"}
		err := DeleteEntity(store, filename, ref)
		if err == nil {
			t.Error("DeleteEntity succeeded for non-existent entity, expected error")
		}
	})
}

func TestDiskStore(t *testing.T) {
	tmpDir := t.TempDir()
	ds := NewDiskStore(tmpDir)

	t.Run("Ref", func(t *testing.T) {
		if err := ds.Refresh(); err != nil {
			t.Errorf("Refresh() failed: %v", err)
		}

		s, err := ds.Store("")
		if err != nil {
			t.Fatalf("Store(\"\") failed: %v", err)
		}
		if s != ds {
			t.Error("Store(\"\") returned new instance, expected same *DiskStore")
		}

		if _, err := ds.Store("main"); err == nil {
			t.Error("Store(\"main\") succeeded, expected error")
		}
	})

	t.Run("FileOps", func(t *testing.T) {
		fname := "test.txt"
		content := []byte("hello")
		if err := ds.WriteFile(fname, content); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		got, err := ds.ReadFile(fname)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(got) != string(content) {
			t.Errorf("ReadFile got %q, want %q", got, content)
		}
	})

	t.Run("PathTraversal", func(t *testing.T) {
		// Attempt to write outside root
		err := ds.WriteFile("../escaped.txt", []byte("bad"))
		if err == nil {
			t.Error("WriteFile(../...) succeeded, expected error")
		}

		// Attempt to read outside root
		// We try to create a file outside. Note that t.TempDir() usually is in a subdir of /tmp,
		// so ".." should be writable if we are lucky, but we might not want to pollute /tmp.
		// Instead, we can create a nested structure.
		nestedDir := filepath.Join(tmpDir, "nested")
		os.Mkdir(nestedDir, 0755)
		nestedDS := NewDiskStore(nestedDir)

		// Create file in tmpDir (which is parent of nestedDir)
		parentFile := "parent.txt"
		ds.WriteFile(parentFile, []byte("parent"))

		// Try to read parent from nestedDS using ..
		_, err = nestedDS.ReadFile("../parent.txt")
		if err == nil {
			t.Error("ReadFile(../...) succeeded, expected error")
		}
	})

	t.Run("ListFiles", func(t *testing.T) {
		// Prepare a clean store for listing
		listDir := t.TempDir()
		lds := NewDiskStore(listDir)

		files := []string{
			"root.txt",
			"sub/a.txt",
			"sub/nested/b.txt",
		}
		for _, f := range files {
			if err := os.MkdirAll(filepath.Join(listDir, filepath.Dir(f)), 0755); err != nil {
				t.Fatal(err)
			}
			if err := lds.WriteFile(f, []byte("content")); err != nil {
				t.Fatal(err)
			}
		}

		// List all
		got, err := lds.ListFiles(".")
		if err != nil {
			t.Fatalf("ListFiles(.) failed: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("ListFiles(.) returned %d files, want 3. Got: %v", len(got), got)
		}

		// Check containment
		for _, want := range files {
			if !slices.Contains(got, want) {
				t.Errorf("ListFiles(.) missing %q", want)
			}
		}

		// List subdir
		gotSub, err := lds.ListFiles("sub")
		if err != nil {
			t.Fatalf("ListFiles(sub) failed: %v", err)
		}
		if len(gotSub) != 2 {
			t.Errorf("ListFiles(sub) returned %d files, want 2. Got: %v", len(gotSub), gotSub)
		}
		// Expected: sub/a.txt, sub/nested/b.txt (relative to root)
		if !slices.Contains(gotSub, "sub/a.txt") {
			t.Errorf("ListFiles(sub) missing sub/a.txt")
		}
	})

	t.Run("CatalogFiles", func(t *testing.T) {
		// Uses ListFiles but filters .yml
		catDir := t.TempDir()
		cds := NewDiskStore(catDir)
		cds.WriteFile("a.yml", nil)
		cds.WriteFile("b.yaml", nil) // Not .yml
		cds.WriteFile("c.txt", nil)
		os.Mkdir(filepath.Join(catDir, "sub"), 0755)
		cds.WriteFile("sub/d.yml", nil)

		files, err := CatalogFiles(cds, ".")
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 2 {
			t.Errorf("CatalogFiles returned %v, want [a.yml sub/d.yml]", files)
		}
		if !slices.Contains(files, "a.yml") || !slices.Contains(files, "sub/d.yml") {
			t.Errorf("CatalogFiles missing expected files")
		}
	})
}

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
		st, tmpfile := writeTempFile(t, "entities.yaml", content)
		defer os.Remove(tmpfile)

		entities, err := ReadEntities(st, filepath.Base(tmpfile))
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
		st, tmpfile := writeTempFile(t, "empty.yaml", "")
		defer os.Remove(tmpfile)

		entities, err := ReadEntities(st, filepath.Base(tmpfile))
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
		st, tmpfile := writeTempFile(t, "no-kind.yaml", content)
		defer os.Remove(tmpfile)

		_, err := ReadEntities(st, filepath.Base(tmpfile))
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
		st, tmpfile := writeTempFile(t, "invalid-kind.yaml", content)
		defer os.Remove(tmpfile)

		_, err := ReadEntities(st, filepath.Base(tmpfile))
		if err == nil {
			t.Errorf("ReadEntities() error = %v, wantErr %v", err, true)
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := ReadEntities(NewDiskStore("."), "non-existent-file.yaml")
		if err == nil {
			t.Errorf("ReadEntities() error = %v, wantErr %v", err, true)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		content := `
invalid: yaml: here
`
		st, tmpfile := writeTempFile(t, "invalid.yaml", content)
		defer os.Remove(tmpfile)

		_, err := ReadEntities(st, filepath.Base(tmpfile))
		if err == nil {
			t.Errorf("ReadEntities() error = %v, wantErr %v", err, true)
		}
	})
}

func writeTempFile(t *testing.T, name, content string) (Store, string) {
	t.Helper()
	dir := t.TempDir()
	tmpfile := filepath.Join(dir, name)
	err := os.WriteFile(tmpfile, []byte(content), 0666)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}
	return NewDiskStore(dir), tmpfile
}
