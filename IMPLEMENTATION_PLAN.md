# Implementation Plan: [config-tool] CT-1 Application shell, start screen and target stage

## Overview
Create the `config-tool/` Go module (`github.com/FIWARE/data-space-connector/config-tool`) with
the layout from `doc/config-tool/TECH_STACK.md`: a core library with no HTTP in it, a loopback
HTTP API with a per-run token and a WebSocket, a Vite/React/TypeScript SPA embedded with
`embed.FS`, the window chrome, screens S0 (Start) and S1 (Target), a cobra CLI over the same core,
and a CI workflow for the tool, with path filters so a tool-only PR skips `helm lint` and the k3s
tests. Nothing reads or writes a `values.yaml`, and nothing writes to a cluster.

Design sources: `doc/config-tool/TECH_STACK.md`, `USER_JOURNEY.md` (Stage 0/1), `MOCKUPS.md`
(*Window chrome*, *S0*, *S1*), `mockups/prototype.html` (take layout and wording from it, not its
code) and `doc/adr/0009-config-tool-stack.md`.

Cross-cutting decisions for every step:
- **Toolchain:** Go is the current stable release, pinned by the `go` directive in
  `config-tool/go.mod`; CI uses `go-version-file`. Node is 22 LTS with npm and a committed
  `package-lock.json`.
- **Layering:** the core is `internal/{version,model,profile,cluster,render,validate}`. It never
  imports `net/http` or `internal/server`, and a Go test enforces this (step 1). Do not create
  empty packages for later tickets (`roundtrip`, `chart`, `secrets`, `export`). They arrive with
  CT-2 and later tickets.
- **Libraries are the ones TECH_STACK names:** cobra, client-go (`tools/clientcmd`, read-only),
  wails/v2, React 18, TypeScript and Vite. TECH_STACK names no WebSocket library, so use
  `github.com/coder/websocket`. Vitest and React Testing Library are the UI unit-test runner, next
  to the Playwright layer TECH_STACK asks for. Neither choice changes the architecture.
- **No magic constants:** stage IDs, the token length, cookie and header names, the DNS-1123
  limits (namespace 63, Helm release name 53) and the default chart name are named constants in Go
  and TS.
- **Doc comments:** one-line GoDoc on every exported identifier and package, and a one-line JSDoc
  on every exported component, hook and store function.
- **No chart change:** no `Chart.yaml` bump and no `values.yaml` or schema regeneration.

## Steps

### Step 1: Go module skeleton, core packages and the CLI front end
Create the module and the CLI, which proves that the core is shared.

Files:
- `config-tool/go.mod` / `go.sum`: module `github.com/FIWARE/data-space-connector/config-tool`.
- `config-tool/cmd/dsc-config/main.go`: calls `internal/cli`.
- `config-tool/internal/cli/`: cobra root command `dsc-config`.
  - `--version` prints `dsc-config <version> (<commit>)`.
  - With no subcommand, the root command calls a `RunGUI func(ctx) error` hook. In this step it
    is a stub returning "GUI not available yet"; step 2 replaces it.
  - The `generate` subcommand takes the flags from TECH_STACK: `-p/--profile` and `-o/--output`.
    `generate --help` documents them. Running it calls `render.Render(...)`, which returns
    `render.ErrNotImplemented`, so the command exits non-zero with a clear message.
- `config-tool/internal/version/`: `Version` and `Commit` vars, set through
  `-ldflags "-X …/internal/version.Version=…"` with `dev` defaults.
- `config-tool/internal/render/` and `config-tool/internal/validate/`: exported `ErrNotImplemented`
  and stub entry points (`Render`, `Validate`). The CLI calls them here, and the HTTP stubs call
  them in step 2.
- `config-tool/internal/profile/`: the answer-set types needed now.
  - `Target{Type TargetType; Kubeconfig, Context, Namespace string; CreateNamespace bool;
    ReleaseName, Chart, ChartVersion string; Environment Environment}`.
  - Enum constants `TargetFile`/`TargetCluster` and `EnvDevelopment`/`EnvProduction`.
  - `(Target) Validate() []FieldError`: DNS-1123 label checks, namespace ≤ 63, release ≤ 53, and
    the context required only when `Type == TargetCluster`.
