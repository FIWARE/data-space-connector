# Implementation Plan: [E3 Foundations] US-3.4 — Override one host (pinned hosts survive a domain change)

## Overview
Make every derived `host.*` foundation individually editable. An edited host is *pinned*, which means it is a given
answer and is not re-derived when `domain.base` changes. *Reset to derived* unpins it. A domain change lists the
re-derived hosts and the hosts that are *pinned, not updated*, Review shows the pinned hosts, and a new L2 error flags
two components that resolve to the same public host. `<component>.<base>` stays the only naming scheme, and there is
no scheme selector.

**Open decision, path-prefix scheme.** The ticket's Work section asks for a second scheme (one host with path
prefixes) as a foundation value. The plan session read the ticket thread as dropping it, but no citable comment
(author, date) is in the repository, and USER_JOURNEY.md only says US-3.1 did not offer it. wistefan confirms on
PR #14 before Step 1 starts. If the scheme stays in scope, add a step after Step 3: a `host.scheme` choice foundation
(`subdomain` | `path`) whose `defaultWhen` switches every `host.*` default between `<component>.<base>` and `<base>`,
a `path.*` prefix per component routed through the ingresses, and a `distinct` rule over host+path, because under
the single-host scheme every host is identical by design.

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
    - `deployment.server.host` (`https://{{ .host.verifier }}`).

    Check each path against the chart's `values.yaml` and the subchart defaults. Two leaves stay out, each as an
    exclusion row in `provider_test.go` owned by US-8.11 (identity material: keys and certificates), which configures
    the verifier's signing key:
    - `deployment.verifier.clientIdentification.id` (`x509_san_dns:<host>`): it only works with `keyPath`,
      `certificatePath` and the `signing-key` volume that mounts the ingress TLS secret (provider.yaml ~103-112). The
      chart sets no `clientIdentification` by default, so a lone `id` would advertise x509_san_dns identification
      without a key to sign with;
    - `deployment.additionalVolumes[0].secret.secretName`: the list carries other content (`root-ca`).
  - Add `host.did` (default `{{ .domain.base }}`, label "DID document host"). It targets the `did:` subchart
    (`k3s/provider.yaml` ~694-730): the ingress (`did.ingress.hosts[0].host`, its `paths[0].path`/`pathType` as
    consts, the cert-manager annotations, `tls[0].secretName` `{{ .host.did }}-tls` and `tls[0].hosts[0]`) and
    `did.config.server.hostUrl` (`http://{{ .host.did }}`, as in the reference; the did-helper builds the document
    from it). `did.volumes[0].secret.secretName` stays out as an exclusion row owned by US-8.11, because the list
    entry carries `items` and pairs with `volumeMounts`.
  - The DID document host and the DID have to agree: `did:web:<host>[:path]` resolves at `https://<host>/…`. So
    `did.value`'s default changes from `did:web:{{ .domain.base }}` to `did:web:{{ .host.did }}`. The derivation
    chain is base → `host.did` → `did.value` (the foundations' topological order already resolves it), and pinning
    `host.did` re-derives the DID. A pinned `did.value` whose host differs from `host.did` is the Step 3
    `did-web-host` error, so neither case is silent. Update `did.value`'s `help` ("Derived from the DID document
    host"). The rendered values are unchanged while `host.did` is derived, which `provider_test.go` pins.
  - Add an optional `features: [<feature id>, …]` list to a foundation. It is display and check scope only: the
    Stage 3 panel hides the host while any listed feature is off, and the Step 3 rules skip it. Render gating stays
    US-5.3 (ADR 0020 consequences), so rendering is unchanged. The rule for the list:
    - the feature whose `keys:` cover the host's own ingress target (`….ingress.hosts[0].host` / `configIngress`);
    - for a host routed through APISIX (targets under `decentralizedIam.odrlAuthorization.apisix`), that is
      `access.policies`, plus the feature that deploys the route's upstream (its `nodes` in provider.yaml);
    - a host with no such feature stays ungated.

    The resulting mapping:

    | Host | `features` |
    |---|---|
    | `host.keycloak` | `identity.issuer` |
    | `host.statusList` | `identity.statuslist` |
    | `host.did` | `identity.did` |
    | `host.verifier`, `host.ccs`, `host.til` | `access.verification` |
    | `host.pap` | `access.policies` |
    | `host.scorpio` | `data.scorpio` |
    | `host.tmForumApi` | `contract.tmforum` |
    | `host.marketplace` | `market.local` |
    | `host.dashboard` | `obs.metrics` |
    | `host.dsp`, `host.dcp`, `host.identityhub`, `host.identityhubManagement`, `host.dcpManagement`, `host.oid4vcManagement` | `access.policies`, `dsp.edc` |
    | `host.dataService`, `host.tppDataService`, `host.consentDataService` | `access.policies`, `data.scorpio` |
    | `host.tppService` | `access.policies`, `access.verification` |
    | `host.tmfApi` | `access.policies`, `contract.tmforum` |
    | `host.contractManagement` | `access.policies`, `contract.management` |
    | `host.vault` | `access.policies` (no feature deploys Vault) |
  - Bump `revision` and document `features` in the header comment.
- `internal/model/foundations.go`: add a `Features []string` field. Validation refuses an unknown feature id. Add tests
  to `foundations_test.go`, table-driven.
- `internal/model` drift test over the shipped bundle: every host's ingress target (`hosts[0].host`, or the APISIX
  `hosts[n].host`) sits under the keys of a feature in its list, and no feature whose keys cover that target is
  missing from the list. The upstream feature of an APISIX host is pinned by a table in the test, one row per host,
  so a new APISIX host without a row fails.
