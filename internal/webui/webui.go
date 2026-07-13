// Package webui serves the embedded dashboard (--ui): a Vue app (see the
// sibling ui/ project at the repo root) showing configured backends,
// registered inference hosts with live up/down status and their
// discovered models, registered downstream MCP servers, and a
// live-streaming log tail - all backed by the same /admin/* endpoints an
// external client could call directly.
//
// dist/ is the Vue app's build output (ui/'s vite.config.ts points its
// build outDir directly here, so no separate copy step is needed). A
// placeholder dist/index.html is committed so a bare `go build` never
// breaks on a fresh clone without Node installed; CI and goreleaser always
// run the real `npm run build` first, overwriting the placeholder with
// the actual dashboard before compiling release/test binaries.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves the embedded dashboard at "/".
func Handler() (http.Handler, error) {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}
	return http.FileServerFS(sub), nil
}
