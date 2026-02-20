package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/plugins/maven"
	"github.com/dnswlt/swcat/internal/plugins/sbom"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

type jfrogAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// If set, the plugin will attempt to get user/pass from MavenSettingsPath
	// (or the default ~/.m2/settings) for the specified server.
	MavenServerID     string `yaml:"mavenServerID"`
	MavenSettingsPath string `yaml:"mavenSettingsPath"`
}

type jfrogXrayPluginSpec struct {
	JFrogURL          string `yaml:"jfrogURL"`
	DefaultRepository string `yaml:"defaultRepository"`
	// Annotation in which to find the Docker image name
	ImageAnnotation string `yaml:"imageAnnotation"`
	// Annotation in which to find the Artifactory repository name
	RepositoryAnnotation string               `yaml:"repositoryAnnotation"`
	Auth                 jfrogAuth            `yaml:"auth"`
	ComponentFilter      sbom.ComponentFilter `yaml:"componentFilter"`
	TargetAnnotation     string               `yaml:"targetAnnotation"`
}

type JFrogXrayPlugin struct {
	name string
	spec *jfrogXrayPluginSpec
}

func readAuthFromMavenSettings(path string, serverID string) (jfrogAuth, error) {
	settings, err := maven.ReadSettings(path)
	if err != nil {
		return jfrogAuth{}, err
	}
	server, err := settings.ServerByID(serverID)
	if err != nil {
		return jfrogAuth{}, err
	}
	return jfrogAuth{
		Username:      server.Username,
		Password:      server.Password,
		MavenServerID: server.ID,
	}, nil
}

func NewJFrogXrayBOMPlugin(name string, specYaml *yaml.Node) (*JFrogXrayPlugin, error) {
	var spec jfrogXrayPluginSpec
	if err := specYaml.Decode(&spec); err != nil {
		return nil, fmt.Errorf("failed to decode JFrogXrayPlugin spec for %s: %v", name, err)
	}

	if spec.JFrogURL == "" {
		return nil, fmt.Errorf("field 'jfrogURL' not specified for plugin %s", name)
	}
	if !catalog.IsValidAnnotation(spec.TargetAnnotation, "true") {
		return nil, fmt.Errorf("invalid targetAnnotation %q for plugin %s", spec.TargetAnnotation, name)
	}

	if spec.Auth.MavenServerID != "" {
		auth, err := readAuthFromMavenSettings(spec.Auth.MavenSettingsPath, spec.Auth.MavenServerID)
		if err != nil {
			log.Printf("Failed to use maven settings for jFrog auth: %v", err)
		} else {
			spec.Auth = auth
		}
	}

	return &JFrogXrayPlugin{
		name: name,
		spec: &spec,
	}, nil
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

func (p *JFrogXrayPlugin) fetchSBOM(ctx context.Context, repository, image string) (string, error) {

	// Get versions list
	tagsURL := fmt.Sprintf("%s/artifactory/api/docker/%s/v2/%s/tags/list", p.spec.JFrogURL, repository, image)
	req, err := http.NewRequestWithContext(ctx, "GET", tagsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	if p.spec.Auth.Username != "" {
		req.SetBasicAuth(p.spec.Auth.Username, p.spec.Auth.Password)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get tags, status code: %d", resp.StatusCode)
	}

	var tagsResp TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return "", fmt.Errorf("failed to decode tags response: %w", err)
	}

	// Extract latest version using semver.Compare
	latestVersion := ""
	for _, tag := range tagsResp.Tags {
		// Ensure tag has 'v' prefix for semver comparison
		tagWithV := tag
		if !strings.HasPrefix(tag, "v") {
			tagWithV = "v" + tag
		}

		if !semver.IsValid(tagWithV) {
			continue
		}

		if latestVersion == "" || semver.Compare(tagWithV, latestVersion) > 0 {
			latestVersion = tagWithV
		}
	}

	if latestVersion == "" {
		return "", fmt.Errorf("no valid semver tags found")
	}

	// Remove 'v' prefix for component name
	version := strings.TrimPrefix(latestVersion, "v")

	// Download the SBOM for the latest version
	sbomReq := SBOMRequest{
		PackageType:     "docker",
		ComponentName:   fmt.Sprintf("%s:%s", image, version),
		Path:            fmt.Sprintf("%s/%s/%s/manifest.json", repository, image, version),
		CycloneDX:       true,
		CycloneDXFormat: "json",
		Vex:             false,
	}

	reqBody, err := json.Marshal(sbomReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal SBOM request: %w", err)
	}

	sbomURL := fmt.Sprintf("%s/xray/api/v2/component/exportDetails", p.spec.JFrogURL)
	req, err = http.NewRequestWithContext(ctx, "POST", sbomURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create SBOM request: %w", err)
	}
	if p.spec.Auth.Username != "" {
		req.SetBasicAuth(p.spec.Auth.Username, p.spec.Auth.Password)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get SBOM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to get SBOM, status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Read the zip file into memory
	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read SBOM response: %w", err)
	}

	// Extract the SBOM from the zip file
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", fmt.Errorf("failed to open zip file: %w", err)
	}

	// Find and read the SBOM JSON file
	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, ".json") {
			rc, err := file.Open()
			if err != nil {
				return "", fmt.Errorf("failed to open file in zip: %w", err)
			}
			defer rc.Close()

			sbomData, err := io.ReadAll(rc)
			if err != nil {
				return "", fmt.Errorf("failed to read SBOM from zip: %w", err)
			}

			return string(sbomData), nil
		}
	}

	return "", fmt.Errorf("no JSON file found in SBOM zip")
}

func (p *JFrogXrayPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	repository := p.spec.DefaultRepository
	if ra := p.spec.RepositoryAnnotation; ra != "" {
		// Get the repository from annotations
		if r, ok := entity.GetMetadata().Annotations[ra]; ok {
			repository = r
		}
	}
	if repository == "" {
		return nil, fmt.Errorf("No repository specified for %v", entity.GetQName())
	}

	image := entity.GetMetadata().Name
	if ia := p.spec.ImageAnnotation; ia != "" {
		// Get the image from annotations
		if img, ok := entity.GetMetadata().Annotations[ia]; ok {
			image = img
		}
	}

	sbomStr, err := p.fetchSBOM(ctx, repository, image)
	if err != nil {
		return nil, err
	}
	sbomObj, err := sbom.Parse(sbomStr)
	if err != nil {
		return nil, fmt.Errorf("cannot parse SBOM: %w", err)
	}
	components, err := sbom.FilterComponents(sbomObj, p.spec.ComponentFilter)
	if err != nil {
		return nil, fmt.Errorf("filtering components: %w", err)
	}
	return &PluginResult{
		Annotations: map[string]any{
			p.spec.TargetAnnotation: components,
		},
	}, nil
}
