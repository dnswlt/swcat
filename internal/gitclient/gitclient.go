package gitclient

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
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

// Author identifies the author of a git commit.
type Author struct {
	Name  string
	Email string
}

// Client holds the repository in memory.
type Client struct {
	mu   sync.Mutex
	repo *git.Repository
	auth *Auth
}

func New(url string, auth *Auth) (*Client, error) {
	// In-memory storage
	storer := memory.NewStorage()

	cloneOpts := &git.CloneOptions{
		URL:        url,
		NoCheckout: true, // Critical: Don't inflate files into a worktree
		Progress:   nil,
		Depth:      0, // Full history: tags can point to any commit, so a shallow clone would miss them.
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
		return nil, fmt.Errorf("failed to create Client: %w", err)
	}

	return &Client{repo: repo, auth: auth}, nil
}

func (c *Client) Fetch() error {
	// 1. Configure the Fetch options
	opts := &git.FetchOptions{
		RemoteName: "origin",
		// Download all tags, even those unreachable from the branch heads
		Tags: git.AllTags,
		// Force-update all remote branches to match origin
		// The '+' allows non-fast-forward updates (e.g. if history was rewritten on remote)
		RefSpecs: []config.RefSpec{
			"+refs/heads/*:refs/remotes/origin/*",
		},
	}

	// 2. Attach Auth if provided
	if c.auth != nil {
		opts.Auth = &http.BasicAuth{
			Username: c.auth.Username,
			Password: c.auth.Password,
		}
	}

	// 3. Execute Fetch
	// This downloads missing objects into memory.NewStorage() and updates
	// refs/remotes/origin/* and refs/tags/*
	err := c.repo.Fetch(opts)

	// 4. Handle the "Already Up To Date" case
	// go-git treats this as an error, but for an "Update" operation, it is usually a success state.
	if err == git.NoErrAlreadyUpToDate {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to fetch updates: %w", err)
	}

	return nil
}

func (c *Client) DefaultBranch() (string, error) {
	refs, err := c.repo.References()
	if err != nil {
		return "", err
	}
	var headName string
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name()
		if name == plumbing.HEAD {
			headName = ref.Target().Short()
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if headName == "" {
		return "", fmt.Errorf("could not identify HEAD branch")
	}
	return headName, nil
}

func (c *Client) ListReferences() ([]string, error) {
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

func (c *Client) resolveRevision(revision string) (*plumbing.Hash, error) {
	// Try with origin/ prefix first.
	// We prefer the remote branch because Update() only updates refs/remotes/origin/*.
	// The local branch (refs/heads/*) created by Clone might be stale.
	if !strings.HasPrefix(revision, "refs/") && !strings.HasPrefix(revision, "origin/") {
		if hash, err := c.repo.ResolveRevision(plumbing.Revision("origin/" + revision)); err == nil {
			return hash, nil
		}
	}

	// Fallback: try exact revision (tags, hashes, full refs, etc.)
	hash, err := c.repo.ResolveRevision(plumbing.Revision(revision))
	if err == nil {
		return hash, nil
	}

	return nil, fmt.Errorf("revision %q not found: %w", revision, err)
}

func (c *Client) ReadFile(revision, filePath string) ([]byte, error) {
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
	if errors.Is(err, object.ErrFileNotFound) {
		// Return the same error as os.ReadFile(),
		// so we can deal with missing files in the same way.
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, err
	}

	// 5. Read the Blob content
	reader, err := file.Reader()
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return io.ReadAll(reader)
}

func (c *Client) ListFilesRecursive(revision, dirPath string) ([]string, error) {
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

// CreateBranch creates a new local branch pointing to the same commit as baseRef.
func (c *Client) CreateBranch(name, baseRef string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	hash, err := c.resolveRevision(baseRef)
	if err != nil {
		return fmt.Errorf("resolve base ref %q: %w", baseRef, err)
	}
	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(name), *hash)
	return c.repo.Storer.SetReference(ref)
}

// CommitFile creates a commit on the given local branch that writes content to filePath.
// filePath must use forward slashes and be relative to the repo root.
//
// This uses bare-repo "plumbing" (manual blob/tree/commit creation) rather than
// worktree.Add + worktree.Commit for two reasons:
//
//  1. The Client has no worktree (cloned with nil filesystem). Creating one would
//     require inflating all files into a memfs — expensive for a single-file edit.
//  2. go-git supports only one worktree per Repository. With multiple concurrent
//     edit sessions on different branches, a shared worktree would require serial
//     checkout/switching. The plumbing approach operates directly on immutable objects
//     and independent branch refs, so concurrent sessions don't interfere.
//
// The mutex protects writes against each other (preventing lost commits when two
// CommitFile calls race on the same branch). Reads don't need the mutex: they resolve
// a ref to immutable objects, and the ref update is the last step of any write.
func (c *Client) CommitFile(branch, filePath string, content []byte, author Author, message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	branchRef := plumbing.NewBranchReferenceName(branch)

	// Resolve branch tip.
	ref, err := c.repo.Storer.Reference(branchRef)
	if err != nil {
		return fmt.Errorf("resolve branch %q: %w", branch, err)
	}
	tipHash := ref.Hash()

	tipCommit, err := c.repo.CommitObject(tipHash)
	if err != nil {
		return fmt.Errorf("get tip commit: %w", err)
	}
	rootTree, err := tipCommit.Tree()
	if err != nil {
		return fmt.Errorf("get root tree: %w", err)
	}

	// Store new blob.
	blobObj := c.repo.Storer.NewEncodedObject()
	blobObj.SetType(plumbing.BlobObject)
	blobObj.SetSize(int64(len(content)))
	w, err := blobObj.Writer()
	if err != nil {
		return fmt.Errorf("create blob writer: %w", err)
	}
	if _, err := w.Write(content); err != nil {
		w.Close()
		return fmt.Errorf("write blob: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close blob writer: %w", err)
	}
	blobHash, err := c.repo.Storer.SetEncodedObject(blobObj)
	if err != nil {
		return fmt.Errorf("store blob: %w", err)
	}

	// Rebuild tree chain.
	segments := strings.Split(filePath, "/")
	newRootHash, err := updateTree(c.repo.Storer, rootTree, segments, blobHash)
	if err != nil {
		return fmt.Errorf("update tree: %w", err)
	}

	// Create commit.
	now := time.Now()
	sig := object.Signature{
		Name:  author.Name,
		Email: author.Email,
		When:  now,
	}
	commit := &object.Commit{
		TreeHash:     newRootHash,
		ParentHashes: []plumbing.Hash{tipHash},
		Author:       sig,
		Committer:    sig,
		Message:      message,
	}
	commitObj := c.repo.Storer.NewEncodedObject()
	if err := commit.Encode(commitObj); err != nil {
		return fmt.Errorf("encode commit: %w", err)
	}
	commitHash, err := c.repo.Storer.SetEncodedObject(commitObj)
	if err != nil {
		return fmt.Errorf("store commit: %w", err)
	}

	// Advance branch ref.
	newRef := plumbing.NewHashReference(branchRef, commitHash)
	return c.repo.Storer.SetReference(newRef)
}

// Push pushes a local branch to the remote.
func (c *Client) Push(branch string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))
	opts := &git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
	}
	if c.auth != nil {
		opts.Auth = &http.BasicAuth{
			Username: c.auth.Username,
			Password: c.auth.Password,
		}
	}
	return c.repo.Push(opts)
}