- `config-tool/internal/layering_test.go` (or one test per core package): uses `go/parser` to
  assert that no core package directly imports `net/http`, `.../internal/server` or
  `.../internal/cli`. Check direct imports only, because client-go pulls `net/http` in
  transitively.
- `config-tool/Makefile`: targets `build`, `test`, `lint` (`go vet` + `gofmt -l`), with the
  version ldflags.
- `.gitignore` entries: `config-tool/bin/`.

Tests (table-driven): CLI `--version` output; `generate --help` lists `--profile` and `--output`;
`generate` returns `ErrNotImplemented`; `Target.Validate` cases (valid, uppercase, too long,
leading dash, missing context for the cluster target, no context needed for the file target).

Acceptance: `cd config-tool && go build ./... && go test ./...` passes, `dsc-config --version` and
`dsc-config generate --help` work, and the layering test fails if a core file imports `net/http`.

### Step 2: Loopback HTTP API, token, WebSocket, stub endpoints and browser launcher
Add `config-tool/internal/server/` (the only package that knows HTTP) and
`config-tool/internal/launcher/`, and wire them into the no-argument path of `dsc-config`.

- **Listener:** `net.Listen("tcp", "127.0.0.1:0")`. The address is never configurable to a
  non-loopback host. `Server.URL()` returns `http://127.0.0.1:<port>/?token=<t>`.
- **Token:** 32 bytes from `crypto/rand`, base64url, generated once per run. The token in the URL
  is single-use. The first `GET /?token=` that matches (constant-time compare) sets an `HttpOnly`,
  `SameSite=Strict`, `Path=/` session cookie holding a separate random session secret, then
  redirects to `/` to strip the token from the address bar. After that, every request (static
  assets, `/api/*`, the WebSocket) must carry the cookie. `Authorization: Bearer <session>` is also
  accepted for non-browser clients and tests. Without it the response is `401`, except for the bare
  `/` without a cookie, which gets a short "open the link printed by dsc-config" page with status
  401.
- **DNS-rebinding guard:** reject a `Host` header other than `127.0.0.1:<port>` or
  `localhost:<port>` with `403`. On the WebSocket upgrade, also require a same-origin `Origin`.
- **Routes** (stdlib `http.ServeMux` with method patterns, JSON errors `{ "error": "...",
  "ticket": "CT-n" }`):
  - `GET /api/model`: stub `501` for now; step 4 fills it.
  - `GET/PUT /api/profile`: `501` (CT-4).
  - `POST /api/render`: calls `render.Render`, maps `ErrNotImplemented` to `501`.
  - `POST /api/validate`: calls `validate.Validate`, same mapping.
  - `POST /api/load`: `501` (CT-2/CT-3).
  - `GET /api/ws`: WebSocket via `github.com/coder/websocket`. It sends one
    `{"type":"ready","version":…}` event, then keeps the connection open for future progress
    events. Provide a small `Progress` broadcaster type that is unused until later tickets.
  - `/`: the static SPA from an `fs.FS` passed in, with a fallback to `index.html` for client-side
    routes.
- **Embedding:** `config-tool/ui/embed.go` (package `ui`) with `//go:embed all:dist` and a
  committed `config-tool/ui/dist/.gitkeep`, so `go build` works before the UI is built. When
  `index.html` is missing, the server serves a "UI not built — run `make ui`" page. `.gitignore`
  ignores `config-tool/ui/dist/*` and keeps `!config-tool/ui/dist/.gitkeep`.
- **Launcher (default build, no build tag):** `launcher.Open(url)` opens the default browser with
  `xdg-open`, `open` or `rundll32 url.dll,FileProtocolHandler` per `runtime.GOOS`, with no extra
  dependency. `dsc-config` with no args starts the server, prints the URL to stdout, opens it and
  serves until SIGINT/SIGTERM, then shuts down gracefully. A `--no-browser` flag only prints the
  URL; Playwright and headless hosts use it.

