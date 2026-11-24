# Implementation Plan: [E3 Foundations] US-3.2 — Our identity, once (the DID and its 15 places)

## Overview
Add the participant's DID as a Stage 3 foundation: `did.method` (`did:web` | `did:key`) and `did.value`
in `foundations:` of `config-tool/model/10.16.0/bundle.yaml`. `did.value` derives `did:web:<domain.base>`
until the user edits it, then stays pinned. It fans out to every chart key that names the participant's
DID. The first L2 rule warns when `production` is combined with `did:key`. That rule needs a minimal,
data-driven rule engine, which does not exist yet. Stage 3 then shows the DID, where it is used, and the
method choice.

## Facts the steps rely on (verified while planning)
- US-3.1 is merged: `internal/model/foundations.go` (types `string|hostname|domain`, fns
  `tpl|join|copy|const`, `Default` template, `topoOrder`, `checkRefs`), `internal/render` (`resolve`
  derives defaults only for *unanswered* ids, so a given answer already overrides its default; `Render`,
  `Targets`, `Diff`, `Provenance.Pinned` = hand edit in a *loaded file*), `GET /api/foundations` and
  `POST /api/render` in `internal/server/render.go`, `ui/src/stages/Foundations.tsx` (Organization panel =
  ids starting with `org.`; Network panel = ids without default; `Hosts` = ids with a default).
  The bundle is at `revision: 5`.
- `model.Rule` / `Bundle.Rules()` already name the `recognize:` rules. `internal/validate` is the L1
  schema stub (`ErrNotImplemented`). There is no `rules:` section and no L2 engine.
