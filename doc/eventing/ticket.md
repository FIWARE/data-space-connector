# Ticket: Data space eventing and participant-removal cleanup

**Type:** Epic (12 sub-tickets across 4 repositories; the contract-management helm chart is a
fifth, touched from DSC-1)
**Design:** [architecture.md](architecture.md) — **the decisions behind it, Q1–Q13 and F1–F8: [questions.md](questions.md)**
**Status:** refined, unblocked — design decisions taken in the reviews of 2026-10-28 and
2026-10-29; no open question is blocking

## Summary

Give a data space a generic, OID4VP-authenticated event channel between its entities, and use it for
the first concrete case: when the operator removes a participant from a trust anchor's Trusted
Issuers Registry, every participant learns about it and — if it has configured that — cleans up the
local trust that existed because of that membership.

## Motivation

Removing a participant from the TIR is invisible to everybody else. Each participant keeps TIL
entries for the removed identifier — written on order activation, by `registration.issuer` at deploy
time, or by hand — and nothing ever removes them. Where a service's credential configuration omits
`trustedParticipantsLists`, VCVerifier trusts the local list alone
(`VCVerifier/verifier/trustedparticipant.go`), so those entries are not stale decoration but a live
grant to an organization the data space has expelled. Everywhere else they are silent drift that no
operator can see.

There is also no channel at all for one entity of a data space to tell another that something
happened, so every future need of that kind would grow its own callback. This epic builds the
channel once, on CloudEvents and on the authentication the connector already uses, and consumes it
for one use case.

## Decisions taken in review (2026-10-28, 2026-10-29)

| | Decision |
|---|---|
| Component | Both ends in `contract-management` (D-1). It may have to be **renamed** later — out of scope here |
| Wire format | CloudEvents HTTP Protocol Binding, structured mode; webhook handshake from the CloudEvents HTTP Web Hooks document; subscription request shaped after the CloudEvents Subscriptions API (D-9) |
| `dataspaceid` | optional; `source` identifies the data space (D-2) |
| Delivery | best-effort, stateless, leased subscriptions — no proof of delivery required (D-4) |
| Signatures | none (D-7) |
| Callback trust | CloudEvents webhook handshake, **never** derived from the identifier: `did:web` is not guaranteed (D-8) |
| Default reaction | `report` — local entries are left untouched unless cleanup is explicitly configured (D-6) |
| Multi-data-space | supported: attribution through the TIL scope, every reaction and sweep is per publisher (D-10) |
| Publishers | runtime-refreshable configuration, not start-up only |
| Trust anchor deployment | separate release in the trust anchor's namespace, reusing the consent-authority identity — the `trust-anchor` chart gains no dependency |
| Credential type | per-participant configuration, not a data space-wide parameter |
| PAP / agreements | reporting is enough for now |
| Scope in the TIL | stays internal; may be exposed on the **management** API only |
| Latency | ~2 min (poll interval × confirmations) is acceptable |
| Scope attribution | a **typed** grant in the TIL — `scope=<publisher id>` + `scopeType=dataspace` — added backwards compatibly; no reserved prefix (D-11) |
| Configuration namespace | **`dataspace.events`**, not `contract-management.events`; the component keeps its name for now (D-12) |
| Onboarding portal | **in scope for this epic**: it both writes and removes membership trust, so it has to do both with the scope (OBP-1) |
| `full` mode | kept |
| `adoptUnattributed` | kept as designed: adopts for a single-publisher deployment, refuses for several |
| Publisher origin | the receiver configures the publisher's DNS name beside its identifier; two pieces of identity are acceptable |
| Publisher management API | out of scope — a later epic |
| Specification | an **interoperability write-up in this repository** (DSC-3). Not a submission: DSBA, Gaia-X and IDSA are unaffected by this change |

## Scope

**In scope**

- A CloudEvents-based event API, an OID4VP-authenticated subscription API with the webhook
  handshake, and per-publisher, per-type authorization at the receiver.
