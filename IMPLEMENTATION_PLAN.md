# Implementation Plan: [E2 Roles] US-2.3 — Change the role later without redoing the configuration

## Overview
Picking another role for an existing configuration (e.g. Consumer -> Consumer + Provider) re-applies the
preset to features the user never touched, keeps hand-set features and every answer, and shows a change
summary before anything is applied: *turns on*, *turns off*, *kept*, and for a move to or from
`trust-anchor` (Operator) a chart-switch block that names what is carried over and what is dropped.
Review lists *role changed from X to Y* and reopens the same summary.

### What already exists (do not rebuild)
- `profile.ApplyPreset` (`config-tool/internal/profile/role.go`) already re-applies a preset: `source: preset`
  features take the new role's value, `source: loaded` features are re-derived from the loaded file, and
  `source: user` features in the new role's chart are kept. A chart switch returns `*ChartSwitch` with
  `DroppedChoices`; it is refused with `ErrChartSwitchLoaded` while a values file is loaded.
- `POST /api/preset` (`internal/server/role.go`, `handlePreset`) is already side-effect free: it returns the new
  profile and the UI's store decides whether to take it. **No dry-run flag or new endpoint is needed.** The
  preview is the same call; applying is the UI dispatching `roleApplied`.
- `Profile.Answers` (`internal/profile/format.go`) exists, but `Validate` still refuses every key, and the UI's
  `toProfile` always sends `answers: {}`. Answer handling is therefore asserted in core tests that build
  profiles in Go, which is the "on the profile" fallback the ticket allows (US-5.3 / Stage 6 do not exist).
- `ui/src/chrome/ConfirmDialog.tsx` (focus trap, Escape = cancel, focus starts on cancel) and the one-shot
  `state.chartSwitch` notice on Stage 2 (`ui/src/stages/Role.tsx`).

### Decisions (recorded in ADR 0019, Step 6)
1. **Re-apply rule.** `preset` takes the new role's value; `loaded` is re-derived from the loaded file; `user`
   keeps its state (unchanged from ADR 0018). Both `user` and `loaded` features of the new role's chart are listed
   as *kept*, each with its source, so the UI can say "you set it" or "from your values.yaml".
2. **Answers are never cleared within a chart**, not for a feature that goes off either (US-5.2): `Answers` is
   carried over unchanged.
3. **Chart switch** (to or from `trust-anchor`): carried over are the target fields except `chart` and
   `chartVersion`, **every** open-question mark (`Open` is left as it is, as `ApplyPreset` does today; a mark on
   `target.chartVersion` stays true because that field is reset to ""), and answers whose id a `recognize:` rule
   of the old model bundle places in stage `target` or `foundations`. Every other answer and every feature state is
   dropped and named. User toggles are normally dropped too (`DroppedChoices`); `ApplyPreset` keeps one only when it
   is in scope for the new role, and such a toggle is listed as *kept* like any other. The summary never assumes
   that none survives. The switch keeps requiring the existing confirmation path (no values file loaded).
4. **Summary = core values, words = UI.** The core returns ids and booleans only; titles come from the catalogue
   (`GET /api/roles`), sentences are UI copy (ADR 0018 rule).
5. **The summary is shown whenever a role is already set.** The very first pick applies at once, as today.
6. **The Review record is session state**, not a profile field: `AppState.roleChanges` holds the summaries
   applied since the configuration was started or opened; *start over* (new/open/load/trial) drops it. The
   profile format stays untouched, so older builds still open profiles written by this one. Consequence, stated
   in the ADR: after save and reopen the record is gone; the summary stays reachable from Review for the rest of
   the session in which the role was changed.
7. **One effective-state rule.** A feature's effective state is its recorded state, else its role's preset value
   (recommended = on, otherwise off), exactly what `FeatureChoices` shows on Stage 5. `RoleChangeTo` and
   `FeatureChoices` share one helper, so the summary and Stage 5 cannot drift (e.g. a feature that a later bundle
   revision added as recommended, unrecorded in an older profile, counts as on before the change).

## Steps