- `profile.Target.Environment` is `development` | `production` (`internal/profile/profile.go`).
- `k3s/provider.yaml` has 15 lines that contain `did:web:mp-operations.org`. One of them is a comment
  (line 513). The 14 leaves are:

  | provider.yaml path | Proposed owner |
  |---|---|
  | `decentralizedIam.vcAuthentication.vcverifier.deployment.verifier.did` | **target** |
  | `contract-management.did` | **target** |
  | `keycloak.issuerDid` | **target** |
  | `keycloak.signingKey.did` | exclusion, US-5.5 (the keystore block): the chart default is `signingKey: {}` and `_realm.tpl` tests only `if .Values.keycloak.signingKey`, so emitting `did` alone yields a `java-keystore` provider with null store, password, alias and algorithm. It is also a key id (`<did>#key-1` in `dsp-provider.yaml`); unset, `_realm.tpl` falls back to `${DID}` |
  | `decentralizedIam.vcAuthentication.vcverifier.deployment.initContainers[0].args[0]` (curl TIR registration script) | exclusion, US-3.3: a k3s-local script, and the list has content no foundation owns |
  | `decentralizedIam.odrlAuthorization.odrl-pap.additionalEnvVars[i].value` (`GENERAL_ORGANIZATION_DID`) | exclusion, US-5.6: an env list with other content |
  | `marketplace.bizEcosystemLogicProxy.additionalEnvVars[i].value` (`BAE_LP_SIOP_CLIENT_ID`) | exclusion, US-5.3: an env list with other content (also holds the proxy) |
  | `keycloak.realm.verifiableCredentials.membership-credential.protocolMappers[i].config.staticValue` | exclusion, US-5.5 (realm credentials) |
  | `keycloak.realm.clientRoles` (JSON string), `keycloak.realm.users` (JSON string) | exclusion, US-5.5 |
  | `keycloak.realm.clients[i].clientId`, `...protocolMappers[i].config."usermodel.clientRoleMapping.clientId"` | exclusion, US-5.5 (an existing `^keycloak\.realm\.clients\[\d+\]\.` row may already match) |
  | `registration.issuer[0].did` | exclusion, US-3.3: the TIL issuer list also lists other participants |
  | `fdsc-dashboard.auth.config.providers[i].clientId` | exclusion, US-5.3 (an existing row shape) |

  Chart keys that name the DID but are absent from `provider.yaml`, because those components are off
  there:
  - `fdsc-edc.*`, verified in `k3s/dsp-provider.yaml` (lines 41-42, 161-171):
    `fdsc-edc.common.config.edc.participant.id`, `fdsc-edc.common.config.issuerId`,
    `fdsc-edc.common.config.oauth.clientId` (the STS client; it must equal the participant id or STS
    token requests fail) and `fdsc-edc.deployment.oid4vc.config.oid4vp.holder.id` are `copy` targets.
    `fdsc-edc.common.config.oauth.secretAlias` (`<did>-sts-client-secret`) is a `tpl` target, so the
    alias keeps naming the secret the IdentityHub bootstrap writes for this DID.

  Two chart-only keys name the DID but are **not** targets, because together they trip the chart's
  `dsc.identityhub.assertHolderKid` (`_helpers.tpl` ~461, run by `identityhub-cm.yaml`; pinned by
  `tests/identityhub-holder-kid_test.yaml`). Once `identityhub.bootstrap.did` is set, every
  `fdsc-edc.{common,deployment.<name>}.config.oid4vp.holder.kid` must be `<bootstrap.did>#<keyAlias>`,
  or `helm template` fails. `dsp-provider.yaml` has a bare `holder.kid` only because it leaves
  `bootstrap.did` unset. The kid's right form therefore depends on the bootstrap switch and on
  `keyAlias`, which the tool does not own:
  - `identityhub.bootstrap.did` (values.yaml ~3087): exclusion, US-8.11 (identity material: the
    bootstrap block with `keyAlias`, whose fragment the DID document publishes).
  - `fdsc-edc.deployment.oid4vc.config.oid4vp.holder.kid`: exclusion, US-8.11. It is a key id, like
    `keycloak.signingKey.did`, not the bare DID.

  How a loaded file behaves follows US-3.1's loader. `load` marks a leaf as owned only inside an owned
  marker block (`classifier.owned` → `roundtrip.OwnedBy`). No `recognize:` rule matches a DID key, so
  every DID leaf outside that block is `Unknown`, and `render.pin` keeps every rendered leaf that
  overlaps an `Unknown` path at its loaded value. Two cases follow:
  - A hand-written file (such as `dsp-provider.yaml`): every DID leaf, targets included, is pinned. A
    `domain.base` change moves none of them, and nothing splits.
  - A file the tool generated, with the DID targets in its owned block, to which the user has
    hand-added `identityhub.bootstrap.did` and `holder.kid` outside the block. A `domain.base` change
    moves the owned targets (`participant.id`, `holder.id`, ...) to the new DID and keeps the
    hand-added pair on the old one. The pair stays consistent with itself, so `assertHolderKid` still
    passes. But `bootstrap.did` no longer equals `fdsc-edc.common.config.edc.participant.id`, which
    `values.yaml` (~3085) requires. That drift is accepted until US-8.11 owns the pair, and Step 2 pins
    it with a test that US-8.11 flips.

  A fresh render emits neither key, so the chart defaults (`bootstrap.did: ""`, no kid) apply.

  The five `fdsc-edc.*` targets are not feature-gated (gating is US-5.3), so every render emits them,
  even with `fdsc-edc` disabled. That is accepted until US-5.3: the keys are inert while the component
  is off. Leaving them out would split the DID in a generated file whose user later enables the
  component by hand: the hand-added `fdsc-edc.*` DID keys sit outside the owned block, so they are
  pinned on the old DID, while the owned verifier/contract-management/keycloak targets follow a
  `domain.base` change. As targets, the five keys sit in the owned block from the first generation and
  follow too. Step 2 pins this with a test, and US-5.3 replaces that test with a gated one.

  `keycloak.signingKey.*` stays unowned: a render without keystore answers emits no leaf under it.

  `dataSpaceConfig` and APISIX carry **no** DID key in chart 10.16.0 (verified in `values.yaml` and
  provider.yaml). `FEATURE_MODEL.md`'s list is aspirational. Do not invent keys.

