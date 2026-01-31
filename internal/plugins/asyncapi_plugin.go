package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/plugins/asyncapi"
	"gopkg.in/yaml.v3"
)

type asyncAPIImporterPluginSpec struct {
	ProviderPlugin   string `yaml:"providerPlugin"`
	TargetAnnotation string `yaml:"targetAnnotation"`
}

type AsyncAPIImporterPlugin struct {
	name string
	spec *asyncAPIImporterPluginSpec
}

func newAsyncAPIImporterPlugin(name string, specYaml *yaml.Node) (*AsyncAPIImporterPlugin, error) {
	var spec asyncAPIImporterPluginSpec
	if err := specYaml.Decode(&spec); err != nil {
		return nil, fmt.Errorf("failed to decode AsyncAPIImporterPlugin spec for %s: %v", name, err)
	}

	if !catalog.IsValidAnnotation(spec.TargetAnnotation, "true") {
		return nil, fmt.Errorf("invalid targetAnnotation %q for plugin %s", spec.TargetAnnotation, name)
	}

	return &AsyncAPIImporterPlugin{
		name: name,
		spec: &spec,
	}, nil
}

func (m *AsyncAPIImporterPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	trigger, ok := args.Registry.triggers[m.spec.ProviderPlugin]
	if !ok {
		return nil, fmt.Errorf("plugin %q not found", m.spec.ProviderPlugin)
	}

	res, err := trigger.plugin.Execute(ctx, entity, args.EmptyArgs())
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

	channels := spec.SimpleChannels()
	marshaled, err := json.Marshal(channels)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal AsyncAPI channels: %w", err)
	}

	return &PluginResult{
		Annotations: map[string]string{
			m.spec.TargetAnnotation: string(marshaled),
		},
	}, nil
}
