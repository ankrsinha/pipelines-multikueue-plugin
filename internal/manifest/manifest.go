package manifest

import (
	"embed"
	_ "embed"
)

//go:embed manifests/*.yaml
var ManifestFS embed.FS
