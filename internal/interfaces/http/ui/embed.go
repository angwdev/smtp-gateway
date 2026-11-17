// Package ui embeds the built single-page app. Only dist/.gitkeep is in git;
// `make ui` (or the Docker build) fills dist/ before the binary is compiled.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the built app rooted at dist/.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // unreachable: "dist" is a compile-time constant directory
	}
	return sub
}
