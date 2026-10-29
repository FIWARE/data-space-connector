# Data Space Eventing

**Status:** Draft — the decisions of the design review of 2026-10-28 are folded in
**Date:** 2026-10-28
**Scope:** a generic, OID4VP-authenticated event mechanism between the entities of a data space, and
one concrete event — *a participant was removed from the data space* — that lets every participant
clean up its local Trusted Issuers List.

<!-- ToC created with: https://github.com/thlorenz/doctoc -->

- [1. Problem](#1-problem)
- [2. Goals and non-goals](#2-goals-and-non-goals)
- [3. Overview](#3-overview)
- [4. The event model](#4-the-event-model)
- [5. The APIs](#5-the-apis)
- [6. Trust and authorization](#6-trust-and-authorization)
- [7. Delivery semantics](#7-delivery-semantics)
- [8. Detecting the removal at the trust anchor](#8-detecting-the-removal-at-the-trust-anchor)
- [9. Attributing local trust, and reacting to a removal](#9-attributing-local-trust-and-reacting-to-a-removal)
- [10. Reconciliation: the backstop](#10-reconciliation-the-backstop)
- [11. Deployment](#11-deployment)
- [12. Decision register](#12-decision-register)
- [13. Failure modes and security considerations](#13-failure-modes-and-security-considerations)
- [14. What this opens up](#14-what-this-opens-up)
- [15. Impact per repository](#15-impact-per-repository)

## 1. Problem

Trust in a FIWARE data space is evaluated against two lists, per credential type, by the
participant's VCVerifier (see `k3s/provider.yaml`, `oidcScopes.*.credentials[]`):

| List | Who owns it | Configured as |
|---|---|---|
| Trusted Issuers Registry (TIR) — a data space's participant list | the trust anchor / operator | `trustedParticipantsLists` |
| Trusted Issuers List (TIL) — who *this* participant accepts credentials from | the participant | `trustedIssuersLists` |

The verifier runs every configured validation service and requires all of them to pass
(`VCVerifier/verifier/verifier.go`), so an issuer normally has to be in the global TIR **and** in
the local TIL. There is one important exception, implemented in
`VCVerifier/verifier/trustedparticipant.go`: when a credential configuration names **no**
participants list, the participant check returns `true` for everybody and the local TIL alone
decides. (A third case exists — for a trusted Gaia-X credential chain the verifier skips both
registry checks — but there neither list is consulted, so it is not affected by any of this.)

When the operator removes a participant from the TIR (`DELETE /issuer/{did}` on the trust anchor's
trusted-issuers-list), nothing tells the other participants. The consequences are:

1. **Dangling entries.** Every participant that ever traded with the removed organization keeps TIL
   entries for its identifier — written by `contract-management` on order activation
   (`TrustedIssuersListAdapter.allowIssuer`, scoped to the ProductOrder id), by the chart's
   `registration.issuer` values, or by hand. Nothing ever removes them.
2. **A latent, configuration-dependent hole.** For every service whose credential configuration
   omits `trustedParticipantsLists`, those dangling entries are not stale decoration — they are a
   live grant. The organization stays able to issue credentials that open that service.
3. **No visibility.** Even where the entries are inert, an operator has no way to see that its local
   trust configuration refers to organizations the data space no longer knows.

The same gap exists for other facts: today there is **no channel at all** through which one entity of
a data space can tell another that something happened. Each new need (a provider handing a consumer
the configuration of a service it just bought, a trust anchor announcing a new participant, a
revocation notice) would otherwise grow its own bespoke callback.

> **Participant identifiers are not necessarily `did:web`.** Nothing in the connector constrains the
> DID method a participant uses, so no part of this design may derive a host, a key or a document
> location from the identifier. This rules out two otherwise attractive shortcuts — discovering a
> participant's event endpoint from its DID document, and validating a callback URL against the
> participant's domain (§5.2, D-8).

## 2. Goals and non-goals

**Goals**

- A **generic** event mechanism between data space entities — trust anchor to participants first,
  participant to participant on the same mechanism later.
- Events are **authenticated with the mechanisms the connector already uses**: the sender presents a
  verifiable credential over OID4VP, the receiver authorizes it through its PEP exactly like any
  other cross-participant call.
- Events are **authorized per event type**: being a participant does not make you entitled to
  announce that another participant was removed.
- A participant **configures what an event does to its local state** — by default, nothing but a
  report. Removal from a data space does not necessarily end a bilateral relationship, and a
  participant may be a member of several data spaces at once.
- Standard formats where they exist (§5, D-9), so that the channel is not FIWARE-only.
- Nothing new in the trust model: no new root of trust, no new key material, no shared secret, no
  message broker to operate.

**Non-goals (explicitly out of scope here)**

- Participant-to-participant configuration delivery after a purchase. The mechanism is designed so
  it can carry that later (§14); it is not implemented in this ticket.
- Cleaning up anything other than the TIL — ODRL policies at the PAP, Rainbow/EDC agreements,
  identityhub credentials — on a removal event (§9, "what is deliberately not touched").
- Durable, auditable delivery for every event type (§7, D-4).
- Changing how the operator removes a participant. The TIR management API stays as it is.

## 3. Overview

```mermaid
flowchart LR
    subgraph TA["Trust Anchor (operator)"]
        TIR[("trusted-issuers-list<br/>TIR")]
        PUB["contract-management<br/><i>publisher mode</i>"]
        PUB -. "1. poll + diff<br/>GET /issuer" .-> TIR
    end

    subgraph P["Participant (provider / consumer)"]
        PEP["APISIX (PEP)"]
        VER["VCVerifier"]
        CM["contract-management<br/><i>listener mode</i>"]
        TIL[("trusted-issuers-list<br/>TIL")]
        PEP --> CM
        PEP -. "4. validate token" .-> VER
        CM -- "5. cleanup<br/>DELETE /issuer/{did}/credential?scope=..." --> TIL
    end

    CM -. "0. subscribe (OID4VP) + webhook handshake" .-> PUB
    PUB -- "2. OID4VP: get access token" --> VER
    PUB -- "3. POST /events (CloudEvent, Bearer)" --> PEP
    CM -. "6. reconcile: GET /v4/issuers" .-> TIR
```

0. On start-up, and periodically afterwards, the participant's `contract-management` **subscribes**
   at the configured publishers, authenticating over OID4VP; the publisher confirms the callback
   with the CloudEvents webhook handshake (§5.2). This mirrors what `contract-management` already
   does for TMForum notifications (`NotificationSubscriber`: callback registration with retry and a
   health indicator).
1. The publisher at the trust anchor — a `contract-management` instance deployed there in publisher
   mode (§11) — **watches the TIR** and derives `participant.removed` events from it.
2. For each subscriber, it obtains an access token at the **subscriber's** verifier by presenting
   the trust anchor's credential — the `Oid4VpAuthHandler` / `OID4VPClient` path that
   `ContractManagementAdapter` already uses to notify a provider about an order.
3. It `POST`s the event through the subscriber's PEP.
4. The PEP validates the token and applies the ODRL policy that restricts the events path to holders
   of the agreed credential type — the same shape as
   `it/src/test/resources/policies/allowContractManagement.json`.
5. The receiver authorizes the *event type* against the authenticated publisher identifier,
   deduplicates, and applies the configured reaction to the TIL entries **attributable to that data
   space** (§9).
6. Independently of all of the above, the receiver periodically **reconciles** those same entries
   against that data space's TIR. This is the backstop that makes the mechanism safe to build
   without durable delivery (§7, §10).

Both ends are the same component — `contract-management` — in two modes (D-1).

## 4. The event model

Events are **CloudEvents 1.0**, exchanged over the
[CloudEvents HTTP Protocol Binding](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/http-protocol-binding.md)
in structured content mode, media type `application/cloudevents+json` (D-9).

```json
{
  "specversion": "1.0",
  "id": "urn:uuid:0f3b9a2c-1f3c-4a0e-9a2d-2d1b4f0a77c1",
  "type": "org.fiware.dataspace.participant.removed",
  "source": "did:web:trust-anchor.org",
  "subject": "did:elsi:VATDE-1234567",
  "time": "2026-10-28T09:12:44Z",
  "datacontenttype": "application/json",
  "data": {
    "participantId": "did:elsi:VATDE-1234567",
    "observedAt": "2026-10-28T09:12:40Z",
    "registry": "https://tir.trust-anchor.org"
  }
}
```

| Attribute | Meaning |
|---|---|
| `id` | unique per event, the **deduplication key** together with `source` |
| `type` | reverse-DNS, `org.fiware.dataspace.<entity>.<past-tense-verb>`; a receiver ignores types it does not know |
| `source` | the identifier of the publishing entity. Must equal the identity authenticated at the transport layer (§6), and it is what identifies the data space to the receiver |
| `subject` | the entity the event is about — for participant events, its identifier, in whatever DID method it uses |
| `data` | type-specific payload, versioned with the `type` — a breaking change is a new `type` |
| `dataspaceid` | **optional** CloudEvents extension. Most data spaces have no identifier of their own; where one exists it may be carried, but no receiver behaviour depends on it. The publisher's `source` is the authoritative discriminator |

**Event types defined here**

| Type | Payload | Emitted when |
|---|---|---|
| `org.fiware.dataspace.participant.removed` | `participantId`, `observedAt`, `registry` | an identifier that was in the TIR is gone from it |

There is deliberately **no `reason`**: the watcher of §8 cannot know one, nothing in the receiver
branches on it, and an attribute that is advisory in practice but looks authoritative invites
exactly the per-reason defaults the review rejected. A later, better-informed source of removals can
introduce its own type.

**Types reserved for later** (named here so the namespace stays coherent, not implemented):
`participant.registered`, `participant.credentials.changed`, `participant.suspended`,
`service.configuration.provided` (§14).

## 5. The APIs

Two small APIs added to `contract-management` and specified in its `api/` folder as the existing
`contract-management.yaml` is (OpenAPI-first, code generated).

### 5.1 Receiving — `POST /events`

```
POST /events
Authorization: Bearer <access token from the receiver's verifier>
Content-Type: application/cloudevents+json
WebHook-Request-Origin: <sender's DNS name>          (see 5.2)

202 Accepted      the event was accepted (and handled, or queued for handling)
400 Bad Request   malformed envelope
401 Unauthorized  no/invalid token — the PEP answers this, and it triggers the sender's OID4VP flow
403 Forbidden     authenticated, but this publisher may not emit this type (§6)
422 Unprocessable the type is known but the payload is invalid
502 Bad Gateway   a downstream (the TIL) failed — the publisher should retry
```

- **Idempotent.** `(source, id)` is remembered for a configurable window (default 24h, in memory);
  a repeat returns `202` without acting again. The handlers converge anyway, so a lost dedup entry
  after a restart is harmless.
- **Unknown types are accepted and ignored** with a log line and a metric, never rejected. A data
  space must be able to introduce a type without a flag day.
- `OPTIONS /events` implements the webhook validation response of §5.2.
- The path is published through the participant's ingress/PEP, like `/order` is today.

### 5.2 Subscribing — `POST /subscriptions`

Offered by a publisher. The subscriber calls it authenticated over OID4VP. The request shape follows
the CloudEvents [Subscriptions API](https://github.com/cloudevents/spec/blob/main/subscriptions/spec.md)
(`sink`, `types`, `protocol`, `protocolsettings`) without claiming conformance to it:

```
POST /subscriptions
Authorization: Bearer <access token from the publisher's verifier>
{
  "subscriberId": "did:web:mp-operations.org",
  "sink": "https://contract-management.mp-operations.org/events",
  "types": ["org.fiware.dataspace.participant.removed"],
  "protocol": "HTTP",
  "protocolsettings": { "clientId": "contract-management", "scopes": ["dataspace-events"] }
}

201 Created  { "id": "...", "expiresAt": "2026-10-28T10:12:44Z" }
```

- `subscriberId` **must** equal the identity authenticated in the request; the publisher does not
  take the caller's word for it.
- **The `sink` is confirmed with the CloudEvents webhook handshake, not derived from the
  identifier.** Before the subscription becomes active the publisher sends
  `OPTIONS <sink>` with `WebHook-Request-Origin: <its own DNS name>`, and activates it only if the
  response carries a matching `WebHook-Allowed-Origin`; every delivery then repeats the origin
  header. An endpoint that did not agree to receive from this publisher does not answer the
  handshake, so the subscription API cannot be turned into an amplifier that makes the trust anchor
  POST authenticated traffic at an arbitrary host. This replaces the `did:web` host matching of the
  first draft, which assumed a DID method the connector does not mandate (D-8).
  The handshake is specified in
  [HTTP 1.1 Web Hooks for Event Delivery](https://github.com/cloudevents/spec/blob/main/cloudevents/http-webhook.md)
  (1.0.3-wip at the time of writing — the headers are stable and widely implemented, but the
  document is not a released version, which is why §12 D-9 adopts it by reference and pins the
  behaviour here).
- `protocolsettings.clientId`/`scopes` are what the publisher must use when it fetches an access
  token at the subscriber's verifier.
- Subscriptions are **leased** (default TTL 15 min) and held in memory. Subscribers renew them on a
  timer; a publisher restart loses them and the subscribers re-register. `DELETE /subscriptions/{id}`
  ends one early (D-4).
- A publisher caps the number of subscriptions per subscriber (default 5) and in total, and honours
  `WebHook-Allowed-Rate` when the sink returns one.

### 5.3 What is deliberately *not* added

- **No catch-up feed** (`GET /events?since=`). It would need a durable, retained event log at the
  publisher, and it answers a question — "which participants are gone?" — that the TIR already
  answers directly and authoritatively. §10 uses the TIR instead.
- **No broker, no queue.** Nothing in the connector operates one today.

## 6. Trust and authorization

The sender authenticates with exactly the mechanism used for the central-marketplace notification
(`doc/CENTRAL_MARKETPLACE.md`, step 4): it presents a verifiable credential it holds, over OID4VP,
at the **receiver's** verifier and uses the returned access token as a bearer.

**At the receiver, three checks apply, in this order:**

1. **PEP / ODRL policy (transport).** A policy restricts the events path to holders of an agreed
   credential type, e.g. a `TrustAnchorCredential` issued by the operator. The type is **not fixed
   by the connector and not a data space-wide parameter**: each participant configures which
   credential it admits to its own events path, as it already does for the `MarketplaceCredential`
   of the central marketplace.
2. **Publisher authorization (application).** `contract-management` resolves the authenticated
   holder identifier from the forwarded token and checks it against its configured publishers:

   ```yaml
   publishers:
     - id: did:web:trust-anchor.org
       origin: trust-anchor.org                   # answered on OPTIONS /events, §5.2
       types: [ "org.fiware.dataspace.participant.removed" ]
       registry: https://tir.trust-anchor.org     # used by §10
   ```

   A publisher is therefore known by two things: its **identifier**, which authorizes the event,
   and its **origin** — a DNS name — which is what the webhook handshake of §5.2 works on. They are
   configured together and deliberately not derived from one another, for the same reason D-8
   gives: nothing may be inferred from a participant identifier.

   An event whose `source` differs from the authenticated identifier, or whose `type` that publisher
   is not configured for, is `403`. **This is the check that matters**: without it every
   organization that holds the credential needed to reach the endpoint — in a typical configuration,
   every participant — could announce the removal of any other participant and have its competitors'
   trust entries deleted. A PEP policy on the credential *type* alone cannot express "only this
   publisher".
3. **Self-events are ignored.** An event whose `subject` is this participant's own identifier is
   recorded and reported but never acted on; it would otherwise be a remote self-destruct.

**The publisher list is runtime configuration, not start-up configuration.** A data space may gain a
second trust anchor, or a participant may join another data space, without a restart being an
acceptable cost. The list is therefore held in a `@Refreshable` configuration bean fed from the
mounted ConfigMap, picked up through a file watch and through the Micronaut `/refresh` endpoint, so
`kubectl apply` on the ConfigMap (or a `helm upgrade`) takes effect without recreating the pod. A
publisher added at runtime is active for the next event; subscriptions to it are registered by the
same code path that runs at start-up (§5.2). A self-service management API would additionally
require the list to be persisted, which D-4 avoids; it is noted as the extension if a data space
ever wants participants to register publishers themselves.

**How the holder identifier is obtained.** The token is a JWT minted by the receiver's own
VCVerifier and it carries the presented credential (`jwtInclusion.fullInclusion: true` in the
shipped configuration). `contract-management` verifies it against the verifier's JWKS
(`/.well-known/jwks`) and reads the holder/issuer identifier from it, rather than trusting a header
the PEP injects. That keeps check 2 valid even if the events path is ever exposed without a PEP in
front, and it is the only part of the mechanism that needs the verifier's address at the receiving
side.

**Not done:** signing the event *payload* (a JWS by the publisher) so it is verifiable independently
of the transport. The review found no governance requirement for non-repudiation of a removal
notice, and it stays additive — a `signature` extension attribute — if one appears (D-7).

## 7. Delivery semantics

| Property | This design |
|---|---|
| Direction | push, publisher to subscriber |
| Guarantee | **at-least-once, best effort** — in-memory retry with exponential backoff (default 5 attempts over ~5 min), then dropped with an error log and a metric |
| Ordering | none; events carry `time` and are idempotent |
| Duplicates | expected; receivers deduplicate on `(source, id)` |
| Loss | possible — covered, for this event type, by the reconciliation sweep of §10 |

This is a deliberate trade (D-4), confirmed in review: no party needs auditable proof that a removal
notice was delivered. A durable outbox would mean a database for `contract-management`, which today
is **completely stateless** (no datasource, no Liquibase, nothing to migrate), and a retention and
audit policy for the event log. For `participant.removed` the cheaper and *more* reliable answer is
that the authoritative state — the TIR — is public and can be re-read at any time, so a lost event
costs latency, not correctness.

An event type that cannot be reconciled from authoritative state — the future
`service.configuration.provided`, where the event *is* the state — must not be carried this way. Its
introduction is the point at which a durable outbox is added. The API contract above does not change
when that happens, which is why the publisher keeps its own identity for `source` and its own `id`
space from the start.

## 8. Detecting the removal at the trust anchor

The watcher is part of the **publisher-mode `contract-management` instance that the operator runs in
the trust anchor's namespace** (§11) — not the trusted-issuers-list, and not a separate process. It
**watches the registry and diffs it**; the registry is not instrumented.

```
every dataspace.events.publisher.tirWatch.interval (default 60s):
  snapshot := enumerate GET /issuer   (all pages, trusted-issuers-list management API)
  if the enumeration did not complete       -> discard, keep the previous snapshot, warn
  missing := previous \ snapshot
  for each id in missing:
      missingSince[id]++                     (confirmation counter)
      if missingSince[id] == confirmations   -> emit participant.removed
  for each id in snapshot: missingSince.remove(id)
  if |missing| > maxRemovalsPerCycle         -> emit nothing, alert, keep the previous snapshot
  previous := snapshot
```

**Why diffing and not a hook in the trusted-issuers-list.** The operator documentation states
explicitly that the FIWARE trusted-issuers-list is *one* implementation of the TIR and that any
EBSI-compatible registry — including blockchain-backed ones — may be used
(`doc/deployment-integration/roles/operator/README.md`). A webhook inside the FIWARE implementation
would only ever work for deployments that use it. Diffing works against any registry that can be
enumerated, and it also catches removals made directly in the database or by a migration, which no
application-level hook would see.

**The safeguards are not optional.** A diffing watcher turns *any* failure that makes the registry
look empty — a truncated page, a database that comes up without its data, a misconfigured base URL —
into a mass-removal announcement that would delete trust configuration at every participant in the
data space. Hence: only a completed enumeration becomes a snapshot; an identifier must be absent for
`confirmations` consecutive cycles (default 2) before it is announced; more than
`maxRemovalsPerCycle` disappearances at once (default 5) emits nothing and raises an alert for a
human. The first snapshot after a start is taken silently — a fresh publisher announces nothing.

**Latency** is therefore `interval × confirmations`, about two minutes with the defaults, which the
review accepted as appropriate for a governance action that is itself manual. If a data space ever
promises a shorter propagation time, a webhook from the FIWARE trusted-issuers-list becomes worth
building *on top of* the watcher, which stays as the backstop for the registries that cannot emit
one.

**Manual publication.** The publisher also exposes an operator-only `POST /events` of its own
(cluster-internal, not published through the ingress) so the onboarding portal or an operator script
can announce a removal immediately. The watcher then sees the identifier disappear and deduplicates
against the already-published event.

## 9. Attributing local trust, and reacting to a removal

A participant may be a member of several data spaces, and its TIL legitimately contains issuers that
belong to none of them. A removal announced by one data space therefore may not be answered by
looking at the TIL as a whole — the connector has to know **why** a local entry exists before it may
touch it.

The trusted-issuers-list already records that: every credential it holds is attributed to the
**scope** that granted it (`/issuer/{did}/credential?scope=`, see `TrustedIssuersListAdapter`). This
design extends the existing mechanism rather than adding a second one — entries that exist *because*
an organization is a member of a data space are granted under that data space's scope.

The scope is a free-form string today, holding ProductOrder ids. Rather than reserve a prefix inside
it, the grant gains a **type**: the trusted-issuers-list takes an optional `scopeType` beside
`scope` (D-11), so a membership grant is `scope=<publisher id>`, `scopeType=dataspace` and cannot
collide with an order id whatever an order id happens to look like.

| Origin of the entry | `scope` | `scopeType` | Meaning |
|---|---|---|---|
| `contract-management` on order activation | the ProductOrder id | *(none)* | "this order grants them this" |
| membership registration — chart `registration.issuer`, onboarding portal | the publisher id | `dataspace` | "I trust them because they are a member of that data space" |
| operator by hand, or anything created before this feature | *(none)* | *(none)* | unattributed; outside the mechanism |

The addition is backwards compatible in both directions: the column is nullable, so every existing
row stays valid, and a scoped call that passes no `scopeType` addresses the **untyped** grants only
— which is exactly what every caller writing ProductOrder scopes does today, so none of them can
reach a membership grant by accident, and none of them has to change.

The scope stays **internal to the trusted-issuers-list and its management API** — it is never
exposed through any TIR projection, as the specification requires today.

**The reaction is configured per publisher**, and the default changes nothing:

| `onParticipantRemoved` | Action | TIL call |
|---|---|---|
| `report` *(default)* | record and report only; no local state changes | — |
| `dataspace` | drop what this data space's membership granted | `DELETE /issuer/{did}/credential?scope=<publisher id>&scopeType=dataspace` |
| `granted` | `dataspace`, plus every order-scoped grant to that identifier | additionally one delete per scope from `GET /issuer/{did}/scopes` |
| `full` | drop the issuer entirely, including other data spaces' entries and unattributed ones | `DELETE /issuer/{did}` |

`report` is the default because, as the review put it, local entries should be left untouched unless
cleanup is explicitly configured: an operator action in someone else's namespace should not silently
delete trust configuration here. Overrides refine it per participant:

```yaml
onParticipantRemoved:
  mode: report
  overrides:
    - id: did:web:former-member.example.org
      mode: dataspace
    - id: did:elsi:VATDE-7654321
      mode: full
  keep: [ did:web:trust-anchor.org ]     # never touched by any mode or sweep
```

**Unattributed entries are never removed automatically**, in any mode but `full` — including the
entries that exist in every deployment today, which predate the scoping. They appear in the report
instead. For a participant that is a member of exactly one data space, the opt-in
`adoptUnattributed: true` treats them as that data space's membership grants, which is the migration
path for an existing installation; it refuses to act when more than one publisher is configured,
because then nothing can say which data space an unattributed entry belongs to.

**The report is part of the feature, not an extra.** Whatever the mode, the participant ends up with
a record of "that data space removed X; this is what I hold for X and what I did about it", exposed
as

- a log line and the metric `dsc_dataspace_events_participant_removed_total{publisher,action,mode}`,
- `GET /events/participants/removed` — the identifiers a data space removed for which local entries
  remain, with the scopes holding them and whether they are attributable.

Without it, `report` would quietly accumulate exactly the situation this set out to fix.

**What is deliberately not touched.** ODRL policies at the PAP, Rainbow/EDC agreements, running
access tokens, and credentials already issued to the organization's users. The review confirmed that
reporting them is enough for now: policies and agreements belong to orders, which the default
behaviour keeps, and token lifetimes are short.

## 10. Reconciliation: the backstop

Enabled with the feature, the participant periodically (default 6h, and once at start-up) compares,
**for each configured publisher separately**, the entries attributable to that data space against
that data space's registry:

```
for each publisher P with registry R:
    attributed := GET /issuer?scope=<P>&scopeType=dataspace   (TIL management API)
    members    := enumerate GET R/v4/issuers
    stale      := attributed \ members \ keep
    apply onParticipantRemoved(P) to stale, capped by maxRemovalsPerRun
```

The first draft diffed the TIL *as a whole* against one TIR. That was wrong: a participant may use
many registries — a verifier can be configured with a different `trustedParticipantsLists` per
credential type and per service — so "in my TIL but not in that TIR" says nothing about whether an
entry is stale. Asking the question per data space, over the entries that data space's membership
created, is both answerable and safe: an issuer that belongs to two data spaces keeps the entries
of the one it is still a member of, and an issuer that belongs to none of them is never considered
at all.

This costs two paginated reads per publisher a few times a day and buys: recovery from any lost
event, correct behaviour for a participant installed after a removal happened, and a deployment that
is never *dependent* on the push path being reachable (which is what lets §7 stay stateless).

It introduces no new trust relationship: the participant's verifier already polls those same
registries for every credential validation.

## 11. Deployment

The configuration namespace is **`dataspace.events`**, not `contract-management.events` (D-12).
The mechanism belongs to the data space, not to the component that happens to host it today.
`contract-management` keeps its name — renaming it is explicitly not part of this work — but if it
is ever renamed, nothing in a deployed configuration has to follow. In the umbrella chart the block
still sits under the subchart alias, as every subchart knob does.

**At the participant:**

```yaml
contract-management:
  dataspace:
    events:
      listener:
        enabled: true
        verifier:
          jwksUrl: http://verifier:3000/.well-known/jwks
        # runtime-refreshable (mounted ConfigMap + /refresh), see §6
        publishers:
          - id: did:web:trust-anchor.org
            origin: trust-anchor.org          # for the webhook handshake, §5.2
            types: [ "org.fiware.dataspace.participant.removed" ]
            registry: https://tir.trust-anchor.org
            subscription:
              sink: https://contract-management.mp-operations.org/events
              clientId: dataspace-events
              scopes: [ default ]
              renewInterval: 10m
            onParticipantRemoved:
              mode: report
              adoptUnattributed: false
              keep: [ did:web:trust-anchor.org ]
              overrides: []
            reconciliation:
              enabled: true
              interval: 6h
              maxRemovalsPerRun: 5
```

plus an ODRL policy admitting the trust anchor's credential type to the events path — shipped as an
example next to `it/src/test/resources/policies/allowContractManagement.json` — the events path on
the contract-management ingress, and a `scope`/`scopeType` on the chart's `registration.issuer`
entries so that membership-derived trust is attributable from the first install (§9).

**At the trust anchor** — the publisher stays a **separate release**, as the review asked: the
`trust-anchor` chart does not gain a dependency on something called `contract-management`. It is
deployed into the trust anchor's namespace exactly as the consent-authority is
(`k3s/consent-trust-anchor.yaml`), reusing **the same identity** — the authority's `did:web`, its
signing key secret and its credential — so no new key material or registration is introduced:

```yaml
contract-management:
  enabled: true
  did: did:web:trust-anchor.org
  enableTmForum: false
  enableOdrlPap: false
  enableTrustedIssuersList: false
  enableRainbow: false
  enableCentralMarketplace: false
  oid4vp:
    enabled: true                 # to authenticate towards the participants
    holder: { holderId: did:web:trust-anchor.org, ... }
  dataspace:
    events:
      publisher:
        enabled: true
        origin: trust-anchor.org  # sent as WebHook-Request-Origin, §5.2
        tirWatch:
          enabled: true
          tir: http://tir:8080
          interval: 60s
          confirmations: 2
          maxRemovalsPerCycle: 5
```

**Everything is off by default.** `dataspace.events.listener.enabled`,
`dataspace.events.publisher.enabled` and
`reconciliation.enabled` default to `false`, and the default reaction is `report`, so enabling the
feature on an existing deployment changes no trust configuration until someone asks for it.

## 12. Decision register

Accepted in the review of 2026-10-28 unless noted; each is a candidate ADR (`doc/adr/`).

| # | Decision | Rationale | Alternative rejected |
|---|---|---|---|
| **D-1** | Both ends of the mechanism live in **contract-management**, in a publisher and a listener mode | It already receives authenticated events (`/order/*`), already sends them over OID4VP (`ContractManagementAdapter` + `Oid4VpAuthHandler`), and already registers callbacks at start-up with retry and a health indicator. Accepted with the note that the component may have to be **renamed** once it carries more than contract management | A dedicated `dataspace-events` service; extending the trusted-issuers-list |
| **D-2** | **CloudEvents 1.0** envelope, reverse-DNS types, `dataspaceid` optional | A standard envelope with id/source/type/subject/time is exactly what dedup, routing and authorization need. Most data spaces have no identifier of their own, so the publisher's `source` is the discriminator | A TMForum hub/listener-shaped API; a bespoke JSON body; a mandatory data space id |
| **D-3** | Removals are detected by **polling and diffing the TIR**, not by a hook in the trusted-issuers-list | The TIR is explicitly pluggable, including third-party and blockchain-backed implementations; diffing is the only detection that works for all of them, and it also sees out-of-band changes. The ~2 min propagation this implies was accepted | A webhook in the FIWARE trusted-issuers-list (possible later as a latency optimisation, with the watcher as backstop) |
| **D-4** | Delivery is **best-effort and stateless**; subscriptions are **leased and in memory** | No party needs auditable proof of delivery. Keeps contract-management stateless (it has no database at all today). For this event type the authoritative state is public and re-readable, so §10 is a stronger guarantee than an outbox | A persisted outbox and subscription registry with a database — required later for event types that cannot be reconciled |
| **D-5** | The receiver authorizes **publisher × event type** in the application, on top of the PEP | A PEP policy constrains the credential *type*; in a normal data space many organizations hold it. Without D-5 any of them could announce any other participant's removal. An ODRL left operand for the credential's issuer is not available today and would only add defence in depth | Relying on the ODRL policy alone |
| **D-6** | Local trust is **attributed through the existing TIL scope**; the reaction is configured per publisher and defaults to **`report`** | The scope already records *why* an entry exists, which is what makes multi-data-space membership answerable (D-10); and an action in another namespace must not silently delete trust configuration here | A single on/off switch; a `reason`-driven default; always removing everything |
| **D-7** | **No payload signature** | No governance framework in sight requires non-repudiation of a removal notice; it stays additive if one appears | A JWS over the envelope in v1 |
| **D-8** | A callback is confirmed with the **CloudEvents webhook handshake**, never derived from the subscriber's identifier | Participant identifiers are not necessarily `did:web`, so no host can be derived from them; the handshake proves consent for any sink, whatever the DID method | Matching the callback host against a `did:web` domain (first draft); accepting an arbitrary callback URL |
| **D-9** | Use the **specified protocol where one exists**: CloudEvents HTTP Protocol Binding (structured mode) and the CloudEvents HTTP webhook handshake; the subscription request follows the shape of the CloudEvents Subscriptions API without claiming conformance | The review's rule — if a specification for `application/cloudevents+json` exchange exists, use it. The binding is a released 1.0.2 document; the webhook document is `1.0.3-wip`, so its headers are pinned here | Specifying a FIWARE-local wire protocol |
| **D-11** | Membership grants are **typed** in the trusted-issuers-list (`scope=<publisher id>`, `scopeType=dataspace`) rather than carrying a reserved prefix in `scope` | `scope` is free-form and holds ProductOrder ids; a `dataspace:` prefix is a convention nothing enforces and an order id could collide with. A nullable column plus an optional query parameter is backwards compatible, and a call without `scopeType` keeps addressing the untyped grants | A reserved `dataspace:` prefix (first draft); a bare publisher id as the scope, with the configuration deciding what it means |
| **D-12** | The configuration namespace is **`dataspace.events`**, independent of the component's name | `contract-management` keeps its name for now, and the mechanism is not about contracts. Naming the configuration after the data space means a later rename costs nothing in deployed configuration | `contract-management.events.*` (first draft); renaming the component as part of this epic |
| **D-10** | **Multi-data-space membership is supported**: every reaction and every sweep is per publisher and touches only entries attributable to that data space | A participant can be a member of several data spaces and trust issuers from none of them; without attribution no removal can be answered safely | Treating the TIL as belonging to one data space; a `keep` list as the only guard (the first draft) |

## 13. Failure modes and security considerations

| Failure | Effect | Mitigation |
|---|---|---|
| Registry briefly unreachable or returns a partial list | mass false removals announced | only complete enumerations become snapshots; `confirmations` cycles; `maxRemovalsPerCycle` circuit breaker; first snapshot is silent |
| Publisher restarts | subscriptions lost, events missed | subscribers renew on a timer; reconciliation sweep |
| Participant unreachable | event lost after retries | reconciliation sweep |
| Spoofed removal by another participant | trust entries deleted across the data space | D-5 publisher×type authorization, token verified against the verifier's JWKS, self-events ignored |
| Subscription API pointed at a third party | the trust anchor POSTs authenticated traffic at a host that never asked for it | D-8 webhook handshake — a sink that does not consent is never activated; per-subscriber cap; lease expiry |
| Replayed event | repeated cleanup | `(source, id)` dedup; handlers converge; an entry re-created by a later order carries the order's scope and survives `dataspace` mode |
| Operator removes a participant by mistake | trust entries deleted at every participant | `confirmations`, the circuit breaker, the `report` default, and the report that lets an operator see and undo |
| Participant is a member of two data spaces, one removes it | the other data space's trust is destroyed | D-10 attribution: only the removing data space's scope is touched (except in `full`, which is documented as blunt) |
| The onboarding portal removes a participant from a TIL | it calls `DELETE /issuer/{did}` today, which also drops order-granted and other data spaces' entries | OBP-1: register with the membership scope and remove only that scope's credentials |
| Unattributed legacy entries | invisible to the sweep, never cleaned | listed in the report; `adoptUnattributed` for single-membership deployments; `registration.issuer` carries a scope going forward |

## 14. What this opens up

- **`service.configuration.provided`** — the provider hands the consumer the configuration of a
  service it has bought, instead of the consumer polling or receiving it out of band. Same envelope,
  same auth; it is the event type that will require durable delivery (§7).
- **`participant.registered` / `credentials.changed`** — a participant can pre-populate or refresh
  local configuration without waiting for a first failed call.
- **Status/revocation notices** alongside the status-list server, for consumers that want to react
  rather than poll.
- A **second publisher** is just another entry in `publishers`: a sectoral authority, a second trust
  anchor, or a marketplace announcing catalogue changes — and, with D-10, a second data space.
- **An interoperability specification.** Not a submission to a governance body — DSBA, Gaia-X and
  IDSA are unaffected by this — but the write-up a second implementation needs in order to talk to
  a FIWARE participant: the event types and their payloads, the authentication binding (OID4VP
  access token obtained at the receiver's verifier), the authorization rule of §6 and the
  subscription profile. D-9 keeps the wire format on existing CloudEvents documents, so it is
  short. It lives in this repository (DSC-3) and can be carried outward later if anyone asks for
  it.

## 15. Impact per repository

| Repository | Change |
|---|---|
| [contract-management](https://github.com/FIWARE/contract-management) | the whole mechanism: `/events`, `/subscriptions`, the webhook handshake, the watcher, the per-publisher reaction, the reconciliation sweep, the report, refreshable configuration |
| [trusted-issuers-list](https://github.com/FIWARE/trusted-issuers-list) | management-API additions only: list the scopes holding credentials for an issuer; list the issuers holding credentials of a scope; delete the unattributed credentials of an issuer (for `adoptUnattributed`). The scope stays out of every TIR projection |
| [helm-charts (contract-management chart)](https://github.com/FIWARE/helm-charts) | the `dataspace.events` values block as a mountable ConfigMap, the events path on the ingress |
| [On-Boarding-Portal](https://github.com/SEAMWARE/On-Boarding-Portal) | register issuers with the membership scope instead of unscoped, and remove a participant by dropping that scope's credentials rather than the whole issuer; optionally publish the removal event directly |
| **data-space-connector** (this repo) | umbrella values + schema, a scope on `registration.issuer`, the trust-anchor publisher release, example ODRL policy, k3s overlays, an integration test, documentation, release notes, ADRs |
