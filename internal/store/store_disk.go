package store

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DiskStore is an implementation of Source and Store that reads files from the local file system.
type DiskStore struct {
	rootDir string // Root path of the catalog
}

// Asserts that DiskStore implements both Source and Store.
var _ Source = (*DiskStore)(nil)
var _ Store = (*DiskStore)(nil)

func NewDiskStore(rootDir string) *DiskStore {
	return &DiskStore{
		rootDir: rootDir,
	}
}

func (d *DiskStore) Refresh() error {
	// Nothing to do for a disk-based store.
	return nil
}

func (d *DiskStore) Store(ref string) (Store, error) {
	if ref != "" {
		return nil, fmt.Errorf("invalid ref %q: %w", ref, ErrNoSuchRef)
	}
	return d, nil
}

func (d *DiskStore) ListFiles(dir string) ([]string, error) {
	return listFilesRecursively(d.rootDir, dir)
}

func resolveRelPath(root, subpath string) (string, error) {
	fullPath := filepath.Join(root, subpath)

	// Verify ancestry by calculating the relative path from the root.
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", fmt.Errorf("not a relative path: %v", err) // e.g. paths on different volumes
	}

	// A relative path escaping the root will start with ".."
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path %q escapes root directory", subpath)
	}

	return fullPath, nil
}

func (d *DiskStore) ReadFile(path string) ([]byte, error) {
	fullPath, err := resolveRelPath(d.rootDir, path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(fullPath)
}

func (d *DiskStore) WriteFile(path string, contents []byte) error {
	fullPath, err := resolveRelPath(d.rootDir, path)
	if err != nil {
		return err
	}
	return os.WriteFile(fullPath, contents, 0644)
}

// listFilesRecursively lists all files in subDir, which must
// be a relative path specifying a sub-directory of rootDir.
// The resulting paths will all be relative to rootDir.
//
// Example:
// with rootDir "/foo/bar" and subDir "baz/quz", all files under
// "/foo/bar/baz/quz" will be returned, relative to "/foo/bar", such as
// ["baz/quz/yankee.yml"].
func listFilesRecursively(rootDir, subDir string) ([]string, error) {
	var files []string

	startDir := filepath.Join(rootDir, subDir)
	err := filepath.WalkDir(startDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Handle errors accessing a path (e.g. permission denied)
			return err
		}

		// If it's a directory, we just continue (it will automatically recurse)
		if d.IsDir() {
			return nil
		}

		// Add the relative file path to our list
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}

		files = append(files, relPath)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}