- The `org.fiware.dataspace.participant.removed` event, derived at the trust anchor by watching the
  TIR.
- Attribution of membership-derived TIL entries to the data space that caused them, and a
  configurable, per-publisher reaction that defaults to reporting.
- A per-publisher reconciliation sweep as the backstop.
- Chart wiring, an example ODRL policy, an end-to-end integration test, documentation, ADRs, and the
  specification write-up.

**Out of scope**

- Participant-to-participant configuration delivery after a purchase (architecture §14).
- Durable/auditable delivery (outbox, event log, catch-up feed) — D-4.
- Cleaning up ODRL policies, Rainbow/EDC agreements or issued credentials on removal.
- Renaming `contract-management` — the event configuration is namespaced `dataspace.events` so that
  the rename, whenever it comes, costs nothing here.
- Self-service registration of publishers by a participant (needs persistence, D-4) — a later epic.
- Any change to how an operator removes a participant.

---

## CM-1 — Event envelope, receiving endpoint, authorization, refreshable publishers

**Repo:** `FIWARE/contract-management`

- `api/dataspace-events.yaml` (OpenAPI-first, generated like `contract-management.yaml`):
  `POST /events` taking a CloudEvents 1.0 envelope in structured content mode
  (`application/cloudevents+json`), `dataspaceid` optional.
- `OPTIONS /events` answering the webhook validation request: return `WebHook-Allowed-Origin` for a
  configured publisher's origin, nothing for anybody else, plus `Allow: POST`.
- `EventListener` controller behind `dataspace.events.listener.enabled`, dispatching to a
  `DataSpaceEventHandler` interface — one implementation per event type, mirroring the
  `TMForumEventHandler` registry in `NotificationListener`.
- Authorization (architecture §6): verify the bearer against the configured verifier JWKS, read the
  holder identifier, require `source == authenticated identifier`, require the `(publisher, type)`
  pair to be configured, ignore events whose `subject` is this participant's own identifier.
