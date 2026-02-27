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

// Client is a client for the Bitbucket Data Center REST API.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewClient creates a new Bitbucket Data Center client targeting baseURL
// (e.g. "https://bitbucket.example.com"). Pass empty strings for username
// and password to make unauthenticated requests.
func NewClient(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
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

// GetCommits returns the first page of commits for the given project and
// repository. Use GetCommitsOptions to filter by ref or set a page size.
//
// API: GET /rest/api/latest/projects/{projectKey}/repos/{repositorySlug}/commits
func (c *Client) GetCommits(ctx context.Context, projectKey, repoSlug string, opts GetCommitsOptions) ([]Commit, error) {
	u, err := url.Parse(fmt.Sprintf("%s/rest/api/latest/projects/%s/repos/%s/commits",
		c.baseURL, url.PathEscape(projectKey), url.PathEscape(repoSlug)))
	if err != nil {
		return nil, fmt.Errorf("bitbucket: building commits URL: %w", err)
	}

	q := u.Query()
	if opts.Until != "" {
		q.Set("until", opts.Until)
	}
	if opts.Since != "" {
		q.Set("since", opts.Since)
	}
	if opts.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: creating commits request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: fetching commits: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, c.apiError(resp)
	}

	var paged pagedResponse[Commit]
	if err := json.NewDecoder(resp.Body).Decode(&paged); err != nil {
		return nil, fmt.Errorf("bitbucket: decoding commits response: %w", err)
	}
	return paged.Values, nil
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: creating file request: %w", err)
	}
	c.setAuth(req)

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

func (c *Client) setAuth(req *http.Request) {
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
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
