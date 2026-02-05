package store

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/dnswlt/swcat/internal/gitclient"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-cmp/cmp"
)

// createTestRepo initializes a git repo in a temp dir with some dummy content
// and returns the path to that directory.
//
// This is duplicated from internal/gitclient/gitclient_test.go because we cannot easily share test helpers across packages
// without creating a testutil package, which might be overkill for now.
//
// Structure:
// v1.0.0 (tag)
//   - catalog.yaml ("v1 content")
//
// v2.0.0 (tag)
//   - catalog.yaml ("v2 content")
//   - nested/service.yaml ("service content")
//
// feature/test-branch (branch)
//   - branch-file.txt ("branch content")
func createTestRepo(t *testing.T) string {
	t.Helper()

	// Create Temp Directory
	dir := t.TempDir()

	// Initialize Git Repo
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Helper to commit
	commit := func(msg string) {
		_, err := w.Add(".")
		if err != nil {
			t.Fatalf("Failed to add files: %v", err)
		}
		_, err = w.Commit(msg, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}
	}

	// Create v1.0.0 state
	if err := os.WriteFile(filepath.Join(dir, "catalog.yaml"), []byte("v1 content"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	commit("Initial commit")

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Failed to get HEAD: %v", err)
	}
	if _, err := repo.CreateTag("v1.0.0", head.Hash(), nil); err != nil {
		t.Fatalf("Failed to create tag v1.0.0: %v", err)
	}

	// Create v2.0.0 state
	if err := os.WriteFile(filepath.Join(dir, "catalog.yaml"), []byte("v2 content"), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "service.yaml"), []byte("service content"), 0644); err != nil {
		t.Fatalf("Failed to write nested file: %v", err)
	}
	commit("Second commit")

	head, err = repo.Head()
	if err != nil {
		t.Fatalf("Failed to get HEAD: %v", err)
	}
	if _, err := repo.CreateTag("v2.0.0", head.Hash(), nil); err != nil {
		t.Fatalf("Failed to create tag v2.0.0: %v", err)
	}

	// Create a branch
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature/test-branch"),
		Create: true,
	})
	if err != nil {
		t.Fatalf("Failed to checkout branch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "branch-file.txt"), []byte("branch content"), 0644); err != nil {
		t.Fatalf("Failed to write branch file: %v", err)
	}
	commit("Branch commit")

	// Switch back to master so it's the HEAD when cloned
	err = w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("master"),
	})
	if err != nil {
		t.Fatalf("Failed to checkout master: %v", err)
	}

	return dir
}

// createRepoFromTestdata initializes a git repo with the content of sourceFile
func createRepoFromTestdata(t *testing.T, sourceFile string) string {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}
	w, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Failed to get worktree: %v", err)
	}

	// Read testdata and write to repo
	initialData, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatalf("Failed to read testdata %q: %v", sourceFile, err)
	}
	fileName := filepath.Base(sourceFile)
	if err := os.WriteFile(filepath.Join(dir, fileName), initialData, 0644); err != nil {
		t.Fatalf("Failed to write %s: %v", fileName, err)
	}

	_, err = w.Add(".")
	if err != nil {
		t.Fatalf("Failed to add files: %v", err)
	}
	_, err = w.Commit("Add "+fileName, &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	})
	if err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	return dir
}

func TestGitSource(t *testing.T) {
	repoPath := createTestRepo(t)

	client, err := gitclient.New(repoPath, nil)
	if err != nil {
		t.Fatalf("gitclient.New failed: %v", err)
	}

	gs := NewGitSource(client, "master", "")

	t.Run("DefaultRef", func(t *testing.T) {
		if got := gs.DefaultRef(); got != "master" {
			t.Errorf("DefaultRef() = %q, want %q", got, "master")
		}
	})

	t.Run("ListReferences", func(t *testing.T) {
		refs, err := gs.ListReferences()
		if err != nil {
			t.Fatalf("ListReferences() failed: %v", err)
		}
		expected := []string{"feature/test-branch", "master", "v1.0.0", "v2.0.0"}
		// Use manual comparison since we imported slices but not cmp
		if len(refs) != len(expected) {
			t.Errorf("ListReferences() got %v, want %v", refs, expected)
		}
		for _, e := range expected {
			if !slices.Contains(refs, e) {
				t.Errorf("ListReferences() missing %q", e)
			}
		}
	})

	t.Run("Store_DefaultRef", func(t *testing.T) {
		// Empty ref should default to master
		st, err := gs.Store("")
		if err != nil {
			t.Fatalf("Store(\"\") failed: %v", err)
		}
		// 'master' in our repo doesn't have 'branch-file.txt' (that's on feature/test-branch)
		// but does have catalog.yaml (v2 content)
		content, err := st.ReadFile("catalog.yaml")
		if err != nil {
			t.Fatalf("ReadFile(catalog.yaml) failed: %v", err)
		}
		if string(content) != "v2 content" {
			t.Errorf("master: expected 'v2 content', got %q", string(content))
		}
	})

	t.Run("Store_SpecificRef", func(t *testing.T) {
		st, err := gs.Store("v1.0.0")
		if err != nil {
			t.Fatalf("Store(\"v1.0.0\") failed: %v", err)
		}
		content, err := st.ReadFile("catalog.yaml")
		if err != nil {
			t.Fatalf("ReadFile(catalog.yaml) failed: %v", err)
		}
		if string(content) != "v1 content" {
			t.Errorf("v1.0.0: expected 'v1 content', got %q", string(content))
		}
	})

	t.Run("Store_InvalidRef", func(t *testing.T) {
		_, err := gs.Store("non-existent")
		if err != ErrNoSuchRef {
			t.Errorf("Store(\"non-existent\") error = %v, want ErrNoSuchRef", err)
		}
	})

	t.Run("GitStore_ListFiles", func(t *testing.T) {
		st, err := gs.Store("v2.0.0")
		if err != nil {
			t.Fatalf("Store(\"v2.0.0\") failed: %v", err)
		}

		// List root
		files, err := st.ListFiles(".")
		if err != nil {
			t.Fatalf("ListFiles(.) failed: %v", err)
		}
		expected := []string{"catalog.yaml", "nested/service.yaml"}
		if len(files) != len(expected) {
			t.Errorf("ListFiles(.) got %v, want %v", files, expected)
		}
		for _, e := range expected {
			if !slices.Contains(files, e) {
				t.Errorf("ListFiles(.) missing %q", e)
			}
		}

		// List subdir
		subFiles, err := st.ListFiles("nested")
		if err != nil {
			t.Fatalf("ListFiles(nested) failed: %v", err)
		}
		// Note: The GitStore implementation joins the dir with the filename:
		// result[i] = filepath.Join(dir, f)
		// So checking "nested" should return "nested/service.yaml"
		if len(subFiles) != 1 || subFiles[0] != "nested/service.yaml" {
			t.Errorf("ListFiles(nested) got %v, want [nested/service.yaml]", subFiles)
		}
	})

	t.Run("GitStore_WriteFile", func(t *testing.T) {
		st, err := gs.Store("master")
		if err != nil {
			t.Fatalf("Store(\"master\") failed: %v", err)
		}
		err = st.WriteFile("any.txt", []byte("foo"))
		if err != ErrReadOnly {
			t.Errorf("WriteFile() error = %v, want ErrReadOnly", err)
		}
	})
}