### Step 1: Core — answers on re-apply and on chart switch
Files: `config-tool/internal/profile/role.go`, `config-tool/internal/profile/role_test.go`.

- Keep `ApplyPreset`'s signature. Within one chart, `out.Answers` is a `maps.Clone` of `p.Answers` (never the
  same map, never filtered), so a feature that goes off keeps its answers.
- On a chart switch, split `p.Answers` with the recognize rules of the **old** model bundle
  (`model.Bundle.Rules()`, `CompiledRule.Answer` / `.Stage`): ids whose stage is `model.StageTarget` or
  `model.StageFoundations` are carried, all others (including ids without a rule) are dropped. Keep the carried
  stages in a named package-level slice (e.g. `carriedStages`), no inline literals.
- Extend `ChartSwitch` with `CarriedAnswers []string` and `DroppedAnswers []string` (sorted, never nil, JSON
  `carriedAnswers` / `droppedAnswers`), one-line GoDoc each. Target fields are always carried, so they are not
  listed per call; the UI names them (Step 4).
- `ApplyPreset`'s GoDoc gains one line on the answers rule; refresh the `SourcePreset` comment ("US-2.3
  re-applies ...") if it no longer reads right.
- Table-driven tests (extend `role_test.go`, reuse its fixtures and helpers):
  - Consumer -> Consumer + Provider: `Features` gains exactly the features the 10.16.0 matrix recommends for
    `consumer-provider` but not for `consumer` (derive the expected set from the bundle's `roles:` in the test,
    not a hard-coded list).
  - Answers byte-identical after re-apply: marshal `Answers` with `profile.Marshal` before and after and compare,
    for a role change where features stay on and one where features go off (e.g. Provider -> Consumer).
  - Hand toggle in both directions: a `user` feature turned **on** that the new role does not recommend, and a
    `user` feature turned **off** that the new role recommends, both survive Consumer <-> Consumer + Provider.
  - Chart switch Consumer -> Operator and back: carried/dropped answer ids (use fixture bundles in
  `testdata/` if the shipped bundles have no recognize rule of stage `foundations`), target fields kept, and
  `Open` unchanged, including marks on `target.chart` and `target.chartVersion`.
  - `p` and its maps are not mutated (extend `TestApplyPresetDoesNotMutateInput` to `Answers`).

Acceptance: `go test ./internal/profile/...` green; `internal/layering_test.go` still green.

### Step 2: Core — the role-change summary
Files: new `config-tool/internal/profile/rolechange.go`, new `config-tool/internal/profile/rolechange_test.go`,
`config-tool/internal/profile/choices.go` (shared helper).

- Extract the effective-state rule of `FeatureChoices` into one unexported helper in `choices.go` (e.g.
  `effectiveState(r *model.Role, id string, recorded map[string]FeatureState) FeatureState`: the recorded state,
  else `{Enabled: r.Features[id] == model.OfferRecommended, Source: SourcePreset}`; a feature the role does not
  list is off). `FeatureChoices` calls it instead of its inline fallback; its tests stay green unchanged.
- Type `RoleChange` (JSON tags in camelCase): `FromRole`, `ToRole string`; `Enabled`, `Disabled []string`
  (feature ids whose effective state went off -> on / on -> off); `Kept []KeptFeature` (`ID`, `Enabled`, `Source`).
  All slices are never nil. Order: catalogue order of the new role's catalogue (`CatalogueBundle`), ids not found
  there sorted after it.
- Function `func (p Profile) RoleChangeTo(bundles []model.Bundle, after Profile) (RoleChange, error)`. It is pure,
  compares `p` (before) with `after` (the `ApplyPreset` result) and returns `ErrNoRole` when `p` has no role.
  One rule for both a change within a chart and a chart switch, no special case:
  - the before state of every id is `effectiveState` against the **old** role (its catalogue), the after state
    `effectiveState` against the **new** role; ids range over the union of both roles' features and both
    profiles' recorded features;
  - `Kept` = every feature recorded in `after` with source `user` or `loaded` whose state equals its before state
    (so the hand toggle that survives a chart switch, see decision 3, is listed too);
  - `Enabled` / `Disabled` = the remaining ids whose effective state changed.
