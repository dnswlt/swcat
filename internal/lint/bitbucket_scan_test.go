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
