package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	HTTPAccessTokenUser = "x-token-auth"
)

// ClientOptions holds optional configuration for the Client.
type ClientOptions struct {
	// Username and Password enable HTTP Basic Auth.
	Username string
	Password string
	// Bitbucket HTTP Access Tokens by project.
	// Username is always "x-token-auth" in case these are used.
	PerProjectTokens map[string]string
	// Timeout for HTTP requests. Defaults to 30s.
	Timeout time.Duration
}

// Client is a client for the Bitbucket Data Center REST API.
type Client struct {
	baseURL    string
	opts       ClientOptions
	httpClient *http.Client
}

// NewClient creates a new Bitbucket Data Center client targeting baseURL
// (e.g. "https://bitbucket.example.com").
func NewClient(baseURL string, opts ClientOptions) *Client {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		opts:    opts,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

// Person represents a Bitbucket user (author or committer).
type Person struct {
	Name         string `json:"name"`
	EmailAddress string `json:"emailAddress"`
}

// CommitParent is a minimal reference to a parent commit.
type CommitParent struct {
	ID        string `json:"id"`
	DisplayID string `json:"displayId"`
}

// Commit represents a single commit returned by the Bitbucket API.
type Commit struct {
	ID                 string         `json:"id"`
	DisplayID          string         `json:"displayId"`
	Author             Person         `json:"author"`
	AuthorTimestamp    int64          `json:"authorTimestamp"`
	Committer          Person         `json:"committer"`
	CommitterTimestamp int64          `json:"committerTimestamp"`
	Message            string         `json:"message"`
	Parents            []CommitParent `json:"parents"`
}

// AuthorTime returns the author timestamp as a time.Time.
func (c *Commit) AuthorTime() time.Time {
	return time.UnixMilli(c.AuthorTimestamp)
}

// CommitterTime returns the committer timestamp as a time.Time.
func (c *Commit) CommitterTime() time.Time {
	return time.UnixMilli(c.CommitterTimestamp)
}

// pagedResponse is the generic paged response envelope returned by the Bitbucket API.
type pagedResponse[T any] struct {
	Values        []T  `json:"values"`
	IsLastPage    bool `json:"isLastPage"`
	Size          int  `json:"size"`
	Start         int  `json:"start"`
	Limit         int  `json:"limit"`
	NextPageStart int  `json:"nextPageStart"`
}

// GetCommitsOptions holds optional parameters for GetCommits.
type GetCommitsOptions struct {
	// Until restricts commits to those reachable from this commit or ref.
	// If empty, the default branch HEAD is used.
	Until string
	// Since restricts commits to those after this commit (exclusive).
	Since string
	// Limit is the maximum number of commits to return per page.
	// 0 uses the server default (typically 25).
	Limit int
}

// doGet performs a GET request to u, decodes the JSON response into target,
// and returns any transport or API error.
func (c *Client) doGet(ctx context.Context, u *url.URL, target any, projectKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("bitbucket: creating request for %s: %w", u.Path, err)
	}
	c.setAuth(req, projectKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bitbucket: executing request for %s: %w", u.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.apiError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("bitbucket: decoding response for %s: %w", u.Path, err)
	}
	return nil
}

// GetCommits returns commits for the given project and repository, following
// pagination. If opts.Limit > 0, at most that many commits are returned across
// all pages. If opts.Limit == 0, only the first page is returned (commits in a
// large repository are unbounded, so full pagination requires an explicit limit).
//
// API: GET /rest/api/latest/projects/{projectKey}/repos/{repositorySlug}/commits
func (c *Client) GetCommits(ctx context.Context, projectKey, repoSlug string, opts GetCommitsOptions) ([]Commit, error) {
	u, err := url.Parse(fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/commits",
		c.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug)))
	if err != nil {
		return nil, fmt.Errorf("bitbucket: building commits URL: %w", err)
	}

	// Seed fixed query params (until/since) once; start/limit vary per page.
	base := u.Query()
	if opts.Until != "" {
		base.Set("until", opts.Until)
	}
	if opts.Since != "" {
		base.Set("since", opts.Since)
	}

	var all []Commit
	start := 0
	for {
		q := base
		q.Set("start", fmt.Sprintf("%d", start))
		if opts.Limit > 0 {
			// Request only as many as we still need so the server does less work.
			remaining := opts.Limit - len(all)
			q.Set("limit", fmt.Sprintf("%d", remaining))
		}
		u.RawQuery = q.Encode()

		var paged pagedResponse[Commit]
		if err := c.doGet(ctx, u, &paged, projectKey); err != nil {
			return nil, err
		}
		all = append(all, paged.Values...)

		if paged.IsLastPage || opts.Limit == 0 || len(all) >= opts.Limit {
			break
		}
		start = paged.NextPageStart
	}
	return all, nil
}

