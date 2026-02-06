package comments

import (
	"time"
)

const (
	MaxAuthorLength = 100
	MaxTextLength   = 2000
)

// Comment represents a single note left on an entity.
type Comment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	Resolved  bool      `json:"resolved"`
}

// Store is the interface for persisting and retrieving comments.
type Store interface {
	// GetComments returns all comments for the given entity reference.
	// entityRef is typically in the format kind:namespace/name.
	GetComments(entityRef string) ([]Comment, error)
	// GetOpenComments returns only unresolved comments for the given entity reference.
	GetOpenComments(entityRef string) ([]Comment, error)
	// AddComment adds a new comment for the given entity reference.
	AddComment(entityRef string, comment Comment) error
	// ResolveComment marks a specific comment as resolved.
	ResolveComment(entityRef string, commentID string) error
}

// EmptyStore is a Store that does nothing and returns no comments.
type EmptyStore struct{}

func (s EmptyStore) GetComments(entityRef string) ([]Comment, error) {
	return []Comment{}, nil
}

func (s EmptyStore) GetOpenComments(entityRef string) ([]Comment, error) {
	return []Comment{}, nil
}

func (s EmptyStore) AddComment(entityRef string, comment Comment) error {
	return nil
}

func (s EmptyStore) ResolveComment(entityRef string, commentID string) error {
	return nil
}