## Deviations from the ticket
The story says the DID is used by the verifier, the IdentityHub, the issuer, the registration job,
`dataSpaceConfig` and APISIX. This plan narrows that list. Each item below is recorded in the bundle
comment above the `did.value` targets, in ADR 0021 (Step 4) and in the corrected `FEATURE_MODEL.md` row
(Step 6), so the narrowing can be accepted or rejected at PR time.

| AC consumer | What this ticket does | Reason | Owning story |
|---|---|---|---|
| Registration job: TIL `registration.issuer[0].did` | not a target | Targets may only index into a list the bundle owns whole, and the issuer list also names other participants | US-3.3 |
| Registration: the verifier's TIR curl script (`vcverifier.deployment.initContainers[0].args[0]`) | not a target | A k3s-local shell script inside a list with content no foundation owns | US-3.3 |
| `dataSpaceConfig` | nothing | Chart 10.16.0 has no DID key there | none until the chart adds one |
| APISIX | nothing | Chart 10.16.0 has no DID key there | none until the chart adds one |
| Keycloak signing key `keycloak.signingKey.did` | not a target | Emitting it alone breaks the realm key provider (see the table above); it belongs with the keystore block | US-5.5 |
| IdentityHub `identityhub.bootstrap.did` | not a target | With a bare `holder.kid` it fails the chart's `assertHolderKid`; with a `<did>#<keyAlias>` kid it needs `keyAlias`, which the tool does not own | US-8.11 |
| EDC `fdsc-edc.deployment.oid4vc.config.oid4vp.holder.kid` | not a target | A key id whose form (`<did>` or `<did>#<keyAlias>`) depends on the IdentityHub bootstrap, see the row above | US-8.11 |

## Steps

### Step 1: Model schema — `choice` and `did` types, conditional default, prefix check
Extend the foundation schema in `config-tool/internal/model/foundations.go`. This step changes no
bundle entries.
- `TypeChoice` (`choice`): the foundation declares `options: [{value, label, note}]`. `note` is optional
  text shown at the point of choice. The check accepts only a declared `value`. Because `Check` then
  needs the options, move it to `CompiledFoundation.Check(v any) error`, keep `FoundationType.Check`
  for the pattern types, and update its callers (`render.resolve`, `profile.Validate`, the server).
- `TypeDID` (`did`): `^did:[a-z0-9]+:[A-Za-z0-9._%:-]+$`, a named regexp next to `hostname`.
- `defaultWhen: {id, equals}`: the default applies only while answer `id` (resolved, so derived counts)
  equals `equals`. Add `id` to `Refs` so `topoOrder` and `checkRefs` cover it.
- `prefix`: a template over other answers that the resolved value must start with
  (`"{{ .did.method }}:"`). It is checked in `render.resolve` for given and derived values alike, with
  an error that names both ids. Its refs join `Refs`.
- Allow a foundation with no `targets` when another foundation references it (for `did.method`).
  Check what `compileFoundation` currently refuses.
