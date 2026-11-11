# Implementation Plan: [DSC] Chart: participant side of data space eventing

## Overview
Wire contract-management's (CM) data space eventing listener/subscriber into the umbrella chart, off
by default: `contract-management.dataspace.events.*` values, a refreshable publishers ConfigMap
(rendered by the CM subchart), `scope`/`scopeType` on `registration.issuer`, the APISIX route for
the events path, an example ODRL policy, tests and docs. An upgrade with default values must not
change any eventing-related resource.

**Sources the step agents must read** (read-only, clone outside the repo):
- Design: `doc/eventing/architecture.md` §5–§9, §12 (D-5, D-8, D-12); `doc/eventing/specification.md`
  (§5 authentication binding, §6 authorization, §9 handshake); `doc/eventing/ticket.md` "DSC-1".
- CM application: `git clone https://github.com/FIWARE/contract-management /tmp/cm` (main @ `c7da3c4`,
  release 3.5.3). Property tree: `src/main/java/org/fiware/iam/configuration/DataSpaceEventsProperties.java`
  (`@ConfigurationProperties("dataspace.events")`); commented example in
  `src/main/resources/application.yaml` (lines ~115–209); deployment notes in `README.md` (~148–170
  gateway/`X-Userinfo`, ~742–760 refresh and mounting). Eventing ships in CM 3.5.0, the TIR watcher
  in 3.5.1, `on-participant-removed` and adoption in **3.5.3**.
- CM chart: `https://github.com/FIWARE/helm-charts` `charts/contract-management` (3.6.4, appVersion
  3.5.1; DSC pins 3.6.3).
