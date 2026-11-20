# 0015 - Deliver events best-effort from stateless publishers, and reconcile instead

**Status:** Accepted
**Date:** 2026-11-20
**Context doc:** [`../eventing/architecture.md` §5](../eventing/architecture.md#5-delivery)

## Context

A publisher pushes events to the sinks its subscribers registered. A subscriber may be down, its
gateway may answer `503`, the publisher may be restarted in the middle of a delivery. The usual
answer is a durable outbox: the publisher writes every event to a database, delivers from there, and
keeps a retained event log a subscriber can catch up from.

contract-management, which hosts both ends of the mechanism, has **no database at all** today: no
datasource, no Liquibase, nothing to migrate or back up. An outbox would add one, plus a retention
and audit policy for the event log.

For `org.fiware.dataspace.participant.removed` the authoritative state is the data space's Trusted
Issuers Registry. It is public, it can be read at any time, and a participant's verifier already
polls it for every credential validation.

## Decision

Delivery is **at-least-once and best-effort**, and the publisher keeps no durable state:

- a failed delivery is retried in memory with exponential backoff and a bounded attempt count, then
  dropped with an error log and a metric;
- subscriptions are **leased and held in memory**. Subscribers renew them on a timer, and a
  publisher restart drops them until the subscribers re-register;
- there is no catch-up feed (`GET /events?since=`), no retained event log, no broker and no queue;
- receivers deduplicate on (`source`, `id`) in a bounded, time-limited in-memory cache, so a
  duplicate costs nothing and a cache lost in a restart is harmless.

The guarantee that makes this safe is **reconciliation**, not delivery: every receiver periodically
compares the trust it holds for each publisher against that publisher's registry and applies the
same reaction to whatever is no longer listed
([architecture §9](../eventing/architecture.md#9-reconciliation-the-backstop),
[ADR 0010](0010-detect-removal-by-diffing-the-registry.md)).

An event type whose content **is** the state, and which therefore cannot be reconciled from an
authoritative source — the reserved `service.configuration.provided` is the example — must not be
carried this way. Introducing one is the point at which a durable outbox is added. The wire contract
does not change when that happens, which is why the publisher owns its `source` and its `id` space
from the start.

## Consequences

- contract-management stays stateless, on both ends.
- A lost event costs latency, not correctness: the sweep reaches the same state. A participant
  installed after a removal happened converges too, without a catch-up feed.
- A deployment is never *dependent* on the push path being reachable, which is what lets the
  publisher stay stateless in the first place.
- Nobody can prove that a removal notice was delivered. No governance framework in sight asks for
  that.
- Self-service registration of publishers by a participant would need the publisher list to be
  persisted, so it is out of reach until an outbox exists.

## Alternatives

- **A persisted outbox and subscription registry.** Correct, and required later for event types that
  cannot be reconciled. Today it buys a guarantee nobody needs, at the price of giving a stateless
  component a database.
- **A catch-up feed at the publisher.** It needs the same retained log, and it answers a question —
  "which participants are gone?" — that the registry already answers directly and authoritatively.
- **A broker or queue.** Nothing in the connector operates one.
