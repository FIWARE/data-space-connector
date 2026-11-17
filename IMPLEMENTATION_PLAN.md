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
- **Reconciliation.** It is configured under `participant-removed.reconcile.{initial-delay, interval}`,
  not per publisher. It runs once after `initial-delay`, then every `interval`. There is **no trigger
  endpoint**.
- **Management port 9090.** It serves:
  - `POST /events` `{type, subject}`, which publishes manually and returns `202 {"id","subscriptions":n}`;
  - `GET /participants-removed`, the report: a list of `{publisher, mode, …, participants:[{id,
    trigger, mode, action, applied, attributable, held:[{scope, scopeType, credentialCount}],
    unattributedCount}]}`;
  - `POST /refresh`.

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
- **OID4VP.** `oid4vp.{enabled, holder.{holderId, keyType, keyPath, signatureAlgorithm},
  credentialsFolder, trustAnchors, proxyConfig}`. The chart has no secret-mount value, so mount the
  key and the credential with `deployment.additionalVolumes`/`additionalVolumeMounts`. The subscriber
  needs `subscriber.protocol-settings.client-id` (required at start-up; the chart's own example
  lacks it).
- **TIL API.**
  - `POST /issuer?scope=&scopeType=dataspace-membership` creates an issuer with a scoped grant.
  - `GET /issuer?scope=&scopeType=` lists issuers by scope.
  - `GET /issuer/{did}/scopes` returns an issuer's grants.
  - `PUT /issuer/{did}/credential?scope=&scopeType=` adds a scoped grant.
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
  - `contract-management.enabled: false` (line 283).

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
    and PAP, so the new tag must be excluded there.
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
- **D4: the second publisher (scenario 7).** It is simulated: the IT writes a grant
  `scope=did:web:other-dataspace.org&scopeType=dataspace-membership` for the consumer directly into
  the provider's TIL. No second CM is deployed.
- **D5: timing.** The k3s values for the publisher's `tir-watcher` are `initial-delay: PT10S`,
  `interval: PT10S`, `confirmations: 2`. A removal therefore propagates in ~30 s; the IT polls with a
  3 min timeout.

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
    `enable*` integrations `false`, and `NOTIFICATION_ENABLED=false` appended to a **copy** of the
    default OTEL `additionalEnvVars` list. Add `services.trusted-issuers-registry.url:
    http://tir.trust-anchor.svc.cluster.local:8080` (check the TIR Service name/port).
  - `dataspace.events.publisher`: `enabled`, `origin` (the publisher host).
  - `dataspace.events.tir-watcher`: `enabled`, with the timing of D5; `max-removals-per-cycle` stays 5.
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
      `dataspace-events*` names against participant rules.
  - **A verifier service** (e.g. `dataspace-subscriptions`) under the authority verifier's
    `registration.services`:
    - it admits a participant's MembershipCredential with `trustedParticipantsLists` = the TIR, so a
      removed participant can no longer subscribe;
    - `jwtInclusion` maps the subscriber's DID to the top-level claim `participantId` (as in
      `participant-setup.md` §3). Check which credential field holds the organisation DID in the
      provider's and the consumer's MembershipCredential.
- Root `pom.xml`: a new `eventing` profile modelled on `consent`. It deploys:
  - the `trust-anchor` release from `trust-anchor.yaml` only, with no consent components; if the
    authority release needs them, add `consent-trust-anchor-components.yaml` and say why;
  - `consent-authority` from `consent-trust-anchor.yaml,eventing-trust-anchor.yaml`, reusing the
    `template-consent-authority` execution;
  - provider and consumer: leave their values at the `local` set for now; Step 2 adds the overlays.

  Make `k3s/deploy.sh` (helm-install path) deploy the same.
- Acceptance (local `mvn clean deploy -Peventing`, results in the PR description):
  - the publisher pod is Ready;
  - the `dataspace-tir-watcher` health shows completed cycles;
  - a port-forwarded `POST :9090/events` returns `subscriptions: 0`;
  - `POST /subscriptions` through the new host returns `401` without a token, and the discovery
    route serves the service's metadata;
  - the `consent` profile still renders unchanged (diff `helm template` of the authority release with
    and without the profile).

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
    - `participant-removed.reconcile`: `initial-delay: PT20S`, `interval: PT6H` (D3).
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
  - Consumer `keep: [did:web:dataspace-authority.org, did:web:fancy-marketplace.biz]`.
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
  - `remove did:web:fancy-marketplace.biz`: within ~1 min the provider report lists it with mode
    `report` and the membership grant in `held`, and its TIL is unchanged;
  - `restore` puts it back.

