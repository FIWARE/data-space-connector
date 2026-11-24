# 0020 - Foundations are bundle data rendered by a closed function set into a tree with provenance

**Status:** Proposed
**Date:** 2026-11-24
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/USER_JOURNEY.md`](../config-tool/USER_JOURNEY.md#stage-3--foundations)

## Context

US-3.1 (#125) of [`USER_STORIES.md`](../config-tool/USER_STORIES.md) asks for one base domain that reaches every
chart key derived from it: ingress hosts, TLS secret names and host lists, `cert-manager.io/*` annotations and
public URLs. In `k3s/provider.yaml` 107 leaves carry the domain. The design's figure of "41" was an estimate. The
user must see which keys an answer reaches, and what a change would rewrite, before anything is written. A loaded
file's hand edits must survive a render.

`internal/render` was a stub. Answers existed in the profile (ADR 0015), but `Validate` refused every id, and a
chart switch carried the answers a `recognize:` rule placed in stage `target` or `foundations` (ADR 0019 §3).

## Decision

### 1. Foundations are bundle data

A bundle declares `foundations:`, a list in declaration order of
`{id, stage, type, default?, label, help, doc?, targets: [{path, fn, args?, sep?}]}`. Ids are dotted
(`domain.base`, `host.verifier`): they key `profile.Answers`, and the open-question field is `answers.<id>`.
`type` is `string`, `hostname` or `domain`, each checked by one pattern. A `string` is a single line
without control characters, `"`, `\`, `<` or `>`, because chart templates interpolate it unescaped into JSON and
HTML (`_realm.tpl` writes `keycloak.realm.displayNameHtml` that way). `doc` is a repo-relative doc path. There
is never Go code per key: what reaches a chart key is a target in the bundle.

### 2. A closed function set

- `tpl`: one Go `text/template` arg over the answer namespace (`"{{ .host.keycloak }}-tls"`), parsed with
  `missingkey=error` and an empty FuncMap.
- `join`: renders every template arg and joins the results with `sep` (`cert-manager.io/alt-names`).
- `copy`: the one answer its `{{ .id }}` arg names, with its type kept.
- `const`: one scalar arg written verbatim, referencing no answer, and only under a list index (§4).

The answers a template references are read from its parse tree (`parse.FieldNode`) at bundle load. They give
provenance, the fan-out of an answer and the order of defaults. Adding a function means a line in the bundle
header, one case in `internal/render/funcs.go` and an amendment to this ADR.

### 3. Validation at bundle load

`readBundle` refuses a bundle with a duplicate or malformed id, an unknown stage, type or fn, a template that
does not parse or holds more than text and `{{ .id }}` actions, a reference to an undeclared id, a cycle among
`default`s, a path that does not parse (`roundtrip.ParsePath` syntax, list indices included) or whose root is not
in `topLevelKeys`, two targets writing one path, an index gap, and a `const` outside a list or with an answer
reference. The compiled form stays unexported behind `Bundle.Foundation(id)` and `Bundle.FoundationOrder()`
(topological).

### 4. Lists are owned whole

Helm replaces a list instead of merging it, so writing one field of one element would replace the chart's list
with a fragment. A path may index into a list only when the bundle owns that list whole: its targets write
elements `0..n-1` and every key of each (with `const` for the keys no answer derives). Load refuses gaps; key
completeness is checked by the provider acceptance test. An owned list is emitted all or nothing: while any answer
one of its targets references is absent, none of its leaves is. Lists of objects that carry content no foundation
owns (APISIX routes, realm clients, dashboard providers) are not targets; their stories own them.

### 5. Defaults derive, unanswered writes nothing

A `default` is a template over other answers, so `host.<component>` defaults to `"<component>.{{ .domain.base }}"`
and is present only while every answer it references is. A target is skipped when an answer it references is
absent after defaults are resolved. Every target writes a scalar.

### 6. A tree with provenance

`render.Render(bundle, profile, options)` returns a `Tree` of leaves sorted segment-wise by path (indices
numeric), each with `Provenance {foundation, answers, fn, target, pinned, kept, rendered}`. `Tree.Nested()` builds
the map form, and the JSON of a tree is byte-identical across runs.

### 7. Hand edits are pinned by load classification

With a loaded file (`*load.Result`), a target path that is an **unknown** or **skipped** leaf of the load report
(ADR 0014 §4), that runs through an unknown leaf, or that has unknown leaves below it (the file holds a collection
there), keeps the loaded value and is marked `pinned`. Owned and recognized leaves are re-rendered. The kept value is
decoded from the loaded node, never from the report's capped preview, so its YAML type survives. A node that is not a
scalar leaves the leaf pinned without a kept value, and `Nested()` omits it so the overrides layer keeps the subtree.
`rendered` holds what the render would have written. An empty mapping above a target path (`{}` as the whole file or at
`Result.Root`) is not a conflict: the target merges into it. A hand edit at, above or anywhere below an owned list
(an extra element, a key no target writes, an empty list) pins every leaf of that list without a kept value, so
`Nested()` never writes a fragment of it or drops content the tool did not write; the overrides layer keeps the list.

### 8. The diff is grouped and baselined per session

`render.Diff(prev, next)` groups changes by foundation in bundle order as `added`, `changed`, `removed` or
`pinned`. A pinned leaf is reported when its `rendered` value changed, with the kept value alongside. The server
keeps the last tree per session and resets it on load and on profile open, so the first render diffs against an
empty tree.

### 9. A chart switch carries declared foundation answers

This amends ADR 0019 §3. An answer's stage comes from the old bundle's `recognize:` rules and its `foundations:`.
It is carried when that stage is `target` or `foundations` **and** the new bundle declares the id in
`foundations:`; otherwise it is dropped, together with its open-question mark. A `recognize:` rule does not
declare an id, so the switched profile always validates. The trust-anchor bundle declares `org.displayName` and
`domain.base` (without targets yet), so both survive the switch; `host.verifier` does not.

## Consequences

- `k3s/provider.yaml` is the acceptance measure: every domain-bearing leaf is a target or a row of an explicit
  exclusion table naming its owning story, and a row that matches nothing fails the test.
- The 10.16.0 host labels are `<component>.<base>`, while provider.yaml uses participant-specific labels. The
  acceptance test checks the key set with the base domain only, and the values with every host answered.
- Adding a chart key is a bundle edit and a revision bump; adding a function is a code change and an ADR
  amendment.
- An answer that only a `recognize:` rule places is dropped on a chart switch. No shipped rule is affected.
- Targets are not gated by features yet: answering `domain.base` renders every derived host whatever the role
  and `profile.Features`, so a Consumer gets the provider's APISIX `hosts`/`tls` list and its
  `cert-manager.io/alt-names`. Values of a disabled component are inert, but an enabled APISIX ingress requests a
  certificate for names nobody serves. Gating is US-5.3 ("a disabled feature contributes no keys"). For an owned
  list it means compacting the indices of the enabled elements, because a gap would emit nothing (§4).
- A leaf that carries the domain only because local k3s resolves it through nip.io is not a public host and is
  not a target: `tpp.transfers.host` is TPP's in-cluster back-channel to Rainbow, left to the DSP feature page.

## Alternatives

- **Go code per key or per component.** Untestable against the bundle and repeated per chart version.
- **Merging list elements by a key field.** Ingress hosts have no stable key other than the host, which is the
  value that changes.
- **Pinning every loaded leaf that differs from the render.** A future domain recognition rule would then pin a
  loaded file's whole fan-out.
- **Sprig or a custom FuncMap in templates.** An open function set cannot be validated or traced to answers.
