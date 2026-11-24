# Implementation Plan: [E3 Foundations] US-3.4 — Override one host (pinned hosts survive a domain change)

## Overview
Make every derived `host.*` foundation individually editable. An edited host is *pinned*, which means it is a given
answer and is not re-derived when `domain.base` changes. *Reset to derived* unpins it. A domain change lists the
re-derived hosts and the hosts that are *pinned, not updated*, Review shows the pinned hosts, and a new L2 error flags
two components that resolve to the same public host. The path-prefix naming scheme is dropped (human decision on the
ticket): `<component>.<base>` stays the only scheme, and there is no scheme selector.

Already shipped and reused (do not rebuild):
- Pinned state needs no new profile field. A pinned host is a given answer in `profile.Answers`, and a derived one has
  none. `render.resolve` already lets a given answer override its default, so pin state survives save/open as is.
- `render.HandSet` (`internal/render/handset.go`) already lists the given answers that override a derived default,
  with the value they would derive. Review shows it as "Set by hand" (`ui/src/stages/HandSet.tsx`).
- `DerivedField` in `ui/src/stages/Foundations.tsx` already gives the DID an *Edit* / *Reset to derived* field
  (`RESET_LABEL` in `AnswerField.tsx`). Only text that differs from the derived value counts as an edit.
- `render.Diff` / `StatusPinned` cover *leaves* hand-edited in a loaded file (ADR 0020 §7). That is a different
  mechanism. A pinned host's leaves do not change, so the leaf diff never mentions them. Step 2 adds the answer-level
  diff.

Rules for every step: model changes are bundle data, and a bundle edit bumps `revision:` in
`config-tool/model/10.16.0/bundle.yaml`. The core (`internal/*` except `internal/server`) never imports `net/http`.
An unanswered question emits no key. Tests are table-driven, with no magic constants. Use one-line GoDoc/TSDoc.
Before each PR, run `make test` and `make lint` from `config-tool/` (plus `make ui` when `ui/` changed).

## Steps

### Step 1: Complete the host set in the bundle (verifier, DID document) and add a display feature gate
The ticket asks for the full set of public names the chart needs. US-3.1 shipped 21 `host.*` foundations, but the
**verifier** and the **DID document** host are missing. In `k3s/provider.yaml` both sit on `mp-operations.org` and not
on the reference base domain, so `provider_test.go` never searched for them.
- `config-tool/model/10.16.0/bundle.yaml`:
  - Add `host.verifier` (default `verifier.{{ .domain.base }}`, label "Verifier host"). Its targets mirror
    `k3s/provider.yaml` lines ~60-110 under `decentralizedIam.vcAuthentication.vcverifier`:
    - `ingress.annotations."cert-manager.io/common-name"` / `alt-names`, `ingress.hosts[0].host`,
      `ingress.hosts[0].paths[0]` (const `/`), `ingress.tls[0].secretName` (`{{ .host.verifier }}-tls`) and
      `ingress.tls[0].hosts[0]`;
    - `deployment.server.host` (`https://{{ .host.verifier }}`);
    - `deployment.verifier.clientIdentification.id` (`x509_san_dns:{{ .host.verifier }}`), only if
      `charts/data-space-connector/values.yaml` / the vcverifier subchart declares it as a scalar.

    Check each path against the chart's `values.yaml` and the subchart defaults. Leave
    `deployment.additionalVolumes[*].secret.secretName` out, because that list carries other content. Add an exclusion
    row for it that names the owning story.
  - Add `host.did` (default `{{ .domain.base }}`, label "DID document host"). A `did:web:<base>` DID resolves at
    `https://<base>/.well-known/did.json`. It targets the `did:` subchart ingress (`did.ingress.hosts[0].host` and its
    paths/tls/annotations, as in `k3s/provider.yaml` ~709-730).
  - Add an optional `feature: <feature id>` key to a foundation. It is display and check scope only: the Stage 3
    panel hides the host while that feature is off, and the Step 3 collision rule skips it. Render gating stays
    US-5.3 (ADR 0020 consequences), so rendering is unchanged. Set it on the feature-gated hosts. Derive the mapping
    from each feature's `keys:` and the host's target top-level key:
    - `host.statusList` → `identity.statuslist`;
    - `host.dashboard` → the feature whose keys hold `fdsc-dashboard`;
    - `host.dsp`, `host.dcp`, `host.identityhub*`, `host.dcpManagement`, `host.vault` → `dsp.edc` where its keys
      cover them.

    Leave a host ungated when no single feature owns it.
  - Bump `revision` and document `feature` in the header comment.