// DeleteBranch removes a local branch reference.
func (c *Client) DeleteBranch(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.repo.Storer.RemoveReference(plumbing.NewBranchReferenceName(name))
}

// updateTree returns a new tree hash with the blob at the given path replaced or added.
// segments is the file path split by "/". oldTree is the tree at the current level.
func updateTree(s storer.EncodedObjectStorer, oldTree *object.Tree, segments []string, blobHash plumbing.Hash) (plumbing.Hash, error) {
	if len(segments) == 0 {
		return plumbing.ZeroHash, fmt.Errorf("empty path segments")
	}

	name := segments[0]
	isLeaf := len(segments) == 1

	// Copy existing entries, replacing or skipping the target.
	var entries []object.TreeEntry
	var found bool
	for _, e := range oldTree.Entries {
		if e.Name == name {
			found = true
			if isLeaf {
				// Replace the file entry with the new blob.
				entries = append(entries, object.TreeEntry{
					Name: name,
					Mode: filemode.Regular,
					Hash: blobHash,
				})
			} else {
				// Recurse into subtree.
				subTree, err := object.GetTree(s, e.Hash)
				if err != nil {
					return plumbing.ZeroHash, fmt.Errorf("get subtree %q: %w", name, err)
				}
				newSubHash, err := updateTree(s, subTree, segments[1:], blobHash)
				if err != nil {
					return plumbing.ZeroHash, err
				}
				entries = append(entries, object.TreeEntry{
					Name: name,
					Mode: filemode.Dir,
					Hash: newSubHash,
				})
			}
		} else {
			entries = append(entries, e)
		}
	}

	if !found {
		if isLeaf {
			// New file entry.
			entries = append(entries, object.TreeEntry{
				Name: name,
				Mode: filemode.Regular,
				Hash: blobHash,
			})
		} else {
			// New intermediate directory — create an empty tree and recurse.
			emptyTree := &object.Tree{}
			newSubHash, err := updateTree(s, emptyTree, segments[1:], blobHash)
			if err != nil {
				return plumbing.ZeroHash, err
			}
			entries = append(entries, object.TreeEntry{
				Name: name,
				Mode: filemode.Dir,
				Hash: newSubHash,
			})
		}
	}

	// Store the new tree.
	newTree := &object.Tree{Entries: entries}
	treeObj := &plumbing.MemoryObject{}
	if err := newTree.Encode(treeObj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode tree: %w", err)
	}
	return s.SetEncodedObject(treeObj)
}