- Deduplicate on `(source, id)` with a bounded, time-limited in-memory cache.
- Unknown `type` → `202` + metric, never an error.
- Configuration: **`dataspace.events.*`** (D-12 — the namespace is the data space's, not the
  component's) as a **`@Refreshable`** `@ConfigurationProperties` bean, picked up from
  the mounted ConfigMap through a file watch and through `/refresh`, so a publisher can be added
  without restarting the pod. Conditions in the style of
  `GeneralProperties.CentralMarketplaceCondition`.

**Acceptance**

- A correctly authenticated, configured publisher gets `202`; an unknown type gets `202` and is
  counted; a known type from an unconfigured publisher gets `403`; a mismatched `source` gets `403`;
  a replay gets `202` and acts once; an event about oneself is recorded and not acted on.
- `OPTIONS /events` grants only configured origins.
- A publisher added to the ConfigMap is accepted without a restart (verified against the refresh
  path, not only by unit test).
- Parameterized unit tests over the authorization matrix (JUnit 5 `@ParameterizedTest`).

## CM-2 — Publisher: subscriptions, handshake and delivery

**Repo:** `FIWARE/contract-management`

- `POST /subscriptions`, `DELETE /subscriptions/{id}` behind `dataspace.events.publisher.enabled`:
  OID4VP-authenticated, `subscriberId` must equal the authenticated identifier, request shaped after
  the CloudEvents Subscriptions API (`sink`, `types`, `protocol`, `protocolsettings`), leased with a
  TTL, capped per subscriber and in total.
- **Webhook handshake before activation:** `OPTIONS <sink>` with `WebHook-Request-Origin: <own DNS
  name>`; activate only on a matching `WebHook-Allowed-Origin`; repeat the origin header on every
  delivery; honour `WebHook-Allowed-Rate` when returned. No host is ever derived from the
  subscriber's identifier.
- Delivery: for each matching subscription, `POST` the event following
  `ContractManagementAdapter`'s pattern — `Oid4VpAuthHandler.executeWithAuth` with the
  subscription's `clientId`/`scopes` — with exponential backoff and a bounded attempt count, then
  drop with an error log and a metric.
- Internal `POST /events` (publisher side, not published through the ingress) for the onboarding
  portal or an operator script.

**Acceptance**

- A subscription whose `subscriberId` does not match the authenticated identifier is rejected; a
  sink that does not answer the handshake, or answers with a non-matching origin, is never
  activated and the subscription is refused; an expired lease stops delivery; a `401` from a
  subscriber triggers one OID4VP token fetch and a retry; a permanently failing subscriber does not
  block delivery to the others; the granted rate is not exceeded.

## CM-3 — Subscriber: registration and renewal

**Repo:** `FIWARE/contract-management`

- A `DataSpaceEventSubscriber` modelled on `NotificationSubscriber`: register at every configured
  publisher on `ServerStartupEvent` **and whenever the refreshed configuration adds one**, retry on
  failure, renew before the lease expires, report through a health indicator like
  `SubscriptionHealthIndicator`.

**Acceptance**

- A publisher unavailable at start-up is retried and the health indicator stays unhealthy until
  registration succeeds; a lease is renewed before it expires; `409` on an existing subscription is
  treated as success (as the TMForum subscriber already does); a publisher added at runtime is
  subscribed to without a restart.

## CM-4 — Trust anchor watcher: deriving `participant.removed`

**Repo:** `FIWARE/contract-management` — runs in the publisher-mode instance at the trust anchor

- A scheduled `TirWatcher` (own `micronaut.executors` entry, as `notification-subscriber` has)
  enumerating the configured registry and diffing against the previous snapshot (architecture §8).
- Safeguards, all configurable, all named constants: snapshot only on a complete enumeration,
  `confirmations` consecutive cycles before announcing, `maxRemovalsPerCycle` circuit breaker that
  emits nothing and raises an alert, silent first snapshot.
- Emits through CM-2.

**Acceptance**

- Parameterized tests for: a clean removal; a registry error mid-enumeration (nothing emitted,
  snapshot unchanged); an empty registry (nothing emitted, alert); an identifier that reappears
  before the confirmation count (nothing emitted); a restart (nothing emitted on the first cycle);
  more than the cap disappearing (nothing emitted, alert).

## CM-5 — Reacting: attribution, cleanup, reconciliation and the report

**Repo:** `FIWARE/contract-management`

- `ParticipantRemovedHandler` applying the publisher's `onParticipantRemoved.mode`
  (`report` | `dataspace` | `granted` | `full`) with `keep` and per-identifier `overrides`, through
  a `TrustedIssuersListAdapter` extended with the calls it needs (requires TIL-1 and TIL-2;
  `adoptUnattributed` requires TIL-3).
- Membership grants are written as `scope=<publisher id>`, `scopeType=dataspace` wherever
  `contract-management` creates them, so that what it creates is attributable like what it cleans
  up. Order grants keep writing `scope` alone, unchanged.
- The per-publisher reconciliation sweep (architecture §10): at start-up and on a timer, for each
  publisher, the identifiers holding credentials granted under that publisher's membership scope
  which the publisher's registry no longer lists; same policy, capped by `maxRemovalsPerRun`.
- Reporting in every mode: log, `dsc_dataspace_events_participant_removed_total{publisher,action,mode}`,
  and `GET /events/participants/removed` listing what each data space removed, what is still held
  for it, and whether it is attributable.

**Acceptance**

- Parameterized tests over mode × origin of the entry: `report` changes nothing; `dataspace` removes
  only that publisher's membership grants; `granted` additionally removes order-scoped grants;
  `full` deletes the issuer; unattributed entries survive every mode but `full`; an override beats
  the global mode; an identifier on `keep` is never touched in either path.
- **Multi-membership test:** an identifier attributed to two publishers, removed by one, keeps the
  other's entries in every mode but `full`.
- `adoptUnattributed: true` with one publisher adopts; with two publishers it refuses and says so.
- A failing TIL call yields `502` to the publisher (so it retries) and leaves nothing half-applied.

## TIL-1 — Typed scopes

**Repo:** `FIWARE/trusted-issuers-list`

A grant gains a type, so that "this credential exists because the issuer is a member of data space
X" is expressible without reserving a prefix in a free-form field (D-11).

- `Credential.scopeType`, nullable, beside the existing `scope`; a new Liquibase changeset
  (`changelog-v0_0_7.xml`) adding the nullable column — no data migration.
- An optional `scopeType` query parameter on the three scope endpoints
  (`PUT`/`DELETE /issuer/{did}/credential`, and `POST /issuer`), carried into
  `CredentialRepository` as `findByTrustedIssuerDidAndScopeAndScopeType` /
  `deleteByTrustedIssuerDidAndScopeAndScopeType`.
- **Absent `scopeType` means the untyped grants**, never "any type". Every existing caller writes
  and deletes ProductOrder scopes without it and must keep addressing exactly the rows it addresses
  today.
- `scopeType` is as internal as `scope`: it appears in no `/v4` or `/v5` projection
  (`TIRMapper` untouched).

**Acceptance:** an existing unscoped or untyped-scope row is unaffected by the migration; a typed
grant and an untyped grant with the same `scope` string coexist and are deleted independently; a
scoped call without `scopeType` never touches a typed grant and vice versa; nothing new appears in
a `/v4` or `/v5` response. Parameterized over (scope, scopeType) combinations in
`TrustedIssuersListControllerTest`.

## TIL-2 — Scope queries on the management API

**Repo:** `FIWARE/trusted-issuers-list`

Two read endpoints on the **management** API; the scope stays out of every TIR projection:

- `GET /issuer/{did}/scopes` → the scopes currently holding credentials for this issuer, each with
  its `scopeType` and a count. Needed by the `granted` mode and by the report.
- `GET /issuer?scope=<scope>&scopeType=<type>` → the issuers holding credentials granted under that
  scope. Needed by the reconciliation sweep.

**Acceptance:** exactly the distinct (scope, scopeType) pairs; empty for an issuer with only
unattributed credentials; `404` for an unknown issuer; the scope appears in no `/v4` or `/v5`
response. Covered in `TrustedIssuersListControllerTest`.

## TIL-3 — Delete the unattributed credentials of an issuer *(migration path only)*

**Repo:** `FIWARE/trusted-issuers-list`

`DELETE /issuer/{did}/credential` requires `scope` today and answers `400` without it
(`TrustedIssuersListController.requireScope`), so the credentials that no grant owns cannot be
removed selectively. Needed **only** for `adoptUnattributed`, so it may follow the rest of the
epic. Spell it explicitly rather than by
omitting the parameter, so that a forgotten parameter stays an error instead of becoming a
destructive success.

**Acceptance:** removes only credentials with no scope, leaves every scoped grant, keeps the issuer,
`404` for an unknown issuer, idempotent; a request with neither the explicit marker nor a scope is
still `400`.

## OBP-1 — Onboarding portal: write and remove membership trust with the scope

**Repo:** [`SEAMWARE/On-Boarding-Portal`](https://github.com/SEAMWARE/On-Boarding-Portal)

The portal is the other writer of membership-derived trust, and today it is also the bluntest
remover. `registerIssuer` (`backend/src/service/registry-client.ts`) posts to `<til>/issuer` with no
scope, so everything it creates is unattributed and invisible to the mechanism; `unregisterIssuer`
calls `DELETE <til>/issuer/{did}`, which drops the issuer whole — including order-granted entries
and other data spaces'.

- `TilConfig` gains the data space identifier (the publisher id) for each configured TIL.
- `registerIssuer` posts with `?scope=<publisher id>&scopeType=dataspace` (TIL-1), so what the
  portal creates is attributable from the moment it is created.
- `unregisterIssuer` calls `DELETE <til>/issuer/{did}/credential?scope=<publisher id>&scopeType=dataspace`
  instead of deleting the issuer, and deletes the issuer only when no credential is left.
- Optional, and the reason the portal is worth touching here beyond attribution: when it removes a
  participant it already *knows* — it can `POST` the removal to the publisher's internal `/events`
  (CM-2), which turns the TIR watcher into a backstop rather than the only detection, without
  changing anything about how the event is authorized downstream.
- Both the trust anchor's registry and any participant TIL in `app.til` are reached by the same
  code, so the change lands once.

**Acceptance:** an issuer registered through the portal carries the membership scope in the TIL;
removing a participant through the portal leaves order-granted and differently-scoped credentials
in place; a TIL that does not yet implement TIL-1 still works (the portal falls back to the
unscoped calls and logs it); the direct event publication is configurable and off by default.

## DSC-1 — Chart: participant side

**Repo:** this one

- `contract-management.dataspace.events.*` in `charts/data-space-connector/values.yaml` with
  `# --` comments for helm-docs, every key defaulting to off and the reaction to `report`;
  regenerate `values.schema.json` (`.github/scripts/values-schema.sh`) and the README values table.
  The namespace under the subchart alias is `dataspace.events`, not `events`, so that a later
  rename of the component leaves the feature's configuration surface untouched (D-12).
- The publishers block must be mounted as a ConfigMap the application can refresh (CM-1), not baked
  into the container's environment.
- A `scope`/`scopeType` on `registration.issuer` entries so membership-derived trust is
  attributable from the first install, defaulting to the data space's publisher id and
  `dataspace` where a publisher is configured.
- Events path on the contract-management ingress (and, if the subchart does not support it,
  `FIWARE/helm-charts` first).
- Example ODRL policy `it/src/test/resources/policies/allowDataSpaceEvents.json`.
- helm-unittest suite `charts/data-space-connector/tests/eventing_test.yaml`: nothing rendered when
  disabled, the full block rendered when enabled, `env`/`additionalEnvVars` not clobbered.

## DSC-2 — Chart: trust anchor side, demo and test

**Repo:** this one

- The publisher as a **separate release** in the trust anchor's namespace, documented along the
  `consent-authority` pattern and reusing that identity; the `trust-anchor` chart itself is not
  touched beyond documentation.
- `k3s/` overlays: publisher at the trust anchor, listener + policy at provider and consumer.
- Cucumber feature `it/src/test/resources/it/dataspace_eventing.feature` + step definitions:
  1. an order grants the consumer an order-scoped TIL entry at the provider, and the provider's
     `registration.issuer` holds a `dataspace`-typed one for the same identifier;
  2. the operator deletes the consumer from the trust anchor's TIR;
  3. with the default `report` mode, **nothing is removed** and the report lists the consumer;
  4. with `dataspace` mode, the membership entry is gone and the order-scoped one remains;
  5. an identifier on `keep` is untouched;
  6. with the push path blocked, the reconciliation sweep reaches the same state;
  7. an identifier attributed to a second (simulated) publisher keeps that publisher's entry.
- Documentation: `doc/EVENTING.md` (or a section under `doc/deployment-integration/`), linked from
  `doc/README.md`; operator and provider/consumer role guides; `doc/release-notes/10-x.md`.
- ADRs in `doc/adr/`, cut from the decision register — candidates:
  *Carry data space events as CloudEvents authenticated over OID4VP*,
  *Detect participant removal by diffing the registry, not by instrumenting it*,
  *Authorize events per publisher and type*,
  *Attribute local trust to the data space that caused it*,
  *Confirm event callbacks with the CloudEvents webhook handshake*.
  Update `doc/adr/README.md`.

## DSC-3 — Interoperability specification

**Repo:** this one — `doc/eventing/specification.md`

Not a submission to a governance body: DSBA, Gaia-X and IDSA are unaffected by this change, as the
review pointed out. What is written up is the **interoperability contract** — what a second,
non-FIWARE implementation needs in order to publish to, or subscribe at, a FIWARE participant,
separated from the architecture document's rationale and from this repository's chart values:

- the event types and their payloads, with `org.fiware.dataspace.participant.removed` as the first;
- the authentication binding: an OID4VP access token obtained at the **receiver's** verifier, and
  what a publisher has to present;
- the authorization rule of architecture §6 (publisher × type, `source` equals the authenticated
  identifier, self-events ignored);
- the subscription profile: which parts of the CloudEvents Subscriptions API shape are used, and the
  webhook handshake with the exact headers (pinned, because that document is `1.0.3-wip`);
- what is explicitly *not* specified: the reaction to an event, which is local policy.

D-9 keeps the wire format on existing CloudEvents documents, so this stays short. No ODRL left
operand for the credential issuer exists today; note it as a possible PAP contribution rather than a
dependency. If a governance body ever asks for it, this is the document that goes.

---

## Definition of done

- Every sub-ticket's acceptance criteria met, with unit tests parameterized where the matrix invites
  it, and the integration test green in `mvn -f pom.xml verify`.
- `helm lint`, `helm unittest`, and `values-schema.sh --check` pass; the README values table is
  regenerated.
- An upgrade of an existing release with no new values changes nothing — and enabling the feature
  with defaults still changes no trust configuration (verified by the helm-unittest suite and by
  step 3 of the feature).
- Every public method and class documented (JavaDoc), no magic constants.
- Documentation and release notes merged; ADRs written and indexed; `doc/eventing/specification.md`
  written and linked from `doc/README.md`.
- The feature is demonstrable from the local deployment (`mvn clean deploy -Plocal`) with a scripted
  removal at the trust anchor.

## Rollout and compatibility

- Entirely additive and off by default; no existing values change meaning; the default reaction is
  `report`, so switching the feature on never deletes anything by itself.
- The TIL additions are additive: a nullable column and optional query parameters, with absent
  `scopeType` keeping its present meaning for every caller that exists today. CM-5 and OBP-1 must
  fail loudly — at start-up for CM-5 — when a configured mode needs a trusted-issuers-list that is
  too old, not silently at event time (`TrustedIssuersListAdapter` already documents a comparable
  minimum-version requirement).
- Entries that predate the feature are unattributed and therefore outside the mechanism; they are
  reported, and `adoptUnattributed` is the opt-in migration for a single-data-space deployment.
- Old participants (no listener) and old trust anchors (no publisher) interoperate with new ones;
  the only cost is that nothing is announced or received.

## Risks

| Risk | Handling |
|---|---|
| A registry glitch deletes trust configuration data-space-wide | the §8 safeguards; the `report` default; the report makes it visible |
| A participant spoofs a removal | per-publisher per-type authorization (CM-1); token verified against the verifier's JWKS |
| A removal in one data space destroys trust from another | attribution per publisher (D-10); `full` is documented as the blunt instrument it is |
| The installed base stays unattributed, so the sweep sees nothing | the report lists unattributed entries explicitly; `registration.issuer` carries a scope going forward; `adoptUnattributed` for the simple case |
| contract-management grows a second, unrelated responsibility | kept behind modes and conditions as the central-marketplace mode is; a rename is acknowledged as a likely follow-up |
| The portal keeps deleting whole issuers, undoing the attribution from the other side | OBP-1 is part of the epic, and the integration test covers the portal path once it lands |
| The epic stalls between repositories | CM-1 + CM-4 + CM-5 in `report` mode + DSC-2 already deliver visibility; the cleanup modes can follow the TIL release |
