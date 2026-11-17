# Implementation Plan: [config-tool] CT-3 Open an existing values.yaml — classification and load report

## Overview
`dsc-config` learns to open an existing `values.yaml`. Each leaf is classified as owned, recognized
or unknown. Recognized answers are back-derived onto their stages, and unknown keys are carried along
untouched as overrides. The user lands on Review, which shows a load report. Everything builds on the
CT-2 engine (`config-tool/internal/roundtrip`) and follows one rule: **anything the tool did not write,
it does not touch.** The source bytes stay the truth (ADR 0013). This ticket adds no rendering and
writes nothing to disk; CT-4 wires saving.

## Design decisions (fixed here, recorded in ADR 0014 in Step 1)

1. **Load input is the file's bytes, uploaded by the browser.**
   - `<input type="file">` gives the UI the content but never a path, in both the browser and the
     Wails webview.
   - The UI sends `POST /api/load?name=<basename>` with `Content-Type: application/yaml`. The cap is
     the existing `maxBodyBytes` (8 MiB), so a larger file gives 413.
   - The server reads no new paths. Choosing where to save is CT-4's concern.
2. **The owned marker.**
   - Syntax: a comment line `# dsc-config:owned <id>[,<id>...]` in the head comment of a mapping key.
   - Scope: the marker owns that key's whole subtree and every following sibling key of the same
     mapping. The scope ends at the first blank source line between siblings, at the next sibling
     that has a head comment of its own, or at the end of the mapping.
   - Markers count only when the file carries the generated header (`roundtrip.Header`, `HeaderPrefix`
     on line 1). A file without the header was not written by the tool, so nothing in it is owned.
   - A marker anywhere else (on a sequence item, or in a line or foot comment) is ignored.
3. **Recognition rules are data in the bundle**, not code. Each rule has these fields:
   - `path`: exact, `roundtrip.ParsePath` syntax, no wildcards yet.
   - `answer`: an answer id such as `domain.base` or `identity.did.enabled`.
   - `stage`: one of the UI `StageId` values.
   - `kind`: `toggle`, which needs a `!!bool` scalar, or `value`, which takes any scalar.
   - `pattern`: optional, `value` rules only. A Go regexp with exactly one capture group, which
     extracts the answer from the scalar, e.g. `^[^.]+\.(.+)$` takes `domain.base` from a host.
4. **Classification precedence: owned, then recognized, then unknown.**
   - **owned:** under a marker in scope.
   - **recognized:** the leaf is a scalar (not an alias, not a merge key), its path matches a rule,
     the rule's type or pattern matches, and the derived value equals the answer's value.
   - The answer's value comes from the **first** matching leaf in document order.
   - A later matching leaf whose derived value differs is **unknown**, with a note:
     `differs from <answer> = <value>`. It was pinned by hand, so rendering must never overwrite it.
   - A leaf whose type or pattern does not match is also unknown, with a note.
   - Everything else is unknown. Each leaf of `roundtrip.Classify` gets exactly one `Class`.
5. **Choosing the bundle.**
   - When the file has a header whose chart version has a bundle, that bundle is used and the
     version is back-derived into `target.chartVersion`.
   - Otherwise the latest `data-space-connector` bundle is used, `target.chartVersion` stays
     unanswered, and the report says why.
6. **Where the state lives.**
   - `profile.Profile` is **not** changed, because CT-4 extends it in parallel.
   - The server keeps the most recent load result: the `*roundtrip.Document` plus the report. That
     makes it the overrides layer, ready for CT-4's save. Each new load replaces it.
   - The UI keeps the report in `AppState.loaded`, outside `answers`. The Foundations and Features
     placeholders show the back-derived answers read-only until Epics 3 and 5 move them into real
     answers.
   - The only answer that moves into `answers` today is `target.chartVersion`.

## Steps

### Step 1: ADR 0014 and owned-marker detection in `roundtrip`
- **`doc/adr/0014-config-tool-load-classification.md`** records decisions 1–6 above:
  - status Accepted;
  - context: US-1.2 and FEATURE_MODEL *Round-trip and provenance*;
  - the alternatives that were rejected: a server-side path read, a profile `overrides` map that
    loses comments, and wildcard rule paths.
