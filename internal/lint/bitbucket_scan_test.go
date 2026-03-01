package lint

import (
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
)

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
		got, ok := canonicalizeBitbucketURL(tt.url)
		if ok != tt.wantOk {
			t.Errorf("canonicalizeBitbucketURL(%q) ok = %v, want %v", tt.url, ok, tt.wantOk)
		}
		if got != tt.want {
			t.Errorf("canonicalizeBitbucketURL(%q) got = %q, want %q", tt.url, got, tt.want)
		}
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
