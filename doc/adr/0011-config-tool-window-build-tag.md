# 0011 - Release binaries open a browser tab; the native window is an opt-in build tag

**Status:** Proposed
**Date:** 2026-11-13
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/TECH_STACK.md`](../config-tool/TECH_STACK.md)

## Context

[ADR 0009](0009-config-tool-stack.md) and TECH_STACK show the UI in a native window (Wails) with
the browser as fallback, and also ask for statically linked binaries for five targets from one
CI job. Both cannot hold at once for macOS and Linux: there Wails v2 needs CGO and the OS's
webview SDK (WebKit on macOS, WebKitGTK on Linux), so the binary is built natively on its own OS,
not cross-compiled from one Linux job with `CGO_ENABLED=0`. Windows is the exception: Wails uses
the pure-Go `go-webview2` there, and `CGO_ENABLED=0 GOOS=windows` builds the window from Linux.

## Decision

- **Release binaries are pure Go.** The single cross-compile job builds
  `linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64` with `CGO_ENABLED=0`. They
  serve the UI on `127.0.0.1` and open it in the default browser.
- **Windows gets a tab too, by choice.** It could ship the window from the same job. All five
  binaries behave the same instead, so the one launch path the Playwright smoke test covers is
  the one every user gets, and the window is not shipped on an OS where nobody has run it.
- **The window is the opt-in `wails` build tag**, built with `make build-window` on the target
  OS (CGO on macOS and Linux, none on Windows). `internal/launcher/window_wails.go` carries it; `window_browser.go` is the
  `!wails` counterpart and reports that there is no window. The full tag set is
  `wails,desktop,production`, plus `webkit2_41` on Linux (`libwebkit2gtk-4.1-dev`):
  `desktop,production` are Wails' own tags, without which `wails.Run` only returns an error.
- **The window is a view on the same server.** It loads a tiny bootstrap page from Wails' own
  asset server that meta-refreshes the webview to a single-use launch URL on the loopback
  server. That navigation is cross-site, so a `SameSite=Strict` cookie set by a redirect would
  not stick; the server's launch exchange answers `200` with a meta refresh for exactly that
  reason. From then on the window uses the same cookie, token, Origin checks and WebSocket as a
  browser tab. No API is exposed through Wails bindings.
- **A window that reports its failure falls back to a tab.** When `wails.Run` returns an error
  (missing tags) or panics (GTK cannot start, e.g. no display), the error goes to stderr and a
  browser tab opens. Wails handles some start-up failures itself with `log.Fatal` or `os.Exit`
  (WebView2 creation on Windows, parts of the Linux frontend). Those end the process with no
  fallback; the user then runs a release binary or `--no-browser`.
- **Verification so far.** The window path was run on Linux (WebKitGTK 4.1 under Xvfb): the
  webview followed the bootstrap meta refresh to the loopback server, completed the launch
  exchange and showed S0, and SIGINT closed the window. It has not been run on macOS or
  Windows yet; the webview there may treat the cross-origin top-level navigation differently.

## Consequences

- The release keeps TECH_STACK's "one CI job, statically linked, no runtime".
- Users of release binaries get a browser tab, not a window. This is the fallback TECH_STACK
  already describes ("dropping it leaves a working tool that opens a browser tab").
- Native file dialogs, which only the window offers, cannot be relied on: every path is a text
  field in the UI.
- Shipping the window later means native build jobs on `ubuntu` and `macos` runners with the
  tags above; the Windows window can stay in the cross-compile job. The code needs no change,
  but each OS has to be run once before its window ships.

## Alternatives

- **Window in the released binaries.** One native job per OS, CGO, the webview SDKs on each
  runner, and on Linux a runtime dependency on WebKitGTK. Rejected for CT-1 because it gives up
  the single job and the static binary for a cosmetic gain; it stays open as described above.
- **Drop Wails entirely.** Simpler, but loses the path to native dialogs that TECH_STACK names.
  The build tag keeps it at the cost of one small file.
