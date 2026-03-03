package web

import (
	"fmt"
	"log"
	"net/http"
	"regexp"

	"github.com/dnswlt/swcat/internal/store"
)

// validNamePrefix matches strings that are safe to embed in a git branch name.
// Allows alphanumeric, hyphens, underscores, and slashes; must start with alphanumeric.
var validNamePrefix = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,15}$`)

func (s *Server) createEditSession(w http.ResponseWriter, r *http.Request) {
	g, ok := s.source.(*store.GitSource)
	if !ok {
		http.Error(w, "Sessions only supported for Git sources", http.StatusBadRequest)
		return
	}

	currentRef := s.getRef(r)

	// Use explicit namePrefix from form body if provided, otherwise fall back to
	// extracting it from the Referer header.
	var namePrefix string
	if v := r.FormValue("namePrefix"); v != "" {
		if !validNamePrefix.MatchString(v) {
			http.Error(w, "Invalid branch name prefix: must be 3-16 alphanumeric characters, hyphens, or underscores", http.StatusBadRequest)
			return
		}
		namePrefix = v
	} else if referer := r.Header.Get("Referer"); referer != "" {
		if ref, err := extractEntityRef(referer); err == nil {
			namePrefix = ref.Name
		}
	}

	branchName, err := g.CreateEditSession(currentRef, namePrefix)
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

func (s *Server) uploadEditSession(w http.ResponseWriter, r *http.Request) {
	g, ok := s.source.(*store.GitSource)
	if !ok {
		s.renderErrorSnippet(w, "Sessions only supported for Git sources")
		return
	}

	currentRef := s.getRef(r)
	if !g.IsSession(currentRef) {
		s.renderErrorSnippet(w, "Not a session branch")
		return
	}

	if err := g.PushEditSession(currentRef); err != nil {
		log.Printf("Failed to push edit session: %v", err)
		s.renderErrorSnippet(w, "Failed to push edit session: "+err.Error())
		return
	}

	s.renderSuccessSnippet(w,
		fmt.Sprintf("Session %s successfully uploaded to remote Git server.", currentRef))
}
