package server

import (
	"errors"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const indexFile = "index.html"

const uiMissingPage = `<!doctype html>
<meta charset="utf-8">
<title>dsc-config</title>
<p>UI not built — run <code>make ui</code> in <code>config-tool/</code> and rebuild dsc-config.</p>
`

// staticHandler serves files from ui and falls back to index.html for client-side routes.
// A missing path with a file extension is a 404, so a stale tab gets no HTML for a script.
func staticHandler(ui fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed", "")
			return
		}
		index, err := fs.ReadFile(ui, indexFile)
		if errors.Is(err, fs.ErrNotExist) {
			writeHTML(w, http.StatusServiceUnavailable, uiMissingPage)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "")
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name != "" && name != indexFile && fs.ValidPath(name) {
			if info, err := fs.Stat(ui, name); err == nil && !info.IsDir() {
				http.ServeFileFS(w, r, ui, name)
				return
			}
		}
		if name != indexFile && path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set(headerContentType, contentTypeHTML)
		w.Header().Set(headerCacheControl, "no-cache")
		_, _ = w.Write(index)
	})
}
