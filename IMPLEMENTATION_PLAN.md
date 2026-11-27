# Implementation Plan: [E4 Infrastructure] US-4.1 — Reuse an existing cert-manager (the three-mode model, Stage 4)

## Overview
Stage 4 gets its first dependency, cert-manager, with three modes: `bundled` (installed with the connector), `cluster`
(already in the cluster, asks for the ClusterIssuer name only) and `external` (certificates provided as one TLS Secret
per host). The generic `infrastructure:` section of the bundle, an `ingresses:` table, the profile's
`infrastructure` map and the Stage 4 screen are the model that US-4.2 to US-4.6 extend. The bundle compiles every
dependency into ordinary compiled foundations with mode-gated targets. Render, load ownership, pinning, provenance and
requirements therefore keep working without a second engine (ADR 0026).

## Decisions settled by this plan (record them in ADR 0026)
- **Answer namespace.** The profile stores `infrastructure: {<dep>: {mode, answers: {<question>: value}}}` (local
  question ids, as the ticket asks). The model compiles each dependency `<dep>` into synthetic compiled foundations
  with the reserved id prefix `infra.`:
  - `infra.<dep>.mode`, a choice of the dependency's modes. It has no default, because a never-chosen mode must
    render nothing.
  - `infra.<dep>.<question>`, one for each question of each mode.

  Bundle templates refer to them fully qualified (`{{ .infra.certManager.issuer }}`), so `parseTemplate`, `checkRefs`
  and `topoOrder` work unchanged. A declared foundation id may not start with `infra.`, and `profile.answers` may not
  hold an `infra.*` id: those answers live only under `infrastructure`. Dependency and question ids must match
  `idSegment`, so the dependency is `certManager`, not `cert-manager`.
- **Synthetic foundations are not Stage 3 foundations.** They are part of the compiled list (`FoundationOrder`, `Foundation(id)`)
  but not of the declared `b.Foundations`. Every `range b.Foundations` site must decide whether it covers them:
  `render/render.go:41`, `render/elements.go:99,324`, `render/diff.go:120`, `load/elements.go`, `check/coverage.go:70`
  and `server/render.go:194`. Render, load and diff need to see them, so they iterate a new
  `b.CompiledFoundations()`. `/api/foundations` must not list them. A test pins that Stage 3's list does not change.
- **Mode gating.** A compiled target gains `When []Condition` (all must hold over the resolved answers). Each mode
  target gets `{infra.<dep>.mode equals <mode>}`, and a question's `when` (for example `type equals prod`) is
  ANDed. `Only` stays as it is. `checkTargetPaths` lets two targets share a leaf path when they are *exclusive*,
  meaning both carry a condition on the same id with different `equals`. `Only` counts as a condition on the target's own
  foundation id. So `cert-manager.enabled` can be written by all three modes, and the
  `cert-manager.io/cluster-issuer` annotation by `bundled` and `cluster`.
- **Replacing a host's TLS Secret name.** The host foundations (US-3.4) already write `<ingress>.tls[0].secretName`
  as `<host>-tls` (`apisix-provider-tls` for APISIX). The `external` mode's per-host questions write the same path
  with `replaces: true`. At compile time, an unconditional foundation target must exist at exactly that path, and
  the question's default is copied from that target's template, so a derived external answer equals today's value.
  At render time, a rendered replacing leaf drops the replaced leaf, and provenance names the infrastructure answer.
  Nothing else may share a path with an unconditional target. When the mode is switched away from `external`,
  `<host>-tls` comes back on its own.
