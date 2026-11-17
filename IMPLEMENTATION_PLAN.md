# Implementation Plan: [DSC] Chart: trust anchor side, k3s demo, integration test, docs and ADRs

## Overview
This is the closing ticket of the data space eventing epic (#97). It deploys the event publisher, a
contract-management (CM) instance in publisher + TIR-watcher mode, in the trust anchor's namespace.
It wires listener + subscriber + routes + policy at provider and consumer in `k3s/`, proves the 7
ticket scenarios in a Cucumber feature run by a new `eventing` CI scenario, and writes the operator
docs, release notes and ADRs. No chart template is expected to change. The `trust-anchor` chart is
not touched.

**Base:** `ticket-106/work` = `ticket-97/work` @ `497fd98` (chart 10.16.0, ticket-102 merged:
`contract-management.dataspace.events`, `registration.issuer` scope derivation,
`templates/eventing-validation.yaml`, `doc/eventing/participant-setup.md`, ADR 0008,
`it/src/test/resources/policies/allowDataSpaceEvents.json`, CM chart 3.6.6 / CM 3.5.3,
decentralized-iam 2.3.0 / TIL 0.10.3).

**Sources** (read-only, clone under `/tmp`):
- `doc/eventing/architecture.md` §8–§12 and `doc/eventing/specification.md`.
- `git clone https://github.com/FIWARE/contract-management /tmp/cm` (tag 3.5.3 @ `c7da3c4`), especially:
  - `README.md`: 120–330 for the listener, publisher and subscriber; 375–527 for full configs; 550–700 for modes and the report;
  - `DataSpaceEventsProperties.java`;
  - `RemovedParticipantsEndpoint.java`.
- `https://github.com/FIWARE/helm-charts` `charts/contract-management` 3.6.6.
- `https://github.com/FIWARE/trusted-issuers-list` `api/trusted-issuers-list.yaml`.

**CM facts the plan relies on** (verified at 3.5.3; property names are kebab-case under
`dataspace.events`):
- **Startup-only keys.** These are the `enabled` flags of `listener`, `publisher`, `subscriber` and
  `tir-watcher`, plus `publisher.sink-*-timeout`. Everything else, including `publishers[]` and its
  `on-participant-removed.{mode, keep, overrides}`, reloads from the mounted
  `<fullname>-dataspace-events` ConfigMap through the file watch.
- **Removal modes** are `report`, `dataspace`, `granted` and `full`. Any mode other than `report` needs
  `on-participant-removed.registry-url`; without it the mode is clamped to `report` and startup fails.
- **TIR watcher.** It is configured under `tir-watcher.{initial-delay, interval, confirmations,
  max-removals-per-cycle}` and has **no URL key**. It reads `GET /v4/issuers` from the Micronaut HTTP
  service `trusted-issuers-registry`, so the URL goes in the CM chart's `services:` map. The first
  complete cycle only takes a snapshot. A removal is announced after `confirmations × interval`.
  A DID only counts as removed if it was in an earlier snapshot. A DID that is added and removed
  between two cycles is never announced. The next cycle is scheduled `interval` after the previous
  one ends. The `dataspace-tir-watcher` health indicator reports `state`, `lastSuccessfulCycle`
  (the end time of the last successful cycle), `firstCycleDue`, `snapshotSize`, `pendingSize` and
  `undeliveredSize` (`TirWatcherHealthIndicator`).
- **Reconciliation.** It is configured under `participant-removed.reconcile.{initial-delay, interval}`,
  not per publisher. It runs once after `initial-delay`, then every `interval`. There is **no trigger
  endpoint**.
- **Management port 9090.** It serves:
  - `POST /events` `{type, subject}`, which publishes manually and returns `202 {"id","subscriptions":n}`;
  - `GET /participants-removed`, the report: a list of `{publisher, mode, …, participants:[{id,
    trigger, mode, action, applied, attributable, held:[{scope, scopeType, credentialCount}],
    unattributedCount}]}`. `action` is a `RemovalAction` (`REPORTED`, `KEPT`,
    `REVOKED_MEMBERSHIP`, …) and `trigger` is a `RemovalTrigger` (`EVENT`, `RECONCILIATION`), both
    as lower-case tags. The report does **not** show `keep`;
  - `POST /refresh`;
  - `GET /health`. It shows indicator details only if `endpoints.health.details-visible` is
    `ANONYMOUS` (the default is `AUTHENTICATED`). Set that with the env var
    `ENDPOINTS_HEALTH_DETAILS_VISIBLE=ANONYMOUS`.

  The CM Service exposes 8080 only.
- **CM verifies no tokens.** `/events` and `/subscriptions` only read `X-Userinfo.participantId`, so
  the publisher's `/subscriptions` needs the same gateway pattern (openid-connect +
  `set_userinfo_header` + `X-Userinfo` strip) as a participant's `/events`.
- **Publisher-only CM.** Turn the integrations off: `enableTmForum`, `enableRainbow`, `enableOdrlPap`,
  `enableTrustedIssuersList` and `enableCentralMarketplace` all `false`. Two more pitfalls:
  - The chart's `notification.enabled: false` does **not** turn notifications off: the chart omits
    the block and CM defaults to `true`, so the health check goes DOWN. Set env
    `NOTIFICATION_ENABLED=false`. Helm replaces lists, so copy the whole default
    `contract-management.additionalEnvVars` list (the OTEL entries) and add the entry to it.
  - CM has no DB and keeps all state in memory, so run a single replica.
- **Leases and re-subscription.** The publisher keeps subscriptions in memory: a restart loses them.
  A subscription lives for `publisher.lease-ttl` (default `PT24H`). The subscriber renews it after
  `subscriber.renewal-fraction` (0.5) of the remaining lease, never sooner than `min-renewal-delay`
  (`PT10S`). Its 1 min reconcile backstop only reacts to changes in its *own* `publishers[]`, so it
  does not notice that a restarted publisher lost the subscription: only the next renewal re-registers
  it. A failed registration is retried from `subscriber.retry.initial-backoff` (`PT5S`), doubling up
  to `retry.max-backoff` (`PT5M`), without an attempt limit.
