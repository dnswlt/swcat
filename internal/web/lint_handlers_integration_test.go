//go:build integration

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
