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

**Status (2026-11-19):** Steps 1 (PR #29) and 2 (PR #34) are merged; `ticket-106/work` @ `019ab82`.
`origin/ticket-106/step-2` carries two unmerged commits: `30ec74f` (IT scaffolding + report-mode
scenario, to be reused by Step 3) and `dfc0e16` (the `@typed-scopes` plan change, superseded by
this revision). The CM fix for the typed-scope blocker is released: CM chart **3.6.7** ships CM
**3.5.4**. Step 3 bumps to it; Steps 3–7 then run as planned, with all 7 scenarios.

**Sources** (read-only, clone under `/tmp`):
- `doc/eventing/architecture.md` §8–§12 and `doc/eventing/specification.md`.
- `git clone https://github.com/FIWARE/contract-management /tmp/cm` (tag 3.5.3 @ `c7da3c4`), especially:
  - `README.md`: 120–330 for the listener, publisher and subscriber; 375–527 for full configs; 550–700 for modes and the report;
  - `DataSpaceEventsProperties.java`;
  - `RemovedParticipantsEndpoint.java`.
- `https://github.com/FIWARE/helm-charts` `charts/contract-management` 3.6.6 (3.6.7 from Step 3).
- `https://github.com/FIWARE/trusted-issuers-list` `api/trusted-issuers-list.yaml`.

**CM facts the plan relies on** (verified at 3.5.3; Step 3 moves to 3.5.4, which fixes the typed
scope type only; property names are kebab-case under
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
    trigger, lastSeen, mode, action, applied, attributable, held:[{scope, scopeType,
    credentialCount}], unattributedCount}]}`. `action` is a `RemovalAction` and `trigger` a
    `RemovalTrigger` (`event`, `reconciliation`), both as lower-case tags. The actions the IT asserts
    are `reported` (nothing written: report mode), `kept` (on a keep list), `revoked_membership` and
    `not_held`. The report does **not** show `keep`.

    The report keeps **only the latest outcome per (publisher, DID)**, in memory until the pod
    restarts: `RemovedParticipantsReport.record` replaces the old entry. `lastSeen` is when that
    entry was last written. `ParticipantReconciler` also calls `touch`, which bumps `lastSeen` and
    keeps the old outcome; with the k3s `interval: PT6H` that only happens in the start-up sweep,
    i.e. right after a CM restart (scenario 6). So an entry is only evidence of *this* removal if its
    `lastSeen` is after the removal and its `trigger` is the expected one;
  - `POST /refresh`;
  - `GET /health`. It shows indicator details only if `endpoints.health.details-visible` is
    `ANONYMOUS` (the default is `AUTHENTICATED`). Set that with the env var
    `ENDPOINTS_HEALTH_DETAILS_VISIBLE=ANONYMOUS`.

  The CM Service exposes 8080 only, so 9090 is reached by a port-forward to the Deployment's pod
  (`deploy/<name>`), never through the Service.
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
  - With `scopeType` omitted, the scoped calls address only **untyped** grants.

  CM writes an order grant with `replaceCredentialsByScope(did, <order id>, credentials,
  ORDER_SCOPE_TYPE)`, and `ORDER_SCOPE_TYPE` is `null` (`TrustedIssuersListAdapter`:56, 109). An order
  grant is therefore an **untyped** grant (`scopeType: null`) written by `PUT`, so it cannot `409`.
  Reading, asserting and deleting it must send no `scopeType`; never `dataspace-membership` or an
  invented type.

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
  `did:web:fancy-marketplace.biz`. Its `credential-repo` projection (`oid4vp.credentialsFolder`,
  `consumer-tmf.yaml` ~163–175) holds only `vc-marketplace-credential` and
  `vc-legal-person-credential`. The consumer's MembershipCredential is in secret
  `vc-fdsc-edc-credential` (`consumer-auth.yaml` ~398–414, self-issued, `holderBinding.enabled:
  false`) and is not projected.
- **Consumer `registration.issuer`** (`k3s/consumer-auth.yaml` :8–11) holds only `did:web:mp-operations.org`
  (MembershipCredential). `did:web:dataspace-authority.org` is not in the consumer TIL, so as it
  stands the consumer's gateway would refuse every delivery from the publisher.
- **Provider `registration.issuer`** (~1685–1706) holds mp-operations, fancy-marketplace
  (LegalPerson, User, Membership) and dataspace-authority (OperatorCredential). Helm replaces lists, so
  an overlay must copy the whole list.
- **In-cluster DNS.** `*.127.0.0.1.nip.io` is a CNAME for `traefik-loadbalancer.infra` in-cluster
  (`k3s/infra/coredns`). That Service listens on **8080/8443 only**, so a direct `https://<host>/…`
  (port 443) from a pod is refused (verified in Step 1). A pod reaches such a host either through Squid
  (`squid-proxy.infra.svc.cluster.local:8888`, port 443 works) or directly at `https://<host>:8443`.
  Step 2 must therefore give every in-cluster URL to a nip.io host (the subscribers'
  `publishers[].url`, the sinks they register) a `:8443` port or a Squid proxy, and check that the
  publisher's delivery client reaches the sink. `*.dataspace-authority.org` / `*.mp-operations.org`
  hosts always go through Squid.
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
    - `KubernetesHelper` (`portForward`, k8s client using `it/target/k3s.yaml`/`KUBECONFIG`).
      `portForward(namespace, service, localPort, remotePort)` runs `kubectl port-forward
      svc/<service>`, so `remotePort` must be a *Service* port; the forward is bound to one pod for
      the life of the `kubectl` process;
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
  original and waits the same way until `mode` reads `report` again (Step 3). This needs no helm upgrade and no restart, and it also exercises the refresh path.
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

