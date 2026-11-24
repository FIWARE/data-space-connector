# Implementation Plan: [E3 Foundations] US-3.1 — One domain, everywhere (foundations model, render engine, fan-out)

## Overview
Replace the `internal/render` stub with a data-driven render engine that turns profile answers into a
deterministic values tree whose leaves carry provenance. Declare the first cross-cutting values
(`org.displayName`, `domain.base`, the derived `host.<component>` set) and every chart key they reach
in `k3s/provider.yaml` as a `foundations:` section in the bundle. On top of that, expose the fan-out
(`Targets`), a grouped diff with *pinned, not updated* overrides through `POST /api/render`, and the
Stage 3 Foundations screen.

## Decisions fixed by this plan (step agents follow them, ADR 0020 records them)

- **Answer ids and field ids.** Foundation ids are dotted (`domain.base`, `host.verifier`). They are
  the keys of `profile.Answers`, and the open-question field id is `answers.<id>`, the same prefix
  `answersFieldPrefix` already uses in `internal/profile/format.go`. The UI's `FieldId` gains an
  `answers` section whose stage comes from the bundle (`stage: foundations`).
- **Bundle schema** (`foundations:` list, in declaration order):
  `{id, stage, type, default?, label, help, doc?, targets: [{path, fn, args?}]}`.
  `type` is one of `string | hostname | domain` and is checked with a pattern per type. `default` is a
  template over other answers, so a `host.<component>` is *derived*: it is present only when every
  answer it references is present. `doc` is a repo-relative doc path (`links.ts` resolves it).
- **Function set** (closed, documented in the bundle header, validated at bundle load):
  - `tpl`: a Go `text/template` string over the answer namespace (`{{ .domain.base }}`,
    `"{{ .host.keycloak }}-tls"`, `"https://{{ .host.verifier }}"`). It is parsed with
    `missingkey=error` and an empty FuncMap.
  - `join`: `args: [<tpl>, …]`, `sep:`. It renders each tpl and joins the results (for
    `cert-manager.io/alt-names`).
  - `copy`: the referenced answer's scalar with its type kept (`{{ .x }}` alone would stringify it).
  The answers a target references are taken from the template parse tree (`parse.FieldNode`) at load.
  They give provenance, `Targets()` and dependency order, and a cycle among `default`s is refused.
  Adding a function means a bundle-header doc line, one case in `internal/render/funcs.go` and an ADR
  amendment. There is never Go code per key.
- **Paths** use `roundtrip.ParsePath` syntax including list indices (`…ingress.hosts[0].host`,
  `…tls[0].hosts[1]`). Bundle load refuses an index gap, a path under a root outside `topLevelKeys`,
  and two targets writing the same path.
- **Unanswered emits nothing.** A target is skipped when any answer it references is absent, after
  defaults are resolved. A derived host whose source is unanswered is absent too.
- **Tree.** `render.Tree` holds `[]Leaf{Path, Value, Provenance{Foundation string, Answers []string,
  Fn string, Target int, Pinned bool}}`, sorted segment-wise by path (indices numeric).
  `Tree.Nested()` builds the map/slice form, and `json.Marshal` of the tree is byte-identical across
  runs.
- **Pinned.** `render.Render(b model.Bundle, p profile.Profile, opts Options)`, where
  `Options.Overrides` is the loaded overrides layer (`*load.Result`, nil when nothing is loaded). A
  target path that exists in the loaded document under its root and is **not** owned by a dsc-config
  marker keeps the loaded scalar, and its leaf is marked `Pinned`. Owned leaves are re-rendered.
- **Diff.** `render.Diff(prev, next Tree) Diff` groups the changes by foundation id (bundle order).
  Each entry is `{path, before, after, status}` with status `added | changed | removed | pinned`, and
  unchanged keys are omitted. The server keeps the last tree per session (`Server.lastRender`, guarded
  like `loaded`) and resets it on load or profile open. The first render therefore diffs against an
  empty tree.
- **Host labels.** The 10.16.0 bundle gives each `host.<component>` the default
  `"<component>.{{ .domain.base }}"` (design: `<component>.<base>`, USER_JOURNEY Stage 3).
  `k3s/provider.yaml` uses participant-specific labels (`keycloak-provider`, `til-provider`,
  `dsp-mp-operations`, …). The acceptance test therefore checks the **key set** with a profile that
  answers only `org.displayName` and `domain.base`. It checks the **values** with a fixture that also
  answers each `host.<component>` with provider.yaml's host. Answering a host is allowed now, and its
  UI is US-3.4.