Tests (table-driven `httptest` against the real handler and a real loopback listener):
- the listener address is `127.0.0.1`;
- each endpoint without credentials returns `401`;
- a wrong token returns `401`;
- the URL token exchange sets the cookie and works only once;
- with the cookie, the stubs return their documented codes;
- a foreign `Host` returns `403`;
- the WebSocket upgrade without a cookie fails, and with a cookie it receives `ready`;
- the missing-UI fallback page is served.

Acceptance: `dsc-config` with no args prints a `127.0.0.1` URL and opens a tab. `curl` without
the token gets `401`.

### Step 3: UI scaffold, window chrome, stage rail, footer and placeholders; Wails behind a build tag
Create the SPA under `config-tool/ui/` and the shell every later stage hangs in.

- **Project:** Vite + React 18 + TypeScript (`strict`), ESLint, and Vitest + React Testing Library
  (jsdom). The npm scripts are `dev`, `build` (output to `dist/`), `test`, `lint` and `typecheck`.
  The Vite dev server proxies `/api` to a running `dsc-config --no-browser`, for development only.
  No CDN, web fonts or telemetry: everything is bundled (TECH_STACK requirement 7).
- **`src/api/client.ts`:** same-origin `fetch` wrapper (the cookie travels by itself), typed
  errors for `401` and `501`, and `connectProgress()` for `/api/ws`.
- **`src/state/`:** a single store, either a React context with `useReducer` or a small hand-rolled
  store (no new state library). It holds the current stage and the answer set (`target` only for
  now), lives for the whole app session and is independent of which stage component is mounted.
- **`src/stages.ts`:** the rail definition, in this order: `1 Target`, `2 Role`, `3 Foundations`,
  `4 Infrastructure`, `5 Features`, `6 Configuration`, `7 Review`, `8 Install`. Each entry has an
  id constant, a label and a component. Only Target has one in CT-1.
- **Chrome** (MOCKUPS *Window chrome*):
  - title bar: `DSC Configurator` plus `<namespace> · <role> · chart <version>`, built from what is
    known;
  - left rail: jumpable at any time, with per-stage status glyphs ✓ ● ○ ⚠ ✖ and their legend
    semantics, and the disabled entries `⌘E Expert: YAML` and `⌘S Save profile` below a divider;
  - content pane;
  - footer: `⚠ N warnings ✖ N errors ○ N open questions` with `[ Back ]` and
    `[ Next: <stage> ]`.

  The counters are computed by a pure function from the store, through per-stage selectors that
  return `{warnings, errors, open}`. Stages without a selector count zero.
- **`Placeholder` stage component:** "<Stage> — not available yet", naming the epic or ticket that
  brings it. Any unimplemented stage renders this, never an error boundary.
- **App routing:** state-driven, not URL-driven. S0 is shown until an entry is chosen, and then the
  chrome is shown.
- **Wails** (ADR required, see below):
  - `config-tool/internal/launcher/window_wails.go` is behind `//go:build wails`, with
    `window_browser.go` as its `!wails` counterpart.
  - In the Wails build, the window loads a tiny bootstrap page that navigates the webview to the
    loopback URL. The window then talks to the same server, so the cookie, the token and the
    WebSocket behave exactly as in the browser.
  - If the window cannot start (no webview runtime), it falls back to `launcher.Open`.
  - The default build stays pure Go (`CGO_ENABLED=0`). That is how one job cross-compiles all five
    targets (Wails needs CGO and the native webview SDKs). A native build is
    `go build -tags wails` on macOS/Windows, or `-tags wails,webkit2_41` on Linux with
    `libwebkit2gtk-4.1-dev`.
- **ADR:** add `doc/adr/0011-config-tool-window-build-tag.md`. It records that release binaries
  from the single cross-compile job are pure Go and open a browser tab, that the Wails window is an
  opt-in `wails` build tag built natively per OS, and why: CGO plus the webview SDKs versus "one CI
  job". This follows TECH_STACK's "dropping it leaves a working tool". Add the row to
  `doc/adr/README.md` and its "belong to `../config-tool/`" sentence.
