//go:build wails

package launcher

import (
	"context"
	"fmt"

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

// Window shows the UI in a native webview and blocks until the user closes it or ctx ends.
// The webview talks to the same loopback server as a browser tab would.
func Window(ctx context.Context, src LaunchSource) (err error) {
	// Wails panics when GTK cannot start (no display); the caller falls back to a browser tab, so
	// turn that into an error. Its log.Fatal/os.Exit start-up failures cannot be caught here.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("start window: %v", r)
		}
	}()
	// Closed once Run returns, so a late ctx cancellation never calls Quit on a stopped app.
	stopped := make(chan struct{})
	defer close(stopped)
	return wails.Run(&options.App{
		Title:     WindowTitle,
		Width:     windowWidth,
		Height:    windowHeight,
		MinWidth:  windowMinWidth,
		MinHeight: windowMinHeight,
		AssetServer: &assetserver.Options{
			Handler: bootstrapHandler(src),
		},
		OnStartup: func(wailsCtx context.Context) {
			go func() {
				select {
				case <-ctx.Done():
					wailsruntime.Quit(wailsCtx)
				case <-stopped:
				}
			}()
		},
	})
}
