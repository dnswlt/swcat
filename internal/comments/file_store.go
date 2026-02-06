package comments

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileStore implements the Store interface using local JSON files.
type FileStore struct {
	rootDir string
	mu      sync.RWMutex
}

// NewFileStore creates a new FileStore that stores comments in rootDir.
func NewFileStore(rootDir string) (*FileStore, error) {
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create comments directory: %v", err)
	}
	return &FileStore{rootDir: rootDir}, nil
}

func (s *FileStore) entityPath(entityRef string) string {
	// Escape entityRef to be a safe filename.
	// entityRef is kind:namespace/name
	safeRef := strings.ReplaceAll(entityRef, ":", "_")
	safeRef = strings.ReplaceAll(safeRef, "/", "_")
	return filepath.Join(s.rootDir, safeRef+".json")
}

func (s *FileStore) GetComments(entityRef string) ([]Comment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.entityPath(entityRef)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []Comment{}, nil
	}
	if err != nil {
		return nil, err
	}

	var comments []Comment
	if err := json.Unmarshal(data, &comments); err != nil {
		return nil, err
	}

	return comments, nil
}

func (s *FileStore) GetOpenComments(entityRef string) ([]Comment, error) {
	allComments, err := s.GetComments(entityRef)
	if err != nil {
		return nil, err
	}
	var openComments []Comment
	for _, c := range allComments {
		if !c.Resolved {
			openComments = append(openComments, c)
		}
	}
	return openComments, nil
}

func (s *FileStore) AddComment(entityRef string, comment Comment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.entityPath(entityRef)
	var comments []Comment
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &comments); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	comments = append(comments, comment)

	newData, err := json.MarshalIndent(comments, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, newData, 0644)
}

func (s *FileStore) ResolveComment(entityRef string, commentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.entityPath(entityRef)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var comments []Comment
	if err := json.Unmarshal(data, &comments); err != nil {
		return err
	}

	found := false
	for i := range comments {
		if comments[i].ID == commentID {
			comments[i].Resolved = true
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("comment %s not found", commentID)
	}

	newData, err := json.MarshalIndent(comments, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, newData, 0644)
}
