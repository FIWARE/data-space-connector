# Implementation Plan: [config-tool] CT-4 Save and resume — the profile as the tool's state

## Overview
This makes the answer set (`.dscprofile.yaml`) the persistable state of `dsc-config`. The work covers:
- a deterministic, secret-free profile format in the core;
- save and open through the loopback API, plus a recent list kept in the user config directory;
- a deliberate open-question marker per field, counted in the stage rail and listed on Review;
- the *Open profile* card and the *Recent* list on S0.

Out of scope, as the ticket says: `NEXT_STEPS.md` and the export bundle (US-7.3), real secret fingerprints (Epic 8) and migration between bundle versions (US-9.3).

### Design decisions (fixed by Step 1's ADR, followed by every later step)
1. **Format.** `.dscprofile.yaml` is written by `yaml.v3` from Go structs. The output is deterministic: the struct fields and sorted map keys fix the key order, `open` is sorted by field id, and it is indented with 2 spaces. A fixed header comment comes first. The file has no timestamps and no tool version, so saving twice without changes gives identical bytes. Decoding is strict (`KnownFields(true)`), and a `format` number gates the schema. The profile belongs to the tool, so hand-added comments are not kept. This differs on purpose from `values.yaml`, which goes through `internal/roundtrip`. Shape:
   ```yaml
   # dsc-config profile: the answer set values.yaml is rendered from. It never holds a secret value.
   format: 1
   model: {chart: data-space-connector, version: 10.16.0, revision: 1}   # bundle it was built with
   target: {type, context, namespace, createNamespace, releaseName, chart, chartVersion, environment}
   answers: {}        # answer id -> scalar; later epics fill it
   open:              # deliberately unanswered fields, sorted; a mapping per entry leaves room for a note later
     - field: target.context
   source:            # the values file the overrides layer lives in; metadata only, omitted when none
     file: values.yaml
     root: dsc
     sha256: <hex>
   secrets: {}        # Epic 8: secretName -> {mode, keys, fingerprints}; never a value
   ```