### Step 3: Integration test scaffolding and scenarios 1–3
- `it/pom.xml`: a profile `eventing` that skips deploy, as `consent` does, and an `eventing-test`
  profile with `cucumber.filter.tags=@eventing`.
- `.github/workflows/test.yaml`: add the matrix entry `eventing` (`test,eventing,eventing-test`). It
  runs in both deployments per ADR 0007, which adds 2 CI jobs; say so in the PR.
- `StandardStepDefinitions` (:111): change the hook to `@Before("not @consent and not @eidas and not
  @eventing")`.
- New support classes:
  - `DataSpaceEventingEnvironment`: constants for DIDs, hosts, namespaces, the publisher id, the
    ConfigMap and Deployment names, and the timeouts of D5;
  - `ContractManagementManagementClient`: a port-forward to 9090 through `KubernetesHelper`, with
    `report()`, `publish(type, subject)` and `awaitMode(publisher, mode)`;
  - `TrustAnchorClient`: `removeParticipant` / `restoreParticipant` against the trust anchor's TIL.

  Each gets a one-line Javadoc; parse JSON into small records, not maps.
- New `DataSpaceEventingStepDefinitions`:
  - `@Before("@eventing")` ensures the starting state:
    - the consumer is registered at the TIR;
    - the provider TIL holds the consumer's membership grant (`PUT .../credential?scope=<publisher>
      &scopeType=dataspace-membership`, if missing);
    - mode is `report`;
    - both participants are subscribed: poll `publish` of an unknown DID until `subscriptions == 2`.
  - `@After("@eventing")` restores the TIR entry and the original ConfigMap, and removes test-only
    TIL grants.
  - To get the order-scoped grant, reuse the TMF order flow of `StandardStepDefinitions.buyAccess`.
    Extract a shared helper rather than copy it. Then check that CM wrote a grant with
    `scope=<order id>` for the consumer. If CM's write fails because the issuer already exists (TIL
    `409` on a scoped create), record it and use `PUT /issuer/{did}/credential?scope=` semantics as
    CM does; do not paper over it in the test.
- New `it/src/test/resources/it/dataspace_eventing.feature`, tagged `@eventing`, with a `Background`
  (subscriptions active) and
  `Scenario: With the default report mode a removal changes nothing`, covering ticket items 1–3:
  - an order grants the consumer an order-scoped entry at the provider, and the provider's TIL holds
    a `dataspace-membership` grant scoped to the publisher for the same DID;
  - the operator deletes the consumer from the trust anchor's TIR;
  - the provider report lists the consumer for the publisher, with `action` = reported/none (check
    the CM value), both grants in `held`, `attributable: true`;
  - `GET /issuer/{did}/scopes` at the provider TIL is unchanged.
- Acceptance: `mvn clean integration-test -Ptest,eventing,eventing-test` is green locally (output
  summary in the PR); `@local` features are unaffected (`-Ptest,local-test` locally or in CI).

### Step 4: Scenarios 4 and 5 — `dataspace` mode and `keep`
- Step definitions for D2: patch `contract-management-dataspace-events` (key `dataspace-events.yaml`)
  through the Kubernetes client, changing only `publishers[0].on-participant-removed.{mode, keep}`.
  Wait with `awaitMode`. Keep the YAML editing in a helper that changes one key and leaves the rest
  byte-equal.
- `Scenario: In dataspace mode only the membership grant is removed` (ticket item 4). Switch to
  `dataspace`, place an order, then remove the consumer. Then:
  - the provider TIL has no `dataspace-membership` grant for the consumer, and the order-scoped grant
    remains;
  - the report shows `applied: true`.
- `Scenario: An identifier on keep is untouched` (ticket item 5). In `dataspace` mode, add a
  simulated participant (e.g. `did:web:kept-member.org`): register it at the TIR and give it a
  membership grant at the provider. Put it on `keep`, then remove it. Its grant stays, and the
  report says it was kept (check the CM value).
- Acceptance: as Step 3, all `@eventing` scenarios green; scenario order does not matter
  (`@After` restores state).

### Step 5: Scenarios 6 and 7 — reconciliation sweep and a second publisher
- `Scenario: With the push path blocked the reconciliation sweep reaches the same state` (ticket
  item 6), following D3:
  - switch to `dataspace` mode;
  - scale the publisher Deployment (`consent-authority` release, ns `trust-anchor`) to 0 and wait
    until no pod is left;
  - remove the consumer;
  - `kubectl rollout restart` the provider CM and wait for Ready;
  - within `initial-delay` + margin, the provider TIL matches scenario 4, and the report's `trigger`
    names the sweep;
  - `@After` scales the publisher back to 1 and waits for the `subscriptions == 2` check of the
    Background.
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
