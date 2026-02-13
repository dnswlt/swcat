package plugins

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dnswlt/swcat/internal/catalog"
	plugin_pb "github.com/dnswlt/swcat/internal/plugins/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
	"gopkg.in/yaml.v3"
)

// grpcPluginSpec defines the static configuration for a GRPCPlugin.
type grpcPluginSpec struct {
	// Address is the host:port of the gRPC plugin server.
	Address string `yaml:"address"`

	// Config is an arbitrary map of configuration values passed to the plugin.
	Config map[string]any `yaml:"config"`
}

// GRPCPlugin is a plugin that delegates to a remote gRPC service.
type GRPCPlugin struct {
	name string
	spec *grpcPluginSpec

	conn     *grpc.ClientConn
	connOnce sync.Once
	connErr  error
}

// GRPCPluginOutput implements the FilesReturnValue interface.
type GRPCPluginOutput struct {
	generatedFiles []string
	annotations    map[string]any
}

func (o *GRPCPluginOutput) Files() []string {
	return o.generatedFiles
}

func newGRPCPlugin(name string, specYaml *yaml.Node) (*GRPCPlugin, error) {
	var spec grpcPluginSpec
	if err := specYaml.Decode(&spec); err != nil {
		return nil, fmt.Errorf("failed to decode GRPCPlugin spec for %s: %v", name, err)
	}

	if spec.Address == "" {
		return nil, fmt.Errorf("field 'address' not specified for plugin %s", name)
	}

	return &GRPCPlugin{
		name: name,
		spec: &spec,
	}, nil
}

func (p *GRPCPlugin) getConn() (*grpc.ClientConn, error) {
	p.connOnce.Do(func() {
		conn, err := grpc.NewClient(p.spec.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			p.connErr = fmt.Errorf("failed to connect to gRPC plugin server at %s: %w", p.spec.Address, err)
			return
		}
		p.conn = conn
	})
	return p.conn, p.connErr
}

func (p *GRPCPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	conn, err := p.getConn()
	if err != nil {
		return nil, err
	}

	client := plugin_pb.NewPluginServiceClient(conn)

	// Convert Entity to PB
	pbEntity := catalog.ToPB(entity)

	// Convert Config to StructPB
	pbConfig, err := structpb.NewStruct(p.spec.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to convert config to structpb: %w", err)
	}

	// Convert Args to StructPB
	pbArgs, err := structpb.NewStruct(args.Args)
	if err != nil {
		return nil, fmt.Errorf("failed to convert args to structpb: %w", err)
	}

	req := &plugin_pb.ExecuteRequest{
		PluginName: p.name,
		Entity:     pbEntity,
		Config:     pbConfig,
		Args:       pbArgs,
	}

	resp, err := client.Execute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("gRPC execution failed: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("remote plugin reported failure: %s", resp.Error)
	}

	// Process generated files
	var generatedFiles []string
	for _, f := range resp.Files {
		fullPath := filepath.Join(args.TempDir, f.Path)
		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for generated file %s: %w", f.Path, err)
		}
		if err := os.WriteFile(fullPath, f.Content, 0644); err != nil {
			return nil, fmt.Errorf("failed to write generated file %s: %w", f.Path, err)
		}
		generatedFiles = append(generatedFiles, fullPath)
	}

	// Process annotations
	annotations := make(map[string]any)
	for k, v := range resp.Annotations {
		annotations[k] = v.AsInterface()
	}

	output := &GRPCPluginOutput{
		generatedFiles: generatedFiles,
		annotations:    annotations,
	}

	return &PluginResult{
		Annotations: annotations,
		ReturnValue: output,
	}, nil
}
