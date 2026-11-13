# Implementation Plan: [DSC] Chart: participant side of data space eventing

## Overview
Wire contract-management's (CM) data space eventing listener/subscriber into the umbrella chart, off
by default: `contract-management.dataspace.events.*` values, a refreshable publishers ConfigMap
(rendered by the CM subchart), `scope`/`scopeType` on `registration.issuer`, the APISIX routes for
the events path (rendered by odrl-authorization through a new extension point), an example ODRL
policy, tests and docs. An upgrade with default values must not create or change any
eventing-related resource; the other changes it does bring are listed under "Deviations from the
ticket".

**Stale AGENTS.md lines.** Two lines of the "Data Space Eventing" section of `AGENTS.md` are wrong.
Until a plan-mode agent or Step 6 updates them, **this plan wins**:
- It says the events route is a documented snippet. That approach was rejected (see "APISIX routes"
  below). Events routes go through the odrl-authorization `apisix.extraRoutes` map (Prerequisite B,
  Step 4). The default APISIX mode is standalone/yaml and serves no admin API, so admin-API hook
  Jobs do not work there.
- It says the CM subchart renders `contract-management.dataspace.events` into a refreshable
  ConfigMap. CM charts up to the latest (3.6.5, pinned by Step 2) do not; only the chart version of
  Prerequisite A will.

**Reading subchart values.** `.Values` is `chartutil.Values`, not a plain map. Do not pass it to
`dig`. Index into the subchart map first, as `_helpers.tpl` (`dsc.identityhub.assertHolderKid`) does:
`dig "dataspace" "events" "publishers" list (index .Values "contract-management" | default dict)`.

**Sources the step agents must read** (read-only, clone outside the repo):
- Design: `doc/eventing/architecture.md` §5–§9, §12 (D-5, D-8, D-12); `doc/eventing/specification.md`
  (§5 authentication binding, §6 authorization, §9 handshake); `doc/eventing/ticket.md` "DSC-1".
- CM application: `git clone https://github.com/FIWARE/contract-management /tmp/cm` (main @ `c7da3c4`,
  release 3.5.3). Property tree: `src/main/java/org/fiware/iam/configuration/DataSpaceEventsProperties.java`
  (`@ConfigurationProperties("dataspace.events")`); commented example in
  `src/main/resources/application.yaml` (lines ~115–209); deployment notes in `README.md` (~148–170
  gateway/`X-Userinfo`, ~742–760 refresh and mounting). Eventing ships in CM 3.5.0, the TIR watcher
  in 3.5.1, `on-participant-removed` and adoption in **3.5.3**.
- CM chart: `https://github.com/FIWARE/helm-charts` `charts/contract-management` (3.6.5, appVersion
  3.5.3; DSC pins 3.6.3, 3.6.5 after Step 2).