- **Makefile:** `ui` (`npm ci && npm run build` in `ui/`). `build` depends on `ui`. Add
  `build-window` (`-tags wails`).

Tests (Vitest, parameterized with `it.each`):
- the rail lists the 8 stages in order;
- clicking each stage selects it and renders either Target or the placeholder, with no thrown
  error;
- the footer counters reflect a mocked selector result;
- Back/Next move through the rail order and the Next label names the next stage.

Go: the server test from step 2 still passes with a built `dist/`.

### Step 4: S0 Start and S1 Target screens, the chart-version model and the kubeconfig contexts
Implement the two stages and the two endpoints they need.

- **Model bundle:** `config-tool/model/10.16.0/bundle.yaml` holds only `chart:
  data-space-connector` and `version: 10.16.0` (the chart's current version). Later epics extend
  the file. `config-tool/model/embed.go` (package `model`, `//go:embed */bundle.yaml`).
  `internal/model` provides `Versions(fs.FS) ([]Bundle, error)`, sorted by semver descending,
  with the first marked `latest`, and `Charts()`. It is core and has no HTTP.
- **`GET /api/model`:** returns `{ charts: ["data-space-connector"], versions: [{version,
  latest}] }`. The full question model arrives with Epics 2, 3 and 5. Remove the `501` stub for
  this route.
- **`internal/cluster`:** `Contexts(kubeconfigPath string) (path string, contexts []string,
  current string, err error)`. It uses `clientcmd.NewDefaultClientConfigLoadingRules()`, honours
  `KUBECONFIG`, and uses an explicit path when one is given. It reads the file and nothing else: it
  never connects to the cluster and never writes the file. A missing file returns an empty list,
  not an error.
- **`GET /api/cluster/contexts?kubeconfig=<path>`:** in `internal/server`, returns the path,
  context names and the current context. It returns no credentials and no server URLs.
- **S0 Start** (MOCKUPS *S0*, wording from the prototype):
  - heading and subtitle;
  - four cards: *New configuration*, *Open values.yaml*, *Open profile* and *Try it locally*. Only
    *New configuration* is enabled, and it resets the answer set and lands on Target. The other
    three are rendered disabled (`aria-disabled`) with a "coming in CT-2/CT-3", "CT-4" or "CT-5"
    hint;
  - Documentation ↗ and Release notes ↗ links;
  - no *Recent* list (profiles are CT-4).
- **S1 Target** (MOCKUPS *S1*, wording from the prototype):
  - **Target radio:** *File only* / *Kubernetes cluster*.
  - **Kubeconfig path and Context dropdown:** rendered **only** when the cluster target is chosen.
    The path is a text field defaulting to the server-reported path; there is no Browse button in
    the browser build. The dropdown is filled from `/api/cluster/contexts` and preselects the
    current context. Show the "The cluster is only read… Nothing is installed before step 8" note.
    There is no connection or version probe (stage 4 detection is a later epic).
  - **Namespace** with *create it*. The checkbox is shown only for the cluster target.
  - **Release name**, with the ⓘ "Both appear in generated service URLs…" note.
  - **Chart dropdown** (from `/api/model`).
  - **Version dropdown**, defaulting to the `latest` entry, with a Release notes ↗ link.
  - **Environment radio:** *Development* / *Production*, with the "Only changes defaults" note.

  No field has a hidden default. An unanswered required field counts as an open question, and a
  DNS-1123 violation counts as an error. The TS validation mirrors `profile.Target.Validate` and
  uses the same named limits.
- **State:** every field writes to the store from step 3, so values survive switching to any rail
  stage and back. Kubeconfig and context are kept in the store when the user switches to *File
  only*, but they are hidden and do not count. `profile.Target` (Go) and `Target` (TS) carry the
  same field names, so CT-4 can PUT the store as-is.

Tests:
- Go table tests: `model.Versions` (ordering, `latest`); `cluster.Contexts` against fixture
  kubeconfigs in `testdata/` (two contexts, empty file, missing file, `KUBECONFIG` with two
  paths); `/api/model` and `/api/cluster/contexts` behind auth.
- Vitest, parameterized:
  - S0 shows four entries, exactly one enabled, and *New configuration* reaches S1;
  - **no unneeded question:** with *File only*, the Kubeconfig, Context and *create it* controls
    are absent from the DOM; with *Kubernetes cluster*, they are present;
  - every S1 field keeps its value after switching to each other stage and back;
  - the footer counts open questions and errors for empty, invalid and complete inputs.

### Step 5: CI workflow, path filters, Playwright smoke and documentation
Make CI build and test the tool, keep the chart pipelines out of tool-only PRs, and document it.

- **`.github/workflows/config-tool.yaml`:** `on: pull_request` and `push` to `main`, with `paths:
  ['config-tool/**', '.github/workflows/config-tool.yaml']`. Jobs:
  1. `test`: `actions/setup-go` (`go-version-file: config-tool/go.mod`) and `actions/setup-node`
     (22, npm cache). Runs `npm ci`, `lint`, `typecheck`, `test` and `build` in `config-tool/ui`,
     then `gofmt -l` (must be empty), `go vet ./...` and `go test ./...`.
  2. `build` (needs `test`): **one job** that builds the UI once, then loops over
     `linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64` with `CGO_ENABLED=0` and
     the version ldflags. It uploads the five binaries with `actions/upload-artifact`.
  3. `e2e` (needs `build`): runs Playwright (Chromium) against the linux/amd64 binary.
  4. `window` (optional, non-blocking, `continue-on-error: true`): installs
     `libgtk-3-dev libwebkit2gtk-4.1-dev` and runs `go build -tags wails,webkit2_41
     ./cmd/dsc-config`, so the build-tagged code does not rot.
- **Playwright smoke** (`config-tool/ui/e2e/`, `playwright.config.ts`): a global setup starts
  `dsc-config --no-browser` and parses the printed URL. Tests:
  - a request without the token returns `401`;
  - the token URL loads S0;
  - *New configuration* reaches S1;
  - fill all fields (cluster target with a fixture `KUBECONFIG` from `e2e/fixtures/`);
  - jump to a placeholder stage and see the placeholder;
  - return to Target with all values intact.
- **Path filters:**
  - `test.yaml`: replace `on: [push, pull_request]` with `push` and `pull_request`, each with
    `paths-ignore: ['config-tool/**', '.github/workflows/config-tool.yaml']`. A mixed PR still
    runs both, because `paths-ignore` only skips when every changed file matches.
  - `check.yaml`: add a `changes` job (`dorny/paths-filter@v3`, filter `chart: ['charts/**',
    '.github/scripts/values-schema.sh', ...]`, or invert it as "anything outside `config-tool/`")
    and gate `chart-lint` and `values-schema` with `if: needs.changes.outputs.chart == 'true'`.
    `chart-version` already `needs: chart-lint` and skips with it. The `links` job keeps running.
    Add a one-line "why" comment above the filter.
- **`lychee.toml`:** add `"(^|/)node_modules/"` to `exclude_path`, so local link checks skip
  `config-tool/ui/node_modules`.
- **`config-tool/README.md`:**
  - prerequisites (Go version, Node 22);
  - `make ui`, `make build`, `make test`, `make cross` and `make build-window`;
  - running it: no args opens a tab, and `--no-browser`;
  - the security model in three lines: loopback, per-run token, read-only kubeconfig;
  - the package layout, and that the core never imports `net/http`;
  - the Wails build tag, with a link to ADR 0011;
  - links to `doc/config-tool/`.
- **`doc/README.md`:** add "Configuration tool — building and running" pointing to
  `../config-tool/README.md`, under the existing *Configuration tool — design* entry.
- **`AGENTS.md`:** confirm the `config-tool` Build & Test commands match the Makefile.

Acceptance:
- a PR touching only `config-tool/` runs `config-tool.yaml` (test, build with five binaries, e2e)
  and neither `test.yaml` nor `chart-lint`;
- a PR touching only `charts/` does not run `config-tool.yaml`;
- `.github/scripts/check-links.sh` passes.
