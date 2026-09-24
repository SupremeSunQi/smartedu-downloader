//go:build production

package main

import (
	"embed"
	"io/fs"
)

//go:embed all:frontend/dist
var releaseAssets embed.FS

func loadAssets() fs.FS {
	assets, _ := fs.Sub(releaseAssets, "frontend/dist")
	return assets
}
