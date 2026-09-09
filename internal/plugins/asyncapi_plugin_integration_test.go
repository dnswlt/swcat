//go:build integration

package plugins

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"slices"
	"testing"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/jfrog"
	"github.com/dnswlt/swcat/internal/plugins/asyncapi"
	"github.com/dnswlt/swcat/internal/repo"
	"gopkg.in/yaml.v3"
)

// fakeArtifactFetcher is an in-memory JFrogArtifactFetcher used by the
// AsyncAPIImporterPlugin integration tests.
type fakeArtifactFetcher struct {
	versions []string
	// files maps zip-archive entry name → file content.
	files map[string][]byte
	// versionFiles maps version → entry name → file content (optional override).
	versionFiles map[string]map[string][]byte

	// captured for assertions
	gotRepository string
	gotGroupId    string
	gotArtifactId string
	gotReleaseOK  bool
	gotCoords     jfrog.MavenCoordinates
}

func (f *fakeArtifactFetcher) SearchVersions(ctx context.Context, repository, groupId, artifactId string, releaseOnly bool) ([]string, error) {
	f.gotRepository = repository
	f.gotGroupId = groupId
	f.gotArtifactId = artifactId
	f.gotReleaseOK = releaseOnly
	return f.versions, nil
}

func (f *fakeArtifactFetcher) RetrieveArtifact(ctx context.Context, repository string, coords jfrog.MavenCoordinates, out io.Writer) error {
	f.gotCoords = coords
	zw := zip.NewWriter(out)
	files := f.files
	if vfs, ok := f.versionFiles[coords.Version]; ok {
		files = vfs
	}
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return zw.Close()
}

