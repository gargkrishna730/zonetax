// Package ui embeds the built ZoneTax dashboard (a React + TypeScript SPA using @xyflow/react
// for the interactive zone-to-zone traffic map — draggable/zoomable nodes, floating edges with
// cost-pill labels, hover tooltips) so the collector binary serves it directly with no separate
// frontend deployment. Source lives in ui/app (Vite + React); `npm run build` there outputs
// static assets into ui/dist, which this package embeds at Go build time via go:embed.
//
// This replaced an earlier plain HTML/CSS/JS dashboard pulling D3.js from a CDN. That version
// went through several iterations of hand-rolled SVG drag/zoom/curve math (see ROADMAP.md for
// the history) — each iteration fixed one bug class while introducing another, because
// reimplementing what a dedicated graph-visualization library already solves is exactly the
// kind of problem prone to that. React Flow (@xyflow/react) owns node dragging, pan/zoom, and
// edge-attachment geometry now, which is a more maintainable foundation for further UI work.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var files embed.FS

// FS returns the embedded UI files rooted at ui/dist (not ui/, which also contains this Go
// source file and the unbuilt ui/app source tree) — suitable for
// http.FileServer(http.FS(ui.FS())).
func FS() fs.FS {
	sub, err := fs.Sub(files, "dist")
	if err != nil {
		// dist is committed to the repo and always present at build time (see ui/app's
		// package.json build script + the CI/Docker build step that runs it before `go build`);
		// this can only fail if that invariant is broken, which is a build-time bug worth
		// failing loudly on rather than serving a broken UI silently.
		panic("ui: embedded dist directory missing: " + err.Error())
	}
	return sub
}
