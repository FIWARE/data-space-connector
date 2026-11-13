package cli

import (
	"context"
	"fmt"

	"github.com/FIWARE/data-space-connector/config-tool/internal/launcher"
	"github.com/FIWARE/data-space-connector/config-tool/internal/server"
	"github.com/FIWARE/data-space-connector/config-tool/ui"
)

// runGUI serves the UI on loopback until ctx is cancelled. The token URL goes to stdout as the
// fallback for a failed or consumed launch; the browser only ever gets a launch code.
func runGUI(ctx context.Context, opts GUIOptions) error {
	srv, err := server.New(ui.Dist())
	if err != nil {
		return err
	}
	fmt.Fprintln(opts.Stdout, srv.URL())
	if !opts.NoBrowser {
		if err := launcher.Open(srv); err != nil {
			fmt.Fprintf(opts.Stderr, "%s: could not open a browser: %v; open the URL above instead\n", binaryName, err)
		}
	}
	return srv.Serve(ctx)
}
