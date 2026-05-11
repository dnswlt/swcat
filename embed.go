package swcat

import (
	"embed"
)

// "all:" prefix on static so the Vite manifest at static/dist/.vite/manifest.json
// is embedded too — the default embed pattern skips files whose names start with '.'.
//go:embed all:static
//go:embed templates
var Files embed.FS
