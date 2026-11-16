package launcher

import (
	"fmt"
	"html/template"
	"net/http"
)

// bootstrapPaths are the only requests the bootstrap handler answers; each one issues a launch code.
var bootstrapPaths = map[string]bool{"/": true, "/index.html": true}

// bootstrapPage moves the webview from Wails' own origin to the loopback server. That
// navigation is cross-site, which the server's 200 + meta-refresh launch exchange expects.
var bootstrapPage = template.Must(template.New("bootstrap").Parse(`<!doctype html>
<meta charset="utf-8">
<meta http-equiv="refresh" content="0;url={{.}}">
<title>` + WindowTitle + `</title>
`))

// bootstrapHandler serves the window's first page. Other paths (favicon, ...) get 404 and
// issue no launch code, so the window keeps at most one live code per load.
func bootstrapHandler(src LaunchSource) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !bootstrapPaths[r.URL.Path] {
			http.NotFound(w, r)
			return
		}
		url, err := src.LaunchURL()
		if err != nil {
			http.Error(w, fmt.Sprintf("launch URL: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = bootstrapPage.Execute(w, url)
	})
}
