# Implementation Plan: [E3 Foundations] US-3.5 — Size it for production (the resource profile)

## Overview
Add the `resources.profile` foundation (`development` / `production` / `custom`, defaulting from
`target.environment`) and a bundle `resources:` table that renders `requests`/`limits` on Keycloak,
Scorpio, the TMForum API, the Trusted Issuers List and contract-management. Add a `memoryLimit` L2 warning for
production profiles that leave any deployed JVM component (the five plus the ones the profile does not size) without
a memory limit, and a Stage 3 *Platform*
panel that previews the figures per component. All model content is bundle data (`model/10.16.0/bundle.yaml`).
The Go core stays free of HTTP.

## Decisions (record them in ADR 0024 in Step 5; steps 1–4 implement them as written)

1. **The table.** The bundle gets a top-level `resources:` section:
   ```yaml
   resources:
     answer: resources.profile          # the choice foundation that selects a row
     measuredOn: 10.13.2                # chart version of PREREQUISITES.md#sizing
     doc: doc/deployment-integration/roles/PREREQUISITES.md#sizing
     components:
       - {id: keycloak, label: Keycloak, path: keycloak.resources,
          enabledKeys: {keycloak.enabled: true}, chartMemoryLimit: 2Gi}
       - {id: scorpio, label: Scorpio, path: scorpio.resources,
          enabledKeys: {scorpio.enabled: true}}
       - {id: tmforum, label: TMForum API, path: tm-forum-api.defaultConfig.resources,
          enabledKeys: {tm-forum-api.enabled: true}}
       - {id: til, label: Trusted Issuers List,
          path: decentralizedIam.vcAuthentication.trusted-issuers-list.deployment.resources,
          enabledKeys: {decentralizedIam.enabled: true, decentralizedIam.vcAuthentication.enabled: true,
                        decentralizedIam.vcAuthentication.trusted-issuers-list.enabled: true}}
       - {id: contractManagement, label: contract-management, path: contract-management.deployment.resources,
          enabledKeys: {contract-management.enabled: true}}
     # JVM components the profile does not size; only the memoryLimit rule reads them (Decision 6).
     unsized:
       - {id: ccs, label: credentials-config-service,
          path: decentralizedIam.vcAuthentication.credentials-config-service.deployment.resources,
          enabledKeys: {decentralizedIam.enabled: true, decentralizedIam.vcAuthentication.enabled: true,
                        decentralizedIam.vcAuthentication.credentials-config-service.enabled: false}}
       - {id: dss, label: DSS validation service, path: decentralizedIam.vcAuthentication.dss.deployment.resources,
          enabledKeys: {decentralizedIam.enabled: true, decentralizedIam.vcAuthentication.enabled: true,
                        decentralizedIam.vcAuthentication.dss.enabled: false}}
       - {id: consentFacade, label: consent-facade, path: consent-facade.deployment.resources,
          enabledKeys: {consent-facade.enabled: false}}
       - {id: identityhub, label: IdentityHub, path: identityhub.resources,
          enabledKeys: {identityhub.enabled: false}, chartMemoryLimit: 512Mi}
       - {id: edc, label: fdsc-edc, path: fdsc-edc.deployment.*.deployment.resources,
          fallbackPaths: [fdsc-edc.common.deployment.resources], enabledKeys: {fdsc-edc.enabled: true}}
     profiles:
       development: {keycloak: {requests: {cpu: 100m, memory: 768Mi}, limits: {cpu: "1", memory: 2Gi}}, ...}
       production:  {...}
   ```
   The resource paths were checked against the subcharts: tm-forum-api 0.19.4 reads `defaultConfig.resources`
   (both deploy modes), contract-management 3.6.3 reads `deployment.resources`, scorpio-broker-aaio 0.5.1 reads
   top-level `resources`, and the TIL path is the one `k3s/provider.yaml` uses. The umbrella `values.yaml` already sets
   `keycloak.resources` (500Mi/0.5 → 2Gi/1, kept above the chart default because of OOMs on kernel ≥ 6.14).
   `chartMemoryLimit` records that default for the rule (IdentityHub's 512Mi is the umbrella `identityhub.resources`
   limit). `enabledKeys` maps each chart switch that must be true for the component to deploy to its chart default.
   The default is the umbrella `values.yaml` value; a key the umbrella does not set (`decentralizedIam.vcAuthentication
   .enabled`, `fdsc-edc.enabled`) takes the subchart's default or Helm's "a missing condition is on", both `true`
   (Decision 6). Quantities are YAML strings.

   `unsized` lists the other JVM workloads of the chart, checked by their default images' entrypoints (`java …`):
   credentials-config-service 3.4.3, dss-validation-service 0.0.1, consent-facade 0.4.2, the umbrella's IdentityHub and
   the fdsc-edc connectors. The fdsc-edc chart renders one Deployment per key of `fdsc-edc.deployment` (empty by
   default), each merged over `fdsc-edc.common.deployment`, so its `path` has one `*` segment over the map keys and a
   limit under a `fallbackPaths` entry covers every instance. A `*` is allowed only in `unsized` paths. Not JVM, and
   therefore not listed: `odrl-pap` 1.7.3 (a Quarkus native image, entrypoint `./application`), the verifier,
   vc-operator, did-helper (Go), Rainbow (Rust), the status-list-server, the marketplace (Node/Python) and the
   dashboard. Because the verdict holds for the default images, ADR 0024 records it next to the subchart versions.
