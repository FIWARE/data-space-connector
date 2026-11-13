//go:build wails

package launcher

import (
	"context"
	"fmt"
	"html/template"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	windowWidth     = 1280
	windowHeight    = 860
	windowMinWidth  = 960
	windowMinHeight = 640
)

// bootstrapPage moves the webview from Wails' own origin to the loopback server. That
// navigation is cross-site, which the server's 200 + meta-refresh launch exchange expects.
var bootstrapPage = template.Must(template.New("bootstrap").Parse(`<!doctype html>
<meta charset="utf-8">
<meta http-equiv="refresh" content="0;url={{.}}">
<title>` + WindowTitle + `</title>
`))

// Window shows the UI in a native webview and blocks until the user closes it or ctx ends.
// The webview talks to the same loopback server as a browser tab would.
func Window(ctx context.Context, src LaunchSource) (err error) {
	// Wails panics instead of failing when the webview cannot start (no display, no GTK); the
	// caller falls back to a browser tab, so turn that into an error.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("start window: %v", r)
		}
	}()
	return wails.Run(&options.App{
		Title:     WindowTitle,
		Width:     windowWidth,
		Height:    windowHeight,
		MinWidth:  windowMinWidth,
		MinHeight: windowMinHeight,
		AssetServer: &assetserver.Options{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				url, err := src.LaunchURL()
				if err != nil {
					http.Error(w, fmt.Sprintf("launch URL: %v", err), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_ = bootstrapPage.Execute(w, url)
			}),
		},
		OnStartup: func(wailsCtx context.Context) {
			go func() {
				<-ctx.Done()
				wailsruntime.Quit(wailsCtx)
			}()
		},
	})
}
