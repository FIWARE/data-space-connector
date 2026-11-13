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
- **Layering:** the front ends are `internal/server`, `internal/cli` and `internal/launcher`.
  Every other package under `internal/` is core: in CT-1 that is
  `internal/{version,model,profile,cluster,render,validate}`. The core never imports `net/http`
  or a front-end package, and a Go test enforces this (step 1) with a deny-list of the three
  front ends, so core packages added later are covered without editing the test. Do not create
  empty packages for later tickets (`roundtrip`, `chart`, `secrets`, `export`). They arrive with
  CT-2 and later tickets.
- **Profile contents:** only the kube *context name* goes into the answer set
  (`profile.Target`). The kubeconfig path is session state of the running tool and is never
  stored (TECH_STACK *Security*).
- **Libraries are the ones TECH_STACK names:** cobra, client-go (`tools/clientcmd`, read-only),
  wails/v2, React 18, TypeScript and Vite. TECH_STACK names no WebSocket library, so use
  `github.com/coder/websocket`. Vitest and React Testing Library are the UI unit-test runner, next
  to the Playwright layer TECH_STACK asks for. Neither choice changes the architecture.
- **No magic constants:** stage IDs, the token length, cookie and header names, the DNS-1123
  limits (namespace 63, Helm release name 53) and the default chart name are named constants in Go
  and TS.
- **Doc comments:** one-line GoDoc on every exported identifier and package, and a one-line JSDoc
  on every exported component, hook and store function.
- **No chart change:** no file under `charts/` changes, not even a comment, because
  `chart-version` bumps and publishes every chart a pull request touches. The first comment of
  `values.yaml` still says `see CLAUDE.md`; fix it to `AGENTS.md` with the next chart change, not
  here.
- **Window vs. browser tab (deviation from the ticket wording, needs confirmation before
  step 3):** the ticket and TECH_STACK name the Wails window as primary and the browser tab as
  fallback. TECH_STACK also asks for statically linked binaries from one CI job, and Wails needs
  CGO and each OS's webview SDK, so both cannot hold. This plan keeps the one-job, pure-Go release
  binaries, which open a browser tab, and makes the window an opt-in `wails` build tag (step 3,
  ADR 0011). If the reviewer wants the window in the released binaries instead, step 3 replaces
  the single cross-compile job in step 5 with one native build job per OS (`ubuntu`, `macos`,
  `windows` runners) using `-tags wails`, and ADR 0011 records that instead.

## Steps

### Step 1: Go module skeleton, core packages and the CLI front end
Create the module and the CLI, which proves that the core is shared.

Files:
- `config-tool/go.mod` / `go.sum`: module `github.com/FIWARE/data-space-connector/config-tool`,
  with `ignore ./ui/node_modules` (go.mod `ignore` directive, Go 1.25+). The npm project sits
  inside the module and some npm packages ship `.go` files, so `go vet ./...` and
  `go test ./...` must not walk them.
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
  - `Target{Type TargetType; Context, Namespace string; CreateNamespace bool;
    ReleaseName, Chart, ChartVersion string; Environment Environment}`. There is deliberately no
    kubeconfig field: the path stays out of the profile (see *Profile contents*).
  - Enum constants `TargetFile`/`TargetCluster` and `EnvDevelopment`/`EnvProduction`.
  - `(Target) Validate() []FieldError`: DNS-1123 label checks, namespace ≤ 63, release ≤ 53, and
    the context required only when `Type == TargetCluster`.
- `config-tool/internal/layering_test.go`: walks every package directory under `internal/` except
  the front ends `server`, `cli` and `launcher`, and uses `go/parser` to assert that none of them
  directly imports `net/http`, `.../internal/server`, `.../internal/cli` or
  `.../internal/launcher`. Check direct imports only, because client-go pulls `net/http` in
  transitively.
- `config-tool/Makefile`: targets `build`, `test`, `lint` (`go vet` +
  `gofmt -l $(go list -f '{{.Dir}}' ./...)`, never `gofmt -l .`, which would enter
  `node_modules`), with the version ldflags.
- `.gitignore` entries: `config-tool/bin/`.

Tests (table-driven): CLI `--version` output; `generate --help` lists `--profile` and `--output`;
`generate` returns `ErrNotImplemented`; `Target.Validate` cases (valid, uppercase, too long,
leading dash, missing context for the cluster target, no context needed for the file target);
`Target` marshalled to JSON and YAML has no key containing `kubeconfig`.

Acceptance: `cd config-tool && go build ./... && go test ./...` passes, `dsc-config --version` and
`dsc-config generate --help` work, and the layering test fails if a core file imports `net/http`,
including a file in a newly created core package.