- **One `ingresses:` table** (top level, reusable by the Ingress/DNS story) lists every ingress whose TLS the tool
  owns: `{id, label, path, owner}`, where `path` is the ingress block (for example `keycloak.ingress`) and `owner` is
  the foundation whose `<path>.tls[0].secretName` target exists. A mode's `perIngress:` targets (relative `key` plus
  fn/args) and `perIngressQuestion:` are expanded per row at compile time. They carry the refs of the owner's
  secretName target as `requires`, so an annotation renders only on an ingress that renders. Compile checks run both
  ways: every row's owner target exists, and every foundation target ending in `.tls[0].secretName` has a row. The
  verifier (`vcverifier.ingress`) and DID (`did.ingress`) ingresses are not rows: their TLS certificate is the signing
  key, owned by US-8.11. They are added to `provider_test.go`'s exclusions for the annotation, with story US-8.11.
- **cert-manager modes** (keys from `charts/data-space-connector/values.yaml` L3681-3708,
  `templates/cert-manager-issuer.yaml`, `k3s/operators.yaml` L57-70, `k3s/provider.yaml` L1-8):

  | Mode | Mode targets | Questions → targets |
  |---|---|---|
  | `bundled` | `cert-manager.enabled: true`, `cert-manager.crds.enabled: true` (the issuers need the CRDs; `k3s/operators.yaml` sets it), `certManagerResources.enabled: true` | `type` choice `selfsigned`/`prod`, constant default `selfsigned` (the chart default `prod` with `admins@example.com` must never apply silently) → `certManagerResources.type`; `acmeEmail` (when `type=prod`, new type `email`, no default) → `certManagerResources.acmeEmail`, guarded by the error rule `acme-email-for-prod-issuer` (see below); `ingressClass` (when `type=prod`) → `certManagerResources.ingressClass`; `namespace` (when `type=selfsigned`, `readOnly`, default `{{ .target.namespace }}`: a bundled cert-manager runs in the release namespace, where the CA ClusterIssuer finds `ca-secret`) → `certManagerResources.namespace`; per ingress `annotations."cert-manager.io/cluster-issuer"` = `{{ .infra.certManager.type }}-issuer` |
  | `cluster` | `cert-manager.enabled: false`, `certManagerResources.enabled: false` | exactly one: `issuer` (ClusterIssuer name, DNS-1123 subdomain) → per ingress `annotations."cert-manager.io/cluster-issuer"` (copy) |
  | `external` | `cert-manager.enabled: false`, `certManagerResources.enabled: false` | per ingress `secret.<ingressId>` (type `secretName`, default from the owner target) → `<path>.tls[0].secretName`, `replaces: true` |

  `<type>-issuer` matches the chart's ClusterIssuer names (`prod-issuer`, `selfsigned-issuer`), and a test reads
  `templates/cert-manager-issuer.yaml` to pin this. Namespaced `Issuer`s are out of scope, because the one cluster question names a
  ClusterIssuer (the help text says so, and a hand-set `cert-manager.io/issuer` stays an override). In `external`, the
  host foundations' `cert-manager.io/common-name`/`alt-names` annotations stay: they are owned by US-3.4 and inert
  without an issuer annotation. `cert-manager.io/private-key-algorithm` in k3s is not owned.
- **No silent ACME contact.** An unanswered question emits no key. If `acmeEmail` were left open (or marked *Ask
  later*) in `bundled` + `prod`, the chart's `admins@example.com` would apply, and ACME refuses example.com contacts,
  so `prod-issuer` would never register. Two guards, both bundle data or type checks:
  - a new foundation type `email` (`local@domain`, where the domain is a `TypeDomain` and is not an RFC 2606
    reserved name: `example.{com,net,org}` or the `.example`/`.test`/`.invalid`/`.localhost` TLDs). A typed address
    can therefore never be the chart default or another placeholder;
  - the check rule `acme-email-for-prod-issuer` (severity `error`, stage `infrastructure`, field
    `infrastructure.certManager.acmeEmail`), which fires when `infrastructure.certManager` equals `bundled`,
    `infrastructure.certManager.type` equals `prod` and `infrastructure.certManager.acmeEmail` is unset. As an error
    it blocks `generate` (ADR 0021). The rule reads the resolved answers, so `type`'s default `selfsigned` never
    triggers it.

  Check rules may therefore address infrastructure fields: `when` and `field` accept the single field-id scheme
  below, and the bundle compile resolves each id to its synthetic foundation (an unknown dependency or question is a
  compile error).
