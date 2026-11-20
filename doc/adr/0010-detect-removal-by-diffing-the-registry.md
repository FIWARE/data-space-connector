# 0010 - Detect participant removal by diffing the registry, not by instrumenting it

**Status:** Accepted
**Date:** 2026-11-20
**Context doc:** [`../eventing/architecture.md` §6](../eventing/architecture.md#6-detecting-the-removal-at-the-trust-anchor)

## Context

A `participant.removed` event has to be emitted when the data space authority removes a member from
the Trusted Issuers Registry (TIR). The operator documentation states that the FIWARE
trusted-issuers-list is *one* TIR implementation and that any EBSI-compatible registry, including
blockchain-backed ones, may be used
([operator guide](../deployment-integration/roles/operator/README.md)). Members can also disappear
without any application call: through a direct database change or a migration.

## Decision

The publisher-mode contract-management at the trust anchor **polls the TIR and diffs it**
(`GET /v4/issuers`, all pages). An identifier that was in the previous snapshot and is missing now
is announced. The registry is not instrumented. Because a diff turns any failure that makes the
registry look empty into a mass removal, these safeguards are part of the decision, not options:

- **Only a complete enumeration becomes a snapshot.** A failed or partial read keeps the previous one.
- **`confirmations`** (default 2): an identifier must be missing for that many consecutive cycles
  before it is announced.
- **`max-removals-per-cycle`** (default 5): more disappearances at once announce nothing and raise an
  alert for a human.
- **The first snapshot after a start is silent.** A fresh publisher announces nothing.

An operator can still announce a removal at once through the publisher's cluster-internal
`POST /events`.

## Consequences

- Detection works for every registry that can be enumerated, and it sees out-of-band changes.
- **Latency** is `interval × confirmations`, about two minutes with the defaults, which is
  appropriate for a governance action that is itself manual.
- A DID that is added and removed between two cycles is never announced, and nothing is announced
  for a removal that happens before the first snapshot. The manual publish covers both; the
  participants' reconciliation sweep is the backstop for any missed event
  ([how it works §9](../eventing/architecture.md#9-reconciliation-the-backstop)).
- The publisher reads the TIR once per `interval` (default 60 s); it needs no write access to the
  registry.
- A legitimate removal of more than `max-removals-per-cycle` members at once needs an operator.

## Alternatives

- **A webhook in the FIWARE trusted-issuers-list.** It would only work for deployments that use that
  implementation and would miss out-of-band changes. It stays possible later as a latency
  optimisation *on top of* the watcher, which then remains the backstop for registries that cannot
  emit one.
