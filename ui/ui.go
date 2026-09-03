// Package ui embeds the static ZoneTax dashboard (Sankey flow diagram + top-offenders table)
// so the collector binary serves it directly with no separate frontend deployment or build
// step — plain HTML/CSS/JS pulling D3.js from a CDN, polling the collector's own REST API.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed index.html
var files embed.FS

// FS returns the embedded UI files rooted at their own directory, suitable for
// http.FileServer(http.FS(ui.FS())).
func FS() fs.FS {
	return files
}
