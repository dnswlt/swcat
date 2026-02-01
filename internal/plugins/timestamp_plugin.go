package plugins

import (
	"context"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
)

type TimestampPlugin struct{}

const (
	AnnotExtensionUpdateTime = "swcat/extensions-update-time"
)

func (t TimestampPlugin) Execute(ctx context.Context, entity catalog.Entity, args *PluginArgs) (*PluginResult, error) {
	return &PluginResult{
		Annotations: map[string]any{
			AnnotExtensionUpdateTime: time.Now().UTC(),
		},
	}, nil
}
