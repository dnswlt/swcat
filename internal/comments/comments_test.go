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

func TestGetOpenComments(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comments-open-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fileStore, _ := NewFileStore(tmpDir)
	// Test on caching store as it wraps file store, so we test both effectively if caching logical holds up
	// But let's test specifically FileStore first to be sure

	stores := map[string]Store{
		"FileStore":    fileStore,
		"CachingStore": NewCachingStore(fileStore),
	}

	for name, s := range stores {
		t.Run(name, func(t *testing.T) {
			entityRef := "component:default/service-" + name
			c1 := Comment{Author: "A", Text: "Open", CreatedAt: time.Now()}
			c2 := Comment{Author: "B", Text: "Resolved", CreatedAt: time.Now(), Resolved: true}

			s.AddComment(entityRef, c1)
			s.AddComment(entityRef, c2)

			open, err := s.GetOpenComments(entityRef)
			if err != nil {
				t.Fatalf("GetOpenComments failed: %v", err)
			}
			if len(open) != 1 {
				t.Errorf("Expected 1 open comment, got %d", len(open))
			}
			if open[0].Text != "Open" {
				t.Errorf("Expected comment 'Open', got '%s'", open[0].Text)
			}
		})
	}
}
