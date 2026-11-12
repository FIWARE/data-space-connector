# Tech stack

What the tool is built with, and why. The decision itself is recorded as
[ADR 0009](../adr/0009-config-tool-stack.md); this document carries the reasoning, the
architecture and the libraries.

<!-- ToC -->
- [What constrains the choice](#what-constrains-the-choice)
- [Decision](#decision)
- [Alternatives](#alternatives)
- [Architecture](#architecture)
- [Libraries](#libraries)
- [Packaging and distribution](#packaging-and-distribution)
- [Security](#security)
- [Testing](#testing)
- [Where the code lives](#where-the-code-lives)
- [Iterations](#iterations)
<!-- /ToC -->

## What constrains the choice

| # | Requirement | Consequence |
|---|---|---|
| 1 | Runs on Linux, Windows and macOS | No per-OS runtime the user has to install first; cross-compilation from one CI |
| 2 | A graphical tool a platform engineer installs in minutes | One downloadable artifact, no package manager, no container |
| 3 | Must talk to Kubernetes and Helm **later**, in the same tool | The language needs first-class `client-go` and the Helm SDK, or it will shell out to binaries it cannot error-handle |
| 4 | Must validate against the chart's own `values.schema.json` and render with `helm template` | Helm's rendering is Go text/template plus Sprig plus Helm's own functions — reimplementing it is not an option |
| 5 | Must round-trip a `values.yaml` without destroying comments or key order | A YAML library with a node API, not a struct marshaller |
| 6 | Must run headless in CI (`--generate`) | The GUI is a front end over a library, never the other way round |
| 7 | Offline-capable in an air-gapped cluster's jump host | No CDN, no telemetry, charts resolvable from a local cache |
| 8 | Must be able to create credentials, not only name them, and to hand them to whatever the cluster uses (plain Secrets, Sealed Secrets, External Secrets) | Cryptography and a Kubernetes write path in the tool, with no value ever persisted to disk |
| 9 | Maintained alongside a chart that releases often | The question model is data, shipped with the chart; the binary is thin |

Requirements 3 and 4 do most of the deciding: **Helm is a Go library**
(`helm.sh/helm/v3/pkg/action`, `…/chartutil`, `…/engine`) and has no equivalent anywhere else.
Any other language reaches Helm through the `helm` binary — which means parsing its output,
depending on the user having the right version, and losing the structured errors that make
good validation messages.

## Decision

**A Go backend with an embedded TypeScript/React single-page front end, shipped as one
binary per platform.** The binary serves the UI on `127.0.0.1` and opens it in a native
window (Wails) or, as a fallback, in the default browser.

```
  dsc-config                       → opens the GUI
  dsc-config generate -p profile.yaml -o values.yaml   → headless, same core
  dsc-config validate -f values.yaml --chart-version 10.16.0
  dsc-config install  -p profile.yaml --context prod   (iteration 2)
```

Why this and not something else:

- **Helm in-process.** Dependency resolution, `values.schema.json` validation, rendering and
  (later) install/upgrade with hooks are SDK calls, not subprocesses. Errors come back typed,
  with the file and line of the template that failed — which is what Stage 7's validation pane
  needs.
- **Kubernetes in-process.** `client-go` and the discovery API give stage 4 its detection
  (CRDs, ingress classes, storage classes, operators) and stage 8 its install, with the same
  kubeconfig handling every other Kubernetes tool has.
- **One artifact.** `GOOS`/`GOARCH` cross-compilation produces six binaries from one CI job,
  statically linked, no runtime, ~25–40 MB with the UI embedded via `embed.FS`. A platform
  engineer downloads and runs it; that is the whole installation.
- **The GUI is optional.** The same core answers `generate` in a pipeline, which is what a
  GitOps team will want after the first demo.
- **The front end is ordinary web.** React + TypeScript, no framework exotica, so the complex
  parts — the schema-driven Advanced forms, the YAML diff, the credential-type table — use
  components that already exist (CodeMirror, a JSON-Schema form renderer).
- **Wails is a wrapper, not a dependency.** It uses the OS webview (WebView2, WebKit, GTK) to
  give a real application window with native file dialogs. If it ever becomes a liability,
  dropping it leaves a working tool that opens a browser tab — the architecture does not
  depend on it.

## Alternatives

| Option | Why not |
|---|---|
| **Electron + TypeScript** | No Helm SDK: `helm template` and `helm install` become subprocess calls with version-dependent output. 150+ MB per platform. A second language for the backend anyway, since the Kubernetes JS client is thinner than `client-go`. |
| **Tauri + Rust** | Smallest binaries and a good Kubernetes client (`kube-rs`), but no Helm library at all, and the YAML round-trip story is weaker than Go's `yaml.v3` node API. The whole value of requirement 4 is lost. |
| **Python + Qt / wxWidgets** | Packaging on three platforms (PyInstaller) is the hardest part of the project, for a tool whose selling point is "download and run". No Helm SDK either. |
| **A terminal UI (Go + Bubbletea)** | Would reuse the same core and is attractive for CI hosts, but the request is for a graphical tool, and the heavy screens (credential types, the YAML diff, the schema-driven forms) are poor in a terminal. **Kept as a later front end over the same core**, not as the first one. |
| **A web service the organization hosts** | Means putting a kubeconfig and the endpoints of internal databases on a server. Multi-user, authentication and secret handling for no gain over a local tool. **Kept as a later deployment mode** — the binary already is an HTTP server. |
| **A `values.yaml` generator script (Bash/Python) with a form** | Where the project is today, effectively. No validation against the schema, no cluster awareness, no round-trip, no incremental change. |
| **VS Code extension** | Excellent for the Expert YAML mode, useless for Pia, who does not have VS Code on the jump host. A possible companion, not the tool. |

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│  Front end — TypeScript / React (embedded in the binary)                         │
│    wizard stages · schema-driven Advanced forms · YAML diff · validation pane    │
└───────────────────────────────┬──────────────────────────────────────────────────┘
                 JSON over HTTP │ + WebSocket for progress (install, scan)
┌───────────────────────────────▼──────────────────────────────────────────────────┐
│  API — net/http on 127.0.0.1, one-time token                                     │
│    GET  /api/model          the question model for the selected chart version    │
│    GET/PUT /api/profile     the answer set                                       │
│    POST /api/render         profile → values.yaml (+ diff)                       │
│    POST /api/validate       L1 schema · L2 rules · L3 helm template              │
│    POST /api/load           values.yaml → profile + overrides + conflicts        │
│    GET  /api/cluster/scan   detection for stage 4, incl. sealed-secrets and ESO
    POST /api/secrets/*      generate · seal · apply; values pass through, never stored                                │
│    POST /api/install        iteration 2, streams over the WebSocket              │
└───────────────────────────────┬──────────────────────────────────────────────────┘
┌───────────────────────────────▼──────────────────────────────────────────────────┐
│  Core (a Go library, usable without the UI)                                      │
│                                                                                  │
│   model      loads the version bundle: features, presets, foundations, rules     │
│   profile    the answer set; apply preset, toggle, resolve dependencies          │
│   render     profile → values tree, with per-key provenance                      │
│   roundtrip  parse / classify / merge / write, comments preserved                │
│   validate   L1 schema · L2 rules · L3 render · L4 preflight                     │
│   chart      Helm SDK: pull, dependency update, template, install, upgrade       │
│   cluster    client-go: discovery, CRDs, classes, existing releases              │
│   secrets    delivery modes, generation, sealing, apply — never persists a value
   export     values.yaml · .dscprofile.yaml · NEXT_STEPS.md · sealed-secrets.yaml                      │
└──────────────────────────────────────────────────────────────────────────────────┘
```

The CLI is a second front end over `core`. Nothing in `core` knows about HTTP, which is what
keeps `dsc-config generate` honest and makes the core testable without a browser.

## Libraries

| Concern | Choice | Note |
|---|---|---|
| Helm | `helm.sh/helm/v3/pkg/{action,chart,chartutil,engine,downloader}` | Render, schema validation, dependency update, install/upgrade |
| Kubernetes | `k8s.io/client-go`, `k8s.io/apimachinery` | Discovery for detection; apply for install |
| YAML round-trip | `gopkg.in/yaml.v3` node API | Keeps comments and order; `sigs.k8s.io/yaml` for the schema path, since Helm uses it |
| JSON Schema | `santhosh-tekuri/jsonschema/v6` | Draft-07, what the chart's schema is |
| Sealed Secrets | `github.com/bitnami-labs/sealed-secrets/pkg/crypto` | Seals in process (AES-256-GCM session key wrapped with RSA-OAEP); `kubeseal` is used only to fetch a certificate when it is on the PATH |
| Secret generation | `crypto/rand`, `crypto/ecdsa`, `go-jose` for JWKs | See [Generation](SECRETS.md#generation) |
| Desktop window | `github.com/wailsapp/wails/v2` | OS webview; browser fallback when unavailable |
| CLI | `spf13/cobra` | `generate`, `validate`, `install` |
| Front end | React 18, TypeScript, Vite | Built into `embed.FS` at compile time |
| Editor / diff | CodeMirror 6 with the YAML mode | Expert mode and the diff pane |
| Advanced forms | A JSON-Schema form renderer over `values.schema.json` | The escape hatch of every feature page |
| Tests | `go test`, Playwright for the UI | See [Testing](#testing) |

No telemetry library, by design.

## Packaging and distribution

- **Artifacts:** `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
  `windows/amd64` — signed, with checksums, from one GitHub Actions job, attached to the
  tool's release and linked from the chart's release notes.
- **macOS** needs notarization or the first run is blocked; **Windows** wants a signed
  executable or SmartScreen warns. Both are release-engineering work that has to be budgeted,
  not discovered.
- **No installer.** The binary is self-contained; `dsc-config` with no arguments opens the UI.
- **Air-gapped use:** `--chart-dir` points at a chart already on disk, `--model-dir` at a
  bundle, and nothing reaches the network.
- **Container image** for the CI path, with the same binary.

## Security

- The HTTP server binds `127.0.0.1` on a random port and requires a token generated at start
  and passed to the webview — a local tool should not be reachable from the network.
- **No secret value is ever written into the generated `values.yaml`, the profile or
  `NEXT_STEPS.md`.** The tool does generate credentials and, against a connected cluster, create
  them; a generated value goes to the cluster, or into a `SealedSecret` encrypted for that
  cluster's controller, and is shown to the user exactly once. The profile keeps the delivery
  mode, the Secret name, its key names and a truncated fingerprint. The full model, including
  what happens to the Keycloak bootstrap admin, is in [Secret handling](SECRETS.md).
- Secret values live in the core's memory for the life of the operation and are never logged,
  never cached on disk, and never sent anywhere but the Kubernetes API of the selected context.
  The sealing path needs only the controller's public certificate, so it also works from a
  workstation with no cluster access.
- The kubeconfig is read from the usual location, used in-process, and never copied into the
  profile; only the context *name* is stored.
- Cluster access is read-only until the user presses Install, and that screen shows the exact
  Helm operation first.
- Charts are pulled from their pinned repositories with their digests checked, the same way
  `helm dependency update` does it.

## Testing

| Layer | How |
|---|---|
| `core` units | Table-driven Go tests over the model: toggle resolution, preset application, render functions, classification |
| **Round-trip** | Property test: load each of `k3s/*.yaml`, save without changes, assert the file is byte-identical apart from the generated header |
| **Reproduction** | The acceptance test that matters: the tool must be able to **reproduce the repository's own scenario profiles**. Feed it the answers behind `k3s/provider.yaml`, `k3s/consumer.yaml` and `k3s/trust-anchor.yaml`, render, and assert the result installs the same workloads. This keeps the model and the chart from drifting |
| Validation rules | One test per L2 rule, with the failure it was written for |
| Render | `helm template` the generated values for every role preset, in CI, against the pinned chart version |
| Model/chart agreement | A check in **this** repository: every key the model bundle names exists in `values.schema.json`, and the role presets match the component matrix in [roles/README.md](../deployment-integration/roles/README.md) |
| UI | Playwright over the built binary: each stage, the toggle dialogs, the diff |
| Install | Against k3s in CI, reusing the `it/` harness, for the three role presets |
| Secrets | A test asserts no generated file contains a generated value; sealing is verified by decrypting with a test controller key; `generate+apply` and the `generatePasswords` coupling run against k3s with a Sealed Secrets controller |

## Where the code lives

**In this repository**, under `config-tool/`. The tool has no purpose outside the connector, it
reads this chart's `values.schema.json`, and its question model has to move with `values.yaml` —
so a feature added to the chart and the question that configures it belong in **one** pull
request, testable against the chart in the same checkout.

```
config-tool/
  go.mod                 module github.com/FIWARE/data-space-connector/config-tool
  cmd/dsc-config/        the binary: GUI and CLI
  internal/…             the core packages from the diagram above
  ui/                    the React/TypeScript front end, built into embed.FS
  model/<chart-version>/ the question model, released with the chart
  README.md              building and running the tool
```

The docs stay in [`doc/config-tool/`](README.md) with the rest of the deployment documentation,
and the ADRs in [`doc/adr/`](../adr/README.md), where they already are.

### Why not a repository of its own

A separate repository would split every feature change across two: the question in the model
bundle here, the render function and its test there, with neither side able to run against the
other until one of them publishes. That is the coupling cost without the benefit — and the only
benefit on offer was an independent release cadence, which the layout below gives anyway.

### How it coexists with the chart releases

The chart workflows are not in the way. `release.yml` runs on **push to `main`** and takes its
version from the merged pull request's semver label; `chart-release` publishes only a chart whose
version changed, so a pull request that touches `config-tool/` alone publishes no chart. `check.yaml`
and `pre-release.yml` run on pull requests. **None of them triggers on tags**, which leaves tags
free for the tool:

| | Chart | Tool |
|---|---|---|
| Version | `Chart.yaml` + semver label on the pull request | its own, tagged `config-tool/vX.Y.Z` |
| Released by | `release.yml` on push to `main` | a new `config-tool-release.yml` on `push: tags: ['config-tool/v*']` |
| Artifacts | charts to `quay.io/fiware/helm-charts` and `gh-pages` | six signed binaries on the GitHub release |
| Cadence | every chart change, including automated subchart bumps | when the tool changes |

`config-tool/vX.Y.Z` is also exactly the tag form Go requires for a module in a subdirectory, so
`go install github.com/FIWARE/data-space-connector/config-tool/cmd/dsc-config@config-tool/v0.4.0`
works without any further arrangement.

Two housekeeping items come with it: a `config-tool.yaml` workflow that builds and tests the tool
on pull requests that touch it, and path filters so that `test.yaml` — today `on: [push,
pull_request]` with no filter — does not run `helm lint` and the k3s integration tests for a
front-end change, nor the Go build for a values change. The model-versus-chart check is the
deliberate exception: it runs on both.

Adding Go to the repository is not a new precedent. The repository already carries Helm charts, a
Maven build with 18 modules and the Java integration tests under `it/`, and shell tooling under
`.github/scripts/` and `k3s/`.

### What this costs

If the tool ever did grow a life beyond this connector, extracting it means `git filter-repo` and
a changed Go module path — a breaking move for anyone who imported it. That is a cost worth
accepting for a tool whose entire purpose is this chart; nothing about the layout above is load
bearing for the design.

## Iterations

| Iteration | Scope | Front end | Cluster |
|---|---|---|---|
| **1** | Wizard, role presets, features, round-trip, validation L1–L3, export bundle, secret modes `chart` and `reference` | Full | Read-only detection, optional |
| **2** | Install and upgrade, preflight L4, `fdsc-verify` integration, diff against the deployed release, secret modes `generate+apply`, `sealed` and `external` | + install and Secrets screens | Read/write |
| **3** | Operator role end to end (trust-anchor chart, participant onboarding), multi-release scenarios | + a release list | Read/write |
| **4** | GitOps export (values per environment, a PR instead of an install), the TUI front end, the hosted mode | + alternative front ends | Read/write |
