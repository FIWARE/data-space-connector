# 0002 - Reuse the consent-facade's OID4VP client instead of implementing or importing OID4VP in Go

**Status:** Accepted
**Date:** 2026-08-26
**Applies to:** `FIWARE/data-space-connector`, `[consent-facade](https://github.com/SEAMWARE/consent-plugin)`, `[consent-facade](https://github.com/SEAMWARE/consent-facade)`
**Context doc:** [`../CONSENT_MANAGEMENT.md`](../CONSENT_MANAGEMENT.md)

## Context

[ADR-0001](0001-oid4vp-for-participant-authentication.md) requires the provider to obtain an OID4VP
access token for the consent-manager. The component that needs it is the **consent-plugin**, an
APISIX external plugin written in Go. Something in the provider namespace must therefore hold the
holder key, build and sign a verifiable presentation, and exchange it for an access token.

Two facts shape the choice:

1. **The DSC uses the FIWARE/DSBA `vp_token` grant**, not the plain OID4VP presentation flow. The
   holder discovers the verifier's `.well-known/openid-configuration`, checks that
   `grant_types_supported` contains `vp_token`, and POSTs `grant_type=vp_token&vp_token=<signed JWT>`
   to the **token endpoint** to receive an OAuth2 access token. See
   `oid4vp-client-lib` (`OIDConstants.VP_TOKEN_GRANT_TYPE`, `OpenIdConfigurationClient`) and
   [`../scripts/get_access_token_oid4vp.sh`](../scripts/get_access_token_oid4vp.sh).
2. **A Java implementation already exists and is in production use here**:
   `io.github.wistefan:oid4vp-client-lib`, used by the consent-facade
   (`Oid4VpAuthHandler`) to authenticate its TM Forum calls to each provider's OID4VP-protected
   `mp-tmf-api`. The provider already runs its own consent-facade instance.

## Decision

The provider's **consent-facade instance obtains the token** using the existing
`oid4vp-client-lib`, and the consent-plugin asks it for one. No OID4VP protocol code is written in,
or imported into, the Go plugin.

## Consequences

**Positive**

- Zero new protocol implementations; the credential handling lives in one place, in code already
  exercised against a real verifier.
- The plugin change is confined to token *acquisition*. `internal/consent/client.go` already keeps a
  per-key token cache with TTL, refresh coalescing and per-entry locking; only the call that fills
  it changes.
- Holder key material stays out of the APISIX pod.

**Negative**

- The consent-facade becomes a hard dependency of the provider's consent path (for token refresh,
  not per request).
- It also becomes credential-bearing, so it must be reachable only by the plugin - the provider
  namespace needs the NetworkPolicies it currently lacks.

## Alternatives considered

- **A Go OID4VP library.** [`trustbloc/wallet-sdk`](https://github.com/trustbloc/wallet-sdk) and
  [`trustbloc/vcs`](https://github.com/trustbloc/vcs) are the credible options and are actively
  maintained. Rejected on fit, not health: they implement presenting to a verifier's `response_uri`
  (`direct_post`) and OIDC4VCI issuance, i.e. they produce a `vp_token` - not the token-endpoint
  exchange that yields the access token this profile needs. We would get VP construction and still
  write the discovery + `vp_token` grant ourselves, while pulling a large dependency tree into an
  APISIX external plugin.
- **Implement the flow natively in Go.** It is genuinely small - the reference shell script is 49
  lines - but it means a second, independently-maintained implementation of a security protocol,
  diverging from the Java one on every profile change (credential sets, DCQL, algorithms). Rejected
  as an explicit product constraint.
- **A dedicated token-service container** rather than the facade. Same benefits, but a new component
  to build, image, deploy and secure, when the facade already runs in that namespace with the
  library wired in.