- **Scope of targets.** These are the provider.yaml leaves that contain `127.0.0.1.nip.io` and come
  from `domain.base`: hosts, TLS `secretName`s, TLS host lists, `cert-manager.io/*` annotations and
  public URLs. Leaves owned by later stories are excluded through an **explicit exclusion table in
  the test**, each row giving a reason: `tir.*` / `tirAddress` (US-3.3), DID values and
  `did:web:` strings (US-3.2). The test fails when a domain-bearing leaf is neither a target nor
  excluded, so the 107 matching lines are pinned exactly instead of asserting the design's
  approximate "41".

## Steps

### Step 1: ADR 0020, `foundations:` schema and loader, profile answer validation
- `doc/adr/0020-config-tool-foundations-render.md` records the decisions above: the schema, the
  function set, path rules, provenance, pinned semantics, the diff baseline and host labels. Add it to
  `doc/adr/README.md`.
- `internal/model/foundations.go`: `Foundation`, `Target`, `Fn` types and the `Foundations
  []Foundation` field on `Bundle` (yaml `foundations`). Validation runs in `readBundle`: unique ids,
  known stage, type and fn, templates that parse, referenced ids that exist, acyclic defaults, valid
  paths with roots in `topLevelKeys`, contiguous indices and no duplicate target path. The compiled
  form (parsed templates and the references of each foundation/target) stays unexported, behind
  `Bundle.Foundation(id)` and `Bundle.FoundationOrder()` (topological).
- `model/10.16.0/bundle.yaml`: add the header block documenting `foundations:` and the function set,
  plus a minimal `foundations:` holding `org.displayName` and `domain.base` with no targets yet. Bump
  `revision` (3 → 4). `model/1.3.6/bundle.yaml`: `foundations: []` and a revision bump.
- `internal/profile`: answers are validated against the profile's bundle. Accept the declared ids,
  check the value's type pattern, keep refusing unknown ids, and accept `answers.<id>` in `open` for
  declared ids. Update the `Answers` doc comment.
- Tests (table-driven): bundle validation errors (bad fn, unknown reference, cycle, index gap,
  duplicate path, root outside `topLevelKeys`) with fixtures in `internal/model/testdata/`. Profile
  validation of answers and open ids.
- Acceptance: `go test ./...` and `make lint` green, and the existing profile/model tests are
  unchanged apart from the answers doc.

### Step 2: Render engine core in `internal/render`
- Replace the stub. `render.go` has `Render(b, p, opts) (Tree, error)`: it resolves answers and
  derived defaults in topological order, then evaluates every target in bundle order, skips targets
  with an absent reference, applies `Options.Overrides` pinning and sorts the leaves. `funcs.go` holds
  `tpl`/`join`/`copy`, `tree.go` holds `Tree`, `Leaf`, `Provenance`, `Nested()` and stable JSON.
- Keep the package HTTP-free (`internal/layering_test.go`). Remove `ErrNotImplemented` from render
  and keep the "writing values.yaml is US-7.1" error where the callers need it (Step 4).
- Temporary callers: make `internal/server/api.go` `handleRender` and `internal/cli/cli.go`
  `generate` compile against the new signature, with their behaviour still "not implemented" (the
  real wiring is Step 4).
- Tests with a fixture bundle in `internal/render/testdata/`: unanswered → no key, derived host absent
  without a domain, `join`/`copy`/`tpl` output, list-index paths building slices, provenance contents,
  pinned vs owned leaves using a `load.Load` result, and two renders giving `bytes.Equal` JSON.
- Acceptance: all of the above green, plus `make lint`.

### Step 3: `domain.base` / `org.displayName` targets and the provider.yaml acceptance test
- Fill `model/10.16.0/bundle.yaml`: one `host.<component>` per public host in `k3s/provider.yaml`
  (verifier, keycloak, til, pap, scorpio, tm-forum-api, contract-management, marketplace, dashboard,
  status-list, ccs, rainbow, fdsc-edc DSP/management endpoints, identityhub (+management), vault,
  the tpp/consent data services, …). Each gets `label`, `help`, `doc`, a `"<component>.{{ .domain.base }}"`
  default and its targets (ingress hosts, TLS host lists, `secretName`s, `cert-manager.io/*`
  annotations, public URLs such as the verifier's `host` and APISIX `discovery` URLs).
  `org.displayName` targets the keys that carry the organization's display name (Keycloak realm
  display name, BAE organization) as provider.yaml has them. If provider.yaml has none, it targets
  the chart key documented in `charts/data-space-connector/values.yaml`. Bump `revision` again only if
  Step 1's bump has already merged on the work branch.
