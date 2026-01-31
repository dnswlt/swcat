package plugins

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dnswlt/swcat/internal/catalog"
	"gopkg.in/yaml.v3"
)

type mavenArtifactExtractorPluginSpec struct {
	JarFile               string `yaml:"jarFile"`
	MavenCoordsAnnotation string `yaml:"mavenCoordsAnnotation"`
	DefaultGroupId        string `yaml:"defaultGroupId"`
	Packaging             string `yaml:"packaging"`
	Classifier            string `yaml:"classifier"`
	IncludeSnapshots      bool   `yaml:"includeSnapshots"`
	File                  string `yaml:"file"`
}

type MavenArtifactExtractorPlugin struct {
	name string
	spec *mavenArtifactExtractorPluginSpec
}

type MavenArtifactExtractorPluginRetVal struct {
	outputFile string
}

func (r MavenArtifactExtractorPluginRetVal) Files() []string {
	return []string{r.outputFile}
}

func newMavenArtifactExtractorPlugin(name string, specYaml *yaml.Node) (*MavenArtifactExtractorPlugin, error) {
	var spec mavenArtifactExtractorPluginSpec
	if err := specYaml.Decode(&spec); err != nil {
		return nil, fmt.Errorf("failed to decode MavenArtifactExtractorPlugin spec for %s: %v", name, err)
	}

	if spec.JarFile == "" {
		return nil, fmt.Errorf("field 'jarFile' not specified for plugin %s", name)
	}
	if spec.MavenCoordsAnnotation != "" {
		if !catalog.IsValidAnnotation(spec.MavenCoordsAnnotation, "true") {
			return nil, fmt.Errorf("invalid mavenCoordsAnnotation for plugin %s", name)
		}
	}
	if spec.File == "" {
		return nil, fmt.Errorf("field 'file' not specified for plugin %s", name)
	}

	return &MavenArtifactExtractorPlugin{
		name: name,
		spec: &spec,
	}, nil
}

func (m *MavenArtifactExtractorPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	meta := entity.GetMetadata()
	groupId := m.spec.DefaultGroupId
	artifactId := meta.Name

	if m.spec.MavenCoordsAnnotation != "" && meta.Annotations != nil {
		if val, ok := meta.Annotations[m.spec.MavenCoordsAnnotation]; ok {
			parts := strings.Split(val, ":")
			if len(parts) >= 2 {
				groupId = parts[0]
				artifactId = parts[1]
			}
		}
	}

	if groupId == "" {
		return nil, fmt.Errorf("groupId is required but not set (check defaultGroupId or annotation %s)", m.spec.MavenCoordsAnnotation)
	}

	tempDir, err := os.MkdirTemp(args.TempDir, "maven-plugin-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	outputFileName := filepath.Base(m.spec.File)
	outputFilePath := filepath.Join(tempDir, outputFileName)

	cmdArgs := []string{
		"-jar", m.spec.JarFile,
		"-g", groupId,
		"-a", artifactId,
		"-f", m.spec.File,
		"-o", outputFilePath,
	}

	if m.spec.Packaging != "" {
		cmdArgs = append(cmdArgs, "-p", m.spec.Packaging)
	}
	if m.spec.Classifier != "" {
		cmdArgs = append(cmdArgs, "-c", m.spec.Classifier)
	}
	if m.spec.IncludeSnapshots {
		cmdArgs = append(cmdArgs, "-s")
	}

	log.Printf("MavenArtifactExtractorPlugin command: java %s", strings.Join(cmdArgs, " "))
	cmd := exec.Command("java", cmdArgs...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("java command failed: %w, output: %s", err, string(output))
	}

	return &PluginResult{
		ReturnValue: MavenArtifactExtractorPluginRetVal{outputFile: outputFilePath},
	}, nil
}
