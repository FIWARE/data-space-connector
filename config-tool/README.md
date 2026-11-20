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

`dsc-config generate -p <name>.dscprofile.yaml [-o values.yaml]` renders a saved
[profile](#profiles) without the GUI (stdout by default). It refuses a profile with an unanswered or malformed Target field, naming the file and the field,
because rendering needs a complete target. An existing output file is refused until the render
epic decides how rendered keys merge with overrides (ADR 0015). Rendering itself is not
implemented yet.

`dsc-config generate --trial [--output-dir <dir>]` writes the three files of the
[local trial](#try-it-locally).

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
internal/profile    core: the answer set and its .dscprofile.yaml format (ADR 0015)
internal/atomicfile core: temp-file-and-rename writes that keep the file mode
internal/cluster    core: kube context names from kubeconfig files, read-only
internal/render     core: profile to values tree
internal/roundtrip  core: parse / classify / merge / write of a values.yaml, byte-preserving
internal/load       core: owned / recognized / unknown classification and the load report
internal/validate   core: values against the chart's schema
internal/trial      core: local trial descriptor, rendered outputs, production warnings, all-or-nothing write
model/<version>/    question-model bundles, embedded
trial/              local trial descriptor and k3s/ values copies, embedded (ADR 0016)
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

## Loading a values.yaml

*Open an existing values.yaml* on S0 uploads the chosen file's bytes, never a path or decoded
text, to `POST /api/load?name=<basename>`. The body is capped at 8 MiB (413 above it), and a file
`roundtrip` refuses gives 422. `internal/load` classifies every leaf in this order
([ADR 0014](../doc/adr/0014-config-tool-load-classification.md)):

- **owned**: under a `# dsc-config:owned <id>[,<id>...]` marker. The marker sits in the head
  comment of a mapping key and owns that key's subtree and the following siblings, until a blank
  line between siblings, the next sibling with its own head comment, or the end of the mapping.
  Markers count only in a file that carries the generated header.
- **recognized**: a scalar whose path matches a `recognize:` rule of the bundle (see
  [Chart versions](#chart-versions)) and whose derived value is the answer's value.
- **unknown**: everything else, including a matching leaf whose value differs from the answer
  (`differs from <answer> = <value>`): it was pinned by hand and is never overwritten.

The chart's values may sit under a wrapper key, e.g. `dsc:` when Argo CD or an umbrella chart
packages this chart as a dependency. Rule paths are relative to that *root*: the load picks the
mapping, at most three keys deep, whose chart top-level keys (`topLevelKeys` in the bundle)
outnumber its other keys by the widest margin, and `?root=<path>` overrides it (`?root=` is the whole file).
Review names the root and re-reads the file under another candidate; keys outside the root are
unknown. A re-read only swaps the report (stage, answers and the chosen context stay). It is applied
even when the user has left Review meanwhile, as the server's slot follows it too; it is dropped
only when another file was opened or the configuration started over. One re-read runs at a time:
the picker stays disabled until it answers, even across leaving and returning to Review.

Owned leaves back-derive answers too, and before the others, so an owned value always wins.
Pairs `roundtrip` cannot address (a shadowed duplicate key, an alias or complex key) are listed
once each as *skipped*. The bundle is the one for the header's chart version, otherwise the
latest one, and the report says why. Recognizing a new key is a bundle change: the epic that adds
a question appends its rule to `model/<version>/bundle.yaml`, without code.

The UI lands on Review with the report: counts, derived answers, and each entry with its file
line. Back-derived answers show read-only on their stage's placeholder; only `target.chartVersion`
becomes an answer. Expert / Overrides lists the unknown and skipped entries. The server keeps the
latest load in memory as the overrides layer, and a saved [profile](#profiles) references it; the
load itself writes nothing to disk.

## Profiles

A `.dscprofile.yaml` is the tool's whole state: the answers, the open questions and a reference to
the values file the overrides came from ([ADR 0015](../doc/adr/0015-config-tool-profile.md)).
`internal/profile` encodes it from Go structs, unlike a `values.yaml`, which goes through
`internal/roundtrip`. The output is deterministic: fixed key order, `open` sorted, 2-space indent,
no timestamp and no tool version, so saving twice without a change gives the same bytes. Comments
added by hand are not kept. Decoding is strict: an unknown key, a second document or a `format`
other than 1 is refused.

```yaml
# dsc-config profile: the answer set values.yaml is rendered from. It never holds a secret value.
format: 1
model:              # the question-model bundle it was built with
  chart: data-space-connector
  version: 10.16.0
  revision: 1
target:
  type: cluster
  context: kind-dsc
  namespace: provider
  createNamespace: true
  releaseName: provider
  chart: data-space-connector
  chartVersion: 10.16.0
  environment: production
answers: {}         # no answer id exists yet, so any key is refused
open:               # fields left open on purpose
  - field: target.context
source:             # the loaded values file: metadata only, never its content
  file: values.yaml
  root: dsc
  sha256: <hex>
secrets: {}         # Epic 8; any entry is refused until then
```

The profile never copies the overrides, because a loaded `values.yaml` may hold plaintext passwords
in keys the tool does not own ([SECRETS.md](../doc/config-tool/SECRETS.md) rule 1). It records only
`source`: the file's base name, the root it was read under, and its SHA-256. **Keep the profile next
to its values file.** Opening a profile reads `<profile dir>/<source.file>` under the recorded
root and restores the overrides layer. A changed file is restored with a note. A missing or
unreadable one is reported on Review, and saving again drops the reference to it. `source.file`
must be a plain base name, so a shared profile cannot make the tool read outside its own directory.

**Save** (rail button, *Save profile* on Review, ⌘S / Ctrl+S) replaces the configuration's own
profile in place, without a prompt, once it has one (opened or saved before). Otherwise it asks for a
path, by default `<dir>/<releaseName or connector>.dscprofile.yaml` in the directory of the latest
recent profile, else the home directory. `PUT /api/profile` takes `path`, `overwrite`, `profile`
and the `sourceSha256` of the load report the UI holds:

- Only an absolute path ending in `.dscprofile.yaml` is accepted. Without `overwrite` an existing
  file gives 409, and the dialog asks to confirm *Replace*. The write is atomic (temp file and
  rename, file mode kept; new files are 0644).
- `source` is attached only when `sourceSha256` equals the server's current load. *New
  configuration* drops it, and a hash of another load gives 422.
- The save compares the file next to the target path with the loaded one, and a note says
  when reopening will not find it. The save still goes through, so the file can be copied later.
- A half-finished configuration saves as typed: format errors stay stage errors. Only structural
  errors give 422: `format`, `model`, unknown `target.type` / `target.environment` values,
  unknown `open` ids, `source`, `answers` and `secrets`.

**Open** is the *Open profile* card on S0, a path field, or a click in the *Recent* list. Both use
`POST /api/profile/open` with the same path rule (404 when the file is missing) and land on Review
with the profile's notes. A bundle whose `chart@version` is not shipped, or whose `revision` is
newer than the shipped one, is refused (migration is [US-9.3](../doc/config-tool/USER_STORIES.md#epic-9--changing-a-configuration-later)).
An older revision opens with a note.

**Recent** (`GET /api/profile/recent`) lives in `os.UserConfigDir()/dsc-config/recent.yaml`
(directory 0700, file 0600), or under `$DSC_CONFIG_HOME` when that is set. It holds absolute
paths with name, chart version and save time, newest first, deduplicated by path, at most 10, and
drops entries whose file is gone. Browser storage is not used, because the port, and with it the
origin, changes on every run. A failing list never fails a save or an open: it comes back as a
note, and S0 shows it below the list.

**Open questions.** *Ask later* next to a Target field marks it open; the value stays as
provisional, and unmarking keeps it too. A field is open when it is unanswered or marked, and
it is counted once. The stage rail marks each stage that has one, the footer shows the total, and
Review lists every open question with a link to its field. A marked empty field is open, not an error.

## Try it locally

The fourth S0 card produces the values of the [Quick Start](../doc/deployment-integration/local-deployment/LOCAL.MD#quick-start):
three releases, in the order `k3s/deploy.sh` installs them. The tool only writes the values files; the namespaces,
operators, CA and infrastructure the Quick Start applies first, and the install itself, stay with the Quick Start.

| Release | Namespace | Chart | File | Same values as |
|---|---|---|---|---|
| `trust-anchor` | `trust-anchor` | `trust-anchor` | `trust-anchor.yaml` | [`k3s/trust-anchor.yaml`](../k3s/trust-anchor.yaml) |
| `provider` | `provider` | `data-space-connector` | `provider.yaml` | [`k3s/provider.yaml`](../k3s/provider.yaml) |
| `consumer` | `consumer` | `data-space-connector` | `consumer.yaml` | [`k3s/consumer.yaml`](../k3s/consumer.yaml) |

The card opens a prerequisite screen: what the trial needs (a local cluster, kubectl and Helm, the Quick Start's
setup, and memory, with a link to [Sizing](../doc/deployment-integration/roles/PREREQUISITES.md#sizing) rather than a
figure) and what it gives. *Generate the three configurations* lands on Review, which shows "Local trial" with a
**Development** badge and lists each release with namespace, `chart@version`, file, source and its values (collapsed).
Stages Target to Configuration are marked skipped (–); choosing one offers *Start a new configuration*. The trial is
fixed, so Save (rail button, Review action and ⌘S) is disabled. *Write files* takes an absolute, existing directory,
by default the one of the latest recent profile.

`dsc-config generate --trial [--output-dir <dir>]` does the same without the GUI (`.` by default); `--trial` excludes
`--profile` and `--output`. It prints one line per written file and the Quick Start link.

**The values are bundled, not rendered** ([ADR 0016](../doc/adr/0016-config-tool-local-trial.md)): the renderer that
turns answers into values does not exist yet. `trial/` holds the descriptor `trial.yaml` and byte-identical copies of
the three `k3s/` files, because `go:embed` cannot reach above the module. A test fails when they drift, or when a
release's `chartVersion` differs from its `charts/*/Chart.yaml`; `make trial-sync` copies the `k3s/` files over them. Each output is the bundled file behind the generated header (profile
`local-trial`) and a two-line development banner naming chart, version and source file. Only the provider and
consumer files can be reopened with *Open values.yaml*: the trust-anchor header carries no chart version, as no
trust-anchor model bundle exists.

**Nothing is overwritten.** If any of the three files exists, symlinks included, none is written: the CLI fails and
`POST /api/trial/write` answers 409, both naming every existing file. A failure halfway removes the files the run
created.

**Production warnings** are a fixed list in `internal/trial/warnings.go` until Epic 6 validates values: self-signed
certificates, chart-deployed databases, `127.0.0.1.nip.io` host names, `tlsTrustAll`, the example identities, the
published keystore password, the local squid proxy and laptop memory limits. Review lists them as "Production
warnings (N)" with the releases each applies to, and the CLI prints them to stderr as `warning:` lines on every run,
before writing, so a refused run shows them too. Neither can hide them. Each warning carries evidence, a text in the
bundled values or a chart default, and a test fails when a release no longer contains it.

`GET /api/trial` returns the releases with their rendered values and the warnings; `POST /api/trial/write` takes
`{"dir": "<absolute path>"}` and answers 201 with the written files, or 422 for a relative or missing directory.

`TestRenderedValuesTemplate` runs `helm template` on each output against the repo's chart. It skips without `helm` on
`PATH` or without the charts' dependencies; `DSC_CONFIG_REQUIRE_HELM=1` turns the skip into a failure, as in the CI
job `trial-render`. To run it locally, add the Helm repositories listed in the *Add Helm repositories* step of
[`pre-release.yml`](../.github/workflows/pre-release.yml), then, from the repository root:

```shell
helm dependency update charts/data-space-connector
helm dependency update charts/trust-anchor
cd config-tool && DSC_CONFIG_REQUIRE_HELM=1 go test ./internal/trial -run TestRenderedValuesTemplate
```

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
one capture group. Rules that share an answer must agree on its `stage` and `kind`. A bundle with
an unknown key, an invalid rule or a duplicate path fails to load, and a test checks every
shipped rule path against the chart's `values.yaml`.
`topLevelKeys` lists the chart's top-level values keys, by which a load finds values wrapped
under another chart's key. When it is set, every rule must start with one of them, and a test
keeps the latest bundle's list equal to the chart's `values.yaml`.
`revision` (a positive integer, required) counts changes to a bundle within one chart version.
A profile records `chart`, `version` and `revision` of the bundle it was built with (ADR 0015 §6).