- **OID4VP.** `oid4vp.{enabled, holder.{holderId, keyType, keyPath, signatureAlgorithm},
  credentialsFolder, trustAnchors, proxyConfig}`. The chart has no secret-mount value, so mount the
  key and the credential with `deployment.additionalVolumes`/`additionalVolumeMounts`. The subscriber
  needs `subscriber.protocol-settings.client-id` (required at start-up; the chart's own example
  lacks it).
- **TIL API.**
  - `POST /issuer?scope=&scopeType=dataspace-membership` creates an issuer with a scoped grant.
  - `GET /issuer?scope=&scopeType=` lists issuers by scope.
  - `GET /issuer/{did}/scopes` returns an issuer's grants as `{scope, scopeType, credentialCount}`,
    without the credentials.
  - `GET /issuer/{did}` returns `{did, credentials[]}`. Each entry is a `{credentialsType, validFor,
    claims}` with no scope, so the list merges every grant and the unscoped credentials.
  - `PUT /issuer/{did}/credential?scope=&scopeType=` (`replaceCredentialsByScope`) **sets** the scope's
    credentials to exactly the body and leaves other grants alone. It creates the issuer if it does
    not exist, so it is the only write the IT needs. `POST /issuer?scope=` 409s on an existing issuer.
  - `DELETE /issuer/{did}` removes the issuer.

  The trust anchor removes a participant with `DELETE /issuer/{did}` on its TIL
  (`https://til.127.0.0.1.nip.io` in k3s); the TIR (`https://tir.127.0.0.1.nip.io`) is read-only.

**Repo facts** (k3s / IT):
- **The identity to reuse.** It lives in the `consent-authority` release (DSC chart, ns `trust-anchor`,
  values `k3s/consent-trust-anchor.yaml`, deployed by the `consent` profile in the root `pom.xml`,
  ~508–567, and by `k3s/deploy.sh`):
  - DID `did:web:dataspace-authority.org`, key secret `dataspace-authority.org-tls`;
  - an OperatorCredential, kept by vc-operator in secret `vc-operator-credential` (key `credential`,
    Keycloak client `contract-management`);
  - verifier `verifier.dataspace-authority.org`, and APISIX in traditional mode + etcd;
  - `contract-management.enabled: false` (line 283);
  - its APISIX reads the `consent-manager` secret through `extraEnvVars` (~608–613). That secret is
    created by the consent-manager chart in the `trust-anchor` release, through
    `k3s/consent-trust-anchor-components.yaml` (header lines 13–15). Without it the APISIX pod stays
    in `CreateContainerConfigError`;
  - its lists, which an overlay replaces as a whole:
    - the APISIX `routes` (~614–769: the consent-manager routes and `/consent-manager/*`);
    - the authority verifier's `registration.services` (`decentralizedIam.vcAuthentication.vcverifier`,
      ~498–562, holding `consent-manager` at ~523).
- **Maven profiles.** The `template-consent-authority` execution is declared inside the `consent`
  profile's `<build>` (`pom.xml` ~531–546). Another profile cannot reference it. When several
  profiles are active, Maven merges them in POM declaration order: for a property both set, the
  profile declared later wins, and executions with the same id are merged.
  `deploy.gaiaxInfra.skip` (~58, ~628) is the existing pattern for an execution in the main build that
  a profile switches on: a skip property that defaults to `true`, set by the profile to
  `${deploy.template.skip}`.

  `allowDataSpaceEvents.json` already admits exactly that credential (OperatorCredential,
  `vc:currentParty` = `did:web:dataspace-authority.org`).
- **The provider's credential.** It holds a MembershipCredential in secret `vc-fdsc-edc-credential`
  (`k3s/provider.yaml` ~1726). `k3s/consent-provider.yaml` ~248–300 shows the OID4VP holder wiring
  that presents it to the authority's verifier; reuse that pattern for the subscriber.
- **Provider CM.** It is enabled in `k3s/provider.yaml` ~806–855 and runs behind the APISIX host
  `contract-management.127.0.0.1.nip.io`: a `/*` route ~625, a discovery `/*`+`vars` route ~445 and
  verifier service `contract-management` ~320.
- **Consumer CM.** It is off in `consumer.yaml` and on in `consumer-tmf.yaml`, with the OID4VP holder
  `did:web:fancy-marketplace.biz`.
- **Provider `registration.issuer`** (~1685–1706) holds mp-operations, fancy-marketplace
  (LegalPerson, User, Membership) and dataspace-authority (OperatorCredential). Helm replaces lists, so
  an overlay must copy the whole list.
- **In-cluster DNS.** `*.127.0.0.1.nip.io` resolves to Traefik in-cluster (`k3s/infra/coredns`), so a
  sink such as `https://contract-management.127.0.0.1.nip.io/events` is reachable from the trust
  anchor's pods. `*.dataspace-authority.org` / `*.mp-operations.org` hosts go through Squid
  (`squid-proxy.infra.svc.cluster.local:8888`).
