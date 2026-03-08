package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/plugins/asyncapi"
	"gopkg.in/yaml.v3"
)

type asyncAPIImporterPluginSpec struct {
	ProviderPlugin   string `yaml:"providerPlugin"`
	TargetAnnotation string `yaml:"targetAnnotation"`
	File             string `yaml:"file"`
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

	if spec.File == "" {
		return nil, fmt.Errorf("field 'file' not specified for plugin %s", name)
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
		Annotations: map[string]any{
			m.spec.TargetAnnotation: map[string]any{
				"$data": spec.SimpleChannels(),
				"$meta": map[string]string{
					"createTime": now.Format("2006-01-02 15:04:05"),
				},
			},
		},
	}, nil
}
