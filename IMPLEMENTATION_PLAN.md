# Implementation Plan: [DSC] Chart: participant side of data space eventing

## Overview
Wire contract-management's (CM) data space eventing listener/subscriber into the umbrella chart, off
by default: `contract-management.dataspace.events.*` values, a refreshable publishers ConfigMap
(rendered by the CM subchart), `scope`/`scopeType` on `registration.issuer`, the APISIX routes for
the events path (ordinary `decentralizedIam.odrlAuthorization.apisix.routes` entries the operator
adds, from a documented example the umbrella validates), an example ODRL policy, tests and docs. An upgrade with default values must not create or change any
eventing-related resource; the other changes it does bring are listed under "Deviations from the
ticket".

**AGENTS.md.** The "Data Space Eventing" section of `AGENTS.md` matches this plan again: the events
routes are a documented snippet in the normal `routes` list (plan update after the Step 4 review
question), and CM chart 3.6.6 (Prerequisite A, pinned by Step 3) renders
`contract-management.dataspace.events` into its refreshable ConfigMap.

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
  3.5.3; DSC pinned 3.6.3, then 3.6.5 in Step 2, and pins **3.6.6** since Step 3).
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
  is TIL **0.10.0** and has no scopes. The other 2.1.x releases (up to 2.1.30) deploy the same;
  2.3.0 is the first with TIL ≥ 0.10.1 (TIL chart 0.19.2, TIL 0.10.3, see Prerequisite B). The
  umbrella sets no TIL image override.
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
    `traditional`; `decoupled` control plane): a hook Job PUTs each entry to the admin API with the
    id `<release>-<index>` (`/apisix/admin/routes/<release>-<index>`). The entry's `name` goes only
    into the route body (verified in odrl-authorization 2.5.0 `routes-script.yaml`).
  - Ingress-controller mode: one `ApisixRoute` CR per entry.

  Helm replaces lists instead of merging them. A default route in the umbrella's list would therefore
  be lost as soon as a user sets `routes`. But the umbrella declares no `routes` default at all: the
  list is entirely operator-owned (see `k3s/provider.yaml`), every existing DSC route lives there, and
  every entry may carry a `name`, which odrl-authorization 2.5.0 (shipped by decentralized-iam 2.3.0)
  carries through all three paths: verbatim in the yaml ConfigMap, as the `name` field of the route
  body in the admin-API path, and as `<name>-route` in the `ApisixRoute` metadata. `name` is a label,
  never the route id, so duplicate names do not overwrite each other in yaml or admin-API mode; the
  validation checks every entry that carries one of the three events names.
  In ingress-controller mode the `ApisixRoute` template reads only `host`, `uri`, `upstream.nodes`
  and `plugins`: it drops `methods`, `hosts` and `priority`. The `OPTIONS` handshake route would then
  match every method on `/events` without authentication, so the events routes are refused in that
  mode (Step 4).
- **Rejected option: an umbrella hook Job that PUTs the route to the admin API** (the pattern of
  `templates/apisix-otel-global-rule-job.yaml`):
  - It only works in the etcd-backed modes. The default yaml mode has no admin API, so the Job would
    wait forever and fail the install.
  - It cannot clean up. odrl-authorization only PUTs its own `<release>-<index>` ids and never
    deletes, so a route PUT from outside it survives turning the feature off and `helm uninstall`.
  - It does not cover the ingress-controller mode.

  The existing OTEL global-rule hook has the same yaml-mode limitation. That is outside this ticket
  and is noted for a follow-up.
- **Rejected option: a new odrl-authorization `apisix.extraRoutes` map** (the earlier Prerequisite B).
  It would only buy two things: route defaults shipped by the umbrella that survive a user-set `routes`
  list, and a `DELETE` of a disabled route in the etcd-backed modes. Neither is worth an upstream
  chart change and two releases: the umbrella ships no route defaults anywhere else (the events routes
  need a deployment-specific host and verifier service anyway, so a default could not be enabled
  as-is), and the stale-route problem in etcd mode (`<release>-<index>` ids are only PUT, never
  deleted) applies to every route in the list today and belongs in odrl-authorization as a general
  fix, not an events feature.
- **Chosen option (Step 4): the normal `routes` list.** The operator appends three named entries
  (`dataspace-events`, `dataspace-events-handshake`, `dataspace-events-discovery`) to their own
  `decentralizedIam.odrlAuthorization.apisix.routes`, copied from an example values file that the
  repo ships and the docs explain (Steps 4 and 6). The umbrella finds them by `name` and `fail`s on
  settings that weaken access control; consistency between the entries is a guide checklist (Step 4,
  "Scope of the checks"). Removing them works in yaml mode (the ConfigMap is rewritten);
  in the etcd-backed modes it leaves stale `<release>-<index>` routes, as for any route removed from
  the list today, which the docs state.