- Constant default for `choice` only: `compileFoundation` refuses a default with no refs ("default
  references no answer"), which would reject Step 2's `default: did:web`. Relax it for `type: choice`
  only, and require the constant to be one of the declared `options`. Keep the check for every other
  type, because it catches typos in derived values. `render.resolve` already derives a default whose
  `Refs` are empty.
- Tests: table-driven in `internal/model/foundations_test.go` (compile errors: unknown option, choice
  without options, `defaultWhen` on an unknown id, a cycle through `defaultWhen` or `prefix`, a constant
  default on a non-choice type, a choice default that is not an option) and in
  `internal/render/render_test.go` (default applied or skipped by `defaultWhen`, prefix mismatch error,
  given answer overrides). Use fixtures in `internal/model/testdata/`, not the real bundle.
- Mirror the two types in the UI's field validation (`ui/src/state/foundations.ts` `answerErrors`), so
  the TS check matches the Go patterns. Add a vitest case.
- Document the new fields in the bundle's header comment.
- Accept when `go test ./...`, `make lint` and `npm test` are green.

### Step 2: Bundle — `did.method`, `did.value` and their targets, with provider DID coverage tests
In `config-tool/model/10.16.0/bundle.yaml` (bump `revision` 5 → 6), after `domain.base`:
```yaml
  - id: did.method
    stage: foundations
    type: choice
    default: did:web
    options:
      - {value: "did:web", label: ..., note: ...}
      - {value: "did:key", label: ..., note: "Meant for local development: ..."}
  - id: did.value
    stage: foundations
    type: did
    default: "did:web:{{ .domain.base }}"
    defaultWhen: {id: did.method, equals: "did:web"}
    prefix: "{{ .did.method }}:"
    targets: [copy of {{ .did.value }} to the 3 provider.yaml keys marked "target" above,
              and the 4 fdsc-edc copy keys;
              tpl "{{ .did.value }}-sts-client-secret" to fdsc-edc.common.config.oauth.secretAlias]
```
Label, help and `doc` (`doc/deployment-integration/roles/README.md` or the IdentityHub/DID section that
fits best) are bundle data. Add a comment above the targets that lists the deviations from the ticket
(see that section) with their owning stories.

`did.method` has no targets, so it never emits a key. An unanswered `did.value` with `did:key` emits
nothing.

Tests in `config-tool/internal/render/provider_test.go`, modelled on the domain tests:
- Generalise `exclusion` with an `answer` field, or add a separate `didExclusions` table, so each
  exclusion row names the foundation it exempts. Fill it with the excluded rows from the table above,
  with their stories.
- `TestProviderDIDKeySet`: every provider.yaml leaf that contains `did:web:mp-operations.org` is a
  `did.value` target or excluded, every `did.value` target is such a leaf or a listed chart-only
  target, and stale exclusion rows fail.
- `TestProviderValues`: rendered leaves absent from provider.yaml are allowed only if they are in a
  `chartOnlyTargets` table (path → reference file, `k3s/dsp-provider.yaml` for `fdsc-edc.*`). The
  same test asserts their values against that file: all five `fdsc-edc.*` targets, `oauth.clientId` and
  `oauth.secretAlias` included, against `dsp-provider.yaml`. This replaces the `orgDisplayName` special
  case.
- `TestChartOnlyTargetsUngated`: a provider render with `fdsc-edc` off still emits the five
  `fdsc-edc.*` DID targets. It pins the accepted ungated state, and its comment says US-5.3 replaces it.
- `TestHolderKidMatchesBootstrapDID`, mirroring `dsc.identityhub.assertHolderKid`: whenever the merged
  output (targets plus pinned loaded keys) holds both `identityhub.bootstrap.did` and any
  `fdsc-edc.{common,deployment.*}.config.oid4vp.holder.kid`, each kid equals
  `<bootstrap.did>#<keyAlias>` (`keyAlias` from the output, else the chart default `key-1`). Cases: a
  fresh render (neither key, so it passes trivially), and a tool-generated fixture in `testdata/`: an
  owned marker block with the DID targets, plus a hand-added `identityhub.bootstrap.did: <old did>`
  and `holder.kid: <old did>#key-1` outside it, loaded with a changed `domain.base`. For that fixture,
  assert that the kid check holds, and pin the accepted drift explicitly: `bootstrap.did` stays the
  old DID while `fdsc-edc.common.config.edc.participant.id` is the new one. The test comment says
  US-8.11 flips that assertion. A hand-written `dsp-provider.yaml` is not a case here, because it pins
  every DID leaf and so tests nothing. Also assert that no render emits either key as a `did.value`
  target.
- No `keycloak.signingKey.*` leaf: a render without keystore answers emits nothing under
  `keycloak.signingKey`, so the chart's `signingKey: {}` default and the `${DID}` fallback stay intact.
- `TestProviderDIDChange`: changing `domain.base` changes exactly the domain leaves plus the DID
  targets while `did.value` is derived, and no DID target once `did.value` is given.
- `did:key` with no `did.value` emits no DID key.
- Update `internal/server` tests that pin the bundle revision or the foundation count.
- Accept when `go test ./...` and `make lint` are green.

### Step 3: Pinned answers — core listing and the Review note
"Editing the DID by hand pins it: changing `domain.base` afterwards does not re-derive it, and Review
says so." `resolve` already keeps a given answer. What is missing is saying so.
- Core: `render.HandSet(b model.Bundle, p profile.Profile) []HandSetAnswer` (name it as you see fit,
  but do not reuse `Pinned`, which means a loaded-file hand edit). It lists every given answer whose
  foundation has a default that references other answers (non-empty `Refs`), in `FoundationOrder`,
  with `ID`, `Value` and `Derived`. A constant `choice` default such as `did.method`'s is left out:
  picking a radio is not pinning. `Derived` is what the default would give now, and empty when its refs
  or `defaultWhen` do not hold. Table-driven test: a hand-set DID survives a `domain.base` change and
  `Derived` follows the new domain. Hosts are covered the same way. A given `did.method` is not listed.
- API: add `handSet` to the `POST /api/render` response (`renderResponse` in
  `internal/server/render.go`). Add a server test.
- UI: `ui/src/stages/Review.tsx` gets a "Set by hand" section, one row per entry: label, value, and
  "would be `<derived>` if derived" when that differs. The row links to the field with the existing
  `fieldElementId`/focus mechanism (open field id `answers.<id>`). Wording is UI copy. Add a vitest case.
- Accept when `go test ./...`, `npm test` and `make lint` are green.

### Step 4: Minimal L2 rule engine in the core, with the `did:key` in production rule, and ADR 0021
- Model (`internal/model`): add the type `CheckRule` (`model.Rule` is taken by `recognize:`), read from a
  bundle `rules:` list:
  ```yaml
  rules:
    - id: did-key-in-production
      severity: warning            # warning | error
      stage: foundations           # model.Stage, for the "Stage 3" label and the link
      field: answers.did.method    # open-field id the finding points at
      when:                        # all must hold
        - {field: target.environment, equals: production}
        - {field: answers.did.method, equals: "did:key"}
      message: >-
        A did:key cannot be resolved by other participants over the web ...
      doc: doc/deployment-integration/roles/README.md#...
  ```
  Validate at bundle load: the id is unique, the severity is known, the stage is known, `field` and
  every `when.field` are `target.environment` or an `answers.<declared foundation id>` (keep the set of
  addressable fields a named table), and `doc` is a repo-relative path. Accessor `Bundle.CheckRules()`.
  The trust-anchor bundle (`model/1.3.6`) has none.
- Core package `internal/check` (no `net/http`; `layering_test.go` covers it): `Evaluate(b, p)
  []Finding`. `Finding` carries `Rule`, `Severity`, `Stage`, `Field`, `Message` and `Doc`.
  Conditions read **resolved** answers (derived defaults count), so export a small
  `render.Answers(b, p)` wrapping `resolve`. Findings come back sorted (severity, stage, rule id).
  `Blocking(findings) bool` is true only for `error`.
- Bundle: append the rule to `model/10.16.0/bundle.yaml` (revision bump if Step 2 has merged in the
  same release; otherwise one bump covers both). Update the header comment that says "Value rules come
  with Epic 3's questions".
- Tests: a table-driven test for the rule against the real bundle: `development+did:web`,
  `development+did:key` and `production+did:web` give none; `production+did:key` gives exactly one
  warning on `answers.did.method`, stage `foundations`; `production` with `did.method` unanswered gives
  none, because it derives `did:web`. Add a model test for the compile errors with testdata fixtures,
  and an evaluator test with a fixture bundle that has an `error` rule for `Blocking`.
- ADR `doc/adr/0021-config-tool-l2-rules.md` (Context / Decision / Consequences, like 0020). It covers
  rules as bundle data, the conjunctive `equals` conditions (extend with new operators when a rule
  needs one), warning vs error, findings addressed by open-field id, and why this lives in
  `internal/check` and not `internal/validate` (L1 schema). It also carries the "Deviations from the
  ticket" table of this plan, because the DID target list is the first foundation that narrows an AC,
  and the loaded-file consequence as stated in "Facts" above: a hand-written file pins every DID leaf,
  and only a generated file with a hand-added bootstrap pair drifts (`bootstrap.did` ≠
  `participant.id`) until US-8.11.
  Add it to `doc/adr/README.md`. In
  `FEATURE_MODEL.md` §L2, note that the rules are in `bundle.yaml` `rules:` instead of a separate
  `rules.yaml`.
- Accept when `go test ./...` and `make lint` are green.

### Step 5: Findings in the API, the CLI and Review (warnings never block)
- `POST /api/render`: the response gains `findings` (from `check.Evaluate`). They are computed even
  when the tree is unchanged. A warning never turns the response into an error. Add a server test for
  `production+did:key`: 200, the tree is present, and there is one finding.
- CLI `dsc-config generate` (`internal/cli/cli.go` `renderProfile`): print each finding to stderr as
  `warning: Stage 3 Foundations, <field>: <message> (<doc>)`, and exit 0 for warnings. Exit non-zero
  only when `check.Blocking`. Add a CLI test with a production `did:key` profile in testdata.
- UI: `ui/src/stages/Review.tsx` gets a "Findings" section. Each row shows the severity, "Stage 3 ·
  Foundations", the field label (from the foundations list), the message and a "Read more" link to
  `doc`. A "Go to field" link uses the focus mechanism. `ui/src/stages/Foundations.tsx` shows a finding
  inline under its field, from the render response it already fetches. Neither disables Next or
  Generate. Add vitest cases for both.
- Accept when `go test ./...`, `npm test` and `make lint` are green.

### Step 6: Stage 3 Organization panel — the DID, its method and its reach; docs
- Model/API: add an optional `group: organization | network` (default `network`) to the foundation
  schema. Set `group: organization` on `org.displayName`, `did.method` and `did.value`. Serve `group`,
  `options` and `defaultWhen` in `foundationBody` (`GET /api/foundations`). `Foundations.tsx` groups by
  `group` instead of the `ORGANIZATION_PREFIX`/`default === ''` heuristic, so `did.value` (which has a
  default) does not land among the hosts.
- UI in the Organization panel, reusing the `Hosts` override pattern:
  - `did.method` is a radio group from `options`. Each option's `note` shows beside it; the `did:key`
    note says it is meant for local development. Switching the method drops a `did.value` answer whose
    prefix no longer matches, so `did:web` re-derives and `did:key` asks for input.
  - `did.value` shows the resulting DID: derived and read-only-looking with an "Edit" action, or the
    hand-set value with "Reset to derived". Below it: "Used by N values in M components"
    (`count`/`components` from `GET /api/foundations`). The components list expands.
  - The inline finding from Step 5 sits under the method.
  - All strings are UI copy constants or bundle data. There are no magic numbers.
- Tests: vitest for grouping, method switch clearing a mismatched value, edit → hand-set → reset, and
  the used-by text. Add a Go test for `group` compile and default.
- Docs: `config-tool/README.md` (Stage 3 DID, the rules/findings, `generate` warnings),
  `doc/config-tool/USER_JOURNEY.md#stage-3--foundations` if the panel wording changed, and in
  `doc/config-tool/FEATURE_MODEL.md` correct the `did.method / did.value` row to the real target list
  and add the deviations (registration job and TIR script → US-3.3, `dataSpaceConfig`/APISIX have no
  key, `keycloak.signingKey.did` → US-5.5, `identityhub.bootstrap.did` and `holder.kid` → US-8.11).
  In `AGENTS.md`, mark US-3.2 shipped with one line. Its planned-entry wording "only 4 are owned
  scalars" becomes 3, since `keycloak.signingKey.did` is excluded.
- Accept when `make ui`, `make build`, `make test` and `make lint` are green. Check the UI by hand with
  `make build && ./dsc-config`: set the domain, see the derived DID and its count, switch to `did:key`
  with production selected, see the warning in Stage 3 and Review, and confirm generation still
  proceeds.