- **`doc/adr/README.md`**: add 0014 to the index and change the "ADRs 0009 to 0013" sentence to
  "0009 to 0014".
- **`config-tool/internal/roundtrip/marker.go`** (new):
  - `const MarkerPrefix = "# dsc-config:owned "`, next to the header constants' style.
  - `type OwnedMarker struct { Path Path; Line int; AnswerIDs []string }`.
  - `func (d *Document) OwnedMarkers() []OwnedMarker`. It returns nil when `d.Header().IsZero()`.
    Otherwise it walks the mapping nodes and applies the scope rule of decision 2. Blank-line
    detection uses the source line table the document already has; do not re-encode nodes.
  - `func (d *Document) OwnedBy(markers []OwnedMarker, p Path) (OwnedMarker, bool)`, or an
    equivalent helper. It reports whether a leaf path is in scope, and `internal/load` can use it.
    Keep the API minimal and decide its exact shape here.
- **Tests:** `marker_test.go`, table-driven. Add fixtures under `internal/roundtrip/testdata/` when
  inline strings get long. Cases:
  - the FEATURE_MODEL example, where one marker covers `did` and `host`;
  - a blank line ends the scope;
  - the next commented sibling ends the scope;
  - a nested subtree is owned;
  - a marker without the header is ignored;
  - a marker on a sequence item is ignored;
  - several ids, with whitespace trimmed;
  - CRLF input.
- **Acceptance:** `go test ./internal/roundtrip/...` passes, and `TestCorpusRoundTrip` is unchanged
  and green. Every exported identifier has a one-line GoDoc comment.

### Step 2: Recognition rules in the question-model bundle
- **`config-tool/internal/model/model.go`:**
  - Extend `Bundle` with `Recognize []Rule \`yaml:"recognize"\``.
  - `Rule` has `Path`, `Answer`, `Stage`, `Kind`, `Pattern`. Give `Kind` named constants
    `KindToggle = "toggle"` and `KindValue = "value"`. Give the stage ids constants that mirror
    `ui/src/state/stageIds.ts`.
  - `readBundle` validates every rule, and an invalid one is an error naming the bundle file and the
    rule index:
    - `path` parses with `roundtrip.ParsePath` and is not empty;
    - `answer` is not empty;
    - `stage` and `kind` are known;
    - `pattern` compiles, has exactly one capture group, and appears only on `value` rules;
    - no two rules share a path.
  - Add a method that returns the compiled rules, e.g. `func (b Bundle) Rules() []CompiledRule`, or
    compile them during validation and keep them on an unexported field. `internal/model` must not
    import `net/http`. Importing `roundtrip` is fine, and `layering_test.go` must stay green.
- **`config-tool/model/10.16.0/bundle.yaml`:**
  - Seed **toggle rules only**, for features in FEATURE_MODEL's catalogue whose "Chart keys" column
    names exactly one chart section, and whose `<section>.enabled` switch exists in
    `charts/data-space-connector/values.yaml`. Example: `identity.did` ↔ `did.enabled`.
  - Rule shape: `answer: <featureId>.enabled`, `stage: features`.
  - Verify every path against `values.yaml` and leave out anything ambiguous.
  - Leave cross-cutting `value` rules out of the real bundle; Epic 3 defines those questions. Say so
    in the file's one-line header comment.
- **Test fixture bundle** under `config-tool/internal/model/testdata/` (or the `testdata` of
  `internal/load` in Step 3; pick one and reuse it). It has:
  - a few toggles;
  - one cross-cutting `value` rule that uses `pattern`: `domain.base` from two host keys;
  - one plain `value` rule.
- **Tests:** parameterized `model_test.go` cases for every validation error. Add a test that every
  real bundle loads and that each rule path resolves in the chart's `values.yaml` at
  `../charts/data-space-connector/values.yaml`, loaded through `roundtrip.Parse` and `Lookup`, so a
  renamed chart key breaks the build.
- **Acceptance:** `go test ./...` passes, and `GET /api/model` behaves as before.

### Step 3: `internal/load` core — classifier, back-derivation, overrides, report, corpus test
New core package `config-tool/internal/load`. It imports no `net/http`; check that
`layering_test.go` covers it.