- **IT harness** (`it/src/test/java/org/fiware/dataspace/it/components/`):
  - `RunCucumberTest` is the runner. Features are selected by `cucumber.filter.tags` per `*-test`
    profile in `it/pom.xml`.
  - `StandardStepDefinitions` `@Before("not @consent and not @eidas")` (:111) wipes the provider TIL
    and PAP, so the new tag must be excluded there. The same hook also does the per-scenario init
    the order flow needs: `CryptoIntegration.init`, the BouncyCastle provider and a fresh
    `fancyMarketplaceEmployeeWallet`. `cleanUpTIL` and `cleanUpPolicies` delete **every** entry, not
    only the test's.
  - The IT uses plain cucumber-java, with no DI container. Each glue class is instantiated once per
    scenario, so a feature may call steps of any glue class, and each class's fields hold that
    scenario's state (e.g. `fancyMarketplaceRegistration`, `createdPolicies`).
  - The order flow (`mvds_basic.feature`, scenario 2) is built by these `StandardStepDefinitions`
    steps: `M&P Operations offers a managed kubernetes.` (TMF category, catalog, spec with an
    OperatorCredential `credentialsConfig`, offering), `… allows self-registration of organizations.`
    and `… allows to buy its offerings.` (PAP policies, tracked in `createdPolicies`), `Fancy
    Marketplace issues a user credential to its employee.` (consumer Keycloak → wallet), `Fancy
    Marketplace registers itself at M&P Operations.` (sets `fancyMarketplaceRegistration`) and
    `Fancy Marketplace buys access to M&P's k8s services.` (`buyAccess`).
  - Helpers:
    - `KubernetesHelper` (`portForward`, k8s client using `it/target/k3s.yaml`/`KUBECONFIG`);
    - `TestUtils.OK_HTTP_CLIENT` (via Squid);
    - `MPOperationsEnvironment.TIL_DIRECT_ADDRESS`;
    - `TrustAnchorEnvironment.TIR_ADDRESS`;
    - `StandardStepDefinitions.buyAccess` (:577), which creates a TMF productOrder and completes it.
  - `ConsentStepDefinitions` is the closest model for a profile with its own release and port-forwards.
- **CI.** `.github/workflows/test.yaml` runs a matrix of scenario × {template, helm-install} with
  `mvn clean integration-test -Ptest,<scenario>,<scenario>-test[,helm-install]`.

**Decisions taken by this plan:**
- **D1: where the publisher runs.** It is the `contract-management` of the authority release
  (`consent-authority`), enabled by a new overlay `k3s/eventing-trust-anchor.yaml`. The release is
  separate from the `trust-anchor` chart, as the ticket requires. It already owns the identity
  (DID, key, credential), the verifier and the APISIX the publisher's `/subscriptions` needs. A
  second release would have to duplicate verifier and gateway, or route across releases, for no
  gain. Documented in the operator guide and ADR 0014.
- **D2: switching modes in the IT.** The IT patches the provider's `contract-management-dataspace-events`
  ConfigMap (refreshable keys only) and polls `GET :9090/participants-removed` until the publisher's
  `mode` shows the new value (kubelet sync + watch, ≤ ~2 min). An `@After` hook restores the
  original. This needs no helm upgrade and no restart, and it also exercises the refresh path.
