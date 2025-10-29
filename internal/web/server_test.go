package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
)

// fakeRunner is a fake implementation of dot.Runner.
type fakeRunner struct {
	calls atomic.Int32
}

func (f *fakeRunner) Run(ctx context.Context, dotSource string) ([]byte, error) {
	f.calls.Add(1)
	// Minimal valid SVG; enough for downstream parsing if needed
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"></svg>`), nil
}

// newServer creates a Server with real templates (BaseDir = repo root)
// and a fake dot runner.
func newTestServer(t *testing.T, repo *repo.Repository) *Server {
	t.Helper()

	s, err := NewServer(ServerOptions{
		Addr:    "127.0.0.1:0",
		BaseDir: "../..", // loads templates from <repo-root>/templates
		DotPath: "dot",
	}, repo)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	fr := &fakeRunner{}
	s.dotRunner = fr // ensure we never invoke real dot in handler tests

	return s
}

// ---- Tests ------------------------------------------------------------------

func TestHealth_OK(t *testing.T) {
	repo := repo.NewRepository()
	s := newTestServer(t, repo)
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Body.String(); got != "OK\n" {
		t.Fatalf("body = %q, want %q", got, "OK\n")
	}
}

func TestRoot_Redirect(t *testing.T) {
	repo := repo.NewRepository()
	s := newTestServer(t, repo)
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusTemporaryRedirect)
	}
	if loc := rr.Header().Get("Location"); loc != "/ui/components" {
		t.Fatalf("Location = %q, want %q", loc, "/ui/components")
	}
}

func TestListPages_RenderLinksForAllKinds(t *testing.T) {
	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	h := s.Handler()

	cases := []struct {
		name       string
		path       string
		expectHref string
	}{
		{"components", "/ui/components", "/ui/components/test-component"},
		{"systems", "/ui/systems", "/ui/systems/test-system"},
		{"resources", "/ui/resources", "/ui/resources/test-resource"},
		{"apis", "/ui/apis", "/ui/apis/test-api"},
		{"domains", "/ui/domains", "/ui/domains/test-domain"},
		{"groups", "/ui/groups", "/ui/groups/test-group"},
		{"entities", "/ui/entities", "/ui/systems/test-system"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
			}
			if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Fatalf("Content-Type = %q, want prefix %q", ct, "text/html")
			}

			body := rr.Body.String()
			if !strings.Contains(body, tc.expectHref) {
				max := min(600, len(body))
				t.Fatalf("[%s] expected link %q not found; body (truncated):\n%s",
					tc.name, tc.expectHref, body[:max])
			}
		})
	}
}

func TestComponentDetail_TriggersDotAndCaches(t *testing.T) {
	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo) // real templates, fake dot runner
	h := s.Handler()

	// Access the fake runner to inspect call count
	fr, ok := s.dotRunner.(*fakeRunner)
	if !ok {
		t.Fatalf("dotRunner is not *fakeRunner (got %T)", s.dotRunner)
	}
	before := fr.calls.Load()

	// First request should render and invoke dot once
	rr1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/ui/components/test-component", nil)
	h.ServeHTTP(rr1, req1)

	if rr1.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr1.Code, http.StatusOK)
	}
	if ct := rr1.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want prefix %q", ct, "text/html")
	}
	if body := rr1.Body.String(); !strings.Contains(body, "<svg") {
		t.Fatalf("expected embedded <svg> not found in response body")
	}

	afterFirst := fr.calls.Load()
	if afterFirst != before+1 {
		t.Fatalf("dot.Run calls = %d, want %d", afterFirst, before+1)
	}

	// Second request should hit cache and NOT invoke dot again
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/ui/components/test-component", nil)
	h.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("status (second) = %d, want %d", rr2.Code, http.StatusOK)
	}
	afterSecond := fr.calls.Load()
	if afterSecond != afterFirst {
		t.Fatalf("dot.Run calls after cache = %d, want %d", afterSecond, afterFirst)
	}
}

func TestDetailPages_RenderSVGAndName(t *testing.T) {
	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	h := s.Handler()

	cases := []struct {
		name       string
		path       string
		expectText string // substring we expect to see (entity id)
	}{
		{"component", "/ui/components/test-component", "test-component"},
		{"system", "/ui/systems/test-system", "test-system"},
		{"api", "/ui/apis/test-api", "test-api"},
		{"resource", "/ui/resources/test-resource", "test-resource"},
		{"domain", "/ui/domains/test-domain", "test-domain"},
		// NOTE: groups do NOT render SVG
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
			}
			if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Fatalf("Content-Type = %q, want prefix %q", ct, "text/html")
			}

			body := rr.Body.String()
			if !strings.Contains(body, "<svg") {
				t.Fatalf("[%s] expected embedded <svg> not found", tc.name)
			}
			if !strings.Contains(body, tc.expectText) {
				max := min(600, len(body))
				t.Fatalf("[%s] expected text %q not found; body (truncated):\n%s",
					tc.name, tc.expectText, body[:max])
			}
		})
	}
}

