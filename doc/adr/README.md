# Architecture Decision Records

Short records of decisions that are expensive to reverse or easy to re-litigate. Format is
[MADR](https://adr.github.io/madr/)-lite: context, decision, consequences, alternatives.

| # | Decision | Status | Also in |
|---|---|---|---|
| [0001](0001-oid4vp-for-participant-authentication.md) | Authenticate participants to the consent-manager over OID4VP | Accepted | |
| [0002](0002-reuse-facade-oid4vp-client.md) | Reuse the consent-facade's OID4VP client instead of implementing or importing OID4VP in Go | Accepted | `consent-facade` |
| [0003](0003-token-endpoint-not-consent-proxy.md) | The facade exposes an internal token endpoint, not a consent proxy | Accepted | `consent-facade` |
| [0004](0004-map-oid4vp-tokens-to-participants-by-did.md) | Map OID4VP tokens to participants by DID | Accepted | |
| [0005](0005-vc-based-participant-onboarding.md) | Onboard participants with a verifiable credential, not client credentials | Proposed | |

ADRs 0002 and 0003 constrain the consent-facade's own API, so they are **mirrored** in
[consent-facade](https://github.com/SEAMWARE/consent-facade) under `doc/adr/` with the same numbers. This repository holds the
canonical copy; change it here first.

Design document these belong to: [`../CONSENT_MANAGEMENT.md`](../CONSENT_MANAGEMENT.md).