2. **Figures** (component: requests cpu/memory → limits cpu/memory). The floors measured *at rest* on 10.13.2
   are Keycloak 0.6 GiB, Scorpio 0.4, TMF 0.35, TIL 0.2 and CM 0.2. Write them as a YAML comment next to the table.

   | Component | development | production |
   |---|---|---|
   | keycloak | 100m/768Mi → 1/2Gi | 500m/2Gi → 2/2Gi |
   | scorpio | 100m/512Mi → 1500m/1Gi | 250m/1Gi → 2/1Gi |
   | tmforum | 100m/512Mi → 1/1Gi | 250m/1Gi → 2/1Gi |
   | til | 50m/256Mi → 500m/512Mi | 100m/512Mi → 1/512Mi |
   | contractManagement | 50m/256Mi → 500m/512Mi | 100m/512Mi → 1/512Mi |

   The rule behind the figures:
   - *development*: the memory request is the floor rounded up to 256Mi, and the limit is 2× the request.
   - *production*: memory request = limit = 2.5× the floor rounded up to 512Mi. Request = limit, so the scheduler
     reserves what the JVM may take and the pod is not evicted first under node pressure.
   - Neither profile ever lowers the chart's own Keycloak limit of 2Gi.
   - CPU requests are a fraction of a core, because the components idle at rest. CPU limits leave room for the
     startup burst, since a throttled JVM misses its probes (the reasoning of the `k3s/provider.yaml` comment).
   - `custom` has no row. It renders nothing, so the chart's defaults stay in force.
3. **Default from Stage 1.** `resources.profile` is a `choice` (`development`, `production`, `custom`) in the new
   group `platform` with `default: "{{ .target.environment }}"`. Templates may reference the reserved, read-only
   ref `target.environment` (`model.FieldTargetEnvironment`). It is seeded from `profile.Target.Environment` and is
   never a foundation. The ref itself never appears in `Answers()`, the tree, or as an entry of `HandSet` or
   `DerivedDiff`. An empty environment leaves it unresolved, so there is no default and no key. A given answer
   overrides it, as every given answer does (ADR 0020).
   A *given* `resources.profile` is a hand-set answer like any other with a derived default: `HandSet` lists it
   (its `DefaultRefs()` is non-empty), so Review's "Set by hand" shows it with `Derived` = the environment's profile.
   `DerivedDiff` reports it as *pinned, not updated* when a Stage 1 environment switch changes what it would derive.
   That is intended: the user chose a profile that no longer follows the environment.
   The default gate must see the reserved ref. `render.defaults` gates on `all(answers, f.DefaultRefs())`, and
   `answers` never holds `target.environment`. `resolve` therefore builds a `reserved` set (the reserved refs with a
   non-empty value, today only `target.environment`) and returns it with the answers and the namespace. Every caller
   of `defaults` takes it: `resolve` itself, `checkPrefix` and `derived()` in `derived.go` (used by `HandSet` and
   `DerivedDiff`, which resolve `prev` and `next` with their own environments). They gate on "every ref is in
   `answers` or in `reserved`" (a helper `resolved(answers, reserved, ids)` replaces `all` there).
   `Render`'s target gate keeps `all(answers, t.Refs)`, because a target never references a reserved ref; the model
   refuses that.