func TestAsyncAPIImporterPlugin_ExecuteInternal(t *testing.T) {
	const (
		entityName = "my-service"
		groupId    = "com.example"
		repository = "libs-release"
	)

	// AsyncAPI v3 spec with two operations and @@placeholders@@ that must be
	// substituted from the .properties file before parsing. One operation
	// declares a reply (request/reply), the other does not (fire-and-forget).
	specYAML := `
asyncapi: '3.0.0'
info:
  title: Test API
  version: 4.2.0
channels:
  signups:
    address: @@channel.signups.address@@
    messages:
      userSignedUp:
        name: UserSignedUp
  orders:
    address: @@channel.orders.address@@
    messages:
      orderCreated:
        name: OrderCreated
operations:
  onUserSignedUp:
    action: send
    channel:
      $ref: '#/channels/signups'
  onOrderCreated:
    action: receive
    channel:
      $ref: '#/channels/orders'
    reply:
      address:
        location: "$message.header#/replyTo"
`
	props := `
# A comment line
channel.signups.address=user/signedup
channel.orders.address=order/created
`

	fetcher := &fakeArtifactFetcher{
		versions: []string{"0.9.0", "1.2.0", "1.0.0"},
		files: map[string][]byte{
			"META-INF/asyncapi.yaml":    []byte(specYAML),
			"META-INF/swcat.properties": []byte(props),
		},
	}

	pluginYAML := `
fetcher:
  packaging: jar
  replaceProperties: true
file: META-INF/asyncapi.yaml
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(pluginYAML), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	plugin, err := newAsyncAPIImporterPlugin("asyncapi-test", doc.Content[0], fetcher)
	if err != nil {
		t.Fatalf("newAsyncAPIImporterPlugin: %v", err)
	}

	entity := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: entityName,
			Annotations: map[string]string{
				catalog.AnnotMavenGroupID: groupId,
				JFrogRepositoryAnnotation: repository,
			},
		},
		Spec: &catalog.ComponentSpec{Type: "service", Lifecycle: "production"},
	}

	repository_ := repo.NewRepository()
	repository_.AddEntity(entity)

	result, err := plugin.Execute(t.Context(), entity, &PluginArgs{Repository: repository_})
	if err != nil {
		t.Fatalf("executeInternal: %v", err)
	}

	// Verify fetcher was called with the resolved Maven coordinates.
	if fetcher.gotRepository != repository {
		t.Errorf("SearchVersions repository = %q, want %q", fetcher.gotRepository, repository)
	}
	if fetcher.gotGroupId != groupId || fetcher.gotArtifactId != entityName {
		t.Errorf("SearchVersions g/a = %q/%q, want %q/%q", fetcher.gotGroupId, fetcher.gotArtifactId, groupId, entityName)
	}
	if !fetcher.gotReleaseOK {
		t.Errorf("SearchVersions releaseOnly = false, want true")
	}
	wantCoords := jfrog.MavenCoordinates{
		GroupID:    groupId,
		ArtifactID: entityName,
		Version:    "1.2.0", // latest semver from fetcher.versions
		Extension:  "jar",
	}
	if fetcher.gotCoords != wantCoords {
		t.Errorf("RetrieveArtifact coords = %+v, want %+v", fetcher.gotCoords, wantCoords)
	}

	// Verify the observation: two operations with addresses substituted from the .properties file.
	obs, ok := result.Observations[AsyncAPIPluginTarget]
	if !ok {
		t.Fatalf("%q observation missing", AsyncAPIPluginTarget)
	}
	if obs.Producer != "asyncapi-test" {
		t.Errorf("observation Producer = %q, want %q", obs.Producer, "asyncapi-test")
	}
	if obs.Version != "1.2.0" {
		t.Errorf("observation Version = %q, want %q", obs.Version, "1.2.0")
	}
	var results []*VersionedSpec
	if err := json.Unmarshal(obs.Value, &results); err != nil {
		t.Fatalf("unmarshal observation: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d versioned results, want 1", len(results))
	}
	if results[0].AsyncAPIVersion != "3.0.0" {
		t.Errorf("AsyncAPIVersion = %q, want %q", results[0].AsyncAPIVersion, "3.0.0")
	}
	if results[0].Channels != nil {
		t.Errorf("Channels = %+v, want nil for a v3 spec", results[0].Channels)
	}
	operations := results[0].Operations
	if len(operations) != 2 {
		t.Fatalf("got %d operations, want 2: %+v", len(operations), operations)
	}
	// Index by name for order-independent assertions.
	byName := map[string]*asyncapi.SimpleOperation{}
	for _, op := range operations {
		byName[op.Name] = op
	}
	check := func(name, wantChannel, wantAddr, wantMsg string, wantReply bool) {
		t.Helper()
		op, ok := byName[name]
		if !ok {
			t.Errorf("operation %q missing", name)
			return
		}
		if op.Channel != wantChannel {
			t.Errorf("operation %q channel = %q, want %q", name, op.Channel, wantChannel)
		}
		if op.Address != wantAddr {
			t.Errorf("operation %q address = %q, want %q", name, op.Address, wantAddr)
		}
		if !slices.Equal(op.Messages, []string{wantMsg}) {
			t.Errorf("operation %q messages = %v, want [%s]", name, op.Messages, wantMsg)
		}
		if op.Reply != wantReply {
			t.Errorf("operation %q reply = %v, want %v", name, op.Reply, wantReply)
		}
	}
	check("onUserSignedUp", "signups", "user/signedup", "UserSignedUp", false)
	check("onOrderCreated", "orders", "order/created", "OrderCreated", true)

	// Sanity check that placeholder substitution actually ran.
	for _, op := range operations {
		if op.Address == "" || op.Address[0] == '@' {
			t.Errorf("operation %q: placeholder %q not substituted", op.Name, op.Address)
		}
	}
}

func TestAsyncAPIImporterPlugin_ExecuteInternal_MultipleVersions(t *testing.T) {
	const (
		entityName = "my-api"
		groupId    = "com.example"
		repository = "libs-release"
	)

	v1Spec := `
asyncapi: '3.0.0'
info:
  title: Test API v1
  version: 1.2.3
channels:
  v1channel:
    address: v1/addr
operations:
  v1op:
    action: send
    channel:
      $ref: '#/channels/v1channel'
`
	v2Spec := `
asyncapi: '3.0.0'
info:
  title: Test API v2
  version: 2.0.1
channels:
  v2channel:
    address: v2/addr
operations:
  v2op:
    action: send
    channel:
      $ref: '#/channels/v2channel'
`

	fetcher := &fakeArtifactFetcher{
		versions: []string{"1.2.3", "1.2.2", "2.0.1", "2.0.0"},
		versionFiles: map[string]map[string][]byte{
			"1.2.3": {"META-INF/asyncapi.yaml": []byte(v1Spec)},
			"2.0.1": {"META-INF/asyncapi.yaml": []byte(v2Spec)},
		},
	}

	pluginYAML := `
fetcher:
  packaging: jar
file: META-INF/asyncapi.yaml
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(pluginYAML), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	plugin, err := newAsyncAPIImporterPlugin("asyncapi-test", doc.Content[0], fetcher)
	if err != nil {
		t.Fatalf("newAsyncAPIImporterPlugin: %v", err)
	}

	entity := &catalog.API{
		Metadata: &catalog.Metadata{
			Name: entityName,
			Annotations: map[string]string{
				catalog.AnnotMavenGroupID: groupId,
				JFrogRepositoryAnnotation: repository,
			},
		},
		Spec: &catalog.APISpec{
			Versions: []*catalog.APISpecVersion{
				{Version: catalog.Version{RawVersion: "v1", Major: 1}},
				{Version: catalog.Version{RawVersion: "v2", Major: 2}},
			},
		},
	}

	repository_ := repo.NewRepository()
	repository_.AddEntity(entity)

	result, err := plugin.Execute(t.Context(), entity, &PluginArgs{Repository: repository_})
	if err != nil {
		t.Fatalf("executeInternal: %v", err)
	}

	obs, ok := result.Observations[AsyncAPIPluginTarget]
	if !ok {
		t.Fatalf("%q observation missing", AsyncAPIPluginTarget)
	}

	if obs.Meta["version-v1"] != "1.2.3" {
		t.Errorf("Meta[version-v1] = %q, want %q", obs.Meta["version-v1"], "1.2.3")
	}
	if obs.Meta["version-v2"] != "2.0.1" {
		t.Errorf("Meta[version-v2] = %q, want %q", obs.Meta["version-v2"], "2.0.1")
	}

	var results []*VersionedSpec
	if err := json.Unmarshal(obs.Value, &results); err != nil {
		t.Fatalf("unmarshal observation: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d versioned results, want 2", len(results))
	}

	// Verify that each result has its correct operations
	for _, r := range results {
		if r.Version == "1.2.3" {
			if len(r.Operations) != 1 || r.Operations[0].Channel != "v1channel" {
				t.Errorf("v1 operations mismatch: %+v", r.Operations)
			}
		} else if r.Version == "2.0.1" {
			if len(r.Operations) != 1 || r.Operations[0].Channel != "v2channel" {
				t.Errorf("v2 operations mismatch: %+v", r.Operations)
			}
		}
	}

	if _, ok := result.Observations["swcat-lint/finding-newer-version"]; ok {
		t.Errorf("expected no newer major version finding, but got one")
	}
}

