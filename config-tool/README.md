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
because rendering needs a complete target. It then renders the answers, so bundle and profile errors surface, and
prints every [finding](#rules-and-findings) to stderr (`warning: Stage 3 Foundations, answers.did.method: ...`), fails
on an error finding only, and otherwise stops with "writing values.yaml lands with US-7.1". An existing output file is refused until the render
epic decides how rendered keys merge with overrides (ADR 0015).

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
- Only two requests ever leave the machine, and only when the user presses their button on the Stage 3 *Data space*
  panel: *Check* (one GET to the TIR address) and *Import* (one GET of a data space's descriptor). Both live in
  `internal/server`, take the proxy from `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY`, time out after 10 s and never put the
  URL into an error. The import reads at most 64 KiB and follows up to 5 redirects, none that changes the scheme
  ([ADR 0022](../doc/adr/0022-config-tool-data-space-parameters.md#8-outbound-calls)). With no button pressed the tool
  works offline.

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
internal/render     core: answers to a values tree with provenance, fan-out and diff (ADR 0020)
internal/roundtrip  core: parse / classify / merge / write of a values.yaml, byte-preserving
internal/load       core: owned / recognized / unknown classification and the load report
internal/check      core: L2 rules of the bundle into warnings and blocking errors (ADR 0021)
internal/dataspace  core: data space descriptor parse and import preview, credential types for later stories (ADR 0022)
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

Derived answers of stage `foundations` (today the TIR address, recognized at the verifier's `tirAddress`) are
seeded into the profile as given answers (`load.SeedAnswers`, ADR 0022 Decision 7): the UI copies them from the report
on load, and `POST /api/profile/open` seeds the profile it returns. An answer the profile already holds always wins.
The value then lives in the saved `.dscprofile.yaml`, so the CLI sees it too. A profile saved by an earlier build lacks
it, and a CLI `generate` on it stops at the TIR rule (see [Rules and findings](#rules-and-findings)) until the profile
is opened and saved once in the GUI.

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
  revision: 2
catalogue:          # the bundle the roles came from; set by the server, only when it is not `model`
target:
  type: cluster
  context: kind-dsc
  namespace: provider
  createNamespace: true
  releaseName: provider
  chart: data-space-connector
  chartVersion: 10.16.0
  environment: production
role: provider      # see Roles
features:           # excerpt: every offered or turned-on feature; source is preset, loaded or user
  data.scorpio:
    enabled: true
    source: preset
answers:            # ids the bundle declares under foundations:, values checked against their type
  domain.base: acme.example.org
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
  unknown `open` ids, `source`, `role`, `features`, `catalogue`, `answers` and `secrets`.

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
A configuration without a role has one open question on Role.

## Roles

Stage 2 offers five presets ([ADR 0017](../doc/adr/0017-config-tool-role-presets.md)): **Consumer**,
**Provider** and **Consumer + Provider** deploy `data-space-connector`, **Operator** deploys `trust-anchor`, and
**Custom** deploys `data-space-connector` with nothing recommended. A role rates each catalogue feature:
`recommended` (on), `available` (off) or `not-offered` (off, left out of the profile unless the user or a loaded
file turns it on, see [Features](#features-stage-5)). A role offers only features of its own chart, so Custom does
not offer the three trust-anchor features.
Each card shows a floor of memory and CPU from [Sizing](../doc/deployment-integration/roles/PREREQUISITES.md#sizing),
and the chart version it was measured with. Custom has none, because its footprint depends on the selection.

The presets are data in `model/<version>/bundle.yaml`: `features:` is the catalogue, `roles:` the presets and
`roleMatrix:` maps the components of the matrix in [Deployment by Role](../doc/deployment-integration/roles/README.md)
to feature ids. That matrix is authoritative, and `internal/model/rolematrix_test.go` fails when the bundle disagrees
with it. Changing the matrix therefore means changing the bundle and bumping its `revision`. A feature's `keys` are
`roundtrip` paths, and each names its node with the whole subtree (`keycloak`, not `keycloak.*`). A segment with `*`
is refused. Features may share a key. The trust-anchor bundle declares no roles, so its catalogue comes from the
latest `data-space-connector` release bundle that declares them.

`GET /api/roles?chart=&version=` lists the roles and features for a target. `POST /api/preset` takes `profile`,
`role` and `sourceSha256`, and returns the profile with the preset applied:

- **Operator switches the chart** to `trust-anchor` and clears `chartVersion`. A notice names the chart and version
  that were dropped, and Target asks for the version again instead of preselecting the latest. Picking another role
  switches back the same way. A switch is refused (422) while a values file is loaded, because the load was classified
  against the current chart. Start a new configuration instead. Changing the chart on Target clears the role.
- **Toggles from a loaded values file win over the preset.** A feature the file switches keeps the file's value as
  `source: loaded`, whatever the preset says, also when the role does not offer it. A file that enables a feature of
  another chart is refused (422), naming the features. The server reads the toggles from its own load, never from the
  request.
- **User toggles win over both** and survive a role switch within the chart. A chart switch drops them, and the
  notice names them (`chartSwitch.droppedChoices`).
- **Answers survive a role change within the chart**, also those of a feature that goes off. A chart switch keeps
  the target fields except chart and version, every open-question mark, and the answers a `recognize:` rule of the
  old bundle places in Target or Foundations (by a `recognize:` rule or its `foundations:`) that the new bundle also
  declares in `foundations:` (ADR 0020 §9); a dropped answer loses its open-question mark too. `chartSwitch.carriedAnswers` and `chartSwitch.droppedAnswers` list the
  answer ids it kept and dropped, sorted.
- **`change` summarises a role change** when the request profile already has a role (null on the first pick):
  `enabled` and `disabled` list the features whose effective state flips, and `kept` lists the user and loaded
  toggles that keep it, each with its `source`. It holds ids only, in catalogue order. The effective state is the
  one Stage 5 shows: the recorded state, else the role's preset value. The call writes no server state, so the UI
  sends it again as the preview of a change ([ADR 0019](../doc/adr/0019-config-tool-role-change.md)).

Changing the role of a configuration that already has one asks first. The dialog shows the summary under *Turns
on*, *Turns off* and *Kept as they are* (with "you set it" or "from your values.yaml"), and for a chart switch
under *Carried over* and *Dropped*. Focus starts on *Keep <role>*, and Escape keeps the old role. The first pick
applies at once. Review lists every role change of the session as *Role changed from X to Y*, and its *summary*
button reopens the same summary, read-only. The record is session state, not part of the profile: starting over
(new, open, load, trial) clears it, and a reopened profile shows none. Titles on Review come from the catalogue of the
current target and fall back to the id for a role or feature that catalogue does not list.

The profile records `role`, `features` (each offered or turned-on feature with `enabled` and `source`) and, when the roles came
from another bundle than `model`, `catalogue`. The server sets `catalogue` on save, as it does `model`, and opening
applies the same revision rules to it.

## Foundations (Stage 3)

Stage 3 asks the cross-cutting answers of the target's bundle once ([ADR 0020](../doc/adr/0020-config-tool-foundations-render.md)).
A foundation's `group` picks its panel. *Organization* holds the display name and the DID, *Data space* the data
space's parameters (below), *Network* `domain.base` and the hosts derived from it, with a link to the resolve-inside-the-cluster prerequisite,
*Platform* the resource profile and *Egress* the forward proxy and the private CA (both below).
Each field has its doc link, *Ask later* (an `answers.<id>` open question on Review) and a *used by N values in M
components* button that lists its targets. Every answer change renders the profile through `POST /api/render`
(debounced). The render on entering the stage is the baseline, and the screen shows the grouped diff of each later
edit, with a hand-edited leaf of the loaded file flagged *pinned, not updated* next to its kept and would-be value.

A host derives as `<component>.<base>`, and `host.did` (the DID document host) as the base itself. Every host can be
overridden ([ADR 0023](../doc/adr/0023-config-tool-pinned-hosts.md)). *Edit* opens the input, and a text that differs
from the derived value pins the host: it becomes a given answer in the profile, shows a `pinned` badge and keeps its
value when the base domain changes. *Reset to derived* drops the answer and shows the derived value again. Pinning
needs no profile field, so it survives save and open. After a base change the diff lists the *Re-derived* answers
(`before → after`) and the answers that are *pinned, not updated* (the kept value and what it would have become).
Review lists every given answer that overrides a derived default with the label *pinned, not updated* and the value
it would take if derived. A host whose `features` are off is hidden, unless it is pinned or a finding names it.
There is no path-prefix scheme: every host is a sub-domain of the base domain. Pinning `host.verifier` or `host.did`
moves the ingress host and the URLs, but not the TLS certificate and its key, which stay with US-8.11.

The DID is two answers. `did.method` is a radio group (`did:web` or `did:key`), each option with its note. `did.value`
shows `did:web:<DID document host>` as *derived* with *Edit* while the method is `did:web`; *Edit* opens the input but the DID
stays derived until its text differs from the derived one, and an edited value gets *Reset to derived*. Under `did:key` there is no default and the DID is asked for. A DID must start with `<method>:`, so switching
the method drops a hand-set DID of the other method. The DID reaches the verifier, contract-management, the Keycloak
issuer and the `fdsc-edc.common` participant, issuer, STS client and holder ids; [FEATURE_MODEL.md](../doc/config-tool/FEATURE_MODEL.md#cross-cutting-values)
lists the DID keys left to other stories.

The *Data space* panel holds three answers ([ADR 0022](../doc/adr/0022-config-tool-data-space-parameters.md)):

- `dataspace.tirAddress` (type `url`): the Trust Anchor's TIR. It is the only one that renders, to the verifier's
  `decentralizedIam.vcAuthentication.vcverifier.deployment.verifier.tirAddress`. *Check* sends one GET to it and shows
  *reachable (HTTP n)* or *unreachable: reason*. The result is session state, shown on Review as information that never
  blocks; it is not saved in the profile.
- `dataspace.credentialTypes` (type `credentialTypes`): an ordered list of `{type, description?}` rows with unique
  types, which can be added, moved and removed.
- `dataspace.autoRegistration` (type `bool`): whether the data space registers participants at its Trust Anchor.

Chart 10.16.0 has no key for auto-registration, and its credential-type key sits in the verifier's service
registration list, which US-5.6 owns. So these two stay answers that write no key. Later stories read them through
`dataspace.CredentialTypes` and `dataspace.AutoRegistration`.

*Import from* takes a data space's address (or its `/.well-known/data-space-configuration` URL) and, on *Import*, has
`POST /api/dataspace/import` fetch the descriptor and preview it against the profile. The preview lists each value
that differs from the current answer, each value the answer's type refuses (*Not imported*), the chart's own published
keys (`supported_models`, `supported_protocols`, `authentication_protocols`) and every other key as *Not used*.
Nothing changes until *Apply*. The tool reads three optional top-level keys:

| Descriptor key | Answer | Shape |
|---|---|---|
| `tir_address` | `dataspace.tirAddress` | a URL |
| `credential_types` | `dataspace.credentialTypes` | a list whose elements are a string or `{type, description?}` |
| `participant_registration` | `dataspace.autoRegistration` | `automatic` or `manual` |

No publisher emits these keys yet, the chart's own `dsconfig-cm.yaml` included. A real data space therefore yields an
empty preview today, which says so and asks for the values by hand. `POST /api/dataspace/check` takes `{url}` and
returns `{status, httpStatus?, error?}`; the import takes `{url, profile}` and returns `{url, preview}`, with 502 when
the fetch or the parse fails.

The *Platform* panel asks `resources.profile` (`development`, `production` or `custom`; [ADR 0024](../doc/adr/0024-config-tool-resource-profile.md)).
It derives from Stage 1's environment and stays pending while no environment is chosen. Picking another profile pins it,
and *Reset to derived* follows the environment again. Below the choice, a table shows the requests and limits the
resolved profile sets on Keycloak, Scorpio, the TMForum API, the Trusted Issuers List and contract-management, with
the chart version the figures were measured on and a link to the [sizing guidance](../doc/deployment-integration/roles/PREREQUISITES.md#sizing).
`custom` sets nothing, so the chart's defaults stay in force.

The *Egress* panel declares the forward proxy and the private CA once ([ADR 0025](../doc/adr/0025-config-tool-proxy-and-private-ca.md)).
Each component gets them through its own mechanism: the verifier, Keycloak, the marketplace logic proxy, the
vc-operator and APISIX.

- `proxy.https` switches the proxy group. While it is empty, `proxy.http` and `proxy.no` do not derive and no proxy
  value renders. `proxy.http` follows `proxy.https`. `proxy.no` derives as soon as Stage 1 names the release and the
  namespace. It is one string that the Go, Keycloak, `proxy-from-env` and lua-resty-http matchers all read: loopback,
  `svc`, `cluster.local`, `<namespace>.svc` and the bare in-cluster service names of the bundle's `noProxyServices`
  table. It stays editable. CIDRs help only the verifier and the vc-operator, so the field asks for the pod and
  service CIDRs. Nothing renders while `proxy.no` is unresolved, so a proxy never goes out without it.
- `ca.privateSecret` names one Secret in the release namespace whose key `ca.crt` holds the PEM bundle. The tool never
  reads it. APISIX then trusts only that file and verifies every route's OpenID provider (`ssl_verify: true`), so a
  route that calls a publicly signed provider needs the public roots appended to `ca.crt`.

Below the fields, a table previews what each component gets: *set* for the proxy, *trusts ca.crt* for the CA, or why a
proxy element or CA mechanism is withheld and which loaded element occupies its path. The CA shows *not in effect*
while the loaded file overrides a value it needs, such as a route's `ssl_verify: false`. Notes then cover what the values cannot reach:
APISIX without an openid-connect route (the per-route values appear once US-5.6 or the loaded file has one), the
marketplace's VC login, which always uses the HTTPS proxy, and the vc-operator's Kubernetes API calls while `proxy.no`
holds no CIDR. Keycloak shows *ignored* while the loaded file sets `keycloak.proxy.spiProxyMappings`. *Create before
installing* lists the Secrets the values reference. No option disables TLS verification, and
`internal/model/insecure_test.go` keeps it that way.

### The `foundations:` section

A bundle's `foundations:` list declares the answers and every chart key they reach; there is no Go code per key.
Each entry has a dotted `id` (the key in the profile's `answers`), `stage`, an optional `group` (`organization`, `dataspace`,
`platform`, `egress` or `network`, the default), `type` (`string`, `hostname`, `domain`, `did` or `url`, each checked by one pattern, `choice`
from its `options`, `bool`, or `credentialTypes`, a list of `{type, description?}`), `label`, `help`, an optional repo-relative `doc` path, an optional `default`, `defaultWhen`
(`{id, equals}`: the default applies only while that answer resolves to `equals`; `{id, set: true}`: while it resolves at all), `prefix` (a template the answer must
start with), an optional `features` list (display only: Stage 3 hides the answer while any listed feature is off; render
is not gated) and `targets`. A `default` is a template over other answers, so `host.keycloak` (`"keycloak.{{ .domain.base }}"`)
exists only while `domain.base` is answered. A default may also read the reserved ref `target.environment`, Stage 1's
environment, which is never an answer and which no target may read. Templates hold only text and `{{ .id }}` actions. A target writes one
value with one of four functions; only `copy` writes a `bool` or a list:

```yaml
- {path: 'keycloak.realm.frontendUrl', fn: tpl, args: ["https://{{ .host.keycloak }}"]}   # render one template
- {path: 'keycloak.ingress.hosts[0].host', fn: copy, args: ["{{ .host.keycloak }}"]}     # the answer, type kept
- path: 'decentralizedIam.odrlAuthorization.apisix.ingress.annotations."cert-manager.io/alt-names"'
  fn: join                                                                               # render each, join with sep
  sep: ","
  args: ["{{ .host.dataService }}", "{{ .host.tmfApi }}"]
- {path: 'keycloak.ingress.hosts[0].paths[0].pathType', fn: const, args: [Prefix]}       # verbatim, only in a list
```

Paths use the round-trip syntax with list indices; quote one in a flow mapping. Helm replaces a list instead of
merging it, so a path indexes into a list only when the targets own it whole: elements `0..n-1` and every key of
each, with `const` for the keys no answer derives. An owned list is emitted all or nothing. A bundle with an unknown
function, a reference to an undeclared id, a cycle among defaults, a path outside `topLevelKeys`, a list index gap
or two targets on one path (except the table's `Only` targets, below) fails to load. A profile's `answers` accepts exactly the declared ids, and `open` accepts
`answers.<id>` for them. `internal/render/provider_test.go` measures the targets against `k3s/provider.yaml`: every
leaf that carries the domain is a target or a row of an exclusion table naming the story that owns it.

Lists that other stories also fill take keyed elements instead ([ADR 0025](../doc/adr/0025-config-tool-proxy-and-private-ca.md)):
`{path, element: {key, value}}` writes the one element whose `name` is `key`, into a list declared in the top-level
`lists:` table with its key field, the fields that must stay `unique` (`mountPath`) and the chart's `base` elements.
The render keeps the loaded elements, replaces the owned ones in place and appends the missing ones; without a loaded
list it writes the base first. Targets that only work together share a `mechanism`, which is withheld whole when one
of its elements would collide with a foreign one. `each: <list>` with `within: <mapping>` writes a key into every
element that has the mapping (APISIX `proxy_opts` and `ssl_verify` per route) and never creates one. `requires: [ids]`
renders a target only while those answers resolve. Defaults may read the reserved refs `target.release` and
`target.namespace`, and the template function `fullname <release> <chart>` follows Helm's fullname rule.

A bundle's `resources:` table sizes components per option of the choice foundation it names (`answer`). It lists the
sized `components` (`id`, `label`, `path` of the `resources` block, `enabledKeys` with their chart defaults, optional
`chartMemoryLimit`), the `unsized` JVM workloads only the `memoryLimit` rule reads (whose path may hold one `*` over a
map's keys, with `fallbackPaths`), `measuredOn`, `doc` and `profiles`: one row per option, `requests` and `limits`
(`cpu`, `memory`) per component. An option without a row (`custom`) sizes nothing. A Go test walks every table path
through `values.schema.json`.

### The render

`render.Render` turns the answers into a `Tree` of scalar leaves sorted by path, each with its provenance: the
foundation, the answers it used, the function and the target's index. A target that references an unanswered id writes
nothing. With a loaded file, a target path that the load classified as unknown or skipped (a hand edit) is
`pinned`: the leaf keeps the loaded value (`kept`), and `rendered` holds what the render would have written. When the
loaded node is not a scalar, there is no kept value and `Nested()` leaves the path to the overrides layer. Owned and
recognized leaves are re-rendered. The JSON of a tree is byte-identical across runs.

The model compiles the `resources:` table into `const` targets of its answer, one per profile, component and leaf,
each with `Only: <option>` and the answer as its ref. The render writes such a target only while the resolved answer
equals its `Only`, so one path may carry one target per option, and `render.Targets` lists each path once
(`resources.profile`: 20 values in 5 components). A loaded `resources` block is not back-derived; it stays unknown and
is pinned.

`GET /api/foundations?chart=&version=` lists the foundations with label, help, doc, type, group, options, default,
`defaultWhen`, prefix, `features` (empty when ungated), stage and their
fan-out (`targets`, `count`, sorted `components`), transitive through derived hosts, and `resources` (the table's
`answer`, `measuredOn`, `doc`, sized components and profiles, without `unsized`; `null` without a table). `POST /api/render` takes
`{profile, sourceSha256}` (422 with field errors when the profile does not validate, or when the sha is not the
loaded file's), renders it with the load the sha names as the overrides layer (none when it is empty, as on save) and
returns `{tree, diff, derived, handSet, findings, secrets}`. `tree.withheld` lists the mechanisms that did not
render, and `secrets` (`{namespace, name, key, purpose}`, from `requirements.Secrets`) the Secrets the values reference. `derived` (`render.DerivedDiff`) lists the answers whose default
derives from other answers and whose value moved since the previous render: `rederived` (`{id, before, after}`, not
given) and `pinned` (`{id, kept, before, after}`, given, so kept). Its baseline is dropped on a load, a profile open, a
source change and a model change, which return empty lists. `handSet` lists the given answers that override a derived
default, with the value they would derive. The diff groups `added`, `changed`, `removed` and `pinned` entries by foundation, against the
previous render of the same source. A load, a profile open or a render of another source resets that baseline. A
pinned leaf is reported only when its rendered value changes (ADR 0020 §7-8).

### Rules and findings

A bundle's `rules:` are L2 checks across answers ([ADR 0021](../doc/adr/0021-config-tool-l2-rules.md)). Each rule has
an `id`, a `severity` (`warning` or `error`), the `stage` and open `field` it points at, `when` conditions
(`{field, equals}`, all must hold, read on the resolved answers so a derived default counts), a `message` and a `doc`
path. Core package `internal/check` evaluates them into findings. Stage 3 shows a finding under its field, Review lists
it with a link back, and `generate` prints it. A warning never blocks; only an error stops `generate`.

A `when` may also address a feature as `features.<feature id>` with `equals: "on"` or `"off"`, and may use
`unset: true` instead of `equals`, which holds while the resolved answer, given or derived, is absent. Each condition
has exactly one of `equals` and `unset` (ADR 0022 Decision 6).

Two more rule kinds, each used instead of `field` and `when` ([ADR 0023](../doc/adr/0023-config-tool-pinned-hosts.md)).
`distinct: {prefix}` is an error per pair of foundations under the prefix that resolve to the same host (ignoring case
and a trailing dot); its `message` may use only `{{ .first }}`, `{{ .second }}` (the labels) and `{{ .host }}`.
`didWebHost: {did, host}` fires when a `did:web` DID's host (percent-decoded, without port, before the document path)
is not the resolved host foundation. Neither reads `features`, because every host renders whatever the role. Their
findings point at one field and name the other in `related`, so Stage 3 and Review link both.

`memoryLimit: {components}` ([ADR 0024](../doc/adr/0024-config-tool-resource-profile.md#6-rule-kind-memorylimit)) keeps
`field` and `when` as its precondition and yields one finding naming every listed table component that deploys (each
of its `enabledKeys` true in the render, else the loaded file, else the chart default) and ends up without a memory
limit (none rendered, loaded or at a fallback path, and no `chartMemoryLimit`, which a loaded `null` drops). Its
`message` may use only `{{ .components }}`. Until US-5.3 renders `enabled` keys, a feature that is off does not stop a
component from being named. `check.Evaluate` takes the loaded overrides as `render.Options`.

`caCoverage`, `noProxyCoverage` and `valueSet` ([ADR 0025](../doc/adr/0025-config-tool-proxy-and-private-ca.md#10-l2-rules-and-the-no-insecure-guard))
check the egress fan-out against the final values. The 10.16.0 bundle has eight rules:

- `did-key-in-production` warns when the environment is `production` and the DID method is `did:key`;
- `tir-address-for-verification` is an error on `answers.dataspace.tirAddress` while the feature
  `access.verification` is on and no TIR address is set, because the verifier cannot check issuers without it;
- `host-collision` (`distinct: {prefix: host.}`) is an error when two hosts resolve to the same name, because the
  chart routes by host. It points at the pinned host when exactly one of the pair is pinned, else at the later one;
- `did-web-host` (`didWebHost: {did: did.value, host: host.did}`) is an error on `answers.did.value` when a `did:web`
  DID does not resolve on the DID document host;
- `java-memory-limit` (`memoryLimit`) warns on `answers.resources.profile` while the environment is `production` and a
  deployed Java component, sized by the table or not, has no memory limit;
- `ca-coverage` (`caCoverage`) is an error while `ca.privateSecret` is set and a deployed component's CA mechanism is
  not in effect in the final values, or an APISIX route keeps `ssl_verify: false`. A withheld mechanism and
  contract-management, which has no mechanism in the tool yet, are warnings;
- `no-proxy-coverage` (`noProxyCoverage`) warns when `proxy.no` does not bypass an in-cluster service, for example one
  the loaded file renames;
- `keycloak-proxy-mappings` (`valueSet`) warns while the loaded file sets `keycloak.proxy.spiProxyMappings`, which makes
  Keycloak ignore the proxy settings.

## Features (Stage 5)

Stage 5 lists every feature of the chart the role deploys, and only those
([ADR 0018](../doc/adr/0018-config-tool-deliberate-choices.md)). Features of the other chart never appear, because
their keys do not exist in the target chart. The list has three sections: *Recommended for <role>*, *Available* and
*Not recommended for this role*. Each feature has a checkbox, and none is disabled.

- A toggle is recorded as `source: user` and stays so, also on a role switch within the chart.
- A feature whose state differs from the preset and did not come from it is a *deliberate choice*. It carries a
  marker such as *recommended on for Provider — you turned it off*, and Review lists it under *Deliberate choices*.
- Turning off a recommended feature asks first. The dialog names the feature's `consequence` (or its `summary`) and
  links the role guide. Focus starts on *Keep it on*, Tab stays inside the dialog, and Escape keeps the feature on.
  Every other toggle, including turning on a not-offered feature, applies at once.
- Toggles are sent one at a time, each on top of the previous answer, so a quick second click is never lost.

`POST /api/features` takes `profile`, `sourceSha256` and an optional `toggle: {feature, enabled}`, and returns the
profile and `choices`: per feature `offer`, `preset`, `enabled`, `source` and `deliberate`. Without `toggle` it only
reads the choices. No role, an unknown feature or a feature of another chart is 422. The core returns values only,
and every sentence shown is UI copy or bundle data. A catalogue feature may set `consequence:` in the bundle, and
`go test` requires it on every feature a shipped role recommends.

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
consumer files can be reopened with *Open values.yaml*: the trust-anchor header carries no chart version, because
loading reads that version as a `data-space-connector` one (ADR 0016 §6).

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
`features:`, `roles:` and `roleMatrix:` hold the [role presets](#roles).
`foundations:` holds the [cross-cutting answers](#foundations-stage-3) and the chart keys they reach.
`roles:` rate the features of `features:`, and `roleMatrix:` needs `roles:`.
Two charts are shipped: `data-space-connector` (`model/10.16.0/`) and `trust-anchor` (`model/1.3.6/`, only
`topLevelKeys` and two foundations without targets, no rules and no roles). Their version numbers differ, so they fit the one-bundle-per-version layout.
