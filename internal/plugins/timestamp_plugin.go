package plugins

import (
	"context"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
)

type TimestampPlugin struct{}

const (
	AnnotPluginsUpdateTime = "swcat/plugins-update-time"
)

func (t TimestampPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	return &PluginResult{
		Annotations: map[string]any{
			AnnotPluginsUpdateTime: time.Now().UTC(),
		},
	}, nil
}
