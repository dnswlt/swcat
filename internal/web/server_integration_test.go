//go:build integration

package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/store"
)

// mustFindDotPath locates the dot executable or fails the test.
func mustFindDotPath(t *testing.T) string {
	path, err := exec.LookPath("dot")
	if err != nil {
		t.Skipf("dot executable not found: %v. Skipping integration test requiring Graphviz.", err)
	}
	return path
}

// setupIntegrationServer configures and starts a test server using the flights example.
// It returns the test server instance and the internal Server object.
func setupIntegrationServer(t *testing.T) (*httptest.Server, *Server) {
	// Root of the project, assuming we are running from internal/web
	projectRoot, err := filepath.Abs("../../")
	if err != nil {
		t.Fatalf("Failed to resolve project root: %v", err)
	}

	exampleDir := filepath.Join(projectRoot, "examples", "flights")
	catalogDir := "catalog"
	configFile := "swcat.yml"
	dotPath := mustFindDotPath(t)

	// Verify paths exist
	if _, err := os.Stat(exampleDir); os.IsNotExist(err) {
		t.Fatalf("Example directory not found at %s", exampleDir)
	}

	// Create source using the local disk store
	st := store.NewDiskStore(exampleDir)

	// Create Server
	opts := ServerOptions{
		Addr:       "localhost:0", // Random port
		BaseDir:    projectRoot,   // Use templates/static from source
		CatalogDir: catalogDir,
		ConfigFile: configFile,
		DotPath:    dotPath,
		ReadOnly:   true,
		Version:    "integration-test",
	}

	server, err := NewServer(opts, st)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Validate catalog to ensure everything is loaded and cache is primed
	if _, err := server.ValidateCatalog(""); err != nil {
		t.Fatalf("Failed to validate catalog: %v", err)
	}

	// Create a test HTTP server using the application's handler
	ts := httptest.NewServer(server.Handler())

	return ts, server
}

func TestIntegration_ServerSmoke(t *testing.T) {
	ts, _ := setupIntegrationServer(t)
	defer ts.Close()

	// List of explicit URLs to check with expected substrings
	testCases := []struct {
		path     string
		expected []string
	}{
		{"/", []string{"<title>", "Components"}}, // Redirects to /ui/components
		{"/ui/components", []string{"Components", "cache-loader"}},
		{"/ui/systems", []string{"Systems", "flights-search"}},
		// The Internal tab should be highlighted:
		{"/ui/systems/flights-frontend?view=internal", []string{
			`border-blue-600">Internal`, "External", "flights-frontend",
		}},
		{"/ui/domains", []string{"Domains", "analytics"}},
		{"/ui/apis", []string{"APIs", "flights-search-api"}},
		{"/ui/resources", []string{"Resources", "routes-database"}},
		{"/ui/groups", []string{"Groups", "flights-owner"}},
		{`/ui/graph`, []string{"diagram"}},
		{`/ui/graph?q=rel%3D%27component%3Aflights-routes%27&e=component%3Aflights-search-backend&e=api%3Aflights-cache-api&e=component%3Aflights-routes`,
			[]string{"routes-database", "flights-routes", "<svg"}},
		{"/static/dist/main.js", []string{"function"}},
		{"/static/dist/main.css", []string{"clickable-node"}},
	}

	client := ts.Client()

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			fullURL := ts.URL + tc.path
			resp, err := client.Get(fullURL)
			if err != nil {
				t.Fatalf("Failed to GET %s: %v", fullURL, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s returned status %d, expected 200", tc.path, resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("Failed to read body from %s: %v", tc.path, err)
			}
			if len(body) == 0 {
				t.Errorf("GET %s returned empty body", tc.path)
			}

			bodyStr := string(body)
			for _, exp := range tc.expected {
				if !strings.Contains(bodyStr, exp) {
					t.Errorf("GET %s: response body missing expected substring %q", tc.path, exp)
				}
			}
		})
	}
}

func TestIntegration_ServerEntities(t *testing.T) {
	ts, s := setupIntegrationServer(t)
	defer ts.Close()

	// Access internal state to get the list of all entities.
	// We use the default ref "" (local disk store).
	sd, err := s.loadStoreData("")
	if err != nil {
		t.Fatalf("Failed to load store data: %v", err)
	}

	// We want to test the detail page for every entity in the repo.
	entities := sd.repo.FindEntities("")
	if len(entities) == 0 {
		t.Fatal("No entities found in repository")
	}

	client := ts.Client()
	// Set a reasonable timeout for the client
	client.Timeout = 10 * time.Second

	for _, entity := range entities {
		ref := entity.GetRef()
		// Construct the UI URL for the entity.
		// The routing scheme in serveMux is:
		// /ui/{kind}/{id}

		var pathPrefix string
		switch ref.Kind {
		case catalog.KindComponent:
			pathPrefix = "/ui/components/"
		case catalog.KindSystem:
			pathPrefix = "/ui/systems/"
		case catalog.KindDomain:
			pathPrefix = "/ui/domains/"
		case catalog.KindAPI:
			pathPrefix = "/ui/apis/"
		case catalog.KindResource:
			pathPrefix = "/ui/resources/"
		case catalog.KindGroup:
			pathPrefix = "/ui/groups/"
		default:
			t.Logf("Skipping unknown kind %s for entity %s", ref.Kind, ref)
			continue
		}

		// Build URL using QName (e.g., "my-ns/my-component"); the kind is encoded in the prefix.
		urlPath := pathPrefix + ref.QName()

		t.Run(fmt.Sprintf("Entity_%s", ref), func(t *testing.T) {
			fullURL := ts.URL + urlPath

			// Retry loop to handle potential timeouts during heavy SVG generation
			var resp *http.Response
			var reqErr error
			var successCancel context.CancelFunc

			for i := 0; i < 3; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				req, _ := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
				resp, reqErr = client.Do(req)

				if reqErr == nil && resp.StatusCode == http.StatusOK {
					successCancel = cancel
					break
				}
				cancel() // Cancel failed attempt
				if resp != nil {
					resp.Body.Close()
				}
				time.Sleep(100 * time.Millisecond)
			}

			if successCancel != nil {
				defer successCancel()
			}

			if reqErr != nil {
				t.Fatalf("Failed to GET %s after retries: %v", fullURL, reqErr)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("GET %s returned status %d, expected 200", urlPath, resp.StatusCode)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Errorf("Failed to read body from %s: %v", urlPath, err)
			}

			bodyStr := string(body)
			if !strings.Contains(bodyStr, "<body") {
				t.Errorf("GET %s: response body missing <body tag", urlPath)
			}
			if !strings.Contains(bodyStr, entity.GetMetadata().Name) {
				t.Errorf("GET %s: response body missing entity name %q", urlPath, entity.GetMetadata().Name)
			}
			if ref.Kind != catalog.KindGroup && !strings.Contains(bodyStr, "<svg") {
				t.Errorf("GET %s: response body missing <svg tag for entity kind %s", urlPath, ref.Kind)
			}
		})
	}
}
