package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/jfrog"
	"github.com/dnswlt/swcat/internal/plugins/asyncapi"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

const (
	// The status field that the simplified representation of AsyncAPI channels gets written to.
	AsyncAPIPluginTarget = "swcat-plugins/asyncapi-channels"
	// The status field that lint findings are stored in.
	AsyncAPIPluginLintTarget = "swcat-lint/finding-newer-version"
)

type VersionedChannels struct {
	Version  string                    `json:"version"`
	Channels []*asyncapi.SimpleChannel `json:"channels"`
}

type asyncAPIFetcherConfig struct {
	// The packaging (e.g., "zip", "jar")
	Packaging string `yaml:"packaging"`
	// An optional artifact classifier (gets dash-appended to the file name).
	Classifier string `yaml:"classifier"`
	// If set, reads all .properties files found in the fetched artifact
	// and replaces them in the File before parsing the YAML.
	// If this seems weird, that's because it is. But we've seen cases where
	// the AsyncAPI spec is not stored as a proper YAML file, but has @@placeholders@@
	// in it that need to be replaced first. (Don't ask...)
	ReplaceProperties bool `yaml:"replaceProperties"`
}

type asyncAPIImporterPluginSpec struct {
	// The built-in JFrog client used to fetch the AsyncAPI spec.
	Fetcher *asyncAPIFetcherConfig `yaml:"fetcher"`
	// The file name of the AsyncAPI spec.
	File string `yaml:"file"`
}

type AsyncAPIImporterPlugin struct {
	name    string
	spec    *asyncAPIImporterPluginSpec
	fetcher JFrogArtifactFetcher
}

type JFrogArtifactFetcher interface {
	// https://{jfrog_url}/artifactory/{repoKey}/{filePath}
	RetrieveArtifact(ctx context.Context, repository string, coords jfrog.MavenCoordinates, out io.Writer) error
	// https://{jfrog_url}/artifactory/api/search/versions
	SearchVersions(ctx context.Context, repository, groupId, artifactId string, releaseOnly bool) ([]string, error)
}

// newAsyncAPIImporterPlugin creates a new instance of the AsyncAPI importer plugin.
func newAsyncAPIImporterPlugin(name string, specYaml *yaml.Node, fetcher JFrogArtifactFetcher) (*AsyncAPIImporterPlugin, error) {
	var spec asyncAPIImporterPluginSpec
	if err := specYaml.Decode(&spec); err != nil {
		return nil, fmt.Errorf("failed to decode AsyncAPIImporterPlugin spec for %s: %v", name, err)
	}

	if spec.File == "" {
		return nil, fmt.Errorf("field 'file' not specified for plugin %s", name)
	}
	if spec.Fetcher == nil {
		return nil, fmt.Errorf("'fetcher' config not specified for plugin %s", name)
	}
	if spec.Fetcher.Packaging == "" {
		return nil, fmt.Errorf("field 'fetcher.packaging' not specified for plugin %s", name)
	}

	if fetcher == nil {
		return nil, fmt.Errorf("no JFrogArtifactFetcher provided for plugin %s: %w", name, ErrPreconditionFailed)
	}

	return &AsyncAPIImporterPlugin{
		name:    name,
		spec:    &spec,
		fetcher: fetcher,
	}, nil
}

// Execute runs the AsyncAPI importer plugin, resolving artifacts, fetching versions, and returning the updated status observations.
func (m *AsyncAPIImporterPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	now := time.Now()

	groupId, artifactId, explicitVersion, repository, err := m.resolveArtifactContext(entity, args)
	if err != nil {
		return nil, err
	}

	storedResults := m.getStoredObservation(entity)

	available, err := m.fetcher.SearchVersions(ctx, repository, groupId, artifactId, true)
	if err != nil {
		return nil, fmt.Errorf("failed to search versions for %s:%s: %w", groupId, artifactId, err)
	}
	if len(available) == 0 {
		// The artifact is not / no longer stored in JFrog.
		return nil, fmt.Errorf("no versions available for %s:%s", groupId, artifactId)
	}

	targetVersions, targetMeta, err := m.identifyTargetVersions(entity, groupId, artifactId, explicitVersion, available)
	if err != nil {
		return nil, err
	}

	observations := make(map[string]catalog.Observation)
	var removedObservations []string

	if finding := checkForNewerMajorVersion(entity, available, now); finding != nil {
		observations[AsyncAPIPluginLintTarget] = catalog.Observation{
			Producer:  m.name,
			Value:     api.MustMarshalJSON(finding),
			UpdatedAt: now,
		}
	} else {
		// Clear any previously-emitted finding so it doesn't linger after the
		// entity catches up with the available versions.
		removedObservations = append(removedObservations, AsyncAPIPluginLintTarget)
	}

	results, err := m.fetchResults(ctx, repository, groupId, artifactId, targetVersions, storedResults)
	if err != nil {
		return nil, err
	}

	// For non-API entities (or APIs with no declared versions), targetMeta is
	// nil and we record the single resolved version in Observation.Version.
	var obsVersion string
	if targetMeta == nil && len(targetVersions) > 0 {
		obsVersion = targetVersions[0]
	}

	observations[AsyncAPIPluginTarget] = catalog.Observation{
		Producer:  m.name,
		Value:     api.MustMarshalJSON(results),
		UpdatedAt: now,
		Version:   obsVersion,
		Meta:      targetMeta,
	}

	return &PluginResult{
		Observations:        observations,
		RemovedObservations: removedObservations,
	}, nil
}

