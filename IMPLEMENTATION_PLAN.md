# Implementation Plan: [E2 Roles] US-2.1 — Pick a role (Stage 2, the role presets in the bundle)

## Overview
This plan adds Stage 2 (Role) to `dsc-config`. The feature catalogue, the five role presets and the mapping from the role
guide's component matrix to feature ids go into the model bundle as data. `internal/model` validates them strictly, and
`internal/profile` records `role` + `features` and applies a preset with a pure `ApplyPreset`. A thin API and
`ui/src/stages/Role.tsx` make the role pickable. A Go test keeps the bundle in agreement with the HTML matrix in
`doc/deployment-integration/roles/README.md`.

## Decisions taken while planning (binding for all steps)

1. **The role guide's matrix is authoritative.** Where `doc/config-tool/FEATURE_MODEL.md#role-presets` disagrees with it,
   the bundle follows the matrix and the design doc is corrected in Step 2. The disagreements are:
   - `contract.tmforum` and `contract.management` are *Optional* for Provider and Consumer + Provider, so they are
     `available`, not `recommended`.
   - The Operator column offers only trust-anchor features (see 3).
2. **Cell translation:** `Required` → `recommended`, `Optional` → `available`, `-` → `not-offered`. `Built into <X>`
   takes the value of row `<X>` in the same column. Any other cell text fails the test.