## Resolved blocker: CM 3.5.3 could not use the TIL's typed scopes
Found in Step 2: CM 3.5.3 sent the scope type by its Java enum name (`scopeType=DATASPACE_MEMBERSHIP`)
and the TIL 0.10.3 answered `400 Unknown scope type`. CM's typed-scopes probe read `too_old`, health
was `DEGRADED`, `report` mode showed `held: null`, the reconciler skipped every sweep and the writing
modes could not start. CM **3.5.4** (chart `oci://quay.io/fiware/helm-charts/contract-management`
**3.6.7**) sends `scopeType=dataspace-membership`. Step 3 bumps the dependency and proves the fix on
k3s before any scenario relies on it. There is **no** `@typed-scopes` tag and no exclusion in
`eventing-test`: every `@eventing` scenario runs.

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
    - follow the shape of the three participant routes of `doc/eventing/examples/events-routes.yaml`,
      but use **different names** (e.g. `dataspace-subscriptions*`): the umbrella validates the three
      `dataspace-events*` names against participant rules;
    - the subscription routes carry **no `opa` plugin**. `consent-trust-anchor.yaml` sets
      `odrl-pap.enabled: false` (~769), so the authority has no PDP. Authorization is the verifier
      service below (a TIR-listed participant's MembershipCredential) plus CM, which takes the
      subscriber's identity from `X-Userinfo.participantId`;
    - the overlay sets the authority APISIX `routes` to a **full copy** of the list in
      `consent-trust-anchor.yaml` (~614–769) with the three subscription routes appended. Helm
      replaces lists, so setting only the new routes would drop every consent route. `extraEnvVars`
      and `catchAllRoute` are not touched;
    - the new host is added to the authority APISIX Ingress (`consent-trust-anchor.yaml` ~588–600),
      otherwise Traefik never forwards it and every call gets a Traefik `404`: append it to **full
      copies** of `ingress.hosts` and `ingress.tls[0].hosts`, and to the comma-separated
      `cert-manager.io/alt-names` annotation (a copy of the whole `annotations` map is not needed:
      Helm merges maps, so setting that one key is enough).
  - **A verifier service** (e.g. `dataspace-subscriptions`), appended to a **full copy** of the
    authority verifier's `registration.services` (which holds `consent-manager`):
    - it admits a participant's MembershipCredential with `trustedIssuersLists: ["*"]` and
      `trustedParticipantsLists` = the TIR, as `consent-manager` does. That checks the credential's
      **issuer** against the TIR, nothing else;
    - **`participantId` is mapped from the credential's `issuer`, not from `credentialSubject.id`.**
      The subject is a value the issuer writes freely, so with a subject mapping any TIR-listed
      participant could mint a MembershipCredential for `did:web:fancy-marketplace.biz` and
      subscribe, or unsubscribe someone, as the removed consumer. The issuer is the one DID the TIR
      check binds. In k3s it is the participant's own DID: both MembershipCredentials are minted by
      the participant's own Keycloak (`provider.yaml` ~1726, `consumer-auth.yaml` ~409), so the issuer
      is `did:web:mp-operations.org` and `did:web:fancy-marketplace.biz` respectively. The contract
      this records: a subscriber is identified by the DID that issued its MembershipCredential, so a
      data space whose memberships are issued by a third party needs another binding (documented in
      `trust-anchor-setup.md`);
    - the mapping is explicit, as `participant-setup.md` §3 requires: `jwtInclusion.enabled: true`,
      `fullInclusion: false`, `flatClaims: true`, `claimsToInclude: [{originalKey: issuer, newKey:
      participantId}]`. `fullInclusion: true` would make the verifier ignore the mapping. No
      `verifiableCredential.*` claims are needed, as no ODRL policy reads them here;
    - residual risk: both MembershipCredentials are minted with `holderBinding.enabled: false`, so
      anyone who obtains a participant's JWT can replay it, exactly as the warning at
      `consent-trust-anchor.yaml` ~518 records for consent. `trust-anchor-setup.md` repeats it and
      names holder binding as the hardening step.
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
- Acceptance (render checks, results in the PR description). The cluster checks of the publisher
  (pods Ready, watcher health, `401` on `/subscriptions`, discovery route, the provider token's
  `participantId`) are not run in Step 1: they are items **S1-a** to **S1-e** of Step 2's acceptance,
  whose deploy is the first with a subscriber that can obtain a subscription token. The old
  `POST :9090/events` → `subscriptions: 0` check is replaced there by `subscriptions: 2`.
  - the consent scenario is unchanged. `mvn package -Pconsent` renders
    `target/rendered/participants/consent-authority` byte-identical before and after the `pom.xml`
    change, apart from the randomly generated secrets and the checksums derived from them;
  - the overlay only adds. Diff `helm template consent-authority` with
    `consent-trust-anchor.yaml` against `consent-trust-anchor.yaml,eventing-trust-anchor.yaml`: the
    only differences are the CM objects, the three `dataspace-subscriptions*` routes in the APISIX
    route config, the new host in the APISIX Ingress (`hosts`, `tls[0].hosts`, `alt-names`) and
    the `dataspace-subscriptions` service in the verifier config. Every consent
    route, the `consent-manager` service and the `CONSENT_KEY` env var are still present.

### Step 2: Listener, subscriber, routes and policy at provider and consumer
- **Lists are copied whole.** Helm replaces lists, so every list-valued key either overlay sets is a
  **full copy** of the base value files' list with the additions appended, as Step 1 does for the
  authority. The keys are:

  | Key | Provider base (`provider.yaml`) | Consumer base |
  |-----|---------------------------------|---------------|
  | `decentralizedIam.vcAuthentication.vcverifier.registration.services` | ~162 (`tmf-api`, `contract-management`, …) | `consumer-auth.yaml` ~151 (`tm-forum`, `dsp`) |
  | `decentralizedIam.odrlAuthorization.apisix.routes` | ~417 | `consumer-auth.yaml` ~262 |
  | APISIX `ingress.hosts` and `ingress.tls[].hosts` | untouched: the sink host is already listed (~381) | `consumer-auth.yaml` ~232/~251, plus the new sink host |
  | `contract-management.additionalEnvVars` | ~844 | `consumer-tmf.yaml` ~75 |
  | `contract-management.deployment.additionalVolumes` / `additionalVolumeMounts` | only if the oid4vp mounts need them (the base sets none) | `consumer-tmf.yaml` ~152/~184: `additionalVolumes` is set, a full copy with a `vc-fdsc-edc-credential` → `membership-credential.jwt` source appended to the `credential-repo` projection. `additionalVolumeMounts` is not set (`/credential-repo` is already mounted) |
  | `registration.issuer` | ~1685 | `consumer-auth.yaml` :8 |

  A key not in this table is a list the overlay must not set. A naive partial list fails far from
  the cause: without the old verifier services the Background's order steps fail with "service
  not found" or a 302.
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
    - `flatClaims: true` and `jwtInclusion` with `fullInclusion: false` and the explicit
      `claimsToInclude` of `participant-setup.md` §3: `credentialSubject.id` → `participantId`,
      `type` → `verifiableCredential.type`, `issuer` → `verifiableCredential.issuer` (the last two
      for the ODRL policy). `fullInclusion: true` must not be used: the verifier then ignores the
      mapping, the token has no `participantId`, CM refuses every delivery and the Background's
      probe step times out. The subject is safe here, unlike at the authority (Step 1): the
      provider TIL trusts only `did:web:dataspace-authority.org` as issuer of the
      OperatorCredential, so no one else can choose its subject.
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
  - **The consumer's sink host is `contract-management-fancy-marketplace.127.0.0.1.nip.io`**, a new
    host on the consumer APISIX, which mirrors the provider's `contract-management.127.0.0.1.nip.io`.
    The existing `fancy-marketplace.127.0.0.1.nip.io` is not reused: its `/*` and
    `/.well-known/openid-configuration` routes belong to the `tm-forum` service, and a separate
    host keeps the events surface apart from it. The host appears in four places:
    - appended to full copies of the consumer APISIX `ingress.hosts` and `ingress.tls[0].hosts`
      (`consumer-auth.yaml` ~232/~251), so Traefik routes it and cert-manager adds it to
      `apisix-consumer-tls`;
    - the consumer subscriber's `sink: https://contract-management-fancy-marketplace.127.0.0.1.nip.io/events`;
    - the `host` of all three consumer `dataspace-events*` routes;
    - the demo script's and the docs' consumer references.

    The routes' upstreams are `contract-management:8080` (the consumer CM Service; the umbrella's
    `contract-management.fullnameOverride` is `contract-management`, ns `consumer`) and
    `verifier:3000`, as on the provider. The consumer CM reaches the publisher through Squid, which
    its `MICRONAUT_HTTP_CLIENT_PROXY_*` env vars already configure.
  - **The consumer subscriber presents its MembershipCredential** (secret `vc-fdsc-edc-credential`,
    issuer `did:web:fancy-marketplace.biz`), the only credential type the publisher's
    `dataspace-subscriptions` service admits. The base `credential-repo` projection does not hold it
    (see the repo facts), so the overlay appends it to a full copy of
    `contract-management.deployment.additionalVolumes` (table above). Without it the consumer's
    `POST /subscriptions` never gets a token and the `Background` probe never reaches
    `subscriptions == 2`.
  - Consumer `keep: [did:web:dataspace-authority.org, did:web:fancy-marketplace.biz]`, and the same
    `subscriber.retry` values and `ENDPOINTS_HEALTH_DETAILS_VISIBLE=ANONYMOUS` entry as the provider.
  - `registration.issuer`: a full copy of the `consumer-auth.yaml` list (Helm replaces lists) plus
    `did:web:dataspace-authority.org` with `OperatorCredential` and `scope: ""`, as at the provider.
    Without it the consumer's `dataspace-events` verifier service refuses the publisher's
    credential and no event reaches the consumer.
  - The `mp-operations` entry keeps the derived default (scope = publisher id, `scopeType:
    dataspace-membership`). This is intended: mp-operations is a data space member whose
    MembershipCredential the consumer trusts *because* of that membership, the mirror of the
    provider's fancy-marketplace grant. It is not on the consumer's `keep`, but no scenario removes
    mp-operations. The provider gives its own DID `scope: ""` only because a participant's own entry
    is never attributed to the data space.