- **D3: blocking the push path (scenario 6).** The IT scales the publisher Deployment to 0, deletes
  the participant, then restarts the provider CM. The sweep runs once at start-up, after the k3s
  `reconcile.initial-delay`. The k3s overlay sets the reconcile `interval` long (`PT6H`), so no sweep
  races the push scenarios. The report's `trigger` tells event from sweep (check the enum values in
  `RemovedParticipantsEndpoint`/the report model).

  **Recovery.** Scaling the publisher down drops all subscriptions, and with CM's defaults they come
  back only at the next lease renewal, ~12 h later. The k3s values therefore shorten both clocks:
  - the publisher's `lease-ttl` is `PT2M` (`k3s/eventing-trust-anchor.yaml`), so every subscriber
    renews about once a minute;
  - the subscribers' `retry.max-backoff` is `PT20S` (`k3s/eventing-{provider,consumer}.yaml`), so a
    registration that failed while the publisher was down is retried within 20 s. This also covers
    the provider CM restarted while the publisher is down.

  A subscription is thus back within one renewal (≤ 60 s) plus one backoff (≤ 20 s) after the
  publisher is Ready. `SUBSCRIPTION_RECOVERY_TIMEOUT` (3 min) covers that plus the publisher's
  start-up. The removed consumer cannot renew until it is back at the TIR, because the publisher's
  `dataspace-subscriptions` verifier service checks the TIR. Its recovery therefore counts from the
  `@After`'s TIR restore, plus the authority verifier's TIR cache. Step 1 reads that cache's TTL from
  the verifier config and shortens it in the overlay if it exceeds 30 s. The overlay values are
  demo/test values; `trust-anchor-setup.md` gives the production defaults.

  Restarting both participant CMs in the `@After` was rejected: a restart also runs the start-up
  sweep, which would act on whatever mode the ConfigMap holds at that moment. Blocking only the
  delivery (a NetworkPolicy, or dropping the provider's `/events` route) was rejected: the publisher
  stays up, so its delivery retries (`delivery.max-attempts` 5) can still land the event once the
  block is lifted. The scenario could then no longer tell which path removed the grant. Scaling to 0
  is also the outage the sweep exists for.
- **D4: the second publisher (scenario 7).** It is simulated: the IT writes a grant
  `scope=did:web:other-dataspace.org&scopeType=dataspace-membership` for the consumer directly into
  the provider's TIL. No second CM is deployed.
- **D5: timing.** The k3s values for the publisher's `tir-watcher` are `initial-delay: PT10S`,
  `interval: PT10S`, `confirmations: 2`. A removal therefore propagates in ~30 s; the IT polls with a
  3 min timeout. The lease and retry values of D3 (`lease-ttl: PT2M`, `retry.initial-backoff: PT5S`,
  `retry.max-backoff: PT20S`) size `SUBSCRIPTION_RECOVERY_TIMEOUT` (3 min).
- **D6: the watcher must have seen a DID before the IT removes it.** The watcher only announces a DID
  that an earlier snapshot holds. A removal that comes before the first cycle after a
  (re-)registration therefore publishes nothing, and a "nothing happened" assertion would pass
  vacuously. That applies to `did:web:kept-member.org` and to the consumer after an `@After` restore.
  So the IT's removal step does three things:
  1. it checks that the DID is listed at the TIR and takes `t0` = now;
  2. it waits, through the publisher's `dataspace-tir-watcher` health on 9090, until
     `lastSuccessfulCycle` has taken `WATCHER_CYCLES_TO_SEE` (= 2) distinct values after `t0`.
     The first such cycle may have read the registry before `t0`, but the second one started after
     the first ended, so it read it after `t0`;
  3. only then it deletes the DID.

  The bound is `WATCHER_SEEN_TIMEOUT`: two cycles plus margin, ~1 min with the D5 values. The wait
  lives in the removal step, not in a scenario, so no scenario can skip it. The only exception is
  scenario 6, which removes the consumer while the publisher is scaled to 0 and tests the sweep,
  not the watcher (Step 5). The publisher overlay
  sets `ENDPOINTS_HEALTH_DETAILS_VISIBLE=ANONYMOUS`, so the cluster-internal port shows the details.
  A fixed sleep of `interval × (confirmations + 1)` was rejected: it is still a guess when a cycle
  is slow, and the health value is exact.

## Deviations from the ticket (need human sign-off)
1. **The demo command is `mvn clean deploy -Peventing`, not `-Plocal`.** It is a scenario profile, as
   `-Pconsent` is. Putting eventing into `local` would add the authority's Keycloak/verifier/APISIX to
   every local deployment and CI job, and would change the provider TIL that `@local` features wipe
   and rebuild. Step 1 checks whether `-Plocal,eventing` composes and documents it if so.
2. **"`dataspace`-typed" is `scopeType=dataspace-membership`.** The TIL API enum wins, as in ticket-102.
3. **"`mvn -f pom.xml verify`" means the CI form** `mvn clean integration-test -Ptest,eventing,eventing-test`.
   A bare `verify` deploys nothing and runs every feature untagged against whatever cluster exists.
4. **Docs entry point.** `doc/EVENTING.md` is a short overview + demo, in the style of
   `doc/CONSENT_MANAGEMENT.md`. The guides stay in `doc/eventing/`: the existing `participant-setup.md`
   and a new `trust-anchor-setup.md`.
5. **A sixth ADR (0014).** It records D1, because AGENTS.md requires an ADR per architectural decision.

## Steps

### Step 1: Publisher at the trust anchor — `k3s/eventing-trust-anchor.yaml` and the `eventing` profile
- New `k3s/eventing-trust-anchor.yaml`, layered on `k3s/consent-trust-anchor.yaml` for the
  `consent-authority` release:
  - `contract-management.enabled: true`, single replica, `did: did:web:dataspace-authority.org`, all
    `enable*` integrations `false`. Append `NOTIFICATION_ENABLED=false` and
    `ENDPOINTS_HEALTH_DETAILS_VISIBLE=ANONYMOUS` (D6) to a **copy** of the default OTEL
    `additionalEnvVars` list. Add `services.trusted-issuers-registry.url:
    http://tir.trust-anchor.svc.cluster.local:8080` (check the TIR Service name/port).
  - `dataspace.events.publisher`: `enabled`, `origin` (the publisher host).
  - `dataspace.events.tir-watcher`: `enabled`, with the timing of D5; `max-removals-per-cycle` stays 5.
  - `dataspace.events.publisher.lease-ttl: PT2M` (D3), so subscriptions lost by a publisher restart
    come back within a minute instead of ~12 h.
  - `oid4vp` holder `did:web:dataspace-authority.org`: key from `dataspace-authority.org-tls`,
    credential from `vc-operator-credential`, dsc-ca as trust anchor, Squid proxy. Mount them through
    `deployment.additionalVolumes`/`additionalVolumeMounts`. Copy the converter/init pattern of
    `k3s/consent-provider.yaml` ~248–330 and of the consent-facade in
    `k3s/consent-trust-anchor-components.yaml`, which already present this identity. No new key or
    credential.
  - **Gateway for `/subscriptions`** on the authority APISIX, at a new host (prefer
    `dataspace-events.127.0.0.1.nip.io`, reachable in-cluster and from the IT; otherwise a
    `*.dataspace-authority.org` host wired like `consent-manager.dataspace-authority.org`):
    - routes `POST /subscriptions` and `DELETE /subscriptions/*` with `openid-connect`
      (`bearer_only`, `set_userinfo_header`), a `proxy-rewrite` that removes `X-Userinfo` with
      `_meta.priority` 3000, and an unauthenticated discovery route
      `/.well-known/openid-configuration` → verifier `/services/<id>/...`;
    - mirror the three participant routes of `doc/eventing/examples/events-routes.yaml`, but use
      **different names** (e.g. `dataspace-subscriptions*`): the umbrella validates the three
      `dataspace-events*` names against participant rules;
    - the overlay sets the authority APISIX `routes` to a **full copy** of the list in
      `consent-trust-anchor.yaml` (~614–769) with the three subscription routes appended. Helm
      replaces lists, so setting only the new routes would drop every consent route. `extraEnvVars`
      and `catchAllRoute` are not touched.
  - **A verifier service** (e.g. `dataspace-subscriptions`), appended to a **full copy** of the
    authority verifier's `registration.services` (which holds `consent-manager`):
    - it admits a participant's MembershipCredential with `trustedParticipantsLists` = the TIR, so a
      removed participant can no longer subscribe;
    - `jwtInclusion` maps the subscriber's DID to the top-level claim `participantId` (as in
      `participant-setup.md` §3). Check which credential field holds the organisation DID in the
      provider's and the consumer's MembershipCredential.
- **Decision: the trust anchor keeps the consent components.** The authority APISIX needs the
  `consent-manager` secret, so the `eventing` profile deploys the `trust-anchor` release exactly as
  `consent` does: `trust-anchor.yaml,consent-trust-anchor-components.yaml` with `--wait=false`
  (the components and the authority release need each other's secrets; `deploy.sh` waits for every
  release at the end). The authority release then differs from the consent scenario only by the
  overlay's additions. The other option was to clear `extraEnvVars` and the consent routes in
  `eventing-trust-anchor.yaml`. It was rejected because the authority would then run a gateway
  configuration that no other scenario tests, and the overlay would have to undo consent wiring
  instead of only adding to it. The cost is the consent-manager stack (consent-manager, its MongoDB,
  the facade) in the eventing deployment. `trust-anchor-setup.md` says that a production publisher
  does not need it.
