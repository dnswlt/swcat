package plugins

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/jfrog"
	"github.com/dnswlt/swcat/internal/plugins/asyncapi"
	"gopkg.in/yaml.v3"
)

const (
	// The status field that the simplified representation of AsyncAPI channels gets written to.
	AsyncAPIPluginTarget = "swcat-plugins/asyncapi-channels"
)

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
	// If specified, uses the named plugin to fetch the AsyncAPI spec.
	ProviderPlugin string `yaml:"providerPlugin"`
	// Otherwise, the built-in JFrog client is used to fetch it, based on the
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

func newAsyncAPIImporterPlugin(name string, specYaml *yaml.Node, fetcher JFrogArtifactFetcher) (*AsyncAPIImporterPlugin, error) {
	var spec asyncAPIImporterPluginSpec
	if err := specYaml.Decode(&spec); err != nil {
		return nil, fmt.Errorf("failed to decode AsyncAPIImporterPlugin spec for %s: %v", name, err)
	}

	if spec.File == "" {
		return nil, fmt.Errorf("field 'file' not specified for plugin %s", name)
	}

	if spec.ProviderPlugin != "" {
		if spec.Fetcher != nil {
			return nil, fmt.Errorf("both 'providerPlugin' and 'fetcher' specified for plugin %s", name)
		}
	} else if spec.Fetcher != nil {
		if fetcher == nil {
			return nil, fmt.Errorf("no JFrogArtifactFetcher provided: %w", ErrPreconditionFailed)
		}
		if spec.Fetcher.Packaging == "" {
			return nil, fmt.Errorf("field 'fetcher.packaging' not specified for plugin %s", name)
		}
	} else {
		return nil, fmt.Errorf("neither 'fetcher' nor 'providerPlugin' specified for plugin %s", name)
	}

	return &AsyncAPIImporterPlugin{
		name:    name,
		spec:    &spec,
		fetcher: fetcher,
	}, nil
}

func (m *AsyncAPIImporterPlugin) executeInternal(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	artifactId := entity.GetMetadata().Name
	groupId, _ := args.Repository.IAnnotation(entity, catalog.AnnotMavenGroupID)
	var version string
	repository, _ := args.Repository.IAnnotation(entity, JFrogRepositoryAnnotation)

	if mc, ok := entity.GetMetadata().Annotations[catalog.AnnotMavenCoords]; ok {
		gav := parseGAV(mc)
		groupId = gav.g
		artifactId = gav.a
		if gav.v != "" {
			version = gav.v
		}
	}
	if groupId == "" {
		return nil, fmt.Errorf("no Maven groupId for %v (set %q or %q annotation)",
			entity.GetQName(), catalog.AnnotMavenGroupID, catalog.AnnotMavenCoords)
	}
	if repository == "" {
		return nil, fmt.Errorf("no JFrog repository for %v (set %q annotation)",
			entity.GetQName(), JFrogRepositoryAnnotation)
	}

	if version == "" {
		versions, err := m.fetcher.SearchVersions(ctx, repository, groupId, artifactId, true)
		if err != nil {
			return nil, fmt.Errorf("failed to search versions for %s:%s: %w", groupId, artifactId, err)
		}
		latest := latestSemverVersions(versions, 1)
		if len(latest) == 0 {
			return nil, fmt.Errorf("no valid semver versions found for %s:%s", groupId, artifactId)
		}
		version = latest[0]
	}

	coords := jfrog.MavenCoordinates{
		GroupID:    groupId,
		ArtifactID: artifactId,
		Version:    version,
		Classifier: m.spec.Fetcher.Classifier,
		Extension:  m.spec.Fetcher.Packaging,
	}

	// Both .zip and .jar artifacts are zip archives.
	var buf bytes.Buffer
	if err := m.fetcher.RetrieveArtifact(ctx, repository, coords, &buf); err != nil {
		return nil, fmt.Errorf("failed to retrieve artifact for %v: %w", entity.GetQName(), err)
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

	now := time.Now()
	return &PluginResult{
		Observations: map[string]catalog.Observation{
			AsyncAPIPluginTarget: {
				Producer:  m.name,
				Value:     api.MustMarshalJSON(spec.SimpleChannels()),
				UpdatedAt: now,
				Version:   spec.Version(),
			},
		},
	}, nil
}

// readZipFile reads and returns the contents of the named file from the archive.
func readZipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("file %q not found in archive", name)
}

// readAllProperties reads every .properties file in the archive and merges
// them into a single key/value map. On key collisions, last write wins.
func readAllProperties(zr *zip.Reader) (map[string]string, error) {
	props := map[string]string{}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".properties") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		for k, v := range parseProperties(data) {
			props[k] = v
		}
	}
	return props, nil
}

// parseProperties parses Java-style .properties content into a key/value map.
// Recognises '=' or ':' as separators; lines starting with '#' or '!' are comments.
func parseProperties(data []byte) map[string]string {
	props := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' || line[0] == '!' {
			continue
		}
		sep := strings.IndexAny(line, "=:")
		if sep < 0 {
			continue
		}
		k := strings.TrimSpace(line[:sep])
		v := strings.TrimSpace(line[sep+1:])
		props[k] = v
	}
	return props
}

// replacePropertyPlaceholders replaces all @@key@@ placeholders in data with
// the corresponding value from props. Unknown placeholders are left untouched.
func replacePropertyPlaceholders(data []byte, props map[string]string) []byte {
	if len(props) == 0 {
		return data
	}
	pairs := make([]string, 0, len(props)*2)
	for k, v := range props {
		pairs = append(pairs, "@@"+k+"@@", v)
	}
	return []byte(strings.NewReplacer(pairs...).Replace(string(data)))
}

func (m *AsyncAPIImporterPlugin) executeProvider(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	trigger, ok := args.Registry.triggers[m.spec.ProviderPlugin]
	if !ok {
		return nil, fmt.Errorf("plugin %q not found", m.spec.ProviderPlugin)
	}

	providerArgs := args.EmptyArgs()
	providerArgs.Args = map[string]any{
		"file": m.spec.File,
	}

	res, err := trigger.plugin.Execute(ctx, entity, providerArgs)
	if err != nil {
		return nil, fmt.Errorf("provider plugin %q failed: %w", m.spec.ProviderPlugin, err)
	}

	rv, ok := res.ReturnValue.(FilesReturnValue)
	if !ok {
		return nil, fmt.Errorf("unexpected return value type from provider plugin, got %T", res.ReturnValue)
	}
	if len(rv.Files()) != 1 {
		return nil, fmt.Errorf("expected 1 output file from provider plugin, got %d", len(rv.Files()))
	}

	spec, err := asyncapi.Parse(rv.Files()[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse AsyncAPI spec: %w", err)
	}

	now := time.Now()
	return &PluginResult{
		Observations: map[string]catalog.Observation{
			AsyncAPIPluginTarget: {
				Producer:  m.name,
				Value:     api.MustMarshalJSON(spec.SimpleChannels()),
				UpdatedAt: now,
				Version:   spec.Version(),
			},
		},
	}, nil
}

func (m *AsyncAPIImporterPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	if m.spec.ProviderPlugin != "" {
		return m.executeProvider(ctx, entity, args)
	}
	return m.executeInternal(ctx, entity, args)
}
