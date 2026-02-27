//go:build integration

package web

// This file contains test cases for integration tests that exercise plugins.

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/plugins"
	"github.com/dnswlt/swcat/internal/store"
)

// setupPluginIntegrationServer configures and starts a test server using catalog data
// from the given directory. Unlike setupIntegrationServer, it does not require Graphviz
// and accepts any directory, making it suitable for per-test testdata fixtures.
func setupPluginIntegrationServer(t *testing.T, dir string) (*httptest.Server, *Server) {
	t.Helper()

	st := store.NewDiskStore(dir)

	pluginsConfigPath := filepath.Join(dir, store.PluginsFile)
	cfg, err := plugins.ReadConfig(pluginsConfigPath)
	if err != nil {
		t.Fatalf("Failed to read plugins config from %s: %v", pluginsConfigPath, err)
	}
	pluginRegistry, err := plugins.NewRegistry(cfg)
	if err != nil {
		t.Fatalf("Failed to create plugin registry: %v", err)
	}

	opts := ServerOptions{
		Addr:     "localhost:0",
		DotPath:  "dot",
		ReadOnly: false,
		Version:  "integration-test",
	}
	server, err := NewServer(opts, st, nil, pluginRegistry, nil, nil, nil)
	server.dotRunner = &fakeRunner{}
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	if _, err := server.ValidateCatalog(""); err != nil {
		t.Fatalf("Failed to validate catalog: %v", err)
	}

	ts := httptest.NewServer(server.Handler())
	return ts, server
}

func TestIntegration_TimestampPlugin(t *testing.T) {
	// Copy testdata to a temp dir so the generated sidecar file doesn't
	// end up in the source tree.
	tmpDir := t.TempDir()
	if err := os.CopyFS(tmpDir, os.DirFS("testdata/timestamp-plugin")); err != nil {
		t.Fatalf("Failed to copy testdata: %v", err)
	}

	ts, _ := setupPluginIntegrationServer(t, tmpDir)
	defer ts.Close()

	client := ts.Client()

	// 1. GET the entity page and verify that a run-plugins link is present.
	entityPageURL := ts.URL + "/ui/components/test-component"
	resp, err := client.Get(entityPageURL)
	if err != nil {
		t.Fatalf("Failed to GET entity page: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s returned status %d", entityPageURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read entity page body: %v", err)
	}
	// Extract the run-plugins path from the hx-post attribute in the response.
	runPluginsRe := regexp.MustCompile(`hx-post="([^"]+/run-plugins)"`)
	m := runPluginsRe.FindSubmatch(body)
	if m == nil {
		t.Fatalf("Entity page does not contain a /run-plugins link:\n%s", body)
	}
	runPluginsPath := string(m[1])

	// 2. POST to the run-plugins endpoint (simulating the HTMX button click).
	entityRef := "component:test-component"
	req, err := http.NewRequest(http.MethodPost, ts.URL+runPluginsPath, nil)
	if err != nil {
		t.Fatalf("Failed to create run-plugins request: %v", err)
	}
	req.Header.Set("HX-Request", "true")

	resp2, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to POST to run-plugins: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		body2, _ := io.ReadAll(resp2.Body)
		t.Fatalf("run-plugins returned status %d: %s", resp2.StatusCode, body2)
	}

	// 3. Verify the sidecar file was written with the timestamp annotation.
	sidecarPath := filepath.Join(tmpDir, "catalog", "component.ext.json")
	sidecarData, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatalf("Sidecar file not created at %s: %v", sidecarPath, err)
	}

	var exts api.CatalogExtensions
	if err := json.Unmarshal(sidecarData, &exts); err != nil {
		t.Fatalf("Failed to parse sidecar file: %v", err)
	}

	entityExts, ok := exts.Entities[entityRef]
	if !ok {
		t.Fatalf("Sidecar has no entry for %q; got: %v", entityRef, exts.Entities)
	}
	if _, ok := entityExts.Annotations[plugins.AnnotPluginsUpdateTime]; !ok {
		t.Errorf("Sidecar missing %q annotation; got annotations: %v",
			plugins.AnnotPluginsUpdateTime, entityExts.Annotations)
	}
}
