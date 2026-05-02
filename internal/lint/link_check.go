package lint

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/dnswlt/swcat/internal/bitbucket"
	"github.com/dnswlt/swcat/internal/catalog"
)

// LinkFetchers bundles the external clients ValidateLink uses to test
// catalog entity links. Each field is optional; a nil field means links of
// that source cannot be checked and ValidateLink will return LinkCheckSkipped
// for them.
type LinkFetchers struct {
	Bitbucket bitbucket.Searcher
}

// Names returns the human-readable names of the link sources that are
// active (i.e. backed by a non-nil fetcher) in this LinkFetchers.
func (f LinkFetchers) Names() []string {
	var names []string
	if f.Bitbucket != nil {
		names = append(names, "Bitbucket")
	}
	return names
}

// LinkCheckStatus is the outcome of validating a single link.
type LinkCheckStatus int

const (
	// LinkCheckSkipped means no fetcher was able to handle the link
	// (e.g. an unsupported URL or a link.Type that isn't "code"/"bitbucket").
	LinkCheckSkipped LinkCheckStatus = iota
	// LinkCheckOK means the link target was found.
	LinkCheckOK
	// LinkCheckBroken means the link target was not found.
	LinkCheckBroken
	// LinkCheckError means the check could not be completed due to a
	// transport or API error (distinct from "not found").
	LinkCheckError
	// LinkCheckNoAccess means the source rejected the request for
	// authorization reasons (HTTP 401/403). The link target may or may
	// not exist; we cannot say either way.
	LinkCheckNoAccess
)

// LinkCheckResult is the outcome of a single ValidateLink call.
type LinkCheckResult struct {
	Status LinkCheckStatus
	// Reason is a short human-readable explanation, suitable for display.
	Reason string
	// Err is set when Status is LinkCheckError.
	Err error
}

// EntityLinkCheck pairs a single link with the entity it belongs to and the
// result of validating it.
type EntityLinkCheck struct {
	Entity catalog.Entity
	Link   *catalog.Link
	Result LinkCheckResult
}

// ValidateLink checks a single link against the configured fetchers.
// It returns LinkCheckSkipped if no fetcher claimed the URL.
func (l *Linter) ValidateLink(ctx context.Context, f LinkFetchers, link *catalog.Link) LinkCheckResult {
	if link == nil || link.URL == "" {
		return LinkCheckResult{Status: LinkCheckSkipped, Reason: "empty URL"}
	}
	if res, ok := validateBitbucketLink(ctx, f.Bitbucket, link); ok {
		return res
	}
	return LinkCheckResult{Status: LinkCheckSkipped}
}

// validateBitbucketLink tests link against the given bitbucket client.
// The second return is false if bitbucket should not handle this link.
func validateBitbucketLink(ctx context.Context, bb bitbucket.Searcher, link *catalog.Link) (LinkCheckResult, bool) {
	if bb == nil {
		return LinkCheckResult{}, false
	}
	lt := strings.ToLower(link.Type)
	if lt != "code" && lt != "bitbucket" {
		return LinkCheckResult{}, false
	}

	u, err := url.Parse(link.URL)
	if err != nil {
		return LinkCheckResult{}, false
	}
	base, err := url.Parse(bb.BaseURL())
	if err != nil {
		return LinkCheckResult{}, false
	}
	if !strings.EqualFold(u.Host, base.Host) {
		return LinkCheckResult{}, false
	}

	projectKey, repoSlug, path, ok := parseBitbucketURLPath(u.Path)
	if !ok {
		return LinkCheckResult{}, false
	}

	exists, err := bb.PathExists(ctx, projectKey, repoSlug, path, "")
	if err != nil {
		var apiErr *bitbucket.APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden) {
			return LinkCheckResult{
				Status: LinkCheckNoAccess,
				Reason: "no access to Bitbucket project",
			}, true
		}
		return LinkCheckResult{
			Status: LinkCheckError,
			Reason: "Bitbucket request failed",
			Err:    err,
		}, true
	}
	if !exists {
		return LinkCheckResult{
			Status: LinkCheckBroken,
			Reason: "path not found in Bitbucket",
		}, true
	}
	return LinkCheckResult{Status: LinkCheckOK}, true
}

// parseBitbucketURLPath extracts (projectKey, repoSlug, path) from a Bitbucket
// path of the form:
//
//	/projects/<KEY>/repos/<SLUG>
//	/projects/<KEY>/repos/<SLUG>/browse
//	/projects/<KEY>/repos/<SLUG>/browse/<path>
//
// path is empty when the URL points to the repo root or /browse with no
// further path. Case is preserved (the API treats project/repo
// case-insensitively but file paths are case-sensitive).
func parseBitbucketURLPath(p string) (projectKey, repoSlug, path string, ok bool) {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) < 4 || parts[0] != "projects" || parts[2] != "repos" {
		return "", "", "", false
	}
	projectKey = parts[1]
	repoSlug = parts[3]
	if len(parts) == 4 {
		return projectKey, repoSlug, "", true
	}
	if parts[4] != "browse" {
		return "", "", "", false
	}
	if len(parts) == 5 {
		return projectKey, repoSlug, "", true
	}
	return projectKey, repoSlug, strings.Join(parts[5:], "/"), true
}

// ScanLinks validates every link on every entity in parallel, with at most
// `concurrency` checks in flight. Links for which no fetcher can be used
// (LinkCheckSkipped) are omitted from the result. Concurrency is clamped to
// at least 1.
func (l *Linter) ScanLinks(ctx context.Context, f LinkFetchers, entities []catalog.Entity, concurrency int) []EntityLinkCheck {
	if concurrency < 1 {
		concurrency = 1
	}

	type job struct {
		entity catalog.Entity
		link   *catalog.Link
	}
	var jobs []job
	for _, e := range entities {
		for _, link := range e.GetMetadata().Links {
			jobs = append(jobs, job{entity: e, link: link})
		}
	}

	results := make([]LinkCheckResult, len(jobs))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j job) {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = l.ValidateLink(ctx, f, j.link)
		}(i, j)
	}
	wg.Wait()

	var out []EntityLinkCheck
	for i, j := range jobs {
		if results[i].Status == LinkCheckSkipped {
			continue
		}
		out = append(out, EntityLinkCheck{
			Entity: j.entity,
			Link:   j.link,
			Result: results[i],
		})
	}
	return out
}
