# 0018 - User toggles are sticky, scoped to the role's chart, and the core returns values only

**Status:** Proposed
**Date:** 2026-11-23
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/USER_JOURNEY.md`](../config-tool/USER_JOURNEY.md#stage-5--features)

## Context

US-2.2 (#123) of [`USER_STORIES.md`](../config-tool/USER_STORIES.md) makes the preset a starting point
and not a cage: every feature can be toggled regardless of the role, a feature that differs from the
preset is marked as a deliberate choice, and turning off a feature the role requires warns with the
consequence and then allows it.

Three earlier rules stood in the way. ADR 0015 §7 refuses an enabled feature the role marks
`not-offered`. ADR 0017 §7 refuses a role when the loaded file enables a feature that role does not
offer. And the bundle marks every feature of the *other* chart `not-offered`: each data-space-connector
role lists `ta.registry`, `ta.onboarding` and `consent.authority`, and Operator lists every DSC feature,
because the bundle check skips the chart check only for not-offered entries. Those keys do not exist in
the target chart, and its `values.schema.json` refuses unknown keys, so "every feature" cannot mean
those.

## Decision

### 1. Preset value and *deliberate*

The preset value of a feature is `offer == recommended`. A feature is *deliberate* when
`enabled != preset value` and `source != preset`. `source: user` stays when the user toggles a feature
back to its preset value, but the feature then has no deliberate marker, because it no longer differs.

### 2. Chart scope

"Not offered by the role" and "not part of this chart" are two different things. A feature is in scope
when its `chart` (defaulting to the bundle's chart) equals the role's chart. Only in-scope features are
toggleable and listed by `FeatureChoices` and Stage 5. An enabled out-of-scope feature is refused from
any source (`ErrFeatureOfOtherChart`, 422): by `Validate`, by `SetFeature` and by `ApplyPreset` for a
loaded `true`. The story's "every feature" reads as "every feature of the chart being configured".

### 3. Not-offered features may be enabled within the chart

`Validate` refuses an enabled in-scope not-offered feature only with `source: preset`, because no preset
produces one. `ApplyPreset` keeps a loaded `true` on such a feature as `{enabled: true, source: loaded}`,
and `ErrPresetConflictsLoaded` is gone. A values file that enables the feature is as legitimate as a user
toggle. This supersedes the not-offered sentence of ADR 0015 §7 and the "loaded `true` refuses the role"
bullet of ADR 0017 §7.

### 4. `source: user` is sticky

`SetFeature` records every toggle as `{enabled, source: user}`, whatever decided the feature before.
Picking a role rebuilds the preset entries, lays the loaded toggles over them and then carries every
`source: user` entry of the previous profile over unchanged when it is in scope of the new role. So
user beats loaded beats preset. Only a chart switch (Provider → Operator) can drop user toggles, and
`ChartSwitch.droppedChoices` names them, so the switch notice lists them. A user choice is never lost
silently. Resetting untouched features and a re-apply diff stay with US-2.3, which re-applies a preset
only to `source: preset` features.

### 5. The consequence is bundle data

A catalogue feature has an optional `consequence:`, the sentence shown when the user turns the feature
off while the role recommends it. A blank value is refused, and the UI falls back to `summary` when none
is set. `TestShippedRecommendedHaveConsequence` requires a consequence on every feature that a shipped
role recommends, so a new recommendation cannot ship without one.

### 6. The core returns values, the UI words them

`FeatureChoices` returns `id`, `offer`, `preset`, `enabled`, `source` and `deliberate` per in-scope
feature, in catalogue order. No warning text lives in Go. The deliberate marker is UI copy built from the
role title, the preset value and the source (`ui/src/state/deliberate.ts`, shared by Stage 5 and Review).
The confirmation before turning off a recommended feature is a UI concern: `ui/src/chrome/ConfirmDialog.tsx`
traps focus, focuses the default *Keep it on* button, closes on Escape and returns focus to the opener.
Every other toggle, including turning on a not-offered feature, needs no confirmation.

### 7. API

`POST /api/features` takes `{profile, sourceSha256, toggle?: {feature, enabled}}`, behind the per-run
token and the body cap. It prepares the profile like `POST /api/preset`, applies `SetFeature` when
`toggle` is set and answers `{profile, choices}`. Without `toggle` it only reads the choices. No role,
an unknown feature and a feature of another chart are 422. `GET /api/roles` adds `consequence` to each
feature (empty when the bundle names none). The `chartSwitch` of `POST /api/preset` carries
`droppedChoices`, never null.

## Consequences

- A profile may hold an enabled not-offered feature with `source: user` or `loaded`. Profiles of the
  earlier rules still open, because the rules only got looser within the chart.
- A values file that enables a feature the role does not offer no longer blocks the role.
- Each toggle is a round trip, because the core computes the choices. Stage 5 serialises the requests, so
  no checkbox has to be disabled while one is pending.
- Adding a recommendation to a role needs a `consequence` on that feature, or `go test ./...` fails.

## Alternatives

- **Keep not-offered as a cage.** It contradicts the story, and the role guide's `-` means "this role
  does not need it", not "this role cannot run it".
- **Make out-of-scope features toggleable too.** Their keys do not exist in the target chart, so the
  output would fail schema validation.
- **Clear user toggles on every role switch.** A choice would vanish without a word, and US-2.3 owns the
  explicit re-apply.
- **Warning texts in Go.** The core would carry UI wording, and the CLI and the web UI could not word it
  for their own medium.
- **Consequence hard-coded in the UI.** It would name feature ids in TypeScript and drift from the
  catalogue.