// Branch represents a Bitbucket repository branch.
type Branch struct {
	ID           string `json:"id"`
	DisplayID    string `json:"displayId"`
	Type         string `json:"type"`
	LatestCommit string `json:"latestCommit"`
	IsDefault    bool   `json:"isDefault"`
}

// ListBranches returns all branches for the given project and repository,
// following pagination to collect the complete list.
//
// API: GET /rest/api/latest/projects/{projectKey}/repos/{repositorySlug}/branches
func (c *Client) ListBranches(ctx context.Context, projectKey, repoSlug string) ([]Branch, error) {
	u, err := url.Parse(fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/branches",
		c.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug)))
	if err != nil {
		return nil, fmt.Errorf("bitbucket: building branches URL: %w", err)
	}

	var all []Branch
	start := 0
	for {
		q := u.Query()
		q.Set("start", fmt.Sprintf("%d", start))
		u.RawQuery = q.Encode()

		var paged pagedResponse[Branch]
		if err := c.doGet(ctx, u, &paged, projectKey); err != nil {
			return nil, err
		}
		all = append(all, paged.Values...)
		if paged.IsLastPage {
			break
		}
		start = paged.NextPageStart
	}
	return all, nil
}

// Tag represents a Bitbucket repository tag.
type Tag struct {
	ID              string `json:"id"`
	DisplayID       string `json:"displayId"`
	Type            string `json:"type"`
	LatestCommit    string `json:"latestCommit"`
	LatestChangeset string `json:"latestChangeset"`
	Hash            string `json:"hash"`
}

// Project represents a Bitbucket project.
type Project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// Repository represents a Bitbucket repository.
type Repository struct {
	Slug    string  `json:"slug"`
	Name    string  `json:"name"`
	Project Project `json:"project"`
}

// ListRepositories returns all repositories for the given project,
// following pagination to collect the complete list.
//
// API: GET /rest/api/latest/projects/{projectKey}/repos
func (c *Client) ListRepositories(ctx context.Context, projectKey string) ([]Repository, error) {
	u, err := url.Parse(fmt.Sprintf("%s/rest/api/latest/projects/%s/repos",
		c.baseURL, url.PathEscape(projectKey)))
	if err != nil {
		return nil, fmt.Errorf("bitbucket: building repos URL: %w", err)
	}

	var all []Repository
	start := 0
	for {
		q := u.Query()
		q.Set("start", fmt.Sprintf("%d", start))
		u.RawQuery = q.Encode()

		var paged pagedResponse[Repository]
		if err := c.doGet(ctx, u, &paged, projectKey); err != nil {
			return nil, err
		}
		all = append(all, paged.Values...)
		if paged.IsLastPage {
			break
		}
		start = paged.NextPageStart
	}
	return all, nil
}

// ListTags returns all tags for the given project and repository,
// following pagination to collect the complete list.
//
// API: GET /rest/api/latest/projects/{projectKey}/repos/{repositorySlug}/tags
func (c *Client) ListTags(ctx context.Context, projectKey, repoSlug string) ([]Tag, error) {
	u, err := url.Parse(fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/tags",
		c.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug)))
	if err != nil {
		return nil, fmt.Errorf("bitbucket: building tags URL: %w", err)
	}

	var all []Tag
	start := 0
	for {
		q := u.Query()
		q.Set("start", fmt.Sprintf("%d", start))
		u.RawQuery = q.Encode()

		var paged pagedResponse[Tag]
		if err := c.doGet(ctx, u, &paged, projectKey); err != nil {
			return nil, err
		}
		all = append(all, paged.Values...)
		if paged.IsLastPage {
			break
		}
		start = paged.NextPageStart
	}
	return all, nil
}