- Invariants to test: `Enabled`, `Disabled` and the ids of `Kept` are pairwise disjoint; a kept feature's state is
  equal before and after; on a chart switch every id of `ChartSwitch.DroppedChoices` is absent from `Kept`, a dropped
  toggle that was on before is in `Disabled`, and one that was off is in none of the three lists. `Disabled` stays a
  plain effective-state diff; the UI removes the overlap (Step 4).
- Table-driven tests: Consumer -> Consumer + Provider (Enabled = matrix delta, Disabled empty); Consumer + Provider
  -> Consumer (Disabled = delta); the same two with a hand toggle in each direction (the feature appears only in
  `Kept`, with its source); a loaded toggle listed as kept with `source: loaded`; **an unrecorded feature the old
  role recommends** (e.g. delete it from `p.Features`, as a profile from an older bundle revision would be) counts
  as on before, so it is neither in `Enabled` nor, when the new role recommends it too, in `Disabled`; Consumer ->
  Operator (switch, features of the old chart in `Disabled`); a switch with an **off, in-scope user toggle**
  (e.g. `consent.authority: {enabled: false, source: user}` under Consumer, if the shipped bundles make it in scope
  for Operator; otherwise a fixture bundle) lists it in `Kept` and not in `DroppedChoices`; same role re-applied
  (all lists empty); no role before (`ErrNoRole`).

Acceptance: `go test ./internal/profile/...` green; each exported identifier has a one-line GoDoc.

### Step 3: API — `POST /api/preset` returns the summary
Files: `config-tool/internal/server/role.go`, `config-tool/internal/server/role_test.go`.

- `presetResponse` gains `Change *profile.RoleChange` (`json:"change"`), null when the request profile has no
  role. `handlePreset` computes it with `p.RoleChangeTo(bundles, out)` after `ApplyPreset`. `chartSwitch` stays
  where it is, now with `carriedAnswers` / `droppedAnswers`.
- No server state is written by this handler; keep it that way (a test calls it twice with the same request and
  gets equal responses, and the load slot is unchanged).
