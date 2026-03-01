package lint

import (
	"context"
	"testing"

	"github.com/dnswlt/swcat/internal/bitbucket"
	"github.com/dnswlt/swcat/internal/catalog"
)

// fakeBitbucketSearcher is a test double for bitbucket.Searcher.
type fakeBitbucketSearcher struct {
	baseURL  string
	repos    map[string][]bitbucket.Repository // project key → repos
	files    map[string][]string               // "projectKey/repoSlug" → file paths
	listCalls int
}

func (f *fakeBitbucketSearcher) BaseURL() string { return f.baseURL }

func (f *fakeBitbucketSearcher) ListRepositories(_ context.Context, projectKey string) ([]bitbucket.Repository, error) {
	f.listCalls++
	return f.repos[projectKey], nil
}

func (f *fakeBitbucketSearcher) FileExists(_ context.Context, projectKey, repoSlug, filePath, _ string) (bool, error) {
	key := projectKey + "/" + repoSlug
	for _, f := range f.files[key] {
		if f == filePath {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeBitbucketSearcher) ListFiles(_ context.Context, projectKey, repoSlug, _ string) ([]string, error) {
	return f.files[projectKey+"/"+repoSlug], nil
}

func TestCanonicalizeBitbucketURL(t *testing.T) {
	tests := []struct {
		url    string
		want   string
		wantOk bool
	}{
		{
			url:    "https://bitbucket.example.com/projects/PROJ/repos/my-repo",
			want:   "/projects/proj/repos/my-repo",
			wantOk: true,
		},
		{
			url:    "https://bitbucket.example.com/projects/PROJ/repos/monorepo/browse/svc-a",
			want:   "/projects/proj/repos/monorepo/browse/svc-a",
			wantOk: true,
		},
		{
			url:    "https://bitbucket.example.com/context/projects/PROJ/repos/monorepo/browse/svc-a/",
			want:   "/projects/proj/repos/monorepo/browse/svc-a",
			wantOk: true,
		},
		{
			url:    "https://other.com/foo",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		got, ok := canonicalizeBitbucketURLPath(tt.url)
		if ok != tt.wantOk {
			t.Errorf("canonicalizeBitbucketURL(%q) ok = %v, want %v", tt.url, ok, tt.wantOk)
		}
		if got != tt.want {
			t.Errorf("canonicalizeBitbucketURL(%q) got = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// TestMatchBitbucketFilesByLinks_BinarySearchBug demonstrates the bug where
// binary search lands at a repo that sorts after the target, causing the
// backwards scan to break immediately before reaching the actual match.
//
// Sorted links: [/projects/p1/repos/my-repo, /projects/p1/repos/xyz/browse/x]
// Target:        /projects/p1/repos/my-repo/browse/catalog.yaml
//
// Binary search insertion point = 1 (xyz entry), because
// "my-repo/browse/catalog.yaml" > "my-repo" but < "xyz/...".
// The backwards scan then checks j=1: HasPrefix(xyz-url, "my-repo") = false → break.
// j=0 is never checked, so the repo-root entity link is missed.
func TestMatchBitbucketFilesByLinks_BinarySearchBug(t *testing.T) {
	file := BitbucketFile{ProjectKey: "P1", RepoSlug: "my-repo", Path: "catalog.yaml"}
	links := sortedEntityLinks([]catalog.Entity{
		&catalog.Component{
			Metadata: &catalog.Metadata{
				Name: "comp-my-repo",
				Links: []*catalog.Link{
					// Repo-root link: sorts before the target file path.
					{Type: "code", URL: "https://bitbucket.example.com/projects/P1/repos/my-repo"},
				},
			},
		},
		&catalog.Component{
			Metadata: &catalog.Metadata{
				Name: "comp-xyz",
				Links: []*catalog.Link{
					// A repo that sorts after "my-repo" pushes the binary search
					// insertion point past the matching entry.
					{Type: "code", URL: "https://bitbucket.example.com/projects/P1/repos/xyz/browse/x"},
				},
			},
		},
	})

	got, ok := matchBitbucketFileByLinks(file, links)
	if !ok {
		t.Fatal("matchBitbucketFileByLinks returned no match, want comp-my-repo")
	}
	if got.GetRef().Name != "comp-my-repo" {
		t.Errorf("got entity %q, want comp-my-repo", got.GetRef().Name)
	}
}

func TestMatchBitbucketFiles(t *testing.T) {
	l := &Linter{}
	entities := []catalog.Entity{
		&catalog.Component{
			Metadata: &catalog.Metadata{
				Name: "comp-root",
				Links: []*catalog.Link{
					{Type: "code", URL: "https://bitbucket.example.com/projects/P1/repos/R1"},
				},
			},
		},
		&catalog.Component{
			Metadata: &catalog.Metadata{
				Name: "comp-sub1",
				Links: []*catalog.Link{
					{Type: "code", URL: "https://bitbucket.example.com/projects/P1/repos/R1/browse/sub1"},
				},
			},
		},
		&catalog.Component{
			Metadata: &catalog.Metadata{
				Name: "comp-sub1-deep",
				Links: []*catalog.Link{
					{Type: "code", URL: "https://bitbucket.example.com/projects/P1/repos/R1/browse/sub1/deep"},
				},
			},
		},
	}

	results := []BitbucketFile{
		{ProjectKey: "P1", RepoSlug: "R1", Path: "other/file.go"},      // Should match comp-root
		{ProjectKey: "P1", RepoSlug: "R1", Path: "sub1/file.go"},       // Should match comp-sub1
		{ProjectKey: "P1", RepoSlug: "R1", Path: "sub1/deep/file.go"},  // Should match comp-sub1-deep
		{ProjectKey: "P1", RepoSlug: "R1", Path: "sub1-extra/file.go"}, // Should match comp-root
		{ProjectKey: "P1", RepoSlug: "R1", Path: ""},                   // Should match comp-root
		{ProjectKey: "P2", RepoSlug: "R2", Path: "file.go"},            // Should match nothing
	}

	scanResults := l.MatchBitbucketFiles(results, entities)

	wants := []string{"comp-root", "comp-sub1", "comp-sub1-deep", "comp-root", "comp-root", ""}
	if len(scanResults) != len(wants) {
		t.Fatalf("expected %d results, got %d", len(wants), len(scanResults))
	}

	for i, res := range scanResults {
		var got string
		if res.Entity != nil {
			got = res.Entity.GetRef().Name
		}
		if got != wants[i] {
			t.Errorf("Result %d (%s): got entity %q, want %q", i, res.File.Path, got, wants[i])
		}
	}
}

func newBitbucketLinter(t *testing.T, cfg BitbucketConfig) *Linter {
	t.Helper()
	l, err := NewLinter(&Config{Bitbucket: cfg}, nil)
	if err != nil {
		t.Fatalf("NewLinter: %v", err)
	}
	return l
}

func TestFindBitbucketFiles_PathQuery(t *testing.T) {
	searcher := &fakeBitbucketSearcher{
		baseURL: "https://bb.example.com",
		repos: map[string][]bitbucket.Repository{
			"PROJ": {
				{Slug: "repo-a", Project: bitbucket.Project{Key: "PROJ"}},
				{Slug: "repo-b", Project: bitbucket.Project{Key: "PROJ"}},
			},
		},
		files: map[string][]string{
			"PROJ/repo-a": {"catalog.yaml"},
			// repo-b does not have the file
		},
	}
	l := newBitbucketLinter(t, BitbucketConfig{
		Projects: []string{"PROJ"},
		Queries:  []BitbucketPathQuery{{Path: "catalog.yaml"}},
	})

	got := l.FindBitbucketFiles(context.Background(), searcher, false)

	if len(got) != 1 {
		t.Fatalf("want 1 file, got %d: %v", len(got), got)
	}
	if got[0].RepoSlug != "repo-a" || got[0].Path != "catalog.yaml" {
		t.Errorf("unexpected file: %+v", got[0])
	}
}

func TestFindBitbucketFiles_PathRegexQuery(t *testing.T) {
	searcher := &fakeBitbucketSearcher{
		baseURL: "https://bb.example.com",
		repos: map[string][]bitbucket.Repository{
			"PROJ": {{Slug: "monorepo", Project: bitbucket.Project{Key: "PROJ"}}},
		},
		files: map[string][]string{
			"PROJ/monorepo": {
				"svc-a/asyncapi.yaml",
				"svc-b/asyncapi.yaml",
				"svc-a/README.md",
			},
		},
	}
	l := newBitbucketLinter(t, BitbucketConfig{
		Projects: []string{"PROJ"},
		Queries:  []BitbucketPathQuery{{PathRegex: `^svc-.*/asyncapi\.yaml$`, Repositories: []string{"monorepo"}}},
	})

	got := l.FindBitbucketFiles(context.Background(), searcher, false)

	want := []BitbucketFile{
		{Path: "svc-a/asyncapi.yaml", RepoSlug: "monorepo", ProjectKey: "PROJ"},
		{Path: "svc-b/asyncapi.yaml", RepoSlug: "monorepo", ProjectKey: "PROJ"},
	}
	if len(got) != len(want) {
		t.Fatalf("want %d files, got %d: %v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("file[%d]: got %+v, want %+v", i, got[i], w)
		}
	}
}

func TestFindBitbucketFiles_RepoExclusion(t *testing.T) {
	searcher := &fakeBitbucketSearcher{
		baseURL: "https://bb.example.com",
		repos: map[string][]bitbucket.Repository{
			"PROJ": {
				{Slug: "keep-this", Project: bitbucket.Project{Key: "PROJ"}},
				{Slug: "skip-this", Project: bitbucket.Project{Key: "PROJ"}},
				{Slug: "also-skip", Project: bitbucket.Project{Key: "PROJ"}},
			},
		},
		files: map[string][]string{
			"PROJ/keep-this": {"catalog.yaml"},
			"PROJ/skip-this": {"catalog.yaml"},
			"PROJ/also-skip": {"catalog.yaml"},
		},
	}
	l := newBitbucketLinter(t, BitbucketConfig{
		Projects:      []string{"PROJ"},
		ExcludedRepos: []string{"skip-.*", "also-skip"},
		Queries:       []BitbucketPathQuery{{Path: "catalog.yaml"}},
	})

	got := l.FindBitbucketFiles(context.Background(), searcher, false)

	if len(got) != 1 || got[0].RepoSlug != "keep-this" {
		t.Errorf("want only keep-this, got %v", got)
	}
}

func TestFindBitbucketFiles_CacheHit(t *testing.T) {
	searcher := &fakeBitbucketSearcher{
		baseURL: "https://bb.example.com",
		repos: map[string][]bitbucket.Repository{
			"PROJ": {{Slug: "repo-a", Project: bitbucket.Project{Key: "PROJ"}}},
		},
		files: map[string][]string{
			"PROJ/repo-a": {"catalog.yaml"},
		},
	}
	l := newBitbucketLinter(t, BitbucketConfig{
		Projects: []string{"PROJ"},
		Queries:  []BitbucketPathQuery{{Path: "catalog.yaml"}},
	})

	// First call populates the cache.
	l.FindBitbucketFiles(context.Background(), searcher, false)
	callsAfterFirst := searcher.listCalls

	// Second call with useCache=true must not call ListRepositories again.
	l.FindBitbucketFiles(context.Background(), searcher, true)
	if searcher.listCalls != callsAfterFirst {
		t.Errorf("cache hit should not call ListRepositories; calls before=%d, after=%d",
			callsAfterFirst, searcher.listCalls)
	}
}
