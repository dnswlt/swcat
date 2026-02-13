package plugins

import (
	"testing"

	"gopkg.in/yaml.v3"
)

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
      providerPlugin: mavenAsyncApiProvider
      targetAnnotation: swcat/asyncapi
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
