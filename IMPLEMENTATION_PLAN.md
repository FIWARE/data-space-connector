# Implementation Plan: [E3 Foundations] US-3.5 — Size it for production (the resource profile)

## Overview
Add the `resources.profile` foundation (`development` / `production` / `custom`, defaulting from
`target.environment`) and a bundle `resources:` table that renders `requests`/`limits` on Keycloak,
Scorpio, the TMForum API, the Trusted Issuers List and contract-management. Add a `memoryLimit` L2 warning for
production profiles that leave an enabled Java component without a memory limit, and a Stage 3 *Platform*
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
       - {id: keycloak, label: Keycloak, path: keycloak.resources, features: [identity.issuer], chartMemoryLimit: 2Gi}
       - {id: scorpio, label: Scorpio, path: scorpio.resources, features: [data.scorpio]}
       - {id: tmforum, label: TMForum API, path: tm-forum-api.defaultConfig.resources, features: [contract.tmforum]}
       - {id: til, label: Trusted Issuers List,
          path: decentralizedIam.vcAuthentication.trusted-issuers-list.deployment.resources, features: [access.verification]}
       - {id: contractManagement, label: contract-management, path: contract-management.deployment.resources,
          features: [contract.management, market.central]}
     profiles:
       development: {keycloak: {requests: {cpu: 100m, memory: 768Mi}, limits: {cpu: "1", memory: 2Gi}}, ...}
       production:  {...}
   ```
   The resource paths were checked against the subcharts: tm-forum-api 0.19.4 reads `defaultConfig.resources`
   (both deploy modes), contract-management 3.6.3 reads `deployment.resources`, scorpio-broker-aaio 0.5.1 reads
   top-level `resources`, and the TIL path is the one `k3s/provider.yaml` uses. The umbrella `values.yaml` already sets
   `keycloak.resources` (500Mi/0.5 → 2Gi/1, kept above the chart default because of OOMs on kernel ≥ 6.14).
   `chartMemoryLimit` records that default for the rule. Quantities are YAML strings.
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
   ref `target.environment` (`model.FieldTargetEnvironment`). It is seeded from `profile.Target.Environment`, is never a
   foundation, never appears in `Answers()`, the tree, `HandSet` or `DerivedDiff`, and an empty environment leaves it
   unresolved, so there is no default and no key. A given answer overrides it, as every given answer does (ADR 0020).
4. **Render.** The model compiles the table into targets of the `answer` foundation: one `const` target per
   component × leaf (`requests.cpu`, `requests.memory`, `limits.cpu`, `limits.memory`) per profile, each carrying
   `Only: <profile>`. The render emits a target with `Only` only while the resolved answer equals it. Provenance,
   pinning of hand edits, `Diff` and the fan-out therefore work unchanged. The bundle-level rule "const only under a
   list index" stays a rule for hand-written targets. `render.Targets` lists each path once, so
   `resources.profile` reports 20 values in 5 components.
5. **No back-derivation on load.** A loaded `resources` block stays unknown and is pinned (ADR 0014). The answer
   lives in the profile, and re-loading a generated file without its profile lists `resources.profile` as underived.
6. **Rule kind `memoryLimit`**: `{components: [<table component id>...]}`, combined with `when`. It is the first kind
   that keeps `when`. It produces one warning on `field` that names every listed component that:
   - is enabled: any of its `features` is on in `p.Features`, and
   - has no memory limit. That means no `<path>.limits.memory` leaf in the rendered tree (overrides pinned in), no
     non-null scalar at that path in the loaded file, and no `chartMemoryLimit`, except where the loaded file sets that
     path to `null`.

   The message template takes `{{ .components }}` (labels joined with ", "). `check.Evaluate` gains a
   `render.Options` argument, so the server passes the loaded overrides and the CLI passes the zero value.
7. **Schema check.** A Go test walks every table path through `charts/data-space-connector/values.schema.json`
   until it reaches a node with `additionalProperties: true` and no named properties (every subchart alias here).
   Each named segment before that must exist. This is the per-path check US-11.6 will generalize.
8. **Trust-anchor bundle (`model/1.3.6`)** is untouched: the story is scoped to the connector chart.

## Steps

### Step 1: Model — `platform` group, `target.environment` ref, `resources:` table and its data
- `config-tool/internal/model/foundations.go`:
  - add `GroupPlatform FoundationGroup = "platform"`, last in `knownFoundationGroups` (screen order).
  - let `parseTemplate` refs include the reserved `FieldTargetEnvironment`. Exclude it from the refs that order
    foundations or must name a foundation, and keep it in `DefaultRefs()` so the default stays gated on it.
- New `config-tool/internal/model/resources.go`:
  - types `ResourceTable`, `ResourceComponent`, `ResourceSpec` (`Requests`, `Limits` of `CPU`, `Memory`), plus
    `Bundle.Resources`, decoded from `resources:`.
  - validation, wired into the bundle loader. Each check gets its own error:
    - the answer is a choice foundation, and every profile key is one of its options;
    - `measuredOn` is semver;
    - component ids are unique and `features` exist;
    - every profile row names every component with all four quantities;
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
  - Keycloak's `chartMemoryLimit` equals `keycloak.resources.limits.memory` in the chart's `values.yaml`.

  `foundations_test.go` gains the cases for the platform group and the `target.environment` ref.
- Acceptance: `go test ./...` and `make lint` are green. No render output changes yet, because the table is not
  compiled into targets.

### Step 2: Render — seed `target.environment`, compile the table, `custom` renders nothing
- `internal/model`: compile the table into `CompiledTarget`s with the new field `Only string` on the answer
  foundation (Decision 4). Paths are `<component path>.<requests|limits>.<cpu|memory>`.
- `internal/render/render.go`: `resolve` takes the profile's environment and seeds the reserved ref into the
  template namespace only, never into the returned answers. `Render` skips a target whose `Only` differs from the
  foundation's resolved answer. `render.Targets` (fan-out) dedupes by path. Check that `HandSet` and `DerivedDiff`
  call the new `resolve` signature, and that switching Stage 1 environment re-derives an un-given profile.
- `internal/render/testdata/provider.dscprofile.yaml` answers `resources.profile: custom`. Its environment is
  `development`, and `k3s/provider.yaml`'s resources are hand-tuned for a single k3s node. Add exclusion rows in
  `provider_test.go` naming US-3.5 for provider.yaml's resources leaves on the five components.
- Tests (table-driven, `render_test.go`):
  - production → exactly 20 leaves, 4 on each of the 5 paths, with the production figures;
  - development → the development figures;
  - `custom` → no leaf under any table path;
  - an empty environment and no answer → no leaf;
  - a given answer beats the environment;
  - a loaded hand edit of `scorpio.resources.limits.memory` is pinned and kept;
  - a production → development switch shows up in `Diff`.

  Server `render_test.go`/`foundations_test.go`: `resources.profile` lists group `platform`, 20 targets and
  5 components.
- Acceptance: the first ticket acceptance criterion (production renders on all five, custom renders none). Tests
  and lint are green.

### Step 3: Check — the `memoryLimit` rule kind and the production warning
- `internal/model/checkrules.go`:
  - add `RuleMemoryLimit {Components []string}` and the `CheckRule.MemoryLimit` field;
  - in validation, `field` and `when` are required with it, components must exist in the table and be unique, and
    the kind excludes `distinct`/`didWebHost`;
  - add `MessageComponents = "components"` for `RenderMessage`.
- `internal/check/check.go`:
  - `Evaluate(b, p, opts render.Options)`: render the tree with the options and evaluate `memoryLimit` per
    Decision 6. `when` must hold first. Components follow table order. Emit no finding when none is unlimited.
  - update callers `internal/server/render.go` (`overridesFor(...)`) and `internal/cli/cli.go` (zero options).
- `config-tool/model/10.16.0/bundle.yaml`: rule `java-memory-limit`.
  ```yaml
  - id: java-memory-limit
    severity: warning
    stage: foundations
    field: answers.resources.profile
    when:
      - {field: target.environment, equals: production}
    memoryLimit: {components: [keycloak, scorpio, tmforum, til, contractManagement]}
    message: >-
      {{ .components }} run without a memory limit, so the JVM sizes its heap from the node.
    doc: doc/deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components
  ```
  Document the kind in the header comment and bump `revision` to 13.
- Tests (table-driven, `check_test.go`, `checkrules_test.go`):
  - production + `custom` + Scorpio and TMF on → one warning naming exactly those two (Keycloak excluded through
    `chartMemoryLimit`);
  - production + `production` → none;
  - production + `development` → none;
  - development + `custom` → none;
  - a feature that is off → not named;
  - a loaded `scorpio.resources.limits.memory` → not named;
  - a loaded `keycloak.resources.limits.memory: null` → named;
  - `check.Blocking` is false, and `generate` succeeds with the warning (CLI test);
  - the model validation errors.
- Acceptance: the second ticket acceptance criterion. Tests and lint are green.

### Step 4: API + UI — the Stage 3 *Platform* panel with the per-component preview
- `internal/server/render.go`: `foundationsResponse` gains `resources`, which carries `answer`, `measuredOn`,
  `doc`, the components (`id`, `label`, `path`) and the profiles' figures. It is `null` for a bundle without a table.
  Add a server test.
- `ui/src/api/foundations.ts`: types and parsing for it, with tests.
- `ui/src/stages/Foundations.tsx`, plus a new `ui/src/stages/PlatformPanel.tsx`:
  - a `Platform` panel after Network (constant `PLATFORM_PANEL`);
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
  - the `custom` line;
  - the `java-memory-limit` finding renders on the field and links the guidance.
- Acceptance: the panel shows what the chosen profile will set before it is applied. `make ui`, `npm test`,
  `make test` and `make lint` are green.

### Step 5: Documentation — ADR 0024, README, design docs, AGENTS.md
- `doc/adr/0024-config-tool-resource-profile.md`, status Proposed: Decisions 1–8 with the figure derivation and
  the 10.13.2 citation. Add it to the index in `doc/adr/README.md` (table row and the range sentence).
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
