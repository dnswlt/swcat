package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/plugins/sbom"
	"github.com/dnswlt/swcat/internal/repo"
	"gopkg.in/yaml.v3"
)

func TestJFrogXrayPlugin_DetectDependencyMismatches(t *testing.T) {
	repository := repo.NewRepository()

	// Create some catalog entities
	e1 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "alpha"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	e2 := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: "beta",
			Annotations: map[string]string{
				JFrogXrayPluginCoordsAnnotation: "org.acme:beta",
			},
		},
		Spec: &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	e3 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "gamma"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}

	repository.AddEntity(e1)
	repository.AddEntity(e2)
	repository.AddEntity(e3)

	// Main component with some dependencies
	mainComp := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "main"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     &catalog.Ref{Name: "owner"},
			System:    &catalog.Ref{Name: "system"},
			DependsOn: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: catalog.KindComponent, Name: "alpha"}},
				{Ref: &catalog.Ref{Kind: catalog.KindComponent, Name: "beta"}},
			},
		},
	}
	repository.AddEntity(mainComp)

	p := &JFrogXrayPlugin{
		spec: &jfrogXrayPluginSpec{},
	}
	fullIdx := newCatalogIndexFromEntities(repository.AllEntities())

	t.Run("PerfectMatch", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				"org.acme:beta:2.0.0",
			},
		}
		missing, extra := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		if len(missing) != 0 || len(extra) != 0 {
			t.Errorf("expected 0 mismatches, got missing=%v, extra=%v", missing, extra)
		}
	})

	t.Run("MissingInCatalog", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				"org.acme:beta:2.0.0",
				"org.example:gamma:1.0.0", // gamma is in catalog but not in main's deps
			},
		}
		missing, extra := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		wantMissing := []string{"org.example:gamma:1.0.0"}
		if !slices.Equal(missing, wantMissing) {
			t.Errorf("got missing=%v, want %v", missing, wantMissing)
		}
		if len(extra) != 0 {
			t.Errorf("got extra=%v, want empty", extra)
		}
	})

	t.Run("MissingInSBOM", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				// beta is missing
			},
		}
		missing, extra := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		wantExtra := []string{"beta"}
		if !slices.Equal(extra, wantExtra) {
			t.Errorf("got extra=%v, want %v", extra, wantExtra)
		}
		if len(missing) != 0 {
			t.Errorf("got missing=%v, want empty", missing)
		}
	})

	t.Run("BothWaysMismatch", func(t *testing.T) {
		bom := &sbom.MiniBOM{
			Components: []string{
				"org.example:alpha:1.0.0",
				"org.example:gamma:1.0.0", // extra
				// beta missing
			},
		}
		missing, extra := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		wantMissing := []string{"org.example:gamma:1.0.0"}
		wantExtra := []string{"beta"}
		if !slices.Equal(missing, wantMissing) {
			t.Errorf("got missing=%v, want %v", missing, wantMissing)
		}
		if !slices.Equal(extra, wantExtra) {
			t.Errorf("got extra=%v, want %v", extra, wantExtra)
		}
	})
}

