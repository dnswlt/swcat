package comments

import (
	"sync"
)

// CachingStore is a write-through cache that wraps another Store.
type CachingStore struct {
	underlying Store
	mu         sync.RWMutex
	cache      map[string][]Comment
}

// NewCachingStore creates a new CachingStore wrapping the given Store.
func NewCachingStore(underlying Store) *CachingStore {
	return &CachingStore{
		underlying: underlying,
		cache:      make(map[string][]Comment),
	}
}

func (s *CachingStore) GetComments(entityRef string) ([]Comment, error) {
	s.mu.RLock()
	comments, ok := s.cache[entityRef]
	s.mu.RUnlock()
	if ok {
		return comments, nil
	}

	// Cache miss
	comments, err := s.underlying.GetComments(entityRef)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cache[entityRef] = comments
	s.mu.Unlock()

	return comments, nil
}

func (s *CachingStore) AddComment(entityRef string, comment Comment) error {
	// Write-through to underlying store first
	if err := s.underlying.AddComment(entityRef, comment); err != nil {
		return err
	}

	// Update cache
	s.mu.Lock()
	defer s.mu.Unlock()

	comments, ok := s.cache[entityRef]
	if ok {
		s.cache[entityRef] = append(comments, comment)
	} else {
		// Even if not in cache, we could choose to add it now or just leave it for the next GetComments.
		// Let's add it to avoid a follow-up cache miss.
		s.cache[entityRef] = []Comment{comment}
	}

	return nil
}

func (s *CachingStore) ResolveComment(entityRef string, commentID string) error {
	if err := s.underlying.ResolveComment(entityRef, commentID); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	comments, ok := s.cache[entityRef]
	if ok {
		for i := range comments {
			if comments[i].ID == commentID {
				comments[i].Resolved = true
				break
			}
		}
	}

	return nil
}
