package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/lint"
	"github.com/dnswlt/swcat/internal/store"
)

// newFindingsServer returns a server over testdata/linting with the linter
// enabled but no external clients, which is the common deployment shape: lint
// findings work, the scans are not configured.
func newFindingsServer(t *testing.T) http.Handler {
	t.Helper()
	testDataDir := "../../testdata/linting"

	lintCfg, err := lint.ReadConfig(filepath.Join(testDataDir, "lint.yml"))
	if err != nil {
		t.Fatalf("failed to load lint config: %v", err)
	}
	linter, err := lint.NewLinter(lintCfg)
	if err != nil {
		t.Fatalf("failed to create linter: %v", err)
	}

	s, err := NewServer(ServerOptions{
		Addr:    "127.0.0.1:0",
		BaseDir: "../..",
		DotPath: "dot",
	}, store.NewDiskStore(testDataDir), WithLinter(linter))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.dotRunner = &fakeRunner{}
	return s.Handler()
}

// postFindings sends body to the endpoint and returns the status and decoded
// response. body may be empty, standing in for a request with no body at all.
func postFindings(t *testing.T, h http.Handler, body string) (int, map[string]any) {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(http.MethodPost, "/catalog/findings", nil)
	} else {
		r = httptest.NewRequest(http.MethodPost, "/catalog/findings", strings.NewReader(body))
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)

	if rr.Code != http.StatusOK {
		return rr.Code, nil
	}
	var out map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, rr.Body.String())
	}
	return rr.Code, out
}

func TestFindings_EmptyBodyReturnsLintOnly(t *testing.T) {
	h := newFindingsServer(t)

	for _, body := range []string{"", "{}"} {
		code, out := postFindings(t, h, body)
		if code != http.StatusOK {
			t.Fatalf("body %q: status = %d, want 200", body, code)
		}
		lintSection, ok := out["lint"].(map[string]any)
		if !ok {
			t.Fatalf("body %q: no lint section in %v", body, out)
		}
		findings, _ := lintSection["findings"].([]any)
		if len(findings) == 0 {
			t.Errorf("body %q: expected lint findings, got none", body)
		}
		// Nothing expensive may run unless it was asked for by name.
		for _, scan := range []string{"prometheusWorkloads", "kubeWorkloads", "bitbucketFiles", "linkCheck"} {
			if _, present := out[scan]; present {
				t.Errorf("body %q: %s ran without being requested", body, scan)
			}
		}
	}
}

func TestFindings_FindingShape(t *testing.T) {
	h := newFindingsServer(t)
	_, out := postFindings(t, h, `{"lint": {"query": "kind:api"}}`)

	findings := out["lint"].(map[string]any)["findings"].([]any)
	if len(findings) == 0 {
		t.Fatal("no findings for kind:api")
	}
	for _, f := range findings {
		f := f.(map[string]any)
		entity, ok := f["entity"].(map[string]any)
		if !ok {
			t.Fatalf("finding has no entity: %v", f)
		}
		if entity["kind"] != "API" {
			t.Errorf("query kind:api returned a %v finding", entity["kind"])
		}
		for _, field := range []string{"ruleName", "severity", "message"} {
			if s, _ := f[field].(string); s == "" {
				t.Errorf("finding is missing %s: %v", field, f)
			}
		}
	}
}

func TestFindings_UnconfiguredScanIsReportedNotEmpty(t *testing.T) {
	h := newFindingsServer(t)
	_, out := postFindings(t, h, `{"scans": ["SCAN_PROMETHEUS_WORKLOADS"]}`)

	section, ok := out["prometheusWorkloads"].(map[string]any)
	if !ok {
		t.Fatalf("no prometheusWorkloads section in %v", out)
	}
	status, _ := section["status"].(map[string]any)
	if got := status["state"]; got != "STATE_NOT_CONFIGURED" {
		t.Errorf("state = %v, want STATE_NOT_CONFIGURED", got)
	}
	if _, present := section["workloads"]; present {
		t.Error("unconfigured scan returned workloads")
	}
	// The lint section must still be there: one unavailable scan does not
	// suppress the rest of the response.
	if _, ok := out["lint"]; !ok {
		t.Error("lint section missing when a scan was not configured")
	}
}

func TestFindings_BadRequests(t *testing.T) {
	h := newFindingsServer(t)

	tests := []struct {
		name string
		body string
	}{
		{"unknown scan name", `{"scans": ["SCAN_NOPE"]}`},
		{"unspecified scan", `{"scans": ["SCAN_UNSPECIFIED"]}`},
		{"options without scan", `{"bitbucketFiles": {"refresh": true}}`},
		{"unknown field", `{"lnit": {}}`},
		{"malformed query", `{"lint": {"query": "kind:("}}`},
		{"not json", `not json`},
		// protojson accepts numeric enum values, including ones the enum never
		// declared. Left unchecked, 99 falls through the handler's switch and
		// the caller gets a lint-only 200 that looks like a completed scan.
		{"undeclared numeric scan", `{"scans": [99]}`},
		// A repeated scan would run twice and overwrite its own section.
		{"duplicate scan", `{"scans": ["SCAN_BITBUCKET_FILES", "SCAN_BITBUCKET_FILES"]}`},
		{"duplicate scan, numeric", `{"scans": [3, 3]}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := postFindings(t, h, tc.body)
			if code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", code)
			}
		})
	}
}

// TestFindings_NumericScanAccepted guards the fix for undeclared enum values
// from over-reaching: a numeric value that the enum does declare is still a
// valid way to name a scan.
func TestFindings_NumericScanAccepted(t *testing.T) {
	h := newFindingsServer(t)

	code, out := postFindings(t, h, `{"scans": [2]}`)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if _, ok := out["kubeWorkloads"]; !ok {
		t.Errorf("scan 2 (SCAN_KUBE_WORKLOADS) did not run: %v", out)
	}
}
