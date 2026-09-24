//go:build !production

package main

import (
	"embed"
	"io/fs"
)

//go:embed frontend/index.html
var developmentAssets embed.FS

func loadAssets() fs.FS {
	assets, _ := fs.Sub(developmentAssets, "frontend")
	return assets
}