- **`func Load(src []byte, name string, bundles []model.Bundle) (*Result, error)`:**
  - `roundtrip.Parse`. Its errors are returned wrapped, and `ErrMultiDocument` stays matchable with
    `errors.Is`.
  - Choose the bundle as in decision 5.
  - Build a `roundtrip.Classifier` from `OwnedMarkers` plus the bundle's rules, with the precedence of
    decision 4.
  - Run `roundtrip.Classify`.
- **`Result` holds:**
  - `Doc *roundtrip.Document`, the untouched source, which is the overrides layer;
  - `Report`.
- **`Report` (JSON tags in camelCase) holds:**
  - `source`;
  - `chart` and `chartVersion` (the bundle used);
  - `chartVersionFrom`: `header` or `latest`;
  - `generated`: the header is present;
  - `counts {owned, recognized, unknown, total}`;
  - `owned`, `recognized` and `unknown`, each `[]Entry{path, line, value, answer?, note?}`. `value`
    is a short display string of the scalar, `""` for collections, `*name` for an alias;
  - `answers []Answer{id, stage, value, paths}`. `value` is typed: a bool for toggles, a string or
    number for values.
- **Name the limits.** `maxValuePreview`, for example, truncates long scalars in entries. No magic
  numbers.
- **Never touch the document.** `Load` makes no `Set`, `Merge` or `SetHeader` call.
- **Tests (`load_test.go`), table-driven with the fixture bundle:**
  - a file without markers gives only recognized and unknown leaves;
  - a generated file with markers gives all three classes;
  - the first value wins, and a later differing leaf is unknown with a note;
  - a toggle with a non-bool scalar is unknown;
  - alias and merge-key leaves are unknown;
  - a header chart version with a bundle gives `chartVersionFrom=header`, and one without a bundle
    falls back to `latest`;
  - for each case, `counts.total == len(roundtrip.Classify(doc, nil))`, the three lists add up to
    it, and no path appears twice.
- **Corpus test (`corpus_test.go`):**
  - For every `k3s/*.yaml` and both charts' `values.yaml`, `Load` with the **real** embedded bundles
    (`bundles.FS`) gives a report. Its counts add up to the leaf count.
  - `Result.Doc.Write()` returns exactly the input bytes. This is the CT-2 byte-identity property,
    checked after a load.
  - Copy the `corpusFiles` and `requiredCorpus` guard from `internal/roundtrip/roundtrip_test.go`; it
    is test-only and in another package. Fail if the k3s directory is missing, so the test cannot
    pass vacuously.
- **Acceptance:** `make test` passes, and every acceptance-criteria bullet on classification,
  preservation and the corpus is covered by a named test.

### Step 4: `POST /api/load` endpoint with the loaded-document session slot
- **`config-tool/internal/server/api.go`:**
  - Replace the 501 stub with `handleLoad`. It reads the body through
    `http.MaxBytesReader(maxBodyBytes)` (413 via `bodyErrorStatus`) and takes `name` from the query.
    Only the base name is kept: strip any directory part, and default to `values.yaml`.
  - It loads `model.Versions(s.models)` and calls `load.Load`. On success it stores the result in a
    new `Server` field guarded by a `sync.Mutex`, e.g. `s.loaded`, and returns the `Report` as JSON.
  - Parse and multi-document errors give 422 with the message, the same pattern as `handleValidate`.
  - Remove `ticketLoad` if nothing uses it any more.
  - Give the slot a one-line GoDoc. CT-4 reads it to save, and a later load replaces it.
- **Tests (`server_test.go` or a new `load_test.go` in `internal/server`), table-driven:**
  - 200 with the report for a k3s profile;
  - 422 for invalid YAML and for multi-document input;
  - 413 above the cap;
  - 401 without the token, through the existing guard helpers;
  - a path in `name` is reduced to its base name;
  - a second load replaces the slot.
- **`config-tool/ui/src/test/api.ts`:** if it mocks endpoints, add the load route there for Step 5.
- **Acceptance:** `go test ./...` passes, and `curl` with the token against a running binary returns a
  report for `k3s/provider.yaml`.

### Step 5: UI — Open values.yaml on S0, Review load report, Expert / Overrides, derived answers
Under `config-tool/ui/src/`, TypeScript with one-line JSDoc on components:

- **`api/load.ts`:**
  - `loadValues(file: File): Promise<LoadReport>`. It posts `await file.text()` as
    `application/yaml` to `/api/load?name=<file.name>` through the existing `api` client, so the
    token and errors are handled there.
  - `LoadReport` and its types mirror the Go JSON.
- **`state/reducer.ts`:**
  - `AppState.loaded?: LoadReport`.
  - Action `fileLoaded { report }`. It sets `entered: true`, `stage: StageId.Review`, adds Review to
    `visited`, and resets `answers` to `emptyAnswers()`. If `report.chartVersionFrom === 'header'`
    it sets `target.chartVersion`. It stores `loaded`.
  - `newConfiguration` clears `loaded`.
  - Add reducer tests.
- **`stages/Start.tsx`:**
  - Move *Open values.yaml* out of `COMING` into a live card. A hidden `<input type="file"
    accept=".yaml,.yml">` is triggered by the card.
  - While loading, the card shows a busy state. A failure (`ApiError`: 413, 422) is shown inline on
    S0, and the state is not entered.
  - Add a test in `Start.test.tsx` with a mocked API.
- **`stages/Review.tsx`** (new), registered in `stages.ts` as Review's component:
  - When `loaded` is set, it shows the load report: source name, chart version and where it came
    from, the counts per class, and lists per class. Unknown entries are grouped by top-level key in
    collapsed `<details>`, with path, line, value and note. It also shows the back-derived answers
    grouped by stage, with a "go" link (`goTo`).
  - The heading wording follows MOCKUPS S7 and the prototype, e.g. *Opened `<name>`: understood N,
    carried over untouched M*.
  - When `loaded` is not set, it keeps the old placeholder text for Epic 6. Review's `comingIn`
    stays for that case only.
- **Derived answers on their stages:**
  - A small read-only `DerivedAnswers` component lists `loaded.answers` filtered by stage. The
    Foundations and Features placeholders render it under their placeholder text, e.g. through an
    optional `stageId` prop on `Placeholder`, or a wrapper in `stages.ts`.
  - The Target stage picks up `chartVersion` from answers, which needs no extra work.
- **Expert / Overrides:**
  - Enable the rail's *Expert* button when `loaded` is set. It opens an Expert view, an
    `AppState.expert` flag or a non-journey view id, showing an **Overrides** list: the unknown
    entries, with a note that they are kept verbatim and written back in place.
  - The YAML editor part stays disabled, titled "Epic 5".
  - Keep the view out of `STAGES`, so the journey order and `neighbour` do not change.
- **Tests:**
  - Vitest for Review, Overrides, DerivedAnswers, Start and the reducer.
  - Extend `e2e/smoke.spec.ts`: open `k3s/provider.yaml` through `setInputFiles` and expect the
    Review heading and the counts. If the fixture path needs it, add the path to `e2e/env.ts` or
    `e2e/fixtures/`.
  - `npm run lint` passes.
- **Acceptance:** `make ui && make test` passes, as does the Playwright smoke run (see
  `.github/workflows/config-tool.yaml` for the exact command). Opening a file lands on Review with
  the report.

### Step 6: Documentation and context
- **`config-tool/README.md`:** add a *Loading a values.yaml* section after *Round-trip*. It covers:
  - the three classes and their precedence;
  - the marker syntax and scope;
  - how recognition rules are declared in `model/<version>/bundle.yaml` and how a later epic adds
    them;
  - the upload and the 8 MiB cap;
  - that nothing is written in CT-3.
  Update *Package layout* with `internal/load`.
- **`doc/config-tool/FEATURE_MODEL.md`, *Round-trip and provenance*:** one sentence linking ADR 0014
  for the marker scope and the precedence rule.
- **`doc/config-tool/USER_JOURNEY.md`, Stage 0 *Open an existing values.yaml* row:** link the README
  section. Do not rewrite the design.
- **`AGENTS.md`, *Config tool* section:** replace the CT-3 "planned" note with the real package and
  file names, in one or two lines.
- **Checks:** run `.github/scripts/check-links.sh`, or at least make sure every new relative link
  resolves.
- **Acceptance:** the docs describe the shipped behaviour, the links check is clean, and `make test`
  is still green.
