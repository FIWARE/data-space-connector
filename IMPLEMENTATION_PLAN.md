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
     trust-anchor features. This is how "custom offers everything" is read. It deviates from
     `USER_JOURNEY.md#stage-2--role`, which lists Custom as "choose chart": Custom is fixed to `data-space-connector`, so
     picking it after Operator switches the chart back (decision 9). Operator already offers every trust-anchor feature,
     so nothing is lost. ADR 0017 and USER_JOURNEY record the deviation (Steps 2 and 6).
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
   declares `roles:`. Otherwise it falls back among the bundles that declare `roles:`: it takes one with `Latest` set
   (highest semver first if several are), and the highest semver only when none of them is `Latest`. A shipped
   `10.17.0-rc1` therefore never becomes the fallback while a released DSC bundle with roles exists, the same as
   "an rc never becomes the default" in `Versions`. For trust-anchor that is the latest DSC release bundle. This lets an Operator profile still show its roles. The rule is generic, with no chart name in Go code.
   Because the fallback bundle is not the profile's `model`, the profile **records the bundle its catalogue came from**
   (`catalogue: {chart, version, revision}`, set only when it differs from `model`). `Validate` reads roles and features
   from that recorded bundle, never from "whatever is newest now", and applies the same rules as for `model`: not shipped
   or a newer revision → 422 pointing at US-9.3, an older revision → an open/save note. A later DSC bundle that renames
   a `ta.*` feature therefore cannot silently invalidate a saved Operator profile, and the profile says what it was
   built against. **The server owns `catalogue` on save**, the same way `saveModel` owns `model`: a new
   `saveCatalogue(bundles, model, role, held)` runs in `handleSaveProfile` right after `saveModel`. It returns nil
   without a role, and nil when `RoleCatalogue` for the saved `model` gives the `model` bundle itself. Otherwise it keeps
   a `held` catalogue that is shipped, of the same chart as the fallback and declares roles (with the shipped
   revision, as `saveModel` does), and else takes the `RoleCatalogue` result. A client that drops `catalogue` still
   saves a valid Operator profile. The UI carries it anyway, as the held value next to `profileModel`.
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
   - **Stage 1 does not fill the cleared version in.** Today `Target.tsx` (the `useEffect` at lines 231-234) dispatches
     the latest version whenever `chartVersion` is `''`, which would undo the rule above as soon as Stage 1 mounts. The
     reducer gets a flag `versionOpen: boolean`: `roleApplied` with a `chartSwitch` sets it, and so does `profileOpened`
     when the profile's `chartVersion` is empty (a saved Operator profile without a version is not guessed on reopen
     either). The effect skips while it is set. A `setTarget` that sets a version, a `setTarget` that changes the chart
     (the Stage 1 chart select, where the preselected latest version is visible next to it, as today), `fileLoaded`,
     `trialStarted` and `newConfiguration` reset it. A fresh configuration therefore keeps the existing auto-default.
   - **A chart change on Stage 1 clears the role.** `Validate` keeps the invariant "the role's `chart` equals
     `target.chart`", and every role belongs to exactly one chart. So any change of `target.chart` through Stage 1's
     chart select clears `role`, `features` and the held `catalogue` in the reducer, and stores a one-shot notice
     `roleCleared {role, fromChart, toChart}`. Stage 1 shows it ("Role Operator was cleared because it deploys
     trust-anchor — pick a role on step 2"), and the rail counts the role as open again. Refusing the change instead
     would leave a user who picked the wrong chart first with no way forward on Stage 1. The server's `Validate` stays
     the backstop: a stale client that sends a mismatched role gets 422.
10. **Applying a preset overwrites preset features; toggles from a loaded values file win.** Nothing can be
    `source: user` yet, because Stage 5 does not exist. But a loaded values.yaml already back-derives feature toggles:
    `recognize:` maps `scorpio.enabled` and others to `<featureId>.enabled` at `stage: features`, and US-1.2 shows them
    filled in. The profile must not contradict the `source` it references, so:
    - `ApplyPreset` takes the loaded toggles (feature id → bool) as an input. The API reads them from the loaded slot's
      `Report.Answers` (stage `features`, id `<featureId>.enabled`, bool value), named by `sourceSha256`, so the client
      cannot send different ones.
    - A loaded toggle for an offered feature is kept as `{enabled: <file value>, source: loaded}`, whatever the preset
      says. Example: a file with `scorpio.enabled: false` + Provider gives `data.scorpio: {enabled: false, source: loaded}`.
    - A loaded `false` for a `not-offered` feature is omitted, like any not-offered feature.
    - A loaded `true` for a `not-offered` feature refuses the role with `ErrPresetConflictsLoaded`, naming the
      features (422). Example: Consumer on a file with `scorpio.enabled: true`. The file is not a Consumer setup, and
      enabling a not-offered feature would fail `Validate`. Nothing is cleared silently, as in decision 9.
    - `source: loaded` is a third `FeatureSource`. US-2.3's "re-apply to untouched features only" treats it like
      `user`, so a re-applied preset never overrides the file.
    - Every `recognize` rule at `stage: features` must name `<catalogue id>.enabled` (Step 1), so no loaded toggle
      can miss the catalogue.
11. **HTML parsing in the matrix test** uses `golang.org/x/net/html`, and only in `_test.go` files. `golang.org/x/net`
    is already in `go.mod` (indirect), so this adds no new module. It moves to the direct `require` block. Record this
    in ADR 0017.
12. **Feature `keys` format.** A key is a `roundtrip` path, and a path names that node **and its whole subtree**.
    FEATURE_MODEL's `keycloak.*` is therefore written `keycloak` in the bundle. No `*` notation exists: validation
    refuses any key segment that contains `*`, so a copied `keycloak.*` fails instead of meaning a literal key named
    `*`. Qualifiers such as "(central endpoints)" or "+ ingress" are not keys either; the bundle lists the concrete
    paths. **Features may share keys**, and `keys` says what a feature touches, not exclusive ownership. Today
    `vcCredentials` (identity.issuer / identity.vcoperator), `grafana` (obs.traceStore / obs.metrics) and
    `contract-management` (contract.management / market.central) overlap. The meaning US-5.1 inherits is that a
    shared key is needed while any feature that lists it is enabled. Within one feature, a key listed twice, or a key
    under another key of the same feature, is an error. ADR 0017 records this.

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
  - `keys`: parse with `roundtrip.ParsePath`; refuse a segment containing `*`, a duplicate, and a key nested under
    another key of the same feature (decision 12). Keys shared between features are valid.
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
  - When `features` is declared, every `recognize` rule with `stage: features` has an `answer` of the form
    `<catalogue id>.enabled` and `kind: toggle` (decision 10).
- **Cross-bundle checks in `Versions`**, after all bundles are read:
  - A role's `chart` must have a shipped bundle.
  - A feature's `keys` must start with a `topLevelKeys` entry of the latest bundle of the feature's chart (when that list is set).
- **`RoleCatalogue(bundles, chart, version) (Bundle, error)`** per decision 8: the fallback prefers a `Latest` bundle
  with roles and takes the highest semver only when none is `Latest`. It returns an error when no bundle declares roles.
- **Tests** in `internal/model/model_test.go`:
  - Table-driven: one fixture string per failure, each asserting its error substring.
  - A valid fixture under `testdata/bundles/`. Add a second chart fixture so the cross-chart rule is exercised; keep
    existing fixture tests green.
  - `RoleCatalogue` cases: own bundle, fallback to the latest bundle with roles, and none. An empty `version` means
    the chart's latest bundle, because a chart switch leaves `target.chartVersion` empty (decision 9). A pre-release
    case: fixtures `10.16.0` and `10.17.0-rc1`, both with roles, plus a chart without roles; the fallback is `10.16.0`.
    With only `10.17.0-rc1` declaring roles (and not `Latest`), the fallback is the rc.
  - A `recognize` rule at `stage: features` whose answer is not `<catalogue id>.enabled` is an error.
- GoDoc in one line per exported identifier, matching the file's density.

**Acceptance:** `go test ./internal/model/...` is green, the existing shipped bundle still loads unchanged, and `make lint` is clean.

### Step 2: Bundle content, trust-anchor bundle, matrix-agreement test, ADR 0017
- **`config-tool/model/10.16.0/bundle.yaml`:**
  - Bump `revision: 1` → `2`.
  - Append `features:` with the 24 catalogue entries. Take `title` and `summary` from the "Gives you" column, and
    `group`, `requires` and `keys` from `doc/config-tool/FEATURE_MODEL.md`, with keys written as bare subtree paths
    (decision 12: `keycloak.*` → `keycloak`). Trust-anchor features get `chart: trust-anchor` and trust-anchor keys.
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
    and the "matrix is authoritative" note. Under the catalogue, add one line saying the bundle writes `x.*` as `x`
    and that keys may be shared (decision 12).
  - `doc/config-tool/MOCKUPS.md#s2--role`: feature counts / floors become 2 · 5 · 5 · 1, and the "turns on" line lists
    the 5 provider features.
  - New `doc/adr/0017-config-tool-role-presets.md` (decisions 1–4, 7, 8, 10, 11, 12) plus its row in `doc/adr/README.md`.
    Under decision 3 it records that Custom is fixed to `data-space-connector`, unlike USER_JOURNEY's "choose chart",
    and why. Under decision 8 it records the `Latest`-first fallback.

**Acceptance:**
- `go test ./...` is green, including the matrix test.
- Editing one cell of the matrix locally makes the test fail with role + feature in the message.
- `make lint` is clean, and `.github/scripts/check-links.sh` passes for the touched docs.

### Step 3: Profile `role` + `features`, pure `ApplyPreset`, chart switch, ADR 0015
In `config-tool/internal/profile/`:

- **Profile fields:**
  - `Role string` (`json/yaml:"role,omitempty"`).
  - `Features map[string]FeatureState` (`json/yaml:"features,omitempty"`), where
    `FeatureState{Enabled bool; Source FeatureSource}` and the sources are `SourcePreset = "preset"`,
    `SourceLoaded = "loaded"` and `SourceUser = "user"` (decision 10).
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
- **Server save path** (`internal/server/profile.go`):
  - `saveCatalogue` per decision 8, called in `handleSaveProfile` after `saveModel`.
  - `addRecent` passes `Role: p.Role` to `recent.Entry`, which `Start.tsx` already renders.
  - Tests in `internal/server/profile_test.go`: saving an Operator profile without `catalogue` returns 200 with
    `catalogue` = the DSC bundle; a held `catalogue` of an older shipped version is kept; a Provider profile saves with
    a nil `catalogue`; a mismatched role/chart gives 422; after a save of a Provider profile the recent entry's `role`
    is `provider`.
- **ApplyPreset:**
  ```go
  // ApplyPreset sets role and its preset features; toggles from the loaded values file win (decision 10).
  func (p Profile) ApplyPreset(bundles []model.Bundle, role string, loaded map[string]bool) (Profile, *ChartSwitch, error)
  ```
  - Pure and deterministic, and it does not modify its input maps.
  - Writes `{enabled: true, source: preset}` for `recommended` and `{enabled: false, source: preset}` for `available`,
    and omits `not-offered`.
  - Then applies `loaded` per decision 10: an offered feature takes `{enabled: <value>, source: loaded}`, a loaded
    `false` on a not-offered feature is omitted, and a loaded `true` on a not-offered feature returns
    `ErrPresetConflictsLoaded` (wrapped with the sorted feature ids) and leaves the profile unchanged.
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
  - Loaded toggles, table-driven (decision 10): `{data.scorpio: false}` + provider gives
    `data.scorpio: {enabled: false, source: loaded}` and the rest of the provider preset; `{dsp.rainbow: true}` +
    consumer gives `dsp.rainbow: {enabled: true, source: loaded}`; `{data.scorpio: false}` + consumer omits
    `data.scorpio`; `{data.scorpio: true}` + consumer returns `ErrPresetConflictsLoaded` naming `data.scorpio` and an
    unchanged profile. `Validate` accepts `source: loaded`.
  - `Validate`: a `catalogue` that is not shipped, or newer than shipped, is an error. A `catalogue` without a role is an
    error. An older `catalogue` revision gives a note.
  - The input is not mutated.
  - Round-trip: `ApplyPreset` → `Marshal` → `Unmarshal` gives the same role and features, and `Validate` is clean.
  - Add `testdata/role.dscprofile.yaml`, and add `Validate` failure cases to the existing table.
  - Check the existing fixtures that pin `revision: 1` for 10.16.0. If validation compares revisions, update them or
    keep them deliberately as "older revision".
- **`doc/adr/0015-config-tool-profile.md`:**
  - New section "§7 Role and features": `role` + `features` live in format 1 because the keys are optional and old
    profiles still decode; unknown role → 422; `source: preset|loaded|user` is kept for US-2.3, which treats `loaded`
    like `user`; toggles back-derived from the loaded values file win over the preset, and a loaded `true` on a
    not-offered feature refuses the role (decision 10); presets are read via
    `RoleCatalogue` and the fallback bundle is pinned in `catalogue`, which the server recomputes on save; the Operator
    chart switch is refused while a `source` is set and clears `target.chartVersion`, which Stage 1 then leaves empty
    instead of preselecting latest; a chart change on Stage 1 clears the role.
  - Update the format example.

**Acceptance:** `go test ./internal/profile/... ./internal/server/...` is green, and `make lint` is clean.

### Step 4: API for roles and preset application
In `config-tool/internal/server/`, wired in `api.go` the same way as the existing endpoints (token auth, JSON errors):

- **`GET /api/roles?chart=&version=`** returns `{roles: [{id, title, summary, chart, floor, docs, features}], features: [{id, title, summary, group}]}`
  from `model.RoleCatalogue`. It returns 404 for an unknown chart or version.
- **`POST /api/preset`** takes `{profile, role, sourceSha256}`. `sourceSha256` names the loaded slot exactly as in
  `PUT /api/profile`. `toProfile` never sends `source`, so the decoded profile always has `Source == nil`. Before
  calling `ApplyPreset`, the handler therefore sets `p.Source, err = s.source(req.SourceSHA256)`, the same way
  `handleSaveProfile` does (`internal/server/profile.go:92`). Without that, the `ErrChartSwitchLoaded` check could
  never fire. It reads the loaded toggles from
  that slot's `Report.Answers` (decision 10; none without a loaded slot), calls `profile.ApplyPreset` and returns
  `{profile, chartSwitch: {fromChart, fromVersion, toChart} | null}`. An unknown role, an invalid profile,
  `ErrChartSwitchLoaded` or `ErrPresetConflictsLoaded` gives 422, and a stale `sourceSha256` gives the same error as save.
- No endpoint touches a cluster, and the core stays free of `net/http` (`internal/layering_test.go`).
- **Tests:** `internal/server/role_test.go`, table-driven: happy paths, operator switch, operator with a loaded source
  → 422 (the request's profile carries no `source`, only `sourceSha256`, so the test proves the handler sets it), unknown role 422, missing token 401. Loaded-slot cases: load a provider values file with
  `scorpio.enabled: false`, pick Provider → `data.scorpio` is `{enabled: false, source: loaded}`; load one with
  `scorpio.enabled: true`, pick Consumer → 422 naming `data.scorpio`.
- **UI API client:** `ui/src/api/roles.ts` (`fetchRoles`, `applyPreset`) with types mirroring the JSON, and
  `ui/src/api/roles.test.ts`. Extend `ui/src/test/api.ts` mocks.

**Acceptance:** `go test ./...` and `npm test` are green, and `make lint` is clean.

### Step 5: Stage 2 UI — `Role.tsx`, reducer, rail, Target re-offer
In `config-tool/ui/src/`:

- **State:**
  - `state/answers.ts`: add `role?: string` and
    `features: Record<string, {enabled: boolean, source: 'preset' | 'loaded' | 'user'}>`
    to `Answers`.
  - `api/profile.ts` (where `toProfile` / `fromProfile` live): the TS `Profile` gains `role?`, `features?` and
    `catalogue?: ModelRef`. `toProfile` sends `role`, `features` and `catalogue: state.profileCatalogue`; `fromProfile`
    restores `role` and `features`. A saved profile reopens with the same role and features.
  - `state/reducer.ts`: `AppState` gains `profileCatalogue?: ModelRef` next to `profileModel`. `profileOpened`,
    `profileSaved` and the new `roleApplied` set it from the profile, and `newConfiguration` clears it. The server
    recomputes it on save anyway (decision 8).
  - New action `roleApplied {profile, chartSwitch}`. It takes `role`, `features`, `target.chart`, `target.chartVersion`
    and the model/catalogue refs from the returned profile, and stores `chartSwitch` as a one-shot notice.
  - `setTarget` with a `chart` different from the current one clears `role`, `features` and `profileCatalogue`, and
    stores the `roleCleared` notice when a role was set (decision 9). A `setTarget` that leaves the chart alone keeps
    the role.
  - `AppState` gains `versionOpen: boolean` (decision 9). `roleApplied` with a `chartSwitch` sets it, and
    `profileOpened` sets it when the profile's `chartVersion` is empty. A `setTarget` that sets a non-empty
    `chartVersion` or changes the chart, `fileLoaded`, `trialStarted` and `newConfiguration` reset it. The latest-version `useEffect` in
    `stages/Target.tsx` returns early while `state.versionOpen` is set.
  - Tests in `reducer.test.ts`: `roleApplied` with and without a switch; a chart change clears role, features and
    catalogue and sets the notice; a version-only change keeps them; `profileSaved` keeps the returned catalogue;
    `versionOpen` is set by a switching `roleApplied` and by opening a profile without a version, and is not set by
    a non-switching `roleApplied`; each reset path clears it.
  - Rail selector `roleIssues` in `state/openQuestions.ts`: no role means one open question.
- **`stages/Role.tsx`**, a one-line JSDoc per component:
  - Fetch `/api/roles` for the current target. Render one card per role as in `doc/config-tool/MOCKUPS.md#s2--role`:
    title, summary, `chart: trust-anchor` on Operator, the recommended feature count, the memory/CPU floor (or
    "depends on your selection"), and a "Sizing ↗" docs link built in `links.ts`.
  - The cards form a `role="radiogroup"` with `role="radio"` / `aria-checked`, reachable with Tab and the arrow keys,
    and selectable with Space/Enter. The selected card is marked.
  - Below the cards, list the titles of the features the selected role turns on, plus "Everything else stays available
    on step 5 — nothing here is locked." Features with `source: loaded` are listed as "from your values.yaml" (on or
    off), so the user sees why the preset did not change them (decision 10). A 422 from `applyPreset` (for example
    `ErrPresetConflictsLoaded`) shows its message under the card and leaves the selection unchanged.
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
  Mounting Stage 1 must not dispatch a `setTarget` with a version (decision 9, `versionOpen`). The same holds for the
  switch back to `data-space-connector` from a pinned `10.16.0`: the select stays on "— choose a version —". Keep the
  existing fresh-configuration test that preselects the latest version, and add one where the user changes the chart
  on Stage 1 and the new chart's latest version is preselected.
  Add a second case: from an applied Operator preset, changing the chart select to `data-space-connector` shows the
  `roleCleared` notice, and the rail counts the role as open. Add the reverse case for a Provider preset switched to
  `trust-anchor`.
- **Tests:** `stages/Role.test.tsx`
  - five cards rendered with floors;
  - keyboard selection;
  - each role dispatches and marks the card;
  - the operator notice and the switch-back notice, both naming the dropped version;
  - with a loaded values file, the Operator card is disabled with its explanation and `applyPreset` is not called;
  - feature list text;
  - with a loaded file that has `scorpio.enabled: false`, picking Provider lists Scorpio as "from your values.yaml";
    a 422 for Consumer shows the message and keeps the previous selection.

  Add a save → reopen test, in the existing profile/Chrome test style, asserting that role and features survive. Add
  an Operator save test asserting that the request body carries `role`, `features` and `catalogue`.
- Follow `styles.css` conventions, with no new UI dependency.

**Acceptance:** `make ui`, `npm test` and `make lint` are clean, and `go test ./...` is still green.

### Step 6: Documentation and full verification
- **`config-tool/README.md`:** a short *Roles* section after *Profiles*:
  - the five presets and what `recommended`, `available` and `not-offered` mean;
  - that they are data in `model/<version>/bundle.yaml` (`features:`, `roles:`, `roleMatrix:`);
  - that they come from the matrix in `doc/deployment-integration/roles/README.md`, guarded by
    `internal/model/rolematrix_test.go`, so changing the matrix means changing the bundle and bumping `revision`;
  - the floors come from `PREREQUISITES.md#sizing`;
  - Operator switches the target to `trust-anchor` and clears the chart version, which step 1 then asks for instead of
    preselecting latest. This is refused while a values file is loaded. Changing the chart on step 1 clears the role;
  - toggles from a loaded values file win over the preset (`source: loaded`), and a role that does not offer a
    feature the file enables is refused;
  - the profile keys `role`, `features` and `catalogue` (set by the server on save), and the `keys` format (decision 12).

  Also update *Chart versions*: there is now a trust-anchor bundle, and the bundle fields list grows.
- **`doc/config-tool/USER_JOURNEY.md#stage-2--role`:** reference the shipped behaviour where it differs, kept short:
  the chart switch notice; Custom's chart is `data-space-connector`, not "choose chart" (decision 3); toggles from a
  loaded values file win over the preset (decision 10).
- **`AGENTS.md`:** change the E2 line from "planned" to shipped and drop the `IMPLEMENTATION_PLAN.md` reference.
- Run from `config-tool/`: `make ui`, `make build`, `make test` and `make lint`. Then run
  `.github/scripts/check-links.sh`, and `helm lint charts/data-space-connector` (unchanged, sanity only). Fix anything red.

**Acceptance:** every ticket acceptance criterion holds. `go test ./...` and the UI suite are green, `make lint` is
clean, and the README has a *Roles* section.
