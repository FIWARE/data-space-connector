# Data Space Eventing — Interoperability Specification

**Version:** 1.0 (draft)
**Date:** 2026-11-11
**Source API:** `dataspace-events.yaml` `0.0.1`, [FIWARE/contract-management](https://github.com/FIWARE/contract-management)
at commit `c7da3c4`

<!-- ToC created with: https://github.com/thlorenz/doctoc -->

- [1. Status and scope](#1-status-and-scope)
- [2. Normative references](#2-normative-references)
- [3. Event format](#3-event-format)
- [4. Event types](#4-event-types)
- [5. Authentication binding](#5-authentication-binding)
- [6. Authorization rule](#6-authorization-rule)
- [7. Receiving endpoint](#7-receiving-endpoint)
- [8. Subscription profile](#8-subscription-profile)
- [9. Webhook validation handshake](#9-webhook-validation-handshake)
- [10. Not specified](#10-not-specified)
- [11. Conformance checklist](#11-conformance-checklist)
- [Relation to the architecture](#relation-to-the-architecture)

## 1. Status and scope

This document is the interoperability contract for data space events: what a second, non-FIWARE
implementation needs in order to publish events to, or subscribe at, a FIWARE participant. It is
normative and short; the reasons behind each rule are in the [architecture](architecture.md).

It is not a submission to a governance body. DSBA, Gaia-X and IDSA are unaffected by it. If one of
them, or any other party, asks for the eventing contract, this is the document to hand over.

Roles:

- **Publisher:** emits events, offers `POST /subscriptions`, and delivers events to the sinks of its
  subscribers.
- **Receiver / subscriber:** offers `POST /events` (the *sink*), subscribes at publishers, and acts on
  the events it receives.

The key words MUST, MUST NOT, REQUIRED, SHOULD, SHOULD NOT, RECOMMENDED, MAY and OPTIONAL are to be
interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) and
[RFC 8174](https://www.rfc-editor.org/rfc/rfc8174) when, and only when, they appear in all capitals.

## 2. Normative references

- [CloudEvents 1.0.2 core](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md).
- [CloudEvents HTTP Protocol Binding 1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/bindings/http-protocol-binding.md),
  structured content mode only.
- [CloudEvents JSON Event Format 1.0.2](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/formats/json-format.md).
- [HTTP 1.1 Web Hooks for Event Delivery, 1.0.3-wip](https://github.com/cloudevents/spec/blob/56b54136a6088c47058ed43c2df45e433b4868f0/cloudevents/http-webhook.md),
  pinned to that commit. Only §4 (abuse protection) is used, as profiled in [§9](#9-webhook-validation-handshake).
  The released 1.0.2 differs only in callback validation and rate wording, neither of which this profile relies on.
- [CloudEvents Subscriptions API, 0.1-wip](https://github.com/cloudevents/spec/blob/56b54136a6088c47058ed43c2df45e433b4868f0/subscriptions/spec.md),
  pinned to the same commit: the shape of the subscription request is borrowed; conformance is **not** claimed ([§8](#8-subscription-profile)).
- [OpenID for Verifiable Presentations 1.0](https://openid.net/specs/openid-4-verifiable-presentations-1_0.html) (OID4VP),
  as profiled in [§5.1](#51-token-flow-fiware-m2m-profile-of-oid4vp).

## 3. Event format

An event is a CloudEvent in the JSON event format, sent in HTTP structured mode with
`Content-Type: application/cloudevents+json`. Binary mode is not supported; a receiver answers it
with `415`.

| Attribute | Requirement | Rule |
|---|---|---|
| `specversion` | REQUIRED | MUST be `"1.0"`. A receiver does not accept other CloudEvents versions. |
| `id` | REQUIRED | Unique per `source`. (`source`, `id`) is the deduplication key. |
| `source` | REQUIRED | The publisher's participant identifier. Any DID method is allowed. MUST equal the authenticated identity ([§5](#5-authentication-binding)). |
| `type` | REQUIRED | `org.fiware.dataspace.<entity>.<past-tense-verb>` ([§4](#4-event-types)). |
| `subject` | OPTIONAL | The entity the event is about; defined per type. |
| `time` | OPTIONAL | RFC 3339 timestamp. |
| `datacontenttype` | OPTIONAL | `application/json` for the types of [§4](#4-event-types). |
| `dataschema` | OPTIONAL | Not used by the types of [§4](#4-event-types). |
| `dataspaceid` | OPTIONAL | Extension attribute naming the data space. If the event carries it and the receiver has a data space id configured, the two MUST be equal, otherwise the receiver answers `403`. If either side leaves it blank, it is not checked. |
| `data` | per type | Type-specific payload. |

Type naming: a breaking change of the `data` of a type MUST be introduced as a new `type`.

A receiver MUST accept and ignore an event of a type it does not know, with `202`. It does so after
authentication and the `source`/`dataspaceid` checks, but before the publisher × type check
([§6](#6-authorization-rule)). The `202` applies only to a complete event: an incomplete event of an
unknown type gets `400`.

## 4. Event types

### 4.1 `org.fiware.dataspace.participant.removed`

A participant was removed from the data space's trusted issuers registry.

- `subject`: the removed participant's identifier. A publisher MUST send it.
- `data.participantId` (string): equal to `subject`. A publisher MUST send it. It is the only defined
  `data` member.
- A receiver identifies the removed participant from `subject`, and falls back to
  `data.participantId` if `subject` is blank. If both are missing, the event is invalid (`400`).
  The self-event rule of [§6](#6-authorization-rule) (check 8) applies to this resolved identifier.
- A publisher MAY send further `data` members. A receiver MUST ignore unknown `data` members and MUST
  NOT depend on them.
- There is deliberately no `reason` ([architecture §4](architecture.md#4-the-event-model)).

```json
{
  "specversion": "1.0",
  "id": "0f3b9a2c-1f3c-4a0e-9a2d-2d1b4f0a77c1",
  "type": "org.fiware.dataspace.participant.removed",
  "source": "did:web:trust-anchor.org",
  "subject": "did:elsi:VATDE-1234567",
  "time": "2026-10-28T09:12:44Z",
  "datacontenttype": "application/json",
  "data": {
    "participantId": "did:elsi:VATDE-1234567"
  }
}
```

### 4.2 Reserved names

The following types are reserved and not specified: `org.fiware.dataspace.participant.registered`,
`org.fiware.dataspace.participant.credentials.changed`, `org.fiware.dataspace.participant.suspended`,
`org.fiware.dataspace.service.configuration.provided`.

## 5. Authentication binding

Every call of [§7](#7-receiving-endpoint) and [§8](#8-subscription-profile) is authenticated with an
OID4VP-issued access token, sent as `Authorization: Bearer <token>`.

- **Token.** The caller obtains the token over OID4VP at the **callee's** verifier, i.e. the verifier
  that protects the called path. For `POST /events` the publisher authenticates at the receiver's
  verifier; for the subscription calls the subscriber authenticates at the publisher's verifier.
- **Credential.** The caller presents a verifiable credential of the type the callee admits on that
  path. The type is local configuration of the callee, not a data space-wide parameter.
- **401.** A callee answers `401` when the token is missing or invalid. The caller then runs the
  flow of [§5.1](#51-token-flow-fiware-m2m-profile-of-oid4vp) and repeats the request.
- **Delivery.** When delivering to a sink, the publisher uses the subscriber's
  `protocolsettings.clientId` and `protocolsettings.scopes` ([§8](#8-subscription-profile)).
- **Flow.** The token is obtained with the profile of [§5.1](#51-token-flow-fiware-m2m-profile-of-oid4vp).

### 5.1 Token flow (FIWARE M2M profile of OID4VP)

The token flow is a machine-to-machine profile of OID4VP, not the OID4VP same-device or cross-device
flow: the presentation goes to the verifier's token endpoint, not to a `response_uri`. A verifier that
implements only OID4VP 1.0 does not interoperate with it. It is the behaviour of the FIWARE client
(contract-management `Oid4VpAuthHandler`, oid4vp-client-lib `0.0.7`, `OID4VPClient` and
`AuthorizationClient`).

1. **Discovery.** On a `401`, the caller derives the verifier's metadata URL from the called URL
   `<scheme>://<host>[:<port>]/<path>` as
   `<scheme>://<host>[:<port>]/<path>/.well-known/openid-configuration`, with leading and trailing
   `/` of `<path>` removed (`<scheme>://<host>[:<port>]/.well-known/openid-configuration` for an empty
   path). The metadata MUST list `authorization_endpoint`, `token_endpoint`, `scopes_supported`
   containing the requested scopes, `grant_types_supported` containing `vp_token`, and
   `response_types_supported` containing `code`.
2. **Authorization call.** The caller sends `GET <authorization_endpoint>` with the query parameters
   `client_id`, `scope`, `state`, `nonce` and `response_type=code`.
3. **Redirect.** The verifier MUST answer `302` with a `Location` in the `openid4vp://` scheme. Its
   query MUST carry `client_id` and either a `request` (the request object inline) or a
   `request_uri` with `request_uri_method=get`, which the caller fetches with `GET` (answer `200`, the
   JWT as body). Any other answer ends the flow.
4. **Request object.** The request object MUST be a JWT signed by the verifier with an `RS*` or `ES*`
   algorithm. The caller resolves the key from `client_id` (`did:key:`, `did:web:` or
   `x509_san_dns:`) and verifies the signature. Its payload MUST echo the caller's `nonce` and carry
   `response_type=vp_token` and `response_mode=direct_post`; it SHOULD carry a `dcql_query`
   describing the credential the callee admits ([§5](#5-authentication-binding)). The caller does not check `state`.
5. **Token request.** The caller posts `application/x-www-form-urlencoded` to `token_endpoint` with
   `grant_type=vp_token`, `vp_token` (the signed presentation, holder = the caller's identifier),
   `scope` (space-separated) and `client_id`. The verifier MUST accept `grant_type=vp_token` there
   and MUST answer `200` with a JSON token response carrying `access_token`; any other status ends the
   flow.

### 5.2 Caller identity

The value the callee compares with the event's `source` (or with
`subscriberId`) is the `credentialSubject.id` of the presented credential, i.e. the holder's
identifier.

- A caller MUST present a credential whose `credentialSubject.id` equals the `source` (or the
  `subscriberId`) it sends.
- A callee MUST configure its verifier so that exactly this value reaches the comparison.

*Informative:* in a FIWARE receiver the verifier's `jwtInclusion` (`claimsToInclude` with
`flatClaims`, credentials-config-service) maps `credentialSubject.id` to the top-level token claim
`participantId`. The API gateway verifies the token and passes its claims in a header (`X-Userinfo`),
and contract-management reads the claim named by `dataspace.events.listener.identity.participant-claim`.

## 6. Authorization rule

A receiver admits only tokens issued by its own verifier
([architecture §6](architecture.md#6-trust-and-authorization)). It then applies the checks below **in
this order**; the first that matches decides the answer. The order is normative: it decides which
status a publisher gets when several checks fail.

| # | Condition | Answer |
|---|---|---|
| 1 | No authenticated identifier (token missing or invalid) | `401` |
| 2 | Authenticated identifier ([§5](#5-authentication-binding)) ≠ `source` (a missing `source` is a mismatch) | `403` |
| 3 | `dataspaceid` present on both sides and different ([§3](#3-event-format)) | `403` |
| 4 | `type` known to the receiver, and the publisher not configured for that `type` (skipped for an unknown type) | `403` |
| 5 | `specversion`, `id`, `source` or `type` missing | `400` |
| 6 | `type` unknown to the receiver (the event is complete) | `202`, dropped, not deduplicated |
| 7 | Deduplication on (`source`, `id`): a repeat | `202` |
|   | … the first copy is still being processed | `503` with `Retry-After` |
| 8 | The event is about the receiver itself: `subject`, or the identifier the type resolves when `subject` is blank ([§4.1](#41-orgfiwaredataspaceparticipantremoved)), is the receiver's own identifier | `202`, no action; recorded for deduplication |
| 9 | Otherwise the event is dispatched | `202` |
|   | … a handler rejects the event as invalid | `400` |
|   | … a downstream service fails | `502` |
|   | … any other failure | `500` |

Consequences a publisher must expect:

- An unconfigured publisher that sends a complete event of an unknown type gets `202`, not `403`.
- An incomplete event of an unknown type gets `400`, not `202`.
- A malformed event whose `source` differs from the token gets `403`, not `400`.
- A malformed event of a known type from a publisher not configured for it gets `403`, not `400`.
- A body that cannot be read as an event at all (not a JSON object, `specversion` other than `"1.0"`)
  MAY be answered `400` at any point after check 1.
- An event a handler rejected as invalid (`400`) is recorded for deduplication, so its redelivery
  MAY be answered `202`.

The publisher × type check (4) is what keeps one data space member from announcing events about
another ([architecture D-5](architecture.md#12-decision-register)). An ODRL left operand for the
credential **issuer** does not exist today. It would only add defence in depth at the PEP; it is a
possible contribution to the PAP and not a dependency of this specification.

## 7. Receiving endpoint

`POST <sink>`, where the `sink` is the absolute URL the receiver announces in its subscription
([§8](#8-subscription-profile)); in contract-management it is `<base URL>/events`.

| Request header | Rule |
|---|---|
| `Authorization` | REQUIRED. `Bearer <token>` ([§5](#5-authentication-binding)). |
| `Content-Type` | REQUIRED. `application/cloudevents+json`. |
| `WebHook-Request-Origin` | The publisher MUST send its configured origin ([§9](#9-webhook-validation-handshake)). Not used for authorization. |

| Status | Meaning |
|---|---|
| `202` | Accepted. Also returned for duplicates, unknown types and events about the receiver itself. |
| `400` | The event is malformed or invalid. |
| `401` | The caller was not identified. |
| `403` | The participant is not allowed to publish this event. |
| `415` | The request is not a structured-mode CloudEvent. |
| `500` | The event could not be handled. The publisher may retry. |
| `502` | A downstream service failed while the event was handled. The publisher should retry. |
| `503` | An event with the same `source` and `id` is still being processed. `Retry-After` (seconds) says when to retry. |

Delivery semantics ([architecture §7](architecture.md#7-delivery-semantics)):

- Delivery is at-least-once and best-effort. There is no ordering guarantee. A receiver MUST be
  idempotent on (`source`, `id`).
- A publisher retries a delivery answered with a `5xx`, `408` or `429`, with backoff, for a bounded
  number of attempts. Other answers are final.
- A `410 Gone` from the sink ends the subscription: the publisher removes it and sends no further
  events to it.
- *Informative:* the reference publisher also retries transport failures without an HTTP status
  (connection refused, timeout, TLS error), so an unreachable sink sees redeliveries.

## 8. Subscription profile

Offered by the publisher, relative to a base URL the subscriber is configured with.

### 8.1 `POST /subscriptions`

Request body (`application/json`, `SubscriptionRequest`):

| Field | Required | Rule |
|---|---|---|
| `subscriberId` | yes | The subscriber's participant identifier; MUST equal the caller's identity ([§5](#5-authentication-binding)). |
| `sink` | yes | Absolute `https` URL of the subscriber's receiving endpoint. A publisher MAY admit `http` for testing. |
| `types` | yes | Array of event types, `minItems: 1`, no blank item. |
| `protocol` | yes | `HTTP`, the only value. |
| `protocolsettings` | yes | `clientId` (required, non-blank): OID4VP client id at the sink's verifier. `scopes` (optional array, may be empty): scopes requested with the token. |

Responses:

- `201` with a `Subscription`: the five request fields echoed back plus `id` and `expiresAt` (RFC
  3339). All seven are required.
- `400` with `SubscriptionError {"reason": "<text>"}`: the request is invalid, or the sink did not
  pass the handshake ([§9](#9-webhook-validation-handshake)). The reason is generic and never reveals
  the sink's answer.
- `401`: the caller was not identified.
- `403`: `subscriberId` differs from the caller's identity.
- `503`: the subscription cannot be accepted now (no publisher origin configured, or a subscription
  cap reached). The subscriber SHOULD retry later.

Check order: `401` → missing `subscriberId` `400` → `subscriberId` mismatch `403` → other missing or
invalid fields `400` → `503` → handshake (`400` on failure) → `201`.

Leases ([architecture D-4](architecture.md#12-decision-register)):

- A request by the same subscriber for the same sink renews the existing subscription in place: same
  `id`, new `expiresAt`. The `types` and `protocolsettings` of the request replace the previous ones,
  so a renewal MUST carry every type the subscriber still wants. The handshake runs again, and its
  `WebHook-Allowed-Rate` replaces the previous rate.
- A subscriber MUST renew before `expiresAt`.
- Leases are not durable. A subscriber MUST re-subscribe after a publisher restart; since it cannot
  observe the restart directly, renewing on a timer well within the lease covers it.
- Lease length, caps and deduplication windows are implementation choices, not part of this contract.

### 8.2 `DELETE /subscriptions/{subscriptionId}`

| Status | Meaning |
|---|---|
| `204` | The subscription was removed. |
| `401` | The caller was not identified. |
| `404` | No subscription with this id is owned by the caller. |

### 8.3 Relation to the CloudEvents Subscriptions API

| Subscriptions API | This profile |
|---|---|
| `sink`, `types`, `protocol`, `protocolsettings` | used (`protocol`: `HTTP` only; `protocolsettings`: `clientId`, `scopes`) |
| `id` | used in the response |
| — | added: `subscriberId` (request), `expiresAt` (response) |
| `filters`, `config`, `source`, `subscriptionId` in the request | not used |
| `GET` / `PUT` on subscriptions, discovery | not used |

## 9. Webhook validation handshake

Profile of §4 of the pinned webhook document ([§2](#2-normative-references)); the origin is
configured together with the publisher's identifier and never derived from it
([architecture D-8](architecture.md#12-decision-register)).

Publisher:

- Before it activates (or renews) a subscription, the publisher MUST send `OPTIONS <sink>` with
  `WebHook-Request-Origin: <publisher DNS name>`, without following redirects.
- It activates the subscription only if the sink answers `2xx` with `WebHook-Allowed-Origin` equal to
  that origin (case-insensitive) or `*`. Any other answer, or none, leaves the subscription inactive,
  and the subscriber gets `400`.
- If the answer carries `WebHook-Allowed-Rate`, the publisher MUST honour it: a positive integer
  (requests per minute) or `*` (unlimited). Any other value fails the handshake. A missing header
  means unlimited.
- Every delivery repeats `WebHook-Request-Origin`.
- Callback-request validation (`WebHook-Request-Callback`) is NOT used.

Receiver (sink):

- `OPTIONS` is unauthenticated.
- The sink MUST answer `2xx` and SHOULD include `Allow` (listing `POST`).
- The sink MUST include `WebHook-Allowed-Origin` only when the requesting origin belongs to a
  publisher it is configured to receive from; otherwise it answers without that header.
- The sink MAY include `WebHook-Allowed-Rate`.

## 10. Not specified

- The receiver's reaction to an event: local policy ([architecture §9](architecture.md#9-attributing-local-trust-and-reacting-to-a-removal)).
- How a publisher detects a removal ([architecture §8](architecture.md#8-detecting-the-removal-at-the-trust-anchor)).
- Reconciliation ([architecture §10](architecture.md#10-reconciliation-the-backstop)).
- How a receiver configures its publisher list, and the Helm chart values.
- How a subscriber learns a publisher's base URL for `/subscriptions`, and the client id, scope and
  credential type of the publisher's verifier: out-of-band configuration, like the publisher list.
- Payload signatures ([architecture D-7](architecture.md#12-decision-register)); a future `signature`
  extension attribute would be additive.
- A catch-up feed and brokers ([architecture §5.3](architecture.md#53-what-is-deliberately-not-added)).

## 11. Conformance checklist

A **publisher**:

- sends structured-mode CloudEvents with `specversion` `"1.0"`, `id` unique per `source`, `source`
  equal to its own identifier, and a `type` of [§4](#4-event-types);
- authenticates at the receiver's verifier with the token flow of
  [§5.1](#51-token-flow-fiware-m2m-profile-of-oid4vp) and a credential whose `credentialSubject.id`
  equals `source`, using the subscription's `protocolsettings`;
- protects `/subscriptions` with a verifier that implements the verifier side of
  [§5.1](#51-token-flow-fiware-m2m-profile-of-oid4vp);
- for `participant.removed`, sends `subject` and `data.participantId` with the same value;
- validates every sink with the handshake before activating a subscription, and sends
  `WebHook-Request-Origin` on every delivery ([§9](#9-webhook-validation-handshake));
- honours `WebHook-Allowed-Rate`, retries only `5xx`/`408`/`429` (and transport failures), and drops a
  subscription on `410`;
- offers `POST /subscriptions` and `DELETE /subscriptions/{subscriptionId}` as in [§8](#8-subscription-profile).

A **receiver**:

- admits only tokens from its own verifier, and configures that verifier so that the presented
  credential's `credentialSubject.id` is the identity compared with `source`;
- runs that verifier with the verifier side of [§5.1](#51-token-flow-fiware-m2m-profile-of-oid4vp):
  the well-known metadata, a `302` to an `openid4vp://` URI carrying a signed request object
  (`request`, or `request_uri` with `request_uri_method=get`) that echoes `nonce` and sets
  `response_type=vp_token` and `response_mode=direct_post`, and a `token_endpoint` that accepts
  `grant_type=vp_token` and answers `200` with a JSON token response;
- ignores events about itself, judged by the resolved identifier ([§4.1](#41-orgfiwaredataspaceparticipantremoved));
- applies the checks of [§6](#6-authorization-rule) in that order, with the listed answers;
- accepts and ignores complete events of unknown types (`202`) and unknown `data` members;
- is idempotent on (`source`, `id`);
- answers `415` to non-structured requests and uses the status codes of [§7](#7-receiving-endpoint);
- answers the unauthenticated `OPTIONS` handshake with `2xx`, and includes `WebHook-Allowed-Origin`
  only for the origin of a configured publisher;
- as a subscriber, obtains its token at the publisher's verifier with the flow of
  [§5.1](#51-token-flow-fiware-m2m-profile-of-oid4vp), sends a `subscriberId` equal to its
  authenticated identity, renews before
  `expiresAt` and re-subscribes after a publisher restart.

## Relation to the architecture

This specification follows `dataspace-events.yaml` and the contract-management code at `c7da3c4`.
Where the [architecture](architecture.md) differs, this document wins:

- **Status codes** ([architecture §5.1](architecture.md#51-receiving--post-events)): there is no
  `422`; an invalid payload of a known type is `400`. `415`, `500` and `503` + `Retry-After` are added.
- **`participant.removed` payload** ([architecture §4](architecture.md#4-the-event-model)): the
  architecture lists `observedAt` and `registry`. The reference publisher sends only `participantId`,
  so the two are not specified; a publisher MAY send them, and a receiver ignores them. Generally, a
  field the reference publisher does not emit is never REQUIRED.
- **`dataspaceid`** ([architecture §4](architecture.md#4-the-event-model)): the architecture says no
  receiver behaviour depends on it. A receiver with a configured data space id rejects a different
  one with `403` ([§6](#6-authorization-rule), check 3).
- **Subscription response** ([architecture §5.2](architecture.md#52-subscribing--post-subscriptions)):
  the `201` body is the full `Subscription`, not only `id` and `expiresAt`.
- **Defaults**: the deduplication window, lease length and caps in the architecture (24h, 15 min,
  5 per subscriber) differ from the reference implementation (1h, 24h, 10 per subscriber). Neither is
  normative.
