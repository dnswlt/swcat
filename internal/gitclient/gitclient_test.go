package gitclient

import (
	"fmt"
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

// testCommit describes a single commit to create in a test repo.
type testCommit struct {
	// Files maps file paths to their content.
	Files map[string]string
	// Tag, if non-empty, tags this commit with the given name.
	Tag string
	// Branch, if non-empty, creates and checks out this branch before committing.
	// After all commits are processed, the worktree switches back to master.
	Branch string
}

// createTestRepo initializes a git repo in a temp dir from the given commit
// sequence and returns the path to that directory. Files accumulate across
// commits (just like a real repo), so later commits only need to specify
// new or changed files.
func createTestRepo(t *testing.T, commits []testCommit) string {
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

	onMaster := true
	for i, c := range commits {
		if c.Branch != "" {
			err := w.Checkout(&git.CheckoutOptions{
				Branch: plumbing.NewBranchReferenceName(c.Branch),
				Create: true,
			})
			if err != nil {
				t.Fatalf("Failed to checkout branch %s: %v", c.Branch, err)
			}
			onMaster = false
		}

		for path, content := range c.Files {
			full := filepath.Join(dir, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				t.Fatalf("Failed to create dir for %s: %v", path, err)
			}
			if err := os.WriteFile(full, []byte(content), 0644); err != nil {
				t.Fatalf("Failed to write %s: %v", path, err)
			}
		}

		if _, err := w.Add("."); err != nil {
			t.Fatalf("Failed to add files: %v", err)
		}
		_, err := w.Commit(fmt.Sprintf("commit %d", i), &git.CommitOptions{
			Author: &object.Signature{
				Name:  "Test User",
				Email: "test@example.com",
				When:  time.Now(),
			},
		})
		if err != nil {
			t.Fatalf("Failed to commit: %v", err)
		}

		if c.Tag != "" {
			head, err := repo.Head()
			if err != nil {
				t.Fatalf("Failed to get HEAD: %v", err)
			}
			if _, err := repo.CreateTag(c.Tag, head.Hash(), nil); err != nil {
				t.Fatalf("Failed to create tag %s: %v", c.Tag, err)
			}
		}
	}

	if !onMaster {
		err := w.Checkout(&git.CheckoutOptions{
			Branch: plumbing.NewBranchReferenceName("master"),
		})
		if err != nil {
			t.Fatalf("Failed to checkout master: %v", err)
		}
	}

	return dir
}

func TestClient(t *testing.T) {
	repoPath := createTestRepo(t, []testCommit{
		{
			Files: map[string]string{"catalog.yaml": "v1 content"},
			Tag:   "v1.0.0",
		},
		{
			Files: map[string]string{"catalog.yaml": "v2 content", "nested/service.yaml": "service content"},
			Tag:   "v2.0.0",
		},
		{
			Branch: "feature/test-branch",
			Files:  map[string]string{"branch-file.txt": "branch content"},
		},
	})

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
	repoPath := createTestRepo(t, []testCommit{
		{Files: map[string]string{"catalog.yaml": "v2 content"}},
	})
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
	repoPath := createTestRepo(t, []testCommit{
		{Files: map[string]string{"catalog.yaml": "v2 content"}},
	})
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
	repoPath := createTestRepo(t, []testCommit{
		{Files: map[string]string{"catalog.yaml": "v2 content", "nested/service.yaml": "service content"}},
	})
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

func TestCommitFileTwice(t *testing.T) {
	repoPath := createTestRepo(t, []testCommit{
		{Files: map[string]string{"catalog.yaml": "original"}},
	})
	client, err := New(repoPath, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := client.CreateBranch("edit/twice", "master"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	author := Author{Name: "Test", Email: "test@example.com"}

	if err := client.CommitFile("edit/twice", "catalog.yaml", []byte("first update"), author, "first"); err != nil {
		t.Fatalf("CommitFile (first) failed: %v", err)
	}
	if err := client.CommitFile("edit/twice", "catalog.yaml", []byte("second update"), author, "second"); err != nil {
		t.Fatalf("CommitFile (second) failed: %v", err)
	}

	content, err := client.ReadFile("edit/twice", "catalog.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "second update" {
		t.Errorf("Expected 'second update', got %q", string(content))
	}

	// Master should be unchanged.
	original, err := client.ReadFile("master", "catalog.yaml")
	if err != nil {
		t.Fatalf("ReadFile master failed: %v", err)
	}
	if string(original) != "original" {
		t.Errorf("Expected master to still have 'original', got %q", string(original))
	}
}

func TestDeleteBranch(t *testing.T) {
	repoPath := createTestRepo(t, []testCommit{
		{Files: map[string]string{"catalog.yaml": "content"}},
	})
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

func TestCommitFileSiblings(t *testing.T) {
	repoPath := createTestRepo(t, []testCommit{
		{Files: map[string]string{
			"a.yaml":          "a original",
			"b.yaml":          "b original",
			"nested/one.yaml": "one original",
			"nested/two.yaml": "two original",
		}},
	})
	client, err := New(repoPath, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := client.CreateBranch("edit/siblings", "master"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	author := Author{Name: "Test", Email: "test@example.com"}

	// Commit to a.yaml, then to nested/one.yaml. All siblings must survive.
	if err := client.CommitFile("edit/siblings", "a.yaml", []byte("a updated"), author, "update a"); err != nil {
		t.Fatalf("CommitFile a.yaml failed: %v", err)
	}
	if err := client.CommitFile("edit/siblings", "nested/one.yaml", []byte("one updated"), author, "update one"); err != nil {
		t.Fatalf("CommitFile nested/one.yaml failed: %v", err)
	}

	for _, tc := range []struct {
		path    string
		want    string
	}{
		{"a.yaml", "a updated"},
		{"b.yaml", "b original"},
		{"nested/one.yaml", "one updated"},
		{"nested/two.yaml", "two original"},
	} {
		content, err := client.ReadFile("edit/siblings", tc.path)
		if err != nil {
			t.Fatalf("ReadFile %s failed: %v", tc.path, err)
		}
		if string(content) != tc.want {
			t.Errorf("ReadFile %s: got %q, want %q", tc.path, string(content), tc.want)
		}
	}
}

func TestCommitFileNonExistentBranch(t *testing.T) {
	repoPath := createTestRepo(t, []testCommit{
		{Files: map[string]string{"catalog.yaml": "content"}},
	})
	client, err := New(repoPath, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	author := Author{Name: "Test", Email: "test@example.com"}
	err = client.CommitFile("no-such-branch", "file.yaml", []byte("data"), author, "msg")
	if err == nil {
		t.Fatal("CommitFile to non-existent branch should have failed")
	}
}

func TestCommitFileOverwriteDirWithFile(t *testing.T) {
	repoPath := createTestRepo(t, []testCommit{
		{Files: map[string]string{
			"nested/service.yaml": "service content",
		}},
	})
	client, err := New(repoPath, nil)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := client.CreateBranch("edit/overwrite-dir", "master"); err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}

	author := Author{Name: "Test", Email: "test@example.com"}

	// "nested" is a directory. Writing a file at path "nested" replaces the dir entry with a blob.
	err = client.CommitFile("edit/overwrite-dir", "nested", []byte("now a file"), author, "replace dir")
	if err != nil {
		t.Fatalf("CommitFile failed: %v", err)
	}

	content, err := client.ReadFile("edit/overwrite-dir", "nested")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "now a file" {
		t.Errorf("Expected 'now a file', got %q", string(content))
	}

	// The old nested/service.yaml should no longer be accessible.
	_, err = client.ReadFile("edit/overwrite-dir", "nested/service.yaml")
	if err == nil {
		t.Error("Expected error reading nested/service.yaml after dir was replaced, but got nil")
	}
}