### Step 2: Loopback HTTP API, token, WebSocket, stub endpoints and browser launcher
Add `config-tool/internal/server/` (the only package that knows HTTP) and
`config-tool/internal/launcher/`, and wire them into the no-argument path of `dsc-config`.

- **Listener:** `net.Listen("tcp", "127.0.0.1:0")`. The address is never configurable to a
  non-loopback host. `Server.URL()` returns `http://127.0.0.1:<port>/?token=<t>`.
- **Token:** 32 bytes from `crypto/rand`, base64url, generated once per run and valid for the
  whole run, so the printed URL still works when `xdg-open` picked the wrong browser or profile.
  Each `GET /?token=` that matches (constant-time compare) sets an `HttpOnly`, `SameSite=Strict`,
  `Path=/` session cookie holding a separate random session secret (the server keeps the set of
  issued secrets), then redirects to `/` to strip the token from the address bar. After that,
  every request (static assets, `/api/*`, the WebSocket) must carry the cookie.
  `Authorization: Bearer <token>` with the run token is also accepted for non-browser clients,
  tests and the Vite dev proxy (step 3). Without either, the response is `401`, except for the
  bare `/` without a cookie, which gets a short "open the link printed by dsc-config" page with
  status 401.
- **Cookie name carries the port:** cookies are scoped by host, not port, so the name is
  `dsc_session_<port>` (prefix as a named constant). Two instances running side by side (e.g.
  provider and consumer for CT-5) then do not overwrite each other's session, and an instance
  only ever reads its own cookie.
- **DNS-rebinding guard:** reject a `Host` header other than `127.0.0.1:<port>` or
  `localhost:<port>` with `403`.
- **Cross-origin guard (CSRF):** `SameSite=Strict` does not help here, because every other port on
  `127.0.0.1` is the same *site*, so a page from any local dev server could send a credentialed
  `POST`. Every cookie-authenticated request with an unsafe method (anything but `GET`/`HEAD`) and
  the WebSocket upgrade must carry an `Origin` equal to `http://127.0.0.1:<port>` or
  `http://localhost:<port>`; a missing or foreign `Origin` returns `403`. Bearer-authenticated
  requests skip this check, because a browser never attaches that header on its own. `GET`
  handlers have no side effects (the token exchange only sets the cookie, and needs the token).
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
- the URL token exchange sets the cookie and works repeatedly within one run;
- two servers on two listeners use different cookie names, and one server's cookie is `401` on
  the other;
- `Authorization: Bearer` with the run token is accepted, with a session secret or a wrong value
  it is `401`;
- with the cookie, the stubs return their documented codes;
- a foreign `Host` returns `403`;
- with the cookie, a `POST` with a foreign-port `Origin` (`http://127.0.0.1:<other port>`) or no
  `Origin` returns `403`, and one with the server's own `Origin` reaches the handler;
- the WebSocket upgrade without a cookie fails, and with a cookie it receives `ready`;
- the missing-UI fallback page is served.

Acceptance: `dsc-config` with no args prints a `127.0.0.1` URL and opens a tab. `curl` without
the token gets `401`.

### Step 3: UI scaffold, window chrome, stage rail, footer and placeholders; Wails behind a build tag
Create the SPA under `config-tool/ui/` and the shell every later stage hangs in.

- **Project:** Vite + React 18 + TypeScript (`strict`), ESLint, and Vitest + React Testing Library
  (jsdom). The npm scripts are `dev`, `build` (output to `dist/`), `test`, `lint` and `typecheck`.
  No CDN, web fonts or telemetry: everything is bundled (TECH_STACK requirement 7).
