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
   - The body is the `File` itself, never decoded text: `Blob.text()` strips a UTF-8 BOM and turns
     invalid UTF-8 into U+FFFD, and the server slot keeps these bytes as the overrides layer that
     CT-4 writes back. The name goes through `encodeURIComponent`.
   - The server reads no new paths. Choosing where to save is CT-4's concern.
2. **The owned marker.**
   - Syntax: a comment line `# dsc-config:owned <id>[,<id>...]` in the head comment of a mapping key.
   - Scope: the marker owns that key's whole subtree and every following sibling key of the same
     mapping. The scope ends at the first blank source line between siblings, at the next sibling
     that has a head comment of its own, or at the end of the mapping.
   - "Between siblings" means after the end of the previous sibling's subtree and before the next
     sibling's key line. The end of the subtree is where `subtreeEnd` in `insert.go` puts it: past
     the last leaf's text as the scanner ends it (`lastEnd`) and the deeper-indented comment lines
     right below it. Blank lines inside a value (a block scalar with an empty line or `|+` keep
     chomping, a nested mapping spaced out with blank lines) are content, so they never end the
     scope; ending it there would leave owned keys unowned, the unsafe direction.
   - Markers count only when the file carries the generated header, i.e. when `roundtrip.Parse`
     actually stripped the three-line header shape (`HeaderPrefix` on line 1). That is decided by
     whether the header was stripped, not by `Header().IsZero()`: a header line whose fields do not
     parse still counts, and a later `SetHeader` does not change it. A file without the header was
     not written by the tool, so nothing in it is owned.
   - A marker anywhere else (on a sequence item, or in a line or foot comment) is ignored.
   - yaml.v3 (v3.0.1, checked) attaches a marker directly above a key, the first key of a mapping
     included, to that key's `HeadComment`. A comment only moves to the document node when a blank
     line separates it from the key, and such a marker is not "directly above" anything, so it is
     ignored. No fallback walk over document or parent nodes is needed; the first-top-level-key and
     first-nested-key tests of Step 1 pin this layout against a yaml.v3 change.
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
   - **Owned leaves back-derive too.** The class decides what a re-render may touch; it does not
     stop an owned leaf from answering. CT-3 loads a bare file without its profile, and a generated
     file is exactly the one with markers, so without this its owned answers (`domain.base`,
     `did.value` in the FEATURE_MODEL example) would be lost, and a recognized leaf elsewhere could
     set them to a value the next re-render then writes over the owned block.
   - **Two passes, owned first.** Pass 1 visits only owned leaves in document order and applies the
     same rule, kind and pattern check; the first matching owned leaf sets the answer. Pass 2
     visits the remaining leaves in document order; an answer still unset is set by the first
     matching one. So an owned value always wins over a recognized one.
   - A matching leaf whose derived value differs from the answer's value keeps its class if owned
     (with the note) and is otherwise **unknown**, with the note `differs from <answer> = <value>`.
     A non-owned one was pinned by hand, so rendering must never overwrite it.
   - A leaf whose type or pattern does not match gets a note; it is unknown unless owned.
   - Everything else is unknown. Each leaf of `roundtrip.Classify` gets exactly one `Class`.
   - **Skipped pairs are reported, not dropped.** `Classify` skips shadowed duplicate keys and
     pairs whose key is not a scalar (an alias or complex key), together with their subtrees.
     `k3s/consumer.yaml` has such duplicates. Their bytes stay in the document, so the report
     lists each skipped pair once, in its own `skipped` list, with the note `shadowed by line N`
     (N is the live key's file line) or `key is not a scalar`. Its subtree is not walked: the entry
     stands for the whole pair, and pairs nested inside it are not listed again. A skipped pair is
     never owned or recognized, because Helm ignores a shadowed copy and no `Path` addresses a
     non-scalar key. Expert / Overrides lists it next to the unknown leaves, since it too is
     carried over verbatim. It is a separate list rather than `unknown` entries because a
     shadowed key has the same `Path` as its live twin, and the report keeps one path per entry.
   - Answer ids that a marker names but no owned leaf derived are listed in the report as owned but
     not derived, so the user sees which answers the profile would have to supply.
   - **Line numbers in the report are file lines.** `roundtrip.Parse` strips the three-line header
     before decoding, so `yaml.Node.Line` and `Leaf.Line` count from the body. The offset is added
     in one place, `roundtrip`, and every line that leaves the core (report entries,
     `OwnedMarker.Line`) is the line an editor shows.
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
- **`config-tool/internal/roundtrip/document.go`:** `func (d *Document) FileLine(bodyLine int) int`
  maps a body line (`yaml.Node.Line`, `Leaf.Line`) to its line in the file: it adds the number of
  lines in the stripped header (`headerLineCount` when `Parse` stripped one, 0 otherwise). This is
  the only place the offset is computed.
  - Add one unexported `func (d *Document) hasHeader() bool { return d.headerRaw != nil }` and use
    it for every "header is present" decision: `FileLine`, `OwnedMarkers`, and an exported
    `func (d *Document) Generated() bool` that `internal/load` uses for `Report.generated`. Never
    use `Header().IsZero()` for this: `parseHeaderLine` leaves unparseable fields empty, and
    `Header()` returns whatever `SetHeader` got (CT-4 calls it on the stored document), while the
    body offset is fixed at parse time.
  - Tests: in a file with a header, the first key's `Leaf.Line` is 1 and its `FileLine` is 4;
    without a header both are equal; with an unparseable header line such as
    `# Generated by dsc-config x y` (zero `Header`, header still stripped) `FileLine` is still
    body line + 3 and `Generated()` is true; after `SetHeader(Header{})` `FileLine` is unchanged.
- **`config-tool/internal/roundtrip/marker.go`** (new):
  - `const MarkerPrefix = "# dsc-config:owned "`, next to the header constants' style.
  - `type OwnedMarker struct { Path Path; Line int; AnswerIDs []string }`. `Line` is the marker's
    file line, via `FileLine`.
  - `func (d *Document) OwnedMarkers() []OwnedMarker`. It returns nil when `!d.hasHeader()`.
    Otherwise it walks the mapping nodes, reads markers from key `HeadComment`s only, and applies
    the scope rule of decision 2. Blank-line detection scans the source lines from
    `subtreeEnd(prevValue)` up to the next sibling's key line, reusing `subtreeEnd`/`lastEnd`
    from `insert.go`; it never looks inside a value's own lines. Do not re-encode nodes.
  - `func (d *Document) OwnedBy(markers []OwnedMarker, p Path) (OwnedMarker, bool)`, or an
    equivalent helper. It reports whether a leaf path is in scope, and `internal/load` can use it.
    Keep the API minimal and decide its exact shape here.
- **Tests:** `marker_test.go`, table-driven. Add fixtures under `internal/roundtrip/testdata/` when
  inline strings get long. Cases:
  - the FEATURE_MODEL example, where one marker covers `did` and `host`;
  - a blank line ends the scope;
  - a blank line inside a value does not: an owned key whose value is a `|` block scalar with an
    empty line inside, one with `|+` and trailing empty lines, and one whose nested mapping has a
    blank line between its keys, each followed directly by a sibling that must still be owned;
  - the next commented sibling ends the scope;
  - a nested subtree is owned;
  - a marker without the header is ignored;
  - a marker on a sequence item is ignored;
  - several ids, with whitespace trimmed;
  - CRLF input;
  - the marker on the first top-level key right after the header (no other comment between);
  - the marker on the first key of a nested mapping (`parent:` then the marker, then the key);
  - in both, assert the owned path and that `OwnedMarker.Line` is the marker's file line. They
    guard the yaml.v3 attachment behaviour that decision 2 relies on;
  - a marker followed by a blank line before the key is ignored (yaml.v3 puts it on the document);
  - a marker under an unparseable header line (`# Generated by dsc-config x y`) still counts.
- **`config-tool/internal/roundtrip/classify.go`:** the pairs `Classify` skips (decision 4).
  - `type SkipReason int` with `SkipShadowed` and `SkipNonScalarKey`, and
    `type SkippedPair struct { Parent Path; Key, Value *yaml.Node; Line int; ShadowedBy int; Reason SkipReason }`.
    `Line` is the key's body line; `ShadowedBy` is the live key's body line, 0 unless shadowed.
  - `func Skipped(doc *Document) []SkippedPair` returns them in document order, outermost only.
    `Classify` and `Skipped` share one unexported walker over `livePairs`, so they cannot disagree
    about which pairs are live; `livePairs` stays unexported.
  - Tests in `classify_test.go`, table-driven: `testdata/duplicate-keys.yaml` gives the shadowed
    `a` and `c` with their `ShadowedBy` lines; `testdata/non-scalar-keys.yaml` gives the alias key
    and both complex keys; a duplicate nested inside a shadowed subtree is not listed twice; the
    corpus test asserts `Skipped` is non-empty for `k3s/consumer.yaml`.
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
  real bundle loads and that each rule path resolves in the chart's `values.yaml`, loaded through
  `roundtrip.Parse` and `Lookup`, so a renamed chart key breaks the build. The test runs in
  `config-tool/internal/model`, so use the `repoRoot = "../../.."` constant pattern of
  `internal/roundtrip/roundtrip_test.go`: `repoRoot + "/charts/data-space-connector/values.yaml"`.
  The test fails, rather than skips, when the file is missing.
- **Acceptance:** `go test ./...` passes, and `GET /api/model` behaves as before.

### Step 3: `internal/load` core — classifier, back-derivation, overrides, report, corpus test
New core package `config-tool/internal/load`. It imports no `net/http`; check that
`layering_test.go` covers it.

- **`func Load(src []byte, name string, bundles []model.Bundle) (*Result, error)`:**
  - `roundtrip.Parse`. Its errors are returned wrapped, and `ErrMultiDocument` stays matchable with
    `errors.Is`.
  - Choose the bundle as in decision 5.
  - Build the classifier from `OwnedMarkers` plus the bundle's rules, with the precedence and the two
    passes of decision 4: pass 1 derives answers from owned leaves, pass 2 runs
    `roundtrip.Classify` with those answers pre-set.
  - `roundtrip.Skipped` fills the `skipped` list (decision 4).
  - Convert every line it reports with `Doc.FileLine`, `ShadowedBy` included.
- **`Result` holds:**
  - `Doc *roundtrip.Document`, the untouched source, which is the overrides layer;
  - `Report`.
- **`Report` (JSON tags in camelCase) holds:**
  - `source`;
  - `chart` and `chartVersion` (the bundle used);
  - `chartVersionFrom`: `header` or `latest`;
  - `generated`: `Doc.Generated()`, i.e. `Parse` stripped a header;
  - `counts {owned, recognized, unknown, skipped, total}`. `total` is the sum of the four:
    the leaves of `Classify` plus the skipped pairs;
  - `owned`, `recognized` and `unknown`, each `[]Entry{path, line, value, answer?, note?}`. `line`
    is the file line. `value` is a short display string of the scalar, `""` for collections,
    `*name` for an alias;
  - `skipped []Entry`: one per skipped pair. `path` is the parent path plus the key as it reads:
    the scalar key, `*name` for an alias key, or a named placeholder constant for a complex key.
    `value` follows the leaf rule above, and `note` gives the reason;
  - `answers []Answer{id, stage, value, paths, from}`. `value` is typed: a bool for toggles, a
    string or number for values. `from` is `owned` or `recognized`;
  - `markers []Marker{path, line, answerIds, underived}`: each owned marker, and the answer ids it
    names that no owned leaf derived.
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
  - an owned leaf that matches a rule stays `owned` and sets the answer with `from=owned`;
  - an owned value wins over a recognized leaf with a different value that comes **earlier** in
    the document; that recognized leaf becomes unknown with the `differs` note;
  - a marker id with no matching owned leaf appears in `markers[].underived`;
  - in a file with a header, an entry's `line` is its file line (body line + 3).
  - `testdata/duplicate-keys.yaml` of `roundtrip` (copied or read via a relative path): the shadowed
    `a` and `c` are in `skipped` with `shadowed by line N`, where N is the file line of the live key;
  - `testdata/non-scalar-keys.yaml`: the alias key and the complex keys are in `skipped` with
    `key is not a scalar`;
  - for each case, `counts.owned + recognized + unknown == len(roundtrip.Classify(doc, nil))`,
    `counts.skipped == len(roundtrip.Skipped(doc))`, `counts.total` is their sum, each list's
    length equals its count, and no path appears twice across `owned`, `recognized` and `unknown`.
- **Corpus test (`corpus_test.go`):**
  - For every `k3s/*.yaml` and both charts' `values.yaml`, `Load` with the **real** embedded bundles
    (`bundles.FS`) gives a report. Its counts add up to the leaf count plus the skipped pairs, and
    `k3s/consumer.yaml` reports a non-empty `skipped` list.
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
  - a second load replaces the slot;
  - a BOM-prefixed body: `s.loaded.Doc.Write()` equals the uploaded bytes exactly.
  - the existing `/api/load` row in the 501 stub table of `server_test.go` (with `ticketLoad`) is
    removed; the cases above replace it.
- **`config-tool/ui/src/test/api.ts`:** if it mocks endpoints, add the load route there for Step 5.
- **Acceptance:** `go test ./...` passes, and `curl` with the token against a running binary returns a
  report for `k3s/provider.yaml`.

### Step 5: UI — Open values.yaml on S0, Review load report, Expert / Overrides, derived answers
Under `config-tool/ui/src/`, TypeScript with one-line JSDoc on components:

- **`api/load.ts`:**
  - `loadValues(file: File): Promise<LoadReport>`. It posts the `File` itself as the fetch body
    (not `file.text()`, which decodes; see decision 1) to
    `/api/load?name=${encodeURIComponent(file.name)}` through the existing `api` client, so the
    token and errors are handled there. Set `Content-Type: application/yaml` explicitly, because
    `request` in `api/client.ts` defaults any body without one to JSON.
  - `api/load.test.ts` (Vitest, stubbed `fetch`): the request body is the same `File` object, the
    header is `application/yaml`, and a name such as `a&b #1%.yaml` arrives encoded.
  - `LoadReport` and its types mirror the Go JSON.
- **`state/reducer.ts`:**
  - `AppState.loaded?: LoadReport`.
  - Action `fileLoaded { report }`. A load starts a new configuration, so it resets the same
    session state as `newConfiguration`: `entered: true`, `stage: StageId.Review`,
    `visited: [StageId.Review]`, `answers: emptyAnswers()`, `session.chosenContext: undefined`
    (the kubeconfig path is kept, as in `newConfiguration`). If
    `report.chartVersionFrom === 'header'` it sets `target.chartVersion`. It stores `loaded`.
  - `newConfiguration` clears `loaded`.
  - Add reducer tests. One starts from a state with several visited stages, a chosen context and
    answers, dispatches `fileLoaded`, and asserts `visited` is `[Review]`, `chosenContext` is
    unset, the kubeconfig path is kept and `answers` is empty apart from the header version.
- **`stages/Start.tsx`:**
  - Move *Open values.yaml* out of `COMING` into a live card. A hidden `<input type="file"
    accept=".yaml,.yml">` is triggered by the card.
  - While loading, the card shows a busy state. A failure (`ApiError`: 413, 422) is shown inline on
    S0, and the state is not entered.
  - Add a test in `Start.test.tsx` with a mocked API.
- **`stages/Review.tsx`** (new), registered in `stages.ts` as Review's component:
  - When `loaded` is set, it shows the load report: source name, chart version and where it came
    from, the counts per class, and lists per class. Unknown entries are grouped by top-level key in
    collapsed `<details>`, with path, line, value and note. Skipped pairs get their own collapsed
    group, worded as keys Helm ignores or cannot address, with the line and reason. It also shows the back-derived answers
    grouped by stage, with a "go" link (`goTo`), and the marker answers that were not derived.
  - The heading wording follows MOCKUPS S7 and the prototype, e.g. *Opened `<name>`: understood N,
    carried over untouched M*, where M counts unknown leaves plus skipped pairs.
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
    entries and the skipped pairs (with their reason), with a note that they are kept verbatim and written back in place.
  - The YAML editor part stays disabled, titled "Epic 5".
  - Keep the view out of `STAGES`, so the journey order and `neighbour` do not change.
- **Tests:**
  - Vitest for Review, Overrides, DerivedAnswers, Start and the reducer. Review and Overrides
    tests include a report with a skipped pair.
  - Extend `e2e/smoke.spec.ts`: open `k3s/provider.yaml` through `setInputFiles` and expect the
    Review heading and the counts. If the fixture path needs it, add the path to `e2e/env.ts` or
    `e2e/fixtures/`.
  - `npm run lint` passes.
- **Acceptance:** `make ui && make test` passes, as does the Playwright smoke run (see
  `.github/workflows/config-tool.yaml` for the exact command). Opening a file lands on Review with
  the report.

### Step 6: Documentation and context
- **`config-tool/README.md`:** add a *Loading a values.yaml* section after *Round-trip*. It covers:
  - the three classes and their precedence, and the skipped pairs (shadowed or non-scalar keys);
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
