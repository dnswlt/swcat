package swcat

import (
	"embed"
)

//go:embed static
//go:embed templates
var Files embed.FS
