# 0006 - Validate eIDAS certificate chains inside VCVerifier, not in an external DSS service

**Status:** Accepted
**Date:** 2026-09-28
**Context doc:** [`../deployment-integration/eidas/README.md`](../deployment-integration/eidas/README.md)

## Context

Until chart 10.8.0 the connector proved eIDAS 2.0 compliance by shipping the
`dss-validation-service` alongside the verifier. VCVerifier held no trust material of its own: for
every `did:elsi` credential it posted the signature to the DSS sidecar over
`verifier.elsi.validationEndpoint`, and the sidecar answered valid/invalid against keystores and CRL
snapshots that the deployment had to mount, populate and keep fresh.

That arrangement cost a Deployment, a Service, keystore and truststore Secrets, and a CRL-update
sidecar or CronJob per participant. The trust material was deployment-local, so "which CAs do we
trust" was an operator's mount rather than a published EU list, and keeping the CRLs current was the
operator's problem.

VCVerifier 6.22.0 (`decentralized-iam` >= 2.1.23) does the job itself: it fetches the EU List of
Trusted Lists, parses the national Trusted Lists it points at, builds a trust store from the
Qualified Trust Service Providers it finds, and validates the `x5c` chain of an incoming credential
against it with standard PKIX chain building, consulting OCSP/CRL separately.

## Decision

Use VCVerifier's built-in trust-list validation. Deprecate `decentralizedIam.vcAuthentication.dss`
and stop deploying the DSS sidecar in the shipped overlays.

`...vcverifier.deployment.eidas.enabled` builds the trust store. Two mechanisms then consult it:

- the **`did:elsi` proof check**, triggered by the issuer DID alone, for any credential format; and
- **`EidasValidationService`**, triggered by `eidasConfig.enabled` on a credential type, SD-JWT only,
  and the only source of per-type country and qualified-service filters.

`did:web` and HTTPS issuers resolve their signing key from the DID document or JWKS and their `x5c`
header is ignored by the proof check, so for them the second mechanism is the only one available.

The shipped overlays issue `dc+sd-jwt` with `eidasConfig`, so both mechanisms apply.

## Consequences

**The trust anchor is the published EU list, not a mounted keystore.** What the verifier trusts is
whatever the LOTL says at the last refresh, which is the property the regulation is about. It also
means the verifier now depends on reaching `ec.europa.eu` and every national list host at startup,
so an air-gapped or proxied deployment has to allow that egress — the local overlay exempts the
trust-list host from the squid proxy for the same reason.

**A non-`did:elsi` deployment can be silently non-validating.** For `did:web` and HTTPS issuers the
per-type `eidasConfig` is the only trigger and its absence is a pass-through, so enabling just the
global block fetches the lists forever and consults them never, with nothing failing to indicate it.
`did:elsi` deployments are not exposed to this — the proof check runs on the issuer DID alone — but
they also get no per-type filters. This asymmetry is the main operational hazard of the design, and
it is why the shipped overlay carries a scope whose country filter matches no trust service and the
integration test asserts that the same credential is refused there. A deployment that cannot
demonstrate a refusal has not demonstrated that mechanism.

**SD-JWT becomes mandatory for the per-type mechanism.** `EidasValidationService` rejects any other
format outright rather than skipping the check, so `jwt_vc_json` is not an option for a credential
type with `eidasConfig` enabled. A `did:elsi` deployment may stay on `jwt_vc_json` and rely on the
proof check alone; anything wanting per-type filters, or any non-`did:elsi` issuer, must migrate the
format and switch its DCQL from `meta.type_values` to `meta.vct_values`.

**Rollback is a dependency pin, not a values change.** vcverifier 4.13.0 removed
`verifier.elsi.validationEndpoint` entirely, so reverting to DSS requires pinning
`decentralized-iam` below 2.1.23.

**Trust-list freshness becomes a runtime concern.** A failed initial fetch is not retried before the
next refresh interval, whose floor is one hour, and the verifier stays healthy and serving with an
empty trust store throughout. Deployments need to watch the `TrustListFetcher` log lines rather than
the readiness probe, and local deployments have to order the verifier behind whatever serves the
list.

**Less to deploy and nothing to rotate.** One Deployment, one Service, two Secrets and a CRL job per
participant disappear, and revocation moves to live OCSP/CRL lookups with a cache instead of
periodically refreshed snapshots.

## Alternatives considered

- **Keep the DSS sidecar.** It works and it is a known quantity. Rejected because the trust material
  stays deployment-local — the thing eIDAS conformance is supposed to anchor in a published list —
  and because upstream has already removed the configuration key that wires it up, so staying would
  mean pinning `decentralized-iam` indefinitely.
- **Validate globally, with no per-credential-type configuration.** Simpler to configure and
  impossible to leave half-enabled, which is a real advantage given the hazard above. Rejected
  because it is not what upstream implements, and because a dataspace legitimately mixes credential
  types that need eIDAS assurance with ones that do not — forcing the whole verifier into one mode
  would push those onto separate verifier instances.
- **Pre-seed the trust store from a mounted bundle instead of fetching the LOTL.** Removes the
  startup egress dependency and the freshness problem. Rejected for production because it
  reintroduces exactly the operator-curated trust material that motivated the move. It is, in
  effect, what the local overlay's mock LOTL does — acceptable there precisely because a
  locally generated test CA is in no national list and never should be.