func TestDetail_NotFound_AllKinds(t *testing.T) {
	repo := repo.NewRepository()
	s := newTestServer(t, repo)
	h := s.Handler()

	fr := s.dotRunner.(*fakeRunner)
	start := fr.calls.Load()

	for _, url := range []string{
		"/ui/components/nope",
		"/ui/systems/nope",
		"/ui/resources/nope",
		"/ui/apis/nope",
		"/ui/domains/nope",
		"/ui/groups/nope",
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, url, nil)
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want %d", url, rr.Code, http.StatusNotFound)
		}
	}

	if got := fr.calls.Load(); got != start {
		t.Fatalf("dot.Run should not be called on 404s; calls %d -> %d", start, got)
	}
}
func TestGroupDetail_OK_NoSVG(t *testing.T) {
	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo) // real templates
	h := s.Handler()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/groups/test-group", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want prefix %q", ct, "text/html")
	}

	body := rr.Body.String()
	if !strings.Contains(body, "test-group") {
		t.Fatalf("expected text %q not found; body (truncated):\n%s",
			"test-group", body[:min(600, len(body))])
	}
}

func TestCreateEntity_OK(t *testing.T) {
	// Create a temporary copy of the catalog file
	tmpfile, err := os.CreateTemp("", "catalog-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	src, err := os.Open("../../testdata/catalog.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	_, err = io.Copy(tmpfile, src)
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{tmpfile.Name()})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	h := s.Handler()

	newEntityYAML := `
apiVersion: swcat.io/v1
kind: Component
metadata:
  name: new-component
spec:
  type: service
  lifecycle: experimental
  owner: test-group
  system: test-system
`
	form := url.Values{}
	form.Set("cloned_from", "component:default/test-component")
	form.Set("yaml", newEntityYAML)

	req := httptest.NewRequest(http.MethodPost, "/ui/entities", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	redirectURL := rr.Header().Get("HX-Redirect")
	if redirectURL != "/ui/components/new-component" {
		t.Fatalf("HX-Redirect = %q, want %q", redirectURL, "/ui/components/new-component")
	}

	// Check if the entity was actually created in the repo
	ref, _ := catalog.ParseRef("component:default/new-component")
	if s.repo.Entity(ref) == nil {
		t.Fatalf("entity was not created in the repository")
	}

	// Check if the file was updated
	updatedContent, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updatedContent), "new-component") {
		t.Fatalf("new component was not written to the catalog file")
	}
}

func TestCreateEntity_ReadOnly(t *testing.T) {
	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	s.opts.ReadOnly = true // Set read-only mode
	h := s.Handler()

	newEntityYAML := `
apiVersion: swcat.io/v1
kind: Component
metadata:
  name: new-component
spec:
  type: service
  lifecycle: experimental
  owner: test-group
  system: test-system
`
	form := url.Values{}
	form.Set("cloned_from", "component:default/test-component")
	form.Set("yaml", newEntityYAML)

	req := httptest.NewRequest(http.MethodPost, "/ui/entities", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusPreconditionFailed)
	}
}

func TestCreateEntity_MissingSystem(t *testing.T) {
	// Create a temporary copy of the catalog file
	tmpfile, err := os.CreateTemp("", "catalog-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	src, err := os.Open("../../testdata/catalog.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	_, err = io.Copy(tmpfile, src)
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{tmpfile.Name()})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	h := s.Handler()

	newEntityYAML := `
apiVersion: swcat.io/v1
kind: Component
metadata:
  name: new-component
spec:
  type: service
  lifecycle: experimental
  owner: test-group
`
	form := url.Values{}
	form.Set("cloned_from", "component:default/test-component")
	form.Set("yaml", newEntityYAML)

	req := httptest.NewRequest(http.MethodPost, "/ui/entities", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	expectedError := "Invalid entity"
	if !strings.Contains(body, expectedError) {
		t.Fatalf("expected validation error %q not found in body: %s", expectedError, body)
	}
}

func TestUpdateEntity_OK(t *testing.T) {
	// Create a temporary copy of the catalog file
	tmpfile, err := os.CreateTemp("", "catalog-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	src, err := os.Open("../../testdata/catalog.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	_, err = io.Copy(tmpfile, src)
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{tmpfile.Name()})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	h := s.Handler()

	updatedEntityYAML := `
apiVersion: swcat.io/v1
kind: Component
metadata:
  name: test-component
  title: Updated Test Component
spec:
  type: service
  lifecycle: production
  owner: test-group
  system: test-system
`
	form := url.Values{}
	form.Set("yaml", updatedEntityYAML)

	req := httptest.NewRequest(http.MethodPost, "/ui/entities/component:default%2Ftest-component/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	redirectURL := rr.Header().Get("HX-Redirect")
	if redirectURL != "/ui/components/test-component" {
		t.Fatalf("HX-Redirect = %q, want %q", redirectURL, "/ui/components/test-component")
	}

	// Check if the entity was actually updated in the repo
	ref, _ := catalog.ParseRef("component:default/test-component")
	entity := s.repo.Entity(ref)
	if entity == nil {
		t.Fatalf("entity not found in the repository")
	}
	if entity.GetMetadata().Title != "Updated Test Component" {
		t.Fatalf("entity was not updated in the repository")
	}

	// Check if the file was updated
	updatedContent, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updatedContent), "Updated Test Component") {
		t.Fatalf("updated component was not written to the catalog file")
	}
}