- **Dev proxy (development only, no server change):** `npm run dev` reads `DSC_CONFIG_URL`, the
  URL printed by a running `dsc-config --no-browser`, and takes the port and token from it. The
  Vite dev server binds `127.0.0.1` only. The Vite proxy forwards `/api` (with `ws: true`) to
  `http://127.0.0.1:<port>` with `changeOrigin: true`, so the `Host` is the server's own.
  Because the proxy adds the run token, it must apply the step-2 guards itself, otherwise any
  page in the developer's browser could use it as an authenticated, CSRF-free front for the API.
  Its `configure` hook calls a pure function `devProxyGuard(method, headers, devOrigin)` in
  `ui/dev-proxy.ts` on every proxied request, the WebSocket upgrade included, and answers `403`
  (and drops the WebSocket) when:
  - `Origin` is present and is not the dev server's own origin (`http://127.0.0.1:<vite port>`
    or `http://localhost:<vite port>`);
  - `Origin` is missing on an unsafe method (anything but `GET`/`HEAD`) or the upgrade;
  - `Sec-Fetch-Site` is present and is not `same-origin`.

  Only a request that passes gets `Authorization: Bearer <token>` added and, if it has an
  `Origin`, that header rewritten to `http://127.0.0.1:<port>`. The server's Host, Origin and
  auth guards stay exactly as in step 2; never add a dev mode that relaxes them. Without
  `DSC_CONFIG_URL`, `npm run dev` fails with a message saying how to get it.
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
  - If the reviewer rejected the build-tag approach (see *Window vs. browser tab* above), follow
    the alternative recorded there instead of this one.
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
- **Makefile:** `ui` (`npm ci && npm run build` in `ui/`). `build` depends on `ui`. `test` adds
  `npm test` in `ui/` after `go test ./...`. Add `build-window` (`-tags wails`) and `cross`
  (depends on `ui`): it loops over the five `GOOS/GOARCH` pairs `linux/amd64 linux/arm64
  darwin/amd64 darwin/arm64 windows/amd64` (a named Makefile variable) with `CGO_ENABLED=0` and
  the version ldflags, writing `bin/dsc-config-<os>-<arch>[.exe]`.

Tests (Vitest, parameterized with `it.each`):
- `devProxyGuard`: own origin passes (GET, POST, upgrade); foreign-port and remote `Origin`
  return `403`; a POST or upgrade without `Origin` returns `403`; a GET without `Origin` passes;
  `Sec-Fetch-Site: cross-site` and `same-site` return `403`;
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
  the file. `config-tool/model/embed.go` declares package `bundles` (`//go:embed */bundle.yaml`),
  so importers of both it and `internal/model` need no alias; its package doc says why the name
  differs from the directory.
  `internal/model` provides `Versions(fs.FS) ([]Bundle, error)`, sorted by semver descending,
  with the first marked `latest`, and `Charts()`. It is core and has no HTTP.
- **`GET /api/model`:** returns `{ charts: ["data-space-connector"], versions: [{version,
  latest}] }`. The full question model arrives with Epics 2, 3 and 5. Remove the `501` stub for
  this route.
