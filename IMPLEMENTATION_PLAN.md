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
  | `keycloak.signingKey.did` | **target** |
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
  - `identityhub.bootstrap.did` (values.yaml ~3087: "DID of this participant ... must match").
  - `fdsc-edc.*`, taken from `k3s/dsp-provider.yaml`: `fdsc-edc.common.config.edc.participant.id`,
    `fdsc-edc.common.config.issuerId`, `fdsc-edc.deployment.oid4vc.config.oid4vp.holder.id`, and possibly
    `.holder.kid` (`<did>#<key>`, a `tpl`). Check each value in `dsp-provider.yaml` before adding it.

  `dataSpaceConfig` and APISIX carry **no** DID key in chart 10.16.0 (verified in `values.yaml` and
  provider.yaml). `FEATURE_MODEL.md`'s list is aspirational. Record this in the bundle comment and the
  ADR, and do not invent keys.

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
- Tests: table-driven in `internal/model/foundations_test.go` (compile errors: unknown option, choice
  without options, `defaultWhen` on an unknown id, a cycle through `defaultWhen` or `prefix`) and in
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
    targets: [copy of {{ .did.value }} to every key marked "target" above + identityhub/fdsc-edc keys]
```
Label, help and `doc` (`doc/deployment-integration/roles/README.md` or the IdentityHub/DID section that
fits best) are bundle data. Add a comment above the targets that states why `dataSpaceConfig`/APISIX
have none.

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
  `chartOnlyTargets` table (path → reference file, e.g. `k3s/dsp-provider.yaml` for `fdsc-edc.*`,
  `charts/data-space-connector/values.yaml` for `identityhub.bootstrap.did`). The same test asserts their
  values against that file. This replaces the `orgDisplayName` special case.
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
  foundation has a default, in `FoundationOrder`, with `ID`, `Value` and `Derived`. `Derived` is what the
  default would give now, and empty when its refs or `defaultWhen` do not hold. Table-driven test: a
  hand-set DID survives a `domain.base` change and `Derived` follows the new domain. Hosts are covered
  the same way.
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
  `internal/check` and not `internal/validate` (L1 schema). Add it to `doc/adr/README.md`. In
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
  `doc/config-tool/FEATURE_MODEL.md` correct the `did.method / did.value` row to the real target list.
  In `AGENTS.md`, mark US-3.2 shipped with one line.
- Accept when `make ui`, `make build`, `make test` and `make lint` are green. Check the UI by hand with
  `make build && ./dsc-config`: set the domain, see the derived DID and its count, switch to `did:key`
  with production selected, see the warning in Stage 3 and Review, and confirm generation still
  proceeds.