- TIL API: `https://github.com/FIWARE/trusted-issuers-list` `api/trusted-issuers-list.yaml`.
  `POST /issuer` takes `scope` and `scopeType` as **query parameters**; `scopeType` is the enum
  `dataspace-membership` (the ticket's `dataspace` is not a valid value — the API wins). Scope types
  exist from TIL 0.10.1: the 0.10.0 API has neither parameter.
- Gateway chart: `https://github.com/FIWARE/odrl-authorization` (published at
  `https://fiware.github.io/odrl-authorization/`), pulled in through
  `https://github.com/FIWARE/decentralized-iam`.

**Facts that shape the plan** (verified while planning):
- CM reads the eventing config from Micronaut config files (`MICRONAUT_CONFIG_FILES`, comma list) and
  re-reads it on a `micronaut.io.watch` event under `dataspace.events.listener.watch.path` (reacts to
  the kubelet `..data` symlink swap) or on `POST :9090/refresh`. A `subPath` mount never updates.
  Startup-only keys: the `enabled` flags of listener/publisher/subscriber/tir-watcher,
  `listener.watch.*`, `publisher.sink-*-timeout`. Everything else reloads at runtime.
- The CM charts 3.6.4 and 3.6.5 have no eventing values. They mount `application.yaml` with `subPath` and set
  `MICRONAUT_CONFIG_FILES=/application.yaml` itself. A `config:` override replaces the whole file.
  The chart has no Ingress or route template.
- CM does not verify tokens. The API gateway must run `openid-connect` with
  `set_userinfo_header: true` and must drop any `X-Userinfo` sent by the client. CM then reads the
  caller id from the claim `dataspace.events.listener.identity.participant-claim` (`participantId`).
- The `dataspace` removal mode revokes TIL grants with scope = publisher id and
  scopeType = `DATASPACE_MEMBERSHIP` (`ParticipantAdoption.grantMembership(did, publisherId, …)`).
- **TIL version.** `decentralizedIam` 2.1.28 (pinned) deploys trusted-issuers-list chart 0.19.1, which
  is TIL **0.10.0** and has no scopes. The latest releases, 2.1.29 and 2.1.30, deploy the same. TIL
  chart 0.19.2 (TIL 0.10.3) exists in FIWARE/helm-charts. The umbrella sets no TIL image override.
  Micronaut ignores query parameters it does not bind, so a scoped `POST /issuer` against TIL 0.10.0
  succeeds and stores an unscoped grant. CM's `on-participant-removed.mode: dataspace` would then find
  nothing to revoke.
  **Decision:** upgrade the TIL through a `decentralizedIam` bump (Prerequisite B, Step 4), not through
  an image override in the umbrella. An override would bypass the version pairing that decentralized-iam
  tests. An external TIL (`registration.til` pointing outside the release) must be ≥ 0.10.1, and the
  release notes say so.
- **TIL `409` on a scoped create.** In TIL 0.10.1, 0.10.3 and 0.10.4 (`TrustedIssuersListController
  .createTrustedIssuer`), `POST /issuer` answers `409` whenever the issuer DID exists
  (`existsById(did)`), before it looks at `scope`. A scoped create never adds a second grant to an
  existing issuer. So the registration Job's `409` path leaves an issuer registered before the upgrade
  with its unscoped grant, and it also skips the membership grant of an issuer that exists for another
  reason (e.g. a ProductOrder grant written through `PUT /issuer/{did}/scopes`). CM's
  `adopt-unattributed` covers the first case only. Moving the Job to `PUT /issuer/{did}/scopes` would
  cover both, but changes the upgrade no-op of Step 1; it is out of scope here and noted for a follow-up.
- **APISIX routes.** The routes are the subchart list `decentralizedIam.odrlAuthorization.apisix.routes`.
  odrl-authorization (2.4.3 in 2.1.28, 2.4.4 in 2.1.30) renders it in one of three ways, depending on
  the APISIX deployment mode:
  - Default mode (`standalone`, `data_plane`, `config_provider: yaml`; the umbrella's `values.yaml`
    does not change it): the routes go into the ConfigMap `apisix-routes` (`apisix.yaml`). **No admin
    API is served.**
  - etcd-backed modes (`traditional` + `etcd`, which `k3s/provider.yaml` uses; `standalone` +
    `traditional`; `decoupled` control plane): a hook Job PUTs each entry to the admin API as
    `<release>-<index>`.
  - Ingress-controller mode: one `ApisixRoute` CR per entry.

  Helm replaces lists instead of merging them. A default route in the umbrella's list is therefore lost
  as soon as a user sets `routes`. The subchart has no extra-routes value.
- **Rejected option: an umbrella hook Job that PUTs the route to the admin API** (the pattern of
  `templates/apisix-otel-global-rule-job.yaml`):
  - It only works in the etcd-backed modes. The default yaml mode has no admin API, so the Job would
    wait forever and fail the install.
  - It cannot clean up. odrl-authorization only PUTs its own `<release>-<index>` ids and never
    deletes, so a route PUT from outside it survives turning the feature off and `helm uninstall`.
  - It does not cover the ingress-controller mode.

  The existing OTEL global-rule hook has the same yaml-mode limitation. That is outside this ticket
  and is noted for a follow-up.
- **Chosen option, as the ticket asks ("if the subchart does not support it, FIWARE/helm-charts
  first"):** add an extension point to odrl-authorization (Prerequisite B). The umbrella then declares
  the events routes under it, disabled by default (Step 4).
- CI runs `helm unittest charts/data-space-connector` **without** `helm dependency build`, so suites
  can only target the umbrella's own `templates/`. Subchart rendering is checked in the upstream PRs
  and by local `helm template` runs (Steps 3 and 4). The umbrella's own templates can still read
  subchart values (`.Values.decentralizedIam...`), so validation across values is testable in CI.
- `contract-management` is an open (subchart) key in `values.schema.json`, so declaring keys under it
  does not change the schema. Still run `.github/scripts/values-schema.sh --check`.
- There is no helm-docs-generated values table in this repo. The `# --` comments are still required.
  If a step finds a generated table, regenerate it; otherwise report that there is none.

**Prerequisite A (outside this repo, blocks Step 3): contract-management chart change.**
Status 2026-11-13: CM chart **3.6.5** is released. It only bumps `appVersion` to 3.5.3 (the last bullet
below), and Step 2 picks it up. It has no `dataspace.events` value and no events ConfigMap, and
FIWARE/helm-charts `main` (`bcc4207`) has neither. Every other bullet below is still open, and
Step 3 waits for a chart version > 3.6.5 that has them.
The Gitea fork `general-agent-3/helm-charts` exists but is stale (main `56cd6c9`, CM chart 3.5.23,
far behind upstream 3.6.4). Before Step 3 can start, a human must either sync that fork's `main` to
upstream and run the change there, or open the PR on FIWARE/helm-charts directly. The new chart
version must then be released to `oci://quay.io/fiware/helm-charts`. The PR must:
- Add `dataspace.events` to `charts/contract-management/values.yaml` as the Micronaut
  `dataspace.events` property tree, verbatim (kebab-case keys as in `DataSpaceEventsProperties`).
  Default `{}`; every feature stays off; `# --` docs and a commented participant example with
  `on-participant-removed.mode: report`.
- Render the tree into a **separate** ConfigMap `<fullname>-dataspace-events` (key
  `dataspace-events.yaml`, content `dataspace: {events: …}`) only when
  `listener.enabled`, `publisher.enabled` or `subscriber.enabled` is true. Mount it as a directory
  **without `subPath`** at a fixed path (named in a helper, e.g. `/dataspace-events`). Append that
  file to `MICRONAUT_CONFIG_FILES` (`/application.yaml,/dataspace-events/dataspace-events.yaml`;
  check the comma handling in Micronaut's `DefaultEnvironment`). Default
  `listener.watch.enabled: true` and `listener.watch.path` to the mount path; a user-set value wins
  (`mergeOverwrite` with the user tree last).
- Put a checksum annotation over the **startup-only subset only**, so a change to those keys rolls
  the pod while a change to `publishers` reaches the running pod through the watch.
- Leave `additionalEnvVars`, `deployment.additionalVolumes` and `deployment.additionalVolumeMounts`
  untouched: append to them, never replace them.
- With eventing off, the rendering must be byte-identical to before (assert it with a diff of
  `helm template` against the previous chart version).
- Bump `appVersion` to 3.5.3 (the first CM release with `on-participant-removed`). Done in 3.6.5.
- Version via the PR label; FIWARE/helm-charts bumps chart versions in CI (see its `AGENTS.md`).
If this prerequisite is not met when Step 3 is picked up, Step 3 writes completion status `blocked`
and names the missing chart version.

**Prerequisite B (outside this repo, blocks Step 4): odrl-authorization extra routes and a
decentralized-iam release.** These repos have no Gitea forks, so a human opens both PRs upstream.
1. FIWARE/odrl-authorization, chart `odrl-authorization`:
   - Add the value `apisix.extraRoutes`: a **map** from route name to
     `{enabled: <bool>, route: <APISIX route object, same shape as an apisix.routes entry>}`.
     It must be a map, not a list: Helm merges maps key by key, so a parent chart's defaults and the
     user's values combine. Default `{}`, with `# --` docs.
   - Render the `enabled` entries after `routes` in all three paths:
     - the `apisix-routes` ConfigMap;
     - the admin-API script, under the stable id `<release>-<name>`. A disabled entry is `DELETE`d by
       that id (404 tolerated), so turning it off removes the route in the etcd-backed modes;
     - one `ApisixRoute` per entry.
   - With `extraRoutes: {}` the rendering must be byte-identical to before. Check it with a
     `helm template` diff in all three modes.
2. FIWARE/decentralized-iam: a release that pins that odrl-authorization version and
   `trusted-issuers-list` chart ≥ 0.19.2 (TIL ≥ 0.10.1). The PR must state whether TIL 0.10.1+ migrates
   its database at start-up, so that Step 4 can put it in the release notes.

If Prerequisite B is not met when Step 4 is picked up, Step 4 writes completion status `blocked` and
names the missing release.

**Release gating.** Steps merge into `ticket-102/work`. That branch must not be merged to `main` or
released until Steps 2 to 4 have landed. Step 1 alone would scope grants that the deployed TIL 0.10.0
drops, for a CM version that has no listener.

## Deviations from the ticket (need explicit human sign-off)
1. **`scopeType`** is `dataspace-membership`, not `dataspace`. The TIL API enum wins.
2. **"An upgrade with no new values changes nothing" does not fully hold.** The feature needs
   CM ≥ 3.5.3 and TIL ≥ 0.10.1, so a default upgrade after Steps 2 to 4 does change things:
   - the CM image moves to 3.5.3. That changes app behaviour `helm template` cannot show: CM 3.5.3's
     bundled `application.yaml` sets `endpoints.refresh: {enabled: true, sensitive: false}`
     (`SerializedRefreshEndpoint` replaces Micronaut's `RefreshEndpoint`), so **every** CM pod now
     answers an unauthenticated `POST :9090/refresh` (re-reads config, rebinds `@Refreshable` beans),
     even with eventing off. In 3.4.2 the endpoint was sensitive and answered `401`. The CM Service
     only exposes 8080, so 9090 is reachable by pod IP inside the cluster only. Operators who want the
     old behaviour add `ENDPOINTS_REFRESH_SENSITIVE=true` to `contract-management.additionalEnvVars`.
     That value is a list, and Helm replaces lists instead of merging them, so the operator must copy
     the whole default list (the nine `OTEL_*` entries in the umbrella's `values.yaml`) into their
     values and add the entry to it; setting only the new entry drops the tracing env vars.
     `/events` and `/participants-removed` on 9090 stay off unless their eventing feature is enabled;
   - the TIL image moves to ≥ 0.10.1, possibly with a DB migration;
   - the CM, odrl-authorization and decentralized-iam chart labels change;
   - every other change between decentralized-iam 2.1.28 and the new release comes along.

   The criterion this plan enforces instead:
   - no eventing ConfigMap, route, env entry or volume appears;
   - the registration Job script is byte-identical;
   - every other diff is listed in the Step 2, 3 and 4 PR descriptions.
3. **Two switches.** The events routes are off by default and are turned on separately from
   `contract-management.dataspace.events.listener.enabled`. A subchart value cannot depend on another
   value, and the verifier discovery URL depends on the deployment. The umbrella `fail`s on
   inconsistent combinations (Step 4). A listener without the chart's route is allowed, for operators
   who run their own gateway.
4. **Unit tests** reach only the umbrella's templates, because CI does not build dependencies.
   Subchart rendering is checked upstream and by local `helm template` runs.

**Link check in the agent container.** `.github/scripts/check-links.sh` runs lychee in Docker with a
bind mount that the sidecar cannot see ("lychee.toml not found"). That is a tooling failure. Use the
static musl lychee 0.24.2 binary instead:
`lychee --config lychee.toml --root-dir "$(pwd)" '**/*.md'`. It must report 0 errors.

## Steps

### Step 1: Attribute `registration.issuer` grants to the data space publisher
Umbrella-only; not blocked by either prerequisite. It only takes effect once Steps 2 to 4 have
landed (see "Release gating").
- `charts/data-space-connector/templates/participant-registration.yaml`: send `scope` and
  `scopeType` as query parameters on `POST {til}/issuer`, URL-encoded (`urlquery`).
  - Per issuer entry, optional `scope` and `scopeType`. If `scope` is unset, it defaults to the `id`
    of the one entry in `contract-management.dataspace.events.publishers` whose `types` contain
    `org.fiware.dataspace.participant.removed`. The key may be absent; read it as described under
    "Reading subchart values".
  - `scopeType` defaults to `dataspace-membership` whenever a scope is in effect.
  - An explicit `scope: ""` opts the entry out (unscoped, as today).
  - If several publishers subscribe to `participant.removed` and an entry sets no scope, `fail` with a
    message saying to set `scope` per issuer.
  - With no such publisher the generated script must be **byte-identical** to today's (upgrade no-op).
- Add the `scope`/`scopeType` keys to the commented `registration.issuer` example in `values.yaml`
  with `# --` text. The text must say that the Job's existing `409` path leaves already-registered
  issuers unscoped (verified, see "TIL `409` on a scoped create"); CM's
  `on-participant-removed.adopt-unattributed` covers those. It must also say:
  - the scope needs TIL ≥ 0.10.1, which an older TIL silently drops; the chart's own TIL reaches that
    version with Step 4;
  - the default derives from `contract-management.dataspace.events.publishers`, which only takes
    effect with the CM chart version of Step 3;
  - the default is right only for a **membership** grant (the ticket scopes membership grants only)
    that lands in a TIL whose CM subscribes to the **same** publisher id. That holds for this
    release's own TIL, and for another release's TIL (e.g. `k3s/consumer-*.yaml` registering at the
    provider's TIL) when both releases subscribe to the same data space publisher. Otherwise, or for
    a grant that does not come from membership (e.g. an eIDAS `UserCredential` issuer, which a
    `participant.removed` for that DID would revoke), set `scope` explicitly or `scope: ""`.
- Do not bump `decentralizedIam` here. That is Step 4, decided above.
- New suite `charts/data-space-connector/tests/eventing_test.yaml`, parameterized where helm-unittest
  allows. Cases:
  - default values render the old script (no `scope=` anywhere);
  - one publisher → `?scope=<id>&scopeType=dataspace-membership`;
  - explicit `scope` and `scopeType` win;
  - `scope: ""` → unscoped;
  - two publishers without a scope → `failedTemplate`.
  Leave `tests/participant-registration_test.yaml` passing unchanged.
- Run `.github/scripts/values-schema.sh` (regenerate if needed), `helm lint charts/data-space-connector`
  and `helm unittest charts/data-space-connector`.
- Acceptance: all pass, and `helm template` with default values is unchanged against the base branch.

### Step 2: Bump contract-management to 3.6.5 (CM 3.5.3)
Split out of the original Step 2 because 3.6.5 carries only the `appVersion` part of Prerequisite A.
- `charts/data-space-connector/Chart.yaml`: set the `contract-management` dependency to 3.6.5.
- Diff `helm template` with default values before and after, and list every change in the PR
  description. Expected: the image moves from 3.4.2 to 3.5.3 and chart labels change (Deviation 2).
  Nothing eventing-related may appear. CM 3.5.3 keeps every eventing feature off by default.
  Also list the app-level change `helm template` cannot show: the unauthenticated `POST :9090/refresh`
  (Deviation 2).
- Do not declare `dataspace.events` values yet: chart 3.6.5 ignores them, so they would be inert.
- Acceptance: `helm lint`, `helm unittest`, `values-schema.sh --check` pass; the default-values diff
  has no eventing resource in it.

### Step 3: Bump contract-management and declare `contract-management.dataspace.events`
Blocked until a CM chart version > 3.6.5 that implements the rest of Prerequisite A is released.
- `charts/data-space-connector/Chart.yaml`: set the `contract-management` dependency to the new
  chart version.
- Diff `helm template` with default values before and after, and list every change in the PR
  description. Expected: only chart labels change; the image stays 3.5.3 unless the new chart moves it.
  Nothing eventing-related may appear.
- `charts/data-space-connector/values.yaml`, under `contract-management:`, add
  `dataspace.events` with `# --` comments. Declare only keys whose default is safe:
  - `listener.enabled: false`;
  - `subscriber.enabled: false`;
  - `publishers: []`, with a commented participant example:
    - `id`, `origin`, `types: [org.fiware.dataspace.participant.removed]`,
      `subscriptions-url`, `client-id`, `scopes`;
    - `on-participant-removed.mode: report`, `registry-url`.

  Do **not** declare empty-string defaults that would override CM's own fallbacks, e.g.
  `own-participant-id` (falls back to `general.did`) or `dataspace-id`. Document them in comments
  instead.
- Explain in a comment that the namespace is `dataspace.events`, not `events` (D-12), and that the
  publisher/tir-watcher side belongs to the trust anchor and is not configured here.
- Add a `# @schema` object block (`type: object`, `additionalProperties: true`, `properties: {}`)
  only if the generator needs it, then run `.github/scripts/values-schema.sh --check`.
- Do not touch the existing `contract-management.additionalEnvVars` (OTEL list). Eventing must not add
  env entries there.
- Extend `tests/eventing_test.yaml` with the umbrella-renderable cases:
  - Step 1's default-publisher derivation reads the new `values.yaml` example shape;
  - the values schema accepts a full participant block (if `values-schema_test.yaml` style allows it).
- Local only, not in CI: after `helm dependency update charts/data-space-connector`, script three
  `helm template` checks and paste the results into the PR description:
  1. defaults → no `*-dataspace-events` ConfigMap;
  2. `listener.enabled` + one publisher → ConfigMap, a directory mount without `subPath`, and
     `MICRONAUT_CONFIG_FILES` that contains the events file;
  3. the same plus a custom `additionalEnvVars` entry → the OTEL entries and the custom entry are
     all present.

  Do not commit `charts/data-space-connector/charts/*.tgz` or `Chart.lock` changes unless the repo
  already tracks them.
- Acceptance: `helm lint`, `helm unittest`, `values-schema.sh --check` pass; the default-values diff
  has no eventing resource in it.

### Step 4: Bump decentralizedIam; declare the events routes
Blocked until the decentralized-iam release of Prerequisite B is out.
- `charts/data-space-connector/Chart.yaml`: set the `decentralizedIam` dependency to that release.
  Diff `helm template` with default values before and after, and list every change in the PR
  description: the TIL image (≥ 0.10.1), the odrl-authorization version, labels, and any other change
  that comes with the release (Deviation 2). No route may appear.
- `values.yaml`, under `decentralizedIam.odrlAuthorization.apisix.extraRoutes`, declare three
  entries, all `enabled: false`, each with `# --` comments:
  - `dataspace-events`: `POST` `/events`, upstream `contract-management:8080`. The name comes from
    the umbrella's `contract-management.fullnameOverride` and matches the existing CM route in
    `k3s/provider.yaml`. Do not trust that: check name and port against the Service in a
    `helm template` render, and say in the `# --` comment that `route.upstream.nodes` must be
    overridden when `fullnameOverride` changes or CM runs outside the release. Plugins:
    - `openid-connect` with `bearer_only`, `use_jwks` and `set_userinfo_header: true`. Its
      `discovery` points at the verifier service of the events scope and is empty by default
      (deployment-specific).
    - `opa` (as on the existing routes).
    - A `proxy-rewrite` that removes a client-sent `X-Userinfo` **before** `openid-connect` sets it.
      APISIX runs higher priorities first (`openid-connect` 2599, `proxy-rewrite` 1008), so the
      rewrite needs `_meta.priority` above 2599. Verify this against the APISIX version that ships and
      against the CM README (~148–170).
  - `dataspace-events-handshake`: `OPTIONS` `/events`, the same upstream, no auth plugins.
  - `dataspace-events-discovery`: `GET` `/events/.well-known/openid-configuration`, upstream
    `verifier:3000`, no auth plugins, and a `proxy-rewrite` whose `uri` is
    `/services/<events-service>/.well-known/openid-configuration`. Empty by default
    (deployment-specific).
    - Why: on a `401` the publisher's OID4VP client fetches `<sink>/.well-known/openid-configuration`
      (specification §5.1 step 1), and the callee MUST serve it for every protected path, with the
      events scope in `scopes_supported`. Without this route that `GET` hits the host's catch-all: a
      `401` behind `openid-connect`, a `404`, or on the k3s CM host the priority-100 `/*` +
      `vars` route (`k3s/provider.yaml` ~443–454) that serves the **contract-management** service's
      metadata, which lacks the events scope. The publisher can then never get a token.
    - The path assumes the announced sink is `<scheme>://<host>/events`. Say in the `# --` comment
      that a sink under another path needs both `uri`s changed.
    - `<events-service>` is the verifier service of Step 6 item 3, and the same service that
      `dataspace-events`' `openid-connect.discovery` points at.
    - `verifier:3000` comes from `decentralizedIam.vcAuthentication.vcverifier.fullnameOverride:
      verifier` and the k3s routes. Check it against the rendered Service, as for the CM upstream.
  - **A host binding is required once enabled, but there is none in the defaults.**
    - Why required: APISIX's default router `radixtree_host_uri` tries host-bound routes first and
      falls back to host-less routes only when no host-bound route matches. DSC deployments bind
      their routes to hosts, with `/*` catch-alls (e.g. `k3s/provider.yaml`, `tpp-data-service`,
      `contract-management`). A host-less `/events` would lose to the catch-all of whatever host the
      publisher delivers to, which applies the wrong verifier scope and policy. The host must be the
      host of the sink URL the subscriber announces.
    - Why no default: the host is deployment-specific, and APISIX rejects an empty one. `hosts`
      has `minItems: 1` in `apisix/schema_def.lua`; with `hosts: []` the yaml mode logs "failed to
      check item data" and drops the route, and the admin-API mode answers the PUT with 400, which
      fails the hook Job. Document `host` / `hosts` in the `# --` comment only. Helm merges maps, so
      the user's key reaches the route.
    - Both keys exist: the existing routes use the singular `host`. APISIX does not allow `host` and
      `hosts` on the same route (a `not`/`anyOf` clause in the route schema).
    - Verify against the APISIX version that ships: the `minItems`, the host/hosts exclusion, the
      host-first fallback of `radixtree_host_uri`, and that the exact `/events` and
      `/events/.well-known/openid-configuration` beat a `/*` on the **same** host, including the
      priority-100 `/*` + `vars` discovery route (lua-resty-radixtree matches the exact path before
      prefixes, and `priority` only orders routes of the same path). If a `/*` wins, give all three
      entries a default `priority` above the catch-alls and say so in the PR description.
- New template `templates/eventing-validation.yaml`: no output, `fail` only. It fails when
  `dataspace-events` is enabled and:
  - `contract-management.enabled` is true and `contract-management.dataspace.events.listener.enabled`
    is not true (with an external CM the umbrella's listener flag means nothing), or
  - its `openid-connect.discovery` is empty, or
  - `dataspace-events-handshake` or `dataspace-events-discovery` is not enabled, or
  - the `proxy-rewrite.uri` of `dataspace-events-discovery` is empty or is not a suffix of
    `dataspace-events`' `openid-connect.discovery` (both must name the same verifier service).

  It also fails when any of the three entries is enabled and its `route`:
  - has neither a non-empty `host` nor a non-empty `hosts`, or
  - has both `host` and `hosts`, or
  - binds other hosts than another enabled entry (handshake and discovery must answer where the
    events arrive).

  The keys may be absent, so it reads them as described under "Reading subchart values", starting
  from `index .Values "decentralizedIam"` and `index .Values "contract-management"`. A listener without the
  route stays valid (Deviation 3).
- Extend `tests/eventing_test.yaml`, parameterized: defaults render nothing; each inconsistent
  combination gives `failedTemplate` with its message, including on each enabled entry: no host,
  `host: ""`, `hosts: []`, both keys set, and hosts that differ between the entries; the discovery
  entry disabled, with an empty rewrite `uri`, and with a service other than `discovery`'s. The consistent
  combination renders with `host`, with one entry in `hosts` and with two; and with
  `contract-management.enabled: false` and the listener off.
- Local only, not in CI: after `helm dependency update`, render with the routes enabled in the default
  yaml mode and in `traditional`+`etcd` mode. Check that the `apisix-routes` ConfigMap and the admin
  script both contain all three routes, that the user's `routes` list is unchanged, and that there is
  a `DELETE` per route when they are disabled. Then run the rendered yaml-mode config in an APISIX
  container of the shipped version, with a host-bound `/*` catch-all on the same host and on another
  host, plus a copy of the k3s priority-100 `/*` + `vars` discovery route on the same host. Show with
  `curl` that `POST /events`, `OPTIONS /events` and `GET /events/.well-known/openid-configuration` on
  the bound host reach the events routes, the last one with the rewritten path (an echo upstream is
  enough). Paste the results into the PR description.
- `values-schema.sh --check` (`decentralizedIam` is an open key), `helm lint`, `helm unittest`.
- Acceptance: all pass; the default-values diff contains no route and no eventing resource.

### Step 5: Example ODRL policy for the events path
- New `it/src/test/resources/policies/allowDataSpaceEvents.json`, in the style of
  `allowContractManagement.json`:
  - target `http:path` `isInPath` `/events`;
  - the assignee constrained by the credential type the receiver admits for events
    (architecture §6 / specification §5; the type is local configuration, so use the type the
    integration-test publisher would present and say so in the PR description);
  - action limited to `POST`, if the existing policies express methods.

  `OPTIONS /events` (the handshake) is unauthenticated and does not go through OPA.
- Validate that it is well-formed JSON (`python3 -I -m json.tool`). Do not wire it into the
  integration tests (`it/`); that is a separate ticket.
- Acceptance: the file parses and mirrors the structure of the existing policies.

### Step 6: Docs, release notes, version bump
- New `doc/eventing/participant-setup.md` (doctoc ToC). Write it as an operator guide and link to
  specification/architecture for the reasons. Content:
  1. the values to enable the listener/subscriber, and which keys are startup-only (a rollout
     follows) vs. refreshed at runtime;
  2. the gateway: enable the three `extraRoutes` entries of Step 4, set `discovery` and the
     discovery route's rewrite `uri` to the events verifier service, and bind all three to the host
     of the announced sink URL with `host` or `hosts` (never empty, not both). Override
     `upstream.nodes` if CM is not `contract-management:8080` or the verifier not `verifier:3000`;
     change the `uri`s if the sink path is not `/events`. Explain:
     - why the host is required (host-bound catch-alls win under `radixtree_host_uri`);
     - that the sink host should be the host whose catch-all already routes to CM (e.g. the k3s
       `contract-management` host), so the exact `/events` routes shadow it. Any other route whose
       upstream is CM (a `/*` on another host) also forwards `POST /events`, with that route's
       verifier scope and policy and without the `X-Userinfo` strip. The events scope and policy
       then do not apply, and unless that route's `openid-connect` sets `X-Userinfo` itself, a
       client-sent header reaches CM's source == identity check. Such routes must exclude
       `/events` (a `vars` condition or an ODRL policy that denies it);
     - why the discovery route is needed (specification §5.1 step 1);
     - why there are two switches (Deviation 3);
     - what the `proxy-rewrite` priority is for;
     - that operators with their own gateway must reproduce the same routes and plugin chain,
       including the discovery route and removing `X-Userinfo`;
  3. a verifier service registration
     (`decentralizedIam.vcAuthentication.vcverifier.registration.services`), whose id is the
     `<events-service>` of the `dataspace-events` and `dataspace-events-discovery` routes, and whose
     scope is the one subscribers request:
     - `jwtInclusion` maps `credentialSubject.id` to the top-level claim `participantId`;
     - check the field names in the credentials-config-service API;
  4. the ODRL policy from Step 5;
  5. that `registration.issuer` grants are scoped (Step 1) and need TIL ≥ 0.10.1, and when the
     default scope is wrong (Step 1: a non-membership grant, or a TIL whose CM subscribes to another
     publisher) so that `scope` or `scope: ""` must be set;
  6. that management port 9090 (`/refresh`, `/participants-removed`) must stay cluster-internal; link
     the release-notes entry, since `/refresh` is open on every CM ≥ 3.5.3, eventing on or off.
- `doc/README.md`: link the new page under the existing `### Data Space Eventing` subsection.
- `doc/release-notes/10-x.md`: add a new top section for the minor version, in the existing
  format. Cover:
  - the CM and decentralizedIam bumps, with the image changes and any TIL DB migration (Deviation 2,
    listing what a default upgrade does change);
  - that CM ≥ 3.5.3 serves an unauthenticated `POST :9090/refresh` in **every** deployment with CM
    enabled, not only with eventing on, so port 9090 must stay cluster-internal (no Service, Ingress or
    route for it; a NetworkPolicy where pod IPs are reachable), and the `ENDPOINTS_REFRESH_SENSITIVE=true`
    opt-out: copy the full default `contract-management.additionalEnvVars` list into the values and add
    the entry to it, because Helm replaces the list (Deviation 2);
  - the new values and the registration scope;
  - that an external TIL must be ≥ 0.10.1;
  - the gateway routes;
  - that no eventing resource appears with default values.
- `charts/data-space-connector/Chart.yaml`: minor version bump (10.15.10 → 10.16.0, or the next
  minor if the base has moved).
- `AGENTS.md`: update the contract-management and decentralizedIam version mentions, if any, and
  the two stale eventing lines (see "Stale AGENTS.md lines"), unless a plan-mode agent already did;
  add the required host binding of the events routes and the discovery route next to them.
- Acceptance: the link check passes (lychee fallback above), `helm lint` and `helm unittest` pass.
