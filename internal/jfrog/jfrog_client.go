package jfrog

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client interface {
	// https://{jfrog_url}/artifactory/{repoKey}/{filePath}
	RetrieveArtifact(ctx context.Context, repository string, coords MavenCoordinates, out io.Writer) error
	// https://{jfrog_url}/artifactory/api/search/versions
	SearchVersions(ctx context.Context, repository, groupId, artifactId string, releaseOnly bool) ([]string, error)
	// https://{jfrog_url}/artifactory/api/docker/{repo-key}/v2/{imageName}/tags/list
	ListDockerTags(ctx context.Context, repository, image string) ([]string, error)
	// https://{jfrog_url}/xray/api/v2/component/exportDetails
	XrayExportDetails(ctx context.Context, repository, image, version string) ([]byte, error)
}

// Interface assertion: jfrogClient should implement Client.
var _ Client = (*jfrogClient)(nil)

type Auth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Config struct {
	// URL under which to find JFrog.
	JFrogURL string `yaml:"jfrogUrl"`
	// Authentication config
	Auth Auth `yaml:"auth"`
	// Timeout to use for requests to the JFrog API (except downloads).
	Timeout time.Duration
}

type jfrogClient struct {
	Config Config
}

type MavenCoordinates struct {
	GroupID    string
	ArtifactID string
	Version    string
	Classifier string
	Extension  string
}

type TagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

type VersionsResponse struct {
	Results []struct {
		Version     string `json:"version"`
		Integration bool   `json:"integration"`
	} `json:"results"`
}

type SBOMRequest struct {
	PackageType     string `json:"package_type"`
	ComponentName   string `json:"component_name"`
	Path            string `json:"path"`
	CycloneDX       bool   `json:"cyclonedx"`
	CycloneDXFormat string `json:"cyclonedx_format"`
	Vex             bool   `json:"vex"`
}

func NewClient(cfg Config) Client {
	return &jfrogClient{
		Config: cfg,
	}
}

func (c *jfrogClient) setBasicAuth(req *http.Request) {
	if c.Config.Auth.Username != "" {
		req.SetBasicAuth(c.Config.Auth.Username, c.Config.Auth.Password)
	}
}

// RetrieveArtifact downloads the Maven artifact at the given coordinates from repository and writes it to out.
// No timeout is applied; downloads may take arbitrarily long.
func (c *jfrogClient) RetrieveArtifact(ctx context.Context, repository string, coords MavenCoordinates, out io.Writer) error {
	groupPath := strings.ReplaceAll(coords.GroupID, ".", "/")
	filename := fmt.Sprintf("%s-%s", coords.ArtifactID, coords.Version)
	if coords.Classifier != "" {
		filename += "-" + coords.Classifier
	}
	filename += "." + coords.Extension
	u := fmt.Sprintf("%s/artifactory/%s/%s/%s/%s/%s",
		c.Config.JFrogURL, repository, groupPath, coords.ArtifactID, coords.Version, filename)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return fmt.Errorf("failed to create artifact request: %w", err)
	}
	c.setBasicAuth(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("artifact request for %s returned status %d", u, resp.StatusCode)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write artifact: %w", err)
	}
	return nil
}

// SearchVersions uses the Artifact Version Search API to list the available versions of the given artifact.
func (c *jfrogClient) SearchVersions(ctx context.Context, repository, groupId, artifactId string, releaseOnly bool) ([]string, error) {
	if c.Config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Config.Timeout)
		defer cancel()
	}
	params := url.Values{}
	params.Set("g", groupId)
	params.Set("a", artifactId)
	params.Set("repos", repository)
	u := fmt.Sprintf("%s/artifactory/api/search/versions?%s", c.Config.JFrogURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create versions request: %w", err)
	}
	c.setBasicAuth(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("versions request returned status %d", resp.StatusCode)
	}
	var versionsResp VersionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&versionsResp); err != nil {
		return nil, fmt.Errorf("failed to decode versions response: %w", err)
	}
	versions := make([]string, 0, len(versionsResp.Results))
	for _, r := range versionsResp.Results {
		if !releaseOnly || !r.Integration {
			versions = append(versions, r.Version)
		}
	}
	return versions, nil
}

// ListDockerTags returns the list of tags for the given image in repository.
func (c *jfrogClient) ListDockerTags(ctx context.Context, repository, image string) ([]string, error) {
	if c.Config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Config.Timeout)
		defer cancel()
	}
	url := fmt.Sprintf("%s/artifactory/api/docker/%s/v2/%s/tags/list", c.Config.JFrogURL, repository, image)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create tags request: %w", err)
	}
	c.setBasicAuth(req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tags request returned status %d", resp.StatusCode)
	}
	var tagsResp TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("failed to decode tags response: %w", err)
	}
	return tagsResp.Tags, nil
}

// XrayExportDetails fetches the CycloneDX SBOM zip for image:version from JFrog
// Xray and returns the JSON content of the first .json file in the archive.
func (c *jfrogClient) XrayExportDetails(ctx context.Context, repository, image, version string) ([]byte, error) {
	sbomReq := SBOMRequest{
		PackageType:     "docker",
		ComponentName:   fmt.Sprintf("%s:%s", image, version),
		Path:            fmt.Sprintf("%s/%s/%s/manifest.json", repository, image, version),
		CycloneDX:       true,
		CycloneDXFormat: "json",
	}
	body, err := json.Marshal(sbomReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal SBOM request: %w", err)
	}

	url := fmt.Sprintf("%s/xray/api/v2/component/exportDetails", c.Config.JFrogURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create SBOM request: %w", err)
	}
	c.setBasicAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch SBOM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SBOM request for %s returned status %d: %s", sbomReq.Path, resp.StatusCode, errBody)
	}

	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read SBOM response body: %w", err)
	}
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("failed to open SBOM zip: %w", err)
	}
	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, ".json") {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open %s in SBOM zip: %w", f.Name, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("failed to read %s in SBOM zip: %w", f.Name, err)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("no JSON file found in SBOM zip for %s:%s", image, version)
}
