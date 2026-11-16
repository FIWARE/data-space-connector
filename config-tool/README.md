# dsc-config

Guided configuration of the FIWARE Data Space Connector chart. Design:
[`doc/config-tool/`](../doc/config-tool/).

## Building

Needs Go (version from `go.mod`) and Node 22 with npm.

```shell
make ui            # npm ci + vite build into ui/dist, which the binary embeds
make build         # ui, then bin/dsc-config[.exe] (pure Go, opens a browser tab)
make cross         # ui, then bin/dsc-config-<os>-<arch> for the five release targets
make build-window  # ui, then bin/dsc-config-window[.exe] with a native window (see below)
make test          # go test ./... and the UI's Vitest suite
make lint
```

Release binaries are pure Go and open a browser tab. The native window is the opt-in `wails`
build tag ([ADR 0011](../doc/adr/0011-config-tool-window-build-tag.md)); build it on the target
OS with its webview SDK (`libwebkit2gtk-4.1-dev` on Linux) and, except on Windows, CGO. When Wails
reports a failed start (an error, or a panic such as no display), the binary says so on stderr
and opens a browser tab instead; some Wails start-up failures exit the process on their own. The
window has been run on Linux only so far.

## Developing the UI

The Vite dev server proxies `/api` to a running `dsc-config` and adds the run token itself:

```shell
dsc-config --no-browser                    # prints http://127.0.0.1:<port>/?token=...
cd ui && DSC_CONFIG_URL='<printed URL>' npm run dev
```

Open `http://127.0.0.1:5173`. The proxy applies the server's Host, Origin and `Sec-Fetch-Site` rules
before it adds the token, so other pages in the browser cannot use it; the server itself has no
relaxed dev mode. The proxy authenticates nothing itself: any local process can send it any
`Host` or `Origin`, so do not run `npm run dev` on a shared host. `npm test`, `npm run lint` and `npm run typecheck` run in `ui/`.

## Running

`dsc-config` with no arguments starts a local server, prints its URL and opens it in the
default browser. Ctrl+C stops it. If no browser opens, or the wrong one does, open the
printed URL yourself. `dsc-config --no-browser` only prints the URL.

## Security model

- The server listens on `127.0.0.1` with a random port and answers only for the hosts
  `127.0.0.1:<port>` and `localhost:<port>`.
- The printed URL holds a per-run token that stays valid until the process exits. Do not share
  it. Opening it sets a session cookie; state-changing requests also need the server's own
  `Origin`.
- The token never goes on a command line. The browser is started with a single-use launch code
  that expires after 60 seconds. On a shared host another account could race for that code, so
  use `--no-browser` there and paste the printed URL into the browser.
- The Target stage reads kubeconfig files (`KUBECONFIG`, else `~/.kube/config`, or the path
  typed in) for context names only. It never connects to a cluster, never returns credentials
  or server URLs, and never stores the kubeconfig path; only the context name joins the answers.
  A typed path must be a regular file of at most 8 MiB, so a device or FIFO is refused unread.

## Chart versions

Each supported chart version has a question-model bundle in `model/<version>/bundle.yaml`,
embedded into the binary. The highest release is offered as the default on the Target stage; a
pre-release becomes the default only when no release is bundled.
