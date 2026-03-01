//go:build integration

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"slices"

	"github.com/dnswlt/swcat/internal/bitbucket"
	"github.com/dnswlt/swcat/internal/lint"
	"github.com/dnswlt/swcat/internal/prometheus"
	"github.com/dnswlt/swcat/internal/store"
)

// fakePrometheusQuerier is a test double for prometheus.Querier.
type fakePrometheusQuerier struct {
	samples []prometheus.Sample
	err     error
}

func (f *fakePrometheusQuerier) Query(_ context.Context, _ string) ([]prometheus.Sample, error) {
	return f.samples, f.err
}

func newPrometheusTestServer(t *testing.T, querier prometheus.Querier) *Server {
	t.Helper()

	st := store.NewDiskStore("../../testdata/test1")

	linter, err := lint.NewLinter(&lint.Config{
		Prometheus: lint.PrometheusConfig{
			Enabled:           true,
			WorkloadsQuery:    `kube_pod_info`,
			WorkloadNameLabel: "pod",
			DisplayLabels: []lint.DisplayLabel{
				{
					Key:   "namespace",
					Label: "Namespace",
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}

	s, err := NewServer(ServerOptions{
		Addr:    "127.0.0.1:0",
		BaseDir: "../..",
		DotPath: "dot",
	}, st, WithLinter(linter), WithPrometheusClient(querier))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.dotRunner = &fakeRunner{}
	return s
}

func TestServePrometheusWorkloads_OK(t *testing.T) {
	// "test-component" exists as a Component in testdata/test1, so it is tracked.
	// "untracked-service" has no catalog entry, so it is untracked.
	querier := &fakePrometheusQuerier{
		samples: []prometheus.Sample{
			{Labels: map[string]string{"pod": "test-component", "namespace": "prod"}, Value: 1},
			{Labels: map[string]string{"pod": "untracked-service", "namespace": "staging"}, Value: 1},
		},
	}
	s := newPrometheusTestServer(t, querier)
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/ui/lint/prometheus-workloads", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"test-component", "untracked-service"} {
		if !strings.Contains(body, want) {
			t.Errorf("response body missing %q", want)
		}
	}
	if !strings.Contains(body, "Tracked in catalog") {
		t.Errorf("expected tracked indicator for test-component")
	}
	if !strings.Contains(body, "Untracked workload") {
		t.Errorf("expected untracked indicator for untracked-service")
	}
	if !strings.Contains(body, "Namespace") {
		t.Errorf("expected display label column header %q", "Namespace")
	}
	for _, want := range []string{"prod", "staging"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected namespace value %q in display labels", want)
		}
	}
}

func TestServePrometheusWorkloads_NotEnabled(t *testing.T) {
	st := store.NewDiskStore("../../testdata/test1")

	linter, err := lint.NewLinter(&lint.Config{}, nil)
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}

	s, err := NewServer(ServerOptions{
		Addr:    "127.0.0.1:0",
		BaseDir: "../..",
		DotPath: "dot",
	}, st, WithLinter(linter))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.dotRunner = &fakeRunner{}
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/ui/lint/prometheus-workloads", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if want := "not enabled"; !strings.Contains(rr.Body.String(), want) {
		t.Errorf("expected %q in response body, got: %s", want, rr.Body.String())
	}
}

// fakeBBSearcher is a test double for bitbucket.Searcher.
type fakeBBSearcher struct {
	baseURL string
	repos   map[string][]bitbucket.Repository // project key → repos
	files   map[string][]string               // "projectKey/repoSlug" → file paths
}

func (f *fakeBBSearcher) BaseURL() string { return f.baseURL }

func (f *fakeBBSearcher) ListRepositories(_ context.Context, projectKey string) ([]bitbucket.Repository, error) {
	return f.repos[projectKey], nil
}

func (f *fakeBBSearcher) FileExists(_ context.Context, projectKey, repoSlug, filePath, _ string) (bool, error) {
	return slices.Contains(f.files[projectKey+"/"+repoSlug], filePath), nil
}