// GetDefaultBranch returns the default branch for the given project and repository.
//
// API: GET /rest/api/latest/projects/{projectKey}/repos/{repositorySlug}/branches/default
func (c *Client) GetDefaultBranch(ctx context.Context, projectKey, repoSlug string) (*Branch, error) {
	u, err := url.Parse(fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/branches/default",
		c.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug)))
	if err != nil {
		return nil, fmt.Errorf("bitbucket: building default branch URL: %w", err)
	}

	var branch Branch
	if err := c.doGet(ctx, u, &branch, projectKey); err != nil {
		return nil, err
	}
	return &branch, nil
}

// GetFileContents returns the raw byte content of the file at filePath in the
// given project and repository at the specified revision (branch name, tag, or
// commit ID). Pass an empty revision to use the default branch.
//
// filePath may contain path separators (e.g. "src/main/app.go") and must not
// have a leading slash.
//
// API: GET /rest/api/latest/projects/{projectKey}/repos/{repositorySlug}/raw/{path}?at={revision}
func (c *Client) GetFileContents(ctx context.Context, projectKey, repoSlug, filePath, revision string) ([]byte, error) {
	// filePath may contain '/' separators that must not be percent-encoded,
	// so we construct the URL as a raw string and let url.Parse validate it.
	rawURL := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/raw/%s",
		c.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug),
		strings.TrimPrefix(filePath, "/"))

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: building file URL: %w", err)
	}

	if revision != "" {
		q := u.Query()
		q.Set("at", revision)
		u.RawQuery = q.Encode()
	}

	// The raw endpoint returns plain bytes, not JSON, so we handle it manually.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: creating file request: %w", err)
	}
	c.setAuth(req, projectKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: fetching file contents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.apiError(resp)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: reading file contents: %w", err)
	}
	return data, nil
}

// ListFiles returns all file paths in the given project and repository,
// following pagination to collect the complete list. The list is recursive
// from the root.
//
// API: GET /rest/api/latest/projects/{projectKey}/repos/{repositorySlug}/files
func (c *Client) ListFiles(ctx context.Context, projectKey, repoSlug, at string) ([]string, error) {
	u, err := url.Parse(fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/files",
		c.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug)))
	if err != nil {
		return nil, fmt.Errorf("bitbucket: building files URL: %w", err)
	}

	q := u.Query()
	if at != "" {
		q.Set("at", at)
	}

	var all []string
	start := 0
	for {
		q.Set("start", fmt.Sprintf("%d", start))
		u.RawQuery = q.Encode()

		var paged pagedResponse[string]
		if err := c.doGet(ctx, u, &paged, projectKey); err != nil {
			return nil, err
		}
		all = append(all, paged.Values...)
		if paged.IsLastPage {
			break
		}
		start = paged.NextPageStart
	}
	return all, nil
}

// FileExists checks if a file exists at the given path in the repository.
//
// API: GET /rest/api/latest/projects/{projectKey}/repos/{repositorySlug}/raw/{path}?at={at}
func (c *Client) FileExists(ctx context.Context, projectKey, repoSlug, filePath, at string) (bool, error) {
	// filePath may contain '/' separators that must not be percent-encoded,
	// so we construct the URL as a raw string and let url.Parse validate it.
	rawURL := fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/raw/%s",
		c.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug),
		strings.TrimPrefix(filePath, "/"))

	u, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("bitbucket: building file URL: %w", err)
	}

	if at != "" {
		q := u.Query()
		q.Set("at", at)
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	if err != nil {
		return false, fmt.Errorf("bitbucket: creating file head request: %w", err)
	}
	c.setAuth(req, projectKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("bitbucket: executing file head request: %w", err)
	}
	defer resp.Body.Close()

	// Fully consume the response body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, c.apiError(resp)
}

func (c *Client) setAuth(req *http.Request, projectKey string) {
	if token, ok := c.opts.PerProjectTokens[strings.ToLower(projectKey)]; ok {
		req.SetBasicAuth(HTTPAccessTokenUser, token)
		return
	}
	if c.opts.Username != "" {
		req.SetBasicAuth(c.opts.Username, c.opts.Password)
	}
}

// APIError is returned when the Bitbucket API responds with a non-2xx status.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("bitbucket API error %d: %s", e.StatusCode, e.Message)
}

// errorResponse matches the JSON error body returned by the Bitbucket API.
type errorResponse struct {
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var errResp errorResponse
	if json.Unmarshal(body, &errResp) == nil && len(errResp.Errors) > 0 {
		return &APIError{StatusCode: resp.StatusCode, Message: errResp.Errors[0].Message}
	}
	return &APIError{StatusCode: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
}
