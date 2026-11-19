# 0014 - Load an existing values.yaml: owned markers, recognition rules and a load report

**Status:** Accepted
**Date:** 2026-11-17
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md#round-trip-and-provenance)

## Context

CT-3 (#118) implements US-1.2 of [`USER_STORIES.md`](../config-tool/USER_STORIES.md): the user
opens an existing `values.yaml`, the tool derives what answers it can, carries everything else
along, and lands on Review with a load report. FEATURE_MODEL *Round-trip and provenance* sets the
rule: **anything the tool did not write, it does not touch.** The round-trip engine of
[ADR 0013](0013-config-tool-roundtrip-byte-patches.md) keeps the source bytes as the truth; this
record fixes how a loaded file is read on top of it. CT-3 renders and saves nothing; CT-4 wires
saving.

## Decision

1. **The input is the file's bytes, uploaded by the browser.** `<input type="file">` gives the UI
   the content but never a path, in the browser and in the Wails webview alike. The UI sends
   `POST /api/load?name=<basename>` (name through `encodeURIComponent`) with
   `Content-Type: application/yaml` and the `File` itself as the body, never decoded text:
   `Blob.text()` strips a UTF-8 BOM and replaces invalid UTF-8, and these bytes become the
   overrides layer CT-4 writes back. The existing 8 MiB body cap applies (413 above it). The
   server reads no new paths.
2. **The owned marker** is a comment line `# dsc-config:owned <id>[,<id>...]` in the head comment
   of a mapping key. It owns that key's subtree and every following sibling of the same mapping,
   until the first blank source line between siblings, the next sibling with a head comment of
   its own, or the end of the mapping. "Between siblings" starts where the previous sibling's
   subtree ends as `roundtrip` places it, so blank lines inside a value (block scalars, spaced-out
   nested mappings) never end the scope. Markers count only when `roundtrip.Parse` stripped the
   generated header (`Document.Generated`), even one whose fields do not parse; a file without it
   was not written by the tool. Markers on sequence items and in line or foot comments are
   ignored. yaml.v3 attaches a comment directly above a key to that key's `HeadComment`; one
   separated by a blank line goes to the document node and is ignored. Tests pin both layouts.
3. **Recognition rules are data** in `model/<version>/bundle.yaml` under `recognize:`. A rule has
   an exact `path` (`roundtrip.ParsePath` syntax, no wildcards), an `answer` id, a UI `stage`, a
   `kind` (`toggle` needs a `!!bool` scalar, `value` takes any scalar) and, for `value` rules, an
   optional `pattern`: a Go regexp with exactly one capture group that extracts the answer.
4. **Classification is owned, then recognized, then unknown**, one class per leaf of
   `roundtrip.Classify`.
   - A leaf under a marker in scope is owned. Owned leaves back-derive answers too: a loaded
     generated file comes without its profile, and its owned answers would otherwise be lost.
   - Derivation runs in two passes in document order, owned leaves first; the first matching leaf
     sets an answer, so an owned value always wins over a recognized one.
   - A non-owned scalar leaf (not an alias or merge key) whose rule, kind and pattern match and
     whose derived value equals the answer's value is recognized. If the value differs, it was
     pinned by hand: it is unknown with the note `differs from <answer> = <value>`, and rendering
     never overwrites it. A kind or pattern mismatch also gives a note.
   - Pairs `Classify` skips (shadowed duplicate keys, alias or complex keys) are listed once each
     in a separate `skipped` list (`roundtrip.Skipped`) with `shadowed by line N` or
     `key is not a scalar`, without their nested pairs. They are never owned or recognized; their
     bytes are carried over verbatim like unknown leaves.
   - Answer ids a marker names but no owned leaf derived are reported as owned but not derived.
   - Every line in the report is a file line: `Document.FileLine` adds the stripped header's
     lines in one place.
5. **The bundle** is the one for the header's chart version when there is one, and that version
   is back-derived into `target.chartVersion`. Otherwise the latest `data-space-connector` bundle
   is used, `target.chartVersion` stays unanswered, and the report says why.
6. **State.** `profile.Profile` is not changed (CT-4 extends it in parallel). The server keeps the
   latest load result, the `*roundtrip.Document` and the report, as the overrides layer for
   CT-4's save; each load replaces it. The UI keeps the report in `AppState.loaded`, outside
   `answers`, and shows back-derived answers read-only; only `target.chartVersion` moves into
   `answers` today.
7. **The chart's values may sit under a wrapper key.** When Argo CD or an umbrella chart packages
   this chart as a dependency, its values sit under that chart's alias, e.g. `dsc:`. Rule paths
   are relative to a *root* mapping. Without `?root=` the load detects it: every mapping at most
   `maxRootDepth` (3) keys deep that holds a key of the bundle's `topLevelKeys` is a candidate,
   none below a chart key, and the best is the one whose keys (Helm's `global` aside) are most
   often chart keys, then the one with more chart keys, then the shallower. `topLevelKeys` lists
   the chart's top-level `values.yaml` keys; a test keeps it equal to the chart's. The report
   carries `root`, `rootFrom` (`detected` or `requested`) and `rootCandidates`, and Review offers
   the candidates to re-read the same uploaded file under another root. Leaves outside the root
   are unknown; markers and line numbers stay file-wide.

## Consequences

- A loaded file keeps every byte the tool does not own; the class of a leaf decides what a later
  re-render may touch.
- A hand-edited value that contradicts a derived answer is never overwritten, at the cost of
  being reported as unknown rather than recognized.
- New recognition needs a bundle change, not a code change, but only for exact paths.
- Without the generated header nothing is owned, so a file whose header was deleted by hand is
  treated as entirely user-written.

## Alternatives

- **Read the file server-side from a path.** The webview and the browser never expose a path,
  and an API that reads arbitrary paths widens the local attack surface for no gain.
- **Store unknown keys in a profile `overrides` map.** Loses comments, ordering and formatting,
  which breaks the byte-faithful round-trip of ADR 0013.
- **A fixed list of wrapper names** (`dsc`, `data-space-connector`, ...). The alias is the
  wrapping chart's choice, so any list misses some; scoring by chart keys finds any name, and
  `?root=` covers what scoring gets wrong.
- **Wildcard rule paths.** Not needed by any rule yet, and they make precedence between
  overlapping rules ambiguous; they can be added later without changing this format.