func TestGitSource_WithRootDir(t *testing.T) {
	repoPath := createTestRepo(t)

	client, err := gitclient.New(repoPath, nil)
	if err != nil {
		t.Fatalf("gitclient.New failed: %v", err)
	}

	gs := NewGitSource(client, "master", "nested")

	t.Run("GitStore_ReadFile_WithRootDir", func(t *testing.T) {
		st, err := gs.Store("v2.0.0")
		if err != nil {
			t.Fatalf("gs.Store failed: %v", err)
		}
		// Read 'service.yaml' from 'nested' folder via rooted store
		content, err := st.ReadFile("service.yaml")
		if err != nil {
			t.Fatalf("ReadFile(service.yaml) failed: %v", err)
		}
		if string(content) != "service content" {
			t.Errorf("master: expected 'service content', got %q", string(content))
		}
	})

	t.Run("GitStore_ListFiles_WithRootDir", func(t *testing.T) {
		st, err := gs.Store("v2.0.0")
		if err != nil {
			t.Fatalf("gs.Store failed: %v", err)
		}
		// List '.' in 'nested' folder via rooted store
		files, err := st.ListFiles(".")
		if err != nil {
			t.Fatalf("ListFiles(.) failed: %v", err)
		}
		if len(files) != 1 || files[0] != "service.yaml" {
			t.Errorf("ListFiles(.) got %v, want [service.yaml]", files)
		}
	})
}

func TestGitSource_ReadEntities(t *testing.T) {
	// 1. Setup repo
	dir := createRepoFromTestdata(t, "../../testdata/test1/catalog/catalog.yml")

	// 2. Create GitSource
	client, err := gitclient.New(dir, nil)
	if err != nil {
		t.Fatalf("gitclient.New failed: %v", err)
	}
	gs := NewGitSource(client, "master", "")

	// 3. Get Store
	st, err := gs.Store("master")
	if err != nil {
		t.Fatalf("Store(\"master\") failed: %v", err)
	}

	// 4. ReadEntities
	entities, err := ReadEntities(st, "catalog.yml")
	if err != nil {
		t.Fatalf("ReadEntities failed: %v", err)
	}

	// 5. Verify count
	if len(entities) < 3 {
		t.Errorf("Expected more entities, got %d", len(entities))
	}

	// 6. Verify specific entity
	var foundDomain bool
	for _, e := range entities {
		if e.GetKind() == "Domain" && e.GetMetadata().Name == "test-domain" {
			foundDomain = true
			break
		}
	}
	if !foundDomain {
		t.Error("Expected to find Domain:test-domain")
	}
}

func TestGitSource_ReadEntities_Comparison(t *testing.T) {
	sourceFile := "../../testdata/test1/catalog/catalog.yml"
	fileName := filepath.Base(sourceFile)

	// 1. Read from DiskStore (reference)
	// We create a temp disk store to be consistent with how we expect to read entities
	diskDir := t.TempDir()
	data, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatalf("Failed to read source file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(diskDir, fileName), data, 0644); err != nil {
		t.Fatalf("Failed to write to disk store: %v", err)
	}
	ds := NewDiskStore(diskDir)
	diskEntities, err := ReadEntities(ds, fileName)
	if err != nil {
		t.Fatalf("DiskStore ReadEntities failed: %v", err)
	}

	// 2. Git Store setup
	gitDir := createRepoFromTestdata(t, sourceFile)
	client, err := gitclient.New(gitDir, nil)
	if err != nil {
		t.Fatalf("gitclient.New failed: %v", err)
	}
	gs := NewGitSource(client, "master", "")
	st, err := gs.Store("master")
	if err != nil {
		t.Fatalf("gs.Store failed: %v", err)
	}

	gitEntities, err := ReadEntities(st, fileName)
	if err != nil {
		t.Fatalf("GitStore ReadEntities failed: %v", err)
	}

	// 3. Compare
	if diff := cmp.Diff(diskEntities, gitEntities); diff != "" {
		t.Errorf("Entities mismatch (-disk +git):\n%s", diff)
	}
}
