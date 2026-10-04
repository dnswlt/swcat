package plugins

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/repo"
	"gopkg.in/yaml.v3"
)

func TestRegistryRelationshipPredicates(t *testing.T) {
	api := &catalog.API{Metadata: &catalog.Metadata{Name: "payments-api"}, Spec: &catalog.APISpec{}}
	api.SetDomain(catalog.MustParseRef("domain:payments"))
	component := &catalog.Component{Metadata: &catalog.Metadata{Name: "consumer"}, Spec: &catalog.ComponentSpec{
		ConsumesAPIs: []*catalog.LabelRef{{Ref: api.GetRef()}},
	}}
	repository := repo.NewRepository()
	for _, entity := range []catalog.Entity{api, component} {
		if err := repository.AddEntity(entity); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name, trigger, inhibit string
		want                   bool
	}{
		{"trigger matches", "kind:component consumesApis[domain=payments]", "", true},
		{"trigger does not match", "consumesApis[domain=other]", "", false},
		{"inhibit matches", "kind:component", "consumesApis[domain=payments]", false},
		{"inhibit does not match", "kind:component", "consumesApis[domain=other]", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := NewRegistry(&Config{Plugins: map[string]*Definition{
				"timestamp": {Kind: "TimestampPlugin", Trigger: tt.trigger, Inhibit: tt.inhibit},
			}}, Services{})
			if err != nil {
				t.Fatal(err)
			}
			names := registry.MatchingPlugins(component, repository)
			if (len(names) > 0) != tt.want {
				t.Fatalf("MatchingPlugins = %v, want match %v", names, tt.want)
			}
			result, err := registry.Run(t.Context(), repository, component)
			if err != nil {
				t.Fatal(err)
			}
			if _, ran := result.Annotations[AnnotPluginsUpdateTime]; ran != tt.want {
				t.Fatalf("plugin ran = %v, want %v", ran, tt.want)
			}
		})
	}
}

// Invalid predicates are rejected when the config is loaded, whatever the
// catalog holds, rather than failing (or never matching) when evaluated.
func TestRegistryRejectsInvalidPredicates(t *testing.T) {
	for _, tc := range []struct{ name, trigger, inhibit, err string }{
		{"unknown nested trigger attribute", "consumesApis[unknwn=x]", "", "invalid trigger expression"},
		{"unknown nested inhibit attribute", "kind=component", "consumesApis[unknwn=x]", "invalid inhibit expression"},
		// Finder providers such as lint are not available to plugin predicates.
		{"lint trigger", "lint=error", "", "invalid trigger expression"},
		{"lint inhibit", "kind=component", "lint=error", "invalid inhibit expression"},
		// Short-circuiting must not hide the broken regex.
		{"broken regex behind OR", "kind=component OR name~'[a-'", "", "invalid regular expression"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRegistry(&Config{Plugins: map[string]*Definition{
				"broken-plugin": {Kind: "TimestampPlugin", Trigger: tc.trigger, Inhibit: tc.inhibit},
			}}, Services{})
			if err == nil || !strings.Contains(err.Error(), tc.err) || !strings.Contains(err.Error(), "broken-plugin") {
				t.Fatalf("got error %v, want plugin name and %q", err, tc.err)
			}
		})
	}
}

