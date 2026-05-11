package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"

	"github.com/dnswlt/swcat"
)

// loadViteManifest reads the Vite manifest from disk (BaseDir mode) or the
// embedded FS, and populates s.viteManifest with logical-name → hashed URL
// mappings used by the "asset" template function.
func (s *Server) loadViteManifest() error {
	const manifestPath = "static/dist/.vite/manifest.json"
	var data []byte
	var err error
	if s.opts.BaseDir == "" {
		data, err = fs.ReadFile(swcat.Files, manifestPath)
	} else {
		data, err = os.ReadFile(path.Join(s.opts.BaseDir, manifestPath))
	}
	if err != nil {
		// A missing manifest is normal in dev/test before `npm run build` has
		// run — fall back to unhashed paths via assetURL rather than failing.
		// A present-but-broken manifest is still an error worth surfacing.
		if errors.Is(err, fs.ErrNotExist) {
			log.Printf("Vite manifest not found at %s; asset URLs will fall back to unhashed paths. Run `npm run build` to generate it.", manifestPath)
			s.viteManifest = map[string]string{}
			return nil
		}
		return fmt.Errorf("loading vite manifest: %w", err)
	}

	// We only care about a few fields from each manifest entry — see
	// https://vite.dev/guide/backend-integration for the full format.
	type viteEntry struct {
		File    string   `json:"file"`
		Name    string   `json:"name,omitempty"`
		CSS     []string `json:"css,omitempty"`
		IsEntry bool     `json:"isEntry,omitempty"`
	}
	var raw map[string]viteEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parsing vite manifest: %w", err)
	}

	m := make(map[string]string)
	for src, e := range raw {
		if !e.IsEntry {
			continue
		}
		// Entry JS: src like "main.js" → hashed filename like "main-<hash>.js".
		m[src] = e.File
		// CSS bundled with the entry: expose as "<entry-name>.css" so templates
		// can say {{ asset "main.css" }}.
		if e.Name != "" {
			for _, css := range e.CSS {
				m[e.Name+".css"] = css
			}
		}
	}
	s.viteManifest = m
	return nil
}

// assetURL is the implementation of the "asset" template function. It returns
// the hashed URL for a logical asset name (e.g. "main.js" → "/static/dist/main-<hash>.js").
// Missing entries fall back to the unhashed path with a warning, so a stale
// template reference is visible in logs without breaking page rendering.
func (s *Server) assetURL(name string) string {
	const urlPrefix = "/static/dist/"
	if file, ok := s.viteManifest[name]; ok {
		return urlPrefix + file
	}
	log.Printf("asset %q not found in vite manifest; falling back to unhashed path", name)
	return urlPrefix + name
}