- **Notes are bundle data.** A dependency may carry `clusterWide: true`. Its `bundled` mode must then carry
  `warning` and `doc` (compile check). For cert-manager these are "A cluster can only have one cert-manager…" and
  `doc/deployment-integration/roles/PREREQUISITES.md#installing-the-operators-once`. A mode may carry
  `development: true` plus `productionDoc` (*suitable for development*). FEATURE_MODEL.md marks only the databases
  and Vault, so cert-manager's `bundled` is **not** flagged. The schema, the API and the UI support the flag, and a
  fixture bundle tests it. The `selfsigned` option's `note` says it is for testing only. The hard refusal when
  another release owns cert-manager is US-4.6. Cluster detection is not in this ticket.
- **Requirements.** In `external`, each per-ingress Secret is listed once in `requirements.Secrets` (for
  NEXT_STEPS.md) with keys `tls.crt` and `tls.key`, and only while the mode is `external`. Requirement rows gain
  the same `When` gating and an optional `keys` list (exactly one of `key`/`keys`).
- **Profile.** Answers of inactive modes stay in the profile, so switching back restores them. Only the active mode
  renders. Validation refuses an unknown dependency, a mode the dependency does not offer, and a question id no mode
  of the dependency declares, and checks each value with its question's type. Still `format: 1`.