- `internal/model/foundations.go`: add a `Feature string` field. Validation refuses an unknown feature id. Add tests
  to `foundations_test.go`, table-driven.
- `internal/render/provider_test.go`: verifier and DID leaves resolve to the reference values. Extend the reference
  handling, for example with a second needle for the DID domain, or with `chartOnlyTarget` rows. The test must keep
  failing when a target disagrees with `k3s/provider.yaml`.
- `internal/server/render.go` `foundationBody`: expose `feature` (empty when ungated). Update `ui/src/api/foundations.ts`
  plus its test.
- Acceptance: `go test ./...` is green, the new hosts render in `render_test.go`, and an unknown `feature` is refused.

### Step 2: Core answer-level diff: re-derived vs. pinned, not updated
Add `internal/render/derived.go` with `DerivedDiff(b model.Bundle, prev, next profile.Profile) (AnswerDiff, error)`.
It resolves both answer sets with `resolve` and, for every foundation whose default derives from other answers
(the same filter as `HandSet`: non-nil template, refs > 0), reports:
- `Rederived []DerivedChange{ID, Before, After}`: the answer is not given in `next` and its resolved value changed;
- `Pinned []PinnedAnswer{ID, Kept, Before, After}`: the answer is given in `next` and its *derived* value changed.
  `Before`/`After` are the derived values, and `Kept` is the given one. This is the *pinned, not updated* group.

Both lists follow `FoundationOrder`. An answer that only changes from given to unset (*Reset to derived*) is reported
as re-derived. Reuse the default evaluation of `HandSet` (factor out a small helper instead of copying it).
- Tests in `internal/render/derived_test.go`, table-driven over **the whole `host.*` set** of the shipped bundle. For
  each host: pin it, change `domain.base`, and assert that this host is in `Pinned` with its kept value, that every
  other host is in `Rederived` with `<component>.<new base>`, and that the render tree keeps the pinned host's leaves.
  Also cover reset-to-derived, an unchanged base (empty diff), and `did.value` (the existing derived DID behaves the
  same way).
- `internal/profile`: add a round-trip test. Save a profile with two pinned hosts, open it, run `render.HandSet`, and
  check that both hosts still come back pinned with their derived values. This covers "pin states survive save and
  open" in the core. No format change, still `format: 1`.
- Acceptance: the core stays HTTP-free (`internal/layering_test.go`), and `go test ./...` is green.

### Step 3: L2 rule — two components on the same public host (error)
The current rule language only knows `when` equality. Add a second rule kind that is data in the bundle:
- `internal/model/checkrules.go`: `CheckRule` gains `Distinct *RuleDistinct` (`prefix: host.`). A rule has exactly
  one of `when`/`distinct`. The prefix must match at least two foundations. `field` is omitted for a distinct rule
  because the finding names it. `message` is a Go text/template that may use only `{{ .first }}`, `{{ .second }}`
  (foundation labels) and `{{ .host }}`. Validate it at load time the way foundation templates are validated, and
  name the allowed keys as constants. Extend `checkrules_test.go`.
- `internal/check/check.go`: evaluate a distinct rule over the resolved answers (given or derived). Skip a foundation
  whose `feature` (Step 1) is off in `p.Features`. Compare hosts case-insensitively, without a trailing dot. Emit one
  finding per colliding pair, in foundation order. `Field` is `answers.<id>` of the pinned one when exactly one is
  given, else of the later one. Add `Related string` (`json:"related,omitempty"`), the other field id, so the UI can
  link both. Sorting stays as is.
- `model/10.16.0/bundle.yaml`: add rule `host-collision` (`severity: error`, `stage: foundations`,
  `distinct: {prefix: host.}`). Example message: "{{ .first }} and {{ .second }} both resolve to {{ .host }}. The
  chart routes by host, so one of them stops being reachable. Give one of them another host." Set `doc:` to
  `doc/deployment-integration/roles/PREREQUISITES.md#public-names-have-to-resolve-inside-the-cluster` or a better
  anchor. Bump `revision`.
- Tests (`internal/check/check_test.go`, table-driven):
  - pin `host.marketplace` to the derived `host.keycloak` value, and expect one error that names both labels and the
    host, with `Field` = `answers.host.marketplace` and `Related` = `answers.host.keycloak`;
  - three-way collisions;
  - a collision with a feature-gated host whose feature is off, which produces no finding;
  - the derived set alone, which produces no finding.

  In `internal/cli`, add a test that `generate` refuses a profile with the collision (`errBlockingFindings`).
- Acceptance: the shipped bundle's derived hosts never collide (test), and `go test ./...` is green.

