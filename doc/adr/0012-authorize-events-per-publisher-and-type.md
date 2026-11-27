# 0012 - Authorize events per publisher and type

**Status:** Accepted
**Date:** 2026-11-20
**Context doc:** [`../eventing/architecture.md` §4](../eventing/architecture.md#4-trust-and-authorization)

## Context

A receiver's PEP admits the events path to holders of an agreed credential type, through an ODRL
policy. In a normal data space many organizations hold that credential type; in a typical
configuration every participant does. A `participant.removed` event deletes trust configuration at
the receiver, so whoever may send one can have a competitor's trust entries deleted.

## Decision

On top of the PEP, contract-management authorizes every event in the application, against its
configured `publishers[]`:

- the event's `source` must equal the authenticated identity
  ([specification §5.2](../eventing/specification.md#52-caller-identity));
- that publisher must be configured for the event's `type`. Otherwise the receiver answers `403`
  ([specification §6](../eventing/specification.md#6-authorization-rule), check 4);
- an event about the receiver itself is recorded and never acted on.

A publisher is configured with its **identifier**, which authorizes the event, and its **origin**, a
DNS name used by the webhook handshake ([ADR 0014](0014-cloudevents-webhook-handshake.md)). The two
are configured together and never derived from each other. The list is runtime configuration: it is
reloaded from the mounted ConfigMap without a restart.

## Consequences

- No data space member can announce events about another one, whatever credential the PEP admits.
- Each participant decides which publishers it listens to and for which types; adding a second
  data space is a configuration change.
- The publisher × type check needs the identity the trust check binds to reach the application
  unchanged. The verifier's token mapping is therefore part of the security boundary
  ([trust anchor setup](../eventing/trust-anchor-setup.md#who-a-subscriber-is)).
- The check is skipped for an event type the receiver does not know: a complete event of an
  unknown type is accepted and ignored (`202`), so a data space can introduce a type without a
  flag day.

## Alternatives

- **Relying on the ODRL policy alone.** A policy constrains the credential *type*, so it cannot say
  "only this publisher". An ODRL constraint on the credential's issuer (ODRL-PAP `vc:currentParty`)
  pins the issuer, not the holder; it is kept as defence in depth only. The k3s policy
  `allowDataSpaceEvents.json` uses it that way.
