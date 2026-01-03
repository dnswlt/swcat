package gitclient

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Auth holds Basic Auth credentials.
// For Bitbucket Cloud access tokens, use "x-token-auth" as Username
// and the token as Password.
type Auth struct {
	Username string
	Password string // or Token
}

// Loader holds the repository in memory
type CatalogLoader struct {
	repo *git.Repository
}

func NewCatalogLoader(url string, auth *Auth) (*CatalogLoader, error) {
	// In-memory storage
	storer := memory.NewStorage()

	cloneOpts := &git.CloneOptions{
		URL:        url,
		NoCheckout: true, // Critical: Don't inflate files into a worktree
		Progress:   nil,
		Depth:      0, // 0 = Full history (needed if you want to jump between widely divergent tags)
	}

	if auth != nil {
		cloneOpts.Auth = &http.BasicAuth{
			Username: auth.Username,
			Password: auth.Password,
		}
	}

	// Clone "NoCheckout". We only want the object database.
	// We don't need a filesystem abstraction because we won't write files.
	repo, err := git.Clone(storer, nil, cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create CatalogLoader: %w", err)
	}

	return &CatalogLoader{repo: repo}, nil
}

func (c *CatalogLoader) ListReferences() ([]string, error) {
	refMap := make(map[string]bool)

	// List all references (branches, tags, remotes)
	refs, err := c.repo.References()
	if err != nil {
		return nil, err
	}

	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name()
		if name.IsTag() || name.IsBranch() {
			refMap[name.Short()] = true
		} else if name.IsRemote() {
			// e.g. refs/remotes/origin/main -> Short() is "origin/main"
			// We want to strip the remote name
			short := name.Short()
			if slashIdx := strings.Index(short, "/"); slashIdx != -1 {
				refMap[short[slashIdx+1:]] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var references []string
	for v := range refMap {
		references = append(references, v)
	}
	return references, nil
}

func (c *CatalogLoader) resolveRevision(revision string) (*plumbing.Hash, error) {
	hash, err := c.repo.ResolveRevision(plumbing.Revision(revision))
	if err == nil {
		return hash, nil
	}

	// Try with origin/ prefix if not found (common for clones)
	if !strings.HasPrefix(revision, "refs/") {
		if hash, err := c.repo.ResolveRevision(plumbing.Revision("origin/" + revision)); err == nil {
			return hash, nil
		}
	}

	return nil, fmt.Errorf("revision not found: %w", err)
}

func (c *CatalogLoader) ReadFile(revision, filePath string) ([]byte, error) {
	// 1. Resolve the revision (tag/branch name) to a SHA-1 hash
	hash, err := c.resolveRevision(revision)
	if err != nil {
		return nil, err
	}

	// 2. Retrieve the Commit Object
	commit, err := c.repo.CommitObject(*hash)
	if err != nil {
		return nil, err
	}

	// 3. Retrieve the Tree Object (the directory structure for that commit)
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	// 4. Find the file in the tree (Virtual path lookup)
	file, err := tree.File(filePath)
	if err != nil {
		return nil, err // Returns object.ErrFileNotFound if missing
	}

	// 5. Read the Blob content
	reader, err := file.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func (c *CatalogLoader) ListFilesRecursive(revision, dirPath string) ([]string, error) {
	// Resolve Revision to a Commit Hash
	hash, err := c.resolveRevision(revision)
	if err != nil {
		return nil, fmt.Errorf("revision resolution failed: %w", err)
	}

	// Get the Commit Object
	commit, err := c.repo.CommitObject(*hash)
	if err != nil {
		return nil, fmt.Errorf("commit lookup failed: %w", err)
	}

	// Get the Root Tree (the root directory of the repo)
	rootTree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("failed to get root tree: %w", err)
	}

	// Navigate to the specific subdirectory (if provided)
	var targetTree *object.Tree
	if dirPath == "" || dirPath == "." || dirPath == "/" {
		targetTree = rootTree
	} else {
		// Tree() returns an error if the path doesn't exist or isn't a directory
		targetTree, err = rootTree.Tree(dirPath)
		if err != nil {
			return nil, fmt.Errorf("directory %q not found or invalid: %w", dirPath, err)
		}
	}

	var filePaths []string

	// Create the recursive file iterator
	filesIter := targetTree.Files()
	defer filesIter.Close()

	// Iterate and handle errors during traversal
	err = filesIter.ForEach(func(f *object.File) error {
		// f.Name is the path relative to targetTree
		filePaths = append(filePaths, f.Name)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iteration failed: %w", err)
	}

	return filePaths, nil
}
