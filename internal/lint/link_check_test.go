package lint

import (
	"context"
	"errors"
	"testing"

	"github.com/dnswlt/swcat/internal/bitbucket"
	"github.com/dnswlt/swcat/internal/catalog"
)

func TestParseBitbucketURLPath(t *testing.T) {
	tests := []struct {
		path                         string
		wantProj, wantRepo, wantPath string
		wantOK                       bool
	}{
		{
			path:     "/projects/PROJ/repos/my-repo",
			wantProj: "PROJ", wantRepo: "my-repo", wantPath: "",
			wantOK: true,
		},
		{
			path:     "/projects/PROJ/repos/my-repo/browse",
			wantProj: "PROJ", wantRepo: "my-repo", wantPath: "",
			wantOK: true,
		},
		{
			path:     "/projects/PROJ/repos/my-repo/browse/path/to/File.go",
			wantProj: "PROJ", wantRepo: "my-repo", wantPath: "path/to/File.go",
			wantOK: true,
		},
		{
			path:     "/projects/PROJ/repos/my-repo/browse/dir/",
			wantProj: "PROJ", wantRepo: "my-repo", wantPath: "dir",
			wantOK: true,
		},
		{
			// Wrong literal in slot 4 (commits, not browse).
			path:   "/projects/PROJ/repos/my-repo/commits/abcd",
			wantOK: false,
		},
		{
			path:   "/something/else",
			wantOK: false,
		},
		{
			path:   "/projects/PROJ/repos",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			gotProj, gotRepo, gotPath, ok := parseBitbucketURLPath(tt.path)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if gotProj != tt.wantProj || gotRepo != tt.wantRepo || gotPath != tt.wantPath {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
					gotProj, gotRepo, gotPath, tt.wantProj, tt.wantRepo, tt.wantPath)
			}
		})
	}
}

func TestValidateLink(t *testing.T) {
	bb := &fakeBitbucketSearcher{
		baseURL: "https://bitbucket.example.com",
		files: map[string][]string{
			"PROJ/my-repo": {"path/to/File.go", "dir/inner.txt"},
		},
	}
	l := &Linter{}

	tests := []struct {
		name       string
		fetchers   LinkFetchers
		link       *catalog.Link
		wantStatus LinkCheckStatus
	}{
		{
			name:       "nil link",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       nil,
			wantStatus: LinkCheckSkipped,
		},
		{
			name:       "empty URL",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "code", URL: ""},
			wantStatus: LinkCheckSkipped,
		},
		{
			name:       "unsupported type",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "docs", URL: "https://bitbucket.example.com/projects/PROJ/repos/my-repo"},
			wantStatus: LinkCheckSkipped,
		},
		{
			name:       "host mismatch",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "code", URL: "https://other.example.com/projects/PROJ/repos/my-repo"},
			wantStatus: LinkCheckSkipped,
		},
		{
			name:       "non-bitbucket path",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "code", URL: "https://bitbucket.example.com/dashboard"},
			wantStatus: LinkCheckSkipped,
		},
		{
			name:       "nil fetcher",
			fetchers:   LinkFetchers{},
			link:       &catalog.Link{Type: "code", URL: "https://bitbucket.example.com/projects/PROJ/repos/my-repo"},
			wantStatus: LinkCheckSkipped,
		},
		{
			name:       "repo root exists",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "code", URL: "https://bitbucket.example.com/projects/PROJ/repos/my-repo"},
			wantStatus: LinkCheckOK,
		},
		{
			name:       "browse with no path",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "bitbucket", URL: "https://bitbucket.example.com/projects/PROJ/repos/my-repo/browse"},
			wantStatus: LinkCheckOK,
		},
		{
			name:       "existing file",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "code", URL: "https://bitbucket.example.com/projects/PROJ/repos/my-repo/browse/path/to/File.go"},
			wantStatus: LinkCheckOK,
		},
		{
			name:       "existing directory",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "code", URL: "https://bitbucket.example.com/projects/PROJ/repos/my-repo/browse/dir"},
			wantStatus: LinkCheckOK,
		},
		{
			name:       "missing file",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "code", URL: "https://bitbucket.example.com/projects/PROJ/repos/my-repo/browse/missing.go"},
			wantStatus: LinkCheckBroken,
		},
		{
			name:       "case-sensitive file path mismatch",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "code", URL: "https://bitbucket.example.com/projects/PROJ/repos/my-repo/browse/path/to/file.go"},
			wantStatus: LinkCheckBroken,
		},
		{
			name:       "host comparison is case-insensitive",
			fetchers:   LinkFetchers{Bitbucket: bb},
			link:       &catalog.Link{Type: "code", URL: "https://BITBUCKET.example.com/projects/PROJ/repos/my-repo"},
			wantStatus: LinkCheckOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := l.ValidateLink(context.Background(), tt.fetchers, tt.link)
			if got.Status != tt.wantStatus {
				t.Errorf("status = %v, want %v (reason=%q, err=%v)",
					got.Status, tt.wantStatus, got.Reason, got.Err)
			}
		})
	}
}