4. **Render.** The model compiles the table into targets of the `answer` foundation: one `const` target per
   component × leaf (`requests.cpu`, `requests.memory`, `limits.cpu`, `limits.memory`) per profile, each carrying
   `Only: <profile>` and `Refs: [<answer>]`. A hand-written const has empty `Refs`, and `compileTarget` refuses an
   answer reference on it. The table compiler builds its `CompiledTarget`s directly and does not go through
   `compileTarget`, so it sets the ref itself. The ref is what makes the rest work:
   - `Render`'s gate `all(answers, t.Refs)` skips the targets while the profile is unresolved;
   - `Leaf.Provenance.Answers` names `resources.profile`;
   - `render.Targets` reaches the targets, because it lists only targets whose `Refs` reach the answer.

   The render emits a target with `Only` only while the resolved answer equals it. Pinning of hand edits and `Diff`
   then work unchanged. The bundle-level rule "const only under a list index" stays a rule for hand-written targets.
   `render.Targets` lists each path once (it dedupes, since every path has one target per profile), so
   `resources.profile` reports 20 values in 5 components.
5. **No back-derivation on load.** A loaded `resources` block stays unknown and is pinned (ADR 0014). The answer
   lives in the profile, and re-loading a generated file without its profile lists `resources.profile` as underived.
6. **Rule kind `memoryLimit`**: `{components: [<id from components or unsized>...]}`, combined with `when`. It is the
   first kind that keeps `when`. The ticket asks for *any* enabled Java component, so the rule covers the table's
   `components` and `unsized` alike. It produces one warning on `field` that names every listed component (for `edc`,
   every key of `fdsc-edc.deployment` in the rendered tree or the loaded file, labelled `fdsc-edc (<key>)`, in key
   order) that:
   - is deployed: every one of its `enabledKeys` is effectively true. The effective value of a key is the rendered
     tree's leaf, else the loaded file's scalar, else the default the bundle records (a Step 1 test pins those against
     `values.yaml`). So the default-off components (credentials-config-service, dss, consent-facade, IdentityHub) are
     named only once a loaded file switches them on. Features in `p.Features` do not decide it. The tool renders no
     `enabled` key until US-5.3, so a feature that is off does not stop the chart from deploying the component. A
     production `custom` profile with `data.scorpio` off still runs Scorpio, and the warning names it. Once US-5.3
     renders `enabled` from the features, the rule follows through the rendered leaf, with no change to the rule.
     ADR 0024 and a YAML comment above the rule in the bundle record this;
   - has no memory limit. That means no `<path>.limits.memory` leaf in the rendered tree (overrides pinned in), no
     non-null scalar at that path in the loaded file, none at any of its `fallbackPaths`, and no `chartMemoryLimit`.
     Helm's coalesce drops a chart default under a `null`, so a loaded `null` at `<path>.limits.memory` *or at any ancestor from `<path>` up*
     (`keycloak.resources: null`, `keycloak.resources.limits: null`) removes `chartMemoryLimit`. A `null` above
     `<path>` (`keycloak: null`) is not considered; it is not a sizing edit.

   The message template takes `{{ .components }}` (labels joined with ", ") and reads for any count:
   "Without a memory limit, the JVM sizes its heap from the node: {{ .components }}." `check.Evaluate` gains a
   `render.Options` argument, so the server passes the loaded overrides and the CLI passes the zero value.
7. **Schema check.** A Go test walks every table path (`components`, `unsized` and `fallbackPaths`) through
   `charts/data-space-connector/values.schema.json` until it reaches a node with `additionalProperties: true` and no named properties (every subchart alias here).
   Each named segment before that must exist. This is the per-path check US-11.6 will generalize.
8. **Trust-anchor bundle (`model/1.3.6`)** is untouched: the story is scoped to the connector chart.

## Steps

### Step 1: Model — `platform` group, `target.environment` ref, `resources:` table and its data
- `config-tool/internal/model/foundations.go`:
  - add `GroupPlatform FoundationGroup = "platform"`, last in `knownFoundationGroups` (screen order).
  - let `parseTemplate` refs include the reserved `FieldTargetEnvironment`. Exclude it from the refs that order
    foundations or must name a foundation, and keep it in `DefaultRefs()` so the default stays gated on it (the render
    checks it against the seeded reserved set, Decision 3). A target template that references it is refused.