- **`internal/cluster`:** `Contexts(explicitPath string) (Kubeconfig, error)` with
  `Kubeconfig{Paths []string; Contexts []string; Current string}`. An empty `explicitPath` uses
  `clientcmd.NewDefaultClientConfigLoadingRules()`: `KUBECONFIG` (possibly several files, merged
  with clientcmd's rules, so the first file that sets `current-context` wins) or
  `~/.kube/config`. A non-empty one reads only that file. `Paths` is the precedence list actually
  read. It reads the files and nothing else: it never connects to the cluster and never writes a
  file. A missing file returns an empty list, not an error.
- **`GET /api/cluster/contexts?kubeconfig=<path>`:** in `internal/server`, returns `paths`,
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
    The path is a text field; there is no Browse button in the browser build. When the server
    reports exactly one path, the field is pre-filled with it. With several (`KUBECONFIG` with
    more than one file), the field stays empty, an empty field means "default loading rules", and
    a note lists the files read; the dropdown then shows the merged contexts. The dropdown is filled from `/api/cluster/contexts` and preselects the
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
  only*, but they are hidden and do not count. The kubeconfig path lives in a separate `session`
  slice of the store, outside the answer set. `profile.Target` (Go) and the answer set's `Target`
  (TS) carry the same field names, neither has a kubeconfig field, so CT-4 can PUT the answer set
  as-is without storing the path.

Tests:
- Go table tests: `model.Versions` (ordering, `latest`); `cluster.Contexts` against fixture
  kubeconfigs in `testdata/` (two contexts, empty file, missing file, and `KUBECONFIG` with two
  paths: `Paths` lists both in order, contexts are the union, `Current` comes from the first file
  that sets it); `/api/model` and `/api/cluster/contexts` behind auth.
- Vitest, parameterized:
  - S0 shows four entries, exactly one enabled, and *New configuration* reaches S1;
  - the kubeconfig field is pre-filled for one reported path and empty, with the file list shown,
    for two;
  - **no unneeded question:** with *File only*, the Kubeconfig, Context and *create it* controls
    are absent from the DOM; with *Kubernetes cluster*, they are present;
  - every S1 field keeps its value after switching to each other stage and back;
  - the serialised answer set has no `kubeconfig` key while the session still holds the path;
  - the footer counts open questions and errors for empty, invalid and complete inputs.

### Step 5: CI workflow, path filters, Playwright smoke and documentation
Make CI build and test the tool, keep the chart pipelines out of tool-only PRs, and document it.

- **`.github/workflows/config-tool.yaml`:** `on: pull_request` and `push` to `main`, with `paths:
  ['config-tool/**', '.github/workflows/config-tool.yaml']`. Jobs:
  1. `test`: `actions/setup-go` (`go-version-file: config-tool/go.mod`) and `actions/setup-node`
     (22, `cache: npm`, `cache-dependency-path: config-tool/ui/package-lock.json`, because there
     is no lockfile at the repo root). Runs `npm ci`, `lint`, `typecheck`, `test` and `build` in
     `config-tool/ui`, then `gofmt -l $(go list -f '{{.Dir}}' ./...)` (must be empty),
     `go vet ./...` and `go test ./...`.
  2. `build` (needs `test`, same `setup-node` cache settings): **one job** that runs
     `make cross` (step 3), so CI and local builds use the same target list and ldflags. It
     uploads the five binaries from `bin/` with `actions/upload-artifact`.
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
  - `check.yaml`: add a `changes` job (`dorny/paths-filter@v3`, `permissions: pull-requests:
    read`) with the inverted filter, so any file outside the tool counts as a chart change and
    nothing has to be enumerated:
    ```yaml
    predicate-quantifier: every
    filters: |
      chart:
        - '!config-tool/**'
        - '!.github/workflows/config-tool.yaml'
    ```
    Gate `chart-lint` and `values-schema` with `needs: changes` and
    `if: needs.changes.outputs.chart == 'true'`. `chart-version` already `needs: chart-lint` and
    skips with it. The `links` job keeps running. Add a one-line "why" comment above the filter.
  - `pre-release.yml`: add `paths-ignore: ['config-tool/**', '.github/workflows/config-tool.yaml']`
    to its `pull_request` trigger, keeping the existing `types`. Otherwise a tool-only PR needs a
    semver label and pushes chart snapshots to quay.io for a change that touches no chart
    (TECH_STACK: "a pull request that touches `config-tool/` alone publishes no chart").
  - `release.yml`: add the same `paths-ignore` to its `push` trigger. With `chart-version` and
    `pre-release.yml` skipped, nothing makes a tool-only PR carry a semver label, so merging it
    would otherwise fail `generate-version` or cut a chart-numbered GitHub release that ships no
    chart. The tool is released separately under `config-tool/v*` tags (TECH_STACK), which is not
    part of CT-1.
  - Required checks: `test.yaml`, `pre-release.yml` and `release.yml` are skipped at workflow
    level, so a required status check from one of them stays *pending* on a tool-only PR; the
    job-level `if:` in `check.yaml` reports *skipped* and does not hang. Say so in the PR
    description so the maintainer can adjust branch protection.
- **`lychee.toml`:** add `"(^|/)node_modules/"` to `exclude_path`, so local link checks skip
  `config-tool/ui/node_modules`.
- **`config-tool/README.md`:**
  - prerequisites (Go version, Node 22);
  - `make ui`, `make build`, `make test`, `make cross` and `make build-window`;
  - running it: no args opens a tab, and `--no-browser`;
  - the security model in three lines: loopback, per-run token (the printed URL stays valid
    until the process exits, so do not share it), read-only kubeconfig whose path is never
    stored;
  - `npm run dev` with `DSC_CONFIG_URL` (step 3), and the proxy rule: it adds the run token
    only to requests from the dev server's own origin and answers `403` to any other;
  - the package layout, and that the core never imports `net/http`;
  - the Wails build tag, with a link to ADR 0011;
  - links to `doc/config-tool/`.
- **`doc/README.md`:** add "Configuration tool — building and running" pointing to
  `../config-tool/README.md`, under the existing *Configuration tool — design* entry.
- **`AGENTS.md`:** confirm the `config-tool` Build & Test commands match the Makefile.

Acceptance:
- a PR touching only `config-tool/` runs `config-tool.yaml` (test, build with five binaries, e2e)
  and none of `test.yaml`, `pre-release.yml`, `chart-lint` or `values-schema`;
- merging a tool-only PR to `main` does not run `release.yml`, so no GitHub release is cut and no
  chart is published;
- a PR touching `config-tool/` and any file outside it (e.g. `lychee.toml`) still runs
  `chart-lint`;
- a PR touching only `charts/` does not run `config-tool.yaml`. This holds for CT-1 only: the
  `model/10.16.0` bundle falls behind as soon as the chart version moves, and TECH_STACK wants
  the model-versus-chart check to run on both chart and tool PRs. The ticket that adds that
  check also adds `charts/data-space-connector/Chart.yaml` to the `config-tool.yaml` paths.
  Record this in the PR description as a follow-up;
- `.github/scripts/check-links.sh` passes.