- **One field-id scheme**, like `answers.<id>` today, used everywhere a field is named: `FieldError`s,
  open/*Ask later*, check-rule `field`/`when`, and the UI's highlight and finding links.
  - `infrastructure.<dep>` is the mode and maps to the synthetic `infra.<dep>.mode`;
  - `infrastructure.<dep>.<question>` is a question and maps to `infra.<dep>.<question>`.

  `model` owns the mapping (constant `InfrastructureFieldPrefix` beside `AnswerFieldPrefix`, and
  `InfraFieldAnswer(field) (id string, ok bool)` / `InfraAnswerField(id)`), so profile, check, server and UI share
  one table. `mode` is a reserved question id: a question named `mode` would collide with the synthetic
  `infra.<dep>.mode` (compile error). `infra.*` is never addressable as `answers.<id>`: `profile.declared` (used by
  both `validateAnswers` and `isKnownField`) refuses ids under `infra.`, so `answers.infra.certManager.issuer` is
  neither a valid answer nor a valid open field once `Bundle.Foundation` resolves synthetic ids.
- **Load.** Owned infra leaves derive flat `infra.*` answers, and `load/seed.go` moves them into
  `profile.Infrastructure`. The mode is derived only when the derived infra answers belong to exactly one mode
  (`issuer` → `cluster`, `secret.*` → `external`, `type`/`cert-manager.enabled: true` → `bundled`). Otherwise it
  stays unanswered, and the marker reports it as underived (the existing `Marker.Underived`). Recognizing
  cert-manager in hand-written files is US-4.6 detection, not this ticket.

## Steps

### Step 1: Model — `infrastructure:` and `ingresses:` schema, compiled into mode-gated foundations
Code only, tested against fixture bundles. The shipped bundle does not change yet, so the build stays green.
- `config-tool/internal/model/infrastructure.go` (new) holds the declared types, with one-line GoDoc and named constants for
  the `infra` prefix and the mode ids (`ModeBundled`, `ModeCluster`, `ModeExternal`):
  - `Dependency{ID, Title, Help, Doc, ClusterWide, Modes []Mode}`;
  - `Mode{ID, Label, Note, Warning, Doc, Development, ProductionDoc, Targets []Target, Questions []Question, PerIngress []IngressTarget, PerIngressQuestion *IngressQuestion, Requirements}`;
  - `Question` = a foundation-like `{id, type, label, help, doc, options, default, when *Condition (local id), readOnly, targets}`;
  - `Ingress{ID, Label, Path, Owner}`.
  
  Add `Infrastructure []Dependency` (`yaml:"infrastructure"`) and `Ingresses []Ingress` (`yaml:"ingresses"`) to `Bundle` in `model.go`.
- The compile step, called from `readBundle` after `compileFoundations`, produces synthetic `CompiledFoundation`s:
  - the mode choice;
  - one foundation per question, with local `when` ids qualified;
  - the per-ingress expansion with `requires` taken from the owner target;
  - `replaces` with its default copied from the owner target.

  Re-run `checkIDNesting`, `checkRefs`, `topoOrder` and `checkTargetPaths` over the combined list. Extend
  `CompiledTarget` with `When []Condition` and `Replaces bool`, and teach `checkTargetPaths` the exclusivity and
  `replaces` rules from the decisions. Add `Bundle.CompiledFoundations()`, `Bundle.Dependencies()` and
  `Bundle.Dependency(id)`.
- Add `TypeEmail` to `foundations.go` (pattern and RFC 2606 refusal as in the decisions; `Check` never echoes the
  value), with table rows for valid addresses, `admins@example.com`, `a@b.test`, a missing `@` and a single-label
  domain. Add `InfrastructureFieldPrefix`, `InfraFieldAnswer` and `InfraAnswerField` to `checkrules.go`, and let
  rule fields address them (stage `infrastructure`).
- Validation errors (one table row each in `internal/model/infrastructure_test.go`, fixture bundles under
  `internal/model/testdata/`):
  - a duplicate dependency, mode or question id;
  - a mode id outside the three;
  - a declared foundation id under `infra.`;
  - an ingress row whose owner has no `<path>.tls[0].secretName` target;
  - an owned `tls[0].secretName` without a row;
  - a non-exclusive shared path;
  - `replaces` without an unconditional foundation target;
  - `clusterWide` without a `bundled` warning or doc;
  - `development` without `productionDoc`;
  - a question `when` naming an unknown question;
  - a question with the reserved id `mode`;
  - a check rule `field`/`when` naming an unknown `infrastructure.<dep>[.<q>]`, or an `answers.infra.*` field.

  Also test that `b.Foundations` (the Stage 3 list) is unchanged and that `CompiledFoundations()` contains the synthetic ids.
- Update the bundle header comment in `config-tool/model/10.16.0/bundle.yaml` with the `infrastructure:` and
  `ingresses:` grammar. Comment only; no data yet, so `revision` stays.
- Write `doc/adr/0026-config-tool-infrastructure-modes.md` (context, the decisions above, consequences), and add
  it to `doc/adr/README.md`.
- Acceptance: `go test ./internal/model/...` and `make lint` are green, and the existing tests are untouched.

### Step 2: Profile — the `infrastructure` map
- `config-tool/internal/profile/format.go`: add `Infrastructure map[string]InfraChoice` with
  `json:"infrastructure,omitempty" yaml:"infrastructure,omitempty"`, where `InfraChoice{Mode string; Answers map[string]any}`
  (`answers` omitempty). It is strictly decoded and marshalled deterministically (sorted keys, as `Answers`).
- `Profile.Validate` gains `validateInfrastructure(b)`, which reports with the one field-id scheme of the decisions:
  `infrastructure.<dep>` for the mode and `infrastructure.<dep>.<q>` for an answer. It refuses:
  - an unknown dependency;
  - a mode the dependency does not offer;
  - a question no mode declares;
  - a value its question's type refuses.

  `declared` refuses ids under `infra.`, so `validateAnswers` refuses an `infra.*` answer and `isKnownField`'s
  `answers.` branch refuses `answers.infra.*`. `isKnownField` accepts `infrastructure.<dep>` and
  `infrastructure.<dep>.<q>` through `model.InfraFieldAnswer`, the same ids `FieldError` uses. Add the helper `Profile.InfraAnswers() map[string]any`, which returns the flat
  `infra.<dep>.mode`/`infra.<dep>.<q>` view that render and check consume, plus `SetInfraMode` and `SetInfraAnswer`
  (both return a copy and keep the answers of other modes).
- Check how `ApplyPreset` and the chart switch in `role.go` carry answers over, and carry `infrastructure` over the
  same way within a chart (drop it on a chart switch, like foundation answers that the new bundle does not declare).
- Tests (table-driven) in `internal/profile/format_test.go` and a new `infrastructure_test.go`:
  - the strict decode refuses unknown keys under an entry;
  - save → open round-trips the mode and answers byte-identically;
  - each refusal above;
  - open-field ids, and that a refused infrastructure value's `FieldError.Field` equals its open id;
  - `answers: {infra.certManager.issuer: x}` is refused and `open: [{field: answers.infra.certManager.issuer}]` is
    not a known field;
  - answers survive a mode switch.

  Use a fixture bundle with an `infrastructure:` entry (`internal/profile/testdata`).
- Append an "Infrastructure" addendum to `doc/adr/0015-config-tool-profile.md` (the shape, the id scheme, kept
  inactive-mode answers, no credential ever, still `format: 1`).

### Step 3: Render — mode-gated targets, replacing leaves, requirements
- `config-tool/internal/render/render.go`:
  - `resolve` merges `p.InfraAnswers()` into the given answers.
  - `Render`, `composeLists`, `eachLeaves` and `diff.go` iterate `b.CompiledFoundations()`.
  - A target renders only while all of its `When` conditions hold. An unchosen mode therefore renders nothing.
  - A rendered `Replaces` leaf removes the replaced foundation leaf at the same path, and `Provenance.Foundation` is the
    infra id. `pin` treats the replacing leaf like any other.
  - `render.Targets(b, id)` and `TargetRef` cover synthetic ids, so Stage 4 can show the keys of each mode.
- `internal/load/elements.go` and `check/coverage.go` switch to `CompiledFoundations()` where ownership is
  meant, and the load inversion (`invert.go`) handles conditioned targets. `load/seed.go` moves derived `infra.*`
  answers into `profile.Infrastructure` and derives the mode as described in the decisions.
- `internal/model/requirements.go` and `internal/requirements`: add `When` gating and `keys`, and list each Secret once.
- `internal/check/check.go`: `holds` and `value` resolve `infrastructure.<dep>[.<q>]` fields through
  `model.InfraFieldAnswer` against the resolved answers (which include `p.InfraAnswers()`), and a finding's `field` is
  that same id. Table rows in `check_test.go` with a fixture rule: `set`/`unset`/`equals` on a mode and on a question.
- Tests with a fixture bundle (`internal/render/testdata`) holding a three-mode dependency:
  - an unchosen mode emits no key;
  - each mode emits only its keys;
  - shared paths with different values per mode;
  - `replaces` restores the foundation leaf when the mode changes;
  - `when`-gated questions;
  - a loaded file whose `dsc-config:owned infra.*` keys belong to the old mode loses them in the re-render, while a
    hand-set (unknown) annotation at an owned path stays pinned;
  - load derives the mode only when it is unambiguous.
- Acceptance: the existing render, load and check suites pass unchanged, and `go test ./...` is green.

### Step 4: The cert-manager entry in bundle 10.16.0
- In `config-tool/model/10.16.0/bundle.yaml`, add `ingresses:` with ten rows (`apisix` owned by `domain.base`, plus
  `keycloak`, `statusList`, `til`, `ccs`, `pap`, `scorpio`, `tmForumApi`, `marketplace`, `dashboard`, each owned by
  its `host.*`), and the `infrastructure:` entry `certManager` with the title "TLS certificates (cert-manager)",
  `clusterWide: true`, the mode labels of MOCKUPS.md#s4--infrastructure ("Install cert-manager with the connector",
  "Use the cert-manager already in the cluster", "I provide a TLS Secret per host"), and the questions and targets
  of the decision table. Bump `revision: 18` → `19`. The trust-anchor bundle (`model/1.3.6`) gets no entry.
- Tests in `config-tool/internal/render/infrastructure_test.go` (new), using the shipped bundle:
  - **Exact key set per mode (table-driven, one row per mode):** render with every question of the mode answered
    and every host derived from `domain.base`. The set of leaves whose provenance is `infra.certManager.*` must equal
    the paths that the compiled mode declares (expanded per ingress). No path declared *only* by the other two modes
    may appear. One sub-row per mode checks the values (`true`/`false`, `selfsigned-issuer`, `prod-issuer`, the
    Secret names).
  - **`cluster` asks exactly one question:** `len(b.Dependency("certManager").Mode("cluster").Questions) == 1`,
    and it is `issuer`.
  - **Mode switch:** cluster → external → cluster, rendering each time. No annotation is left in `external`, and the
    `tls[0].secretName` leaves fall back to `<host>-tls` in `cluster`.
  - **Unanswered:** a profile without `infrastructure` renders exactly as before (compare with the pre-existing
    golden render of `provider.dscprofile.yaml`).
  - **Issuer names:** `<type>-issuer` is a `ClusterIssuer` name under the matching `type` branch of
    `charts/data-space-connector/templates/cert-manager-issuer.yaml`.
  - **References:** `bundled` with `type: selfsigned` and target namespace `operators` reproduces
    `k3s/operators.yaml`'s `cert-manager.enabled`, `cert-manager.crds.enabled` and `certManagerResources.*`.
    `startupapicheck.enabled` is the subchart default and is excluded with a reason. Extend
    `internal/render/testdata/provider.dscprofile.yaml` with `infrastructure: {certManager: {mode: cluster, answers: {issuer: selfsigned-issuer}}}`,
    so `provider_test.go` checks every rendered annotation and both `enabled: false` keys against
    `k3s/provider.yaml`. Add exclusion rows for the verifier and DID ingress annotations (US-8.11), and for
    `cert-manager.io/private-key-algorithm`, which is not owned.
  - **Requirements:** `external` lists ten Secrets with `tls.crt`/`tls.key`. `cluster` and `bundled` list none.
  - **ACME contact (table-driven, `check.Evaluate` + `check.Blocking`):** add the rule `acme-email-for-prod-issuer` to
    `rules:`. It blocks for `bundled` + `prod` with `acmeEmail` unanswered, and also when the field is marked *Ask
    later*. It does not block for `bundled` + `prod` with an email, `bundled` + `selfsigned` (or `type` unanswered,
    which defaults to `selfsigned`), `cluster` or `external`. A further row asserts that no render of `bundled` + `prod`
    ever emits `certManagerResources.acmeEmail: admins@example.com`, and that the profile refuses
    `acmeEmail: admins@example.com` with field `infrastructure.certManager.acmeEmail`.
- Acceptance: `go test ./...` is green and the model test confirms the revision bump.

### Step 5: Server API and CLI
- `config-tool/internal/server`: `GET /api/infrastructure?chart=&version=` (new handler in `infrastructure.go`,
  registered in `api.go`, token-protected like the rest). For each dependency it returns `{id, title, help, doc,
  clusterWide}` and its modes. Each mode carries `{id, label, note, warning, doc, development, productionDoc, keys}`
  and its `questions`. A question carries `{id, type, label, help, doc, options, default, when, readOnly, ingress}`.
  `keys` is `render.Targets` of the mode's foundations, which shows "what this writes". No text is composed in Go:
  every string is bundle data. `/api/render` already takes the profile, so the infrastructure answers reach the render,
  the findings and the secrets without a new endpoint. `PUT /api/profile` validates them through `Profile.Validate`.
- Tests in `internal/server/infrastructure_test.go`:
  - the response shape for 10.16.0;
  - 404 for an unknown bundle;
  - an empty list for 1.3.6;
  - `/api/render` with each mode returns the mode's keys;
  - `PUT /api/profile` refuses an unknown mode with the field id.
- CLI: `dsc-config generate -p <profile>` (`internal/cli`) already validates and renders the profile. Add a table test
  that a profile with each cert-manager mode validates and renders, that an invalid mode fails with the field id, and
  that `bundled` + `prod` without `acmeEmail` fails `generate` with the `acme-email-for-prod-issuer` finding.
- Keep `internal/layering_test.go` green (no `net/http` in the core).

### Step 6: Stage 4 UI — `ui/src/stages/Infrastructure.tsx`
- Replace the Stage 4 placeholder in `config-tool/ui/src/stages.ts` with `Infrastructure` (one-line JSDoc). Add
  `ui/src/api` types plus a fetcher for `/api/infrastructure`, and state actions in `ui/src/state` that set the mode
  and set an answer on `profile.infrastructure` (keeping inactive answers). Feed the live render the way
  `Foundations.tsx` does.
- One card per dependency, laid out as in MOCKUPS.md#s4--infrastructure:
  - the title, and a radio group with the three mode labels (a fieldset/legend; keyboard-operable);
  - beside each radio, the mode's `note`, plus a *suitable for development* badge and a production-guidance link while
    `development` is true;
  - the chosen mode's `warning` with a link to its `doc` (PREREQUISITES.md#installing-the-operators-once) for `bundled`;
  - the chosen mode's questions revealed below the radios, reusing `AnswerField.tsx` (choice, string, secretName).
    `readOnly` questions show their derived value, `when`-gated questions appear only while the condition holds, and
    `external` shows one row per ingress (label, Secret name);
  - *Ask later* (`chrome/AskLater.tsx`) on the mode and on each question, using the open ids from Step 2;
  - an optional "writes" disclosure listing the mode's keys.

  UI copy that is not bundle data goes into `ui/src/stages/infrastructureText.ts`, like the other `*Text.ts` files.
- `ui/src/state/issues.ts` counts these as Stage 4 open questions: an unchosen mode, plus the unanswered questions
  (no derived default) of the active mode whose `when` holds. Inactive and hidden questions never count.
- `Infrastructure.test.tsx` (table-driven where it fits):
  - three radios render;
  - choosing `cluster` reveals exactly one field (`issuer`);
  - `bundled` shows the warning with the PREREQUISITES link, and `acmeEmail` appears only for `prod`;
  - `external` lists one field per ingress;
  - switching modes hides the previous fields but keeps their values;
  - *Ask later* marks the field open and the rail count follows;
  - a refused value and a finding (`acme-email-for-prod-issuer`) highlight the same field the *Ask later* control
    marks, because the UI uses the one field id for all three;
  - a fixture with `development: true` shows the badge and the production link.

  Update `Chrome.test.tsx` if the placeholder text was asserted.
- Acceptance: `npm test`, `npm run lint` (via `make lint`) and `make ui` are green.

### Step 7: Documentation and final verification
- `config-tool/README.md`: a new "Infrastructure (Stage 4)" section. It covers the `infrastructure:` and `ingresses:`
  grammar, the three modes and their keys for cert-manager (the decision table), `when`, `replaces` and `perIngress`,
  the `infra.` namespace and the one `infrastructure.<dep>[.<q>]` field-id scheme, the `email` type and the ACME rule, `GET /api/infrastructure`, and the rule that a never-chosen mode emits no key. Link
  ADR 0026.
- `doc/config-tool/FEATURE_MODEL.md#integration-modes`: one sentence that cert-manager's `cluster` mode references a
  ClusterIssuer, and a link to ADR 0026. `doc/config-tool/USER_JOURNEY.md` stays as it is unless wording drifted.
- `AGENTS.md`: mark US-4.1 as shipped, with a one-paragraph summary in the style of the other story lines.
- Finalize ADR 0026, so it matches what landed.
- Run from `config-tool/`: `make ui`, `make build`, `make test` and `make lint`. Run
  `.github/scripts/check-links.sh` for the new doc links if docker is available.