3. **Features belong to one chart.** A feature has an optional `chart:` (default: the bundle's chart). A role may offer
   (`recommended` / `available`) only features of its own `chart`, because offering a feature whose keys do not exist in
   the target chart would be wrong:
   - Operator (`trust-anchor`) offers `ta.registry` (R), `ta.onboarding` (o) and `consent.authority` (o). Everything
     else is `not-offered`, `dsp.config` and `obs.*` included, since the trust-anchor chart has neither key.
   - `custom` makes every feature of `data-space-connector` `available`, recommends nothing, and does not offer the three
     trust-anchor features. This is how "custom offers everything" is read.
4. **Matrix-to-feature map is bundle data** (top-level `roleMatrix:`), not test code:
   ```yaml
   roleMatrix:
     components:            # matrix "Component" cell text -> feature id
       VCVerifier: access.verification
       credentials-config-service: access.verification
       trusted-issuers-list: access.verification
       APISIX: access.policies
       OPA: access.policies
       odrl-pap: access.policies
       Keycloak: identity.issuer
       DID: identity.did
       tmforum-api: contract.tmforum
       contract-management: contract.management
       FDSC-EDC: dsp.edc
       Trusted Issuers Registry: ta.registry
     infrastructure: [PostgreSQL]   # matrix rows that are not features
   ```
   The matrix test cannot check features that no matrix row maps to (`data.scorpio`, `market.*`, `obs.*`, ...). For
   those, the bundle values come from FEATURE_MODEL, adjusted by decision 3.
5. **Resulting presets** (R = recommended, o = available, – = not-offered):

   | Feature | consumer | provider | consumer-provider | operator | custom |
   |---|---|---|---|---|---|
   | identity.issuer, identity.did | R | R | R | – | o |
   | identity.statuslist, .vcoperator, .wallets, .eidas | o | o | o | – | o |
   | access.verification, access.policies | – | R | R | – | o |
   | data.scorpio | – | R | R | – | o |
   | data.external | – | o | o | – | o |
   | contract.tmforum, contract.management | – | o | o | – | o |
   | market.local, market.central | – | o | o | – | o |
   | dsp.edc, dsp.rainbow, dsp.config | o | o | o | – | o |
   | consent.provider | – | o | o | – | o |
   | obs.tracing, obs.traceStore, obs.metrics | o | o | o | – | o |
   | consent.authority, ta.onboarding | – | – | – | o | – |
   | ta.registry | – | – | – | R | – |

   That is 24 features: the FEATURE_MODEL catalogue with `obs.*` expanded into three ids.
6. **Floors** come from `PREREQUISITES.md#sizing` and are written as Kubernetes quantity strings:
   - consumer: `0.8Gi` / `50m`, measured with `10.13.2`
   - provider: `2.3Gi` / `200m` (*Provider without the marketplace*, which is what the preset turns on), `10.13.2`
   - consumer-provider: the same as provider. The guide has no row for it, and the provider row already holds Keycloak + DID.
   - operator: `0.3Gi` / `20m`, `1.3.0` (the trust-anchor chart version)
   - custom: no `floor`. The UI says "depends on your selection".
7. **Trust-anchor bundle.** `Profile.Validate` refuses a target chart that has no shipped bundle, and Stage 1 can only
   offer versions that have a bundle. So the plan adds `config-tool/model/1.3.6/bundle.yaml` with `chart: trust-anchor`,
   `version: 1.3.6` (`charts/trust-anchor/Chart.yaml`), `revision: 1`, `topLevelKeys` from `charts/trust-anchor/values.yaml`,
   no `recognize`, no `features` and no `roles`. Directory names are versions, and 1.3.6 does not collide with a DSC version.
8. **Where presets are read from:** `model.RoleCatalogue(bundles, chart, version)` returns the target's bundle when it
   declares `roles:`. Otherwise it returns the latest bundle that declares them, which for trust-anchor is the latest DSC
   bundle. This lets an Operator profile still show its roles. The rule is generic, with no chart name in Go code.
   Because the fallback bundle is not the profile's `model`, the profile **records the bundle its catalogue came from**
   (`catalogue: {chart, version, revision}`, set only when it differs from `model`). `Validate` reads roles and features
   from that recorded bundle, never from "whatever is newest now", and applies the same rules as for `model`: not shipped
   or a newer revision → 422 pointing at US-9.3, an older revision → an open/save note. A later DSC bundle that renames
   a `ta.*` feature therefore cannot silently invalidate a saved Operator profile, and the profile says what it was
   built against.
9. **Chart switch (Operator ↔ the rest).**
   - **Refused while a values file is loaded.** A `source` (and the UI's load report and derived answers) is classified
     against the current chart's bundle. A role whose `chart` differs from `target.chart` is therefore refused by
     `ApplyPreset` with `ErrChartSwitchLoaded` while `p.Source != nil` (422 from the API). The UI disables those cards
     and explains why. Nothing is cleared silently. To configure the other chart, the user starts a new configuration.
   - **The version is cleared, never guessed.** On a switch, `ApplyPreset` clears `target.chartVersion`, so Stage 1
     asks for the new chart's version again. `model` is set to the new chart's latest bundle as a placeholder, which is
     what the server already does while no version is answered. `ChartSwitch` carries the replaced chart and version,
     so the notice can say what was dropped. Picking latest silently would overwrite a version the user chose, for
     example a pinned older DSC release, when they switch back from Operator.
10. **Applying a preset overwrites `features`.** Nothing can be `source: user` yet, because Stage 5 does not exist. US-2.3
   will add "re-apply to untouched features only" on top of the recorded `source`.
11. **HTML parsing in the matrix test** uses `golang.org/x/net/html`, and only in `_test.go` files. `golang.org/x/net`
    is already in `go.mod` (indirect), so this adds no new module. It moves to the direct `require` block. Record this
    in ADR 0017.

## Steps

### Step 1: Model types and strict validation for `features:`, `roles:` and `roleMatrix:`
Extend `config-tool/internal/model/model.go`, or a new `internal/model/roles.go` in the same package, without touching
any shipped bundle yet.

- **Types:**
  - `Feature{ID, Title, Summary, Group, Chart, Requires []string, Keys []string}`
  - `Role{ID, Title, Summary, Chart, Floor *Floor, Docs, Features map[string]Offer}`
  - `Floor{Memory, CPU, MeasuredWith}`
  - `Offer` with constants `OfferRecommended = "recommended"`, `OfferAvailable = "available"`, `OfferNotOffered = "not-offered"`
  - `RoleMatrix{Components map[string]string, Infrastructure []string}`
  - Role id constants `RoleConsumer`, `RoleProvider`, `RoleConsumerProvider`, `RoleOperator`, `RoleCustom`, plus an
    ordered `KnownRoles` list.
  - Add the new fields to `Bundle` with yaml tags. `KnownFields(true)` already makes unknown keys fail.
- **`readBundle` validation**, where every error names the offending index or id:
  - Feature id: non-empty and unique. `title`, `summary` and `group` are required.
  - Group: one of the FEATURE_MODEL groups (named constants).
  - `requires`: must name catalogue ids, with no self-reference.
  - `keys`: parse with `roundtrip.ParsePath`.
  - `roles`: either absent, or exactly the five known ids in `KnownRoles` order.
  - Each role: `title`, `summary`, `chart` and `docs` are required.
  - Floor, when present: `memory` matches `^[0-9]+(\.[0-9]+)?(Mi|Gi)$`, `cpu` matches `^[0-9]+m$`, and `measuredWith`
    is strict semver. Use named regexp vars.
  - Every preset key is a catalogue id, every catalogue id appears in every role, and every value is a known `Offer`.
  - A role offers only features whose chart equals the role's chart (decision 3).
  - `custom` recommends nothing.
  - The `requires` of a recommended feature are recommended. The `requires` of an available feature are offered.
  - `roles` without `features` is an error.
  - `roleMatrix` values are catalogue ids, and `roleMatrix` without `roles` is an error.
- **Cross-bundle checks in `Versions`**, after all bundles are read:
  - A role's `chart` must have a shipped bundle.
  - A feature's `keys` must start with a `topLevelKeys` entry of the latest bundle of the feature's chart (when that list is set).
- **`RoleCatalogue(bundles, chart, version) (Bundle, error)`** per decision 8. It returns an error when no bundle declares roles.
- **Tests** in `internal/model/model_test.go`:
  - Table-driven: one fixture string per failure, each asserting its error substring.
  - A valid fixture under `testdata/bundles/`. Add a second chart fixture so the cross-chart rule is exercised; keep
    existing fixture tests green.
  - `RoleCatalogue` cases: own bundle, fallback to the latest bundle with roles, and none. An empty `version` means
    the chart's latest bundle, because a chart switch leaves `target.chartVersion` empty (decision 9).
- GoDoc in one line per exported identifier, matching the file's density.

**Acceptance:** `go test ./internal/model/...` is green, the existing shipped bundle still loads unchanged, and `make lint` is clean.

### Step 2: Bundle content, trust-anchor bundle, matrix-agreement test, ADR 0017
- **`config-tool/model/10.16.0/bundle.yaml`:**
  - Bump `revision: 1` → `2`.
  - Append `features:` with the 24 catalogue entries. Take `title` and `summary` from the "Gives you" column, and
    `group`, `requires` and `keys` from `doc/config-tool/FEATURE_MODEL.md`. Trust-anchor features get
    `chart: trust-anchor` and trust-anchor keys.
  - Add `roles:` per decisions 3, 5 and 6. Titles exactly as the matrix headers: `Consumer`, `Provider`,
    `Consumer + Provider`, `Operator`, `Custom`. Summaries from the mockup S2. `docs` paths:
    - `doc/deployment-integration/roles/{consumer,provider,consumer-provider,operator}/README.md`
    - custom: `doc/deployment-integration/roles/README.md#component-matrix`
  - Add `roleMatrix:` per decision 4.
  - Update the header comment.
- **`config-tool/model/1.3.6/bundle.yaml`:** the trust-anchor bundle (decision 7). Adjust tests that assume a single
  shipped chart:
  - `internal/server/target_test.go`, `internal/model` tests over `bundles.FS`.
  - The comment in `internal/trial/trial.go` that says trust-anchor has no bundle.
  - Extend the existing "topLevelKeys equals the chart's values.yaml" test to the trust-anchor bundle.
- **Test `internal/model/rolematrix_test.go`.** `go test ./...` picks it up, and US-11.6 runs it in CI.
  - Read `../../../doc/deployment-integration/roles/README.md` and take the first `<table>` under `## Component matrix`.
  - Expand the `rowspan` cells into a grid, locate the columns by header text (`Component` and the role titles), and
    use the text content of each cell. Header lookup compares the trimmed text **exactly**. The table also has
    `Umbrella component`, `Sub-umbrella component` and `Role` (a description column, not a role), which a substring
    match would confuse with `Component` or a role title.
  - Compare against the embedded shipped bundle (`bundles.FS`, the latest DSC bundle). The test fails when:
    - a matrix component is in neither `roleMatrix.components` nor `infrastructure`, or a mapped component is missing
      from the matrix;
    - a cell has unknown text;
    - two components of one feature disagree in a role column;
    - the bundle's offer differs from the matrix.
  - A role column must exist for every role except `custom`, which is checked as "all of its chart's features
    available, none recommended". Every role's `docs` path, without its `#fragment`, must exist in the repo.
  - Put the comparison in a function over parsed inputs. A parameterized sub-test then mutates the README text in memory
    and asserts the exact message per mutation. This proves "changing the matrix without the bundle fails":

    | Mutation | Expected message |
    |---|---|
    | flip `Keycloak`/`Provider` `Required`→`Optional` (a single-component feature) | `role "provider", feature "identity.issuer": bundle says recommended, matrix says available (component "Keycloak")` |
    | flip `OPA`/`Provider` only (a feature with several components) | `role "provider", feature "access.policies": components "APISIX" and "OPA" disagree` |
    | drop the `DID` row | `component "DID" is mapped in roleMatrix but missing from the matrix` |
    | add a row `Foo` | `component "Foo" is in neither roleMatrix.components nor roleMatrix.infrastructure` |
    | set a cell to `Maybe` | `role "consumer", component "Keycloak": unknown cell text "Maybe"` |

    Only cell-level mismatches name a role and a feature. The row-level ones name the component, and the message text
    is the one in this table.
  - Add `golang.org/x/net` to the direct requires (`go mod tidy`).
- **Docs:**
  - `doc/config-tool/FEATURE_MODEL.md#role-presets`: make the table equal decision 5, expand `obs.*`, add a Custom column
    and the "matrix is authoritative" note.
  - `doc/config-tool/MOCKUPS.md#s2--role`: feature counts / floors become 2 · 5 · 5 · 1, and the "turns on" line lists
    the 5 provider features.
  - New `doc/adr/0017-config-tool-role-presets.md` (decisions 1–4, 7, 8, 11) plus its row in `doc/adr/README.md`.

**Acceptance:**
- `go test ./...` is green, including the matrix test.
- Editing one cell of the matrix locally makes the test fail with role + feature in the message.
- `make lint` is clean, and `.github/scripts/check-links.sh` passes for the touched docs.

### Step 3: Profile `role` + `features`, pure `ApplyPreset`, chart switch, ADR 0015
In `config-tool/internal/profile/`:

- **Profile fields:**
  - `Role string` (`json/yaml:"role,omitempty"`).
  - `Features map[string]FeatureState` (`json/yaml:"features,omitempty"`), where
    `FeatureState{Enabled bool; Source FeatureSource}` and the sources are `SourcePreset = "preset"` and `SourceUser = "user"`.
  - `Marshal` stays deterministic, because yaml.v3 sorts map keys. `FormatVersion` stays `1`, and decoding stays strict.
- **`Validate` adds structural errors**, using the recorded catalogue bundle (see `Catalogue` below) and new field ids
  `FieldRole = "role"`, `FieldCatalogue = "catalogue"` and prefix `features.`:
  - an unknown role id;
  - `features` without a role;
  - an unknown feature id;
  - an unknown source;
  - an enabled feature that the role marks `not-offered`;
  - a role whose `chart` differs from `target.chart`, when both are set.

  The server already maps these to 422 on save and open. Add a server test asserting 422 for an unknown role.
- **ApplyPreset:**
  ```go
  // ApplyPreset sets role and its preset features; switches target.chart when the role targets another chart.
  func (p Profile) ApplyPreset(bundles []model.Bundle, role string) (Profile, *ChartSwitch, error)
  ```
  - Pure and deterministic, and it does not modify its input maps.
  - Writes `{enabled: true, source: preset}` for `recommended` and `{enabled: false, source: preset}` for `available`,
    and omits `not-offered`.
  - Sets `Catalogue` to the catalogue bundle's ref when it differs from the resulting `Model`; otherwise it is nil
    (decision 8).
  - When `role.Chart != p.Target.Chart` (decision 9):
    - With `p.Source != nil` it returns `ErrChartSwitchLoaded` and leaves the profile unchanged.
    - Otherwise it sets `Target.Chart`, clears `Target.ChartVersion` and sets `Model` to that chart's `Latest` bundle.
      It returns `&ChartSwitch{FromChart, FromVersion, ToChart}`.
  - An unknown role returns an error.
- **Profile field `Catalogue *ModelRef`** (`json/yaml:"catalogue,omitempty"`), validated like `Model`: shipped, revision
  positive and not newer than shipped, and it declares `roles:`. It is an error without a role. `Validate` reads the
  role catalogue from `Catalogue` when set and from the `Model` bundle otherwise, so an Operator profile is checked
  against the bundle it was built with. An older catalogue revision gives the same open/save note as an older `model`.
- **Tests** in `profile_test.go`:
  - One table-driven test with a case per role, asserting the exact `Features` map from decision 5 against the shipped
    bundles. All five roles.
  - Operator sets the `trust-anchor` chart, an empty `chartVersion`, `model` trust-anchor@1.3.6 and `catalogue` = the
    DSC bundle, with `ChartSwitch{data-space-connector, 10.16.0, trust-anchor}`. Switching from operator back to
    provider returns to `data-space-connector` with an empty `chartVersion` and a nil `catalogue`. No switch → nil
    `ChartSwitch`, and `chartVersion` is kept.
  - With `source` set, operator (from a DSC profile) returns `ErrChartSwitchLoaded` and an unchanged profile. Provider
    and custom still apply.
  - `Validate`: a `catalogue` that is not shipped, or newer than shipped, is an error. A `catalogue` without a role is an
    error. An older `catalogue` revision gives a note.
  - The input is not mutated.
  - Round-trip: `ApplyPreset` → `Marshal` → `Unmarshal` gives the same role and features, and `Validate` is clean.
  - Add `testdata/role.dscprofile.yaml`, and add `Validate` failure cases to the existing table.
  - Check the existing fixtures that pin `revision: 1` for 10.16.0. If validation compares revisions, update them or
    keep them deliberately as "older revision".
- **`doc/adr/0015-config-tool-profile.md`:**
  - New section "§7 Role and features": `role` + `features` live in format 1 because the keys are optional and old
    profiles still decode; unknown role → 422; `source: preset|user` is kept for US-2.3; presets are read via
    `RoleCatalogue` and the fallback bundle is pinned in `catalogue`; the Operator chart switch is refused while a
    `source` is set and clears `target.chartVersion`.
  - Update the format example.

**Acceptance:** `go test ./internal/profile/... ./internal/server/...` is green, and `make lint` is clean.

### Step 4: API for roles and preset application
In `config-tool/internal/server/`, wired in `api.go` the same way as the existing endpoints (token auth, JSON errors):

- **`GET /api/roles?chart=&version=`** returns `{roles: [{id, title, summary, chart, floor, docs, features}], features: [{id, title, summary, group}]}`
  from `model.RoleCatalogue`. It returns 404 for an unknown chart or version.
- **`POST /api/preset`** takes `{profile, role, sourceSha256}`. `sourceSha256` names the loaded slot exactly as in
  `PUT /api/profile`, and the server resolves it with the existing `Server.source`. It calls `profile.ApplyPreset` and
  returns `{profile, chartSwitch: {fromChart, fromVersion, toChart} | null}`. An unknown role, an invalid profile or
  `ErrChartSwitchLoaded` gives 422, and a stale `sourceSha256` gives the same error as save.
- No endpoint touches a cluster, and the core stays free of `net/http` (`internal/layering_test.go`).
- **Tests:** `internal/server/role_test.go`, table-driven: happy paths, operator switch, operator with a loaded source
  → 422, unknown role 422, missing token 401.
- **UI API client:** `ui/src/api/roles.ts` (`fetchRoles`, `applyPreset`) with types mirroring the JSON, and
  `ui/src/api/roles.test.ts`. Extend `ui/src/test/api.ts` mocks.

**Acceptance:** `go test ./...` and `npm test` are green, and `make lint` is clean.

### Step 5: Stage 2 UI — `Role.tsx`, reducer, rail, Target re-offer
In `config-tool/ui/src/`:

- **State:**
  - `state/answers.ts`: add `role?: string` and `features: Record<string, {enabled: boolean, source: 'preset' | 'user'}>`
    to `Answers`. `toProfile` and `fromProfile` carry them, so a saved profile reopens with the same role and features.
  - `state/reducer.ts`: new action `roleApplied {profile, chartSwitch}`. It takes `role`, `features`, `target.chart`,
    `target.chartVersion` and the model/catalogue refs from the returned profile, and stores `chartSwitch` as a one-shot
    notice. Add tests in `reducer.test.ts`.
  - Rail selector `roleIssues` in `state/openQuestions.ts`: no role means one open question.
- **`stages/Role.tsx`**, a one-line JSDoc per component:
  - Fetch `/api/roles` for the current target. Render one card per role as in `doc/config-tool/MOCKUPS.md#s2--role`:
    title, summary, `chart: trust-anchor` on Operator, the recommended feature count, the memory/CPU floor (or
    "depends on your selection"), and a "Sizing ↗" docs link built in `links.ts`.
  - The cards form a `role="radiogroup"` with `role="radio"` / `aria-checked`, reachable with Tab and the arrow keys,
    and selectable with Space/Enter. The selected card is marked.
  - Below the cards, list the titles of the features the selected role turns on, plus "Everything else stays available
    on step 5 — nothing here is locked."
  - Selecting a role calls `applyPreset` (with `state.loaded?.sha256`) and dispatches `roleApplied`. A `chartSwitch`
    shows an inline notice naming both charts and the dropped version, with a link to Stage 1. For example: "Target chart
    switched from data-space-connector 10.16.0 to trust-anchor — pick its version on step 1", and "switched back to
    data-space-connector — pick its version on step 1".
  - While a values file is loaded (`state.loaded`), a card whose chart differs from `target.chart` is `aria-disabled`.
    It says: "Operator deploys the trust-anchor chart; the opened values.yaml is a data-space-connector file. Start a
    new configuration to set up a trust anchor."
- **`stages.ts`:** the Role entry gets `component: Role` and `selector: roleIssues`, and `comingIn` is removed.
- **Target stage:** with `target.chart = trust-anchor` it already lists only that chart's versions. Add a
  `Target.test.tsx` case that starts from an applied Operator preset. It asserts that the chart select shows
  `trust-anchor`, that the version options are `1.3.6` with none selected, and that the rail counts the version as open.
- **Tests:** `stages/Role.test.tsx`
  - five cards rendered with floors;
  - keyboard selection;
  - each role dispatches and marks the card;
  - the operator notice and the switch-back notice, both naming the dropped version;
  - with a loaded values file, the Operator card is disabled with its explanation and `applyPreset` is not called;
  - feature list text.

  Add a save → reopen test, in the existing profile/Chrome test style, asserting that role and features survive.
- Follow `styles.css` conventions, with no new UI dependency.

**Acceptance:** `make ui`, `npm test` and `make lint` are clean, and `go test ./...` is still green.

### Step 6: Documentation and full verification
- **`config-tool/README.md`:** a short *Roles* section after *Profiles*:
  - the five presets and what `recommended`, `available` and `not-offered` mean;
  - that they are data in `model/<version>/bundle.yaml` (`features:`, `roles:`, `roleMatrix:`);
  - that they come from the matrix in `doc/deployment-integration/roles/README.md`, guarded by
    `internal/model/rolematrix_test.go`, so changing the matrix means changing the bundle and bumping `revision`;
  - the floors come from `PREREQUISITES.md#sizing`;
  - Operator switches the target to `trust-anchor` and clears the chart version. This is refused while a values file is
    loaded;
  - the profile keys `role`, `features` and `catalogue`.

  Also update *Chart versions*: there is now a trust-anchor bundle, and the bundle fields list grows.
- **`doc/config-tool/USER_JOURNEY.md#stage-2--role`:** reference the shipped behaviour (the chart switch notice) where it
  differs, kept short.
- **`AGENTS.md`:** change the E2 line from "planned" to shipped and drop the `IMPLEMENTATION_PLAN.md` reference.
- Run from `config-tool/`: `make ui`, `make build`, `make test` and `make lint`. Then run
  `.github/scripts/check-links.sh`, and `helm lint charts/data-space-connector` (unchanged, sanity only). Fix anything red.

**Acceptance:** every ticket acceptance criterion holds. `go test ./...` and the UI suite are green, `make lint` is
clean, and the README has a *Roles* section.
