package lint

import (
	"context"
	"log"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/dnswlt/swcat/internal/bitbucket"
	"github.com/dnswlt/swcat/internal/catalog"
)

// bbFilesCache holds the last result of FindBitbucketFiles for a single Bitbucket instance.
type bbFilesCache struct {
	mu      sync.Mutex
	baseURL string
	valid   bool
	files   []BitbucketFile
}

// get returns the cached files if the cache is valid for the given baseURL.
func (c *bbFilesCache) get(baseURL string) ([]BitbucketFile, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.valid && c.baseURL == baseURL {
		return c.files, true
	}
	return nil, false
}

// set stores files in the cache for the given baseURL.
func (c *bbFilesCache) set(baseURL string, files []BitbucketFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.baseURL = baseURL
	c.files = files
	c.valid = true
}

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
	// Lowercased, canonicalized repository path
	// Examples:
	// 	 /projects/my_project/repos/my_repo
	// 	 /projects/my_project/repos/my_repo/browse/path/to/file.txt
	canonicalPath string
	// The associated entity
	entity catalog.Entity
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
			if u, ok := canonicalizeBitbucketURLPath(link.URL); ok {
				links = append(links, entityLink{canonicalPath: u, entity: e})
			}
		}
	}
	// Sort links for binary search.
	slices.SortFunc(links, func(a, b entityLink) int {
		return strings.Compare(a.canonicalPath, b.canonicalPath)
	})
	return links
}

// FindBitbucketFiles executes the configured queries against Bitbucket to find files.
// If useCache is true and a cached result exists for the same Bitbucket base URL, it is
// returned immediately without hitting the API.
func (l *Linter) FindBitbucketFiles(ctx context.Context, bbClient *bitbucket.Client, useCache bool) []BitbucketFile {
	if useCache {
		if files, ok := l.bbCache.get(bbClient.BaseURL()); ok {
			log.Printf("FindBitbucketFiles: returning %d cached files", len(files))
			return files
		}
	}
	config := l.Bitbucket()
	var allRepos []bitbucket.Repository
	excludeREs := make([]*regexp.Regexp, 0, len(l.config.Bitbucket.ExcludedRepos))
	for _, pattern := range l.config.Bitbucket.ExcludedRepos {
		r, err := regexp.Compile(`^(?:` + pattern + `)$`)
		if err != nil {
			continue // Shouldn't happen, we validate regexps when reading the config
		}
		excludeREs = append(excludeREs, r)
	}
	for _, proj := range config.Projects {
		repos, err := bbClient.ListRepositories(ctx, proj)
		if err != nil {
			log.Printf("Error listing repositories for project %q: %v", proj, err)
			continue
		}
		for _, repo := range repos {
			exclude := false
			for _, ex := range excludeREs {
				if ex.MatchString(repo.Slug) {
					exclude = true
					break
				}
			}
			if !exclude {
				allRepos = append(allRepos, repo)
			}
		}
	}

	var allFiles []BitbucketFile

	for _, repo := range allRepos {
		lenBefore := len(allFiles)
		nQueries := 0
		for _, q := range config.Queries {
			if len(q.Repositories) > 0 && !slices.Contains(q.Repositories, repo.Slug) {
				continue
			}
			nQueries++

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
			} else if q.PathRegex != "" {
				files, err := bbClient.ListFiles(ctx, repo.Project.Key, repo.Slug, "")
				if err != nil {
					log.Printf("Error listing files in %s/%s: %v", repo.Project.Key, repo.Slug, err)
					continue
				}
				re, err := regexp.Compile(q.PathRegex)
				if err != nil {
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
		if nQueries > 0 {
			log.Printf("FindBitbucketFiles found %d files with %d queries in repo %s/%s",
				len(allFiles)-lenBefore, nQueries, repo.Project.Key, repo.Slug)
		}
	}

	l.bbCache.set(bbClient.BaseURL(), allFiles)
	return allFiles
}

// MatchBitbucketFiles matches pre-fetched Bitbucket search results against catalog entities.
// entities should be the full set of catalog entities to match against.
//
// For each query, entities' metadata links with type "code" or "bitbucket"
// are matched against the search results' URLs.
func (l *Linter) MatchBitbucketFiles(files []BitbucketFile, entities []catalog.Entity) []BitbucketScanResult {
	links := sortedEntityLinks(entities)
	log.Printf("Found %d entities with code/bitbucket links for matching", len(links))

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
		return strings.Compare(el.canonicalPath, target)
	})

	// i is the insertion point where links[i].URL >= targetURL.
	// Potential matches (prefixes) must be at i or before.
	if i >= len(links) {
		i = len(links) - 1
	}
	for j := i; j >= 0; j-- {
		// Optimization: if we're past all URLs that could share the repo prefix, stop.
		// URLs from repos sorting before ours are < repoPrefix; nothing earlier can match.
		// (URLs from repos sorting after ours are > repoPrefix; we must keep scanning past them.)
		if links[j].canonicalPath < repoPrefix {
			break
		}
		if isURLMatch(targetURL, links[j].canonicalPath) {
			return links[j].entity, true
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

// canonicalizeBitbucketURLPath parses a Bitbucket URL and returns its canonicalized path.
func canonicalizeBitbucketURLPath(rawURL string) (string, bool) {
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