- Root `pom.xml`:
  - move the `template-consent-authority` execution from the `consent` profile into the main build.
    It skips on a new `deploy.consentAuthority.template.skip`, which defaults to `true`; `consent` and
    `eventing` set it to `${deploy.template.skip}`, as `gaia-x` does with `deploy.gaiaxInfra.skip`.
    Its `-f ${deploy.consentAuthority.values}` already takes a comma-separated list;
  - a new `eventing` profile, declared after `consent`, with:
    - `deploy.trustAnchor.values` = `trust-anchor.yaml,consent-trust-anchor-components.yaml` and
      `deploy.trustAnchor.args` = `--wait=false` (see the decision above);
    - `deploy.consentAuthority.values` = `consent-trust-anchor.yaml,eventing-trust-anchor.yaml`;
    - provider and consumer values left at the `local` set for now; Step 2 adds the overlays.
  - `-Pconsent,eventing` is **not** supported. Both profiles set `deploy.provider.values`, and
    `eventing` is declared later, so its values win and `consent-provider.yaml` would be silently
    dropped. Say so in the `eventing` profile's comment and in `LOCAL.MD`.

  `k3s/deploy.sh` needs no new release: it already installs `consent-authority` whenever
  `CONSENT_AUTHORITY_VALUES` is set, and its `*_VALUES` variables take comma-separated files.
- Acceptance (local `mvn clean deploy -Peventing`, results in the PR description):
  - the publisher pod is Ready;
  - the authority APISIX pod is Ready;
  - `GET :9090/health` shows `dataspace-tir-watcher` with a `lastSuccessfulCycle` and a
    `snapshotSize` > 0;
  - a port-forwarded `POST :9090/events` returns `subscriptions: 0`;
  - `POST /subscriptions` through the new host returns `401` without a token, and the discovery
    route serves the service's metadata;
  - the consent scenario is unchanged. `mvn package -Pconsent` renders
    `target/rendered/participants/consent-authority` byte-identical before and after the `pom.xml`
    change;
  - the overlay only adds. Diff `helm template consent-authority` with
    `consent-trust-anchor.yaml` against `consent-trust-anchor.yaml,eventing-trust-anchor.yaml`: the
    only differences are the CM objects, the three `dataspace-subscriptions*` routes in the APISIX
    route config and the `dataspace-subscriptions` service in the verifier config. Every consent
    route, the `consent-manager` service and the `CONSENT_KEY` env var are still present.

### Step 2: Listener, subscriber, routes and policy at provider and consumer
- New `k3s/eventing-provider.yaml`, added to the `eventing` profile after `provider.yaml`:
  - `contract-management.dataspace.events`:
    - `listener.enabled`;
    - `subscriber.enabled`, with `sink: https://contract-management.127.0.0.1.nip.io/events`,
      `client-id`/`scopes` for the publisher's verifier service, and
      `protocol-settings.{client-id, scopes}` for the provider's events verifier service;
    - `publishers[0]`:
      - `id: did:web:dataspace-authority.org`, the `origin` of Step 1,
        `types: [org.fiware.dataspace.participant.removed]`, `subscriptions-url`;
      - `on-participant-removed.mode: report`, `registry-url` = the TIR;
      - `keep: [did:web:dataspace-authority.org, did:web:mp-operations.org]`;
    - `participant-removed.reconcile`: `initial-delay: PT20S`, `interval: PT6H` (D3);
    - `subscriber.retry`: `initial-backoff: PT5S`, `max-backoff: PT20S` (D3).
  - `ENDPOINTS_HEALTH_DETAILS_VISIBLE=ANONYMOUS` appended to a copy of the provider CM's
    `additionalEnvVars`, so that Step 5 can report each subscriber's per-publisher `state` and
    `expiresAt` from `:9090/health`.
  - Provider CM `oid4vp` for the subscriber: present `vc-fdsc-edc-credential` as
    `did:web:mp-operations.org`, following `consent-provider.yaml`.
  - The three routes of `doc/eventing/examples/events-routes.yaml`, appended to a **copy** of the
    provider's `decentralizedIam.odrlAuthorization.apisix.routes` and bound to
    `contract-management.127.0.0.1.nip.io`. They must pass `templates/eventing-validation.yaml`.
  - A verifier service `dataspace-events`:
    - it admits the OperatorCredential, with the provider TIL as `trustedIssuersLists` and the TIR as
      `trustedParticipantsLists`;
    - `jwtInclusion` with `fullInclusion: true` + the `participantId` mapping
      (`participant-setup.md` §3).
  - The `allowDataSpaceEvents.json` policy at the PAP, loaded by a Job the way
    `k3s/consent-provider.yaml` posts its policy, so the demo works without the IT.
  - `registration.issuer`: a full copy of the provider list. mp-operations and dataspace-authority get
    `scope: ""`. fancy-marketplace keeps the derived default (scope = publisher id, `scopeType:
    dataspace-membership`), so the Job writes the membership grant of scenario 1.
- New `k3s/eventing-consumer.yaml`: the same listener/subscriber/routes/verifier service/policy for
  the consumer.
  - The base set is `consumer.yaml`, `consumer-auth.yaml` and `consumer-tmf.yaml`, as the `central`
    profile uses: CM, gateway, verifier and TIL. Use the smallest set that gives the consumer all
    four, and document it.
  - Consumer `keep: [did:web:dataspace-authority.org, did:web:fancy-marketplace.biz]`, and the same
    `subscriber.retry` values and `ENDPOINTS_HEALTH_DETAILS_VISIBLE=ANONYMOUS` entry as the provider.
- Update the `eventing` profile and `k3s/deploy.sh` with both overlays.
- New `doc/scripts/dataspace-eventing-demo.sh` (shellcheck-clean, `set -euo pipefail`, named
  constants for hosts and DIDs, proxy `127.0.0.1:8888` as the other scripts use):
  - `remove <did>`: `DELETE https://til.127.0.0.1.nip.io/issuer/<did>`;
  - `restore <did>`: re-`POST /issuer` with `credentials: []`, as the participants' init containers
    do;
  - `report [provider|consumer]`: port-forward 9090 and print `GET /participants-removed`;
  - `publish <did>`: a manual `POST :9090/events` at the publisher, the immediate path of §8.
