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
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/plugins/maven"
	"github.com/dnswlt/swcat/internal/plugins/sbom"
	"github.com/dnswlt/swcat/internal/repo"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

type jfrogAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// If set, the plugin will attempt to get user/pass from MavenSettingsPath
	// (or the default ~/.m2/settings) for the specified server.
	MavenServerID     string `yaml:"mavenServerId"`
	MavenSettingsPath string `yaml:"mavenSettingsPath"`
}

type jfrogXrayPluginSpec struct {
	JFrogURL          string `yaml:"jfrogUrl"`
	DefaultRepository string `yaml:"defaultRepository"`
	// Annotation in which to find the Docker image name
	ImageAnnotation string `yaml:"imageAnnotation"`
	// Annotation in which to find the Artifactory repository name
	RepositoryAnnotation  string                `yaml:"repositoryAnnotation"`
	Auth                  jfrogAuth             `yaml:"auth"`
	ComponentsFilter      sbom.ComponentsFilter `yaml:"componentsFilter"`
	CoordsAnnotation      string                `yaml:"coordsAnnotation"`
	TargetAnnotation      string                `yaml:"targetAnnotation"`
	LintFindingAnnotation string                `yaml:"lintFindingAnnotation"`
	// Annotation in which to find a JSON list of "[groupId:]artifactId"
	// strings of dependencies that should never be declared as missing during linting.
	// This is the usual "lint:ignore" hook that's always needed for weird edge
	// cases, to avoid flooding the system with lint warnings that no none looks at.
	LintIgnoreAnnotation string `yaml:"lintIgnoreAnnotation"`
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

	if spec.Auth.MavenServerID != "" && spec.Auth.Username == "" {
		auth, err := readAuthFromMavenSettings(spec.Auth.MavenSettingsPath, spec.Auth.MavenServerID)
		if err != nil {
			log.Printf("Failed to use maven settings for jFrog auth: %v", err)
		} else {
			log.Printf("Successfully read maven settings for server ID %s from %s", spec.Auth.MavenServerID, spec.Auth.MavenSettingsPath)
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

func (p *JFrogXrayPlugin) setBasicAuth(req *http.Request) {
	if p.spec.Auth.Username != "" {
		req.SetBasicAuth(p.spec.Auth.Username, p.spec.Auth.Password)
	}
}

// fetchTags returns the list of tags for the given image in repository.
func (p *JFrogXrayPlugin) fetchTags(ctx context.Context, repository, image string) ([]string, error) {
	// Use short timeout for fetching tags, this should be quick, else we've probably got network issues.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	url := fmt.Sprintf("%s/artifactory/api/docker/%s/v2/%s/tags/list", p.spec.JFrogURL, repository, image)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create tags request: %w", err)
	}
	p.setBasicAuth(req)

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

// semverNormalize returns tag with a "v" prefix for semver comparison,
// leaving tags that already have one unchanged.
func semverNormalize(tag string) string {
	if strings.HasPrefix(tag, "v") {
		return tag
	}
	return "v" + tag
}

// latestSemverVersions filters tags to those with valid semver, sorts them in
// descending order, and returns the top n original tags (preserving their
// original "v"-prefix or lack thereof).
func latestSemverVersions(tags []string, n int) []string {
	var valid []string
	for _, tag := range tags {
		if semver.IsValid(semverNormalize(tag)) {
			valid = append(valid, tag)
		}
	}
	slices.SortFunc(valid, func(v1, v2 string) int {
		return semver.Compare(semverNormalize(v2), semverNormalize(v1)) // descending
	})
	if len(valid) > n {
		valid = valid[:n]
	}
	return valid
}

// downloadSBOM fetches the CycloneDX SBOM zip for image:version from JFrog
// Xray and returns the JSON content of the first .json file in the archive.
func (p *JFrogXrayPlugin) downloadSBOM(ctx context.Context, repository, image, version string) (string, error) {
	sbomReq := SBOMRequest{
		PackageType:     "docker",
		ComponentName:   fmt.Sprintf("%s:%s", image, version),
		Path:            fmt.Sprintf("%s/%s/%s/manifest.json", repository, image, version),
		CycloneDX:       true,
		CycloneDXFormat: "json",
	}
	body, err := json.Marshal(sbomReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal SBOM request: %w", err)
	}

	url := fmt.Sprintf("%s/xray/api/v2/component/exportDetails", p.spec.JFrogURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create SBOM request: %w", err)
	}
	p.setBasicAuth(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch SBOM: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("SBOM request for %s returned status %d: %s", sbomReq.Path, resp.StatusCode, errBody)
	}

	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read SBOM response body: %w", err)
	}
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return "", fmt.Errorf("failed to open SBOM zip: %w", err)
	}
	for _, f := range zipReader.File {
		if strings.HasSuffix(f.Name, ".json") {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("failed to open %s in SBOM zip: %w", f.Name, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				return "", fmt.Errorf("failed to read %s in SBOM zip: %w", f.Name, err)
			}
			return string(data), nil
		}
	}
	return "", fmt.Errorf("no JSON file found in SBOM zip for %s:%s", image, version)
}

// fetchSBOM retrieves the SBOM for the latest available semver version of
// image in repository, trying up to the three most recent versions.
func (p *JFrogXrayPlugin) fetchSBOM(ctx context.Context, repository, image string) (string, error) {
	tags, err := p.fetchTags(ctx, repository, image)
	if err != nil {
		return "", err
	}
	versions := latestSemverVersions(tags, 3)
	if len(versions) == 0 {
		return "", fmt.Errorf("no valid semver tags found for %s/%s", repository, image)
	}
	var lastErr error
	for _, version := range versions {
		data, err := p.downloadSBOM(ctx, repository, image, version)
		if err != nil {
			log.Printf("fetchSBOM: skipping %s:%s: %v", image, version, err)
			lastErr = err
			continue
		}
		return data, nil
	}
	return "", fmt.Errorf("could not download SBOM for %s/%s (tried %v): %w", repository, image, versions, lastErr)
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

	// Fetch CycloneDX SBOM from jFrog XRay
	sbomStr, err := p.fetchSBOM(ctx, repository, image)
	if err != nil {
		return nil, err
	}

	// Process SBOM and extract flat string list of component coordinates
	// (group:artifact).
	sbomObj, err := sbom.Parse(sbomStr)
	if err != nil {
		return nil, fmt.Errorf("cannot parse SBOM: %w", err)
	}
	bom, err := sbom.FilterComponents(sbomObj, p.spec.ComponentsFilter)
	if err != nil {
		return nil, fmt.Errorf("filtering components: %w", err)
	}
	log.Printf("Processed SBOM %s for entity %s: %d components", bom.Name, entity.GetQName(), len(bom.Components))

	if args.Repository == nil {
		return nil, fmt.Errorf("repository not set in plugin args")
	}
	idx := p.newCatalogIndexFromEntities(args.Repository.AllEntities())

	annotations := map[string]any{
		p.spec.TargetAnnotation: bom,
	}

	if p.spec.LintFindingAnnotation != "" {
		missing, _ := p.detectDependencyMismatches(bom, entity, idx, args.Repository)
		if len(missing) > 0 {
			annotations[p.spec.LintFindingAnnotation] = api.LintFinding{
				CreateTime: time.Now(),
				Message:    fmt.Sprintf("Dependencies found in BOM, but missing in entity: %s", strings.Join(missing, ",")),
			}
		}
	}

	return &PluginResult{
		Annotations: annotations,
	}, nil
}

// detectDependencyMismatches compares bom.Components and all dependencies (including API usage)
// of the given entity.
// It returns mismatches as two lists: missing for components present in bom but missing
// in entity's deps; extra for deps present in entity deps, but missing in bom.
// It only works for Component entities.
func (p *JFrogXrayPlugin) detectDependencyMismatches(bom *sbom.MiniBOM, entity catalog.Entity, fullIdx *catalogIndex, repository *repo.Repository) (missing []string, extra []string) {
	comp, ok := entity.(*catalog.Component)
	if !ok {
		return nil, nil
	}

	ignored := make(map[string]bool)
	if p.spec.LintIgnoreAnnotation != "" {
		if val, ok := entity.GetMetadata().Annotations[p.spec.LintIgnoreAnnotation]; ok {
			var list []string
			if err := json.Unmarshal([]byte(val), &list); err == nil {
				for _, s := range list {
					ignored[s] = true
				}
			}
		}
	}

	isIgnored := func(bc gav) bool {
		if len(ignored) == 0 {
			return false
		}
		if ignored[bc.a] { // artifactId
			return true
		}
		if bc.g != "" && ignored[bc.g+":"+bc.a] { // groupId:artifactId
			return true
		}
		return false
	}

	// 1. Resolve all declared dependencies of this component.
	declared := make(map[string]catalog.Entity)
	add := func(refs []*catalog.LabelRef) {
		for _, r := range refs {
			if e := repository.Entity(r.Ref); e != nil {
				declared[e.GetRef().String()] = e
			}
		}
	}
	add(comp.Spec.DependsOn)
	add(comp.Spec.ConsumesAPIs)
	add(comp.Spec.ProvidesAPIs)

	// 2. Create index for declared dependencies for fast matching.
	declaredEntities := make([]catalog.Entity, 0, len(declared))
	for _, e := range declared {
		declaredEntities = append(declaredEntities, e)
	}
	declaredIdx := p.newCatalogIndexFromEntities(declaredEntities)

	// 3. Compare SBOM components with declared dependencies.
	matchedRefs := make(map[string]bool)

	for _, raw := range bom.Components {
		bc := parseGAV(raw)
		if e := declaredIdx.matchEntity(bc); e != nil {
			matchedRefs[e.GetRef().String()] = true
		} else {
			// Not in declared deps. Is it in the catalog at all?
			if e := fullIdx.matchEntity(bc); e != nil {
				// Yes, it's a catalog entity.
				// Add to mising if it's different from 'entity' itself and not explicitly ignored.
				if !isIgnored(bc) && !e.GetRef().Equal(entity.GetRef()) {
					missing = append(missing, raw)
				}
			}
		}
	}

	// 4. Any declared dependency missing from the SBOM?
	for ref, e := range declared {
		if !matchedRefs[ref] {
			extra = append(extra, e.GetQName())
		}
	}

	slices.Sort(missing)
	slices.Sort(extra)
	return
}

type gav struct {
	g, a, v string
}

func parseGAV(s string) gav {
	parts := strings.Split(s, ":")
	if len(parts) == 1 {
		return gav{a: parts[0]}
	}
	if len(parts) == 2 {
		return gav{g: parts[0], a: parts[1]}
	}
	return gav{g: parts[0], a: parts[1], v: parts[2]}
}

type indexedEntity struct {
	entity catalog.Entity
	coords gav
}

type catalogIndex struct {
	entities []indexedEntity
}

func (p *JFrogXrayPlugin) newCatalogIndexFromEntities(allEntities []catalog.Entity) *catalogIndex {
	entities := make([]indexedEntity, 0, len(allEntities))
	for _, e := range allEntities {
		info := indexedEntity{entity: e, coords: gav{a: e.GetMetadata().Name}}
		if p.spec.CoordsAnnotation != "" {
			if coords, ok := e.GetMetadata().Annotations[p.spec.CoordsAnnotation]; ok {
				info.coords = parseGAV(coords)
			}
		}
		entities = append(entities, info)
	}

	slices.SortFunc(entities, func(i, j indexedEntity) int {
		if cmp := strings.Compare(i.coords.a, j.coords.a); cmp != 0 {
			return cmp
		}
		return strings.Compare(i.coords.g, j.coords.g)
	})

	return &catalogIndex{
		entities: entities,
	}
}

func (idx *catalogIndex) matchEntity(bc gav) catalog.Entity {
	if bc.a == "" {
		return nil
	}

	// sort.Search returns the smallest index i such that f(i) is true.
	start := sort.Search(len(idx.entities), func(i int) bool {
		return idx.entities[i].coords.a >= bc.a
	})

	// Scan only the block with matching artifactId.
	for j := start; j < len(idx.entities) && idx.entities[j].coords.a == bc.a; j++ {
		info := idx.entities[j]
		if info.coords.g == "" || bc.g == "" || info.coords.g == bc.g {
			return info.entity
		}
	}
	return nil
}
