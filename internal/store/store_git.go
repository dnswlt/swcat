package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dnswlt/swcat/internal/gitclient"
)

// GitSource is an implementation of Source that reads from a remote Git repository.
type GitSource struct {
	client     *gitclient.Client
	defaultRef string           // ref to use if the empty ref ("") is requested
	rootDir    string           // optional root directory within the repo
	author     gitclient.Author // author for commits in edit sessions
	refs       []string         // cached list of available references
	mu         sync.Mutex
	sessions   map[string]bool // set of active edit session branch names
}

// gitStore is a "view" over a single revision in a GitSource.
type gitStore struct {
	client  *gitclient.Client
	ref     string
	rootDir string
	author  *gitclient.Author // nil = read-only
}

var _ Source = (*GitSource)(nil)
var _ Store = (*gitStore)(nil)

func NewGitSource(client *gitclient.Client, defaultRef string, rootDir string, author gitclient.Author) *GitSource {
	return &GitSource{
		client:     client,
		defaultRef: defaultRef,
		rootDir:    rootDir,
		author:     author,
		sessions:   make(map[string]bool),
	}
}

func (g *GitSource) DefaultRef() string {
	return g.defaultRef
}

func (g *GitSource) Refresh() error {

	// Get remote branches before fetch to detect deletions.
	// We only care about branches that were already known to be on remote.
	oldRemote, err := g.client.ListRemoteBranches()
	if err != nil {
		return fmt.Errorf("failed to list remote branches before fetch: %w", err)
	}

	if err := g.client.Fetch(); err != nil {
		return err
	}

	newRemote, err := g.client.ListRemoteBranches()
	if err != nil {
		return err
	}

	g.mu.Lock()

	g.refs = nil

	// Collect under lock for a consistent snapshot; CloseEditSession also
	// acquires g.mu, so we must release the lock before calling it below.
	var toPrune []string
	for _, b := range removedBranches(oldRemote, newRemote) {
		if g.sessions[b] {
			toPrune = append(toPrune, b)
		}
	}
	g.mu.Unlock()

	for _, b := range toPrune {
		log.Printf("Pruning session %q (deleted from remote)", b)
		_ = g.CloseEditSession(b)
	}

	return nil
}

func (g *GitSource) Store(ref string) (Store, error) {
	if ref == "" {
		ref = g.defaultRef
	}
	refs, err := g.ListReferences()
	if err != nil {
		return nil, fmt.Errorf("cannot list references: %v", err)
	}
	if !slices.Contains(refs, ref) {
		return nil, ErrNoSuchRef
	}
	// Check if this ref is an active edit session (writable).
	g.mu.Lock()
	isSession := g.sessions[ref]
	g.mu.Unlock()
	var author *gitclient.Author
	if isSession {
		author = &g.author
	}
	return &gitStore{
		client:  g.client,
		ref:     ref,
		rootDir: g.rootDir,
		author:  author,
	}, nil
}

// IsReadOnly reports whether editing is disabled for this GitSource.
// A GitSource is read-only when no author has been configured.
func (g *GitSource) IsReadOnly() bool {
	return !g.author.IsSet()
}

func (g *GitSource) IsSession(ref string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sessions[ref]
}

// CreateEditSession creates a new branch and registers it as an active edit session.
// It generates a unique branch name based on the baseRef and a timestamp.
// If namePrefix is not empty, it's used as part of the branch name.
func (g *GitSource) CreateEditSession(baseRef string, namePrefix string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Generate a unique branch name.
	var branchName string
	for {
		var err error
		branchName, err = buildSessionBranchName(baseRef, namePrefix)
		if err != nil {
			return "", err
		}

		if !g.sessions[branchName] {
			break
		}
	}

	if err := g.client.CreateBranch(branchName, baseRef); err != nil {
		return "", err
	}

	g.sessions[branchName] = true
	g.refs = nil // Invalidate cache so the new branch shows up.
	return branchName, nil
}

