# 0022 - The data space's parameters: one rendered key, two answers for later stories, button-triggered outbound calls

**Status:** Proposed
**Date:** 2026-11-24
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md#cross-cutting-values)

## Context

US-3.3 (#127) of [`USER_STORIES.md`](../config-tool/USER_STORIES.md) asks Stage 3 for what the Operator sends a new
participant: the Trust Anchor's TIR address, the credential types the data space defines and whether participant
registration is automatic. The TIR address is checked for reachability, the check is skippable, and the values can be
imported where the data space publishes `/.well-known/data-space-configuration`. Until now the tool sent nothing over
the network, and US-11.3 requires it to work offline and to send no telemetry. The L2 rule "`access.verification` on
⇒ `dataspace.tirAddress` set" of [`FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md#validation-rules) needs two
things ADR 0021 did not provide: a condition on a feature and a condition on an absent answer.

## Decision

### 1. Only the TIR address renders

`dataspace.tirAddress` (type `url`) has one target, `decentralizedIam.vcAuthentication.vcverifier.deployment.verifier.tirAddress`
(`k3s/provider.yaml`: `https://tir.127.0.0.1.nip.io/`). The other TIR leaves of provider.yaml are rows of the exclusion
table in `internal/render/provider_test.go`:

| Leaf | Owning story | Reason |
|---|---|---|
| `vcverifier.registration.services[*].oidcScopes.*.credentials[*].trustedParticipantsLists[*]` | US-5.6 | The verifier's service registration list carries content no foundation owns; US-5.6 pre-seeds it from this story's answers |
| `vcverifier.deployment.initContainers[*].args[*]` | US-3.3 | A k3s-local curl script that registers at the in-cluster TIR; no chart key does this |
| `registration.issuer[*].did` | US-5.5 | The participant's local TIL list, which names other participants |

### 2. Credential types and auto-registration are answers only

The two answers are not rendered, and each has its own reason:

- `dataspace.autoRegistration` (type `bool`): chart 10.16.0 has no key for registering at the Trust Anchor. The
  chart's `registration.*` registers issuers in the participant's *own* TIL, which is a different thing. No chart key
  is invented.
- `dataspace.credentialTypes` (type `credentialTypes`, an ordered list of `{type, description?}` with unique types):
  the chart key exists, `...registration.services[*].oidcScopes.*.credentials[*].type` (provider.yaml line 174). It
  sits in the same service registration elements as the `trustedParticipantsLists` of §1, a list that US-5.6 owns.
  US-5.6 pre-seeds each element's `type` and `trustedParticipantsLists` from the answers.

Core package `internal/dataspace` gives later stories typed access (`CredentialTypes`, `AutoRegistration`), for US-5.5
(local TIL, issuance), US-5.6 (verifier registration) and `NEXT_STEPS.md`. Both return nothing for an unanswered or
invalid answer.

### 3. No data space identifier

No chart key reads the data space's identifier in 10.16.0, so there is no `dataspace.id` or data space DID foundation.

### 4. The descriptor

The chart's `dsconfig-cm.yaml` publishes `supported_models`, `supported_protocols` and `authentication_protocols`.
The tool also reads three **optional** top-level keys in the same snake_case style:

| Key | Shape | Answer |
|---|---|---|
| `tir_address` | URL string | `dataspace.tirAddress` |
| `credential_types` | list; each element a string or `{type, description?}` | `dataspace.credentialTypes` |
| `participant_registration` | `"automatic"` or `"manual"` | `dataspace.autoRegistration` |

The chart's three keys are shown as *also published* and never applied. Every other key is listed as *not used*.
`internal/dataspace.ParseDescriptor` takes a JSON object of at most 64 KiB, and its errors never echo the document.
`Preview` compares the imported values with the profile's given answers. It returns *changes* (current and imported
value), *problems* (a value the answer's type refuses, which is not offered) and the key lists. Nothing is applied
until the user presses *Apply*.

**The import is forward-looking.** No publisher emits the three keys yet, the chart's `dsconfig-cm.yaml` included,
so today every real data space yields an empty preview. Its message says so, and the user does not read it as a
failure: "This data space does not publish a TIR address, credential types or a registration mode yet. Enter them
below." The unused keys are still listed, which shows the document was read. Publishing the keys from the chart's own
`dataSpaceConfig` is a follow-up.

### 5. The reachability record is session state

The TIR check records its outcome for the address it ran for: *reachable* (any HTTP response, with its status),
*unreachable* (the transport error) or *not checked*. Review shows it as information, never as a finding. Like the
role-change record ([ADR 0019](0019-config-tool-role-change.md#6-the-review-record-is-session-state)), it is not a
profile field, because it is tied to one moment. Changing the address makes it *not checked* again.

### 6. Rule grammar: features and unset

A rule's `when` may address `features.<feature id>` of the bundle's catalogue, with `equals: "on"` or `"off"`. A
feature the profile does not record is off. A `when` on `answers.<id>` may use `unset: true` instead of `equals`; it
holds when the resolved answer, given or derived, is absent. A `when` has exactly one of `equals` and `unset`, and
`readBundle` refuses anything else. The 10.16.0 bundle uses both in the error rule `tir-address-for-verification`:
`features.access.verification` on and `answers.dataspace.tirAddress` unset. This amends
[ADR 0021 §2](0021-config-tool-l2-rules.md#2-conjunctive-equals-conditions).

### 7. A loaded TIR address is back-derived and seeded

`internal/load` derives answers only from `recognize:` rules. Without one, a loaded file that sets
`verifier.tirAddress` would leave `dataspace.tirAddress` unanswered, and §6 would block `generate` on a correct file.
The bundle therefore has a `recognize:` rule (`kind: value`, stage `foundations`) on the target path, the first for a
foundation answer. The leaf counts as recognized: it derives the answer, re-renders to the same bytes and is not
pinned ([ADR 0020 §7](0020-config-tool-foundations-render.md#7-hand-edits-are-pinned-by-load-classification)). A loaded
value the `url` type refuses derives no answer and gets a load note. The leaf stays pinned and the rule fires, which is
correct, because the verifier would get an unusable address.

`load.Load` only lists derived answers in the report, and `check.Evaluate` reads the profile's answers and defaults.
`load.SeedAnswers(report, answers)` therefore copies every derived `foundations`-stage answer that the profile does not
answer yet, without mutating its input. `POST /api/profile/open` seeds the profile it returns, and the UI seeds the
answers of a `POST /api/load` report the same way. A seeded answer counts as a given answer: profile answers carry no
origin, and adding one would change the profile format. The value is the user's own and renders to the same bytes. A
given answer always wins, so seeding never overwrites an edit. Feature toggles keep their own `source: loaded` path.

### 8. Outbound calls

The TIR check and the descriptor import are the tool's only outbound HTTP requests:

- **User-triggered only.** Each runs on a button press (*Check*, *Import*) and never on its own: not on load, open,
  render or a timer.
- **Server layer only.** `POST /api/dataspace/check` and `POST /api/dataspace/import` live in `internal/server`.
  `internal/dataspace` parses and previews but never touches the network, so the core stays free of `net/http`.
- **One swappable transport.** Both go through `Server.outbound`, which tests replace, so no test reaches the network.
  It takes the proxy from the environment (`HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY`), read once at start.
- **Bounded.** A URL must pass the `url` type. The whole exchange times out after 10 s. The check sends one GET
  and follows no redirect. The import appends `/.well-known/data-space-configuration` to a base URL, follows at most
  5 redirects and refuses one that changes the scheme, needs a 2xx answer and reads at most 64 KiB plus one byte, so an
  oversized body is refused.
- **No leak back.** Errors do not echo the URL or the document. The server writes nothing: the import returns a
  preview, and the check returns an outcome.

US-11.3 (offline) holds: with no button pressed, the tool sends nothing, and a failed call leaves every answer
editable by hand.

## Consequences

- A participant can configure the data space by hand today. The import becomes useful once a data space publishes
  the §4 keys.
- US-5.5 and US-5.6 read `internal/dataspace` instead of the profile's raw answers.
- A profile saved by an earlier build lacks the seeded TIR address. A CLI `generate` on it fails on
  `tir-address-for-verification`, which names the field, until the profile is opened and saved once in the GUI.
- A new outbound call needs an amendment to §8.

## Alternatives

- **Render the credential types into the verifier registration now.** The list is owned whole or not at all
  ([ADR 0020 §4](0020-config-tool-foundations-render.md#4-lists-are-owned-whole)), and its other content belongs to
  US-5.6.
- **Invent a chart key for auto-registration.** The umbrella chart would carry a value no template reads.
- **Fetch in the core.** It would break the layering rule and make every core test a network test.
- **Check the TIR on every change of the address.** It would send requests the user did not ask for, against
  US-11.3.
- **Record the reachability in the profile.** A stale "reachable" from last month would read as current.
- **Store the seeded answer's origin in the profile.** It changes the format for a value that renders to the same
  bytes either way.
