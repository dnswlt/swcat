package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"gopkg.in/yaml.v3"
)

// TimestampPlugin writes the current time to an entity.
// Target selects where the timestamp is written: "annotation" (default)
// or "observation".
type TimestampPlugin struct {
	Target string `yaml:"target"`
}

const (
	AnnotPluginsUpdateTime = "swcat/plugins-update-time"
)

func newTimestampPlugin(name string, specYaml *yaml.Node) (TimestampPlugin, error) {
	var p TimestampPlugin
	if specYaml.Kind != 0 {
		if err := specYaml.Decode(&p); err != nil {
			return TimestampPlugin{}, fmt.Errorf("failed to decode TimestampPlugin spec for %s: %v", name, err)
		}
	}
	switch p.Target {
	case "", "annotation", "observation":
	default:
		return TimestampPlugin{}, fmt.Errorf("invalid target %q for TimestampPlugin %s", p.Target, name)
	}
	return p, nil
}

func (t TimestampPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	now := time.Now().UTC()
	switch t.Target {
	case "", "annotation":
		return &PluginResult{
			Annotations: map[string]any{
				AnnotPluginsUpdateTime: now,
			},
		}, nil
	case "observation":
		value, err := json.Marshal(now)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal timestamp: %w", err)
		}
		return &PluginResult{
			Observations: map[string]catalog.Observation{
				AnnotPluginsUpdateTime: {
					Value:     value,
					Producer:  "TimestampPlugin",
					UpdatedAt: now,
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("invalid TimestampPlugin target %q", t.Target)
	}
}
