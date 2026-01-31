package plugins

import (
	"context"
	"fmt"
	"log"
	"maps"
	"os"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/query"
	"gopkg.in/yaml.v3"
)

// Definition defines the YAML structure of a plugin definition.
type Definition struct {
	Kind    string    `yaml:"kind"`
	Trigger string    `yaml:"trigger"`
	Inhibit string    `yaml:"inhibit"`
	Spec    yaml.Node `yaml:"spec"`
}

// Config is the top-level YAML node, i.e. the thing that is read from plugins.yml.
type Config struct {
	Plugins map[string]*Definition `yaml:"plugins"`
}

type Trigger struct {
	// The condition under which this Trigger should activate.
	condition *query.Evaluator
	// The condition that blocks this Trigger from being activated.
	inhibitCondition *query.Evaluator
	// The plugin that should be executed if the trigger activates.
	plugin Plugin
}

type PluginResult struct {
	// Annotations that this plugin generated for an enitity.
	Annotations map[string]string

	// An optional additional return value that the plugin returns to its caller.
	ReturnValue any
}

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

type Plugin interface {
	Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error)
}

type Registry struct {
	config   *Config
	triggers map[string]*Trigger
}

// FilesReturnValue is the interface for plugin return values that contain
// a bunch of result files (typically generated or retrieved by the plugin).
type FilesReturnValue interface {
	Files() []string
}

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

func (r *Registry) Run(ctx context.Context, e catalog.Entity) error {
	tempDir, err := os.MkdirTemp("", "swcat-plugins")
	if err != nil {
		return fmt.Errorf("failed to create temp dir for plugin runs: %w", err)
	}
	defer func() {
		err := os.RemoveAll(tempDir)
		if err != nil {
			log.Printf("Failed to delete plugin temp dir %s: %v", tempDir, err)
		}
	}()

	annotations := make(map[string]string)
	for n, t := range r.triggers {
		if !t.Matches(e) {
			continue
		}
		log.Printf("Executing plugin %s for %s", n, e.GetRef().String())
		res, err := t.plugin.Execute(ctx, e, &PluginArgs{
			Registry: r,
			TempDir:  tempDir,
		})
		if err != nil {
			return fmt.Errorf("failed to execute plugin %s: %v", n, err)
		}
		if len(res.Annotations) > 0 {
			maps.Copy(annotations, res.Annotations)
		}
	}
	// TODO: Do something with these annotations (store in sidecar file).
	if len(annotations) > 0 {
		log.Printf("Collected %d annotations for entity %s: %v", len(annotations), e.GetRef().String(), annotations)
	}
	return nil
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
