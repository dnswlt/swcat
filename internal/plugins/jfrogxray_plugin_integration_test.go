//go:build integration

package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/plugins/sbom"
	"github.com/dnswlt/swcat/internal/repo"
	"gopkg.in/yaml.v3"
)

// sbomJSON builds a minimal CycloneDX BOM JSON string with the given image
// version and library components (each as "name:version").
func sbomJSON(imageVersion string, libs ...string) string {
	type comp struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	components := make([]comp, 0, len(libs))
	for _, lib := range libs {
		name, version, _ := strings.Cut(lib, ":")
		components = append(components, comp{Type: "library", Name: name, Version: version})
	}
	compsJSON, _ := json.Marshal(components)
	return fmt.Sprintf(`{
		"bomFormat": "CycloneDX",
		"specVersion": "1.4",
		"metadata": {
			"component": {"type": "container", "name": "myimage", "version": %q}
		},
		"components": %s
	}`, imageVersion, compsJSON)
}

// makeSBOMZip wraps sbomContent in an in-memory zip archive as "bom.json".
func makeSBOMZip(sbomContent string) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("bom.json")
	if err != nil {
		return nil, err
	}
	if _, err := f.Write([]byte(sbomContent)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// newTestPlugin constructs a JFrogXrayPlugin aimed at the given server URL.
func newTestPlugin(t *testing.T, jfrogURL string) *JFrogXrayPlugin {
	t.Helper()
	specYAML := fmt.Sprintf(`
jfrogUrl: %s
defaultRepository: docker-local
targetAnnotation: swcat/bom
`, jfrogURL)
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(specYAML), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	p, err := NewJFrogXrayBOMPlugin("test", doc.Content[0])
	if err != nil {
		t.Fatalf("NewJFrogXrayBOMPlugin: %v", err)
	}
	return p
}

// fakeEntity returns a Component entity with the given image name.
func fakeEntity(name string) *catalog.Component {
	return &catalog.Component{
		Metadata: &catalog.Metadata{Name: name},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production"},
	}
}

// TestLatestSemverVersions tests the pure tag-sorting helper.
func TestLatestSemverVersions(t *testing.T) {
	tags := []string{"latest", "1.0.0", "v2.3.0", "0.9.1", "v2.3.1", "not-a-version", "v2.3.1-beta"}
	got := latestSemverVersions(tags, 3)
	// v2.3.1-beta > v2.3.0 in semver (higher patch, pre-release < release of same version).
	want := []string{"v2.3.1", "v2.3.1-beta", "v2.3.0"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestJFrogXrayPlugin_Execute tests the happy path: tags fetched, latest version
// SBOM downloaded and parsed, annotations set correctly.
func TestJFrogXrayPlugin_Execute(t *testing.T) {
	const image = "myimage"
	const latestVersion = "1.3.0"

	sbomZip, err := makeSBOMZip(sbomJSON(latestVersion,
		"com.example:alpha:1.0.0",
		"org.acme:beta:2.5.0",
	))
	if err != nil {
		t.Fatalf("makeSBOMZip: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tags/list"):
			json.NewEncoder(w).Encode(TagsResponse{
				Name: image,
				Tags: []string{"0.9.0", latestVersion, "1.2.0", "not-semver"},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/exportDetails"):
			w.Header().Set("Content-Type", "application/zip")
			w.Write(sbomZip)
		default:
			http.Error(w, "unexpected request: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := newTestPlugin(t, srv.URL)
	result, err := p.Execute(context.Background(), fakeEntity(image), &PluginArgs{Repository: repo.NewRepository()})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	wrapper, ok := result.Annotations["swcat/bom"].(map[string]any)
	if !ok {
		t.Fatalf("swcat/bom annotation missing or wrong type: %v", result.Annotations)
	}
	bom, ok := wrapper["$data"].(*sbom.MiniBOM)
	if !ok {
		t.Fatalf("swcat/bom $data missing or wrong type: %v", wrapper)
	}
	if want := "myimage:" + latestVersion; bom.Name != want {
		t.Errorf("bom.Name = %q, want %q", bom.Name, want)
	}
	wantComponents := []string{"com.example:alpha:1.0.0", "org.acme:beta:2.5.0"}
	if len(bom.Components) != len(wantComponents) {
		t.Fatalf("bom.Components = %v, want %v", bom.Components, wantComponents)
	}
	for i, c := range wantComponents {
		if bom.Components[i] != c {
			t.Errorf("bom.Components[%d] = %q, want %q", i, bom.Components[i], c)
		}
	}
}

// TestJFrogXrayPlugin_Execute_Fallback verifies that when the SBOM download for
// the latest version fails, the plugin retries with the next version.
func TestJFrogXrayPlugin_Execute_Fallback(t *testing.T) {
	const image = "myimage"
	const latestVersion = "2.0.0"
	const fallbackVersion = "1.9.0"

	sbomZip, err := makeSBOMZip(sbomJSON(fallbackVersion, "com.example:lib:3.0.0"))
	if err != nil {
		t.Fatalf("makeSBOMZip: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/tags/list"):
			json.NewEncoder(w).Encode(TagsResponse{
				Name: image,
				Tags: []string{latestVersion, fallbackVersion},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/exportDetails"):
			var req SBOMRequest
			json.NewDecoder(r.Body).Decode(&req)
			if strings.Contains(req.Path, latestVersion) {
				http.Error(w, "not indexed yet", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/zip")
			w.Write(sbomZip)
		default:
			http.Error(w, "unexpected request: "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := newTestPlugin(t, srv.URL)
	result, err := p.Execute(context.Background(), fakeEntity(image), &PluginArgs{Repository: repo.NewRepository()})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	wrapper, ok := result.Annotations["swcat/bom"].(map[string]any)
	if !ok {
		t.Fatalf("swcat/bom annotation missing or wrong type: %v", result.Annotations)
	}
	bom, ok := wrapper["$data"].(*sbom.MiniBOM)
	if !ok {
		t.Fatalf("swcat/bom $data missing or wrong type: %v", wrapper)
	}
	if want := "myimage:" + fallbackVersion; bom.Name != want {
		t.Errorf("bom.Name = %q, want %q (fallback)", bom.Name, want)
	}
}