// resolveArtifactContext determines the Maven coordinates and repository for the entity.
func (m *AsyncAPIImporterPlugin) resolveArtifactContext(entity catalog.Entity, args *PluginArgs) (g, a, v, repo string, err error) {
	a = entity.GetMetadata().Name
	g, _ = args.Repository.IAnnotation(entity, catalog.AnnotMavenGroupID)
	repo, _ = args.Repository.IAnnotation(entity, JFrogRepositoryAnnotation)

	if mc, ok := entity.GetMetadata().Annotations[catalog.AnnotMavenCoords]; ok {
		coords := parseGAV(mc)
		// Partial GAV annotations (e.g. just an artifactId) must not erase
		// fields that were already resolved from other annotations.
		if coords.g != "" {
			g = coords.g
		}
		if coords.a != "" {
			a = coords.a
		}
		if coords.v != "" {
			v = coords.v
		}
	}

	if g == "" || repo == "" {
		return "", "", "", "", fmt.Errorf("missing Maven groupId or JFrog repository for %v", entity.GetQName())
	}
	return g, a, v, repo, nil
}

// getStoredObservation extracts previously fetched results from the entity's status.
// If the JSON is incompatible (e.g. from an older version of the plugin),
// it safely discards the data by returning nil, triggering a fresh fetch.
func (m *AsyncAPIImporterPlugin) getStoredObservation(entity catalog.Entity) map[string]*VersionedChannels {
	status := entity.GetStatus()
	if status == nil {
		return nil
	}
	obs, ok := status.Observations[AsyncAPIPluginTarget]
	if !ok {
		return nil
	}
	var stored []*VersionedChannels
	dec := json.NewDecoder(bytes.NewReader(obs.Value))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&stored); err != nil {
		return nil
	}
	results := make(map[string]*VersionedChannels, len(stored))
	for _, vch := range stored {
		results[vch.Version] = vch
	}
	return results
}

// identifyTargetVersions determines which versions of the AsyncAPI spec should be present.
// For API entities with declared versions, also returns a meta map keyed by
// the entity's RawVersion → resolved repository version. The meta map is nil
// for the non-API path (where the single resolved version is reported via
// Observation.Version instead).
func (m *AsyncAPIImporterPlugin) identifyTargetVersions(entity catalog.Entity, g, a, explicitVersion string, available []string) ([]string, map[string]string, error) {
	if apiEntity, isAPI := entity.(*catalog.API); isAPI && len(apiEntity.Spec.Versions) > 0 {
		versions, meta := identifyAPIVersions(apiEntity, available)
		return versions, meta, nil
	}

	if explicitVersion != "" {
		return []string{explicitVersion}, nil, nil
	}

	v, err := identifyLatestVersion(g, a, available)
	if err != nil {
		return nil, nil, err
	}
	return []string{v}, nil, nil
}

// identifyAPIVersions identifies the target versions for an API entity from the available versions in the repository,
// along with a meta map keyed by the entity's RawVersion → resolved repository version.
func identifyAPIVersions(apiEntity *catalog.API, available []string) ([]string, map[string]string) {
	var versions []string
	seen := make(map[string]bool)
	meta := make(map[string]string)
	for _, v := range apiEntity.Spec.Versions {
		match := findLatestMatch(v.Version, available)
		if match == "" {
			continue
		}
		meta["version-"+v.Version.RawVersion] = match
		if !seen[match] {
			versions = append(versions, match)
			seen[match] = true
		}
	}
	return versions, meta
}