func TestJFrogXrayPlugin_DetectDependencyMismatches_Ignore(t *testing.T) {
	repository := repo.NewRepository()

	e1 := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "alpha"},
		Spec:     &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	e2 := &catalog.Component{
		Metadata: &catalog.Metadata{
			Name:        "beta",
			Annotations: make(map[string]string),
		},
		Spec: &catalog.ComponentSpec{Type: "service", Lifecycle: "production", Owner: &catalog.Ref{Name: "owner"}, System: &catalog.Ref{Name: "system"}},
	}
	repository.AddEntity(e1)
	repository.AddEntity(e2)

	mainComp := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "main"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     &catalog.Ref{Name: "owner"},
			System:    &catalog.Ref{Name: "system"},
			DependsOn: []*catalog.LabelRef{
				{Ref: &catalog.Ref{Kind: catalog.KindComponent, Name: "alpha"}},
			},
		},
	}
	repository.AddEntity(mainComp)

	p := &JFrogXrayPlugin{
		spec: &jfrogXrayPluginSpec{},
	}

	bom := &sbom.MiniBOM{
		Components: []string{
			"org.example:alpha:1.0.0",
			"org.example:beta:1.0.0", // beta is missing in mainComp deps
		},
	}

	t.Run("NoIgnore", func(t *testing.T) {
		fullIdx := newCatalogIndexFromEntities(repository.AllEntities())
		missing, _ := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		wantMissing := []string{"org.example:beta:1.0.0"}
		if !slices.Equal(missing, wantMissing) {
			t.Errorf("got missing=%v, want %v", missing, wantMissing)
		}
	})

	t.Run("IgnoreByEntityAnnotation", func(t *testing.T) {
		e2.Metadata.Annotations[JFrogXrayPluginLintIgnoreAnnotation] = "ignore"
		t.Cleanup(func() { delete(e2.Metadata.Annotations, JFrogXrayPluginLintIgnoreAnnotation) })

		fullIdx := newCatalogIndexFromEntities(repository.AllEntities())
		missing, _ := p.detectDependencyMismatches(bom, mainComp, fullIdx, repository)
		if len(missing) != 0 {
			t.Errorf("got missing=%v, want empty (beta entity is annotated as ignored)", missing)
		}
	})
}

type fakeJFrogClient struct {
	tags            []string
	sbomJSON        []byte
	failForVersions []string // tags for which to return an error
}

func (c *fakeJFrogClient) ListDockerTags(ctx context.Context, repository, image string) ([]string, error) {
	return c.tags, nil
}

func (c *fakeJFrogClient) XrayExportDetails(ctx context.Context, repository, image, version string) ([]byte, error) {
	if slices.Contains(c.failForVersions, version) {
		return nil, fmt.Errorf("no such version")
	}
	return c.sbomJSON, nil
}

// sbomJSON builds a minimal CycloneDX BOM JSON string with the given image
// version and library components (each as "name:version").
func sbomJSON(imageVersion string, libs ...string) string {
	type comp struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	components := make([]comp, 0, len(libs))
	for _, lib := range libs {
		name, version, _ := strings.Cut(lib, ":")
		components = append(components, comp{Type: "library", Name: name, Version: version})
	}
	compsJSON, _ := json.Marshal(components)
	return fmt.Sprintf(`{
		"bomFormat": "CycloneDX",
		"specVersion": "1.4",
		"metadata": {
			"component": {"type": "container", "name": "myimage", "version": %q}
		},
		"components": %s
	}`, imageVersion, compsJSON)
}

// newTestPlugin constructs a JFrogXrayPlugin with the given (fake) client.
func newTestPlugin(t *testing.T, client JFrogXrayClient) *JFrogXrayPlugin {
	t.Helper()
	specYAML := `lintMissingDependencies: false`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(specYAML), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	p, err := NewJFrogXrayBOMPlugin("test", doc.Content[0], client)
	if err != nil {
		t.Fatalf("NewJFrogXrayBOMPlugin: %v", err)
	}
	return p
}

// fakeEntity returns a Component entity with the given image name.
func fakeEntity(name string) *catalog.Component {
	return &catalog.Component{
		Metadata: &catalog.Metadata{
			Name: name,
			Annotations: map[string]string{
				JFrogDockerRepositoryAnnotation: "example.com/docker-repository",
			},
		},
		Spec: &catalog.ComponentSpec{Type: "service", Lifecycle: "production"},
	}
}

