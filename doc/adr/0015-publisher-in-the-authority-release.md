# 0015 - Run the data space publisher in the authority's release, reusing its identity

**Status:** Accepted
**Date:** 2026-11-20
**Context doc:** [`../eventing/trust-anchor-setup.md`](../eventing/trust-anchor-setup.md#where-it-runs)

## Context

The data space publisher is a contract-management (CM) instance in publisher and TIR-watcher mode
([ADR 0010](0010-cloudevents-over-oid4vp.md), [ADR 0011](0011-detect-removal-by-diffing-the-registry.md)).
It needs:

- an identity the participants trust: a DID, its key and a credential it presents to every
  participant's verifier over OID4VP before it delivers an event;
- a verifier, at which subscribers get their token for `POST /subscriptions`;
- an API gateway in front of `/subscriptions`, because CM verifies no tokens and only reads the
  `X-Userinfo` header the gateway sets.

The `trust-anchor` chart runs the registry (TIR and TIL) and has none of these. The data space
authority already runs a DSC release of its own (in k3s: `consent-authority`, namespace
`trust-anchor`) that holds the identity `did:web:dataspace-authority.org`, its key, an
OperatorCredential, a verifier and APISIX. Its `contract-management` is disabled.

## Decision

The publisher is the `contract-management` of the **authority's DSC release**. In k3s,
[`k3s/eventing-trust-anchor.yaml`](../../k3s/eventing-trust-anchor.yaml) enables it in the
`consent-authority` release, with every CM integration off, the `dataspace-subscriptions` verifier
service and the gateway routes for `/subscriptions`. It reuses the authority's DID, key and
OperatorCredential; no new identity is registered. The `trust-anchor` chart is not changed.

## Consequences

- No new key, credential or DID at the trust anchor. Each participant's TIL must still list the
  authority as issuer of `OperatorCredential`, so that its verifier admits the deliveries.
- The publisher shares the authority release's lifecycle: an upgrade of that release can restart it,
  which drops its in-memory subscriptions until the subscribers renew them
  ([trust anchor setup](../eventing/trust-anchor-setup.md#timing-production-defaults-and-the-k3s-values)).
- The authority's gateway route list and verifier service list must carry the publisher's entries.
  Helm replaces lists, so an overlay copies them whole.
- In k3s the release also runs the consent-manager stack, because the eventing demo builds on the
  consent deployment; a production publisher does not need it.

## Alternatives

- **A dependency in the `trust-anchor` chart.** It would mix the publisher's identity, its verifier
  and its gateway into the registry's chart, and the ticket requires the publisher to stay out of it.
- **A dedicated release with its own verifier and gateway.** It would duplicate both, or route
  across releases, for no gain: the authority release already holds the identity and both
  components.
