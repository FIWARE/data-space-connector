# 0001 - Authenticate participants to the consent-manager over OID4VP

**Status:** Accepted
**Date:** 2026-08-26
**Context doc:** [`../CONSENT_MANAGEMENT.md`](../CONSENT_MANAGEMENT.md)

## Context

The provider's consent-plugin calls the authority's consent-manager on the data path (a consent
check per request). It authenticates with a participant `client_id`/`client_secret` exchanged at
`POST /participants/login` for an HS256 JWT, which the authority's APISIX validates with `jwt-auth`
before injecting the shared consent key.

This is the only cross-participant hop in the connector that is not OID4VP-authenticated. Verified
against a running deployment, it carries four problems:

- Long-lived static participant secrets, held by a component on the data path.
- The secret, the JWT and the subject's `x-user-key` cross the namespace boundary in cleartext, and
  the plugin does not authenticate the consent-manager at all.
- `/participants/login` is published, unauthenticated and unthrottled.
- The `jwt-auth` consumers exist only as imperative gateway state created by a manual `curl` in the
  demo; they are keyed on a *display name* and share one HS256 secret (the consent-manager's
  `jwtSecret`), so any holder can mint a token as any participant.

Everything needed for the OID4VP alternative is already provisioned: the provider has
`did:web:mp-operations.org` with a did-helper key secret, a `vc-operator` in its namespace, and a
registration at the trust-anchor TIR.

## Decision

Participants authenticate to the consent-manager **over OID4VP**, presenting a credential as their
own holder DID, exactly as every other cross-participant call in the connector does. The
client-credentials flow, the `/participants/login` route and the `jwt-auth` consumers are removed
rather than kept alongside - nothing is released yet, so there is no compatibility obligation.

## Consequences

**Positive**

- One trust model for all cross-participant traffic: holder DID + credential + TIR.
- No participant secret in the provider's data path; the bearer becomes short-lived and
  audience-bound.
- The gateway learns the caller's *verified* identity, which is the precondition for scoping the
  consent-manager's subject-lookup endpoints later.
- Gateway auth config becomes declarative (a route with `openid-connect`) instead of imperative
  consumer state.

**Negative / costs**

- Requires a code change in the Prometheus-X consent-manager (see
  [ADR-0004](0004-map-oid4vp-tokens-to-participants-by-did.md)). Until it lands, the participant
  path cannot be switched over end to end.
- Adds the authority verifier to the availability path of a consent check (mitigated by token
  caching in the token service).
- One more verifier service and credential type to operate.

**Explicitly not fixed**

Authorization inside the consent-manager. The shared consent key still gates
`/users/identifier/search`, so any onboarded participant can still look up any subject at any other
participant. This ADR changes *authentication* only.

## Alternatives considered

- **Keep client credentials, add TLS + mTLS between namespaces.** Fixes the transport but keeps the
  static secrets, the open login endpoint and the undeclared consumers, and leaves the connector
  with two different trust models.
- **Per-participant `jwt-auth` secrets, declared in the chart.** A real improvement over the shared
  secret and much cheaper, but still a secret-distribution problem and still a second trust model.
  Worth doing as a stopgap if OID4VP is blocked upstream for long.
