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
| [0006](0006-eidas-validation-in-vcverifier.md) | Validate eIDAS certificate chains inside VCVerifier, not in an external DSS service | Accepted | |
| [0007](0007-test-both-deployment-paths.md) | Test the charts both rendered and applied, and installed as Helm releases | Accepted | |
| [0009](0009-config-tool-stack.md) | Build the configuration tool as a Go core with an embedded web UI, in this repository | Proposed | |
| [0010](0010-config-tool-secret-handling.md) | The configuration tool creates secrets, but never stores their values | Proposed | |
| [0011](0011-config-tool-window-build-tag.md) | Release binaries of the configuration tool open a browser tab; the native window is an opt-in build tag | Proposed | |
| [0012](0012-config-tool-semver-library.md) | Order the configuration tool's model bundles with Masterminds/semver, the library Helm uses | Proposed | |
| [0013](0013-config-tool-roundtrip-byte-patches.md) | Round-trip values.yaml through byte patches over the source, with yaml.v3 only as the index | Proposed | |
| [0014](0014-config-tool-load-classification.md) | Load an existing values.yaml through owned markers, data-driven recognition rules and a load report | Accepted | |
| [0015](0015-config-tool-profile.md) | Keep the configuration tool's state in a deterministic, secret-free profile, saved and opened by absolute path | Proposed | |
| [0016](0016-config-tool-local-trial.md) | Ship the local trial as bundled Quick Start values with a trial descriptor and hard-coded production warnings | Proposed | |

ADRs 0002 and 0003 constrain the consent-facade's own API, so they are **mirrored** in
[consent-facade](https://github.com/SEAMWARE/consent-facade) under `doc/adr/` with the same numbers. This repository holds the
canonical copy; change it here first.

Design document ADRs 0001-0005 belong to:
[`../CONSENT_MANAGEMENT.md`](../CONSENT_MANAGEMENT.md). ADR 0006 belongs to
[`../deployment-integration/eidas/README.md`](../deployment-integration/eidas/README.md). ADR 0007 belongs to
[`../deployment-integration/local-deployment/LOCAL.MD`](../deployment-integration/local-deployment/LOCAL.MD). ADRs 0009 to
0016 belong to [`../config-tool/`](../config-tool/README.md).
