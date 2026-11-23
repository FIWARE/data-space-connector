# Implementation Plan: [E2 Roles] US-2.2 — Overrule the preset (deliberate choices and their consequences)

## Overview
Make every catalogue feature *of the chart being configured* toggleable regardless of the role. A toggle the user operates is recorded as
`source: user` and stays that way. A feature whose state differs from the preset is shown as a *deliberate
choice*, and turning off a `recommended` feature asks for confirmation and names the consequence, which comes
from the bundle. US-5.1 is not shipped yet, so this ticket also builds the minimal Stage 5 toggle list it needs:
three sections, one toggle per feature, nothing disabled. Per-feature configuration and the `requires` cascade
stay out of scope (US-5.x).

Starting point, merged with US-2.1 (#122): `profile.FeatureState{Enabled, Source}` with
`SourcePreset|SourceLoaded|SourceUser` (`config-tool/internal/profile/role.go`), `model.Feature`/`model.Role`/`Offer`
(`config-tool/internal/model/roles.go`), `GET /api/roles` + `POST /api/preset` (`config-tool/internal/server/role.go`),
the UI types in `ui/src/api/roles.ts`, and `answers.features` in the reducer. Stage 5 is a placeholder in `ui/src/stages.ts`.

Decisions this plan makes (step 7 records them in ADR 0018):
- **Preset value** of a feature = `offer == recommended`. *Deliberate* = `enabled != preset value` and
  `source != preset`. `source: user` survives a value that equals the preset again, but the feature then has no
  deliberate marker because it no longer differs.
- **Chart scope.** "Not offered by the role" and "not part of this chart" are two different things. The bundle marks
  every feature of the other chart `not-offered` (every DSC role lists `ta.registry`, `ta.onboarding`,
  `consent.authority` of chart trust-anchor, and Operator lists every DSC feature), because `checkRole` skips the chart
  check only for not-offered entries (`internal/model/roles.go:295-300`). Those keys do not exist in the target
  chart's values, and its `values.schema.json` refuses unknown keys. So a feature is *in scope* when its `Chart`
  (defaulted to the bundle chart by `readBundle`) equals the role's chart. Only in-scope features are toggleable, are
  listed by `FeatureChoices` and Stage 5, and may be enabled. An enabled out-of-scope feature is refused from any
  source. The ticket's "every feature can be toggled regardless of the role" reads as "every feature of the chart
  being configured".
- **Not-offered is no longer a cage** (within the chart). `Validate` refuses an enabled in-scope not-offered feature
  only when it has `source: preset`, because no preset can produce that. `ApplyPreset` keeps a loaded `true` on an
  in-scope not-offered feature as `{enabled: true, source: loaded}` instead of refusing the role.
  `ErrPresetConflictsLoaded` is removed: a values file that enables such a feature is as legitimate as a user toggle.
- **User toggles survive a role switch.** Picking a role rebuilds the preset entries, then lays loaded toggles over
  them, then carries every `source: user` entry of the previous profile over unchanged when the feature is in scope of
  the new role. A user choice is never lost silently: only a chart switch (Provider → Operator) can drop user toggles,
  and `ChartSwitch` names them so the existing switch notice in `Role.tsx` lists them. The explicit re-apply flow
  with a diff and the reset of untouched features stays with US-2.3.
- The core returns values only (preset, enabled, source, offer, deliberate). Every sentence the UI shows is either
  UI copy or bundle data (`summary`, the new optional `consequence`). No warning text lives in Go.

## Steps

### Step 1: Model — optional `consequence:` on catalogue features
- `config-tool/internal/model/roles.go`: add `Consequence string \`yaml:"consequence"\`` to `Feature`, with a one-line
  GoDoc: shown when the user turns off a feature the role recommends, falling back to `Summary`. The loader keeps it
  optional, so `checkFeature` does not require it. It refuses only a consequence made of whitespace, using the same
  style as the `summary is missing` checks.
- `config-tool/model/10.16.0/bundle.yaml`: add `consequence:` to every feature that at least one role marks
  `recommended`. Write it in the user's language and name what breaks, e.g. `access.verification`: *"Without
  credential verification this connector cannot accept a participant's access token."* Bump `revision: 2` → `3`.
  `model/1.3.6/bundle.yaml` has no roles, so leave it untouched.
- Tests (`internal/model/roles_test.go`, table-driven): a consequence is decoded, an empty one is accepted, a blank
  one is refused. Add a data test on the shipped bundles: every feature some role marks `recommended` carries a
  non-empty `consequence`, so later bundle edits cannot forget one.
- Update every fixture that pins DSC revision 2 to 3 where it means "the shipped revision":
  `internal/server/profile_test.go:672`, `ui/src/state/reducer.test.ts`, `ui/src/stages/Role.test.tsx`,
  `ui/src/stages/Target.test.tsx`. In `internal/profile/testdata/role.dscprofile.yaml`, check whether the test
  expects "older revision" notes. Either bump it to 3 or assert the note, whichever the test intends.
- Acceptance: `go test ./...` and `npm test` are green.

### Step 2: Core — `source` transitions, preset view, relaxed not-offered rule
All work is in `config-tool/internal/profile/`, which stays free of `net/http`.
- `role.go`: declare three **new** sentinels in the existing `var (...)` block next to `ErrUnknownRole`, each with a
  one-line GoDoc. They do not exist yet, and `model.ErrNoRoles` (a bundle without roles) is a different thing, so do
  not reuse it:
  - `ErrNoRole`: the profile has no role yet.
  - `ErrUnknownFeature`: the id is not a feature of the role catalogue.
  - `ErrFeatureOfOtherChart`: the feature sets values of another chart than the role deploys.
  Update the block's comment from "Errors of ApplyPreset" to "Errors of ApplyPreset and SetFeature; the API maps them
  to 422".
- An unexported helper `inScope(cat model.Bundle, r *model.Role, id string) bool` reports whether the catalogue
  feature's `Chart` equals `r.Chart`. `SetFeature`, `FeatureChoices`, `ApplyPreset` and `validateRole` all use it,
  so the chart rule lives in one place.
- New `choices.go` (or extend `role.go`):
  - `func (p Profile) SetFeature(bundles []model.Bundle, id string, enabled bool) (Profile, error)` is pure. It
    resolves the catalogue via `CatalogueBundle(bundles, p.Model, p.Catalogue)`, copies `p.Features`, and writes
    `{enabled, SourceUser}` whatever the previous source and value were. An in-scope feature absent from the map
    (not-offered) is added. Errors: `ErrNoRole` when `p.Role == ""`, `ErrUnknownFeature` when the id is not in the
    role's `Features`, `ErrFeatureOfOtherChart` when it is not in scope. On error it returns p unchanged.
  - `type FeatureChoice struct { ID string; Offer model.Offer; Preset, Enabled, Deliberate bool; Source FeatureSource }`
    with JSON tags (`id, offer, preset, enabled, deliberate, source`). It has no text field.
  - `func (p Profile) FeatureChoices(bundles []model.Bundle) ([]FeatureChoice, error)` returns one entry per
    **in-scope** catalogue feature, in bundle order. Features of the other chart are left out. An absent feature
    reports `Enabled: Preset`, `Source: SourcePreset`. `Deliberate = Enabled != Preset && Source != SourcePreset`.
    It returns `ErrNoRole` without a role.
- `role.go` `validateRole`: an enabled feature that is not in scope is refused from any source ("sets values of
  chart %s, not of chart %s that role %s deploys"). For an in-scope one, narrow the case
  `f.Enabled && o == model.OfferNotOffered` to `f.Source == SourcePreset`, with the message "is not offered by role
  %s, so it cannot come from the preset".
- `role.go` `ApplyPreset`, in this order:
  1. Preset entries as today.
  2. Loaded toggles: a loaded `true` on an in-scope not-offered feature becomes `{true, SourceLoaded}`, and a loaded
     `false` on one is still omitted. A loaded `true` on an out-of-scope feature returns `ErrFeatureOfOtherChart`
     (wrapped with the ids). Delete `ErrPresetConflictsLoaded` and the `conflicts` slice.
  3. User toggles: every `p.Features` entry with `SourceUser` is copied over unchanged when the feature is in scope
     of the new role, overwriting preset and loaded entries, because the user operated it last. Entries out of scope
     can occur only on a chart switch. Their ids go, sorted, into a new field
     `ChartSwitch.DroppedChoices []string \`json:"droppedChoices"\`` (GoDoc: "user toggles of the old chart, which
     the new chart cannot carry"), so the drop is never silent.
  Update the `ApplyPreset` GoDoc: "toggles from the loaded values file win over the preset, and user toggles win over
  both (ADR 0018)".
- `format.go:61`: change the `Features` doc comment to "a not-offered feature is enabled only by user or loaded
  choice, and only within the role's chart".
- Tests (table-driven, in `role_test.go` / new `choices_test.go`):
  - `SetFeature` transitions: preset→user (on→off, off→on); loaded→user; user→user with the value back to the
    preset's (stays `user`); absent in-scope not-offered (Consumer + `market.local`)→`{true,user}`; no role →
    `ErrNoRole`; unknown id → `ErrUnknownFeature`; Provider + `ta.registry` → `ErrFeatureOfOtherChart`; Operator +
    `access.verification` → `ErrFeatureOfOtherChart`; the input map is not mutated.
  - `FeatureChoices`: deliberate flags for recommended-off/user, not-offered-on/user, available-on/loaded,
    user-but-equal (not deliberate), and absent features. Provider's choices contain no `ta.*` or
    `consent.authority` entry, and Operator's contain only trust-anchor features.
  - `Validate`: an enabled in-scope not-offered feature passes with `user`/`loaded` and fails with `preset`; an
    enabled out-of-scope feature fails with each of `preset`, `loaded` and `user`.
  - `ApplyPreset`: replace the old conflict case with "loaded true on in-scope not-offered is kept as loaded", add
    "loaded true on an out-of-scope feature → `ErrFeatureOfOtherChart`", and add the role-switch cases: Provider with
    `access.verification` `{false,user}` → Consumer-Provider keeps `{false,user}`; Provider with `market.local`
    `{true,user}` → Consumer keeps it (now an in-scope not-offered, deliberate); a user toggle beats a loaded toggle
    of the same feature; Provider with a user toggle → Operator reports it in `ChartSwitch.DroppedChoices`; with no
    user toggles `DroppedChoices` is empty.
  - Format round trip: `Marshal` → `Unmarshal` → `Validate` keeps `{enabled: false, source: user}` on a
    recommended feature and `{enabled: true, source: user}` on a not-offered one.
- Acceptance: `go test ./internal/...` and `golangci-lint` (via `make lint`) are green, and `internal/layering_test.go`
  passes.

### Step 3: Server — `POST /api/features`, consequence in `GET /api/roles`
All work is in `config-tool/internal/server/`.
- `role.go`: add `Consequence string \`json:"consequence"\`` to `featureBody` and fill it in `catalogueBody`.
- New handler `handleFeatures` for `POST /api/features`, registered next to `/api/preset` in `api.go` behind the
  token and the body cap:
  - Request: `{profile, sourceSha256, toggle?: {feature, enabled}}`.
  - It runs `prepareProfile` exactly like `handlePreset`, then `SetFeature` when `toggle` is set, then `FeatureChoices`.
  - Response: `{profile, choices: []profile.FeatureChoice}`.
  - Errors: `ErrNoRole`/`ErrUnknownFeature`/`ErrFeatureOfOtherChart` → 422; anything else → 500.
- `handlePreset`: done in step 2, which had to keep the build green after deleting `ErrPresetConflictsLoaded`: the
  mapping is now `ErrFeatureOfOtherChart` → 422, and the `chartSwitch` body carries `droppedChoices` (never null)
  because it serialises `profile.ChartSwitch`.
- Tests (`role_test.go`, table-driven):
  - Status codes: no role → 422, unknown feature → 422, a feature of the other chart (Provider + `ta.registry`) →
    422, bad token → 401, and choices without a toggle.
  - `consequence` appears in `GET /api/roles`.
  - The **acceptance round trip**: apply the Provider preset, toggle `access.verification` off, save to a temp path,
    open it, and assert `{enabled: false, source: user}` and `deliberate: true` in a fresh `POST /api/features`.
  - Toggling the in-chart not-offered `market.local` on for Consumer succeeds and is deliberate.
  - Role switch through the API: Provider, toggle `access.verification` off, `POST /api/preset` Consumer-Provider →
    the feature is still `{false, user}`; Provider with a user toggle → Operator → `chartSwitch.droppedChoices` names
    it.
  - The existing preset test that expected a 422 for a loaded not-offered toggle already expects `source: loaded`
    (changed in step 2).
- Acceptance: `go test ./...` is green.

### Step 4: UI — API client, reducer action, accessible confirm dialog
- `ui/src/api/roles.ts`: add `consequence: string` to `Feature` and `droppedChoices: string[]` to `ChartSwitch`.
  Drop the "a loaded toggle the role does not offer" note from the `applyPreset` doc comment.
- `ui/src/stages/Role.tsx` `switchNotice`: when `droppedChoices` is non-empty, append a sentence naming the dropped
  features by catalogue title (UI copy, e.g. *Your choices for X, Y do not apply to this chart and were dropped.*).
  Add a case to `Role.test.tsx`.
- New `ui/src/api/features.ts`: `FeatureSource`, `FeatureChoice`, `FeaturesResult`, and
  `fetchFeatures(profile, sourceSha256?, toggle?)` for `POST /api/features`. Write a `features.test.ts` that mirrors
  `roles.test.ts`.
- `ui/src/state/reducer.ts`: add an action `featuresChanged { features }` that replaces `answers.features` and marks
  the profile dirty the way other edits do. Add a reducer test.
- New `ui/src/chrome/ConfirmDialog.tsx`, a generic modal used by Stage 5 now and by the `requires` cascade later:
  - `role="dialog"`, `aria-modal`, `aria-labelledby`/`aria-describedby`.
  - Focus is trapped: Tab and Shift+Tab cycle inside.
  - Escape and the cancel button call `onCancel`.
  - Initial focus goes to the **default (cancel) button**, and focus returns to the opener on close.
  - Props: `title`, `children`, `cancelLabel`, `confirmLabel`, `onCancel`, `onConfirm`.
  - Reuse the `.dialog panel` styling of `SaveDialog.tsx`.
- `ConfirmDialog.test.tsx`: initial focus is on the cancel button; Tab from the last button wraps to the first and
  Shift+Tab wraps back; Escape calls `onCancel` once; focus returns to the opener.
- Acceptance: `npm test`, `npm run lint` and `npm run typecheck` are green.

### Step 5: UI — Stage 5 Features toggle list with deliberate marker and confirm
- New `ui/src/stages/Features.tsx`, wired in `ui/src/stages.ts` in place of the Features placeholder. Configuration
  stays a placeholder.
  - When no role is picked, it shows a short hint that links to Stage 2.
  - It loads the catalogue (`fetchRoles` for the profile's chart/version) and the choices (`fetchFeatures` without a
    toggle).
  - It lists exactly the features in `choices`, so features of the other chart never appear (the core already
    leaves them out).
  - Three sections, in catalogue order within each: *Recommended for <Role>*, *Available*, *Not recommended for this
    role*.
  - Each row has a checkbox (never `disabled`), the title, the summary, and a deliberate marker when
    `choice.deliberate`. The marker is UI copy built from role title + preset + source, e.g. *recommended on for
    Provider — you turned it off*, *not recommended for Provider — you turned it on*, and for `loaded` *… — your
    values file turns it on/off*.
  - Turning off a `recommended` feature that is currently on opens `ConfirmDialog`:
    - Title: *Turn off <title>?*
    - Body: the feature's `consequence`, falling back to `summary`, plus a link *What the <Role> role needs*
      (`repositoryDocUrl(role.docs)`).
    - Buttons: *Keep it on* (default, focused) and *Turn it off*.
    - Cancel or Escape sends no request and leaves the checkbox checked.
    - Confirm calls `fetchFeatures` with the toggle and dispatches `featuresChanged`.
  - Every other toggle, including switching on a not-offered feature, calls the API directly without a dialog.
  - Toggles are **serialised**, because each request carries the whole profile and its response replaces
    `answers.features`. A queue in a ref (a promise chain) runs one `fetchFeatures` at a time, and each request is
    built from the latest profile, i.e. the previous response, kept in a ref, not from the render's closure. A
    second click while one request is pending is therefore applied on top of the first instead of overwriting it,
    and no checkbox has to be disabled. The list carries `aria-busy` while the queue is not empty. Keep the
    `mounted` ref of `Role.tsx`: a response that arrives after the stage was left is dropped and ends the queue.
    A refused toggle (422) shows its message next to the row and leaves the state of the previous response.
  - Name the UI strings as constants at the top of the file. Do not hard-code feature ids or warning text.
- `Features.test.tsx` (mocked API via `ui/src/test/api.ts`, parameterized with `it.each` where possible):
  - Section grouping, no checkbox is disabled, and a feature missing from `choices` (another chart) is not listed.
  - Not-offered on (Consumer + `market.local`): no dialog, the request carries the toggle, and the marker is shown.
  - Two toggles in a row while the first request is still pending (deferred mock): the second request is sent only
    after the first answers, its body carries the first toggle's result, and both features end up toggled.
  - A response that arrives after unmount dispatches nothing.
  - Recommended off: the dialog shows the consequence text and the role-guide link, and focus is on *Keep it on*.
  - The cancel button and Escape each leave the checkbox checked and send no request.
  - Confirm: the checkbox is unchecked and the marker reads *recommended on for Provider — you turned it off*.
- Acceptance: `npm test`, `npm run lint` and `npm run typecheck` are green.

### Step 6: UI — deliberate choices on Review
- `ui/src/stages/Review.tsx`: add a section *Deliberate choices*, shown only when a role is set and at least one
  choice is deliberate. It is fed by `fetchFeatures` (no toggle) and lists the feature title with the same marker
  text as Stage 5. Extract the marker wording into a shared helper, e.g. `ui/src/state/deliberate.ts`
  `deliberateLabel(choice, roleTitle)`, so Stage 5 and Review cannot drift. Give the helper a unit test.
- `Review.test.tsx`: the section lists a recommended feature turned off and a not-offered feature turned on, and is
  absent with no deliberate choice.
- Acceptance: `npm test`, `npm run lint` and `npm run typecheck` are green. `make test` and `make lint` pass from
  `config-tool/`.

### Step 7: Docs — ADR 0018, README, AGENTS.md
- New `doc/adr/0018-config-tool-deliberate-choices.md` (status Proposed). It covers:
  - `source: user` is sticky: it survives a role switch within the chart (user beats loaded beats preset), a chart
    switch names the user toggles it drops (`droppedChoices`), and only US-2.3 may reset untouched features or offer
    a re-apply diff.
  - The definitions of preset value and *deliberate*.
  - Chart scope: "not offered by the role" differs from "not part of this chart". Only features whose chart equals the
    role's chart are toggleable and listed, and an enabled out-of-scope feature is refused from any source.
  - In-scope not-offered features may be enabled by `user`/`loaded`, which supersedes the not-offered sentence in
    ADR 0015 §7 and the "loaded true refuses the role" bullet of ADR 0017 §7.
  - The optional `consequence:` catalogue field and the data test that requires it on recommended features.
  - The confirm dialog is a UI concern, and the core returns values only.
  - `POST /api/features`.
- Add a "partly superseded by 0018" note in ADRs 0015 §7 and 0017 §7, and a row in `doc/adr/README.md`.
- `config-tool/README.md`:
  - Fix line ~256 ("`not-offered` (cannot be enabled …)").
  - Replace the "loaded toggle refuses the role" bullet.
  - Add a short *Features (Stage 5)* section: only the configured chart's features, three sections, deliberate
    marker, confirm dialog, keyboard behaviour, user toggles kept on a role switch, `POST /api/features`, and the
    `consequence:` field.
- `AGENTS.md`: change the "E2 roles" note from planned to shipped and add one line for US-2.2: `SetFeature` /
  `FeatureChoices` in `internal/profile`, `ConfirmDialog` in `ui/src/chrome`. Its "not-offered features may be enabled"
  wording must say "within the role's chart" (the planned note predates the chart-scope rule).
  Step agents may not edit `AGENTS.md`, so step 7 leaves it alone and puts the wording in its Taiga comment for a
  plan-mode agent to apply. `doc/config-tool/FEATURE_MODEL.md` gets a one-line note on the not-offered legend.
- Run `.github/scripts/check-links.sh` if Docker is available.
- Acceptance: links resolve, and `make test` and `make lint` are green.
