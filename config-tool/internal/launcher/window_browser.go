//go:build !wails

package launcher

import "context"

// Window always returns ErrNoWindow: release binaries are pure Go and open a browser tab.
func Window(context.Context, LaunchSource) error {
	return ErrNoWindow
}
