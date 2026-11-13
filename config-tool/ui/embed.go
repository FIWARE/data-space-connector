// Package ui embeds the built single-page app; `make ui` fills dist/.
package ui

import (
	"embed"
	"io/fs"
)

const distDir = "dist"

//go:embed all:dist
var dist embed.FS

// Dist returns the built UI rooted at dist/; it holds only .gitkeep until the UI is built.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, distDir)
	if err != nil {
		// fs.Sub fails only on an invalid path, and distDir is a constant.
		panic(err)
	}
	return sub
}
