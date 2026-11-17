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
make lint          # go vet, and gofmt must list no file
```

The Playwright smoke test runs against a built binary, `bin/dsc-config` (`bin/dsc-config.exe` on
Windows) unless `DSC_CONFIG_BIN` names another one. It starts `dsc-config --no-browser` with the kubeconfig in `ui/e2e/fixtures/`:

```shell
make build
cd ui && npx playwright install chromium && npm run e2e
```

CI (`.github/workflows/config-tool.yaml`) runs the lint, tests and build, `make cross` and the
smoke test on every change under `config-tool/`, and compiles the window build without blocking.

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

Open `http://127.0.0.1:5173`. The proxy adds the run token only to requests from the dev server's
own origin: it applies the server's Host, Origin and `Sec-Fetch-Site` rules first and answers `403`
to anything else, so other pages in the browser cannot use it. The server itself has no relaxed
dev mode. The proxy authenticates nothing itself: any local process can send it any
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
  `Origin`, and API calls whose `Sec-Fetch-Site` is not `same-origin` are refused.
- The token never goes on a command line. The browser is started with a single-use launch code
  that expires after 60 seconds. On a shared host another account could race for that code, so
  use `--no-browser` there and paste the printed URL into the browser.
- The Target stage reads kubeconfig files (`KUBECONFIG`, else `~/.kube/config`, or the path
  typed in) for context names only. It never connects to a cluster, never returns credentials
  or server URLs, and never stores the kubeconfig path; only the context name joins the answers.
  Every file read, typed or from `KUBECONFIG`, must be a regular file of at most 8 MiB, so a
  device or FIFO (e.g. `KUBECONFIG=<(...)`) is refused unread. A `KUBECONFIG` entry that does
  not exist is skipped; any other failure (permission denied, not a directory) is reported.
  When no default file exists, S1 says so and asks for a path.
  A relative typed path is resolved against the directory `dsc-config` was started in, and S1
  shows the absolute file it read.

## Package layout

```
cmd/dsc-config/     the binary: GUI with no arguments, CLI subcommands otherwise
internal/server     front end: loopback HTTP API, WebSocket and the embedded UI
internal/cli        front end: cobra commands
internal/launcher   front end: browser tab, or the window under the wails tag
internal/version    core: build identity stamped through -ldflags
internal/model      core: question-model bundles per chart version
internal/profile    core: the answer set
internal/cluster    core: kube context names from kubeconfig files, read-only
internal/render     core: profile to values tree
internal/roundtrip  core: parse / classify / merge / write of a values.yaml, byte-preserving
internal/validate   core: values against the chart's schema
model/<version>/    question-model bundles, embedded
ui/                 React/TypeScript SPA, built into ui/dist and embedded
```

The core never imports `net/http` or a front-end package; `internal/layering_test.go` fails `go test`
when one does (directly; client-go pulls it in transitively).

## Round-trip

`internal/roundtrip` loads a `values.yaml` and writes it back byte for byte: comments, blank
lines, quoting, anchors, indentation, a BOM, CRLF and a missing final newline all stay. Edits are
byte patches over the source, and `Write` re-parses its own output and returns
`ErrPatchVerification` instead of a wrong file ([ADR 0013](../doc/adr/0013-config-tool-roundtrip-byte-patches.md)).
The only generated part is the header, `# Generated by dsc-config <version> for chart <version>
from <profile> — profile hash <hash>` plus one fixed line; a loaded header is kept as it is
until a new one is set. `Merge` deep-merges overrides; a map over a key inherited through `<<`
writes an explicit key holding the merged map. Refused: a stream with more than one document
(`ErrMultiDocument`), a path through an alias or onto a `<<` value (`ErrAliasPath`), and a value
whose text span cannot be determined safely (`ErrUnsupportedScalar`). A duplicated key resolves
to its last occurrence, as in Helm; the earlier ones stay untouched. The corpus test round-trips
every `k3s/*.yaml` and both charts' `values.yaml`, and edits each of their scalar leaves on its own.

## Chart versions

Each supported chart version has a question-model bundle in `model/<version>/bundle.yaml`,
embedded into the binary. Its directory name and `version` must be strict `MAJOR.MINOR.PATCH`
semver, checked and ordered with Helm's own library (`Masterminds/semver`). `GET /api/model`
groups the versions by the bundle's `chart`, and the Target stage offers only the selected chart's.
Per chart, the highest release is the default; a pre-release becomes the default only when that
chart has no release bundled. The layout holds one bundle per version, so two charts cannot share
a version number yet.

A bundle's `recognize:` list maps chart keys back to answers when an existing `values.yaml` is
opened (ADR 0014). Each rule has an exact `path`, an `answer` id, a `stage`, a `kind` (`toggle`
for a `!!bool`, `value` for any scalar) and, on `value` rules only, an optional `pattern` with
one capture group. A bundle with an invalid or duplicate-path rule fails to load, and a test
checks every shipped rule path against the chart's `values.yaml`.
