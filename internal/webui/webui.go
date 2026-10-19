// Package webui serves the embedded dashboard (--ui): a Vue app (see the
// sibling ui/ project at the repo root) showing configured backends,
// registered inference hosts with live up/down status and their
// discovered models, registered downstream MCP servers, and a
// live-streaming log tail - all backed by the same /admin/* endpoints an
// external client could call directly.
//
// The Vue build output is never committed. ui/'s vite.config.ts builds
// straight into dist/app (go:embed can't reach outside this package's
// directory); only dist/.gitkeep is tracked, so a bare `go build` on a
// fresh clone without Node still compiles - its dashboard just explains
// how to build the real one. CI and the release workflow always run
// `npm run build` first, so every published binary embeds the real
// dashboard.
package webui

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var distFS embed.FS

// notBuiltPage is served in place of the dashboard by a binary compiled
// without the Vue build output.
const notBuiltPage = `<!doctype html>
<html lang="en">
<head><meta charset="utf-8"><title>local-swarm-mcp</title></head>
<body style="font-family: system-ui, sans-serif; max-width: 40rem; margin: 3rem auto; padding: 0 1rem">
<h1>local-swarm-mcp</h1>
<p>This binary was built without the dashboard. The daemon and its
<code>/admin</code> and <code>/mcp</code> endpoints work normally.</p>
<p>To get the dashboard, use a release binary, or build it from source:</p>
<pre>cd ui &amp;&amp; npm ci &amp;&amp; npm run build
cd .. &amp;&amp; go build ./cmd/local-swarm-mcp</pre>
</body>
</html>
`

// Handler serves the embedded dashboard at "/", or a page explaining how
// to build it when this binary was compiled without it.
func Handler() (http.Handler, error) {
	return handlerFor(distFS, "dist/app")
}

// Built reports whether the dashboard was embedded at build time.
func Built() bool {
	_, err := fs.Stat(distFS, "dist/app/index.html")
	return err == nil
}

func handlerFor(fsys fs.FS, dir string) (http.Handler, error) {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(sub, "index.html"); errors.Is(err, fs.ErrNotExist) {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(notBuiltPage))
		}), nil
	} else if err != nil {
		return nil, err
	}
	return http.FileServerFS(sub), nil
}
