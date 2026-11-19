# 0015 - The profile as the tool's state: format, save and open by path, recent list, open questions

**Status:** Proposed
**Date:** 2026-11-19
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md)

## Context

CT-4 (#119) makes the answer set, `.dscprofile.yaml`, the state the user saves and resumes
(US-1.3 and US-7.2 of [`USER_STORIES.md`](../config-tool/USER_STORIES.md)): human-readable,
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
| `target` | `profile.Target` unchanged: `type` (`file`, `cluster`), `context` (the kube context *name* only), `namespace`, `createNamespace`, `releaseName`, `chart`, `chartVersion`, `environment` (`development`, `production`). |
| `answers` | Answer id → scalar. A slot only: CT-4 has no answer ids, so it is always `{}`. |
| `open` | Fields deliberately left open, one mapping `{field}` per entry so a note can be added later. Field ids are `<section>.<key>`, e.g. `target.namespace`; the section names the stage. |
| `source` | The values file the overrides layer lives in: base name, root mapping (ADR 0014), SHA-256 of its bytes. Metadata only; omitted when nothing was loaded. |
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
list. A failing or corrupt store never fails a save or an open; it becomes a note.

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
`target.chart`, and `model.version` equals `target.chartVersion` when that is set. When it is
empty, the save uses the latest bundle, the same rule as the load. `generate --profile` renders
and so needs a complete target; it also refuses the format and "is required" errors.

### 6. Model bundle version

A bundle is identified by `(chart, version, revision)`; `bundle.yaml` gains `revision: 1`, and
a bundle without one fails to load. Opening a profile whose `chart@version` is not shipped, or
whose revision is newer than the shipped one, is refused with a pointer to US-9.3. An older
revision opens with a note.

### API

All three routes sit behind the per-run token and the 8 MiB body cap. Errors use the existing
body `{error}`; a 422 for an invalid profile adds `fields: [{field, message}]`. The
`GET /api/profile` stub is dropped: reading a profile replaces the server's overrides layer, so
it is a `POST`.

**`PUT /api/profile`** saves.

```json
{"path": "/home/pia/dsc/provider.dscprofile.yaml", "overwrite": false,
 "profile": {"target": {…}, "open": [{"field": "target.context"}]},
 "sourceSha256": "9f2c…"}
```

The server fills in `format`, `model` (the bundle of `target.chartVersion`, else the latest),
`source` (rule 2) and an empty `secrets`, validates, writes and adds the entry to the recent list.
Response 200:

```json
{"path": "/home/pia/dsc/provider.dscprofile.yaml", "profile": {…the profile as written…},
 "notes": ["overrides will not be restored on reopen: values.yaml is not next to the profile"]}
```

**`POST /api/profile/open`** reads, validates, restores `source` and adds the entry to the recent
list. Body `{"path": "…"}`. Response 200 `{path, profile, report?, notes}`: `report` is the
ADR 0014 load report when the source was restored. Notes cover a changed, missing or unreadable
source, an older bundle revision, and a failing recent list.

**`GET /api/profile/recent`** returns
`{"entries": [{"path", "name", "chartVersion", "savedAt"}], "defaultDir": "/home/pia/dsc"}`.

| Status | When |
|---|---|
| 400 | Body is not valid JSON or has trailing data. |
| 404 | Open: the file does not exist. |
| 409 | Save: the file exists and `overwrite` is not set. |
| 413 | Body above the 8 MiB cap. |
| 422 | Path not absolute or without the `.dscprofile.yaml` suffix; file not a valid profile (strict decode); structurally invalid profile (5), including `model` not agreeing with `target`; bundle not shipped or revision newer (6); save with a `sourceSha256` that does not match the current load. |

Per-field format errors in `target` never give a 422, on save or on open.

### `generate` and existing output files

ADR 0013 said the CLI wires replacing an existing file in CT-4. CT-4 adds the plumbing only
(`Parse` the existing file, apply an edit, `Write`, atomic replace keeping the mode).
`values.yaml = render(profile) ⊕ overrides` with overrides winning, and ADR 0014's "do not touch
what the tool did not write", need the render output and its owned markers, which do not exist
yet. Until the render epic decides that precedence, `generate` keeps refusing an existing output
file. This amends ADR 0013.

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