- New `config-tool/internal/model/resources.go`:
  - types `ResourceTable` (with `Components` and `Unsized`), `ResourceComponent` (`EnabledKeys map[string]bool`,
    `FallbackPaths`), `ResourceSpec` (`Requests`, `Limits` of `CPU`, `Memory`), plus `Bundle.Resources`, decoded from
    `resources:`.
  - validation, wired into the bundle loader. Each check gets its own error:
    - the answer is a choice foundation, and every profile key is one of its options;
    - `measuredOn` is semver;
    - component ids are unique across `components` and `unsized`, and every `enabledKeys` key and `fallbackPaths`
      entry is a path whose root is in `topLevelKeys`;
    - a `*` segment appears at most once, only in an `unsized` path, never as the first segment;
    - every profile row names every `components` entry with all four quantities and names no `unsized` one;
    - each quantity parses with `k8s.io/apimachinery/pkg/api/resource.ParseQuantity` (already in the module graph;
      becomes a direct dependency via `go mod tidy`), and request ≤ limit;
    - `chartMemoryLimit` parses when set.
  - Name the leaf keys (`requests`, `limits`, `cpu`, `memory`) as constants.
- `config-tool/model/10.16.0/bundle.yaml`:
  - append the `resources.profile` foundation: group `platform`; options with notes (`custom`: "Leaves the chart's
    defaults; you size the components yourself"); `default: "{{ .target.environment }}"`;
    `doc: doc/deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components`; no `targets`.
  - add the `resources:` table with the Decision 2 figures and the floors comment.
  - document both in the header comment and bump `revision` to 12.
- Tests (table-driven), in `internal/model/resources_test.go`:
  - every validation error;
  - the shipped bundle loads;
  - every table path resolves in `../../../charts/data-space-connector/values.schema.json` (Decision 7);
  - Keycloak's `chartMemoryLimit` equals `keycloak.resources.limits.memory` in the chart's `values.yaml`;
  - every `enabledKeys` default equals the key's value in the chart's `values.yaml`; a key absent there must default
    to `true` and is listed in the test with the subchart that supplies it (Decision 6);
  - IdentityHub's `chartMemoryLimit` equals `identityhub.resources.limits.memory` in the chart's `values.yaml`.

  `foundations_test.go` gains the cases for the platform group and the `target.environment` ref.
- Acceptance: `go test ./...` and `make lint` are green. No render output changes yet, because the table is not
  compiled into targets.

### Step 2: Render — seed `target.environment`, compile the table, `custom` renders nothing
- `internal/model`: compile the table into `CompiledTarget`s with the new field `Only string` and `Refs: [<answer>]`
  on the answer foundation (Decision 4). Paths are `<component path>.<requests|limits>.<cpu|memory>`.
- `internal/render/render.go`: `resolve` takes the profile's environment and seeds the reserved ref into the
  template namespace only, never into the returned answers. It returns the `reserved` set as well, and `defaults`,
  `checkPrefix` and `derived()` (`derived.go`) take it and gate on `resolved(answers, reserved, ids)` instead of
  `all` (Decision 3). `Render` skips a target whose `Only` differs from the foundation's resolved answer.
  `render.Targets` (fan-out) dedupes by path. `HandSet` and `DerivedDiff` call the new `resolve` signature with
  each profile's own environment.
- `internal/render/testdata/provider.dscprofile.yaml` answers `resources.profile: custom`, because
  `k3s/provider.yaml` hand-tunes resources for a single k3s node (Keycloak cpu only, Scorpio and the TIL; none on the
  TMForum API or contract-management). No `providerExclusions` rows: they only match leaves carrying a
  `referenceValues` needle, and the resources leaves carry none. `TestProviderValues` compares only rendered leaves,
  so it gains an assertion that the fixture's tree has no leaf under any table path (`renderProvider` passes no
  `Target`, so the answer, not the environment, decides this).
- Tests (table-driven, `render_test.go`):
  - production → exactly 20 leaves, 4 on each of the 5 paths, with the production figures;
  - development → the development figures;
  - `custom` → no leaf under any table path;
  - environment `production` and no answer → the 20 production leaves (the default gate sees the reserved ref);
  - environment `development` and no answer → the development figures;
  - an empty environment and no answer → no leaf;
  - every table leaf's `Provenance.Answers` is `[resources.profile]`;
  - `Targets(b, "resources.profile")` returns the 20 paths, each once;
  - a given answer beats the environment;
  - a loaded hand edit of `scorpio.resources.limits.memory` is pinned and kept;
  - a production → development switch shows up in `Diff`;
  - `HandSet` (`handset_test.go`): a given `custom` with environment `production` is listed with `Derived:
    production`; an un-given profile is not listed; `target.environment` is never an entry;
  - `DerivedDiff` (`derived_test.go`): environment development → production with no answer re-derives the profile
    (no pinned entry); with a given `custom` it reports `resources.profile` as pinned, `Before: development`,
    `After: production`.

  Server `render_test.go`/`foundations_test.go`: `resources.profile` lists group `platform`, 20 targets and
  5 components.
