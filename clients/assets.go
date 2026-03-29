package clients

import (
	"embed"
	"io/fs"
)

//go:embed web/dist
var dist embed.FS

// WebAssets returns the static assets for the frontend.
func WebAssets() (fs.FS, error) {
	return fs.Sub(dist, "web/dist")
}
