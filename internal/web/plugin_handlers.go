package web

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
	"github.com/dnswlt/swcat/internal/store"
)

func (s *Server) runPlugins(w http.ResponseWriter, r *http.Request, entityRef string) {
	if s.pluginRegistry == nil {
		http.Error(w, "No plugin registry available", http.StatusPreconditionFailed)
		return
	}
	if !s.isHX(r) {
		http.Error(w, "Plugin execution must be done via HTMX", http.StatusBadRequest)
		return
	}
	if s.isReadOnly(r) {
		http.Error(w, "Cannot run plugins in read-only mode", http.StatusPreconditionFailed)
		return
	}

	ref, err := catalog.ParseRef(entityRef)
	if err != nil {
		http.Error(w, "Invalid entity reference", http.StatusBadRequest)
		return
	}

	data := s.getStoreData(r)
	entity := data.repo.Entity(ref)
	if entity == nil {
		http.Error(w, "Invalid entity", http.StatusNotFound)
		return
	}

	entities := pluginEntities(entity, data.repo)

	// 1. Run plugins for each entity first, without holding the lock.
	// This can take a while if there are many entities or slow plugins.
	type pluginResult struct {
		entity catalog.Entity
		exts   *api.CatalogExtensions
		err    error
	}
	results := make([]pluginResult, 0, len(entities))
	for _, e := range entities {
		exts, err := s.pluginRegistry.Run(r.Context(), e)
		results = append(results, pluginResult{entity: e, exts: exts, err: err})
	}

	// 2. Acquire write lock for store access and update.
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.source.Store(data.ref)
	if err != nil {
		s.renderErrorSnippet(w, fmt.Sprintf("Failed to access store: %v", err))
		return
	}

	// 3. Save plugin results to the store.
	var errs []string
	var nSuccess int
	for _, res := range results {
		if res.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", res.entity.GetRef(), res.err))
			continue
		}

		err := store.MergeExtensions(st, res.entity.GetSourceInfo().Path, res.exts)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", res.entity.GetRef(), err))
		} else {
			nSuccess++
		}
	}

	// Clear data, which forces a reload on the next request.
	s.storeDataMap = make(map[string]*storeData)

	if len(errs) > 0 {
		summary := fmt.Sprintf("Plugins completed with %d error(s) and %d successful update(s):", len(errs), nSuccess)
		log.Printf("%s %s", summary, strings.Join(errs, "; "))
		s.renderErrorList(w, summary, errs)
		return
	}

	w.Header().Set("HX-Trigger-After-Swap", "pluginsSuccess")
	s.renderSuccessSnippet(w, "Plugins ran successfully. Reloading…")
}

// pluginEntities returns the entity itself and, if it is a System,
// all of its children (components, resources, APIs).
func pluginEntities(entity catalog.Entity, r *repo.Repository) []catalog.Entity {
	entities := []catalog.Entity{entity}
	if system, ok := entity.(*catalog.System); ok {
		for _, childRef := range system.GetComponents() {
			if child := r.Entity(childRef); child != nil {
				entities = append(entities, child)
			}
		}
		for _, childRef := range system.GetResources() {
			if child := r.Entity(childRef); child != nil {
				entities = append(entities, child)
			}
		}
		for _, childRef := range system.GetAPIs() {
			if child := r.Entity(childRef); child != nil {
				entities = append(entities, child)
			}
		}
	}
	return entities
}