- Acceptance (local deploy, results in the PR description):
  - `helm template` of all overlays passes the umbrella's validation;
  - `publish` of an unknown DID returns `subscriptions: 2`;
  - after `kubectl rollout restart` of the publisher, `publish` returns `subscriptions: 2` again
    within ~90 s (the lease and backoff of D3);
  - `remove did:web:fancy-marketplace.biz`: within ~1 min the provider report lists it with mode
    `report` and the membership grant in `held`, and its TIL is unchanged;
  - `restore` puts it back.

### Step 3: Integration test scaffolding and scenarios 1–3
- `it/pom.xml`: a profile `eventing` that skips deploy, as `consent` does, and an `eventing-test`
  profile with `cucumber.filter.tags=@eventing`.
- `.github/workflows/test.yaml`: add the matrix entry `eventing` (`test,eventing,eventing-test`). It
  runs in both deployments per ADR 0007, which adds 2 CI jobs; say so in the PR.
- `StandardStepDefinitions`: split `setup()` (:111) in two hooks.
  - `@Before(order = 0)` `initWallet()`, untagged: `CryptoIntegration.init`, the BouncyCastle
    provider, a fresh `fancyMarketplaceEmployeeWallet` and the mapper setting. It only creates
    in-memory objects, so it is safe for `@consent`/`@eidas` too, and it lets the eventing feature
    reuse the order steps.
  - `@Before("not @consent and not @eidas and not @eventing")` `setup()`: `clean()`, `prepareTil()`
    and the sleep, as today. `@eventing` must stay out, because `cleanUpTIL` would delete the grants
    the registration Job wrote (scenario 1's proof and the authority's OperatorCredential grant that
    the provider's `dataspace-events` service needs) and `cleanUpPolicies` would delete the
    `allowDataSpaceEvents` policy loaded by the Step 2 Job.
  - A new `@After("@eventing")` `cleanUpEventingFixtures()` in the same class, because only it sees
    that scenario's `createdPolicies`. It deletes those policies by their `Location`, then calls
    `cleanUpTMForum()`. The eventing deployment holds no other TMF fixtures. It does **not** call
    `cleanUpPolicies`, `cleanUpTIL`, `cleanUpEntities`, `cleanUpDcatCatalog` or `cleanUpAgreements`.
- New support classes:
  - `DataSpaceEventingEnvironment`: constants for DIDs, hosts, namespaces, the publisher id, the
    ConfigMap, Deployment and registration Job names, and the timeouts of D5 and D3
    (`REMOVAL_TIMEOUT`, `MODE_SWITCH_TIMEOUT`, `SUBSCRIPTION_RECOVERY_TIMEOUT`);
  - `ContractManagementManagementClient`: a port-forward to 9090 through `KubernetesHelper`, with
    `report()`, `publish(type, subject)`, `awaitMode(publisher, mode)`,
    `awaitSubscriptions(n, timeout)` and, for the publisher, `awaitWatcherSaw(since)` (D6: wait for
    `WATCHER_CYCLES_TO_SEE` distinct `lastSuccessfulCycle` values after `since`, within
    `WATCHER_SEEN_TIMEOUT`);
  - `TrustAnchorClient`: `removeParticipant` / `restoreParticipant` against the trust anchor's TIL.
    `removeParticipant` checks the TIR listing and calls `awaitWatcherSaw` before the `DELETE` (D6).
    `removeParticipantWhilePublisherDown` (scenario 6 only) asserts the publisher has no pod and
    skips that wait;
  - `ProviderTilClient`:
    - `scopes(did)` (`GET /issuer/{did}/scopes`) and `credentials(did)` (the `credentials` field of
      `GET /issuer/{did}`);
    - `putGrant(did, scope, scopeType, credentials)`, which is `PUT
      /issuer/{did}/credential?scope=&scopeType=` only. It creates a missing issuer and replaces the
      scope's credentials, so the callers pass the complete list;
    - `removeGrant(did, scope, scopeType)` (`DELETE /issuer/{did}/credential?scope=&scopeType=`).

  Each gets a one-line Javadoc; parse JSON into small records, not maps.
- New `DataSpaceEventingStepDefinitions`:
  - `@Before("@eventing")` **checks** the starting state and never writes trust:
    - the consumer is registered at the TIR;
    - the provider TIL holds the consumer's `dataspace-membership` grant scoped to the publisher.
      If it is missing, the hook **fails** with "registration Job grant missing"; it does not add
      it. The first `@eventing` `@Before` of the JVM also asserts that the registration Job
      succeeded, stores the grant (scope, scopeType, credentials) in a static `jobGrant` and sets the
      static `jobGrantVerified`. At that point no fixture of this run has written trust yet. Scope
      and scopeType come from `scopes(did)`. The credentials come from the `credentials` field of
      `GET /issuer/{did}`, which does not tell grants apart. They are taken as the Job's grant only
      if `scopes(did)` lists exactly that one grant and its `credentialCount` equals
      `credentials.length`, i.e. the issuer holds nothing else. Otherwise the hook fails with
      "cannot isolate the registration Job grant";
    - mode is `report`.

    The subscription check is the `Background`'s first step, `The data space publisher has two
    active subscriptions.`. It polls `publish` of an unknown DID until `subscriptions == 2`, within
    `SUBSCRIPTION_RECOVERY_TIMEOUT`.
  - `@After("@eventing")` restores, in this order, everything a scenario may have changed:
    1. the consumer's TIR entry (`restoreParticipant`, if it is gone);
    2. the publisher's replicas (back to 1 and Ready, see Step 5);
    3. the original ConfigMap (D2);
    4. the membership grant, re-added from `jobGrant` **only** if the scenario removed it. This is the
       only place that writes it;
    5. test-only TIL state: the consumer's order-scoped grants (every scope other than the
       publisher's, read from `scopes(did)`), the `other-dataspace` grant and the
       `did:web:kept-member.org` issuer (Steps 4–5);
    6. then it waits for `subscriptions == 2`, so a failed recovery is reported by the scenario that
       caused it, not by the next one's `@Before`.

    The TMF/PAP fixtures are cleaned by `StandardStepDefinitions.cleanUpEventingFixtures()`, above.
    Each restore step runs in its own try/catch and the hook fails at the end with all errors, so
    one failed restore does not skip the others.
- The order-scoped grant comes from the existing order steps, not from a copy of them. The
  feature's `Background` lists them after the subscription check:
  ```gherkin
  Background:
    Given The data space publisher has two active subscriptions.
    And M&P Operations offers a managed kubernetes.
    And M&P Operations allows self-registration of organizations.
    And M&P Operations allows to buy its offerings.
    And Fancy Marketplace issues a user credential to its employee.
    And Fancy Marketplace registers itself at M&P Operations.
  ```
  Scenarios that need an order add `When Fancy Marketplace buys access to M&P's k8s services.`, then
  `Then the provider's TIL holds an order-scoped grant for Fancy Marketplace.` That step finds the
  order id through the TMF API and checks that CM wrote a grant with `scope=<order id>`. If CM's write
  fails because the issuer already exists (TIL `409` on a scoped create), record it and use `PUT
  /issuer/{did}/credential?scope=` semantics as CM does; do not paper over it in the test.
  - Two preconditions to check at the start of Step 3, and to report rather than work around:
    - the consumer's UserCredential is accepted at the provider only through the registration Job's
      scoped grant (`prepareTil` no longer runs), so the provider verifier must read scoped grants
      through the TIL's TIR API;
    - `buyAccess` uses `offers.stream().findFirst()`, so the `@After` must leave no offering behind
      (`cleanUpTMForum`).
- New `it/src/test/resources/it/dataspace_eventing.feature`, tagged `@eventing`, with the
  `Background` above and
  `Scenario: With the default report mode a removal changes nothing`, covering ticket items 1–3:
  - first, `Then the provider's registration Job wrote the membership grant for Fancy Marketplace.`
    It asserts `jobGrantVerified` (set from the deployment's own state before any fixture ran) and
    that the current grant has `scopeType: dataspace-membership`, `scope` = the publisher id and
    the credentials of the provider's `registration.issuer` entry (LegalPerson, User, Membership).
    A restore by an earlier scenario cannot satisfy it alone, because the static flag is only set
    by the first `@Before`. On a cluster reused for a second run, the first check sees the grant the
    previous run restored. CI always deploys fresh, which is where the Job is proven;
  - an order grants the consumer an order-scoped entry at the provider;
  - the operator deletes the consumer from the trust anchor's TIR;
  - the provider report lists the consumer for the publisher, with `action` = reported/none (check
    the CM value), both grants in `held`, `attributable: true`;
  - `GET /issuer/{did}/scopes` at the provider TIL is unchanged.
- Acceptance: `mvn clean integration-test -Ptest,eventing,eventing-test` is green locally (output
  summary in the PR); `@local` features are unaffected (`-Ptest,local-test` locally or in CI).

### Step 4: Scenarios 4 and 5 — `dataspace` mode and `keep`
- Step definitions for D2: patch `contract-management-dataspace-events` (key `dataspace-events.yaml`)
  through the Kubernetes client, changing only `publishers[0].on-participant-removed.{mode, keep}`.
  Keep the YAML editing in a helper that sets the given keys and leaves the rest byte-equal.
  - `mode` and `keep` are always set in **one** ConfigMap write, by a single step `the provider
    removes participants in <mode> mode, keeping <dids>.` (an empty list keeps the overlay's
    default `keep`). CM reloads the file as a whole, and the report does not show `keep`, so a
    separate `keep` write could not be observed. With one write, `awaitMode` seeing the new mode
    also proves that `keep` was reloaded.
  - A scenario writes the ConfigMap at most once. A second write that only changed `keep` would
    leave `mode` unchanged and `awaitMode` would return at once; the helper fails if the requested
    mode equals the current one, so this cannot happen by accident.
- `Scenario: In dataspace mode only the membership grant is removed` (ticket item 4). Switch to
  `dataspace`, place an order, then remove the consumer. Then:
  - the provider TIL has no `dataspace-membership` grant for the consumer, and the order-scoped grant
    remains;
  - the report shows `applied: true`.
- `Scenario: An identifier on keep is untouched` (ticket item 5). Add a simulated participant
  `did:web:kept-member.org`: register it at the TIR and give it a membership grant at the provider.
  Then switch to `dataspace` mode with `did:web:kept-member.org` appended to the default `keep`, in
  one write. Then remove it; the removal step waits for the watcher first (D6), so the event is
  really published. Its grant stays, and the report lists it with `action: kept`. Without the D6
  wait this assertion could pass with no event at all.
- Acceptance: as Step 3, all `@eventing` scenarios green; scenario order does not matter
  (`@After` restores state).

### Step 5: Scenarios 6 and 7 — reconciliation sweep and a second publisher
- `Scenario: With the push path blocked the reconciliation sweep reaches the same state` (ticket
  item 6), following D3:
  - switch to `dataspace` mode;
  - scale the publisher Deployment (`consent-authority` release, ns `trust-anchor`) to 0 and wait
    until no pod is left;
  - remove the consumer with `removeParticipantWhilePublisherDown`. It is the one removal that skips
    the D6 wait: the publisher is down, so its health cannot be read, and the sweep compares the
    local grants with the TIR directly, without any watcher snapshot;
  - `kubectl rollout restart` the provider CM and wait for Ready;
  - within `initial-delay` + margin, the provider TIL matches scenario 4, and the report's `trigger`
    names the sweep;
  - `@After` (Step 3 order) first restores the consumer at the TIR, so the watcher's first snapshot
    after the restart already contains it. Then it scales the publisher back to 1 and waits for
    Ready. Then it waits up to `SUBSCRIPTION_RECOVERY_TIMEOUT` for `subscriptions == 2`. That
    relies on the short lease and backoff of D3: without them the wait would time out, and every
    later `Background` with it. On a timeout the hook fails with each subscriber's last
    registration state, read from its CM log or health.
- `Scenario: An entry attributed to another data space survives` (ticket item 7), following D4:
  - give the consumer an additional grant `scope=did:web:other-dataspace.org,
    scopeType=dataspace-membership` at the provider TIL, in `dataspace` mode;
  - remove the consumer from the trust anchor;
  - the publisher's membership grant is gone, the other data space's grant and the order grant
    remain, and the report's `held` lists the remaining scope.
- Make sure no step can leave the publisher scaled down or the ConfigMap patched after a failure:
  restore in `@After`, which Cucumber runs on failure too.
- Acceptance: all 5 `@eventing` scenarios green twice in a row on one cluster (proves the cleanup);
  CI `eventing` jobs green in both deployments.

### Step 6: Documentation and release notes
- New `doc/eventing/trust-anchor-setup.md` (doctoc ToC), the operator's guide. It covers:
  - enabling the publisher in the authority release (D1), with the values of Step 1 explained;
  - the identity reuse: which DID, key secret and credential, and that nothing new is registered;
  - the `/subscriptions` gateway routes and verifier service, and why CM needs `X-Userinfo`
    (link `participant-setup.md`);
  - the TIR-watcher safeguards and timing (§8: first snapshot silent, `confirmations`,
    `max-removals-per-cycle`), and the production defaults vs. the k3s values;
  - the CM pitfalls: `NOTIFICATION_ENABLED`, `services.trusted-issuers-registry`, a single replica
    (in-memory state), and port 9090 cluster-internal;
  - removing a participant (`DELETE /issuer/{did}` at the TIL) and the immediate manual publish.
- Update `doc/eventing/participant-setup.md`:
  - the subscriber's OID4VP holder and `protocol-settings.client-id`;
  - `participant-removed.reconcile` (where reconciliation is configured);
  - drop the "not yet covered by an integration test" note;
  - link the trust anchor guide and `EVENTING.md`.
- New `doc/EVENTING.md`, structured like `doc/CONSENT_MANAGEMENT.md`:
  - a short overview: the problem, the flow and a mermaid sequence diagram (removal → watcher →
    event → participant reaction);
  - the components and where they run in k3s;
  - the demo: `mvn clean deploy -Peventing`, then `doc/scripts/dataspace-eventing-demo.sh
    remove|report|restore` with expected output;
  - the integration tests: the feature, the tag and the CI command;
  - links to architecture, specification, the two guides and the ADRs.
- `doc/README.md`: in `### Data Space Eventing`, link `EVENTING.md` and `trust-anchor-setup.md`.
  Update the doctoc ToC if headings change.
- Role guides:
  - `doc/deployment-integration/roles/operator/README.md`: a section "Data space eventing" linking
    the trust anchor guide;
  - `doc/deployment-integration/roles/provider/README.md` and `consumer/README.md`: a section linking
    `participant-setup.md`, noting the `report` default;
  - `doc/deployment-integration/local-deployment/LOCAL.MD`: the `eventing` profile.
- `doc/release-notes/10-x.md`: if `10.16.0` is not yet released on `main` (check `git log
  origin/main -- charts/data-space-connector/Chart.yaml` and gh-pages), add a subsection
  `### Trust anchor side, demo and integration test` to the 10.16.0 section. Otherwise add a new
  section, bump the chart to the next patch version and ask in the PR. Cover:
  - the publisher deployment;
  - the CM pitfalls (`NOTIFICATION_ENABLED`, `subscriber.protocol-settings.client-id`);
  - that the defaults change no trust configuration, as scenario 3 shows;
  - the demo profile.
- `AGENTS.md`: the `eventing` profile, the overlay names, the IT classes and the demo script.
- Acceptance: lychee 0 errors, using the static binary as below if the Docker script cannot mount;
  doctoc ToCs current; `helm lint` / `helm unittest` unaffected.

### Step 7: ADRs and the ADR index
- Write the following ADRs in the MADR-lite format of `doc/adr/0008-*`: status `Accepted`, the date,
  a link to the context doc, and context, decision, consequences and alternatives. Take the content
  from the decision register (architecture §12) and the review recorded there; do not invent new
  rationale.

  | ADR | Title | Decisions | Context doc |
  |-----|-------|-----------|-------------|
  | 0009 | Carry data space events as CloudEvents authenticated over OID4VP | D-2, D-9, D-1 (note the likely CM rename) | `doc/eventing/specification.md` |
  | 0010 | Detect participant removal by diffing the registry, not by instrumenting it | D-3, with the §8 safeguards | `doc/eventing/architecture.md` §8 |
  | 0011 | Authorize events per publisher and type | D-5 | `doc/eventing/architecture.md` §6 |
  | 0012 | Attribute local trust to the data space that caused it | D-6, D-10, D-11; `report` default, `keep`, `adopt-unattributed` | `participant-setup.md` |
  | 0013 | Confirm event callbacks with the CloudEvents webhook handshake | D-8 | `doc/eventing/specification.md` |
  | 0014 | Run the data space publisher in the authority's release, reusing its identity | D1 of this plan; rejected: a dependency in the `trust-anchor` chart, a dedicated release with its own verifier/gateway | `trust-anchor-setup.md` |

  D-4 (best-effort delivery) and D-7 (no payload signature) stay in the register only, unless the
  reviewer asks for them.
- `doc/adr/README.md`: a row for each ADR, plus the "belongs to" paragraph.
- `doc/eventing/architecture.md` §12: add an "ADR" column or a one-line note per D-x linking its ADR.
- Acceptance: lychee 0 errors; every ADR linked from the index and from its context doc.

**Link check in the agent container.** `.github/scripts/check-links.sh` cannot bind-mount through the
Docker sidecar. Use the static musl lychee 0.24.2 binary instead:
`lychee --config lychee.toml --root-dir "$(pwd)" '**/*.md'`.
