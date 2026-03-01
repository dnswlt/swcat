package lint

import (
	"context"
	"log"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/dnswlt/swcat/internal/bitbucket"
	"github.com/dnswlt/swcat/internal/catalog"
)

// BitbucketFile is a single file found by a Bitbucket code search.
type BitbucketFile struct {
	Path       string // path within the repository
	RepoSlug   string
	ProjectKey string
}

// BitbucketScanResult represents a file found by Bitbucket and the catalog
// entity it was matched to. Entity is nil if no match was found.
type BitbucketScanResult struct {
	File   BitbucketFile
	Entity catalog.Entity
}

// entityLink associates a catalog entity with a canonical Bitbucket URL.
type entityLink struct {
	URL    string // lowercased, canonicalized path
	Entity catalog.Entity
}

// entityIndices holds pre-built lookup structures over a set of catalog entities,
// used by the Bitbucket scan matchers.
type entityIndices struct {
	// byLink maps kind → sorted list of entity links, for prefix-based matching.
	byLink map[catalog.Kind][]entityLink
}

func sortedEntityLinks(entities []catalog.Entity) []entityLink {
	var links []entityLink
	for _, e := range entities {
		// Index by repo link.
		for _, link := range e.GetMetadata().Links {
			lt := strings.ToLower(link.Type)
			if lt != "code" && lt != "bitbucket" {
				continue
			}
			if u, ok := canonicalizeBitbucketURL(link.URL); ok {
				links = append(links, entityLink{URL: u, Entity: e})
			}
		}
	}
	// Sort links for binary search.
	slices.SortFunc(links, func(a, b entityLink) int {
		return strings.Compare(a.URL, b.URL)
	})
	return links
}

// FindBitbucketFiles executes the configured queries against Bitbucket to find files.
func (l *Linter) FindBitbucketFiles(ctx context.Context, bbClient *bitbucket.Client) []BitbucketFile {
	config := l.Bitbucket()
	var allRepos []bitbucket.Repository
	for _, proj := range config.Projects {
		repos, err := bbClient.ListRepositories(ctx, proj)
		if err != nil {
			log.Printf("Error listing repositories for project %q: %v", proj, err)
			continue
		}
		for _, repo := range repos {
			if !slices.Contains(config.ExcludedRepos, repo.Slug) {
				allRepos = append(allRepos, repo)
			}
		}
	}

	var allFiles []BitbucketFile
	for _, q := range config.Queries {
		var re *regexp.Regexp
		if q.PathRegex != "" {
			var err error
			re, err = regexp.Compile(q.PathRegex)
			if err != nil {
				log.Printf("Invalid pathRegex %q: %v", q.PathRegex, err)
				continue
			}
		}

		for _, repo := range allRepos {
			if len(q.Repositories) > 0 && !slices.Contains(q.Repositories, repo.Slug) {
				continue
			}

			if q.Path != "" {
				exists, err := bbClient.FileExists(ctx, repo.Project.Key, repo.Slug, q.Path, "")
				if err != nil {
					log.Printf("Error checking existence of %q in %s/%s: %v", q.Path, repo.Project.Key, repo.Slug, err)
					continue
				}
				if exists {
					allFiles = append(allFiles, BitbucketFile{
						Path:       strings.TrimPrefix(q.Path, "/"),
						RepoSlug:   repo.Slug,
						ProjectKey: repo.Project.Key,
					})
				}
			} else if re != nil {
				files, err := bbClient.ListFiles(ctx, repo.Project.Key, repo.Slug, "")
				if err != nil {
					log.Printf("Error listing files in %s/%s: %v", repo.Project.Key, repo.Slug, err)
					continue
				}
				for _, f := range files {
					if re.MatchString(f) {
						allFiles = append(allFiles, BitbucketFile{
							Path:       f,
							RepoSlug:   repo.Slug,
							ProjectKey: repo.Project.Key,
						})
					}
				}
			}
		}
	}

	return allFiles
}

// MatchBitbucketFiles matches pre-fetched Bitbucket search results against catalog entities.
// entities should be the full set of catalog entities to match against.
//
// For each query, entities' metadata links with type "code" or "bitbucket"
// are matched against the search results' URLs.
func (l *Linter) MatchBitbucketFiles(files []BitbucketFile, entities []catalog.Entity) []BitbucketScanResult {
	links := sortedEntityLinks(entities)
	var out []BitbucketScanResult
	for _, f := range files {
		e, _ := matchBitbucketFileByLinks(f, links)
		out = append(out, BitbucketScanResult{
			File:   f,
			Entity: e,
		})
	}
	return out
}

func matchBitbucketFileByLinks(file BitbucketFile, links []entityLink) (catalog.Entity, bool) {
	repoPrefix := "/projects/" + strings.ToLower(file.ProjectKey) + "/repos/" + strings.ToLower(file.RepoSlug)
	targetURL := buildCanonicalURL(file.ProjectKey, file.RepoSlug, file.Path)

	// Binary search for the insertion point.
	i, _ := slices.BinarySearchFunc(links, targetURL, func(el entityLink, target string) int {
		return strings.Compare(el.URL, target)
	})

	// i is the insertion point where links[i].URL >= targetURL.
	// Potential matches (prefixes) must be at i or before.
	if i >= len(links) {
		i = len(links) - 1
	}
	for j := i; j >= 0; j-- {
		// Optimization: if we left the repository, no further prefix match is possible.
		if !strings.HasPrefix(links[j].URL, repoPrefix) {
			break
		}
		if isURLMatch(targetURL, links[j].URL) {
			return links[j].Entity, true
		}
	}

	return nil, false
}

func isURLMatch(target, prefix string) bool {
	if !strings.HasPrefix(target, prefix) {
		return false
	}
	if len(target) == len(prefix) {
		return true
	}
	// Must be followed by a slash or we were already at a slash.
	return prefix[len(prefix)-1] == '/' || target[len(prefix)] == '/'
}

// buildCanonicalURL constructs a canonical Bitbucket path for a file.
func buildCanonicalURL(projectKey, repoSlug, filePath string) string {
	projectKey = strings.ToLower(projectKey)
	repoSlug = strings.ToLower(repoSlug)
	if filePath == "" {
		return "/projects/" + projectKey + "/repos/" + repoSlug
	}
	return "/projects/" + projectKey + "/repos/" + repoSlug + "/browse/" + strings.TrimLeft(strings.ToLower(filePath), "/")
}

// canonicalizeBitbucketURL parses a Bitbucket URL and returns its canonicalized path.
func canonicalizeBitbucketURL(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	path := strings.ToLower(u.Path)
	_, after, found := strings.Cut(path, "/projects/")
	if !found {
		return "", false
	}
	return "/projects/" + strings.TrimRight(after, "/"), true
}