func TestAsyncAPIImporterPlugin_ExecuteInternal_NewerMajorVersionAvailable(t *testing.T) {
	const (
		entityName = "my-api"
		groupId    = "com.example"
		repository = "libs-release"
	)

	v1Spec := `
asyncapi: '3.0.0'
info:
  title: Test API v1
  version: 1.2.3
channels:
  v1channel:
    address: v1/addr
`

	fetcher := &fakeArtifactFetcher{
		versions: []string{"1.2.3", "2.0.1", "3.0.0"},
		versionFiles: map[string]map[string][]byte{
			"1.2.3": {"META-INF/asyncapi.yaml": []byte(v1Spec)},
		},
	}

	pluginYAML := `
fetcher:
  packaging: jar
file: META-INF/asyncapi.yaml
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(pluginYAML), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	plugin, err := newAsyncAPIImporterPlugin("asyncapi-test", doc.Content[0], fetcher)
	if err != nil {
		t.Fatalf("newAsyncAPIImporterPlugin: %v", err)
	}

	entity := &catalog.API{
		Metadata: &catalog.Metadata{
			Name: entityName,
			Annotations: map[string]string{
				catalog.AnnotMavenGroupID: groupId,
				JFrogRepositoryAnnotation: repository,
			},
		},
		Spec: &catalog.APISpec{
			Versions: []*catalog.APISpecVersion{
				{Version: catalog.Version{RawVersion: "v1", Major: 1}},
			},
		},
	}

	repository_ := repo.NewRepository()
	repository_.AddEntity(entity)

	result, err := plugin.Execute(t.Context(), entity, &PluginArgs{Repository: repository_})
	if err != nil {
		t.Fatalf("executeInternal: %v", err)
	}

	findingObs, ok := result.Observations["swcat-lint/finding-newer-version"]
	if !ok {
		t.Fatalf("expected newer major version finding, but got none")
	}

	var finding api.LintFinding
	if err := json.Unmarshal(findingObs.Value, &finding); err != nil {
		t.Fatalf("failed to unmarshal finding: %v", err)
	}

	expectedMsg := "A newer major version (v3) is available in the repository but not listed in the entity's versions."
	if finding.Message != expectedMsg {
		t.Errorf("expected finding message %q, got %q", expectedMsg, finding.Message)
	}
}

func TestAsyncAPIImporterPlugin_ExecuteInternal_NoMatchingVersions(t *testing.T) {
	const (
		entityName = "my-api"
		groupId    = "com.example"
		repository = "libs-release"
	)

	fetcher := &fakeArtifactFetcher{
		versions: []string{"3.0.0"}, // Only version 3 is available
	}

	pluginYAML := `
fetcher:
  packaging: jar
file: META-INF/asyncapi.yaml
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(pluginYAML), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	plugin, err := newAsyncAPIImporterPlugin("asyncapi-test", doc.Content[0], fetcher)
	if err != nil {
		t.Fatalf("newAsyncAPIImporterPlugin: %v", err)
	}

	entity := &catalog.API{
		Metadata: &catalog.Metadata{
			Name: entityName,
			Annotations: map[string]string{
				catalog.AnnotMavenGroupID: groupId,
				JFrogRepositoryAnnotation: repository,
			},
		},
		Spec: &catalog.APISpec{
			Versions: []*catalog.APISpecVersion{
				{Version: catalog.Version{RawVersion: "v1", Major: 1}}, // Entity only has v1
			},
		},
	}

	repository_ := repo.NewRepository()
	repository_.AddEntity(entity)

	result, err := plugin.Execute(t.Context(), entity, &PluginArgs{Repository: repository_})
	if err == nil {
		t.Fatalf("Execute should have failed because no matching versions were found")
	}

	if result != nil {
		t.Errorf("result should be nil when Execute returns an error")
	}
}