- Table-driven handler tests: first pick (`change: null`); Consumer -> Consumer + Provider (exact `enabled`, empty
  `disabled`, `answers` in the returned profile equal to the request's `{}`); hand-toggled features in both
  directions listed in `kept`; Consumer -> Operator returns `chartSwitch` plus a `change` whose `kept` holds
  exactly the user toggles `ApplyPreset` carried (none for a profile without hand toggles); a chart switch with a
  loaded file stays 422.

Acceptance: `go test ./...` green.

### Step 4: UI — change summary dialog on Stage 2
Files: `config-tool/ui/src/api/roles.ts` (+ `roles.test.ts`), new `config-tool/ui/src/stages/RoleChangeSummary.tsx`,
`config-tool/ui/src/stages/Role.tsx`, `config-tool/ui/src/stages/Role.test.tsx`, `config-tool/ui/src/test/api.ts`
if the API mock needs the new field.

- Types: `RoleChange`, `KeptFeature` mirroring the Go JSON; `PresetResult.change: RoleChange | null`;
  `ChartSwitch` gains `carriedAnswers` / `droppedAnswers`.
- `RoleChangeSummary` (one-line JSDoc): renders a `RoleChange` (+ optional `ChartSwitch`) with catalogue titles
  (fallback to the id): sections *Turns on*, *Turns off*, *Kept as they are* (each row with on/off and a reason by
  source: *you set it* for `user`, *from your values.yaml* for `loaded`); headings and reasons are named
  constants. Each empty section is omitted, and an all-empty change says that no feature changes. With a chart
  switch it leads with one sentence that the whole configuration moves from chart X to chart Y, then *Carried
  over* (namespace, release name, target type, kube context, environment, the open-question marks, plus
  `carriedAnswers`) and *Dropped* (chart version, the feature choices of the old chart, the ids of `droppedChoices`
  that are not in `change.disabled`, `droppedAnswers`); a carried hand toggle shows under *Kept as they are*. Rule:
  each feature id appears in exactly one section, so a dropped toggle that was on is listed only under *Turns off*
  and one that was off only under *Dropped*. It is presentational only, so Step 5 reuses it.
- `Role.tsx`: when a role is already set, `select` calls `applyPreset` and keeps the result in local state
  instead of dispatching. A `ConfirmDialog` shows `RoleChangeSummary`. Labels are named constants: cancel *Keep
  <old role title>*, confirm *Change to <new role title>*, and for a chart switch *Move to <chart>*. Confirm
  dispatches `roleApplied` with the held result (and the change, see Step 5). Cancel and Escape drop it, so nothing
  in the store changes. The first pick (no role before) applies at once as today. While the dialog is open further
  card clicks are ignored; a result that arrives after unmount is dropped (keep the existing `mounted` guard).
- Tests (Vitest + Testing Library, table-driven where possible): Consumer -> Consumer + Provider shows exactly the
  enabled titles; a hand-toggled feature is listed as kept in both directions; Operator shows the chart-switch
  statement, carried and dropped lists, and the *Move to trust-anchor* label; a switch to Operator with a hand
  toggle that was on (e.g. `obs.tracing`) and one that was off lists the first only under *Turns off* and the second
  only under *Dropped*, each title exactly once; **Cancel leaves the profile
  untouched**: serialise `toProfile(state)` (or capture the body of a save via the API mock) before the pick and
  after Cancel and assert the strings are equal; Escape behaves like Cancel; Confirm applies.

Acceptance: `npm test`, `npm run lint`, `npm run typecheck` green in `config-tool/ui`.

### Step 5: UI — *role changed from X to Y* on Review
Files: `config-tool/ui/src/state/reducer.ts` (+ `reducer.test.ts`), `config-tool/ui/src/stages/Review.tsx`
(+ `Review.test.tsx`), maybe a small `config-tool/ui/src/stages/RoleChanges.tsx`.

- `AppState.roleChanges: readonly AppliedRoleChange[]` (`{ change: RoleChange; chartSwitch: ChartSwitch | null }`),
  `[]` in `initialState`. `roleApplied` carries the change and appends it when it is non-null. `startOver`
  (new, load, open, trial) resets it. A chart change on Target (`setTarget`) keeps it, because the record is
  history and not a pending notice. One-line doc comment on the new field.
- Review shows a panel *Role changes* (only when the list is non-empty) with one row per change, *Role changed from
  <X> to <Y>*, and a disclosure button (`aria-expanded`) that reveals the same `RoleChangeSummary` read-only.
  Render it on both the loaded-report and the placeholder Review variants, next to `DeliberateChoices`. Role
  titles come from the catalogue via `useRoles`, falling back to the id.
- Tests: reducer (append on `roleApplied` with a change, no append for the first pick, reset on
  `newConfiguration` / `profileOpened` / `fileLoaded`); Review renders the row and expands the summary with the
  same feature titles as the dialog.

Acceptance: UI suite, lint and typecheck green.

### Step 6: ADR 0019, docs and final verification
Files: new `doc/adr/0019-config-tool-role-change.md` (format of 0017/0018), `doc/adr/README.md` (index entry),
`config-tool/README.md` (section *Roles*: the summary, the `change` field of `POST /api/preset`, the
`chartSwitch.carriedAnswers` / `droppedAnswers` fields, the Review record), `doc/config-tool/USER_JOURNEY.md` (the
re-entry row *Change the role* and the paragraph under the role table: what shipped and where it differs),
`doc/config-tool/FEATURE_MODEL.md` (one sentence under *Role presets* on re-apply), `AGENTS.md` (mark US-2.3
shipped with ADR 0019).

- ADR 0019 records decisions 1–7 from the overview (decision 6 with its save-and-reopen consequence) and the rejected options: a dry-run flag on the API
  (unneeded, the handler has no side effects), persisting the record in the profile (format change for history
  only), clearing answers of features that go off (contradicts US-5.2).
- Run from `config-tool/`: `make ui`, `make build`, `make test`, `make lint`; and
  `.github/scripts/check-links.sh` for the new and changed markdown links. Fix anything red.

Acceptance: every command above green; the docs describe only what shipped.
