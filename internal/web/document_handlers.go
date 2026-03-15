package web

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

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

// readDocumentIndex reads the "index.yml" file from the local docs disk directory.
func readDocumentIndex(dir string) (*DocumentIndex, error) {
	if dir == "" {
		return &DocumentIndex{}, nil
	}
	bs, err := os.ReadFile(filepath.Join(dir, "index.yml"))
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

func (s *Server) hasDocuments() bool {
	if s.opts.DocumentsDir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(s.opts.DocumentsDir, "index.yml"))
	return err == nil && info.Size() > 0
}

func (s *Server) serveDocuments(w http.ResponseWriter, r *http.Request, docID string) {
	if s.opts.DocumentsDir == "" {
		http.Error(w, "Documents directory not configured", http.StatusNotFound)
		return
	}

	index, err := readDocumentIndex(s.opts.DocumentsDir)
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
			params["DocumentViewURL"] = "/documents/raw/" + doc.Path
			break
		}
	}

	s.serveHTMLPage(w, r, "documents.html", params)
}