- **How the yaml-mode ConfigMap reaches APISIX** (read from odrl-authorization 2.5.0, apisix chart
  2.18.2, APISIX 3.19.0): `apisix.deployment.standalone.existingConfigMap: apisix-routes` is mounted
  as a directory at `/apisix-config`, **without `subPath`**, and the container command symlinks
  `/usr/local/apisix/conf/apisix.yaml` to `/apisix-config/apisix.yaml`; the rendered file ends with
  the `#END` marker standalone mode requires. The pod's `checksum/config` annotation covers only the
  apisix chart's own `config.yaml`, not `apisix-routes`, so a route change does not roll the pod. It
  relies on the kubelet updating the mounted file (within its sync period, about a minute) and on
  standalone APISIX re-reading `apisix.yaml` when its modification time changes. Step 4 confirms the
  reload in its APISIX container run.
- CI runs `helm unittest charts/data-space-connector` **without** `helm dependency build`, so suites
  can only target the umbrella's own `templates/`. Subchart rendering is checked in the upstream PRs
  and by local `helm template` runs (Steps 3 and 4). The umbrella's own templates can still read
  subchart values (`.Values.decentralizedIam...`), so validation across values is testable in CI.
- `contract-management` is an open (subchart) key in `values.schema.json`, so declaring keys under it
  does not change the schema. Still run `.github/scripts/values-schema.sh --check`.
- There is no helm-docs-generated values table in this repo. The `# --` comments are still required.
  If a step finds a generated table, regenerate it; otherwise report that there is none.

**Prerequisite A (outside this repo, blocks Step 3): contract-management chart change.**
**Met:** CM chart **3.6.6** (appVersion 3.5.3) ships `dataspace.events`, the `<fullname>-dataspace-events`
ConfigMap and the startup checksum; Step 3 pins it. The history below is kept for reference.
Status 2026-11-13 (history; superseded by 3.6.6 above): CM chart **3.6.5** is released. It only bumps `appVersion` to 3.5.3 (the last bullet
below), and Step 2 picks it up. It has no `dataspace.events` value and no events ConfigMap, and
FIWARE/helm-charts `main` (`bcc4207`) has neither. Every other bullet below is still open, and
Step 3 waits for a chart version > 3.6.5 that has them.
The Gitea fork `general-agent-3/helm-charts` exists but is stale (main `56cd6c9`, CM chart 3.5.23,
far behind upstream 3.6.5). Before Step 3 can start, a human must either sync that fork's `main` to
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

**Prerequisite B (blocks Step 4): a decentralized-iam release with TIL ≥ 0.10.1.**
**Met:** decentralized-iam **2.3.0** pins vc-authentication 1.6.0 → `trusted-issuers-list` chart
0.19.2 (TIL 0.10.3) and odrl-authorization 2.5.0. No odrl-authorization change is needed any more
(see "APISIX routes"; the `extraRoutes` patch drafted in the blocked Step 4 run is dropped). Step 4
itself finds out whether TIL 0.10.1+ migrates its database at start-up (its Flyway/Liquibase
migrations in FIWARE/trusted-issuers-list) and records it for the release notes.

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
   - every other change between decentralized-iam 2.1.28 and the new release comes along;
   - a new `fail` path: `templates/eventing-validation.yaml` (Step 4) is on by default. It only
     inspects routes named `dataspace-events`, `dataspace-events-handshake` or
     `dataspace-events-discovery`, which no release has before this feature, so an upgrade without
     them cannot fail. An existing route that happens to carry one of those names can fail
     `helm upgrade`; the fix is to rename it or set `eventingRouteValidation.enabled: false`.

   The criterion this plan enforces instead:
   - no eventing ConfigMap, route, env entry or volume appears;
   - the registration Job script is byte-identical;
   - every other diff is listed in the Step 2, 3 and 4 PR descriptions.
