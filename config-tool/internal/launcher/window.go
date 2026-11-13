package launcher

import "errors"

// WindowTitle is the native window's title; window_test.go pins it to ui/index.html, title.test.ts pins APP_TITLE to the same.
const WindowTitle = "DSC Configurator"

// ErrNoWindow reports a build without the wails tag; the caller opens a browser tab instead.
var ErrNoWindow = errors.New("this build has no native window")