- **The publisher's delivery client.** The Step 1 publisher (`k3s/eventing-trust-anchor.yaml`) sets
  no `MICRONAUT_HTTP_CLIENT_PROXY_*`, so it would deliver to the `*.127.0.0.1.nip.io` sinks directly
  on 443, which is refused in-cluster (see the repo facts). Either register the sinks with `:8443`
  or add the Squid proxy env vars to the publisher's `additionalEnvVars` copy. The reports of the
  `publish` acceptance check below prove which one works.
- Update the `eventing` profile and `k3s/deploy.sh` with both overlays.
- New `doc/scripts/dataspace-eventing-demo.sh` (shellcheck-clean, `set -euo pipefail`, named
  constants for hosts and DIDs, proxy `127.0.0.1:8888` as the other scripts use):
  - `remove <did>`: `DELETE https://til.127.0.0.1.nip.io/issuer/<did>`;
  - `restore <did>`: re-`POST /issuer` with `credentials: []`, as the participants' init containers
    do;
  - `report [provider|consumer]`: `kubectl port-forward -n <provider|consumer>
    deploy/contract-management <local>:9090` (the Service has no 9090) and print
    `GET /participants-removed`;
  - `publish <did>`: a manual `POST :9090/events` at the publisher, the immediate path of §8.
