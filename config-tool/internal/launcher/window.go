package launcher

import "errors"

// WindowTitle is the native window's title, matching APP_TITLE in ui/src/state/title.ts.
const WindowTitle = "DSC Configurator"

// ErrNoWindow reports a build without the wails tag; the caller opens a browser tab instead.
var ErrNoWindow = errors.New("this build has no native window")
