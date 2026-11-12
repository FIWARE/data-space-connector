# 0009 - A Go core with an embedded web UI for the configuration tool

**Status:** Proposed
**Date:** 2026-11-12
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/TECH_STACK.md`](../config-tool/TECH_STACK.md)

## Context

Configuring the connector today means assembling a `values.yaml` by hand from five role guides
and the example profiles in [`k3s/`](../../k3s). The surface is large — 30 feature toggles, a
4187-line `values.yaml`, a second chart for the Operator role — and the same facts are repeated
across it: `k3s/provider.yaml` names one domain on 41 of its 1868 lines, and one DID on 15. A graphical tool
is to take that over, generate the file, and from a later iteration install it.

Four properties constrain the implementation. It has to run on Linux, macOS and Windows with
nothing preinstalled. It has to validate against the chart's own
[`values.schema.json`](../../charts/data-space-connector/values.schema.json) and render the
chart to catch what no rule anticipated. It has to read a cluster, and later install into it.
And it has to round-trip an existing `values.yaml` without destroying the keys, comments and
ordering it does not own, because an operator will not point it at a production file otherwise.

The tool is also not a separate product: it exists only to configure this chart, and it reads
this chart's schema and presets to do it.

Helm is a Go library. `helm.sh/helm/v3/pkg/{action,chartutil,engine,downloader}` does
dependency resolution, schema validation, rendering and install/upgrade with hooks, in process
and with typed errors. In any other language the tool reaches Helm through the `helm` binary:
parsing its output, depending on the user having a compatible version, and losing the error
detail that the validation screen is built on. `client-go` is in the same position for the
cluster side.

## Decision

- The tool is a **Go program with a TypeScript/React single-page UI embedded in the binary**
  (`embed.FS`), served on `127.0.0.1` with a one-time token and shown either in the OS webview
  (Wails) or in the default browser. One signed binary per platform, no runtime, no installer.
- The **core is a library**: model, profile, render, round-trip, validate, chart, cluster,
  export. The web UI and the CLI (`dsc-config generate|validate|install`) are two front ends
  over it. Nothing in the core knows about HTTP.
- **Helm and Kubernetes are used as libraries**, never as subprocesses.
- The YAML round-trip uses the **`gopkg.in/yaml.v3` node API**, so comments and key order
  survive; values the tool does not own are written back untouched.
- The **question model is data, not code**: one bundle per chart version
  (`config-tool/model/<chart-version>/`), kept next to `values.yaml` and released with the
  chart. A CI check fails a bundle that names a key the chart's schema does not have, or role
  presets that disagree with the component matrix in
  [roles/README.md](../deployment-integration/roles/README.md).
- **The tool lives in this repository**, under `config-tool/`, as a Go module
  (`github.com/FIWARE/data-space-connector/config-tool`). It has no purpose outside the
  connector and reads this chart's schema, so the chart change and the question that configures
  it belong in one pull request. It keeps its **own version**, tagged `config-tool/vX.Y.Z` —
  which is both the tag form Go requires for a module in a subdirectory and a trigger the chart
  workflows ignore, since they run on push to `main` and on pull requests, never on tags.

## Consequences

- Rendering, schema validation, dependency resolution and later install behave exactly as
  `helm` does, because they are `helm`. A chart-side change to templates or schema needs no
  change in the tool.
- One CI job cross-compiles six artifacts. macOS notarization and Windows signing become
  release-engineering work that has to be budgeted.
- The GUI never becomes load-bearing: `dsc-config generate` produces the same file in CI, and a
  terminal front end or a hosted mode can be added over the same core without touching it.
- The chart and the questions about it are versioned together, which is what keeps the tool
  usable across chart releases — at the cost of a bundle that has to be updated with every
  release that changes the configuration surface. The CI check makes that failure loud instead
  of silent.
- Go is a second language in this repository, which is otherwise Helm, Java (the integration
  tests) and shell. It is already the language of everything the tool talks to.
- The repository gains a second release train. It costs a `config-tool-release.yml` triggered by
  `config-tool/v*` tags, a build-and-test workflow for the tool, and path filters so that a
  front-end change does not run the k3s integration tests and a values change does not run the Go
  build. The chart's own release path is untouched: a pull request that only changes
  `config-tool/` bumps no chart version, so `chart-release` publishes nothing.
- Extracting the tool later, should it ever get a life of its own, means `git filter-repo` and a
  changed module path. Accepted: nothing in the design depends on the layout.

## Alternatives

- **Electron + TypeScript.** No Helm SDK; `helm template` and `helm install` become subprocess
  calls with version-dependent output, and the Kubernetes JS client is thinner than
  `client-go`. 150 MB artifacts. Rejected: the backend would end up in Go anyway.
- **Tauri + Rust.** Smallest artifacts, good `kube-rs`, but no Helm library at all and a weaker
  comment-preserving YAML story. Rejected on the same ground.
- **Python + Qt.** Packaging for three platforms becomes the hardest part of a tool whose
  selling point is "download and run". No Helm SDK. Rejected.
- **A terminal UI (Bubbletea).** Would reuse the same core, but the request is for a graphical
  tool and the heavy screens — credential types, the YAML diff, the schema-driven forms — are
  poor in a terminal. Kept as a later front end, not as the first one.
- **A hosted web service.** Means putting kubeconfigs and internal database endpoints on a
  server, with authentication and secret handling, for no gain over a local tool. Kept as a
  later deployment mode; the binary already is an HTTP server.
- **A generator script with a form.** Effectively where the project is today: no schema
  validation, no cluster awareness, no round-trip, no incremental change. Rejected.
- **A repository of its own for the tool.** Rejected, and this ADR reverses an earlier draft that
  proposed it. The model bundle has to sit next to `values.yaml` to be checkable against it, so a
  separate repository would split every feature change in two — the question here, the render
  function and its test there — with neither side able to run against the other until one of them
  publishes. The benefit claimed for it, an independent release cadence, comes from the tag scheme
  instead and costs nothing.
