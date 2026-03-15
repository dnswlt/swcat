package web

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/dnswlt/swcat/internal/store"
	"gopkg.in/yaml.v3"
)

// Document describes a single static document to be included in the viewer.
type Document struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description,omitempty"`
	Path        string `yaml:"path"`
}

// DocumentIndex holds the configuration for all documents available in the catalog.
type DocumentIndex struct {
	Documents []Document `yaml:"documents"`
}

// readDocumentIndex reads the "documents/index.yml" file from the store.
func readDocumentIndex(st store.Store) (*DocumentIndex, error) {
	bs, err := st.ReadFile(path.Join(store.DocumentsDir, "index.yml"))
	if err != nil {
		return nil, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(bs))
	dec.KnownFields(true) // Error out on unknown fields to catch mistakes in config

	var index DocumentIndex
	if err := dec.Decode(&index); err != nil {
		return nil, fmt.Errorf("failed to decode documents/index.yml: %w", err)
	}

	return &index, nil
}

func (s *Server) hasDocuments(r *http.Request) bool {
	data := s.getStoreData(r)
	st, err := s.source.Store(data.ref)
	if err != nil {
		return false
	}

	bs, err := st.ReadFile(path.Join(store.DocumentsDir, "index.yml"))
	if err != nil {
		return false
	}
	return len(bs) > 0
}

func (s *Server) serveDocuments(w http.ResponseWriter, r *http.Request, docID string) {
	data := s.getStoreData(r)
	st, err := s.source.Store(data.ref)
	if err != nil {
		http.Error(w, "Failed to get store", http.StatusInternalServerError)
		return
	}

	index, err := readDocumentIndex(st)
	if err != nil {
		// Log but do not fail hard, we can redirect or show an empty page.
		log.Printf("Failed to read document index: %v", err)
		index = &DocumentIndex{}
	}

	if len(index.Documents) > 0 {
		if docID == "" {
			docID = index.Documents[0].ID
		}
	} else {
		// No content. Just clear current doc.
		docID = ""
	}

	params := map[string]any{
		"Documents": index.Documents,
		"ActiveDoc": docID,
	}

	for _, doc := range index.Documents {
		if doc.ID == docID {
			params["DocumentViewURL"] = uiURLWithContext(r.Context(), "documents/raw/"+doc.Path)
			break
		}
	}

	s.serveHTMLPage(w, r, "documents.html", params)
}

func (s *Server) serveRawDocument(w http.ResponseWriter, r *http.Request, docPath string) {
	data := s.getStoreData(r)
	st, err := s.source.Store(data.ref)
	if err != nil {
		http.Error(w, "Failed to get store", http.StatusInternalServerError)
		return
	}

	// Validate path to avoid directory traversal escaping
	cleanPath := path.Clean("/" + docPath)
	if strings.Contains(cleanPath, "..") {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	fullPath := path.Join("documents", cleanPath)

	bs, err := st.ReadFile(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Basic content-type detection based on extension
	ext := path.Ext(fullPath)
	contentType := "application/octet-stream"
	switch ext {
	case ".html", ".htm":
		contentType = "text/html; charset=UTF-8"
	case ".css":
		contentType = "text/css; charset=UTF-8"
	case ".js":
		contentType = "application/javascript; charset=UTF-8"
	case ".json":
		contentType = "application/json; charset=UTF-8"
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".svg":
		contentType = "image/svg+xml"
	case ".txt":
		contentType = "text/plain; charset=UTF-8"
	}

	w.Header().Set("Content-Type", contentType)
	w.Write(bs)
}
