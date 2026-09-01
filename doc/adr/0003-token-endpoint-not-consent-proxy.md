# 0003 - The facade exposes an internal token endpoint, not a consent proxy

**Status:** Accepted
**Date:** 2026-08-26
**Applies to:** `FIWARE/data-space-connector`, `wistefan/consent-facade`
**Context doc:** [`../CONSENT_MANAGEMENT.md`](../CONSENT_MANAGEMENT.md)

## Context

Given [ADR-0002](0002-reuse-facade-oid4vp-client.md) - the provider's consent-facade performs the
OID4VP exchange - there are two ways for the consent-plugin to benefit:

- **(a) Proxy:** the plugin sends its consent-manager requests *to* the facade, which authenticates
  and forwards them.
- **(b) Token service:** the facade hands the plugin an access token; the plugin keeps calling the
  consent-manager directly.

The plugin makes two consent-manager calls per gated request (`/users/identifier/search` and
`/consents/participants/{id}?receipt=true`), so this sits on the data path.

## Decision

**(b).** The facade exposes `POST /internal/tokens`, taking a **named audience** and returning a
cached, short-lived access token:

```
POST /internal/tokens   { "audience": "consent-manager" }
200                     { "access_token": "…", "token_type": "Bearer", "expires_in": 3540 }
```

Two constraints are part of the decision:

1. **The audience is a configured name, never a caller-supplied URL.** The facade resolves it
   against a configured target map (url + OID4VP `client_id` + `scope`); an unknown name is a `400`.
   Accepting a URL would let anything that reaches the endpoint make the facade present the
   provider's credential to an arbitrary host - a signed VP naming the provider as holder, leaked on
   request.
2. **`/internal/**` is never published.** The facade ingress allow-lists only `/participants` and
   `/catalog`; this path must stay off it, and access is further restricted by NetworkPolicy to the
   APISIX pods.

## Consequences

**Positive**

- The consent decision path stays direct: the facade is involved only on token refresh, not on every
  request, so it is not in the per-request latency or availability path.
- The facade does not re-expose or track consent-manager API surface, and cannot drift from it.
- Cleanly separated concerns: the facade owns credentials, the plugin owns the consent decision.
- The plugin's existing cache/coalescing structure is reused verbatim.

**Negative**

- The plugin still needs to handle bearer tokens (obtain, cache, refresh on `401`) - a proxy would
  have hidden that. This is exactly what its `credentials()` already does.
- Two components must agree on the audience name.

**Design notes**

- Cache per audience, refresh shortly before expiry, coalesce concurrent misses so a request burst
  triggers one presentation.
- Distinguish retryable from terminal failures, because the plugin's fail-closed behaviour depends on
  it: verifier unreachable or `vp_token` grant not advertised ⇒ `502`; credential rejected (not in
  the TIR, expired) ⇒ `403`; unknown or blank audience ⇒ `400`; this facade's own OID4VP setup broken
  ⇒ `500` (a server fault, not a caller error).
- The existing `Oid4VpAuthHandler` is deliberately left alone: it authenticates *reactively* on a
  `401` while proxying TM Forum calls, which is the wrong shape for "give me a token now".

## Alternatives considered

- **(a) Proxy all consent calls through the facade.** Rejected: it puts the facade in every gated
  request, and it means re-implementing the consent-manager's request/response surface inside the
  facade and keeping it in sync - for no gain, since the plugin must keep the consent semantics
  either way.
- **Inject the token at the provider's APISIX instead.** APISIX plugins act on inbound requests;
  there is no outbound-credential hook for a call the external plugin itself makes.
- **Write the token to a shared file/secret on a timer.** Avoids the request hop but adds a
  distribution mechanism, a staleness window, and a secret at rest for something that lives an hour.
