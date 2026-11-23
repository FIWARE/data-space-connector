# Architecture Decision Records

Short records of decisions that are expensive to reverse or easy to re-litigate. Format is
[MADR](https://adr.github.io/madr/)-lite: context, decision, consequences, alternatives.

| # | Decision | Status | Also in |
|---|---|---|---|
| [0001](0001-oid4vp-for-participant-authentication.md) | Authenticate participants to the consent-manager over OID4VP | Superseded by 0008 | |
| [0002](0002-reuse-facade-oid4vp-client.md) | Reuse the consent-facade's OID4VP client instead of implementing or importing OID4VP in Go | Superseded by 0008 | `consent-facade` |
| [0003](0003-token-endpoint-not-consent-proxy.md) | The facade exposes an internal token endpoint, not a consent proxy | Superseded by 0008 | `consent-facade` |
| [0004](0004-map-oid4vp-tokens-to-participants-by-did.md) | Map OID4VP tokens to participants by DID | Superseded by 0008 | |
| [0005](0005-vc-based-participant-onboarding.md) | Onboard participants with a verifiable credential, not client credentials | Superseded by 0008 | |
| [0006](0006-eidas-validation-in-vcverifier.md) | Validate eIDAS certificate chains inside VCVerifier, not in an external DSS service | Accepted | |
| [0007](0007-test-both-deployment-paths.md) | Test the charts both rendered and applied, and installed as Helm releases | Accepted | |
| [0008](0008-dsc-consent-manager.md) | Replace the Node consent-manager with dsc-consent-manager on PostgreSQL | Accepted | |
| [0009](0009-events-routes-as-validated-operator-snippet.md) | Ship the data space events routes as a validated operator snippet, not as chart routes | Accepted | |
| [0010](0010-cloudevents-over-oid4vp.md) | Carry data space events as CloudEvents authenticated over OID4VP | Accepted | |
| [0011](0011-detect-removal-by-diffing-the-registry.md) | Detect participant removal by diffing the registry, not by instrumenting it | Accepted | |
| [0012](0012-authorize-events-per-publisher-and-type.md) | Authorize events per publisher and type | Accepted | |
| [0013](0013-attribute-local-trust-to-the-dataspace.md) | Attribute local trust to the data space that caused it | Accepted | |
| [0014](0014-cloudevents-webhook-handshake.md) | Confirm event callbacks with the CloudEvents webhook handshake | Accepted | |
| [0015](0015-publisher-in-the-authority-release.md) | Run the data space publisher in the authority's release, reusing its identity | Accepted | |
| [0016](0016-best-effort-stateless-event-delivery.md) | Deliver events best-effort from stateless publishers, and reconcile instead | Accepted | |
| [0017](0017-no-event-payload-signature.md) | Do not sign the event payload; authenticate events at the transport | Accepted | |

ADRs 0002 and 0003 constrain the consent-facade's own API, so they are **mirrored** in
[consent-facade](https://github.com/SEAMWARE/consent-facade) under `doc/adr/` with the same numbers. This repository holds the
canonical copy; change it here first.

Design document ADRs 0001-0005 (superseded, kept for history) and 0008 belong to:
[`../CONSENT_MANAGEMENT.md`](../CONSENT_MANAGEMENT.md). ADR 0006 belongs to
[`../deployment-integration/eidas/README.md`](../deployment-integration/eidas/README.md). ADR 0007 belongs to
[`../deployment-integration/local-deployment/LOCAL.MD`](../deployment-integration/local-deployment/LOCAL.MD). ADRs 0009, 0012
and 0013 belong to [`../eventing/participant-setup.md`](../eventing/participant-setup.md), ADRs 0010 and 0014 to
[`../eventing/specification.md`](../eventing/specification.md), ADRs 0011, 0016 and 0017 to
[`../eventing/architecture.md`](../eventing/architecture.md) and ADR 0015 to
[`../eventing/trust-anchor-setup.md`](../eventing/trust-anchor-setup.md).
