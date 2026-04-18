package jfrog

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client interface {
	ListDockerTags(ctx context.Context, repository, image string) ([]string, error)
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
}

type jfrogClient struct {
	Config Config
}

type TagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
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

// FetchTags returns the list of tags for the given image in repository.
func (c *jfrogClient) ListDockerTags(ctx context.Context, repository, image string) ([]string, error) {
	// Use short timeout for fetching tags, this should be quick, else we've probably got network issues.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
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
