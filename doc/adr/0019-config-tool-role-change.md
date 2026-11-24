# 0019 - A role change re-applies the preset to untouched features, keeps answers, and is confirmed from a summary

**Status:** Proposed
**Date:** 2026-11-24
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/USER_JOURNEY.md`](../config-tool/USER_JOURNEY.md#stage-2--role)

## Context

US-2.3 (#124) of [`USER_STORIES.md`](../config-tool/USER_STORIES.md) lets the user pick another role for an
existing configuration (Consumer → Consumer + Provider) without redoing it. The preset is re-applied only to
features the user never touched, hand-set features and answers survive, and a summary of what changes is shown
before anything is applied. Review records the change.

Most of the mechanics already existed. `ApplyPreset` (ADR 0017, ADR 0018 §4) rebuilds the preset entries, lays
the loaded toggles over them and keeps in-scope `source: user` toggles. A move to or from Operator switches the
chart and returns a `ChartSwitch`, and it is refused while a values file is loaded. `POST /api/preset` returns the
new profile and writes no server state: the UI decides whether to take it. What was missing was a summary of the
change, a rule for answers, and a record on Review.

## Decision

### 1. Re-apply rule

`source: preset` takes the new role's value, `source: loaded` is re-derived from the loaded file, and
`source: user` keeps its state (unchanged from ADR 0018). The summary lists both `user` and `loaded` features of
the new role's chart as *kept*, each with its source, so the UI can say "you set it" or "from your values.yaml".

### 2. Answers survive within a chart

`Answers` is carried over unchanged, also for a feature that goes off (US-5.2: turning a feature off keeps its
answers for when it comes back).

### 3. Chart switch

A move to or from `trust-anchor` carries over the target fields except `chart` and `chartVersion`, every
open-question mark (a mark on `target.chartVersion` stays, because that field is reset to ""), and the answers a
`recognize:` rule of the old model bundle places in stage `target` or `foundations`. Those hold no chart-specific
values. Every other answer and every feature state is dropped. `ChartSwitch` names them: `droppedChoices` (user
toggles), `carriedAnswers` and `droppedAnswers` (answer ids, sorted). `ApplyPreset` keeps a user toggle only when
it is in scope for the new role, and such a toggle is listed as *kept*, so the summary never assumes that none
survives. The switch still needs no values file to be loaded.

Amended by [ADR 0020 §9](0020-config-tool-foundations-render.md#9-a-chart-switch-carries-declared-foundation-answers):
an answer is carried only when the new bundle also declares it in `foundations:`.

### 4. The core returns values, the UI words them

`Profile.RoleChangeTo(bundles, after)` returns `RoleChange {fromRole, toRole, enabled, disabled, kept}`: ids,
booleans and sources only, in catalogue order. `POST /api/preset` returns it as `change` (null on the first pick).
Titles come from the catalogue (`GET /api/roles`), sentences are UI copy (ADR 0018 §6).

### 5. The summary is shown whenever a role is already set

Picking a role when the configuration has one sends `POST /api/preset` as a preview and shows its `change` in a
confirmation dialog: *Turns on*, *Turns off*, *Kept as they are*, and for a chart switch *Carried over* and
*Dropped*. Focus starts on *Keep <role>*, and Escape keeps the old role. Confirming applies the result already
fetched. The very first pick applies at once, as before.

### 6. The Review record is session state

`AppState.roleChanges` holds the changes applied since the configuration was started or opened. Review lists
each as *Role changed from X to Y* and reopens the same summary, read-only. Every start over (new, open, load,
trial) clears the list. The profile format is untouched, so older builds still open profiles written by this
one. The consequence: after save and reopen the record is gone. The summary stays reachable from Review for the
rest of the session in which the role was changed.

### 7. One effective-state rule

A feature's effective state is its recorded state, else its role's preset value (recommended = on, otherwise
off). This is what `FeatureChoices` shows on Stage 5. `RoleChangeTo` and `FeatureChoices` share one helper, so
the summary and Stage 5 cannot drift. A feature that a later bundle revision added as recommended, unrecorded in
an older profile, counts as on before the change.

## Consequences

- A role change never clears an answer within a chart, so a feature turned off by the new role is configured
  again as soon as it is turned back on.
- A chart switch names everything it drops before it is applied, answers included.
- Answer handling is tested in core tests that build profiles in Go. The UI sends no answers yet, because no
  stage asks questions that become profile answers.
- Re-opening a saved profile shows no role-change history.

## Alternatives

- **A dry-run flag or a preview endpoint.** Unneeded: the handler already has no side effects, so the preview
  and the apply are the same call.
- **Persisting the record in the profile.** A format change for history only, which older builds could not open.
- **Clearing the answers of features that go off.** Contradicts US-5.2, and the user would retype them when the
  feature comes back.
- **Showing the summary on the first pick too.** Nothing is lost on the first pick, so a dialog would only add a
  click.
