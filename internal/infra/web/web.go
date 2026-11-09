// Package web holds the house-status page didban serves at /: one HTML file,
// its stylesheet, script and icon, embedded in the binary so the daemon stays
// a single file with no external assets (the page must load on a network that
// cannot reach a CDN).
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var static embed.FS

// Static returns the page's files, rooted at the static directory.
func Static() fs.FS {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err) // the directory is embedded at build time, so this cannot fail.
	}

	return sub
}