// identifyLatestVersion identifies the latest single target version.
func identifyLatestVersion(g, a string, available []string) (string, error) {
	latest := latestSemverVersions(available, 1)
	if len(latest) == 0 {
		return "", fmt.Errorf("no valid semver versions found for %s:%s", g, a)
	}
	return latest[0], nil
}

// checkForNewerMajorVersion checks if there is a newer major version available in the repository that is not listed in the entity's versions.
func checkForNewerMajorVersion(entity catalog.Entity, available []string, now time.Time) *api.LintFinding {
	apiEntity, isAPI := entity.(*catalog.API)
	if !isAPI || len(apiEntity.Spec.Versions) == 0 || len(available) == 0 {
		return nil
	}

	maxEntityMajor := 0
	for _, v := range apiEntity.Spec.Versions {
		if v.Version.Major > maxEntityMajor {
			maxEntityMajor = v.Version.Major
		}
	}
	maxEntityMajorPrefix := fmt.Sprintf("v%d", maxEntityMajor)

	var newerMajorFound string
	for _, a := range available {
		norm := semverNormalize(a)
		if !semver.IsValid(norm) {
			continue
		}
		major := semver.Major(norm)
		if semver.Compare(major, maxEntityMajorPrefix) <= 0 {
			continue
		}
		// Track the *highest* newer major, not whichever happens to come last.
		if newerMajorFound == "" || semver.Compare(major, newerMajorFound) > 0 {
			newerMajorFound = major
		}
	}

	if newerMajorFound != "" {
		return &api.LintFinding{
			CreateTime: now,
			Message:    fmt.Sprintf("A newer major version (%s) is available in the repository but not listed in the entity's versions.", newerMajorFound),
		}
	}

	return nil
}

// fetchResults populates results, downloading missing versions while reusing stored ones.
func (m *AsyncAPIImporterPlugin) fetchResults(ctx context.Context, repo, g, a string, versions []string, stored map[string]*VersionedChannels) ([]*VersionedChannels, error) {
	var results []*VersionedChannels
	for _, v := range versions {
		if vch, ok := stored[v]; ok {
			results = append(results, vch)
		} else {
			vch, err := m.fetchVersionedChannels(ctx, repo, g, a, v)
			if err != nil {
				return nil, err
			}
			results = append(results, vch)
		}
	}
	return results, nil
}

// fetchVersionedChannels retrieves the artifact for a specific version and parses the AsyncAPI channels.
func (m *AsyncAPIImporterPlugin) fetchVersionedChannels(ctx context.Context, repository, groupId, artifactId, version string) (*VersionedChannels, error) {
	coords := jfrog.MavenCoordinates{
		GroupID:    groupId,
		ArtifactID: artifactId,
		Version:    version,
		Classifier: m.spec.Fetcher.Classifier,
		Extension:  m.spec.Fetcher.Packaging,
	}

	var buf bytes.Buffer
	if err := m.fetcher.RetrieveArtifact(ctx, repository, coords, &buf); err != nil {
		return nil, fmt.Errorf("failed to retrieve artifact for %v: %w", coords, err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		return nil, fmt.Errorf("failed to open artifact archive: %w", err)
	}

	specBytes, err := readZipFile(zr, m.spec.File)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q from artifact: %w", m.spec.File, err)
	}

	if m.spec.Fetcher.ReplaceProperties {
		props, err := readAllProperties(zr)
		if err != nil {
			return nil, fmt.Errorf("failed to read .properties files: %w", err)
		}
		specBytes = replacePropertyPlaceholders(specBytes, props)
	}

	spec, err := asyncapi.ParseBytes(specBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AsyncAPI spec: %w", err)
	}

	return &VersionedChannels{
		Version:  version,
		Channels: spec.SimpleChannels(),
	}, nil
}

// findLatestMatch finds the latest version in the available list that matches the major version of the provided catalog version.
func findLatestMatch(v catalog.Version, available []string) string {
	majorPrefix := fmt.Sprintf("v%d", v.Major)
	var candidates []string
	for _, a := range available {
		if semver.Major(semverNormalize(a)) == majorPrefix {
			candidates = append(candidates, a)
		}
	}
	latest := latestSemverVersions(candidates, 1)
	if len(latest) > 0 {
		return latest[0]
	}
	return ""
}
