package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"gopkg.in/yaml.v3"
)

// externalPluginSpec defines the static configuration for an ExternalPlugin.
type externalPluginSpec struct {
	// Command is the executable to run (e.g. "java", "python", "/path/to/script").
	Command string `yaml:"command"`

	// Args are static command-line arguments passed to the external process.
	// (E.g., "-jar", "myapp.jar")
	Args []string `yaml:"args"`

	// Config is an arbitrary map of configuration values that will be passed
	// to the external process via the JSON input.
	//
	// The specific structure of this map must be defined by the external process,
	// and conformed to by the plugin configuration in plugins.yml.
	Config map[string]any `yaml:"config"`

	// Verbose enables logging of input and output JSON payloads.
	Verbose bool `yaml:"verbose"`
}

// ExternalPlugin is a generic plugin that delegates to an external process.
type ExternalPlugin struct {
	name string
	spec *externalPluginSpec
}

// ExternalPluginInput is the JSON structure sent to the external process's stdin.
type ExternalPluginInput struct {
	Entity  catalog.Entity `json:"entity"`
	Config  map[string]any `json:"config"`
	TempDir string         `json:"tempDir"`
	Args    map[string]any `json:"args"`
}

// ExternalPluginOutput is the JSON structure expected from the external process's stdout.
type ExternalPluginOutput struct {
	// Success indicates if the operation was successful.
	Success bool `json:"success"`

	// Error message - populated if Success is false.
	Error string `json:"error,omitempty"`

	// GeneratedFiles is a list of file paths generated/extracted by the plugin.
	// This maps to the FilesReturnValue interface.
	GeneratedFiles []string `json:"generatedFiles,omitempty"`

	// Annotations to be added to the entity.
	Annotations map[string]any `json:"annotations,omitempty"`
}

// Files returns the list of generated files, satisfying the FilesReturnValue interface.
func (o *ExternalPluginOutput) Files() []string {
	return o.GeneratedFiles
}

func newExternalPlugin(name string, specYaml *yaml.Node) (*ExternalPlugin, error) {
	var spec externalPluginSpec
	if err := specYaml.Decode(&spec); err != nil {
		return nil, fmt.Errorf("failed to decode ExternalPlugin spec for %s: %v", name, err)
	}

	if spec.Command == "" {
		return nil, fmt.Errorf("field 'command' not specified for plugin %s", name)
	}

	return &ExternalPlugin{
		name: name,
		spec: &spec,
	}, nil
}

func (p *ExternalPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	// Prepare Input JSON
	input := ExternalPluginInput{
		Entity:  entity,
		Config:  p.spec.Config,
		TempDir: args.TempDir,
		Args:    args.Args,
	}

	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input JSON: %w", err)
	}

	if p.spec.Verbose {
		log.Printf("[%s] Input JSON: %s", p.name, string(inputBytes))
	}

	// Prepare Command
	cmd := exec.CommandContext(ctx, p.spec.Command, p.spec.Args...)
	cmd.Stdin = bytes.NewReader(inputBytes)

	// Capture stdout (for JSON response) and stderr (for logging)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("Executing ExternalPlugin %s: %s %s", p.name, p.spec.Command, strings.Join(p.spec.Args, " "))

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("external process execution failed: %w, stderr: %s", err, stderr.String())
	}

	// Log stderr if any (it might contain helpful info even on success)
	if stderr.Len() > 0 {
		log.Printf("[%s stderr]: %s", p.name, stderr.String())
	}

	// Parse Output JSON
	if p.spec.Verbose {
		log.Printf("[%s] Output JSON: %s", p.name, stdout.String())
	}

	var output ExternalPluginOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil, fmt.Errorf("failed to parse output JSON: %w, stdout was: %q", err, stdout.String())
	}

	if !output.Success {
		return nil, fmt.Errorf("plugin reported failure: %s", output.Error)
	}

	return &PluginResult{
		Annotations: output.Annotations,
		ReturnValue: &output,
	}, nil
}
