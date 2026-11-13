package launcher

import "errors"

// WindowTitle is the native window's title, matching the UI's title bar.
const WindowTitle = "DSC Configurator"

// ErrNoWindow reports a build without the wails tag; the caller opens a browser tab instead.
var ErrNoWindow = errors.New("this build has no native window")