func TestReadConfig(t *testing.T) {
	// Tests that the spec field of plugin configs are read properly.

	configYaml := `
plugins:
  asyncApiImporter:
    kind: AsyncAPIImporterPlugin
    trigger: |-
      kind:API AND type~'^kafka/'
    inhibit: |-
      annotation='swcat/visibility=internal'
    spec:
      fetcher:
        packaging: jar
        classifier: asyncapi
      file: META-INF/asyncapi.yaml
  grpcPlugin:
    kind: GRPCPlugin
    spec:
      address: localhost:50051
      config:
        foo: bar
`

	var cfg Config
	if err := yaml.Unmarshal([]byte(configYaml), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	if len(cfg.Plugins) != 2 {
		t.Errorf("Expected 2 plugins, got %d", len(cfg.Plugins))
	}

	def, ok := cfg.Plugins["asyncApiImporter"]
	if !ok {
		t.Fatal("asyncApiImporter not found")
	}

	if def.Kind != "AsyncAPIImporterPlugin" {
		t.Errorf("Expected kind AsyncAPIImporterPlugin, got %s", def.Kind)
	}

	grpcDef, ok := cfg.Plugins["grpcPlugin"]
	if !ok {
		t.Fatal("grpcPlugin not found")
	}

	if grpcDef.Kind != "GRPCPlugin" {
		t.Errorf("Expected kind GRPCPlugin, got %s", grpcDef.Kind)
	}

	// yaml.Node Kind: Document=1, Sequence=2, Mapping=4, Scalar=8, Alias=16
	// 0 is invalid/null?
	if def.Spec.Kind == 0 || def.Spec.Tag == "!!null" {
		t.Errorf("Spec seems to be null/empty. Kind: %d, Tag: %s", def.Spec.Kind, def.Spec.Tag)
	}
}

func TestRegistryRun_TimestampPluginAnnotation(t *testing.T) {
	configYaml := `
plugins:
  timestamp:
    kind: TimestampPlugin
    trigger: "kind:Component"
`
	cfg, err := ParseConfig([]byte(configYaml))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	registry, err := NewRegistry(cfg, Services{})
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	component := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "some-component"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     &catalog.Ref{Name: "owner"},
			System:    &catalog.Ref{Name: "system"},
		},
	}
	repository := repo.NewRepository()
	repository.AddEntity(component)

	before := time.Now().UTC()
	result, err := registry.Run(context.Background(), repository, component)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.Observations) != 0 {
		t.Errorf("Expected no observations, got %v", result.Observations)
	}
	value, ok := result.Annotations[AnnotPluginsUpdateTime]
	if !ok {
		t.Fatalf("Expected annotation %q, got %v", AnnotPluginsUpdateTime, result.Annotations)
	}
	ts, ok := value.(time.Time)
	if !ok {
		t.Fatalf("Expected annotation value of type time.Time, got %T: %v", value, value)
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("Annotation timestamp %v not in [%v, %v]", ts, before, after)
	}
}

func TestRegistryRun_TimestampPluginObservation(t *testing.T) {
	configYaml := `
plugins:
  timestamp:
    kind: TimestampPlugin
    trigger: "kind:Component"
    spec:
      target: observation
`
	cfg, err := ParseConfig([]byte(configYaml))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	registry, err := NewRegistry(cfg, Services{})
	if err != nil {
		t.Fatalf("NewRegistry failed: %v", err)
	}

	component := &catalog.Component{
		Metadata: &catalog.Metadata{Name: "some-component"},
		Spec: &catalog.ComponentSpec{
			Type:      "service",
			Lifecycle: "production",
			Owner:     &catalog.Ref{Name: "owner"},
			System:    &catalog.Ref{Name: "system"},
		},
	}
	repository := repo.NewRepository()
	repository.AddEntity(component)

	before := time.Now().UTC()
	result, err := registry.Run(context.Background(), repository, component)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if len(result.Annotations) != 0 {
		t.Errorf("Expected no annotations, got %v", result.Annotations)
	}
	obs, ok := result.Observations[AnnotPluginsUpdateTime]
	if !ok {
		t.Fatalf("Expected observation %q, got %v", AnnotPluginsUpdateTime, result.Observations)
	}
	if obs.Producer != "TimestampPlugin" {
		t.Errorf("Expected producer TimestampPlugin, got %q", obs.Producer)
	}
	if obs.UpdatedAt.Before(before) || obs.UpdatedAt.After(after) {
		t.Errorf("UpdatedAt %v not in [%v, %v]", obs.UpdatedAt, before, after)
	}
}

func TestParseInterval(t *testing.T) {
	configYaml := `
enabled: true
baseInterval: 24h`

	var sc SchedulerConfig
	if err := yaml.Unmarshal([]byte(configYaml), &sc); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if !sc.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if sc.BaseInterval != 24*time.Hour {
		t.Errorf("Interval = %v, want %v", sc.BaseInterval, 24*time.Hour)
	}
}

// noRepository is a RepositoryProvider whose catalog failed to load.
type noRepository struct{}

func (noRepository) GetRepository() *repo.Repository { return nil }

// A catalog that cannot be loaded is a runtime failure, reported as an error.
// As a query.Resolver, a nil *Repository is not nil, so Matches cannot catch it.
func TestRunWithoutCatalog(t *testing.T) {
	registry, err := NewRegistry(&Config{Plugins: map[string]*Definition{
		"timestamp": {Kind: "TimestampPlugin", Trigger: "consumesApis[name=x]"},
	}}, Services{})
	if err != nil {
		t.Fatal(err)
	}
	component := &catalog.Component{Metadata: &catalog.Metadata{Name: "c"}, Spec: &catalog.ComponentSpec{}}
	if result, err := registry.Run(t.Context(), noRepository{}, component); err == nil || result != nil {
		t.Fatalf("Run = %v, %v; want an error", result, err)
	}
}
