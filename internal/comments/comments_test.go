package comments

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comments-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewFileStore(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	entityRef := "component:default/my-service"
	comment := Comment{
		Author:    "Alice",
		Text:      "Test comment",
		CreatedAt: time.Now().Round(time.Second), // Round to avoid small diffs in JSON
	}

	if err := store.AddComment(entityRef, comment); err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}

	comments, err := store.GetComments(entityRef)
	if err != nil {
		t.Fatalf("GetComments failed: %v", err)
	}

	if len(comments) != 1 {
		t.Fatalf("Expected 1 comment, got %d", len(comments))
	}

	if comments[0].Author != comment.Author || comments[0].Text != comment.Text {
		t.Errorf("Comment mismatch. Got %+v, want %+v", comments[0], comment)
	}
}

func TestCachingStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comments-cache-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fileStore, _ := NewFileStore(tmpDir)
	cacheStore := NewCachingStore(fileStore)

	entityRef := "component:default/my-service"
	comment := Comment{
		Author:    "Bob",
		Text:      "Cache test",
		CreatedAt: time.Now().Round(time.Second),
	}

	// Add to cache
	if err := cacheStore.AddComment(entityRef, comment); err != nil {
		t.Fatal(err)
	}

	// Manually delete the file to verify it's served from cache
	safeRef := "component_default_my-service.json"
	if err := os.Remove(filepath.Join(tmpDir, safeRef)); err != nil {
		t.Fatal(err)
	}

	comments, err := cacheStore.GetComments(entityRef)
	if err != nil {
		t.Fatal(err)
	}

	if len(comments) != 1 {
		t.Fatalf("Expected 1 comment from cache, got %d", len(comments))
	}
}
