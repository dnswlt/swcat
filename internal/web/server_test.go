package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

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
	repo, err := repo.LoadRepositoryFromPaths([]string{"../../testdata/catalog.yml"})
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
	repo, err := repo.LoadRepositoryFromPaths([]string{"../../testdata/catalog.yml"})
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
	repo, err := repo.LoadRepositoryFromPaths([]string{"../../testdata/catalog.yml"})
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
	repo, err := repo.LoadRepositoryFromPaths([]string{"../../testdata/catalog.yml"})
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