// errorBitbucket is a Searcher whose PathExists always fails.
type errorBitbucket struct {
	baseURL string
	err     error
}

func (e *errorBitbucket) BaseURL() string { return e.baseURL }
func (e *errorBitbucket) ListRepositories(_ context.Context, _ string) ([]bitbucket.Repository, error) {
	return nil, nil
}
func (e *errorBitbucket) FileExists(_ context.Context, _, _, _, _ string) (bool, error) {
	return false, nil
}
func (e *errorBitbucket) ListFiles(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, nil
}
func (e *errorBitbucket) PathExists(_ context.Context, _, _, _, _ string) (bool, error) {
	return false, e.err
}

func TestValidateLink_FetcherError(t *testing.T) {
	wantErr := errors.New("network exploded")
	bb := &errorBitbucket{baseURL: "https://bitbucket.example.com", err: wantErr}
	l := &Linter{}

	got := l.ValidateLink(context.Background(), LinkFetchers{Bitbucket: bb},
		&catalog.Link{Type: "code", URL: "https://bitbucket.example.com/projects/PROJ/repos/my-repo"})

	if got.Status != LinkCheckError {
		t.Fatalf("status = %v, want %v", got.Status, LinkCheckError)
	}
	if !errors.Is(got.Err, wantErr) {
		t.Errorf("Err = %v, want wrapping %v", got.Err, wantErr)
	}
}

func TestScanLinks(t *testing.T) {
	bb := &fakeBitbucketSearcher{
		baseURL: "https://bitbucket.example.com",
		files: map[string][]string{
			"PROJ/repo-a": {"existing.go"},
			"PROJ/repo-b": {},
		},
	}
	l := &Linter{}

	entities := []catalog.Entity{
		&catalog.Component{
			Metadata: &catalog.Metadata{
				Name: "comp-a",
				Links: []*catalog.Link{
					// OK: file exists.
					{Type: "code", URL: "https://bitbucket.example.com/projects/PROJ/repos/repo-a/browse/existing.go"},
					// Skipped: not a code link → must not appear in output.
					{Type: "docs", URL: "https://confluence.example.com/page"},
					// Skipped: host mismatch.
					{Type: "code", URL: "https://other.example.com/projects/PROJ/repos/repo-a"},
				},
			},
		},
		&catalog.Component{
			Metadata: &catalog.Metadata{
				Name: "comp-b",
				Links: []*catalog.Link{
					// Broken: file does not exist.
					{Type: "code", URL: "https://bitbucket.example.com/projects/PROJ/repos/repo-a/browse/missing.go"},
					// OK: repo root.
					{Type: "bitbucket", URL: "https://bitbucket.example.com/projects/PROJ/repos/repo-b"},
				},
			},
		},
		// No links → contributes nothing.
		&catalog.Component{
			Metadata: &catalog.Metadata{Name: "comp-c"},
		},
	}

	checks := l.ScanLinks(context.Background(), LinkFetchers{Bitbucket: bb}, entities, 4)

	if len(checks) != 3 {
		t.Fatalf("got %d checks, want 3 (skipped should be filtered out): %+v", len(checks), checks)
	}

	// Build a map keyed by URL for assertions independent of result order.
	byURL := make(map[string]EntityLinkCheck, len(checks))
	for _, c := range checks {
		byURL[c.Link.URL] = c
	}

	cases := []struct {
		url        string
		wantEntity string
		wantStatus LinkCheckStatus
	}{
		{
			url:        "https://bitbucket.example.com/projects/PROJ/repos/repo-a/browse/existing.go",
			wantEntity: "comp-a",
			wantStatus: LinkCheckOK,
		},
		{
			url:        "https://bitbucket.example.com/projects/PROJ/repos/repo-a/browse/missing.go",
			wantEntity: "comp-b",
			wantStatus: LinkCheckBroken,
		},
		{
			url:        "https://bitbucket.example.com/projects/PROJ/repos/repo-b",
			wantEntity: "comp-b",
			wantStatus: LinkCheckOK,
		},
	}
	for _, tc := range cases {
		c, ok := byURL[tc.url]
		if !ok {
			t.Errorf("missing check for %s", tc.url)
			continue
		}
		if c.Result.Status != tc.wantStatus {
			t.Errorf("%s: status = %v, want %v", tc.url, c.Result.Status, tc.wantStatus)
		}
		if c.Entity.GetRef().Name != tc.wantEntity {
			t.Errorf("%s: entity = %q, want %q", tc.url, c.Entity.GetRef().Name, tc.wantEntity)
		}
	}
}

func TestScanLinks_Empty(t *testing.T) {
	l := &Linter{}
	got := l.ScanLinks(context.Background(), LinkFetchers{}, nil, 4)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// Sanity check that the local fake satisfies bitbucket.Searcher; surfaces any
// signature drift at compile time.
var _ bitbucket.Searcher = (*errorBitbucket)(nil)
