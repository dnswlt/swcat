package web

import (
	"log"
	"net/http"

	"github.com/dnswlt/swcat/internal/store"
)

func (s *Server) createEditSession(w http.ResponseWriter, r *http.Request) {
	g, ok := s.source.(*store.GitSource)
	if !ok {
		http.Error(w, "Sessions only supported for Git sources", http.StatusBadRequest)
		return
	}

	currentRef := s.getRef(r)
	branchName, err := g.CreateEditSession(currentRef)
	if err != nil {
		log.Printf("Failed to create edit session: %v", err)
		http.Error(w, "Failed to create edit session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	newURL := switchRef(r.Header.Get("Referer"), branchName)
	w.Header().Set("HX-Redirect", newURL)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) discardEditSession(w http.ResponseWriter, r *http.Request) {
	g, ok := s.source.(*store.GitSource)
	if !ok {
		http.Error(w, "Sessions only supported for Git sources", http.StatusBadRequest)
		return
	}

	currentRef := s.getRef(r)
	if !g.IsSession(currentRef) {
		http.Error(w, "Not a session branch", http.StatusBadRequest)
		return
	}

	if err := g.CloseEditSession(currentRef); err != nil {
		log.Printf("Failed to close edit session: %v", err)
		http.Error(w, "Failed to close edit session: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to default branch
	newURL := switchRef(r.Header.Get("Referer"), g.DefaultRef())
	w.Header().Set("HX-Redirect", newURL)
	w.WriteHeader(http.StatusOK)
}
