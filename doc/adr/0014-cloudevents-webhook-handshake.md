# 0014 - Confirm event callbacks with the CloudEvents webhook handshake

**Status:** Accepted
**Date:** 2026-11-20
**Context doc:** [`../eventing/specification.md` §9](../eventing/specification.md#9-webhook-validation-handshake)

## Context

A subscriber registers a callback URL (the `sink`) at a publisher with `POST /subscriptions`. The
publisher then POSTs authenticated events to that URL. If the publisher accepted any URL, its
subscription API would be an amplifier: a subscriber could make the trust anchor send authenticated
traffic to a host that never asked for it.

Matching the callback host against the subscriber's `did:web` domain suggests itself. Participant
identifiers are not necessarily `did:web`, though, and the connector does not mandate a DID method,
so no host can be derived from them.

## Decision

The sink is confirmed with the abuse-protection handshake of the CloudEvents HTTP webhook
specification (pinned to a commit of `1.0.3-wip`, see
[specification §2](../eventing/specification.md#2-normative-references)), never derived from the
subscriber's identifier:

- before it activates or renews a subscription, the publisher sends `OPTIONS <sink>` with
  `WebHook-Request-Origin: <its DNS name>`, without following redirects;
- it activates the subscription only if the sink answers `2xx` with a matching
  `WebHook-Allowed-Origin` (or `*`), and honours `WebHook-Allowed-Rate`;
- every delivery repeats `WebHook-Request-Origin`;
- the sink answers `OPTIONS` unauthenticated and includes `WebHook-Allowed-Origin` only for the
  origin of a publisher it is configured to receive from.

A publisher's origin is configured at the receiver together with its identifier and is not derived
from it ([ADR 0012](0012-authorize-events-per-publisher-and-type.md)).

## Consequences

- A sink that does not consent is never activated, for any DID method.
- The receiver's gateway needs an unauthenticated `OPTIONS` route next to the authenticated `POST`
  ([ADR 0009](0009-events-routes-as-validated-operator-snippet.md)).
- Each receiver configures every publisher's origin, a second value next to its identifier.
- A per-subscriber cap and the lease expiry still limit what a consenting sink can be sent.

## Alternatives

- **Matching the callback host against a `did:web` domain.** It assumes a DID method
  the connector does not mandate.
- **Accepting an arbitrary callback URL.** It turns the subscription API into an amplifier.