// removedBranches returns branches present in old but absent in new.
func removedBranches(old, new []string) []string {
	newSet := make(map[string]bool, len(new))
	for _, b := range new {
		newSet[b] = true
	}
	var removed []string
	for _, b := range old {
		if !newSet[b] {
			removed = append(removed, b)
		}
	}
	return removed
}

func buildSessionBranchName(baseRef string, namePrefix string) (string, error) {
	suffix := make([]byte, 2)
	if _, err := rand.Read(suffix); err != nil {
		return "", err
	}

	var prefix string
	if namePrefix != "" {
		prefix = strings.ReplaceAll(namePrefix, "/", "-")
	} else {
		prefix = strings.ReplaceAll(baseRef, "/", "-")
	}

	timestamp := time.Now().Format("20060102-1504")
	hexSuffix := hex.EncodeToString(suffix)

	// branchName = "edit/<prefix>-<timestamp>-<hexSuffix>"
	// Limit prefix length so total branch name doesn't exceed 100 chars.
	// "edit/" (5) + "-" (1) + timestamp (13) + "-" (1) + hexSuffix (4) = 24 chars
	// Max prefix length = 100 - 24 = 76
	const maxLen = 100
	fixedPartsLen := len("edit/") + 1 + len(timestamp) + 1 + len(hexSuffix)
	maxPrefixLen := maxLen - fixedPartsLen

	if len(prefix) > maxPrefixLen {
		prefix = prefix[:maxPrefixLen]
	}

	return fmt.Sprintf("edit/%s-%s-%s", prefix, timestamp, hexSuffix), nil
}

// PushEditSession pushes an edit session's branch to the remote.
func (g *GitSource) PushEditSession(branchName string) error {
	return g.client.Push(branchName)
}

// CloseEditSession removes the local branch and deregisters the edit session.
func (g *GitSource) CloseEditSession(branchName string) error {
	if err := g.client.DeleteBranch(branchName); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.sessions, branchName)

	g.refs = nil // Invalidate cache.
	return nil
}

// RestoreSessions scans for remote edit/ branches and reconstructs
// the sessions map. For each branch, a local branch is created (if it
// doesn't already exist). Call this after a fresh clone or server restart
// to resume editing.
func (g *GitSource) RestoreSessions() ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	refs, err := g.client.ListReferences()
	if err != nil {
		return nil, fmt.Errorf("list references: %w", err)
	}

	var restored []string
	for _, ref := range refs {
		if !strings.HasPrefix(ref, "edit/") {
			continue
		}
		if g.sessions[ref] {
			continue // already tracked
		}

		// Ensure a local branch exists. CreateBranch resolves the base ref
		// via resolveRevision, which will find origin/edit/... if there's
		// no local branch yet. If the local branch already exists, this
		// will fail harmlessly.
		_ = g.client.CreateBranch(ref, ref)

		g.sessions[ref] = true
		restored = append(restored, ref)
	}

	g.refs = nil // Invalidate cache.
	return restored, nil
}

func (g *GitSource) ListReferences() ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.refs != nil {
		return g.refs, nil
	}
	refs, err := g.client.ListReferences()
	if err != nil {
		return nil, err
	}
	slices.Sort(refs)
	g.refs = refs
	return refs, nil
}

func (g *gitStore) ListFiles(dir string) ([]string, error) {
	fullDir := path.Join(g.rootDir, dir)
	files, err := g.client.ListFilesRecursive(g.ref, fullDir)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %v", err)
	}
	// Make relative to gitStore root (which is g.rootDir).
	result := make([]string, len(files))
	for i, f := range files {
		// Avoid using filepath here, as gitStore needs "/" on any OS.
		// files are already relative to fullDir.
		result[i] = path.Join(dir, f)
	}
	return result, nil
}

func (g *gitStore) ReadFile(filePath string) ([]byte, error) {
	fullPath := path.Join(g.rootDir, filePath)
	return g.client.ReadFile(g.ref, fullPath)
}

func (g *gitStore) WriteFile(filePath string, contents []byte) error {
	if g.author == nil {
		return ErrReadOnly
	}
	fullPath := path.Join(g.rootDir, filePath)
	return g.client.CommitFile(g.ref, fullPath, contents, *g.author, "Update "+filePath)
}