- Acceptance (local deploy, results in the PR description):
  - `helm template` of all overlays passes the umbrella's validation;
  - the publisher checks deferred from Step 1, run on this deploy:
    - **S1-a** the publisher pod and the authority APISIX pod are Ready;
    - **S1-b** `GET :9090/health` at the publisher shows `dataspace-tir-watcher` with a
      `lastSuccessfulCycle` and a `snapshotSize` > 0;
    - **S1-c** `POST /subscriptions` through `dataspace-events.127.0.0.1.nip.io` returns `401`
      without a token;
    - **S1-d** the discovery route `/.well-known/openid-configuration` on that host serves the
      `dataspace-subscriptions` service's metadata;
    - **S1-e** the subscription token the publisher's `dataspace-subscriptions` service issues to the
      provider carries `participantId: did:web:mp-operations.org` (the credential's issuer, not its
      subject); verifier log or decoded token. This is the safeguard against one participant
      subscribing or unsubscribing as another, so the PR must show the token claim;
  - the overlays only add. Diff `helm template` of the base value files against base +
    `eventing-{provider,consumer}.yaml`, for each participant: every pre-existing APISIX route,
    ingress host, verifier service, `registration.issuer` entry, CM env entry, volume and volume
    mount is still present. The only differences are the eventing config, the three
    `dataspace-events*` routes, the `dataspace-events` verifier service, the added issuer, the
    added env entries, the policy Job and, at the consumer, the new sink host and the
    MembershipCredential source in the `credential-repo` projection;
  - the subscription token the publisher's `dataspace-subscriptions` service issues to the consumer
    carries `participantId: did:web:fancy-marketplace.biz` (verifier log or decoded token), as the
    provider's carries `did:web:mp-operations.org` (S1-e above);
  - `publish` of an unknown DID (e.g. `did:web:probe-<timestamp>.org`, fresh per run) returns
    `subscriptions: 2`, **and** within ~30 s both the provider report and the consumer report list
    that DID with `trigger: event` and a `lastSeen` after the publish. `subscriptions: 2` only counts
    subscriptions; the reports prove that both gateways accepted the delivery;
  - after `kubectl rollout restart` of the publisher, `publish` returns `subscriptions: 2` again
    within ~90 s (the lease and backoff of D3);
  - `remove did:web:fancy-marketplace.biz` (note the time first): within ~1 min the provider report
    lists it with `trigger: event`, a `lastSeen` after that time, mode `report`, `action: reported`
    and the membership grant in `held`, and its TIL is unchanged. An entry from an earlier removal of
    the same DID does not count;
  - `restore` puts it back.