2. **Overrides are referenced, not copied.** A loaded `values.yaml` may hold plaintext passwords in keys the tool does not own. Copying them into the profile would break SECRETS.md rule 1. The profile therefore records only `source` (base name, root, SHA-256 of the file). On open by path, the server reads `<profile dir>/<source.file>` again with the recorded root, which restores the overrides layer (`server.loaded`). The profile is meant to sit next to its values file (US-7.2). A changed hash restores the layer with a note; a missing file gives a notice on Review.
   - **Which load is saved is the client's statement, not server state.** `s.loaded` is server-global and survives the UI's *New configuration*. The `PUT` body therefore carries `sourceSha256`, the `sha256` of the load report the UI currently holds; the UI drops its report on `newConfiguration`, so that request carries none. The server attaches `source` only when `sourceSha256` is set **and** equals `s.loaded`'s hash. Empty means "no source", even when `s.loaded` is still filled. A hash that no longer matches `s.loaded` (another tab loaded a different file) is refused with 422, so a profile never references a file the user did not mean.
   - **Save checks the neighbour.** The values file is uploaded as bytes, so the server does not know where it lives on disk. When `source` is attached, the save reads `<dir of path>/<source.file>` and compares its SHA-256. A missing or different file does not fail the save (the user may copy the file afterwards), but the response carries the note "overrides will not be restored on reopen: <file> is not next to the profile" (or "... differs from the loaded one"), and the UI shows it right away.
   - **`source.file` is untrusted.** Profiles are shared through git. `Validate` requires a plain base name (`filepath.Base(f) == f`, no `/` or `\`, not `.` or `..`, not absolute), so open can only read a file in the profile's own directory.
3. **The server picks the path.** `PUT /api/profile` writes to a path the user gives, and `POST /api/profile/open` reads one. The browser has no file paths, and ADR 0009 keeps native dialogs to the optional Wails window. The UI uses a path text field (the precedent is the kubeconfig field), with a default `<dir>/<releaseName|connector>.dscprofile.yaml`. Both endpoints refuse any path that does not end in `.dscprofile.yaml`, so the API cannot write arbitrary files. An existing file is replaced only with `overwrite: true`, through an atomic temp-file-and-rename that keeps the file mode. Profiles are written with mode 0644 (they hold no secrets).
4. **The recent list** lives at `os.UserConfigDir()/dsc-config/recent.yaml` (dir 0700, file 0600). It holds absolute paths and metadata only: name, chart version, saved-at. It is newest first, deduplicated by path, capped at `MaxRecent = 10`, and drops entries whose file no longer exists when it is listed. It does not use browser localStorage: the origin's port changes on every run, so localStorage would be empty each time. A failing recent list never fails a save; it comes back as a note. `DSC_CONFIG_HOME` overrides the directory, so tests of the real binary use a temp dir.
5. **Marker vs. unanswered.** A field is an *open question* when it is unanswered **or** marked open, and it is counted once (set union), so nothing is counted twice. Marking keeps any value as provisional; unmarking keeps the value. A marked empty field is open, not an error, and format errors still count as errors. Field ids are `<section>.<key>` (`target.namespace`, ...); the section maps to the stage.
6. **Model bundle version** = `(chart, version, revision)`. `bundle.yaml` gains `revision: 1`. Opening a profile whose bundle `chart@version` is not shipped, or whose revision is newer than the shipped one, is refused with a pointer to US-9.3. An older revision opens with a note.

## Steps

### Step 1: ADR 0015 — profile format, save/open mechanism, recent list, open-question marker
Docs-only step, so the design is reviewed before any code.
- Add `doc/adr/0015-config-tool-profile.md` in the format of ADRs 0013/0014 (Context, Decision, Consequences, Alternatives). It records decisions 1–6 above:
  - rejected alternatives: embedding overrides (secret leak), browser download and upload (no path, so no recent list and no source resolution), localStorage (the port changes per run), a separate open-question counter (double counting);
  - the API: `PUT /api/profile` and `POST /api/profile/open`, which replaces server state and so is not a `GET`, plus `GET /api/profile/recent`. The `GET /api/profile` stub is dropped;
  - the three `source` rules of decision 2: `sourceSha256` in the `PUT` body, the save-time neighbour check with its notes, and the base-name rule for `source.file`;
  - the `DSC_CONFIG_HOME` override of the recent-list directory (decision 4, Step 3).
- Update `doc/config-tool/TECH_STACK.md` (API box: `GET/PUT /api/profile` → the three routes above). Add a row to the `doc/adr/README.md` table and extend its "0009–0014 belong to config-tool" sentence to 0015.
- Acceptance: the ADR names every field of the format with an example, the three endpoints with their request and response bodies, and status codes (400 bad body, 404 missing file, 409 exists without overwrite, 422 invalid profile, path suffix or bundle).

### Step 2: Core profile format and atomic file writes
Files: `config-tool/internal/profile/` (new `format.go`, `file.go`, tests, `testdata/`), new core package `config-tool/internal/atomicfile/`, `config-tool/internal/model/model.go`, `config-tool/model/10.16.0/bundle.yaml`.
- `model.Bundle` gains `Revision int` (`yaml:"revision"`), and the bundle file sets `revision: 1`. Loading a bundle without a revision is an error, and `model_test.go` covers that.
- `profile.Profile` is extended with `Format`, `Model` (`ModelRef{Chart, Version, Revision}`), `Target` (unchanged), `Answers map[string]any`, `Open []OpenQuestion{Field}`, `Source *Source{File, Root, SHA256}` and `Secrets map[string]SecretRef`. `SecretRef` is `{Mode, Keys []string, Fingerprints map[string]string}`.
  - JSON and YAML tags match. Every exported identifier gets a one-line GoDoc.
  - Named constants: `FormatVersion = 1`, `FileSuffix = ".dscprofile.yaml"`, the header comment, and the field-id prefix `target.`.
- `Marshal(Profile) ([]byte, error)` writes header + yaml.v3 encoding with indent 2 and `open` sorted. `Unmarshal([]byte) (Profile, error)` decodes strictly: unknown keys, a wrong `format` and trailing documents are errors.
- `(Profile) Validate(bundles []model.Bundle) []FieldError`:
  - runs `Target.Validate`, without the "is required" errors, because a saved profile may be half finished;
  - checks `Model` and `Target.Chart`/`ChartVersion` against the bundles, which resolves the TODO in `Target.Validate`;
  - refuses newer revisions and duplicate or unknown open field ids (known today: the `target.*` field names);
  - requires `source.file`, when `source` is set, to be a plain base name (decision 2) and `source.sha256` to be 64 lowercase hex digits;
  - checks that every fingerprint matches `^sha256:[0-9a-f]{12,64}$` and every mode is one of the five SECRETS.md modes (`chart`, `reference`, `generate+apply`, `sealed`, `external`, as named constants). A value therefore cannot be stored under `secrets`.
- `ReadFile(path)` and `WriteFile(path, Profile, overwrite bool)` both refuse a path without `FileSuffix`. `WriteFile` uses `atomicfile.Write`.
- `atomicfile.Write(path, data, mode, overwrite)`: temp file in the same directory, fsync, rename. It keeps an existing file's mode and refuses a symlink target. Without overwrite, an existing path gives `ErrExists`.
- Tests, table-driven where possible:
  - save → load → save gives identical bytes, for a full fixture (`testdata/full.dscprofile.yaml`, golden) and an empty one;
  - strict-decode failures;
  - `Validate` cases, including `source.file` values `../../etc/passwd`, `/abs/values.yaml`, `sub/values.yaml`, `sub\values.yaml`, `.`, `..` (refused) and `values.yaml` (accepted);
  - **no secret value**: a reflection walk asserts `SecretRef` has exactly the fields Mode/Keys/Fingerprints, and `Validate` refuses a fingerprint that is a plain value;
  - atomicfile: mode kept, `ErrExists`, symlink refused, no temp file left behind on error.
- `internal/layering_test.go` keeps passing: no `net/http` in the core.

### Step 3: Recent-profiles store
Files: new core package `config-tool/internal/recent/` (`recent.go`, `recent_test.go`).
- `Store{Dir string; Now func() time.Time}` with `DefaultDir()`, which is `$DSC_CONFIG_HOME` when that is set, else `os.UserConfigDir()/dsc-config`, and an error when there is neither. The env override (named constant `ConfigHomeEnv`) is how the e2e run (Step 7) and any other test of the real binary keep off the developer's real list; it is one variable on every OS, unlike faking `XDG_CONFIG_HOME`, `HOME` and `AppData`.
- `Entry{Path, Name, ChartVersion, SavedAt}`:
  - `Name` is the file base name without `.dscprofile.yaml`;
  - `SavedAt` is RFC 3339 UTC;
  - leave room for `Role`, filled in Epic 2: `omitempty`, empty today.
- `Add(Entry) error`: moves the path to the front, deduplicates by cleaned absolute path, caps at `MaxRecent = 10` and writes through `atomicfile` with mode 0600, creating the dir with 0700.
- `List() ([]Entry, error)` drops entries whose file is gone and treats a missing store file as empty. A corrupt store file is an error the caller turns into a note.
- Parameterized tests: order, dedup, cap, missing files filtered, corrupt file, permissions (skipped on Windows), relative paths made absolute, `DefaultDir` with and without `DSC_CONFIG_HOME` (`t.Setenv`).

### Step 4: Server API — save, open, recent
Files: `config-tool/internal/server/api.go`, `server.go`, new `profile_test.go`, `config-tool/internal/load/load.go`.
- `load.Report` gains `SHA256 string` (`json:"sha256"`) of the source bytes; `load.Load` fills it. It is on the report so the UI holds it and can name the load it saves.
- `Server` gains `recent *recent.Store`, set in `New` from `recent.DefaultDir()`, or nil when that fails (the list stays empty). Tests swap it, as they do with `models`.
- `PUT /api/profile` with body `{path, overwrite, profile, sourceSha256?}`:
  - the server fills in `format`, `model` (the bundle of `target.chartVersion`, else the latest one, the same rule as `load.chooseBundle`) and an empty `secrets`;
  - `source` (name, root, SHA-256 of `s.loaded`) only when `sourceSha256` is set and equals `s.loaded.Report.SHA256`; without it there is no `source`, whatever `s.loaded` holds (decision 2);
  - then it validates, writes and adds the profile to the recent list;
  - with a `source`, it reads `<dir of path>/<source.file>` and adds the "not next to the profile" or "differs from the loaded one" note when the file is missing or its hash differs;
  - response `{path, profile, notes}`;
  - errors: 409 when the file exists without overwrite, 422 for a bad suffix, a non-absolute path, an invalid profile, or a `sourceSha256` that does not match `s.loaded` (or nothing is loaded).
- `POST /api/profile/open` with body `{path}`:
  - reads and validates the profile, adds it to the recent list;
  - resolves `source` next to the profile and re-runs `load.Load` with the recorded root. On success it replaces `s.loaded`; otherwise it clears `s.loaded`;
  - response `{path, profile, report?, notes[]}`. A note comes for a source that is changed, missing or unreadable, an older bundle revision, or a failing recent list;
  - when the source cannot be restored, the note also says that saving again drops the reference, because the UI then holds no report and sends no `sourceSha256`;
  - errors: 404 missing file, 422 invalid profile.
- `GET /api/profile/recent` returns `{entries, defaultDir}`. `defaultDir` is the directory of the newest entry, else the process working directory.
- Remove the `GET`/`PUT /api/profile` `notImplemented` stubs and the now-unused `ticketProfile` constant.
- Tests (httptest, table-driven):
  - save, then open, then save gives identical bytes on disk;
  - 409/422/404 paths;
  - the source is restored after a load, and the changed/missing notes appear;
  - stale slot: load a file, then save without `sourceSha256` (what the UI sends after *New configuration*), and assert the written profile has no `source`; save with a hash that is not the loaded one gives 422;
  - neighbour check, table-driven: values file next to the profile with the same bytes (no note), missing (note), different bytes (note); the save succeeds in all three;
  - open refuses a profile whose `source.file` is `../x.yaml` with 422 and reads nothing;
  - the recent list gets the entry;
  - **end-to-end no-secret test**: POST a values file holding `keycloak.adminPassword: s3cr3t-<random>` (and a password under an unknown key), save the profile, and assert the file bytes contain neither value.

### Step 5: CLI — `generate` reads the profile, replaces existing output safely
Files: `config-tool/internal/cli/cli.go`, `cli_test.go`.
- `generate --profile`:
  - reads the file with `profile.ReadFile` and validates it against the embedded bundles before calling `render.Render`;
  - errors name the file and the field. The `// Loading the profile file arrives with CT-4` stub goes away.
- `writeOutput` resolves the `TODO(CT-4)` (ADR 0013):
  - an existing output file goes through `roundtrip.Parse` → `Merge` → `Write` and is replaced via `atomicfile.Write` (overwrite, keeping the mode);
  - a symlink is still refused;
  - a new file is created exclusively as today.
- Parameterized tests:
  - missing profile, invalid profile, unknown bundle;
  - existing output merged byte-faithfully: comments are kept and the mode is kept;
  - new output, and stdout.
  - Render is still `ErrNotImplemented`, so test `writeOutput` directly with fixed data.

### Step 6: UI — open-question markers, rail count, Review list with links
Files: `config-tool/ui/src/state/` (`answers.ts`, `reducer.ts`, new `openQuestions.ts` + test, `target.ts`, `issues.ts`), `stages/Target.tsx`, `stages/Review.tsx`, tests next to each.
- `AppState` gains `open: readonly FieldId[]` (sorted, the profile's `open`) and `focus?: FieldId`.
  - `FieldId` is `` `target.${TargetField}` ``; `fieldStage(id)` maps the prefix to `StageId`; a label map gives Review its text.
  - New actions: `toggleOpen {field}`, `focusField {field}` (does `goTo` the field's stage + sets `focus`), and `focusDone`.
  - `newConfiguration` and `fileLoaded` reset `open`.
- `openQuestions(state)` returns the sorted union of unanswered fields (the `MSG_REQUIRED` errors) and marked fields, each `{field, stage, label, marked}`. `targetIssues` counts `open` from that union for the Target stage, and marked empty fields are not errors. Rail and footer pick this up unchanged through `IssueSelector`.
- Target stage: each field row gets a small toggle button, "Ask later" with `aria-pressed`, in the style of the existing controls. It carries `id="field-target.<name>"` on the input. When `state.focus` names one of its fields, it focuses and scrolls to it, then dispatches `focusDone`.
- Review: a new "Open questions (N)" section, always shown above the load report or the placeholder. It lists each open question with its stage, a "marked" badge, and a "go ↗" button that dispatches `focusField` (MOCKUPS S7 wording). When there are none, it says so.
- Vitest/RTL tests:
  - union counting, parameterized: unanswered only, marked only, both — counted once;
  - toggle keeps the value; the rail shows ○ with the count;
  - the Review link lands on Target and focuses the input.

### Step 7: UI — save profile, open profile, recent list on S0, reopen lands on Review
Files: new `config-tool/ui/src/api/profile.ts` + test, `state/reducer.ts`, `stages/Start.tsx`, new `chrome/SaveDialog.tsx`, `chrome/Rail.tsx`, `stages/Review.tsx`, `state/title.ts`, `ui/e2e/smoke.spec.ts`, tests.
- `api/profile.ts`: typed `saveProfile`, `openProfile` and `fetchRecent` over the Step 4 bodies. `saveProfile` sends `sourceSha256 = state.loaded?.sha256`, so after `newConfiguration` (which already clears `loaded`) no source is referenced. It also holds the `Profile` TS type mirroring Go and `toProfile(state)` / `fromProfile(profile)` (answers.target + open). The `ApiError` 409 is surfaced as "replace?".
- `AppState` gains `profilePath?`, `profileNotes?`. New action `profileOpened {path, profile, report?, notes}`: `startOver` on Review with `answers`, `open`, `loaded = report` and `loadedFile = undefined`, so the root picker is disabled because the recorded root is used. New action `profileSaved {path, notes}`.
- Save:
  - Rail `⌘S Save profile` is enabled once a configuration is entered, and there is a "Save profile" button on Review (MOCKUPS S7) as well. Wire the ⌘S/Ctrl+S shortcut.
  - With a known `profilePath` it saves in place (overwrite). Otherwise `SaveDialog` asks for a path, prefilled with `defaultDir/<releaseName || 'connector'>.dscprofile.yaml`. When a values file is loaded, it shows the hint "save next to <source> so reopening finds its overrides". A 409 asks to confirm a replace.
  - The save response's `notes` (the neighbour check of Step 4) are shown at once: in the dialog when it is open, else as `profileNotes` on Review, so a profile that will reopen without its overrides is flagged the moment it is saved.
- S0: the *Open profile* card replaces the "coming in CT-4" card and opens a path field (with the same suffix hint). Below the cards comes the *Recent* list from `fetchRecent`: `name · chart <version> · <relative time>`, with the role once Epic 2 provides it. A click opens that profile. Failures stay on S0 with a message, as the Open values.yaml card does. Remove `'Open profile'` from `COMING` and update `Start.test.tsx`.
- Review shows `profileNotes` (source changed, missing, older revision). The title bar shows the profile name.
- Tests:
  - reducer: `profileOpened` restores answers and markers exactly and lands on Review;
  - Start: recent list rendering and click-to-open, open failure;
  - SaveDialog: default path, 409 confirm, a save note is shown;
  - api/profile: `sourceSha256` is sent from the held report and omitted after `newConfiguration`;
  - Rail Save enabled state;
  - one Playwright smoke path (`ui/e2e`): new configuration → mark a field open → save to a temp path → back to S0 → the recent entry → Review shows the open question.
  - e2e isolation: `ui/e2e/global-setup.ts` creates a temp dir with `mkdtemp`, starts the binary with `DSC_CONFIG_HOME` set to it (next to `KUBECONFIG`) and removes it in the teardown; the profile is saved into another temp dir, exported to the spec through `env.ts`. The developer's real `recent.yaml` is never touched.

### Step 8: Documentation and context
Files: `config-tool/README.md`, `doc/config-tool/USER_JOURNEY.md` (Stage 0 *Open a profile* row: path field + recent list, lands on Review), `doc/config-tool/FEATURE_MODEL.md` (*Concepts*: `overrides` is referenced via `source`, not copied, linking ADR 0015), `AGENTS.md`.
- README: the profile format with an example, where profiles and the recent list live, the save and open flow, the "keep the profile next to its values file" rule, and the open-question marker. Also the `generate --profile` behaviour and the replacement of existing output.
- AGENTS.md: replace the "CT-3 ... `profile.Profile` stays untouched" note with a short CT-4 bullet covering:
  - the format is struct-encoded and strict, unlike values.yaml;
  - no values are written under `secrets` and overrides are not copied;
  - the endpoints;
  - `recent` lives in the user config dir;
  - the open count is the union of unanswered and marked.
- Run `.github/scripts/check-links.sh` for the touched docs, then `make test` and `make lint` from `config-tool/`.
