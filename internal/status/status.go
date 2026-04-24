package status

import (
	"maps"
	"sync"
)

type Updater interface {
	Update(component string, data any)
}

// Registry contains arbitrary JSON serializable content
// keyed by the status providing component.
type Registry struct {
	mu    sync.RWMutex
	state map[string]any
}

func NewRegistry() *Registry {
	return &Registry{
		state: make(map[string]any),
	}
}

func (r *Registry) Update(component string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state[component] = data
}

func (r *Registry) Snapshot() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	copy := make(map[string]any, len(r.state))
	maps.Copy(copy, r.state)
	return copy
}