### Step 3: CM 3.6.7 bump, integration test scaffolding and scenarios 1–3
- **Bump contract-management first** (own commit, before the IT work):
  - `charts/data-space-connector/Chart.yaml`: `contract-management` `3.6.6` → `3.6.7`; run
    `helm dependency update charts/data-space-connector`, then `helm lint`, `helm unittest` and
    `.github/scripts/values-schema.sh --check`. The chart `version` stays `10.16.0`: it is not yet
    released (`origin/main` is at 10.15.11), so the bump joins the 10.16.0 notes.
  - Check whether chart 3.6.7 still fails the render on `contract-management.dataspace: null`. Update
    the version in the comments that name 3.6.6 (`values.yaml` ~2040, `tests/eventing_test.yaml`
    ~47) to match what 3.6.7 does; if it renders, say so instead of keeping a stale warning.
  - `doc/release-notes/10-x.md` 10.16.0 component table: `contract-management` `3.6.3 → 3.6.7`, image
    `3.4.2 → 3.5.4`.
  - `doc/eventing/participant-setup.md` ~151–155: require CM ≥ 3.5.4 (chart 3.6.7) and replace the
    "Known issue in 3.5.3" paragraph with one line: 3.5.3 sends the enum name, the TIL answers `400`,
    so typed scopes, `held`, the writing modes and reconciliation need 3.5.4. Same for the `≥ 3.5.3`
    mentions in `values.yaml` (~2034, ~2135) where they concern typed scopes; `/refresh` notes stay.
  - `AGENTS.md` eventing section: "Feature needs CM ≥ 3.5.4 (chart 3.6.7)", and the "CM 3.5.3
    publisher gotchas" line re-checked against 3.5.4 (keep what still holds).
  - The k3s overlays pin no CM image, so they need no change.
  - **Prove the fix on k3s** (`mvn clean deploy -Peventing`, results in the PR): both participant CMs
    report health `UP` with the typed-scopes indicator `ok` (not `too_old`), and the Step 2 probe
    (`remove did:web:fancy-marketplace.biz` with the demo script) yields a provider report entry
    whose `held` lists the `dataspace-membership` grant. If it does not, stop and report `blocked`.
- **Reuse the scaffolding.** Cherry-pick `30ec74f` from `origin/ticket-106/step-2` (not `dfc0e16`),
  then remove what the blocker added: the `@typed-scopes` tag and scenario split in
  `dataspace_eventing.feature`, and the `and not @typed-scopes` exclusion in the `it/pom.xml`
  `eventing-test` profile. Scenario 1 below is one scenario again, with `held` and `attributable`
  asserted. Review the cherry-picked code against the rest of this step and fill the gaps.
