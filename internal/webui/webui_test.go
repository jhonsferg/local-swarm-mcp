package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHandler_ServesWhateverWasEmbedded(t *testing.T) {
	// The real dashboard when ui/ was built before `go build`, the
	// "not built" page otherwise - either way "/" answers.
	h, err := Handler()
	if err != nil {
		t.Fatalf("Handler: %v", err)
	}
	rec := get(t, h, "/")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "local-swarm-mcp") {
		t.Fatalf("GET / = %d %q", rec.Code, rec.Body.String())
	}
}

func TestHandlerFor_ServesBuiltDashboard(t *testing.T) {
	fsys := fstest.MapFS{
		"dist/app/index.html":        {Data: []byte(`<div id="app">local-swarm-mcp</div>`)},
		"dist/app/assets/index-x.js": {Data: []byte(`console.log(1)`)},
	}
	h, err := handlerFor(fsys, "dist/app")
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}
	if rec := get(t, h, "/"); !strings.Contains(rec.Body.String(), `<div id="app">`) {
		t.Fatalf("GET / = %q, want the built index.html", rec.Body.String())
	}
	if rec := get(t, h, "/assets/index-x.js"); rec.Code != http.StatusOK {
		t.Fatalf("GET asset = %d", rec.Code)
	}
}

func TestHandlerFor_ExplainsMissingBuild(t *testing.T) {
	// A fresh clone has only dist/.gitkeep: the binary must still build
	// and serve something useful instead of a bare 404.
	fsys := fstest.MapFS{"dist/.gitkeep": {}}
	h, err := handlerFor(fsys, "dist/app")
	if err != nil {
		t.Fatalf("handlerFor: %v", err)
	}
	rec := get(t, h, "/")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "npm run build") {
		t.Fatalf("GET / = %d %q, want the not-built page", rec.Code, rec.Body.String())
	}
	if rec := get(t, h, "/assets/missing.js"); rec.Code != http.StatusNotFound {
		t.Fatalf("GET asset without a build = %d, want 404", rec.Code)
	}
}