- Acceptance: the first ticket acceptance criterion (production renders on all five, custom renders none). Tests
  and lint are green.

### Step 3: Check — the `memoryLimit` rule kind and the production warning
- `internal/model/checkrules.go`:
  - add `RuleMemoryLimit {Components []string}` and the `CheckRule.MemoryLimit` field;
  - in validation, `checkCheckRule` stops counting `when` as a kind of its own once `memoryLimit` is set. The kinds
    become `when` alone, `distinct`, `didWebHost` and `memoryLimit` (which requires `when` and `field`), and exactly one
    of them is allowed. "field is for when only" still holds, because a `memoryLimit` rule has `when`.
    `memoryLimit` with `distinct` or `didWebHost`, or without `when` or `field`, gets its own error. Its components
    must exist in the table and be unique. The `when` conditions go through `checkWhenRule` as before;
  - add `MessageComponents = "components"` for `RenderMessage`.
- `internal/check/check.go`:
  - `Evaluate(b, p, opts render.Options)`: render the tree with the options and evaluate `memoryLimit` per
    Decision 6. For such a rule `when` is only the precondition, so the plain `when` branch (`slices.ContainsFunc
    (r.When, ...)`) must not also fire for it: the `memoryLimit` case comes before it in the switch. Components follow
    table order, then `unsized` order. A `*` path expands over the keys of the map at its prefix in the rendered tree
    and the loaded overrides, sorted. Emit no finding when none is unlimited.
  - update callers `internal/server/render.go` (`overridesFor(...)`) and `internal/cli/cli.go` (zero options).
- `config-tool/model/10.16.0/bundle.yaml`: rule `java-memory-limit`.
  ```yaml
  - id: java-memory-limit
    severity: warning
    stage: foundations
    field: answers.resources.profile
    when:
      - {field: target.environment, equals: production}
    memoryLimit: {components: [keycloak, scorpio, tmforum, til, contractManagement,
                               ccs, dss, consentFacade, identityhub, edc]}
    message: >-
      Without a memory limit, the JVM sizes its heap from the node: {{ .components }}.
    doc: doc/deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components
  ```
  A YAML comment above the rule says that a component's deployment is read from its `enabledKeys`, not from the
  features, until US-5.3 renders `enabled` (Decision 6). Document the kind in the header comment and bump `revision`
  to 13.
- Tests (table-driven, `check_test.go`, `checkrules_test.go`):
  - production + `custom`, no loaded file → one warning naming Scorpio, TMForum API, Trusted Issuers List and
    contract-management in table order (Keycloak excluded through `chartMemoryLimit`; the `unsized` components are
    off or, for fdsc-edc, have no instance by default);
  - production + `production` → none;
  - production + `development` → none;
  - development + `custom` → none;
  - production + `custom` with `data.scorpio` off and no loaded `scorpio.enabled` → Scorpio still named (the chart
    default deploys it; the documented pre-US-5.3 limitation);
  - a loaded `scorpio.enabled: false` → Scorpio not named;
  - a loaded `decentralizedIam.enabled: false` → the Trusted Issuers List not named (every `enabledKeys` must hold);
  - a loaded `scorpio.resources.limits.memory` → not named;
  - a loaded `keycloak.resources.limits.memory: null` → named;
  - a loaded `keycloak.resources: null` → named (a null ancestor drops the chart default);
  - the default-off `unsized` components are not named by default; a loaded
    `decentralizedIam.vcAuthentication.credentials-config-service.enabled: true` → credentials-config-service named;
    a loaded `identityhub.enabled: true` → not named (chart 512Mi), plus `identityhub.resources: null` → named;
  - a loaded `fdsc-edc.deployment.dcp` and `.oid4vc` → "fdsc-edc (dcp), fdsc-edc (oid4vc)"; with
    `fdsc-edc.common.deployment.resources.limits.memory` loaded → not named; with `fdsc-edc.enabled: false` → not
    named;
  - the message with one component reads "…from the node: Scorpio.";
  - a `memoryLimit` rule yields exactly one finding, never an extra plain `when` finding;
  - `check.Blocking` is false, and `generate` succeeds with the warning (CLI test);
  - the model validation errors.