- `it/pom.xml`: an `eventing-test` profile that mirrors `consent-test` (~415–432): it sets
  `cucumber.filter.tags=@eventing` **and** re-enables `maven-compiler-plugin` and
  `maven-failsafe-plugin` (`<skip>false</skip>`), with a comment saying why. The root `eventing`
  profile skips both so that `mvn clean deploy -Peventing` only deploys, and the `it` module inherits
  that; a profile that only sets the tag filter yields a CI job that compiles and runs nothing and
  still goes green. Check the job log for the `@eventing` scenario count. (The `it` `eventing`
  profile that skips the module's deploy, as `consent` does, came with Step 1: `mvn clean deploy
  -Peventing` would otherwise deploy twice.)
- `.github/workflows/test.yaml`: add the matrix entry `eventing` (`test,eventing,eventing-test`). It
  runs in both deployments per ADR 0007, which adds 2 CI jobs; say so in the PR.
- `StandardStepDefinitions`: split `setup()` (:111) in two hooks.
  - `@Before(value = "not @consent and not @eidas", order = 0)` `initWallet()`:
    `CryptoIntegration.init`, the BouncyCastle provider, a fresh `fancyMarketplaceEmployeeWallet`
    and `OBJECT_MAPPER.setSerializationInclusion(NON_EMPTY)`. It carries the same tag expression as
    today's `setup()`. An untagged hook would be wrong: `OBJECT_MAPPER` is the JVM-global
    `protected static final` mapper of `StepDefintions` (:27), and `ConsentStepDefinitions` and
    `EidasStepDefinitions` inherit it. Applying `NON_EMPTY` in those runs would silently drop
    empty or null fields from the bodies they serialize (e.g. ConsentStepDefinitions:345). With
    this expression `@consent`/`@eidas` behave exactly as today, and `@eventing` still gets the
    init the order steps need.
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
    (`REMOVAL_TIMEOUT`, `MODE_SWITCH_TIMEOUT`, `SUBSCRIPTION_RECOVERY_TIMEOUT`, `DELIVERY_TIMEOUT`);
  - `KubernetesHelper.portForwardToDeployment(namespace, deployment, remotePort)`, a new variant
    of `portForward` that runs `kubectl port-forward deploy/<deployment> <local>:<remotePort>`, so
    `remotePort` is the **container** port 9090. The CM Service keeps 8080 only: 9090 stays
    cluster-internal, as the docs say, and no chart or overlay adds a Service port for it. The
    variant picks a free local port itself (`new ServerSocket(0)`), so a forward that is still
    shutting down cannot collide with the next one, and it shares the start-up probe and timeout
    of `portForward`;
  - `ContractManagementManagementClient`, for one CM Deployment (namespace + Deployment name from
    `DataSpaceEventingEnvironment`), with `report()`, `publish(type, subject)`,
    `awaitMode(publisher, mode)`, `awaitSubscriptions(n, timeout)`,
    `awaitOutcome(publisher, did, trigger, since, timeout)` and, for the publisher,
    `awaitWatcherSaw(since)` (D6: wait for `WATCHER_CYCLES_TO_SEE` distinct `lastSuccessfulCycle`
    values after `since`, within `WATCHER_SEEN_TIMEOUT`).
    - **Only fresh report entries count.** The report keeps one entry per (publisher, DID) across
      scenarios (see the CM facts), and the consumer is removed in scenarios 3, 4, 6 and 7. So
      `awaitOutcome` polls until the entry for `did` has a `lastSeen` **after** `since` **and** the
      expected `trigger`, and returns that entry; every report assertion is made on the returned
      entry, never on a plain "the report lists the DID". `since` is the instant the removal step
      took right before its `DELETE` (scenario 6: before the provider CM restart, which empties the
      report anyway). k3s runs on the IT's host, so the pod and the IT share one clock. Scenario 6
      expects `trigger: reconciliation`; a `touch` by a later sweep only bumps that entry's
      `lastSeen` and keeps its outcome, so it cannot fake a result there.
    - **A fresh forward per request.** A forward is bound to the pod it started on and dies with it,
      and scenario 6 scales the publisher to 0 and back and runs `rollout restart` on the provider
      CM. So the client holds no long-lived forward: every `report()`/`publish()`/health read opens
      one through `portForwardToDeployment`, sends the request and closes it in a
      try-with-resources. The `await*` methods do this once per poll attempt and treat a forward
      that fails to start (no Ready pod yet) like a failed poll, retrying until their timeout, so
      they work across a scale-up or restart.
    - As a safety net every forward the client opens is also registered in a list that the
      `@After("@eventing")` closes last, as `ConsentStepDefinitions` closes its `portForwards`, so a
      failed step leaves no `kubectl` process behind;
  - `TrustAnchorClient`: `registerParticipant` / `removeParticipant` / `restoreParticipant`
    against the trust anchor's TIL. `registerParticipant` and `restoreParticipant` are idempotent:
    `POST /issuer` with `credentials: []`, and a `409` counts as success once `GET /issuer/{did}`
    confirms the DID is there. `removeParticipant` checks the TIR listing and calls
    `awaitWatcherSaw` before the `DELETE` (D6), and returns the instant taken right before the
    `DELETE`, the `since` of `awaitOutcome`. `removeIfPresent(did)` is the cleanup form (`404` is
    fine).
    `removeParticipantWhilePublisherDown` (scenario 6 only) asserts the publisher has no pod and
    skips that wait;
  - `ProviderTilClient`:
    - `scopes(did)` (`GET /issuer/{did}/scopes`) and `credentials(did)` (the `credentials` field of
      `GET /issuer/{did}`). Both return an empty list on `404`: `dataspace` mode deletes the whole
      issuer when the membership grant was its last grant (CM's `RemovalPlan` turns the revoke into
      `DeleteIfEmpty`), so a missing issuer is a normal state, not an error;
    - `putGrant(did, scope, scopeType, credentials)`, which is `PUT
      /issuer/{did}/credential?scope=&scopeType=` only. It creates a missing issuer and replaces the
      scope's credentials, so the callers pass the complete list;
    - `removeGrant(did, scope, scopeType)` (`DELETE /issuer/{did}/credential?scope=&scopeType=`).
    - `scopeType` is nullable throughout, and `null` sends **no** `scopeType` parameter, which
      addresses an untyped grant. Order grants are untyped (CM's `ORDER_SCOPE_TYPE` is `null`), so
      they are read, asserted and deleted with `scopeType` `null`. The record for a grant keeps
      `scopeType` as a nullable field, never a default value.

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
    - mode is `report`. This is a plain assertion without a wait: the previous scenario's `@After`
      already waited for the reload.

    The subscription check is the `Background`'s first step, `The data space publisher delivers
    events to both participants.`. It picks one fresh probe DID (`did:web:probe-<uuid>.example`,
    held by no one) and polls `publish` of it until `subscriptions == 2`, within
    `SUBSCRIPTION_RECOVERY_TIMEOUT`. Then it calls `awaitOutcome(publisher, probe, event, t)` on
    **both** the provider's and the consumer's client, `t` being the instant before the first
    publish, within `DELIVERY_TIMEOUT` (30 s). `subscriptions == 2` only counts subscriptions; the
    two report entries prove that both gateways accepted the publisher's credential and that both
    listeners processed the event. One probe DID per step keeps repeated publishes to one report
    entry. The action is not asserted (`not_held` or `reported`, depending on the mode).
  - `@After("@eventing")` restores, in this order, everything a scenario may have changed:
    1. the consumer's TIR entry (`restoreParticipant`, if it is gone);
    2. the publisher's replicas (back to 1 and Ready, see Step 5);
    3. the original ConfigMap (D2), then `awaitMode(publisher, report)` within
       `MODE_SWITCH_TIMEOUT`. A scenario field `configMapPatched`, set by the mode-switch step, skips
       both when the scenario never patched it. Without the wait the next `@Before`'s `mode is
       report` check fails inside the reload window (≤ ~2 min), and item 4 would re-add the grant
       while the provider may still act in `dataspace` mode;
    4. the membership grant, re-added from `jobGrant` **only** if `scopes(did)` no longer lists it
       (removed grant, or the whole issuer deleted). This is the only place that writes it;
    5. test-only trust state:
       - at the provider TIL, every grant of the consumer other than the Job's, read from
         `scopes(did)` and deleted with **its own** `scopeType`: the untyped order grants with no
         `scopeType` parameter, the `other-dataspace` grant with `dataspace-membership`. If the
         issuer is gone (`DeleteIfEmpty`, e.g. a scenario that failed before its order step),
         `scopes(did)` is empty and there is nothing to delete; item 4's `PUT` has already
         recreated it with the Job's grant, so in practice the list holds that grant only;
       - at the provider TIL, the `did:web:kept-member.org` issuer (Steps 4–5);
       - at the trust anchor TIL, `did:web:kept-member.org` (`removeIfPresent`). Scenario 5 registers
         it there, and if the scenario fails before its removal step the DID would otherwise stay in
         the TIR for every later scenario and run;
    6. then it waits for `subscriptions == 2` (the probe publish of the `Background` step, without
       the report check), so a failed recovery is reported by the scenario that caused it, not by
       the next one's `Background`.
    7. last, it closes any port-forward the management clients still hold (normally none, since
       each request closes its own).

    The TMF/PAP fixtures are cleaned by `StandardStepDefinitions.cleanUpEventingFixtures()`, above.
    Each restore step runs in its own try/catch and the hook fails at the end with all errors, so
    one failed restore does not skip the others.
- The order-scoped grant comes from the existing order steps, not from a copy of them. The
  feature's `Background` lists them after the subscription check:
  ```gherkin
  Background:
    Given The data space publisher delivers events to both participants.
    And M&P Operations offers a managed kubernetes.
    And M&P Operations allows self-registration of organizations.
    And M&P Operations allows to buy its offerings.
    And Fancy Marketplace issues a user credential to its employee.
    And Fancy Marketplace registers itself at M&P Operations.
  ```
  Scenarios that need an order add `When Fancy Marketplace buys access to M&P's k8s services.`, then
  `Then the provider's TIL holds an order-scoped grant for Fancy Marketplace.` That step finds the
  order id through the TMF API and checks that CM wrote a grant with `scope=<order id>` and
  `scopeType: null` (CM writes it untyped by `PUT`, see the TIL facts).
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
  - an order grants the consumer an order-scoped entry at the provider (`scopeType: null`);
  - the operator deletes the consumer from the trust anchor's TIR;
  - `awaitOutcome(publisher, consumer, event, since)` returns an entry with mode `report`,
    `action: reported`, `applied: false`, `attributable: true`, and in `held` both the membership
    grant (`scopeType: dataspace-membership`) and the order grant (`scopeType: null`);
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
  - the provider TIL has no `dataspace-membership` grant for the consumer, and the untyped
    order-scoped grant remains;
  - `awaitOutcome(publisher, consumer, event, since)` returns `action: revoked_membership`,
    `applied: true`.
- `Scenario: An identifier on keep is untouched` (ticket item 5). Add a simulated participant
  `did:web:kept-member.org`: register it at the TIR and give it a membership grant at the provider.
  The TIR registration goes through the idempotent `registerParticipant`, so a DID left over by an
  aborted run does not fail it (the `@After` also removes it, Step 3 item 5). Then switch to
  `dataspace` mode with `did:web:kept-member.org` appended to the default `keep`, in one write. Then
  remove it; the removal step waits for the watcher first (D6), so the event is really published.
  Its grant stays, and `awaitOutcome(publisher, kept-member, event, since)` returns `action: kept`,
  `applied: false`. Without the D6 wait and the `since` filter this assertion could pass with no
  event at all.
- Acceptance: as Step 3, all `@eventing` scenarios green; scenario order does not matter
  (`@After` restores state).

### Step 5: Scenarios 6 and 7 — reconciliation sweep and a second publisher
- `Scenario: With the push path blocked the reconciliation sweep reaches the same state` (ticket
  item 6), following D3:
  - switch to `dataspace` mode;
  - `When Fancy Marketplace buys access to M&P's k8s services.` and `Then the provider's TIL holds
    an order-scoped grant for Fancy Marketplace.`, **before** the publisher is scaled down (the
    order flow does not use the publisher). Without the order the membership grant is the
    consumer's only grant, CM's `dataspace` plan becomes `DeleteIfEmpty` and the sweep deletes the
    whole issuer, so the scenario could not show item 6's "same state as item 4";
  - scale the publisher Deployment (`consent-authority` release, ns `trust-anchor`) to 0 and wait
    until no pod is left;
  - remove the consumer with `removeParticipantWhilePublisherDown`. It is the one removal that skips
    the D6 wait: the publisher is down, so its health cannot be read, and the sweep compares the
    local grants with the TIR directly, without any watcher snapshot;
  - `kubectl rollout restart` the provider CM and wait for Ready;
  - within `initial-delay` + margin, the provider TIL holds exactly what scenario 4 asserts: no
    `dataspace-membership` grant scoped to the publisher, and the untyped order grant still present.
    `awaitOutcome(publisher, consumer, reconciliation, since)` returns `action: revoked_membership`,
    `applied: true`, with the order grant in `held`. The restart empties the in-memory report, so no entry of an earlier scenario
    can match;
  - `@After` (Step 3 order) first restores the consumer at the TIR, so the watcher's first snapshot
    after the restart already contains it. Then it scales the publisher back to 1 and waits for
    Ready. Then it waits up to `SUBSCRIPTION_RECOVERY_TIMEOUT` for `subscriptions == 2`. That
    relies on the short lease and backoff of D3: without them the wait would time out, and every
    later `Background` with it. On a timeout the hook fails with each subscriber's last
    registration state, read from its CM log or health.
- `Scenario: An entry attributed to another data space survives` (ticket item 7), following D4:
  - in `dataspace` mode, place an order and give the consumer an additional grant
    `scope=did:web:other-dataspace.org, scopeType=dataspace-membership` at the provider TIL;
  - remove the consumer from the trust anchor;
  - the publisher's membership grant is gone, the other data space's grant and the untyped order
    grant remain, and `awaitOutcome(publisher, consumer, event, since)` returns `action:
    revoked_membership`, `applied: true`, with both remaining grants in `held`. Without the `since`
    filter scenario 4's entry, which has the same action, would satisfy this at once.
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
  - that `participantId` is the MembershipCredential's issuer, not its subject (Step 1), what that
    requires of a data space whose memberships a third party issues, and the replay risk of
    credentials minted without holder binding. `specification.md` §5.2 says normatively that the
    identity compared with `subscriberId` is the `credentialSubject.id`; reconcile it (e.g. let a
    publisher bind the identity to the TIR-checked issuer and say so there). In k3s both are the
    participant's own DID, so the IT cannot tell them apart;
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