### Step 4: Server API — derived diff in the render response
- `internal/server/server.go`: next to `lastRender`/`lastRenderSHA`, keep `lastAnswers` (the previous render's
  profile answers). Reset it wherever `lastRender` is reset (load, profile open), under the same `loadMu`.
- `internal/server/render.go`: `renderResponse` gains `Derived render.AnswerDiff` (`json:"derived"`), computed with
  `render.DerivedDiff` against the previous answers. The first render (no previous answers) returns empty lists, not
  null. `check.Finding.Related` passes through unchanged.
- *Reset to derived* needs no endpoint: the UI drops the answer and re-renders, and the derived value is already in
  `handSet[].derived` and in the UI's `defaultValue`. Do not add a preview endpoint.
- Tests in `internal/server/render_test.go`, table-driven. Render a baseline, pin `host.keycloak`, then change
  `domain.base`: `derived.pinned` holds keycloak and `derived.rederived` holds the rest. A load or profile open
  empties the baseline. A collision finding carries `related`.
- Update `ui/src/api/render.ts` (types `AnswerDiff`, `DerivedChange`, `PinnedAnswer`, `Finding.related`) and its test.
- Acceptance: `go test ./...` and the UI api tests are green.

### Step 5: UI — editable hosts, domain-change lists, Review "pinned, not updated"
- `ui/src/stages/Foundations.tsx` `Hosts`:
  - Render each host through the existing `DerivedField`: *Edit* pins, *Reset to derived* unpins and shows the value
    it would take. Keep the doc link and "used by" per row, plus the resolve warning.
  - Remove `HOSTS_EDIT_HINT`, and change `HOSTS_HEADING` to say the hosts can be overridden.
  - Show a `pinned` badge on a given host.
  - Hide a host whose `feature` is off in `state.answers.features`, unless it is pinned. A pinned host stays
    visible, so it can still be reset.
  - The per-field finding note already shows the collision error under the field it points at.
  - Field ids stay `answers.host.<x>`, so ask-later and Review focus work unchanged.
- `RenderDiff` / `useRenderDiff`: above the leaf groups, show two lists from `r.derived`: "Re-derived"
  (`label: before → after`) and "Pinned, not updated" (`PINNED_LABEL`, kept value, `would have become <after>`). Keep
  the baseline rule (the first render shows no diff).
- `ui/src/stages/HandSet.tsx` / `handSetText.ts`: give each entry the `pinned, not updated` label (reuse
  `PINNED_LABEL`, move it to a shared text module if fast refresh needs it) and keep the `would be … if derived`
  note. This satisfies "Review lists pinned hosts as *pinned, not updated*".
- `ui/src/stages/Findings.tsx`: when a finding has `related`, add a second "go ↗" link to that field.
- Tests (vitest + Testing Library, table-driven where possible):
  - `Foundations.test.tsx`: edit a host, which pins it and sends the answer; reset it, which drops the answer and
    shows the derived value; change the base, which shows both lists; hide a feature-gated host.
  - `Review.test.tsx`: the pinned label on a pinned host.
  - Findings: a collision shows two links.
  - `state/foundations.test.ts` where helpers change.
- Acceptance: `make ui`, `npm test` and `make lint` are green.

### Step 6: Documentation and ADR
- New `doc/adr/0023-config-tool-pinned-hosts.md` covers:
  - pinning is a given answer, not a profile field;
  - the answer-level derived diff next to the leaf diff of ADR 0020 §7;
  - the `distinct` rule kind and the `related` finding field;
  - the display-only `feature` gate (render gating stays US-5.3);
  - the two hosts added;
  - the path-prefix scheme dropped by decision.

  Update the range in `doc/adr/README.md` (0009 to 0023).
- `config-tool/README.md`: describe the Stage 3 hosts (editable, pinned, reset), the render response `derived`, the
  `host-collision` rule, `feature` in the bundle and the Review label. Replace "read-only … editing them is US-3.4"
  around line 327.
- `doc/config-tool/USER_JOURNEY.md` (~line 178): rewrite "Derived hosts are read-only" for the shipped behaviour.
  `doc/config-tool/FEATURE_MODEL.md` (~line 192): `host.<component>` is overridable, and pinned hosts are listed as
  *pinned, not updated*.
- `AGENTS.md`: mark US-3.3 as shipped, and replace the planned note with a terse US-3.4 line (ADR 0023, pin = given
  answer, `render.DerivedDiff`, `distinct` rule, `feature` display gate).
- Run `.github/scripts/check-links.sh` if Docker is available.
- Acceptance: docs match the code, links resolve, and `make test` / `make lint` are green.
