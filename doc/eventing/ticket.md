# Ticket: Data space eventing and participant-removal cleanup

**Type:** Epic (7 sub-tickets across 4 repositories)
**Design:** [architecture.md](architecture.md) — **open points: [questions.md](questions.md)**
**Status:** ready for refinement, blocked on Q1–Q3 (see [Decisions needed](#decisions-needed-before-implementation))

## Summary

Give a data space a generic, OID4VP-authenticated event channel between its entities, and use it for
the first concrete case: when the operator removes a participant from the trust anchor's Trusted
Issuers Registry, every participant learns about it and cleans up its local Trusted Issuers List
according to its own configured policy.

## Motivation

Removing a participant from the TIR is invisible to everybody else. Each participant keeps TIL
entries for the removed DID — written on order activation, by `registration.issuer` at deploy time,
or by hand — and nothing ever removes them. Where a service's credential configuration omits
`trustedParticipantsLists`, VCVerifier trusts the local list alone
(`VCVerifier/verifier/trustedparticipant.go`), so those entries are not stale decoration but a live
grant to an organization the data space has expelled. Everywhere else they are silent drift that no
operator can see.

There is also no channel at all for one entity of a data space to tell another that something
happened, so every future need of that kind would grow its own callback. This epic builds the
channel once, on the authentication the connector already uses, and consumes it for one use case.

## Scope

**In scope**

- A CloudEvents-based event API, an OID4VP-authenticated subscription API, and per-publisher,
  per-type authorization at the receiver.
- The `org.fiware.dataspace.participant.removed` event, derived at the trust anchor by watching the
  TIR.
- Configurable cleanup of the local TIL, along the grant scope, with per-DID overrides and a report.
- A reconciliation sweep (local TIL vs. the trust anchor's TIR) as the backstop and as the fix for
  the entries that already dangle today.
- Chart wiring, an example ODRL policy, an end-to-end integration test, documentation, ADRs.

**Out of scope**

- Participant-to-participant configuration delivery after a purchase (the mechanism is designed to
  carry it later; see architecture §14).
- Durable/auditable delivery (outbox, event log, catch-up feed) — architecture §7 and D-4.
- Cleaning up ODRL policies, Rainbow/EDC agreements or issued credentials on removal (Q5).
- Any change to how an operator removes a participant.

## Decisions needed before implementation

| | Question | Blocks |
|---|---|---|
| **Q1** | contract-management as both ends, or a dedicated `dataspace-events` service? | everything |
| **Q2** | Best-effort stateless delivery, or a persisted outbox + subscriptions from the start? | CM-2, CM-3 |
| **Q3** | Default cleanup mode: `unscoped`, `none` or `full`? | CM-5, DSC-2 |

The remaining open points (Q4–Q11) can be settled during implementation; each names the sub-ticket
it lands in.

---

## CM-1 — Event envelope, receiving endpoint and authorization

**Repo:** `FIWARE/contract-management`

- Add `api/dataspace-events.yaml` (OpenAPI-first, generated like `contract-management.yaml`):
  `POST /events` taking a CloudEvents 1.0 JSON envelope.
- `EventListener` controller behind `events.listener.enabled`, dispatching to a
  `DataSpaceEventHandler` interface — one implementation per event type, mirroring the existing
  `TMForumEventHandler` registry in `NotificationListener`.
- Authorization (architecture §6): verify the bearer against the configured verifier JWKS, read the
  holder DID, require `source == authenticated DID`, require the `(publisher, type)` pair to be
  configured, ignore events whose `subject` is this participant's own DID.
- Deduplicate on `(source, id)` with a bounded, time-limited in-memory cache.
- Unknown `type` → `202` + metric, never an error.
- Configuration: `events.listener.*` as `@ConfigurationProperties`, with a `Condition` in the style
  of `GeneralProperties.CentralMarketplaceCondition`.

**Acceptance**

- A correctly authenticated, configured publisher gets `202`; an unknown type gets `202` and is
  counted; a known type from an unconfigured DID gets `403`; a mismatched `source` gets `403`; a
  replay gets `202` and acts once; an event about oneself is recorded and not acted on.
- Parameterized unit tests over the authorization matrix (JUnit 5 `@ParameterizedTest`).

## CM-2 — Publisher: subscriptions and delivery

**Repo:** `FIWARE/contract-management`

- `POST /subscriptions`, `DELETE /subscriptions/{id}` behind `events.publisher.enabled`:
  OID4VP-authenticated, `subscriberId` must equal the authenticated DID, callback host validated
  against that DID, leased with a TTL, capped per DID and in total.
- Delivery: for each matching subscription, `POST` the event through
  `ContractManagementAdapter`'s pattern — `Oid4VpAuthHandler.executeWithAuth` with the
  subscription's `clientId`/`scopes` — with exponential backoff and a bounded attempt count,
  then drop with an error log and a metric.
- Internal `POST /events` (publisher side, not published through the ingress) so an operator or the
  onboarding portal can announce an event directly.

**Acceptance**

- A subscription whose `subscriberId` or callback host does not match the authenticated DID is
  rejected; an expired lease stops delivery; a `401` from a subscriber triggers one OID4VP token
  fetch and a retry; a permanently failing subscriber does not block delivery to the others.

## CM-3 — Subscriber: registration and renewal

**Repo:** `FIWARE/contract-management`

- A `DataSpaceEventSubscriber` modelled on `NotificationSubscriber`: register at every configured
  publisher on `ServerStartupEvent`, retry on failure, renew before the lease expires, and report
  through a health indicator like `SubscriptionHealthIndicator`.

**Acceptance**

- A publisher that is unavailable at start-up is retried and the health indicator stays unhealthy
  until registration succeeds; a lease is renewed before it expires; `409` on an existing
  subscription is treated as success (as the TMForum subscriber already does).

## CM-4 — Trust anchor watcher: deriving `participant.removed`

**Repo:** `FIWARE/contract-management`

- A scheduled `TirWatcher` (own `micronaut.executors` entry, as `notification-subscriber` has)
  enumerating the configured registry and diffing against the previous snapshot
  (architecture §8).
- Safeguards, all configurable and all with named constants: snapshot only on a complete
  enumeration, `confirmations` consecutive cycles before announcing, `maxRemovalsPerCycle` circuit
  breaker that emits nothing and raises an alert, silent first snapshot.
- Emits through CM-2.

**Acceptance**

- Parameterized tests for: a clean removal; a registry error mid-enumeration (nothing emitted,
  snapshot unchanged); an empty registry (nothing emitted, alert); a DID that reappears before the
  confirmation count (nothing emitted); a restart (nothing emitted on the first cycle); more than
  the cap disappearing (nothing emitted, alert).

## CM-5 — Reacting: TIL cleanup, overrides and the report

**Repo:** `FIWARE/contract-management`

- `ParticipantRemovedHandler` applying `events.participantRemoval.mode` (`none` | `unscoped` |
  `granted` | `full`) with `keep` and per-DID `overrides`, through `TrustedIssuersListAdapter`
  extended with the calls CM needs (requires TIL-1/TIL-2).
- The reconciliation sweep (architecture §10): at start-up and on a timer, diff the local TIL
  against the trust anchor's TIR, apply the same policy, capped by `maxRemovalsPerRun`.
- Reporting in every mode: log, `dsc_dataspace_events_participant_removed_total{action,mode}`, and
  `GET /events/participants/removed` listing the DIDs the data space removed for which local entries
  remain, with the scopes holding them.

**Acceptance**

- Parameterized tests over mode × origin of the entry: `unscoped` keeps order-scoped grants and
  removes the hand-managed entry; `granted` does the opposite; `full` deletes the issuer; `none`
  changes nothing; an override beats the global mode; a DID on `keep` is never touched in either
  path; the report lists what was kept.
- A failing TIL call yields `502` to the publisher (so it retries) and leaves nothing half-applied.

## TIL-1 — Delete the unscoped credentials of an issuer

**Repo:** `FIWARE/trusted-issuers-list`

`DELETE /issuer/{did}/credential` requires `scope` today and answers `400` without it, so the
credentials that *no* grant owns cannot be removed selectively. Add that, explicitly (the exact
spelling — omitted parameter vs. an explicit flag — is Q6).

**Acceptance:** removes only credentials with no scope, leaves every scoped grant, keeps the issuer,
`404` for an unknown issuer, idempotent. Covered in `TrustedIssuersListControllerTest`.

## TIL-2 — Report the scopes holding credentials for an issuer

**Repo:** `FIWARE/trusted-issuers-list`

`GET /issuer/{did}/scope` → the scope ids that currently hold credentials for this issuer (with a
count). Needed by the `granted` mode and by the report. The scope stays out of the TIR projections,
as it is today.

**Acceptance:** lists exactly the distinct scopes, empty array for an issuer with only unscoped
credentials, `404` for an unknown issuer.

## DSC-1 — Chart: participant side

**Repo:** this one

- `contract-management.events.*` in `charts/data-space-connector/values.yaml` with `# --` comments
  for helm-docs, every key defaulting to off; regenerate `values.schema.json`
  (`.github/scripts/values-schema.sh`) and the README values table.
- Events path on the contract-management ingress (and, if the subchart does not support it,
  `FIWARE/helm-charts` first).
- Example ODRL policy `it/src/test/resources/policies/allowDataSpaceEvents.json` restricting the
  events path to the trust anchor's credential type.
- helm-unittest suite `charts/data-space-connector/tests/eventing_test.yaml`: nothing rendered when
  disabled, the full block rendered when enabled, user-provided `env`/`additionalEnvVars` not
  clobbered.

## DSC-2 — Chart: trust anchor side, demo and test

**Repo:** this one

- Wire the publisher into `charts/trust-anchor` (or document the second-release path — Q8), with its
  OID4VP identity.
- `k3s/` overlays: publisher at the trust anchor, listener + policy at provider and consumer.
- Cucumber feature `it/src/test/resources/it/dataspace_eventing.feature` + step definitions:
  1. an order grants the consumer a scoped TIL entry at the provider, and `registration.issuer`
     holds an unscoped one for the same DID;
  2. the operator deletes the consumer from the trust anchor's TIR;
  3. within the watch interval the unscoped entry is gone at the provider and the order-scoped one
     is still there (default `unscoped` mode);
  4. a participant on `keep` is untouched;
  5. with the push path blocked, the reconciliation sweep reaches the same state.
- Documentation: `doc/EVENTING.md` (or a section under `doc/deployment-integration/`), linked from
  `doc/README.md`; operator and provider/consumer role guides; `doc/release-notes/10-x.md`.
- ADRs in `doc/adr/`, cut from the decision register once ratified — candidates:
  *Carry data space events over OID4VP-authenticated CloudEvents*,
  *Detect participant removal by diffing the registry, not by instrumenting it*,
  *Authorize events per publisher DID and type*,
  *Make the reaction to a removal configurable along the grant scope*.
  Update `doc/adr/README.md`.

---

## Definition of done

- Every sub-ticket's acceptance criteria met, with unit tests parameterized where the matrix invites
  it, and the integration test green in `mvn -f pom.xml verify`.
- `helm lint`, `helm unittest`, and `values-schema.sh --check` pass; the README values table is
  regenerated.
- An upgrade of an existing release with no new values changes nothing (verified by the
  helm-unittest suite).
- Every public method and class documented (GoDoc/JavaDoc conventions per repo), no magic constants.
- Documentation and release notes merged; ADRs written and indexed.
- The feature is demonstrable from the local deployment (`mvn clean deploy -Plocal`) with a scripted
  removal at the trust anchor.

## Rollout and compatibility

- Entirely additive and off by default; no existing values change meaning.
- The two TIL additions are additive; CM-5 requires a trusted-issuers-list new enough to have them
  and must fail the mode that needs them loudly at start-up, not silently at event time
  (`TrustedIssuersListAdapter` already documents a comparable minimum-version requirement).
- Old participants (no listener) and old trust anchors (no publisher) interoperate with new ones;
  the only cost is that nothing is announced or received.

## Risks

| Risk | Handling |
|---|---|
| A registry glitch deletes trust configuration data-space-wide | the §8 safeguards; the default mode keeps contractual grants; the report makes it visible |
| A participant spoofs a removal | per-publisher per-type authorization (CM-1); token verified against the verifier's JWKS |
| contract-management grows a second, unrelated responsibility | kept behind modes and conditions as the existing central-marketplace mode is; Q1 revisits the component split |
| The epic stalls between repositories | CM-1 + CM-4 + CM-5 + DSC-2 with `mode: none` already deliver visibility; the cleanup modes can follow the TIL release |
