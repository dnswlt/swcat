package gitclient

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-cmp/cmp"
)

// createTestRepo initializes a git repo in a temp dir with some dummy content
// and returns the path to that directory.
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

func TestClient(t *testing.T) {
	repoPath := createTestRepo(t)

	// Initialize the Loader pointing to the local temp repo
	loader, err := New(repoPath, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	t.Run("ListReferences", func(t *testing.T) {
		refs, err := loader.ListReferences()
		if err != nil {
			t.Fatalf("ListReferences failed: %v", err)
		}

		slices.Sort(refs)

		// ListReferences returns branches (master, feature/test-branch) and tags.
		expected := []string{"feature/test-branch", "master", "v1.0.0", "v2.0.0"}
		if diff := cmp.Diff(expected, refs); diff != "" {
			t.Errorf("ListReferences mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("ReadFile v1.0.0", func(t *testing.T) {
		content, err := loader.ReadFile("v1.0.0", "catalog.yaml")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "v1 content" {
			t.Errorf("Expected 'v1 content', got %q", string(content))
		}
	})

	t.Run("ReadFile Branch", func(t *testing.T) {
		content, err := loader.ReadFile("feature/test-branch", "branch-file.txt")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "branch content" {
			t.Errorf("Expected 'branch content', got %q", string(content))
		}
	})

	t.Run("ReadFile v2.0.0", func(t *testing.T) {
		content, err := loader.ReadFile("v2.0.0", "catalog.yaml")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "v2 content" {
			t.Errorf("Expected 'v2 content', got %q", string(content))
		}
	})

	t.Run("ReadFile Nested", func(t *testing.T) {
		content, err := loader.ReadFile("v2.0.0", "nested/service.yaml")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "service content" {
			t.Errorf("Expected 'service content', got %q", string(content))
		}
	})

	t.Run("ListFilesRecursive", func(t *testing.T) {
		files, err := loader.ListFilesRecursive("v2.0.0", "")
		if err != nil {
			t.Fatalf("ListFilesRecursive failed: %v", err)
		}
		sort.Strings(files)

		expected := []string{"catalog.yaml", "nested/service.yaml"}
		sort.Strings(expected)

		if diff := cmp.Diff(expected, files); diff != "" {
			t.Errorf("ListFilesRecursive mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("ListFilesRecursive Subdir", func(t *testing.T) {
		files, err := loader.ListFilesRecursive("v2.0.0", "nested")
		if err != nil {
			t.Fatalf("ListFilesRecursive failed: %v", err)
		}

		// Note: The implementation of ListFilesRecursive returns paths relative to the *targetTree*
		expected := []string{"service.yaml"}

		if diff := cmp.Diff(expected, files); diff != "" {
			t.Errorf("ListFilesRecursive (subdir) mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestCreateBranch(t *testing.T) {
	repoPath := createTestRepo(t)
	client, err := New(repoPath, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := client.CreateBranch("edit/test", "master"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	refs, err := client.ListReferences()
	if err != nil {
		t.Fatalf("ListReferences failed: %v", err)
	}
	if !slices.Contains(refs, "edit/test") {
		t.Errorf("Branch edit/test not found in refs: %v", refs)
	}

	// The new branch should have the same content as master.
	content, err := client.ReadFile("edit/test", "catalog.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "v2 content" {
		t.Errorf("Expected 'v2 content', got %q", string(content))
	}
}

func TestCommitFile(t *testing.T) {
	repoPath := createTestRepo(t)
	client, err := New(repoPath, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := client.CreateBranch("edit/write-test", "master"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	author := Author{Name: "Test", Email: "test@example.com"}

	// Overwrite an existing file.
	if err := client.CommitFile("edit/write-test", "catalog.yaml", []byte("modified content"), author, "update catalog"); err != nil {
		t.Fatalf("CommitFile failed: %v", err)
	}

	content, err := client.ReadFile("edit/write-test", "catalog.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "modified content" {
		t.Errorf("Expected 'modified content', got %q", string(content))
	}

	// The original branch should be unaffected.
	original, err := client.ReadFile("master", "catalog.yaml")
	if err != nil {
		t.Fatalf("ReadFile master failed: %v", err)
	}
	if string(original) != "v2 content" {
		t.Errorf("Expected master to still have 'v2 content', got %q", string(original))
	}
}

func TestCommitFileNested(t *testing.T) {
	repoPath := createTestRepo(t)
	client, err := New(repoPath, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := client.CreateBranch("edit/nested-test", "master"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	author := Author{Name: "Test", Email: "test@example.com"}

	// Write to an existing nested path.
	if err := client.CommitFile("edit/nested-test", "nested/service.yaml", []byte("updated service"), author, "update service"); err != nil {
		t.Fatalf("CommitFile nested failed: %v", err)
	}

	content, err := client.ReadFile("edit/nested-test", "nested/service.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "updated service" {
		t.Errorf("Expected 'updated service', got %q", string(content))
	}

	// Write to a new nested path that doesn't exist yet.
	if err := client.CommitFile("edit/nested-test", "new/dir/file.txt", []byte("new file"), author, "add new file"); err != nil {
		t.Fatalf("CommitFile new path failed: %v", err)
	}

	content, err = client.ReadFile("edit/nested-test", "new/dir/file.txt")
	if err != nil {
		t.Fatalf("ReadFile new path failed: %v", err)
	}
	if string(content) != "new file" {
		t.Errorf("Expected 'new file', got %q", string(content))
	}

	// Existing files should still be readable.
	content, err = client.ReadFile("edit/nested-test", "catalog.yaml")
	if err != nil {
		t.Fatalf("ReadFile catalog failed: %v", err)
	}
	if string(content) != "v2 content" {
		t.Errorf("Expected 'v2 content', got %q", string(content))
	}
}

func TestDeleteBranch(t *testing.T) {
	repoPath := createTestRepo(t)
	client, err := New(repoPath, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := client.CreateBranch("edit/to-delete", "master"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	if err := client.DeleteBranch("edit/to-delete"); err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}

	refs, err := client.ListReferences()
	if err != nil {
		t.Fatalf("ListReferences failed: %v", err)
	}
	if slices.Contains(refs, "edit/to-delete") {
		t.Errorf("Branch edit/to-delete should have been deleted, but found in refs: %v", refs)
	}
}