- `internal/render/provider_test.go`: verifier and DID leaves resolve to the reference values. Extend the reference
  handling, for example with a second needle for the DID domain, or with `chartOnlyTarget` rows. The test must keep
  failing when a target disagrees with `k3s/provider.yaml`.
- `internal/server/render.go` `foundationBody`: expose `features` (empty list when ungated). Update
  `ui/src/api/foundations.ts` plus its test.
- Acceptance: `go test ./...` is green, the new hosts render in `render_test.go`, and an unknown feature id is refused.

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
  Also cover reset-to-derived and an unchanged base (empty diff). DID rows: with a derived DID, a base change
  re-derives both `host.did` and `did.value`; with `did.value` pinned, `did.value` is in `Pinned` and `host.did` in
  `Rederived` (the Step 3 `did-web-host` error then fires); with `host.did` pinned, `host.did` is in `Pinned` and
  `did.value` is in neither list, because it follows the pinned host.
- `internal/profile`: add a round-trip test. Save a profile with two pinned hosts, open it, run `render.HandSet`, and
  check that both hosts still come back pinned with their derived values. This covers "pin states survive save and
  open" in the core. No format change, still `format: 1`.
- Acceptance: the core stays HTTP-free (`internal/layering_test.go`), and `go test ./...` is green.

### Step 3: L2 rules — two components on the same public host, a did:web DID off its document host (errors)
The current rule language only knows `when` equality. Add two rule kinds that are data in the bundle:
- `internal/model/checkrules.go`: `CheckRule` gains `Distinct *RuleDistinct` (`prefix: host.`) and
  `DIDWebHost *RuleDIDWebHost` (`did: did.value`, `host: host.did`, both foundation ids). A rule has exactly
  one of `when`/`distinct`/`didWebHost`. The prefix must match at least two foundations. `field` is omitted for
  both new kinds because the finding names it. A distinct rule's `message` is a Go text/template that may use only
  `{{ .first }}`, `{{ .second }}` (foundation labels) and `{{ .host }}`. Validate it at load time the way foundation
  templates are validated, and name the allowed keys as constants. Extend `checkrules_test.go`.