- TIL API: `https://github.com/FIWARE/trusted-issuers-list` `api/trusted-issuers-list.yaml`.
  `POST /issuer` takes `scope` and `scopeType` as **query parameters**; `scopeType` is the enum
  `dataspace-membership` (the ticket's `dataspace` is not a valid value — the API wins). Scope types
  exist from TIL 0.10.1.

**Facts that shape the plan** (verified while planning):
- CM reads the eventing config from Micronaut config files (`MICRONAUT_CONFIG_FILES`, comma list) and
  re-reads it on a `micronaut.io.watch` event under `dataspace.events.listener.watch.path` (reacts to
  the kubelet `..data` symlink swap) or on `POST :9090/refresh`. A `subPath` mount never updates.
  Startup-only keys: the `enabled` flags of listener/publisher/subscriber/tir-watcher,
  `listener.watch.*`, `publisher.sink-*-timeout`. Everything else reloads at runtime.
- The CM chart 3.6.4 has no eventing values. It mounts `application.yaml` with `subPath` and sets
  `MICRONAUT_CONFIG_FILES=/application.yaml` itself. A `config:` override replaces the whole file.
  The chart has no Ingress or route template.
- CM does not verify tokens. The API gateway must run `openid-connect` with
  `set_userinfo_header: true` and must drop any `X-Userinfo` sent by the client. CM then reads the
  caller id from the claim `dataspace.events.listener.identity.participant-claim` (`participantId`).
- The `dataspace` removal mode revokes TIL grants with scope = publisher id and
  scopeType = `DATASPACE_MEMBERSHIP` (`ParticipantAdoption.grantMembership(did, publisherId, …)`).
- The umbrella's APISIX routes are the plain list `decentralizedIam.odrlAuthorization.apisix.routes`,
  inside a subchart (odrl-authorization 2.4.x renders it with `toYaml`). The umbrella cannot append a
  route gated on a feature flag without replacing the user's list, and the subchart has no
  extra-routes extension point. The route is therefore shipped as a documented, copy-ready snippet
  (Step 4), not rendered.
- CI runs `helm unittest charts/data-space-connector` **without** `helm dependency build`, so suites
  can only target the umbrella's own `templates/`. The subchart's rendering is asserted in the CM
  chart's PR and by a local `helm template` check (Step 2).
- `contract-management` is an open (subchart) key in `values.schema.json`, so declaring keys under it
  does not change the schema. Still run `.github/scripts/values-schema.sh --check`.
- There is no helm-docs-generated values table in this repo. The `# --` comments are still required.
  If a step finds a generated table, regenerate it; otherwise report that there is none.

**Prerequisite (outside this repo, blocks Step 2): contract-management chart change.**
The Gitea fork `general-agent-3/helm-charts` exists but is stale (main `56cd6c9`, CM chart 3.5.23,
far behind upstream 3.6.4). Before Step 2 can start, a human must either sync that fork's `main` to
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
- Bump `appVersion` to 3.5.3 (the first CM release with `on-participant-removed`).
- Version via the PR label; FIWARE/helm-charts bumps chart versions in CI (see its `AGENTS.md`).
If this prerequisite is not met when Step 2 is picked up, Step 2 writes completion status `blocked`
and names the missing chart version.

**Link check in the agent container.** `.github/scripts/check-links.sh` runs lychee in Docker with a
bind mount that the sidecar cannot see ("lychee.toml not found"). That is a tooling failure. Use the
static musl lychee 0.24.2 binary instead:
`lychee --config lychee.toml --root-dir "$(pwd)" '**/*.md'`. It must report 0 errors.

## Steps

### Step 1: Attribute `registration.issuer` grants to the data space publisher
Umbrella-only; not blocked by the prerequisite.
- `charts/data-space-connector/templates/participant-registration.yaml`: send `scope` and
  `scopeType` as query parameters on `POST {til}/issuer`, URL-encoded (`urlquery`).
  - Per issuer entry, optional `scope` and `scopeType`. If `scope` is unset, it defaults to the `id`
    of the one entry in `contract-management.dataspace.events.publishers` whose `types` contain
    `org.fiware.dataspace.participant.removed`. Read it with `dig`/`default`; the key may be absent.
  - `scopeType` defaults to `dataspace-membership` whenever a scope is in effect.
  - An explicit `scope: ""` opts the entry out (unscoped, as today).
  - If several publishers subscribe to `participant.removed` and an entry sets no scope, `fail` with a
    message saying to set `scope` per issuer.
  - With no such publisher the generated script must be **byte-identical** to today's (upgrade no-op).
- Add the `scope`/`scopeType` keys to the commented `registration.issuer` example in `values.yaml`
  with `# --` text. The text must say that the Job's existing `409` path leaves already-registered
  issuers unscoped; CM's `on-participant-removed.adopt-unattributed` covers those.
- Check which TIL image `decentralizedIam` 2.1.28 deploys. If it is older than 0.10.1, say so in the
  values comment and the PR description; do not bump `decentralizedIam` in this step.
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

### Step 2: Bump contract-management and declare `contract-management.dataspace.events`
Blocked until the prerequisite chart version is released.
- `charts/data-space-connector/Chart.yaml`: set the `contract-management` dependency to the new
  chart version.
- Diff `helm template` with default values before and after, and list every change in the PR
  description. Expected: the image moves to appVersion 3.5.3 and chart labels change. Nothing
  eventing-related may appear.
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

### Step 3: Example ODRL policy for the events path
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

### Step 4: Gateway snippet, docs, release notes, version bump
- New `doc/eventing/participant-setup.md` (doctoc ToC). Write it as an operator guide and link to
  specification/architecture for the reasons. Content:
  1. the values to enable the listener/subscriber, and which keys are startup-only (a rollout
     follows) vs. refreshed at runtime;
  2. a copy-ready `decentralizedIam.odrlAuthorization.apisix.routes` snippet:
     - a `POST /events` route to `contract-management:8080` with `openid-connect`
       (`bearer_only`, `use_jwks`, `set_userinfo_header: true`, discovery at the verifier service
       of the events scope) and `opa`;
     - a `proxy-rewrite` that removes a client-sent `X-Userinfo` before the plugins run (check the
       plugin order against the CM README);
     - a separate unauthenticated `OPTIONS /events` route;
     - say explicitly why it is not rendered by the chart (subchart list, no extension point);
  3. a verifier service registration
     (`decentralizedIam.vcAuthentication.vcverifier.registration.services`):
     - `jwtInclusion` maps `credentialSubject.id` to the top-level claim `participantId`;
     - check the field names in the credentials-config-service API;
  4. the ODRL policy from Step 3;
  5. that `registration.issuer` grants are scoped (Step 1);
  6. that management port 9090 (`/refresh`, `/participants-removed`) must stay cluster-internal.
- `doc/README.md`: link the new page under the existing `### Data Space Eventing` subsection.
- `doc/release-notes/10-x.md`: add a new top section for the minor version, in the existing
  format. Cover the dependency bump and image change, the new values, the registration scope, the
  route snippet, and "no change with default values".
- `charts/data-space-connector/Chart.yaml`: minor version bump (10.15.10 → 10.16.0, or the next
  minor if the base has moved).
- `AGENTS.md`: update the contract-management version mentions if any.
- Acceptance: the link check passes (lychee fallback above), `helm lint` and `helm unittest` pass.
