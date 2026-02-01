// Package asyncapi provides a unified interface for parsing AsyncAPI specifications
// across different versions (v2.x and v3.x).
package asyncapi

import (
	"fmt"
	"os"
	"strings"

	v2 "github.com/dnswlt/swcat/internal/plugins/asyncapi/v2"
	v3 "github.com/dnswlt/swcat/internal/plugins/asyncapi/v3"
	"gopkg.in/yaml.v3"
)

// SimpleChannel is a simplified, version-agnostic representation of an AsyncAPI channel.
// It unifies the structural differences between v2.x (where address is the key)
// and v3.x (where address is a field and name is the key).
type SimpleChannel struct {
	Name     string   `json:"name"`
	Address  string   `json:"address"`
	Messages []string `json:"messages"`
}

// ParsedSpec is the common interface implemented by all version-specific spec adapters.
type ParsedSpec interface {
	SimpleChannels() []*SimpleChannel
}

// v2Adapter maps an AsyncAPI v2.x spec to the unified ParsedSpec interface.
type v2Adapter struct {
	spec *v2.Spec
}

func (a *v2Adapter) SimpleChannels() []*SimpleChannel {
	result := []*SimpleChannel{}
	for addr, ch := range a.spec.Channels {
		msgs := make([]string, 0)

		collectMsg := func(op *v2.Operation) {
			if op != nil && op.Message != nil {
				// In 2.x, use Name or Title as the message identifier.
				name := op.Message.Name
				if name == "" {
					name = op.Message.Title
				}
				if name != "" {
					msgs = append(msgs, name)
				}
			}
		}

		collectMsg(ch.Publish)
		collectMsg(ch.Subscribe)

		// In v2.x, the map key is the address; there's no distinct logical name.
		result = append(result, &SimpleChannel{
			Name:     addr,
			Address:  addr,
			Messages: msgs,
		})
	}
	return result
}

// v3Adapter maps an AsyncAPI v3.x spec to the unified ParsedSpec interface.
type v3Adapter struct {
	spec *v3.Spec
}

func (a *v3Adapter) SimpleChannels() []*SimpleChannel {
	result := []*SimpleChannel{}
	for name, ch := range a.spec.Channels {
		msgs := make([]string, 0, len(ch.Messages))
		for mname := range ch.Messages {
			msgs = append(msgs, mname)
		}
		result = append(result, &SimpleChannel{
			Name:     name,
			Address:  ch.Address,
			Messages: msgs,
		})
	}
	return result
}

// Parse reads an AsyncAPI specification from the given path, detects its version,
// and returns a version-agnostic ParsedSpec.
func Parse(path string) (ParsedSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}

	// Detect version from the "asyncapi" root field.
	var meta struct {
		AsyncAPI string `yaml:"asyncapi"`
	}
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML header from %s: %w", path, err)
	}

	if strings.HasPrefix(meta.AsyncAPI, "2.") {
		v2Spec, err := v2.Parse(path)
		if err != nil {
			return nil, err
		}
		return &v2Adapter{spec: v2Spec}, nil
	}

	if strings.HasPrefix(meta.AsyncAPI, "3.") || meta.AsyncAPI == "3" {
		v3Spec, err := v3.Parse(path)
		if err != nil {
			return nil, err
		}
		return &v3Adapter{spec: v3Spec}, nil
	}

	return nil, fmt.Errorf("unsupported AsyncAPI version %q: expected version 2.x or 3.x", meta.AsyncAPI)
}