- `internal/check/check.go`: evaluate a distinct rule over the resolved answers (given or derived). Skip a foundation
  with a feature of its `features` (Step 1) off in `p.Features`. Compare hosts case-insensitively, without a trailing
  dot. Emit one finding per colliding pair, in foundation order. `Field` is `answers.<id>` of the pinned one when
  exactly one is given, else of the later one. Add `Related string` (`json:"related,omitempty"`), the other field
  id, so the UI can link both. Sorting stays as is.
- `did-web-host`: when the resolved DID starts with `did:web:`, take the method-specific id up to the first `:`
  (the rest is the document path), percent-decode it and drop a `:port`, and compare it case-insensitively with the
  resolved `host.did`. Skip it while `host.did`'s features are off: the did-helper is not deployed, so the document
  is served elsewhere. `Field` is `answers.did.value`, `Related` is `answers.host.did`; the message is fixed text
  (no template keys).
- `model/10.16.0/bundle.yaml`: add rule `host-collision` (`severity: error`, `stage: foundations`,
  `distinct: {prefix: host.}`). Example message: "{{ .first }} and {{ .second }} both resolve to {{ .host }}. The
  chart routes by host, so one of them stops being reachable. Give one of them another host." Set `doc:` to
  `doc/deployment-integration/roles/PREREQUISITES.md#public-names-have-to-resolve-inside-the-cluster` or a better
  anchor. Add rule `did-web-host` (`severity: error`, `stage: foundations`, `didWebHost: {did: did.value, host:
  host.did}`), message "The DID resolves on another host than the one serving its DID document. Pin the DID document
  host to the DID's host, or reset the DID to derived.", `doc:` the provider guide's `#did-and-key-management`.
  Bump `revision`.
- Tests (`internal/check/check_test.go`, table-driven):
  - pin `host.marketplace` to the derived `host.keycloak` value, and expect one error that names both labels and the
    host, with `Field` = `answers.host.marketplace` and `Related` = `answers.host.keycloak`;
  - three-way collisions;
  - a collision with a feature-gated host whose feature is off, which produces no finding;
  - the derived set alone, which produces no finding;
  - `did-web-host`: derived DID (none), pinned `did:web:other.org` (error), `did:web:<host.did>:users:alice` (none),
    `did:web:<host.did>%3A8443` (none), `did:key` (none), `identity.did` off (none).

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
  - Hide a host with any of its `features` off in `state.answers.features`, unless it is pinned. A pinned host stays
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
  - the display-only `features` gate on foundations (render gating stays US-5.3);
  - `host.did` driving `did.value`, and the `did-web-host` rule kind;
  - the two hosts added;
  - the path-prefix scheme: dropped, citing wistefan's answer on PR #14 (see the Overview's open decision); if it
    stays in scope, the ADR covers the scheme step instead.

  Update the range in `doc/adr/README.md` (0009 to 0023).
- `config-tool/README.md`: describe the Stage 3 hosts (editable, pinned, reset), the render response `derived`, the
  `host-collision` and `did-web-host` rules, `features` on foundations in the bundle and the Review label. Replace
  "read-only … editing them is US-3.4" around line 327.
- `doc/config-tool/USER_JOURNEY.md` (~line 178): rewrite "Derived hosts are read-only" for the shipped behaviour.
  `doc/config-tool/FEATURE_MODEL.md` (~line 192): `host.<component>` is overridable, and pinned hosts are listed as
  *pinned, not updated*.
- `AGENTS.md`: mark US-3.3 as shipped, and replace the planned note with a terse US-3.4 line (ADR 0023, pin = given
  answer, `render.DerivedDiff`, `distinct`/`didWebHost` rules, `features` display gate).
- Run `.github/scripts/check-links.sh` if Docker is available.
- Acceptance: docs match the code, links resolve, and `make test` / `make lint` are green.