// TestLatestSemverVersions tests the pure tag-sorting helper.
func TestLatestSemverVersions(t *testing.T) {
	tags := []string{"latest", "1.0.0", "v2.3.0", "0.9.1", "v2.3.1", "not-a-version", "v2.3.1-beta"}
	got := latestSemverVersions(tags, 3)
	// v2.3.1-beta > v2.3.0 in semver (higher patch, pre-release < release of same version).
	want := []string{"v2.3.1", "v2.3.1-beta", "v2.3.0"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestJFrogXrayPlugin_Execute tests the happy path: tags fetched, latest version
// SBOM downloaded and parsed, annotations set correctly.
func TestJFrogXrayPlugin_Execute(t *testing.T) {
	const image = "myimage"
	const latestVersion = "1.3.0"

	sbomStr := sbomJSON(latestVersion,
		"com.example:alpha:1.0.0",
		"org.acme:beta:2.5.0",
	)

	p := newTestPlugin(t, &fakeJFrogClient{
		tags:     []string{"0.9.0", latestVersion, "1.2.0", "not-semver"},
		sbomJSON: []byte(sbomStr),
	})
	result, err := p.Execute(context.Background(), fakeEntity(image), &PluginArgs{Repository: repo.NewRepository()})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	obs, ok := result.Observations[JFrogXrayPluginTarget]
	if !ok {
		t.Fatalf("%q observation missing", JFrogXrayPluginTarget)
	}
	var bom sbom.MiniBOM
	if err := json.Unmarshal(obs.Value, &bom); err != nil {
		t.Fatalf("Cannot unmarshal MiniBOM: %v", err)
	}
	if want := "myimage"; bom.Name != want {
		t.Errorf("bom.Name = %q, want %q", bom.Name, want)
	}
	if bom.Version != latestVersion {
		t.Errorf("bom.Version = %q, want %q", bom.Version, latestVersion)
	}
	wantComponents := []string{"com.example:alpha:1.0.0", "org.acme:beta:2.5.0"}
	if len(bom.Components) != len(wantComponents) {
		t.Fatalf("bom.Components = %v, want %v", bom.Components, wantComponents)
	}
	for i, c := range wantComponents {
		if bom.Components[i] != c {
			t.Errorf("bom.Components[%d] = %q, want %q", i, bom.Components[i], c)
		}
	}
}

// TestJFrogXrayPlugin_Execute_Fallback verifies that when the SBOM download for
// the latest version fails, the plugin retries with the next version.
func TestJFrogXrayPlugin_Execute_Fallback(t *testing.T) {
	const image = "myimage"
	const latestVersion = "2.0.0"
	const fallbackVersion = "1.9.0"

	sbomStr := sbomJSON(fallbackVersion, "com.example:lib:3.0.0")

	p := newTestPlugin(t, &fakeJFrogClient{
		tags:            []string{latestVersion, fallbackVersion},
		sbomJSON:        []byte(sbomStr),
		failForVersions: []string{latestVersion},
	})
	result, err := p.Execute(context.Background(), fakeEntity(image), &PluginArgs{Repository: repo.NewRepository()})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	obs, ok := result.Observations[JFrogXrayPluginTarget]
	if !ok {
		t.Fatalf("%q observation missing", JFrogXrayPluginTarget)
	}
	var bom sbom.MiniBOM
	if err := json.Unmarshal(obs.Value, &bom); err != nil {
		t.Fatalf("Cannot unmarshal MiniBOM: %v", err)
	}
	if !ok {
		t.Fatalf("swcat/bom annotation missing or wrong type: %T", bom)
	}
	if want := "myimage"; bom.Name != want {
		t.Errorf("bom.Name = %q, want %q", bom.Name, want)
	}
	if bom.Version != fallbackVersion {
		t.Errorf("bom.Version = %q, want %q (fallback)", bom.Version, fallbackVersion)
	}
}

// TestJFrogXrayPlugin_Execute_Skip verifies that when the latest version has
// already been ingested, the plugin returns early with zero observations.
func TestJFrogXrayPlugin_Execute_Skip(t *testing.T) {
	const image = "myimage"
	const latestVersion = "1.3.0"

	p := newTestPlugin(t, &fakeJFrogClient{
		tags: []string{latestVersion, "1.2.0"},
		// SBOM download shouldn't even be called
		sbomJSON: nil,
	})

	entity := fakeEntity(image)
	catalog.MergeObservations(entity, map[string]catalog.Observation{
		JFrogXrayPluginTarget: {
			Version: latestVersion,
		},
	})

	result, err := p.Execute(context.Background(), entity, &PluginArgs{Repository: repo.NewRepository()})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(result.Observations) != 0 {
		t.Errorf("expected 0 observations, got %d", len(result.Observations))
	}
}
