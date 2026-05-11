package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeManifest creates static/dist/.vite/manifest.json under dir with the
// given content and returns dir for chaining.
func writeManifest(t *testing.T, dir, content string) {
	t.Helper()
	manifestDir := filepath.Join(dir, "static", "dist", ".vite")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestLoadViteManifest(t *testing.T) {
	tmp := t.TempDir()
	// A trimmed but representative manifest: one entry (main.js with bundled
	// CSS), one dynamic entry, and one non-entry asset entry — only the entry
	// and its CSS should end up in the lookup map.
	writeManifest(t, tmp, `{
		"main.js": {
			"file": "main-DLBUwBcq.js",
			"name": "main",
			"src": "main.js",
			"isEntry": true,
			"css": ["main-fOZRu7S3.css"]
		},
		"graph.js": {
			"file": "graph-NAiI8rCb.js",
			"name": "graph",
			"src": "graph.js",
			"isDynamicEntry": true
		},
		"node_modules/@fontsource/noto-sans/files/noto-sans-latin-400-normal.woff2": {
			"file": "noto-sans-latin-400-normal-BTkUljjl.woff2",
			"src": "node_modules/@fontsource/noto-sans/files/noto-sans-latin-400-normal.woff2"
		}
	}`)

	s := &Server{opts: ServerOptions{BaseDir: tmp}}
	if err := s.loadViteManifest(); err != nil {
		t.Fatalf("loadViteManifest: %v", err)
	}

	wantEntries := map[string]string{
		"main.js":  "main-DLBUwBcq.js",
		"main.css": "main-fOZRu7S3.css",
	}
	for name, want := range wantEntries {
		if got := s.viteManifest[name]; got != want {
			t.Errorf("viteManifest[%q] = %q, want %q", name, got, want)
		}
	}

	// Dynamic entries and bare asset entries are loaded by the bundle itself,
	// not from templates — they must not pollute the lookup map.
	for _, name := range []string{
		"graph.js",
		"node_modules/@fontsource/noto-sans/files/noto-sans-latin-400-normal.woff2",
	} {
		if got, ok := s.viteManifest[name]; ok {
			t.Errorf("viteManifest[%q] = %q, want absent", name, got)
		}
	}
}

func TestLoadViteManifest_MissingFile(t *testing.T) {
	// Missing manifest is normal in dev/test before `npm run build`. We expect
	// graceful degradation: no error, empty map, asset URLs fall back to
	// unhashed paths.
	s := &Server{opts: ServerOptions{BaseDir: t.TempDir()}}
	if err := s.loadViteManifest(); err != nil {
		t.Fatalf("loadViteManifest with no manifest should succeed, got %v", err)
	}
	if len(s.viteManifest) != 0 {
		t.Errorf("viteManifest = %v, want empty map", s.viteManifest)
	}
	if got := s.assetURL("main.js"); got != "/static/dist/main.js" {
		t.Errorf("assetURL(main.js) with empty manifest = %q, want unhashed fallback", got)
	}
}

func TestLoadViteManifest_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	writeManifest(t, tmp, `{not valid json`)

	s := &Server{opts: ServerOptions{BaseDir: tmp}}
	err := s.loadViteManifest()
	if err == nil {
		t.Fatal("loadViteManifest with invalid JSON should error, got nil")
	}
	if !strings.Contains(err.Error(), "parsing vite manifest") {
		t.Errorf("error = %v, want it wrapped with %q", err, "parsing vite manifest")
	}
}

func TestAssetURL(t *testing.T) {
	s := &Server{viteManifest: map[string]string{
		"main.js":  "main-DLBUwBcq.js",
		"main.css": "main-fOZRu7S3.css",
	}}

	if got := s.assetURL("main.js"); got != "/static/dist/main-DLBUwBcq.js" {
		t.Errorf("assetURL(main.js) = %q, want hashed path", got)
	}

	// Missing entries fall back to the unhashed path so a stale template
	// reference shows up in logs without breaking rendering.
	if got := s.assetURL("ghost.js"); got != "/static/dist/ghost.js" {
		t.Errorf("assetURL(ghost.js) = %q, want fallback %q", got, "/static/dist/ghost.js")
	}
}
