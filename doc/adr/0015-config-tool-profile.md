# 0015 - The profile as the tool's state: format, save and open by path, recent list, open questions

**Status:** Proposed
**Date:** 2026-11-19
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md)

## Context

CT-4 (#119) makes the answer set, `.dscprofile.yaml`, the state the user saves and resumes
(US-1.4 and US-7.2 of [`USER_STORIES.md`](../config-tool/USER_STORIES.md)): human-readable,
git-friendly, no secrets, and it records the chart version and model bundle it was built with.
The same ticket adds a per-field *open question* marker. Three constraints shape the design:

- A loaded `values.yaml` may hold plaintext passwords under keys the tool does not own.
  [SECRETS.md](../config-tool/SECRETS.md) rules 1 and 7 forbid any value in the profile.
- The UI runs in a browser tab or a Wails webview. Neither gives the page a file path, and the
  origin's port changes on every run ([ADR 0009](0009-config-tool-stack.md)).
- `values.yaml` is round-tripped byte for byte ([ADR 0013](0013-config-tool-roundtrip-byte-patches.md));
  the profile is a file the tool alone writes.

Out of scope: `NEXT_STEPS.md` and the export bundle (US-7.3), real secret fingerprints (Epic 8),
migration between bundle versions (US-9.3).

## Decision

### 1. Format

`.dscprofile.yaml` is encoded by `yaml.v3` from Go structs, not round-tripped. Struct field order
and sorted map keys fix the key order, `open` is sorted by field id, the indent is 2 spaces, and
a fixed header comment comes first. There is no timestamp and no tool version, so saving an
unchanged profile twice gives identical bytes. Hand-added comments are not kept. Decoding is
strict: unknown keys, trailing documents and a `format` other than the supported one are errors.

```yaml
# dsc-config profile: the answer set values.yaml is rendered from. It never holds a secret value.
format: 1
model:
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
  environment: development
role: provider
features:
  access.policies:
    enabled: true
    source: preset
  data.scorpio:
    enabled: false
    source: loaded
  # … one entry per offered feature
answers: {}
open:
  - field: target.context
source:
  file: values.yaml
  root: dsc
  sha256: 9f2c…(64 lowercase hex digits)
secrets: {}
```

| Field | Meaning |
|---|---|
| `format` | Schema version of the file, `1`. Any other number is refused. |
| `model` | The bundle the profile was built with: `chart`, `version` and `revision` (see 6). |
| `catalogue` | The bundle the role presets came from, same shape as `model`. Omitted when that is `model` itself (see 7). |
| `target` | `profile.Target` unchanged: `type` (`file`, `cluster`), `context` (the kube context *name* only), `namespace`, `createNamespace`, `releaseName`, `chart`, `chartVersion`, `environment` (`development`, `production`). |
| `role` | Role preset id (`consumer`, `provider`, `consumer-provider`, `operator`, `custom`); omitted until Stage 2 is answered (see 7). |
| `features` | Feature id → `{enabled, source}`, `source` one of `preset`, `loaded`, `user`. A feature the role does not offer is never enabled; `ApplyPreset` omits it (see 7). |
| `answers` | Answer id → scalar. A slot only: CT-4 has no answer ids, so it is always `{}`. |
| `open` | Fields deliberately left open, one mapping `{field}` per entry so a note can be added later. Field ids are `<section>.<key>`, e.g. `target.namespace`; the section names the stage. |
| `source` | The values file the overrides layer lives in: base name, root mapping (ADR 0014; always written, `""` is the whole file), SHA-256 of its bytes. Metadata only; omitted when nothing was loaded. |
| `secrets` | Epic 8: secret name → `{mode, keys, fingerprints}`, never a value. Always `{}` in CT-4. |

`answers` and `secrets` exist so the format does not change when they get producers. Until then
`Validate` refuses an unknown answer id, the same rule as for `open` ids; none exists, so every
key is refused. It also refuses any `secrets` entry ("secrets are recorded from Epic 8 on"). A
hand-edited or merged profile therefore cannot smuggle a value in, and open-then-save loses
nothing although the UI carries neither field. The epic that adds answer ids, and Epic 8 for
`secrets` (fingerprint `^sha256:[0-9a-f]{12,64}$`, one of the five SECRETS.md modes), lifts the
rule together with the UI carry-through.

### 2. Overrides are referenced, not copied

The profile records `source` and nothing of the file's content. Opening a profile reads
`<profile dir>/<source.file>` again with `source.root` and restores the overrides layer the
server keeps since ADR 0014. A profile is meant to sit next to its values file (US-7.2).

- **Which load is saved is the client's statement.** The server's last load survives the UI's
  *New configuration*, so it cannot decide alone. The load report carries `sha256`, and the
  `PUT` body sends it back as `sourceSha256`. The UI drops its report on *New configuration* and
  then sends none. `source` is attached only when `sourceSha256` is set **and** equals the
  hash of the server's current load. Empty means "no source", whatever the server holds. A
  hash that does not match (another tab loaded another file, or nothing is loaded) is a 422.
- **Save checks the neighbour.** The values file was uploaded as bytes, so the server does not
  know where it lives. With `source` attached, the save reads `<dir of path>/<source.file>`. A
  missing or different file does not fail the save, since the user may copy it afterwards, but
  the response carries the note "overrides will not be restored on reopen: <file> is not next
  to the profile" or "… differs from the loaded one".
- **`source.file` is untrusted**, since profiles travel through git. It must be a plain base
  name: `filepath.Base(f) == f`, no `/` or `\`, not `.` or `..`, not absolute. `source.sha256`
  must be 64 lowercase hex digits. Open can therefore only read a file in the profile's own
  directory.

On open, a changed hash restores the layer with a note, and a missing or unreadable file clears
it with a note saying that saving again drops the reference (the UI then holds no report).

The re-load on open restores the overrides layer only; the profile stays the state:

- **Bundle.** The file is classified against the profile's `model` bundle, passed to
  `load.Load` as an explicit option, not chosen from the file's header or as the latest one
  (ADR 0014 §5). Review then shows the classification of the bundle the profile renders with.
- **Answers.** `target`, `answers` and `open` come from the profile only. Nothing the re-load
  back-derives moves into them, so an empty `target.chartVersion` (an open question) stays empty
  whatever the file's header says (ADR 0014 §5, §6).
- **Root.** `source.root` is always written, never `omitempty`, and is passed as an explicit
  `Options.Root`. `""` means the whole file (ADR 0014 §7), never "detect".

### 3. The server reads and writes by absolute path

The page has no paths and ADR 0009 keeps native dialogs to the optional Wails window, so the UI
asks for a path in a text field, like the kubeconfig field, prefilled with
`<defaultDir>/<releaseName or "connector">.dscprofile.yaml`. Both endpoints refuse a path that is
not absolute or does not end in `.dscprofile.yaml`: the API cannot write arbitrary files, and
nothing resolves against the working directory, which is `/` or the binary's folder for a GUI
started from a launcher. An existing file is replaced only with `overwrite: true`, through a
temporary file in the same directory, fsync and rename, keeping the existing mode and refusing a
symlink (core package `internal/atomicfile`). New profiles get mode 0644; they hold no secrets.

`defaultDir` is the directory of the newest recent entry, else `os.UserHomeDir()`, and the
working directory only when there is no home directory.

### 4. The recent list lives in the user config directory

`recent.yaml` in `os.UserConfigDir()/dsc-config` (directory 0700, file 0600), or in
`$DSC_CONFIG_HOME` when that is set. The override is one variable on every OS, so tests and the
e2e run of the real binary never touch the developer's list. Each entry holds the absolute path,
the name (base name without the suffix), the chart version and `savedAt` (RFC 3339, UTC); a
`role` slot stays empty until Epic 2. The list is newest first, deduplicated by cleaned path,
capped at 10, and drops entries whose file is gone when it is listed. A missing store is an empty
list. A failing or corrupt store never fails a save, an open or `GET /api/profile/recent`; it
becomes a note. The recent route then answers 200 with `entries: []`, the note, and a
`defaultDir` from the home-directory fallback (3).

### 5. An open question is unanswered or marked, counted once

A field is an open question when it is unanswered **or** listed in `open`; the count is the set
union, so a marked empty field counts once. Marking keeps any value as provisional, unmarking
keeps the value, and a marked empty field is open, not an error. The count feeds the existing
stage rail and footer, and Review lists every open question with a link to the field.

Per-field format errors are stage errors, not save errors. A half-finished configuration, say a
namespace `Prod_NS` or a 60-character release name, is saved and reopened as typed, and the
Target stage shows the error again. Save and open refuse only **structural** errors: `format`,
`model`, unknown values of `target.type` and `target.environment`, unknown or duplicate `open`
ids, `source`, `answers` and `secrets`. `model` must agree with `target`: `model.chart` equals
`target.chart`, and `model.version` equals `target.chartVersion` when that is set. An empty
`target.chartVersion` never moves `model`:

- **Open** checks `model` only against the shipped bundles (rule 6), never against "latest". A
  profile saved with `model.version: 10.16.0` and no chart version still opens once 10.17.0
  ships.
- **Save** takes the bundle of `target.chartVersion` when that is set. Otherwise it keeps the
  `model` of the PUT body when that bundle is shipped **and its chart is `target.chart`**; the UI
  holds it from the open and drops it on *New configuration*. Otherwise, as for a profile that has
  never named a bundle, it takes the latest bundle of `target.chart`, the same rule as the load. A
  chart switch in the Target stage clears the chart version, so this keeps save from refusing its
  own result under the agreement rule above. The revision written is the shipped one, which rule 6 announces with a
  note on open.

So `model.version` changes only when the user sets the chart version. A bundle migration is
never implicit (US-9.3). The save does not fill in `target.chartVersion` itself: that would answer
an open question for the user.

The UI follows the same rule. Today the Target stage fills an empty `chartVersion` with the
latest shipped version as soon as it renders. From CT-4 on it does so only while the state holds
no `model` whose chart is `target.chart`, which means a new configuration or a chart switch.
After *Open profile* the state holds the opened `model`, so an empty chart version stays empty and
counts as unanswered (an open question) until the user picks one. Following a Review link to
Target therefore cannot move the bundle. *New configuration* drops the held `model`, and the
auto-fill applies again.

`generate --profile` renders and so needs a complete target; it also refuses the format and "is
required" errors.

### 6. Model bundle version

A bundle is identified by `(chart, version, revision)`; `bundle.yaml` gains `revision: 1`, and
a bundle without one fails to load. Opening a profile whose `chart@version` is not shipped, or
whose revision is newer than the shipped one, is refused with a pointer to US-9.3. An older
revision opens with a note.

### 7. Role and features

> Partly superseded by [ADR 0018](0018-config-tool-deliberate-choices.md): an enabled not-offered feature
> is refused only with `source: preset`, and an enabled feature of another chart from any source.

`role`, `features` and `catalogue` are optional keys, so they stay in format `1`: profiles written
before Epic 2 still decode. `Validate` refuses an unknown role, `features` without a role, an
unknown feature id or source, an enabled feature the role marks `not-offered`, and a role whose
chart differs from `target.chart`; save and open answer those with 422.

Presets are read through `model.RoleCatalogue` (ADR 0017): the `model` bundle when it declares
roles, else a fallback bundle. A trust-anchor profile has no roles of its own, so its presets come
from the latest DSC bundle. Because that bundle is not `model`, the profile pins it in `catalogue`,
and `Validate` reads roles and features from the pinned bundle, never from "whatever is newest
now". `catalogue` follows the rules of 6 (not shipped or newer revision: 422, older revision: a
note) and must declare roles. The server owns it on save, as it owns `model`: without a role, or
when `model` declares the roles, it is dropped; else a held catalogue that is shipped, declares
roles and is of the fallback's chart is kept with the shipped revision, and otherwise the fallback
is written. `ApplyPreset` and `Validate` use the same rule (`profile.CatalogueBundle`), so a
preset is always validated against the bundle it was built from. On open, a `catalogue` the rule
would not keep (one next to a `model` that declares roles, or of another chart) is a 422.

`profile.ApplyPreset` is pure. It writes `{enabled: true, source: preset}` for a recommended
feature and `{enabled: false, source: preset}` for an available one, and omits the rest. Toggles
back-derived from the loaded values file (ADR 0014, `<featureId>.enabled` at stage `features`) win
over the preset, as `source: loaded`, so the profile never contradicts the `source` it references.
A loaded `false` on a not-offered feature is omitted. A loaded `true` on a not-offered feature
refuses the role and names the features. US-2.3 re-applies a preset only to `source: preset`
features; it treats `loaded` like `user`.

A role of another chart (Operator deploys `trust-anchor`) switches the target chart. The switch is
refused while a `source` is set, because the load report and its answers are classified against
the current chart's bundle. Otherwise it sets `target.chart`, clears `target.chartVersion` and
sets `model` to the new chart's latest bundle as a placeholder. Stage 1 then leaves the version
empty instead of preselecting latest, so a version the user chose is never replaced by a guess.
The reverse holds too: a chart change on Stage 1 clears `role`, `features` and `catalogue` and
tells the user why, since every role belongs to exactly one chart.

### 8. Infrastructure

Stage 4 answers (ADR [0026](0026-config-tool-infrastructure-modes.md)) sit in an optional key, so
the format stays `1`:

```yaml
infrastructure:
  certManager:
    mode: cluster
    answers:
      issuer: letsencrypt
```

Answers are keyed by local question id; a per-ingress question is `<question>.<ingressId>`. Field
ids are `infrastructure.<dep>` for the mode and `infrastructure.<dep>.<question>` for an answer,
both in `FieldError`s and in `open`. `Validate` refuses an unknown dependency, a mode it does not
offer, a question none of its modes declares, an answer to a `readOnly` question (its derived
value must apply) and a value the question's type refuses. The
compiled `infra.*` ids are never answers of their own: `answers.infra.*` is refused, as an answer
and as an open field. Answers of inactive modes stay, so switching back restores them; only the
active mode renders. A chart switch drops the whole map and its open questions. Like every other
answer, an infrastructure answer names a Secret or an issuer, never a credential.

### API

All three routes sit behind the per-run token and the 8 MiB body cap. Errors use the existing
body `{error}`; a 422 for an invalid profile adds `fields: [{field, message}]`. The
`GET /api/profile` stub is dropped: reading a profile replaces the server's overrides layer, so
it is a `POST`.

**`PUT /api/profile`** saves.

```json
{"path": "/home/pia/dsc/provider.dscprofile.yaml", "overwrite": false,
 "profile": {"model": {…as opened…}, "target": {…}, "open": [{"field": "target.context"}]},
 "sourceSha256": "9f2c…"}
```

`profile.model` is optional and only read while `target.chartVersion` is empty (5). The server
fills in `format`, `model` (rule 5), `catalogue` (rule 7), `source` (rule 2) and an empty `secrets`, validates, writes and adds the entry to the recent list.
Response 200:

```json
{"path": "/home/pia/dsc/provider.dscprofile.yaml", "profile": {…the profile as written…},
 "notes": ["overrides will not be restored on reopen: values.yaml is not next to the profile"]}
```

**`POST /api/profile/open`** reads, validates, restores `source` and adds the entry to the recent
list. Body `{"path": "…"}`. Response 200 `{path, profile, report?, notes}`: `report` is the
ADR 0014 load report when the source was restored, classified against the profile's `model`
with its recorded root (2). Notes cover a changed, missing or unreadable
source, an older bundle revision of `model` or `catalogue`, and a failing recent list.

**`GET /api/profile/recent`** returns
`{"entries": [{"path", "name", "chartVersion", "role", "savedAt"}], "defaultDir": "/home/pia/dsc", "notes": []}`.
It never fails on the store: an unreadable or corrupt store gives `entries: []` and a note (4).

| Status | When |
|---|---|
| 400 | Body is not valid JSON or has trailing data. |
| 404 | Open: the file does not exist. |
| 409 | Save: the file exists and `overwrite` is not set. |
| 413 | Body above the 8 MiB cap. |
| 422 | Path not absolute or without the `.dscprofile.yaml` suffix; file not a valid profile (strict decode); structurally invalid profile (5, 7), including `model` not agreeing with `target` and an unknown role; bundle not shipped or revision newer (6); save with a `sourceSha256` that does not match the current load; save into a directory that does not exist; save onto a symlink (3). |
| 500 | Any other I/O error, such as permission denied, disk full, or an unreadable file on open. The body carries the OS error. |

Per-field format errors in `target` never give a 422, on save or on open. Save never creates a
missing parent directory: a mistyped path should fail visibly instead of creating a tree.
(The recent store does create its own directory.) The UI shows every error status in the same
place, whatever the code.

### `generate` and existing output files

ADR 0013 said the CLI wires replacing an existing file in CT-4. CT-4 adds the plumbing only
(`Parse` the existing file, apply an edit, `Write`, atomic replace keeping the mode).
`values.yaml = render(profile) ⊕ overrides` with overrides winning, and ADR 0014's "do not touch
what the tool did not write", need the render output and its owned markers, which do not exist
yet. Until the render epic decides that precedence, `generate` keeps refusing an existing output
file. CT-4's save likewise writes only `source` into the profile (rule 2), never the overrides layer
back into a values file. This amends ADR 0013 and ADR 0014 (§1, §6).

## Consequences

- A profile can be committed as is: it holds answers, markers and metadata, never a value from
  the loaded file and never a secret.
- Reopening restores the overrides only while the values file sits next to the profile with the
  recorded name; moving one without the other loses the layer, with a note rather than silently.
- Profile bytes are stable, so git diffs show only real changes; hand comments are lost.
- The user types or confirms paths, the price of having no file dialog in a browser.
- `answers` and `secrets` are refused until their epics; those epics change `Validate` and the UI
  together.

## Alternatives

- **Embed the overrides in the profile.** Copies plaintext passwords from unknown keys into a
  file meant for git, which breaks SECRETS.md rule 1, and loses the byte-faithful round-trip.
- **Browser download and upload.** No path, so no recent list, no in-place save, and no way to
  find the values file next to the profile.
- **Browser localStorage for the recent list.** The origin's port changes every run, so the list
  would be empty each time.
- **A separate open-question counter next to the unanswered count.** A marked empty field would
  be counted twice.
- **Round-trip the profile through `internal/roundtrip`.** Keeps hand comments but makes the
  output depend on history; the profile is the tool's own file, and determinism matters more.
- **Refuse saving with format errors.** Loses half-finished work, the case saving is for.
