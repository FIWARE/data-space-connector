package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/FIWARE/data-space-connector/config-tool/internal/launcher"
	"github.com/FIWARE/data-space-connector/config-tool/internal/server"
	"github.com/FIWARE/data-space-connector/config-tool/ui"
)

// openWindow and openTab are the launcher entry points, variables so tests can replace them.
var (
	openWindow = launcher.Window
	openTab    = launcher.Open
)

// runGUI serves the UI on loopback until ctx is cancelled or the native window closes. The token
// URL goes to stdout as the fallback for a failed or consumed launch; a browser or window only
// ever gets a launch code.
func runGUI(ctx context.Context, opts GUIOptions) error {
	srv, err := server.New(ui.Dist())
	if err != nil {
		return err
	}
	fmt.Fprintln(opts.Stdout, srv.URL())
	if opts.NoBrowser {
		return srv.Serve(ctx)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	served := make(chan error, 1)
	go func() {
		served <- srv.Serve(ctx)
		// A server that stopped on its own must also close a window still showing it.
		cancel()
	}()
	switch err := openWindow(ctx, srv); {
	case err == nil:
		// The user closed the window: that ends the run.
		cancel()
	case errors.Is(err, launcher.ErrNoWindow):
		openBrowser(srv, opts)
	default:
		fmt.Fprintf(opts.Stderr, "%s: could not open a window: %v; opening a browser tab instead\n", binaryName, err)
		openBrowser(srv, opts)
	}
	return <-served
}

func openBrowser(src launcher.LaunchSource, opts GUIOptions) {
	if err := openTab(src); err != nil {
		fmt.Fprintf(opts.Stderr, "%s: could not open a browser: %v; open the URL above instead\n", binaryName, err)
	}
}