- `internal/render/provider_test.go`: walk `k3s/provider.yaml` with `roundtrip`, collect the leaves
  containing the reference domain, subtract the exclusion table (US-3.2 DID, US-3.3 TIR, each row
  with a reason) and assert:
  1. the two-answer profile renders exactly that path set (no missing, no extra);
  2. the fixture `internal/render/testdata/provider.dscprofile.yaml` (two answers + provider hosts)
     renders every value byte-equal to provider.yaml's scalar;
  3. changing `domain.base` changes exactly those leaves (compare two trees);
  4. with `domain.base` unanswered, no domain-derived key is emitted.
- Acceptance: the test is green and the bundle still loads for both charts.

### Step 4: Fan-out, diff, pinned overrides and `POST /api/render`
- Core: `render.Targets(b, answerID) []TargetRef{Path, Component}` (transitive through derived
  defaults, where component is the path's top-level key) and `render.Diff(prev, next) Diff` grouped by
  foundation in bundle order, with `pinned` entries for leaves kept from overrides. Table-driven
  tests: the counts for `domain.base` vs a single host, the diff for a domain change listing exactly
  Step 3's set, an override reported as `pinned` with the loaded value kept.
- Server (`internal/server/render.go`): `POST /api/render` takes the profile JSON, picks the bundle
  the profile names, renders with `s.loaded` as overrides and diffs against `s.lastRender`. It stores
  the new tree and returns `{tree, diff}`. A bad profile is answered with 422 and its `FieldError`s.
  `GET /api/foundations?chart=&version=` returns, per foundation, the label, help, doc, type, default,
  stage and its `Targets` list with `count` / `components`. Reset `lastRender` in `handleLoad` and
  on profile open. Remove `ticketRender` from the render path.
- CLI: `dsc-config generate -p` renders (so bundle and profile errors surface) and then exits with
  "writing values.yaml lands with US-7.1". Update `cli_test.go`.
- Tests: `internal/server/render_test.go` (token required, 422 on bad answers, the diff on the second
  call, pinned after a load, reset after a load) and `foundations_test.go`.
- Acceptance: `go test ./...` and `make lint` green.

### Step 5: Stage 3 `Foundations.tsx`
- `ui/src/api/foundations.ts` (`GET /api/foundations`) and `ui/src/api/render.ts`
  (`POST /api/render`), with types mirroring the Go JSON. Tests in the style of
  `ui/src/api/*.test.ts`.
- State: `Answers` (`ui/src/state/answers.ts`) gains `values: Record<string, string>` for answer ids.
  `toProfile`/`fromProfile` (`ui/src/api/profile.ts`) write and restore `answers`, and the reducer
  gets `SET_ANSWER`. `openQuestions.ts`: `FieldId` takes `answers.${string}`, which maps to
  `StageId.Foundations`, with labels coming from the loaded foundations.
- Move `AskLater` out of `ui/src/stages/Target.tsx` into `ui/src/chrome/AskLater.tsx` and reuse it
  unchanged for both stages.
- `ui/src/stages/Foundations.tsx`: one scrollable screen with the *Organization* panel
  (`org.displayName`) and the *Network* panel (`domain.base`, then the derived host list read-only
  with its rendered values; editing is US-3.4). Wording follows `doc/config-tool/MOCKUPS.md#s3--foundations`.
  Each field has its doc link, `AskLater` and a *used by N values in M components* button that expands
  the target list. The host panel links
  `doc/deployment-integration/roles/PREREQUISITES.md#public-names-have-to-resolve-inside-the-cluster`.
  When `domain.base` changes, the screen calls `/api/render` (debounced) and shows the grouped diff
  with pinned entries flagged *pinned, not updated*.
- Wire the stage in `ui/src/stages.ts` / `App.tsx`, replacing the placeholder.
- `ui/src/stages/Foundations.test.tsx`: the count text, list on demand, ask-later round trip into
  Review's open questions, the diff shown after a domain change, the pinned label, and the doc links
  present. Update `links.test.ts` if new links are added.
- Acceptance: `npm test`, `make ui`, `make lint` green.

### Step 6: Documentation and full verification
- `config-tool/README.md`: document the `foundations:` section (fields, types, the `tpl`/`join`/`copy`
  function set with examples, path rules), the render tree and provenance, `GET /api/foundations`,
  `POST /api/render` with the diff and pinned semantics, and Stage 3.
- `doc/config-tool/FEATURE_MODEL.md#cross-cutting-values`: state the measured fan-out for
  `domain.base` (the pinned set from Step 3) and link ADR 0020. Update `USER_JOURNEY.md` Stage 3 only
  if it contradicts what shipped.
- Update the US-3.1 line in `AGENTS.md` from planned to shipped.
- Run `make ui`, `make build`, `make test`, `make lint` and `.github/scripts/check-links.sh` (the
  links in the new docs), and fix whatever fails.
- Acceptance: every gate from the ticket is green, and README and ADR describe the shipped behaviour.