func (f *fakeBBSearcher) ListFiles(_ context.Context, projectKey, repoSlug, _ string) ([]string, error) {
	return f.files[projectKey+"/"+repoSlug], nil
}

func newBitbucketTestServer(t *testing.T, searcher bitbucket.Searcher) *Server {
	t.Helper()

	st := store.NewDiskStore("../../testdata/test1")

	linter, err := lint.NewLinter(&lint.Config{
		Bitbucket: lint.BitbucketConfig{
			Enabled:  true,
			Projects: []string{"PROJ"},
			Queries:  []lint.BitbucketPathQuery{{Kind: "component", Path: "catalog.yaml"}},
		},
	}, nil)
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}

	s, err := NewServer(ServerOptions{
		Addr:    "127.0.0.1:0",
		BaseDir: "../..",
		DotPath: "dot",
	}, st, WithLinter(linter), WithBitbucketClient(searcher))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.dotRunner = &fakeRunner{}
	return s
}

func TestServeBitbucketResults_OK(t *testing.T) {
	// catalog-repo has catalog.yaml → matches test-component's bitbucket link → tracked.
	// unknown-repo has catalog.yaml → no matching entity → untracked.
	searcher := &fakeBBSearcher{
		baseURL: "https://bb.example.com",
		repos: map[string][]bitbucket.Repository{
			"PROJ": {
				{Slug: "catalog-repo", Project: bitbucket.Project{Key: "PROJ"}},
				{Slug: "unknown-repo", Project: bitbucket.Project{Key: "PROJ"}},
			},
		},
		files: map[string][]string{
			"PROJ/catalog-repo": {"catalog.yaml"},
			"PROJ/unknown-repo": {"catalog.yaml"},
		},
	}
	s := newBitbucketTestServer(t, searcher)
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/ui/lint/bitbucket-results", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Tracked in catalog") {
		t.Errorf("expected tracked indicator for catalog-repo/catalog.yaml")
	}
	if !strings.Contains(body, "Untracked file") {
		t.Errorf("expected untracked indicator for unknown-repo/catalog.yaml")
	}
	// File browse URLs must be rooted at bbClient.BaseURL().
	wantURL := "https://bb.example.com/projects/PROJ/repos/catalog-repo/browse/catalog.yaml"
	if !strings.Contains(body, wantURL) {
		t.Errorf("expected file URL %q in response", wantURL)
	}
}

func TestServeBitbucketResults_UntrackedFilter(t *testing.T) {
	searcher := &fakeBBSearcher{
		baseURL: "https://bb.example.com",
		repos: map[string][]bitbucket.Repository{
			"PROJ": {
				{Slug: "catalog-repo", Project: bitbucket.Project{Key: "PROJ"}},
				{Slug: "unknown-repo", Project: bitbucket.Project{Key: "PROJ"}},
			},
		},
		files: map[string][]string{
			"PROJ/catalog-repo": {"catalog.yaml"},
			"PROJ/unknown-repo": {"catalog.yaml"},
		},
	}
	s := newBitbucketTestServer(t, searcher)
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/ui/lint/bitbucket-results?untracked=on", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "catalog-repo") {
		t.Errorf("tracked file catalog-repo should be filtered out with ?untracked=on")
	}
	if !strings.Contains(body, "unknown-repo") {
		t.Errorf("untracked file unknown-repo should appear with ?untracked=on")
	}
}

func TestServeBitbucketResults_NotEnabled(t *testing.T) {
	st := store.NewDiskStore("../../testdata/test1")

	linter, err := lint.NewLinter(&lint.Config{}, nil)
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}

	s, err := NewServer(ServerOptions{
		Addr:    "127.0.0.1:0",
		BaseDir: "../..",
		DotPath: "dot",
	}, st, WithLinter(linter))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.dotRunner = &fakeRunner{}
	h := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/ui/lint/bitbucket-results", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if want := "not enabled"; !strings.Contains(rr.Body.String(), want) {
		t.Errorf("expected %q in response body, got: %s", want, rr.Body.String())
	}
}
