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

	gs := NewGitSource(client, "master", "", gitclient.Author{})

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

	gs := NewGitSource(client, "master", "nested", gitclient.Author{})

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

func TestGitSource_EditSession(t *testing.T) {
	repoPath := createTestRepo(t)

	client, err := gitclient.New(repoPath, nil)
	if err != nil {
		t.Fatalf("gitclient.New failed: %v", err)
	}

	gs := NewGitSource(client, "master", "", gitclient.Author{Name: "Test", Email: "test@example.com"})

	t.Run("WriteFile_ReadOnly", func(t *testing.T) {
		// A non-session store should still be read-only.
		st, err := gs.Store("master")
		if err != nil {
			t.Fatalf("Store failed: %v", err)
		}
		if err := st.WriteFile("any.txt", []byte("data")); err != ErrReadOnly {
			t.Errorf("WriteFile() = %v, want ErrReadOnly", err)
		}
	})

	t.Run("WriteFile_EditSession", func(t *testing.T) {
		branchName, err := gs.CreateEditSession("master", "")
		if err != nil {
			t.Fatalf("CreateEditSession failed: %v", err)
		}

		// The new branch should appear in references.
		refs, err := gs.ListReferences()
		if err != nil {
			t.Fatalf("ListReferences failed: %v", err)
		}
		if !slices.Contains(refs, branchName) {
			t.Errorf("%s not found in refs: %v", branchName, refs)
		}

		// Get a writable store for the edit session branch.
		st, err := gs.Store(branchName)
		if err != nil {
			t.Fatalf("Store(%s) failed: %v", err, branchName)
		}

		// Write should succeed.
		if err := st.WriteFile("catalog.yaml", []byte("edited content")); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		// Read back the written content.
		content, err := st.ReadFile("catalog.yaml")
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if string(content) != "edited content" {
			t.Errorf("Expected 'edited content', got %q", string(content))
		}

		// Master should be unaffected.
		masterSt, err := gs.Store("master")
		if err != nil {
			t.Fatalf("Store(master) failed: %v", err)
		}
		masterContent, err := masterSt.ReadFile("catalog.yaml")
		if err != nil {
			t.Fatalf("ReadFile master failed: %v", err)
		}
		if string(masterContent) != "v2 content" {
			t.Errorf("Expected master 'v2 content', got %q", string(masterContent))
		}
	})

	t.Run("CloseEditSession", func(t *testing.T) {
		// Create and close a session.
		branchName, err := gs.CreateEditSession("master", "")
		if err != nil {
			t.Fatalf("CreateEditSession failed: %v", err)
		}
		if err := gs.CloseEditSession(branchName); err != nil {
			t.Fatalf("CloseEditSession failed: %v", err)
		}

		// Branch should be gone from references.
		refs, err := gs.ListReferences()
		if err != nil {
			t.Fatalf("ListReferences failed: %v", err)
		}
		if slices.Contains(refs, branchName) {
			t.Errorf("%s should be gone, but found in refs: %v", branchName, refs)
		}

		// Store for the closed session should fail.
		_, err = gs.Store(branchName)
		if err != ErrNoSuchRef {
			t.Errorf("Store(%s) = %v, want ErrNoSuchRef", branchName, err)
		}
	})
}

func TestGitSource_RestoreSessions(t *testing.T) {
	repoPath := createTestRepo(t)

	// First "server lifetime": create a session, make an edit, push.
	client1, err := gitclient.New(repoPath, nil)
	if err != nil {
		t.Fatalf("gitclient.New failed: %v", err)
	}
	author := gitclient.Author{Name: "Alice", Email: "alice@example.com"}
	gs1 := NewGitSource(client1, "master", "", author)

	branchName, err := gs1.CreateEditSession("master", "")
	if err != nil {
		t.Fatalf("CreateEditSession failed: %v", err)
	}

	// Write something via the session.
	st, err := gs1.Store(branchName)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if err := st.WriteFile("catalog.yaml", []byte("alice edit")); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Push the session branch to origin (the local repo acts as origin).
	if err := gs1.PushEditSession(branchName); err != nil {
		t.Fatalf("PushEditSession failed: %v", err)
	}

	// "Server restart": create a fresh Client and GitSource (empty sessions map).
	client2, err := gitclient.New(repoPath, nil)
	if err != nil {
		t.Fatalf("gitclient.New (restart) failed: %v", err)
	}
	gs2 := NewGitSource(client2, "master", "", author)

	// Before restore, the session should not be known.
	if gs2.IsSession(branchName) {
		t.Fatal("session should not exist before RestoreSessions")
	}

	// Restore sessions from remote edit/ branches.
	if _, err := gs2.RestoreSessions(); err != nil {
		t.Fatalf("RestoreSessions failed: %v", err)
	}

	// The session should now be registered.
	if !gs2.IsSession(branchName) {
		t.Fatalf("session %s not restored", branchName)
	}

	// The store should be writable and contain Alice's edit.
	st2, err := gs2.Store(branchName)
	if err != nil {
		t.Fatalf("Store(%s) after restore failed: %v", branchName, err)
	}
	content, err := st2.ReadFile("catalog.yaml")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(content) != "alice edit" {
		t.Errorf("Expected 'alice edit', got %q", string(content))
	}

	// Further edits should work.
	if err := st2.WriteFile("catalog.yaml", []byte("alice edit 2")); err != nil {
		t.Fatalf("WriteFile after restore failed: %v", err)
	}
	content, err = st2.ReadFile("catalog.yaml")
	if err != nil {
		t.Fatalf("ReadFile after second edit failed: %v", err)
	}
	if string(content) != "alice edit 2" {
		t.Errorf("Expected 'alice edit 2', got %q", string(content))
	}
}

func TestGitSource_ReadEntities(t *testing.T) {
	// 1. Setup repo
	dir := createRepoFromTestdata(t, "../../testdata/test1/catalog/catalog.yml")

	// 2. Create GitSource
	client, err := gitclient.New(dir, nil)
	if err != nil {
		t.Fatalf("gitclient.New failed: %v", err)
	}
	gs := NewGitSource(client, "master", "", gitclient.Author{})

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
	gs := NewGitSource(client, "master", "", gitclient.Author{})
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
