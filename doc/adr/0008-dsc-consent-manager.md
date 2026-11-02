# 0008 - Replace the Node consent-manager with dsc-consent-manager on PostgreSQL

**Status:** Accepted
**Date:** 2026-11-02
**Applies to:** `FIWARE/data-space-connector`, [dsc-consent-manager](https://github.com/wistefan/dsc-consent-manager), [consent-plugin](https://github.com/SEAMWARE/consent-plugin)
**Supersedes:** [0001](0001-oid4vp-for-participant-authentication.md), [0002](0002-reuse-facade-oid4vp-client.md), [0003](0003-token-endpoint-not-consent-proxy.md), [0004](0004-map-oid4vp-tokens-to-participants-by-did.md), [0005](0005-vc-based-participant-onboarding.md)
**Context doc:** [`../CONSENT_MANAGEMENT.md`](../CONSENT_MANAGEMENT.md)

## Context

ADRs 0001-0005 bolted OID4VP onto the Prometheus-X consent-manager (Node, MongoDB). That service
authenticates its participant-side endpoints with one shared secret, the consent key, so the
authority had to run an APISIX facade that validated the caller's OID4VP token and then injected the
key. The key carried no caller identity, and the open gaps listed in the old design (identity lookups
across participants, the bearer-equivalent user key, unauthenticated onboarding, client secrets stored
on participant records) all needed changes in a codebase we do not own.

`dsc-consent-manager` (Java, PostgreSQL, published as the
`oci://quay.io/fiware/helm-charts/dsc-consent-manager` chart) is a stateless OAuth2 resource server.
It validates the JWT of a configured identity provider on every call and takes the caller's identity
and role from the token's claims. Its `/v1` API scopes every read to the caller, signs consent
receipts, and derives privacy notices from the same contract-service API the consent-facade serves.

## Decision

- The trust-anchor chart (2.0.0) deploys `dsc-consent-manager` as the `consent-manager` subchart
  alias, on the chart's Zalando `managedPostgres` (fixed user and database `consent_manager`). The
  consent MongoDB, the consent key and the authority APISIX facade routes are removed. The authority
  gateway only publishes the host and does not authenticate.
- Every caller authenticates with a JWT from the authority VCVerifier service `consent-manager`
  (audience `consent-manager`), which the consent-manager trusts as `identityProviders.primary`. The
  verifier copies the credential's `type` array to `roles` and the MembershipCredential `issuer` to
  `participant_id`. `roleMapping` turns `MembershipCredential` into `PARTICIPANT` and
  `UserCredential` into `USER`. The verifier already enforces the credential type per scope and
  checks the issuer against the TIR, so no participant can grant itself a role.
- Participants register themselves (`POST /v1/participants`); the identifier is the DID in the
  token, never the body. Data subjects register themselves (`POST /v1/users/register`); their
  identifier is the token's `sub`, the holder DID.
- A plain (non-hook) setup Job in the trust-anchor chart is the single owner of the two Secrets the
  service reads: `consent-manager-env` (DB credentials copied from Zalando, log-pseudonym and
  erasure keys, SMTP credentials) and `consent-manager-signing-keys` (the receipt signing JWK). It
  creates keys once and never rotates them. Nothing uses `lookup`, so `helm install` and
  `helm template | kubectl apply` behave the same ([ADR 0007](0007-test-both-deployment-paths.md)).
- The consent-facade keeps both of its roles: at the authority it is the consent-manager's
  `config.contractServiceUrl`; at the provider it is the plugin's token service
  (`POST /internal/tokens`). The decisions of 0002 and 0003 carry over unchanged in that respect.
- The provider's consent-filter plugin 1.0.0 calls the `/v1` API directly with that token. It
  resolves the provider via `GET /v1/participants/me`, requires the consumer DID to be a registered
  participant, and allows only a consent that is `consented` with matching provider and consumer.

## Consequences

- No shared secret is left in the consent path, and each answer is scoped to the authenticated caller.
- Existing consent data is not migrated: MongoDB records have to be re-granted under the new service.
  The trust-anchor chart takes a major bump (2.0.0) and `fail`s on the removed values. The DSC chart
  only changes docs and comments (10.16.0).
- External Postgres is not supported in trust-anchor 2.0.0: the setup Job depends on the Zalando
  secrets.
- The plugin and its route config must be upgraded together (1.0.0 drops `consent_api_host` and
  the key options).
- The consent-manager looks contract parties up by participant DID, while contract-management writes
  self-description URLs into an agreement's `provider-id` / `consumer-id`. Privacy-notice generation
  from a marketplace purchase therefore fails until one side maps between them (an upstream fix).
- VCVerifier 6.27.0 does not compare a `jwt_vc_json` credential's `vc.issuer` with the signed `iss`,
  so a hand-crafted VC can still name another DID as `participant_id` until upstream rejects that
  mismatch.

## Alternatives considered

- **Keep the Node consent-manager and close its gaps upstream.** Every gap needed a change in a
  codebase outside the project, and the shared key would remain the trust root of the facade.
- **Keep an authenticating gateway in front of the new service.** It would duplicate the JWT check
  the service already performs and reintroduce a second place for the auth configuration.
- **Generate the Secrets with Helm `lookup`.** `lookup` is empty under `helm template`, so keys would
  rotate on every render of the GitOps / CI path.