- Acceptance: the second ticket acceptance criterion. Tests and lint are green.

### Step 4: API + UI — the Stage 3 *Platform* panel with the per-component preview
- `internal/server/render.go`: `foundationsResponse` gains `resources`, which carries `answer`, `measuredOn`,
  `doc`, the components (`id`, `label`, `path`) and the profiles' figures. It is `null` for a bundle without a table.
  Add a server test.
- `ui/src/api/foundations.ts`: types and parsing for it, with tests.
- `ui/src/state/foundations.ts`: the UI resolves defaults itself (`fill` → `resolvedAnswer` → `defaultValue`), and
  `target.environment` is neither a value nor a foundation there, so `fill` would leave the profile pending forever.
  Mirror Go's reserved set:
  - a constant `TARGET_ENVIRONMENT_REF = 'target.environment'` and `reservedRefs(target)`, which returns
    `{ 'target.environment': env }` while Stage 1's environment is answered, else `{}`;
  - `fill`, `defaultApplies`, `defaultValue`, `resolvedAnswer`, `prefixError` and `answerErrors` take a
    `reserved: Readonly<Record<string, string>>` argument, and `fill` reads a ref from `reserved` before it
    resolves it as an answer. The reserved refs never enter `values`, so they are never saved, counted as given or
    checked as answers;
  - every caller passes `reservedRefs(<the Stage 1 target>)`: `Foundations.tsx`, `AnswerField.tsx`,
    `DataSpacePanel.tsx`, `useReviewRender.ts`, `state/openQuestions.ts`, `api/profile.ts` and the new panel.

  Vitest cases in `state/foundations.test.ts`: environment production with no answer → `resolvedAnswer` gives
  `production`; an unanswered environment → `undefined`; a given `custom` beats the environment; `values` never gains
  `target.environment`.
- `ui/src/stages/Foundations.tsx`, plus a new `ui/src/stages/PlatformPanel.tsx`:
  - a `Platform` panel after Network. The response and the panel carry `components` only; `unsized` stays a
    rule input and is not previewed (constant `PLATFORM_PANEL`);
  - the resource-profile choice, reusing `ChoiceField`, with the derived/given state and its findings, as the other
    choices show them;
  - below it, a table of component | requests | limits for the *resolved* profile (the given answer, else the one
    derived from the environment), captioned "Figures from the sizing guidance, measured on chart {measuredOn}",
    with a link to `doc`;
  - for `custom`, the line "Sets nothing: the chart's defaults stay in force.";
  - while the profile is unresolved, the derived-pending text.

  Every visible string is a named constant or bundle data. The JSDoc is one line.
- `ui/src/stages/Platform*.test.tsx` (vitest, table-driven over development/production/custom/unresolved):
  - the rows and figures;
  - environment production with no answer → the production rows and the derived state (through `reservedRefs`);
  - an unanswered environment with no answer → the derived-pending text, no rows;
  - the `custom` line;
  - the `java-memory-limit` finding renders on the field and links the guidance.
- Acceptance: the panel shows what the chosen profile will set before it is applied. `make ui`, `npm test`,
  `make test` and `make lint` are green.

### Step 5: Documentation — ADR 0024, README, design docs, AGENTS.md
- `doc/adr/0024-config-tool-resource-profile.md`, status Proposed: Decisions 1–8 (including the pre-US-5.3
  enablement limitation of Decision 6, the `unsized` list with how each component was found to be JVM or not, and
  `odrl-pap` as a native image) with the figure derivation and the 10.13.2 citation. Add it to the index in `doc/adr/README.md` (table row and the range sentence).
- `config-tool/README.md`:
  - *Foundations (Stage 3)*: the Platform panel, the profile default and `custom`;
  - *The `foundations:` section*: the `platform` group, the `target.environment` ref and the `resources:` table;
  - *The render*: `Only` targets;
  - *Rules and findings*: the `memoryLimit` kind.

  Link the guidance; do not restate it.
- `doc/config-tool/FEATURE_MODEL.md` (`resources.profile` row: the default comes from the environment; `custom`) and
  `USER_JOURNEY.md` Stage 3 panel 4 (the preview), kept to one sentence each.
- `AGENTS.md`: mark US-3.5 shipped. One line that names the table, `Only` targets, the `target.environment` ref and
  `memoryLimit`.
- Run `.github/scripts/check-links.sh` if docker is available, then the full `make test` and `make lint`.
- Acceptance: the docs match the shipped behaviour, links resolve, and everything is green.