func TestUpdateEntity_ReadOnly(t *testing.T) {
	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	s.opts.ReadOnly = true // Set read-only mode
	h := s.Handler()

	updatedEntityYAML := `
apiVersion: swcat.io/v1
kind: Component
metadata:
  name: test-component
  title: Updated Test Component
spec:
  type: service
  lifecycle: production
  owner: test-group
  system: test-system
`
	form := url.Values{}
	form.Set("yaml", updatedEntityYAML)

	req := httptest.NewRequest(http.MethodPost, "/ui/entities/component:default%2Ftest-component/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusPreconditionFailed)
	}
}

func TestUpdateEntity_NotFound(t *testing.T) {
	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	h := s.Handler()

	updatedEntityYAML := `
apiVersion: swcat.io/v1
kind: Component
metadata:
  name: not-found-component
  title: Updated Test Component
spec:
  type: service
  lifecycle: production
  owner: test-group
  system: test-system
`
	form := url.Values{}
	form.Set("yaml", updatedEntityYAML)

	req := httptest.NewRequest(http.MethodPost, "/ui/entities/component:default%2Fnot-found-component/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}

func TestUpdateEntity_IDChange(t *testing.T) {
	// Create a temporary copy of the catalog file
	tmpfile, err := os.CreateTemp("", "catalog-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	src, err := os.Open("../../testdata/catalog.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	_, err = io.Copy(tmpfile, src)
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{tmpfile.Name()})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	h := s.Handler()

	updatedEntityYAML := `
apiVersion: swcat.io/v1
kind: Component
metadata:
  name: new-name-for-component
  title: Updated Test Component
spec:
  type: service
  lifecycle: production
  owner: test-group
  system: test-system
`
	form := url.Values{}
	form.Set("yaml", updatedEntityYAML)

	req := httptest.NewRequest(http.MethodPost, "/ui/entities/component:default%2Ftest-component/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Updated entity ID does not match original") {
		t.Fatalf("expected error message not found in body: %s", body)
	}
}

func TestDeleteEntity_OK(t *testing.T) {
	// Create a temporary copy of the catalog file
	tmpfile, err := os.CreateTemp("", "catalog-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	src, err := os.Open("../../testdata/catalog.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()

	_, err = io.Copy(tmpfile, src)
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{tmpfile.Name()})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	h := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/ui/entities/component:default%2Ftest-component/delete", nil)
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	redirectURL := rr.Header().Get("HX-Redirect")
	if redirectURL != "/ui/systems/test-system" {
		t.Fatalf("HX-Redirect = %q, want %q", redirectURL, "/ui/systems/test-system")
	}

	// Check if the entity was actually deleted from the repo
	ref, _ := catalog.ParseRef("component:default/test-component")
	if s.repo.Entity(ref) != nil {
		t.Fatalf("entity was not deleted from the repository")
	}

	// Check if the file was updated
	updatedContent, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(updatedContent), "name: test-component") {
		t.Fatalf("deleted component was found in the catalog file")
	}
}

func TestDeleteEntity_ReadOnly(t *testing.T) {
	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	s.opts.ReadOnly = true // Set read-only mode
	h := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/ui/entities/component:default%2Ftest-component/delete", nil)
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusPreconditionFailed)
	}
}

func TestDeleteEntity_NotFound(t *testing.T) {
	repo, err := repo.LoadRepositoryFromPaths(repo.Config{}, []string{"../../testdata/catalog.yml"})
	if err != nil {
		t.Fatalf("failed to load repository: %v", err)
	}
	s := newTestServer(t, repo)
	h := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/ui/entities/component:default%2Fnot-found-component/delete", nil)
	req.Header.Set("HX-Request", "true")

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}
}