3. **The chart ships no events route; the ticket item is replaced.** The ticket (`doc/eventing/ticket.md`
   "DSC-1") asks for an "Events path on the contract-management ingress (and, if the subchart does
   not support it, `FIWARE/helm-charts` first)". This plan delivers it as an example values file plus
   a `fail`-only validation instead (Step 4). There is no route in the chart and no upstream chart
   change. Signing off on this plan signs off on that replacement. Consequences:
   - **Two switches.** The operator appends the routes to their `routes` list, separately from
     `contract-management.dataspace.events.listener.enabled`. A subchart value cannot depend on
     another value, and host and verifier service are deployment-specific. The umbrella does not
     check that the two agree: routes without the listener answer `404`, which a first delivery shows
     at once. A listener without the routes is allowed, for operators who run their own gateway.
   - **Checks are opt-in by name.** The umbrella finds the routes by their `name`. A renamed or
     unnamed copy is not checked at all. The example header and the Step 6 guide say "keep the
     names". An earlier revision also caught an unnamed route at `/events` to the release's CM
     Service. It is dropped: it needed a copy of the CM chart's fullname helper that CI cannot check
     against the subchart (so it would silently stop matching after a CM bump), three name forms,
     two node shapes and a re-run local check on every CM bump. In return it caught only a renamed
     copy with exactly that upstream and path, and it added an upgrade `fail` for routes no
     operator named for this feature.
   - **Access-control checks only.** The validation `fail`s only on settings that weaken access
     control without breaking delivery (seven checks, Step 4 "Scope of the checks"). Consistency
     between the three entries (matching `uri`s and hosts, the discovery service path, entries
     present, `methods: [POST]`, the listener flag) is a checklist in the Step 6 guide: a mistake
     there breaks the handshake, discovery or delivery visibly. An earlier revision of this plan
     checked those too (about 20 rules); they are dropped because every APISIX or
     odrl-authorization bump can invalidate Go-template logic that CI cannot test against the
     subchart, and a wrong rule blocks install and upgrade.
   - **One opt-out.** The validation still has `fail` conditions on operator-owned routes, and a
     setup it does not foresee (another auth chain, an `X-Userinfo` strip through a global rule, an
     APISIX upgrade that moves `openid-connect`'s priority) would block install and upgrade.
     `eventingRouteValidation.enabled: false` (umbrella key, default `true`, Step 4) switches the
     whole validation off; the operator then owns the route security, as with their own gateway.
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
- **Result (PR #20).** Fix agents cannot edit a PR description, so the evidence is recorded here.
  A human must copy it into the PR #20 description before merging.
  `helm template` used the umbrella's `values.yaml`, rendering CM chart 3.6.3 against 3.6.5. Both
  renders are 214 lines, and the diff has exactly these changes:
  - the ConfigMap, Service and Deployment metadata labels, and the pod-template labels:
    `helm.sh/chart` `contract-management-3.6.3` → `contract-management-3.6.5`, and
    `app.kubernetes.io/version` `"3.4.2"` → `"3.5.3"`. The pod rolls once;
  - the container image: `quay.io/fiware/contract-management:3.4.2` → `3.5.3`.

  The diff adds no ConfigMap, env entry, volume, port or eventing resource. The Service still
  exposes only 8080, and the mounted `application.yaml` is unchanged. One change does not show in
  the render: the app now answers an unauthenticated `POST :9090/refresh` (Deviation 2).

### Step 3: Bump contract-management and declare `contract-management.dataspace.events`
Unblocked by CM chart 3.6.6 (Prerequisite A).
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

### Step 4: Bump decentralizedIam; validate the events routes
Prerequisite B is met (decentralized-iam 2.3.0). The events routes are ordinary entries of the
operator's `decentralizedIam.odrlAuthorization.apisix.routes` list (see "APISIX routes"); the chart
ships an example and validates it, but adds no route itself.
- Before anything else, confirm that decentralized-iam 2.3.0 is published and that its
  odrl-authorization (2.5.0) carries `name` through all three render paths, with the admin-API ids
  still `<release>-<index>` (as read from the 2.5.0 chart while revising this plan; see "APISIX
  routes"). If not, write `blocked`; if the ids are keyed by name, fix the stale-route text here and
  in Step 6 first. Also confirm how the yaml-mode ConfigMap reaches APISIX (see "How the yaml-mode
  ConfigMap reaches APISIX": a directory mount without `subPath`, a symlink, no checksum on
  `apisix-routes`); if 2.3.0 mounts it with `subPath` or rolls the pod on it, fix the Step 6
  restart text first.
- `charts/data-space-connector/Chart.yaml`: set the `decentralizedIam` dependency to **2.3.0**.
  Diff `helm template` with default values before and after, and list every change in the PR
  description: the TIL image (≥ 0.10.1, and whether it migrates its DB at start-up), the
  odrl-authorization and vc-authentication versions, labels, and any other change that comes with
  the release (Deviation 2). No route may appear. If 2.2.0/2.3.0 changed or removed a value the
  umbrella's `values.yaml` sets under `decentralizedIam`, adapt it and say so.
- New example values file `doc/eventing/examples/events-routes.yaml`: a
  `decentralizedIam.odrlAuthorization.apisix.routes` list with the three named entries below, every
  deployment-specific field a visible placeholder (`events.example.org`, `<events-service>`), and a
  header comment saying the entries must be **appended** to the operator's own `routes` list (Helm
  replaces lists, so passing this file with `-f` next to another `routes` list drops one of them)
  and that the three `name`s must be kept, because the chart checks the routes by name only.
  The `values.yaml` `odrlAuthorization.apisix` block gets a one-line comment pointing at the file;
  no `routes` key is declared there.
  - `name: dataspace-events`: `methods: [POST]`, `uri: /events`, upstream `contract-management:8080`.
    The name comes from the umbrella's `contract-management.fullnameOverride` and matches the existing
    CM route in `k3s/provider.yaml`. Do not trust that: check name and port against the Service in a
    `helm template` render, and say in a comment that `upstream.nodes` must change when
    `fullnameOverride` changes or CM runs outside the release. Plugins:
    - `openid-connect` with `bearer_only`, `use_jwks` and `set_userinfo_header: true`; `discovery`
      points at the verifier's **external** URL,
      `https://verifier.example.org/services/<events-service>/.well-known/openid-configuration`, as
      every route in `k3s/provider.yaml` does (with `proxy_opts` where the gateway needs a proxy to
      reach it; show it commented out).
      - Why external: the verifier builds `issuer` and `jwks_uri` in its metadata from its configured
        `server.host` (`https://verifier.mp-operations.org` in k3s), not from the URL the metadata was
        fetched from. So `openid-connect` fetches the JWKS from that external host whichever
        discovery URL it uses, and an internal `http://verifier:3000/...` URL saves nothing.
      - An internal URL works too when the gateway can reach the advertised `jwks_uri`. Say so in the
        example comment.
      - Verify this in Step 4 against the shipped verifier (metadata from both URLs is identical) and
        lua-resty-openidc (`bearer_jwt_verify` checks `iss` against the discovery document's
        `issuer`), and record the result in the PR description.
    - `opa` (as on the existing routes).
    - A `proxy-rewrite` with `headers.remove: [X-Userinfo]`, which removes a client-sent `X-Userinfo`
      **before** `openid-connect` sets it.
      APISIX runs higher priorities first (`openid-connect` 2599, `proxy-rewrite` 1008), so the
      rewrite needs `_meta.priority` above 2599. Verify this against the APISIX version that ships and
      against the CM README (~148–170): that it honours a plugin's `_meta.priority`, that both plugins
      run in the `rewrite` phase, and that `proxy-rewrite` applies `headers.remove` there (so the
      header is gone before `openid-connect` sets it). If any of the three does not hold, write
      completion status `blocked` and say which; do not ship the example or the priority check
      against a mechanism that has no effect. Also record whether the shipped `openid-connect`
      overwrites (or clears) a client-sent `X-Userinfo` with `bearer_only` (see the "Why" of the
      priority check below); that sets the rationale, not whether the check ships.
  - `name: dataspace-events-handshake`: `methods: [OPTIONS]`, `uri: /events`, the same upstream, no
    auth plugins.
  - `name: dataspace-events-discovery`: `methods: [GET]`,
    `uri: /events/.well-known/openid-configuration`, upstream `verifier:3000`, no auth plugins, and a
    `proxy-rewrite` whose `uri` is `/services/<events-service>/.well-known/openid-configuration`.
    - Why: on a `401` the publisher's OID4VP client fetches `<sink>/.well-known/openid-configuration`
      (specification §5.1 step 1), and the callee MUST serve it for every protected path, with the
      events scope in `scopes_supported`. Without this route that `GET` hits the host's catch-all: a
      `401` behind `openid-connect`, a `404`, or on the k3s CM host the priority-100 `/*` +
      `vars` route (`k3s/provider.yaml` ~443–454) that serves the **contract-management** service's
      metadata, which lacks the events scope. The publisher can then never get a token.
    - The path assumes the announced sink is `<scheme>://<host>/events`. Say in a comment that a sink
      under another path needs the `uri` of all three entries changed (the chart does not check that
      they agree; it is on the Step 6 checklist).
    - `<events-service>` is the verifier service of Step 6 item 3, and the same service that
      `dataspace-events`' `openid-connect.discovery` points at.
    - `verifier:3000` comes from `decentralizedIam.vcAuthentication.vcverifier.fullnameOverride:
      verifier` and the k3s routes. Check it against the rendered Service, as for the CM upstream.
  - **All three are bound to the sink host** (`host: events.example.org`).
    - Why: APISIX's default router `radixtree_host_uri` tries host-bound routes first and falls back
      to host-less routes only when no host-bound route matches. DSC deployments bind their routes to
      hosts, with `/*` catch-alls (e.g. `k3s/provider.yaml`, `tpp-data-service`,
      `contract-management`). A host-less `/events` would lose to the catch-all of whatever host the
      publisher delivers to, which applies the wrong verifier scope and policy. The host must be the
      host of the sink URL the subscriber announces.
    - APISIX rejects an empty binding: `hosts` has `minItems: 1` in `apisix/schema_def.lua`; with
      `hosts: []` the yaml mode logs "failed to check item data" and drops the route, and the
      admin-API mode answers the PUT with 400, which fails the hook Job. APISIX does not allow `host`
      and `hosts` on the same route (a `not`/`anyOf` clause in the route schema). The existing routes
      use the singular `host`; mention `hosts` in a comment.
    - Verify against the APISIX version that ships: the `minItems`, the host/hosts exclusion, the
      host-first fallback of `radixtree_host_uri`, and that the exact `/events` and
      `/events/.well-known/openid-configuration` beat a `/*` on the **same** host, including the
      priority-100 `/*` + `vars` discovery route (lua-resty-radixtree matches the exact path before
      prefixes, and `priority` only orders routes of the same path). If a `/*` wins, give all three
      entries a `priority` above the catch-alls in the example and say so in the PR description.
- New template `templates/eventing-validation.yaml`: no output, `fail` only. The whole file is gated
  on all of:
  - `eventingRouteValidation.enabled` (new umbrella key, see below);
  - `decentralizedIam.enabled`;
  - `decentralizedIam.odrlAuthorization.enabled`, the condition decentralized-iam 2.3.0's
    `Chart.yaml` puts on odrl-authorization (its `values.yaml` defaults it to `true`).

  Every check **except the ingress-controller refusal** is further gated on
  `decentralizedIam.odrlAuthorization.apisix.enabled`, the condition odrl-authorization 2.5.0's
  `Chart.yaml` puts on the `apisix` dependency (default `true`). In 2.5.0 only `routes.yaml` is gated
  on `.Values.enabled` alone; `routes-script.yaml` additionally requires `.Values.apisix.enabled`, and
  `routes-job.yaml` is gated on `and .Values.enabled .Values.apisix.enabled`. So with
  `apisix.enabled: false` the admin script and Job do not render, and the `apisix-routes` ConfigMap
  still renders but no gateway reads it. `k3s/consumer.yaml` uses this shape.
  The ingress-controller refusal (below) is **not** gated on `apisix.enabled`: `routes.yaml` renders
  one `ApisixRoute` per entry whenever `.Values.enabled` and `apisix.ingress-controller.enabled` are
  true, even with the bundled APISIX off, and an ingress controller deployed outside the release that
  watches the same `ingressClass` (default `apisix`) would serve those method- and host-less routes.

  **How a subchart switch is read.** The umbrella declares neither `decentralizedIam.odrlAuthorization.enabled`
  nor `...apisix.enabled`, and CI builds no dependencies, so an **absent** key counts as deployed.
  All three gate switches (`decentralizedIam.enabled`, `odrlAuthorization.enabled`,
  `odrlAuthorization.apisix.enabled`) use one form: a switch is off **only when it is the boolean
  `false`**, i.e. `$v := dig "enabled" true $map` and off is `and (kindIs "bool" $v) (not $v)`.
  - Why not `ne (toString $v) "false"`: Helm evaluates a dependency `condition` only when it is a
    bool. A string `"false"` (`--set-string`) logs "returned non-bool value", is ignored, and the
    subchart **is deployed** (checked with Helm 3.17.3 and a toy parent/child chart); the subchart's
    own `if .Values.enabled` / `and .Values.apisix.enabled` then see a non-empty, truthy string. So a
    string `"false"` must count as on, or the validation would skip a gateway that is running.
  - `ingress-controller.enabled` is read the way `routes.yaml` reads it, Go-template truthiness
    (`if`): absent (the umbrella declares `false`), `false` or empty is off, anything else, including
    the string `"false"`, is on.

  Treating absent as off would make every failure case pass in CI. With any gate switch off, no
  bundled gateway serves the routes, so route values left in an operator's file (e.g. after switching
  to an external gateway) are inert and must not fail the install.

  `eventingRouteValidation.enabled` is the escape hatch (Deviation 3, "One opt-out"): a top-level
  umbrella key, so it is not rendered into CM's Micronaut config the way a key under
  `contract-management.dataspace.events` would be. Declare it in `values.yaml` as
  `eventingRouteValidation: {enabled: true}` with a `# --` comment saying that `false` skips every
  check below and leaves the route security to the operator, and regenerate `values.schema.json`.
  One switch, not one per check: the checks that guard against a spoofed identity or an open
  `POST /events` are the same ones an unforeseen setup trips (another auth chain, a global-rule strip
  and a moved priority all hit the `X-Userinfo`/priority checks), so a partial switch would not help
  such a setup, and per-check switches would add a value for every rule. Test that the key skips the
  whole file.

  **Scope of the checks.** A check ships only if it guards access control: a misconfiguration it
  catches lets a request through that should be refused, or drops the policy, and the event flow may
  still work, so nobody notices. Consistency between the three entries is **not** checked: a mistake
  there (a handshake `uri` that differs, hosts that differ, a discovery rewrite to the wrong verifier
  service, a missing handshake or discovery entry, `dataspace-events` without `methods: [POST]`, the
  listener off) breaks the handshake, the token fetch or the delivery, and the first test delivery
  shows it. Those items are the Step 6 checklist (Deviation 3, "Access-control checks only"). Do not
  add them back without human sign-off.

  The template walks `decentralizedIam.odrlAuthorization.apisix.routes` (the list and every key may be
  absent: read them as described under "Reading subchart values", starting from
  `index .Values "decentralizedIam"`) and checks **every** entry whose `name` is one of the three, so a
  duplicated name is checked twice rather than refused. It reads no `contract-management` value. The
  checks, one `fail` message each (the message names the route and the setting):
  1. **Ingress controller** (any of the three names present):
     `decentralizedIam.odrlAuthorization.apisix.ingress-controller.enabled` is on. The subchart's
     `ApisixRoute` template drops `methods` and `hosts`, so the handshake route would match every
     method on `/events` without authentication. This check runs even with `apisix.enabled: false`
     (see the gate above), so it is the only one that can fire then.
  2. **Handshake method** (`dataspace-events-handshake`): `methods` is not exactly `[OPTIONS]`.
     Without `methods` APISIX matches every method, so a forgotten key opens `POST /events` without
     authentication.
  3. **Discovery method** (`dataspace-events-discovery`): `methods` is not exactly `[GET]`, for the
     same reason.
  4. **Host binding** (`dataspace-events`): neither a non-empty `host` nor a non-empty `hosts`. A
     host-less `/events` loses to the host-bound `/*` catch-all of the sink host (see "All three are
     bound to the sink host"), which may accept the events token under another verifier scope and
     ODRL policy. Handshake and discovery are not checked here: a host-less copy of them only breaks
     the flow. Both keys set or `hosts: []` is rejected by APISIX itself.
  5. **Identity header** (`dataspace-events`): `openid-connect.set_userinfo_header` is not the boolean
     `true` (APISIX's plugin schema rejects a string, so a string is refused here too).
  6. **Policy** (`dataspace-events`): no `opa` plugin. The ODRL policy of Step 5 only takes effect
     through `opa`; the verifier scope alone admits any holder of the events credential, so trimming
     `opa` silently drops the policy.
  7. **`X-Userinfo` strip** (`dataspace-events`): no `proxy-rewrite` whose `headers.remove` contains
     `X-Userinfo` (compared case-insensitively, as HTTP header names are), or that `proxy-rewrite`'s
     `_meta.priority` is missing or not above `openid-connect`'s **effective** priority:
     `openid-connect._meta.priority` if the route sets it, else the default below.
     - Why: CM does not verify tokens and trusts `X-Userinfo` for its source == identity check.
       With `bearer_only`, `openid-connect` answers `401` without a valid token and on success
       replaces `X-Userinfo` (`core.request.set_header`; APISIX master also clears the client value
       first). So on this route the strip is defence in depth, as the CM README asks, and the
       priority check mainly prevents a functional break: a `proxy-rewrite` at its default 1008 runs
       after `openid-connect` and removes the header it just set, so CM gets no identity. Step 4
       verifies the overwrite against the shipped APISIX and states the result in the template
       comment and the Step 6 guide; if the shipped version does **not** overwrite it, the strip is
       the security control and the comment says so.
     - Define `openid-connect`'s default priority as a named constant in the template, verified
       against the APISIX version that ships (2599 in the APISIX source as of planning), with a
       comment naming that version.

  Every check above depends on the exact names. An operator who renames the entries, drops `name`
  while merging lists, or writes their own events routes gets none of them (Deviation 3). There is
  no guard for unnamed routes (dropped, see Deviation 3); the example header and the Step 6 guide
  say to keep the names.

  A listener without the routes stays valid, and so do routes without the listener (Deviation 3).
- Extend `tests/eventing_test.yaml`, parameterized, with one row per line of the tables below.
  Fixtures: **E** = the three entries of the example file next to an unrelated route; **F** = E with
  `opa` removed from `dataspace-events` (fails check 6 only); **I** = E plus
  `apisix.ingress-controller.enabled: true` (fails check 1 only). "absent" means the test does not
  set the key (CI builds no dependencies, so the subchart default is not merged in).

  (a) Gate and opt-out:

  | # | Values | Expected |
  |---|--------|----------|
  | G1 | chart defaults | renders nothing |
  | G2 | F, I, each with `eventingRouteValidation.enabled: false` | renders nothing |
  | G3 | F, I, each with `decentralizedIam.enabled: false` | renders nothing |
  | G4 | F, I, each with `decentralizedIam.odrlAuthorization.enabled: false` | renders nothing |
  | G5 | F with `odrlAuthorization.apisix.enabled: false` | renders nothing |
  | G6 | I with `odrlAuthorization.apisix.enabled: false` | fails, check 1 |
  | G7 | F with `odrlAuthorization.enabled` and `apisix.enabled` absent | fails, check 6 |
  | G8 | F with each of the three gate switches set to the **string** `"false"` (one row each) | fails, check 6 |
  | G9 | E with `ingress-controller.enabled: "false"` (a truthy string) | fails, check 1 |
  | G10 | only routes without the three names, `ingress-controller.enabled: true` | renders nothing |

  (b) One row per check, each case a parameterized line, asserting `failedTemplate` with the
  message fragment:

  | Check | Cases (on E unless stated) | Message fragment |
  |-------|----------------------------|------------------|
  | 1 | I | `ingress-controller` |
  | 2 | handshake `methods` absent, `[OPTIONS, POST]`, `[GET]`; a second `dataspace-events-handshake` without `methods` | `dataspace-events-handshake`, `methods` |
  | 3 | discovery `methods` absent, `[GET, POST]`, `[POST]` | `dataspace-events-discovery`, `methods` |
  | 4 | `dataspace-events` without `host`/`hosts`, `host: ""`, `hosts: []` | `dataspace-events`, `host` |
  | 5 | `set_userinfo_header` absent, `false`, `"true"` | `set_userinfo_header` |
  | 6 | F | `opa` |
  | 7 | no `proxy-rewrite`; `proxy-rewrite` without `headers.remove`; `headers.remove` without `X-Userinfo`; `_meta.priority` absent, `1008`, `2599`; `2600` with `openid-connect._meta.priority: 3000` | `X-Userinfo` or `priority` |

  (c) Passing variants (render nothing):

  | # | Values |
  |---|--------|
  | P1 | E (singular `host` on all three) |
  | P2 | E with `dataspace-events` `hosts: [a]`, and with `hosts: [a, b]` |
  | P3 | E with `headers.remove: [x-userinfo]` |
  | P4 | E with `openid-connect._meta.priority: 3000` and the rewrite at `3001` |
  | P5 | E with handshake and discovery entries removed, with a handshake `uri` other than the events `uri`, with hosts that differ between the entries, with the discovery rewrite `/.well-known/openid-configuration`, with `dataspace-events` `methods` absent, and with `listener.enabled: false` (consistency is not checked; pins "Scope of the checks") |
  | P6 | an unnamed route at `/events` to `contract-management:8080` without `methods` (no guard, Deviation 3) |
- Local only, not in CI: after `helm dependency update`, render with the example entries appended
  to a `routes` list that holds an unrelated route, in the default yaml mode and in
  `traditional`+`etcd` mode. Check that the `apisix-routes` ConfigMap and the admin script both
  contain the unrelated route and all three events routes, under their `name` (the admin script
  PUTs them with the ids `<release>-<index>`). Then run the rendered
  yaml-mode config in an APISIX container of the shipped version, with a host-bound `/*` catch-all on
  the same host and on another host, plus a copy of the k3s priority-100 `/*` + `vars` discovery
  route on the same host. Show with `curl` that `POST /events`, `OPTIONS /events` and
  `GET /events/.well-known/openid-configuration` on the bound host reach the events routes, the last
  one with the rewritten path, and that `POST /events` does not reach the handshake route (an echo
  upstream is enough). Then mount the config as a directory with the symlink the apisix chart uses,
  replace the file the way the kubelet does (a `..data` symlink swap) with one events route removed,
  and show that APISIX drops it without a restart. Paste the results into the PR description.
- `values-schema.sh` (regenerate for `eventingRouteValidation`; `decentralizedIam` is an open key),
  `helm lint`, `helm unittest`,
  lychee (the example file is linked from `values.yaml` only as a comment; Step 6 links it from the
  docs).
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
  2. the gateway: append the three entries of `doc/eventing/examples/events-routes.yaml` (Step 4)
     to the operator's own `decentralizedIam.odrlAuthorization.apisix.routes` list (Helm replaces
     lists), set `discovery` and the discovery route's rewrite `uri` to the events verifier service,
     and bind all three to the host of the announced sink URL with `host` or `hosts` (never empty, not
     both). Override `upstream.nodes` if CM is not `contract-management:8080` or the verifier not
     `verifier:3000`; change the `uri`s if the sink path is not `/events`. Explain:
     - that removing the entries again leaves stale `<release>-<index>` routes in the etcd-backed
       APISIX modes (odrl-authorization PUTs each entry under the id `<release>-<index>`, `name`
       is only a field of the route body, and it never deletes, as for any route removed from the
       list; shortening the list leaves the trailing ids behind), and how to delete them through the
       admin API;
     - that the ingress-controller mode is refused (Step 4) and why;
     - keep the three `name`s, or the chart cannot check the routes: the checks find them by name,
       and a renamed or unnamed copy is not checked at all (Deviation 3);
     - when a route change takes effect: in the default yaml mode the kubelet updates the mounted
       `apisix-routes` file within its sync period (about a minute) and APISIX reloads it, with no
       restart (the Step 4 result; if Step 4 found otherwise, say `kubectl rollout restart` of the
       APISIX Deployment instead); in the etcd-backed modes the routes hook Job PUTs them on
       install/upgrade;
     - why `discovery` uses the verifier's external URL (Step 4: `issuer` and `jwks_uri` come from the
       verifier's `server.host`), with `proxy_opts` where the gateway needs a proxy;
     - which settings the chart enforces (the seven checks of Step 4: no ingress controller,
       `[OPTIONS]` on the handshake, `[GET]` on discovery, and on `dataspace-events` a host,
       `set_userinfo_header`, `opa`, the `X-Userinfo` strip and its priority above
       `openid-connect`'s effective priority) and why (for the strip: the Step 4 result on whether
       `openid-connect` overwrites a client-sent header);
     - a checklist of what the chart does **not** check, because a mistake there breaks the flow
       visibly rather than weakening access control (Step 4, "Scope of the checks"): all three
       entries present; `dataspace-events` has `methods: [POST]` (otherwise it may shadow the
       handshake, which then answers `401`); the handshake `uri` equals the events `uri` and the
       discovery `uri` is the events `uri` + `/.well-known/openid-configuration`; all three bind the
       same hosts; the discovery rewrite `uri` is `/services/<id>/.well-known/openid-configuration`
       for the same verifier service as `dataspace-events`' `discovery` URL (a gateway path prefix in
       front of the verifier in the `discovery` URL is fine; the discovery route's upstream must
       reach the same verifier); `contract-management.dataspace.events.listener.enabled: true` when
       CM runs in this release. Then send one test event: the handshake, the token fetch and the
       delivery each fail visibly when an item is wrong;
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
       including the discovery route and removing `X-Userinfo`; the route checks do not run then (they
       only run when `decentralizedIam` deploys odrl-authorization with its bundled APISIX, i.e.
       none of `decentralizedIam.enabled`, `odrlAuthorization.enabled`,
       `odrlAuthorization.apisix.enabled` is the boolean `false`, Step 4). The ingress-controller
       refusal still runs with `apisix.enabled: false`, because odrl-authorization still renders
       `ApisixRoute` CRs that an external controller may serve;
     - `eventingRouteValidation.enabled: false` as the opt-out when a correct setup is refused: it
       switches off every check, so the operator then verifies the list of enforced settings above
       by hand;
  3. a verifier service registration
     (`decentralizedIam.vcAuthentication.vcverifier.registration.services`), whose id is the
     `<events-service>` of the `dataspace-events` and `dataspace-events-discovery` routes, and whose
     scope is the one subscribers request:
     - `jwtInclusion` maps `credentialSubject.id` to the top-level claim `participantId`;
     - check the field names in the credentials-config-service API;
  4. the ODRL policy from Step 5 (`it/src/test/resources/policies/allowDataSpaceEvents.json`). Say:
     - it admits `POST` (`tmf:create`, a plain method check in ODRL-PAP) on `/events` for an
       `OperatorCredential` whose issuer (`vc:currentParty`) is the publisher,
       `did:web:dataspace-authority.org` in k3s. Every participant's Keycloak issues
       `OperatorCredential`, so the type alone would admit any participant's operator: the operator
       must replace the issuer DID with their data space publisher's id, or use a dedicated
       credential type;
     - the events verifier service must still trust only the publisher's issuer
       (`trustedIssuersLists`/`trustedParticipantsLists`); the policy is the second line, CM's
       source == identity check on `X-Userinfo` the third;
     - `vc:type` and `vc:currentParty` read the credential from the token, so the events service's
       `jwtInclusion` must keep it there (`fullInclusion: true`, as in k3s); check in the
       credentials-config-service API that this combines with the `participantId` mapping (item 3);
  5. that `registration.issuer` grants are scoped (Step 1) and need TIL ≥ 0.10.1, and when the
     default scope is wrong (Step 1: a non-membership grant, or a TIL whose CM subscribes to another
     publisher) so that `scope` or `scope: ""` must be set;
  6. that management port 9090 (`/refresh`, `/participants-removed`) must stay cluster-internal; link
     the release-notes entry, since `/refresh` is open on every CM ≥ 3.5.3, eventing on or off.
- `doc/README.md`: link the new page under the existing `### Data Space Eventing` subsection.
- New ADR `doc/adr/0008-events-routes-as-validated-operator-snippet.md` (AGENTS.md: every
  architectural decision gets an ADR), in the MADR-lite format of the existing ADRs, and a row for it
  in `doc/adr/README.md` (Status `Accepted`, "Also in" empty; name `doc/eventing/participant-setup.md`
  as the document it belongs to). This plan file is not kept after the ticket, so the ADR is where
  the reasoning survives. Content, from "APISIX routes" and Deviation 3:
  - context: the events routes need a deployment-specific host and verifier service, Helm replaces
    lists, the umbrella declares no `routes` default, odrl-authorization's three render modes;
  - decision: the chart ships no events route; operators append the three named entries of
    `doc/eventing/examples/events-routes.yaml`, and `templates/eventing-validation.yaml` checks them
    by `name`, with `eventingRouteValidation.enabled` as the single opt-out;
  - consequences: two switches, checks are opt-in by name, only access-control checks (consistency is
    a guide checklist), a new upgrade `fail` path for routes that
    carry the names, stale `<release>-<index>` routes in the etcd-backed modes on removal, the
    ingress-controller mode refused;
  - rejected alternatives: an odrl-authorization `apisix.extraRoutes` map, an umbrella hook Job
    PUTting to the admin API, the unnamed-route guard, and full consistency validation of the three
    entries (with the reason for each, as in this plan).

  Link the ADR from `participant-setup.md` and from `doc/eventing/architecture.md` §12 as a one-line
  note (no new D-x entry: §12 records design decisions of the protocol, the ADR records this chart's
  packaging decision).
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
  - the gateway routes, their validation and the `eventingRouteValidation.enabled` opt-out;
  - that `helm upgrade` can now `fail` on an existing route named `dataspace-events`,
    `dataspace-events-handshake` or `dataspace-events-discovery` that does not pass the checks
    (Deviation 2), and the fix: rename it or set `eventingRouteValidation.enabled: false`;
  - that no eventing resource appears with default values.
- `charts/data-space-connector/values.yaml`, the `contract-management.tracing` comment (~line 1927):
  drop the `--set contract-management.additionalEnvVars[0].value=false` hint. Helm replaces the list,
  so it renders `[{value: false}]` (verified with Helm 3.17: one entry, no `name`, a boolean value) and
  drops every `OTEL_*` entry. Say instead: copy the full default list into a values file and change
  `OTEL_SDK_DISABLED` there, as the release notes say for `ENDPOINTS_REFRESH_SENSITIVE`. The
  scorpio (`additionalEnvVars[0]`), tm-forum-api (`defaultConfig.additionalEnvVars[4]`) and
  marketplace (`extraEnvVars[0]`) tracing comments carry the same hint. They are outside this
  ticket; name them in the PR description as a follow-up.
- `charts/data-space-connector/Chart.yaml`: minor version bump (10.15.10 → 10.16.0, or the next
  minor if the base has moved).
- `AGENTS.md`: update the contract-management and decentralizedIam version mentions, if any;
  next to the "documented snippet" line, name the example file, the validation by route `name`, the
  required host binding and the discovery route.
- Acceptance: the link check passes (lychee fallback above), `helm lint` and `helm unittest` pass.
