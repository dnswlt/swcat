package plugins

import (
	"context"
	"fmt"
	"log"
	"maps"
	"os"
	"slices"

	"github.com/dnswlt/swcat/internal/api"
	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/query"
	"gopkg.in/yaml.v3"
)

// Definition defines the YAML structure of a plugin definition.
type Definition struct {
	Kind    string `yaml:"kind"`
	Trigger string `yaml:"trigger"`
	Inhibit string `yaml:"inhibit"`
	// Spec contains an arbitrary YAML subtree with plugin-specific configuration.
	// Each plugin will decode Spec into its own config struct.
	Spec yaml.Node `yaml:"spec"`
}

// Config is the top-level YAML node, i.e. the thing that is read from plugins.yml.
type Config struct {
	Plugins map[string]*Definition `yaml:"plugins"`
}

// Trigger combines a plugin with the conditions under which it should be executed.
type Trigger struct {
	// The condition under which this Trigger should activate.
	condition *query.Evaluator
	// The condition that blocks this Trigger from being activated.
	inhibitCondition *query.Evaluator
	// The plugin that should be executed if the trigger activates.
	plugin Plugin
}

// PluginResult is the return type of individual plugin executions.
type PluginResult struct {
	// Annotations that this plugin generated for an enitity.
	// The value type can be anything, but must be JSON marshallable,
	// which is how the annotation is represented in actual entity
	// annotations downstream.
	Annotations map[string]any

	// An optional additional return value that the plugin returns to its caller.
	ReturnValue any
}

// PluginArgs contains the arguments passed to any plugin execution.
type PluginArgs struct {
	// The registry which initiated the plugin execution.
	Registry *Registry

	// The base directory under which the plugin should create subfolders,
	// in case it generates output files for downstream plugins to use.
	//
	// TempDir itself gets entirely removed up by the plugin registry
	// after all plugins have executed.
	TempDir string

	// Additional arguments passed from the calling plugin or registry.
	Args map[string]any
}

// Plugin in the interface that any plugin implementation must satisfy.
type Plugin interface {
	Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error)
}

// Registry is the class used by clients of the plugins package to manage and execute plugins.
type Registry struct {
	config   *Config
	triggers map[string]*Trigger
}

// FilesReturnValue is the interface for plugin return values that contain
// a bunch of result files (typically generated or retrieved by the plugin).
type FilesReturnValue interface {
	Files() []string
}

// EmptyArgs returns a copy of this instance with an empty Args map.
func (a *PluginArgs) EmptyArgs() *PluginArgs {
	return &PluginArgs{
		Registry: a.Registry,
		TempDir:  a.TempDir,
	}
}

func ReadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugins config: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse plugins config: %w", err)
	}
	return &config, err
}

// NewRegistry creates a new registry and registers all plugins configured in the given config.
func NewRegistry(config *Config) (*Registry, error) {
	r := &Registry{
		config:   config,
		triggers: make(map[string]*Trigger),
	}

	for n, c := range config.Plugins {
		if err := r.registerPlugin(n, c); err != nil {
			return nil, err
		}
	}

	return r, nil
}

func (t *Trigger) Matches(e catalog.Entity) bool {
	if t.condition == nil {
		return false // No trigger condition => never trigger
	}
	if ok, _ := t.condition.Matches(e); !ok {
		return false
	}
	if t.inhibitCondition != nil {
		if inhibit, _ := t.inhibitCondition.Matches(e); inhibit {
			return false
		}
	}
	return true
}

// Matches returns true if the trigger of any plugin in the registry matches the given entity.
func (r *Registry) Matches(e catalog.Entity) bool {
	for _, t := range r.triggers {
		if t.Matches(e) {
			return true
		}
	}
	return false
}

// Plugins returns a list of names of all registered plugins.
func (r *Registry) Plugins() []string {
	keys := make([]string, 0, len(r.triggers))
	for k := range r.triggers {
		keys = append(keys, k)
	}
	return keys
}

func (r *Registry) Run(ctx context.Context, e catalog.Entity) (*api.CatalogExtensions, error) {
	tempDir, err := os.MkdirTemp("", "swcat-plugins")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir for plugin runs: %w", err)
	}
	defer func() {
		err := os.RemoveAll(tempDir)
		if err != nil {
			log.Printf("Failed to delete plugin temp dir %s: %v", tempDir, err)
		}
	}()

	annotations := make(map[string]any)
	execFunc := func(name string, p Plugin) error {
		log.Printf("Executing plugin %s for %s", name, e.GetRef().String())
		res, err := p.Execute(ctx, e, &PluginArgs{
			Registry: r,
			TempDir:  tempDir,
		})
		if err != nil {
			return fmt.Errorf("failed to execute plugin %s: %v", name, err)
		}
		if len(res.Annotations) > 0 {
			maps.Copy(annotations, res.Annotations)
		}
		return nil
	}
	for n, t := range r.triggers {
		if !t.Matches(e) {
			continue
		}
		if err := execFunc(n, t.plugin); err != nil {
			return nil, err
		}
	}
	if len(annotations) > 0 {
		// Execute the timestamp plugin to add an annotation indicating when they were last updated.
		if err := execFunc("timestampPlugin", TimestampPlugin{}); err != nil {
			return nil, err
		}
		keys := slices.Sorted(maps.Keys(annotations))
		log.Printf("Collected annotations for entity %s: %v", e.GetRef().String(), keys)
	}
	return &api.CatalogExtensions{
		Entities: map[string]*api.MetadataExtensions{
			e.GetRef().String(): {
				Annotations: annotations,
			},
		},
	}, nil
}

func (r *Registry) registerPlugin(name string, def *Definition) error {
	if _, ok := r.triggers[name]; ok {
		return fmt.Errorf("multiple definitions for plugin %s", name)
	}

	var condition *query.Evaluator
	if def.Trigger != "" {
		expr, err := query.Parse(def.Trigger)
		if err != nil {
			return fmt.Errorf("invalid trigger expression for plugin %s: %v", name, err)
		}
		condition = query.NewEvaluator(expr)
	}
	var inhibitCondition *query.Evaluator
	if def.Inhibit != "" {
		expr, err := query.Parse(def.Inhibit)
		if err != nil {
			return fmt.Errorf("invalid inhibit expression for plugin %s: %v", name, err)
		}
		inhibitCondition = query.NewEvaluator(expr)
	}

	trigger := &Trigger{
		condition:        condition,
		inhibitCondition: inhibitCondition,
	}
	switch def.Kind {
	case "AsyncAPIImporterPlugin":
		p, err := newAsyncAPIImporterPlugin(name, &def.Spec)
		if err != nil {
			return fmt.Errorf("failed to create AsyncAPIImporterPlugin %s: %w", name, err)
		}
		trigger.plugin = p
	case "MavenArtifactExtractorPlugin":
		p, err := newMavenArtifactExtractorPlugin(name, &def.Spec)
		if err != nil {
			return fmt.Errorf("failed to create MavenArtifactExtractorPlugin %s: %w", name, err)
		}
		trigger.plugin = p
	default:
		return fmt.Errorf("invalid plugin kind %s for plugin %s", def.Kind, name)

	}

	r.triggers[name] = trigger
	return nil
}
