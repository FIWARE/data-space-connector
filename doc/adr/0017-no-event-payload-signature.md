# 0017 - Do not sign the event payload; authenticate events at the transport

**Status:** Accepted
**Date:** 2026-11-20
**Context doc:** [`../eventing/specification.md` §5](../eventing/specification.md#5-authentication-binding)

## Context

An event is authenticated by the way it arrives: the publisher presents a verifiable credential at
the receiver's verifier over OID4VP and uses the returned access token as a bearer; the receiver
checks that token and requires the event's `source` to equal the authenticated identity
([ADR 0010](0010-cloudevents-over-oid4vp.md), [ADR 0012](0012-authorize-events-per-publisher-and-type.md)).

That binds the event to its publisher for the duration of the hop and no longer. A signature over
the envelope — a JWS by the publisher, carried as a CloudEvents extension attribute — would make an
event verifiable on its own: after it was stored, forwarded by a third party, or presented as
evidence that the data space announced something.

## Decision

Events carry **no payload signature**. Authentication is the transport's job.

If a governance framework ever requires non-repudiation of an announcement, a `signature` extension
attribute can be added: a receiver that does not know the attribute ignores it, so the addition is
backwards compatible and needs no flag day.

## Consequences

- No signing key, no key distribution and no signature verification code on either end. The
  publisher needs the key it already has for OID4VP and nothing more.
- An event is only as trustworthy as the hop it arrived on. A receiver cannot prove to anyone else
  that a publisher announced something, and a forwarded or replayed event carries no proof of
  origin.
- That is acceptable because no receiver acts on an event alone in a way it could not undo: the
  default reaction is `report`, and the reconciliation sweep re-derives the same conclusion from the
  registry ([ADR 0016](0016-best-effort-stateless-event-delivery.md)).

## Alternatives

- **A JWS over the envelope in v1.** It solves a problem no governance framework in sight has, and
  every publisher and receiver would carry the cost.
