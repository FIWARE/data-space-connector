# Implementation Plan: Replace Consent-Manager in the FDSC with the new implementation

## Overview
Replace the Prometheus-X consent-manager (Node, MongoDB, shared consent key, authority
APISIX facade routes) with `dsc-consent-manager` (Java, PostgreSQL, a stateless OAuth2
resource server that validates VCVerifier JWTs itself; chart
`oci://quay.io/fiware/helm-charts/dsc-consent-manager` 0.1.0, API base `/v1`, API port 8080,
management port 9090). Charts, k3s scenario, IT tests and docs all change. The
consent-facade stays as the contract service at the authority and at the provider.

Decisions from the analysis (ticket comments), binding for every step:
- **Auth:** every caller (participants and data subjects) gets a JWT from the authority
  VCVerifier (service `consent-manager`, audience `consent-manager`). The consent-manager's
  `identityProviders.primary` trusts that verifier. Its claim paths and `roleMapping`
  (`consent-user`, `consent-participant`, `consent-catalog`) must match what the verifier
  puts in the token. The consent key (`x-visionstrust-consent-key`, `CONSENT_KEY`) and the
  authority APISIX facade routes that injected it are removed.
- **DB:** the consent MongoDB is removed. The service uses the trust-anchor's Zalando
  `managedPostgres`, with an extra user and database. The operator-generated secret is
  wired into `config.database.url` and `secrets.existingSecret`.
- **consent-facade:** unchanged in role. It is `config.contractServiceUrl` at the
  authority, and the provider-local instance stays as the token service of the plugin.
- **ADRs 0001-0005** are marked *Superseded* by a new ADR `0008`.
- **consent-filter plugin** (`quay.io/seamware/consent-plugin`): its source is not in this
  repo, and the human updates it. Here only the image tag and route config change. The
  tag is one value (see Step 4) so that publishing it needs a one-line bump.

Out of scope: the consent-owner-resolver (unchanged) and the DSC chart's own
`managedMongo`. The marketplace (BAE) still uses the latter, so it stays, as do
`k3s/operators.yaml` and the mongo-operator.

## Steps

### Step 1: trust-anchor chart -- swap to dsc-consent-manager on PostgreSQL
Files: `charts/trust-anchor/Chart.yaml`, `charts/trust-anchor/values.yaml`,
`charts/trust-anchor/templates/{mongodb.yaml,consent-network-policies.yaml,postgres.yaml,_helpers.tpl}`,
`charts/trust-anchor/tests/consent_management_test.yaml`, `charts/trust-anchor/Chart.lock`
(if present), and `charts/trust-anchor/README.md` (if present; regenerate the values table).

- `Chart.yaml`: replace the `consent-manager` 0.2.0 dependency with
  `name: dsc-consent-manager`, `version: 0.1.0`,
  `repository: oci://quay.io/fiware/helm-charts`, `alias: consent-manager`,
  `condition: consent-manager.enabled`. The alias keeps the values key and the
  `fullnameOverride: consent-manager` service name stable. Bump `version` to `2.0.0`:
  removing Mongo and the consent key breaks existing installs.
- Delete `templates/mongodb.yaml`, the `managedMongo` block and `consentManagement.mongo`
  from `values.yaml`, plus any `_helpers.tpl` helper used only by them (for example the
  `mongodb-*-password` generation). Add `templates/removed-values.yaml`, which `fail`s with
  a migration hint when `managedMongo`, `consentManagement.mongo`, `consent-manager.mongo`,
  `consent-manager.secret.consentKey` or `consent-manager.externalIdp` is still set.
  Follow the pattern of `charts/data-space-connector/templates/removed-values.yaml`.
- `managedPostgres.config`: add the user `consent_manager` (no extra flags) and the
  database `consent_manager: consent_manager`. Zalando creates the secret
  `consent-manager.<cluster>.credentials.postgresql.acid.zalan.do` (keys `username` /
  `password`). Check the real cluster name in `templates/postgres.yaml`. Expose these
  values with `# --` docs: `consentManagement.database.{user,name}`.
- `consent-manager:` values, using the new chart's keys: `enabled: false`,
  `fullnameOverride: consent-manager`,
  `config.contractServiceUrl: http://consent-facade:8080`, and
  `config.database.url: jdbc:postgresql://<cluster-svc>:5432/consent_manager`.
  Set `secrets.existingSecret` to a chart-owned Secret,
  `consent-manager-env`. A new template `templates/consent-manager-secret.yaml`, gated on
  `consent-manager.enabled`, builds it with `DB_USER`/`DB_PASSWORD` taken from the Zalando
  secret via `lookup`. That cannot work on the first install (the operator creates the
  secret later), so prefer this: `consent-manager.deployment` env `valueFrom` the Zalando
  secret, if the subchart offers that hook (check its `deployment` / `config.extraEnv`
  rules; `extraEnv` refuses credentials). Otherwise use a pre-install Job, the same way
  the authority's `ensure-ccsdb` job in `k3s/consent-trust-anchor.yaml` works. Record the
  choice in a `{{- /* */}}` comment. `LOG_PSEUDONYM_KEY` and `ERASURE_VERIFICATION_SECRET`
  are generated once and kept across upgrades with `lookup`, the same way the removed
  Mongo passwords were.
- Receipt signing key: the subchart never creates `<release>-signing-keys`. Add
  `consentManagement.signingKey.generate: true`, which runs a hook Job that generates an
  EC P-256 JWK into Secret `consent-manager-signing-keys` once, unless that Secret exists,
  and set `consent-manager.signingKeys.secretName` to it. The hook Job follows the strict
  pod security context and gets `priorityClassName` per the AGENTS.md scheduling rules.
- `identityProviders.primary` defaults stay the subchart placeholders. The authority
  values (Step 4) set them.
- `consent-network-policies.yaml`: the consent-manager policy targets port 8080
  (`consentManagementPolicies.consentManagerPort`, default 8080, was 3000). Allow ingress
  from the ingress controller, the consent-facade's clients and provider APISIX pods.
  There is no facade-only path any more: the service authenticates every call itself.
  Never allow the management port 9090 from outside the namespace. Update the comments
  that describe facade-only access.
- Rewrite `tests/consent_management_test.yaml` with one `it:` per behaviour:
  - the dependency renders a `consent-manager` Deployment on port 8080;
  - no MongoDBCommunity is rendered;
  - `managedPostgres` carries the consent user and database;
  - the NetworkPolicy port is 8080 and 9090 is not exposed;
  - each removed value fails;
  - the signing-key Job renders only when enabled.
- Acceptance: `helm dependency update charts/trust-anchor && helm lint charts/trust-anchor
  && helm unittest charts/trust-anchor` pass. `helm template` with
  `k3s/trust-anchor.yaml` renders.

### Step 2: data-space-connector chart -- drop consent Mongo/consent-key leftovers
Files: `charts/data-space-connector/values.yaml` (`consentManagement`, `consent-facade`,
`consent-owner-resolver` blocks at ~4123-4200), `templates/mongodb.yaml`,
`templates/removed-values.yaml`, `values.schema.json`, `README.md` (values table),
`Chart.yaml`.

- Remove the consent comment from `templates/mongodb.yaml` (lines ~46-48). The
  marketplace users stay.
- Rewrite the `consentManagement` / `consent-facade` docs. Remove "APISIX facade route"
  and "managed MongoDB". The provider-side plugin calls the authority's consent-manager
  `/v1` API directly with a VCVerifier JWT from the provider-local consent-facade.
- Grep the chart (`templates/`, `_realm.tpl`, `values.yaml`) for `consentKey`,
  `CONSENT_KEY`, `visionstrust`, `consent-user/`, `:3000`. Remove what is left. Any removed
  key under a subchart alias gets a `fail` in `templates/removed-values.yaml`.
- Regenerate the schema (`.github/scripts/values-schema.sh`) and the README values table.
  Bump `Chart.yaml` `version` to `10.16.0`.
- Acceptance: `helm lint`, `helm unittest charts/data-space-connector` and
  `.github/scripts/values-schema.sh --check` pass. `helm template` with
  `k3s/provider.yaml,k3s/consent-provider.yaml` renders.

### Step 3: authority VCVerifier -- map credentials to the consent-manager's claims
Files: `k3s/consent-trust-anchor.yaml` (keycloak realm/credential config and
`decentralizedIam.vcAuthentication.vcverifier` service `consent-manager`, ~lines
140-565), and `k3s/provider.yaml` / `k3s/consumer.yaml` where the participant and user
credentials are issued, if their claims have to change.

- Scope `participant` (MembershipCredential, used by providers' consent-facade and
  consumers): the token must carry the participant DID at the participant claim and the
  role `consent-participant` at the roles claim.
- Scope `openid` (UserCredential, data subjects): the token must carry the user DID as
  `sub` and the role `consent-user`.
- Implementation, preferred first:
  1. VCVerifier `jwtInclusion.claimsToInclude` (`originalKey` -> `newKey`) on each
     credential. Map the credential's role claim to `roles` and the subject/holder DID to
     `participant_id`. Then set the consent-manager claim paths to match (Step 4):
     `claims.roles: [roles]`, `claims.participantIdentifier: [participant_id]`,
     `claims.userIdentifier: [sub]`.
  2. If the credential carries no role, issue it with one. Add a `roles` claim with
     `consent-participant` / `consent-user` to the keycloak credential definitions
     (`keycloak.realm` client roles + protocol mapper in the issuing realm).
  Before choosing, check the VCVerifier version shipped by `decentralizedIam` for
  `claimsToInclude` support. Record the claim contract (path -> meaning) in a comment
  above the service and in the Step 6 docs.
- `consent-catalog`: map it only if a caller needs it (the consent-facade does not call
  the manager). Otherwise leave the default mapping and document that no credential grants it.
- Delete the authority APISIX facade routes to `consent-manager:3000`, the `CONSENT_KEY`
  env from the `consent-manager` secret, and their comments (~lines 566-720). The
  gateway now only routes the published host `consent-manager.dataspace-authority.org`
  to `consent-manager:8080` (path `/v1/*`, plus `/.well-known/jwks.json` for receipt
  verification), with no header injection and no `openid-connect` plugin. Alternatively
  replace it with a plain Ingress if APISIX is no longer needed at the authority.
- Acceptance: `helm template consent-authority charts/data-space-connector -f
  k3s/consent-trust-anchor.yaml` renders and validates against the schema. A rendered
  verifier config shows both scopes with the claim mapping.

### Step 4: k3s scenario and build wiring
Files: `k3s/consent-trust-anchor-components.yaml`, `k3s/consent-provider.yaml`,
`k3s/provider.yaml` (consent route ~lines 500-560, `CONSENT_SELF_DESCRIPTION_BASE_URL`
comment ~850), `k3s/deploy.sh` (comments only, unless ordering changes), `pom.xml`
(`consent` profile ~line 510), `.github/workflows/test.yaml` (comment ~line 40).

- `consent-trust-anchor-components.yaml`: drop `managedMongo`, `consent-manager.mongo`,
  `secret.consentKey`, `externalIdp`, `appEndpoint`. Set the new chart's keys:
  - `config.consentManagerUrl: https://consent-manager.dataspace-authority.org`;
  - `config.contractServiceUrl: http://consent-facade.trust-anchor.svc.cluster.local:8080`;
  - `identityProviders.primary.issuer: https://verifier.dataspace-authority.org`;
  - `discoveryUrl: .../services/consent-manager/.well-known/openid-configuration`;
  - `audience: consent-manager`;
  - claim paths and `roleMapping` from Step 3;
  - `allowInsecureTransport` only if discovery is http in-cluster;
  - `deployment.replicaCount: 1`, smaller resources for k3s.
  Postgres comes from Step 1. Leave SMTP empty, which disables notifications.
- The trust-anchor Postgres now also has to be up for the consent-manager. Re-check the
  `--wait=false` / ordering notes in the `pom.xml` `consent` profile and `deploy.sh`, and
  update their comments. Mongo is gone from the trust-anchor; the provider's mongo stays.
- `provider.yaml` route `mp-data-service-consent`:
  - point `consent_api_url` at
    `http://consent-manager.trust-anchor.svc.cluster.local:8080` with
    `consent_api_prefix: /v1`;
  - drop `consent_api_host`;
  - keep `owner_resolver_url`, `service`, `token_service_url`, `token_audience`,
    `fail_open`, `audit_*`;
  - add `check_mode: list`.
  Rewrite the route comment (~lines 506-520) for the new single-call check.
- The plugin image tag: one definition in `k3s/consent-provider.yaml` (or a `pom.xml`
  property if the tag is templated there). Set it to the placeholder
  `CONSENT_PLUGIN_TAG_TBD`, with a `TODO(ticket-109)` comment saying the human publishes
  it. List every place that tag is read in the PR description.
- Update the `.github/workflows/test.yaml` comment (no "managed mongo").
- Acceptance: every `helm template` call the `consent` profile makes renders, which you
  can check by running the commands from `deploy.sh` with `--dry-run=client` or
  `helm template`. `mvn -Pconsent` validation phases are optional; the full deploy needs
  the published plugin tag.

### Step 5: IT tests -- move to the dsc-consent-manager /v1 API
Files: `it/src/test/java/org/fiware/dataspace/it/components/ConsentEnvironment.java`,
`ConsentStepDefinitions.java`, `it/src/test/resources/it/consent_management.feature`, and
`KubernetesHelper.java` if it addresses Mongo or `:3000`.

- `ConsentEnvironment`: replace the facade URLs (`/consent-manager/v1`,
  `/consent-user/v1`) with one `CONSENT_MANAGER_API =
  "https://consent-manager.dataspace-authority.org/v1"`. The service port becomes 8080.
  Drop the consent-key constants. Keep the verifier URL and audience. Each public constant
  gets a one-line Javadoc.
- `ConsentStepDefinitions`, following the new OpenAPI
  (`wistefan/dsc-consent-manager` `api/openapi.yaml`):
  - participant onboarding and the data-subject `POST /v1/users/register` use VCVerifier
    tokens (scopes `participant` / `openid`);
  - privacy notices come from `GET /v1/privacy-notices` (filter by the contract URI; there
    is no GET by contract);
  - grant with `POST /v1/consents`, withdraw via the consent's revoke operation;
  - assert with `GET /v1/consents?userIdentifier=` and `GET /v1/consents/check`.
  Remove the `identifier/search` and `consents/participants` calls.
- Keep the three scenarios in `consent_management.feature` (deny without consent, allow
  after grant, deny after revoke). Rename steps only where the wording names the old API.
- Acceptance: `mvn -f it/pom.xml test-compile` passes. The `@consent` scenario passes
  with `mvn clean integration-test -Ptest,consent,consent-test` once the plugin tag from
  Step 4 is published. Note this dependency in the PR.

### Step 6: Documentation, ADRs and release notes
Files: `doc/CONSENT_MANAGEMENT.md`, `doc/img/consent/*.drawio` (and the PNGs where they
can be re-exported; otherwise update the mermaid/text diagrams in the doc and mark the
PNGs as outdated), `doc/adr/0001`-`0005`, new `doc/adr/0008-dsc-consent-manager.md`,
`doc/adr/README.md`, `doc/README.md`, `README.md`, `charts/README.md`,
`doc/release-notes/10-x.md`, `AGENTS.md` (Consent Management section).

- Rewrite `doc/CONSENT_MANAGEMENT.md` for the new architecture:
  - components: the dsc-consent-manager at the authority on Zalando Postgres, the
    authority and provider consent-facades as the contract service, the owner-resolver,
    and the provider consent-filter plugin;
  - the auth model: the VCVerifier JWT, the claim and role contract from Step 3, and
    which credential grants which role;
  - the `/v1` API flows (register, privacy notices, grant, list, check, revoke,
    receipts / JWKS);
  - the plugin config (`check_mode`, `consent_api_prefix`);
  - deployment values for both charts and the k3s `consent` profile;
  - migration from the old consent-manager. There is no data migration from Mongo:
    consents must be re-granted.
- New ADR 0008 in the existing ADR format: context, decision, consequences. ADRs
  0001-0005 get `Status: Superseded by 0008`. Update the index in `doc/adr/README.md`.
- Release notes: in `doc/release-notes/10-x.md`, a 10.16.0 entry with the breaking change
  for consent users and the trust-anchor chart 2.0.0.
- Update the links and summaries in `doc/README.md`, `README.md` and `charts/README.md`.
  `.github/scripts/check-links.sh` must pass.
- Update the AGENTS.md Consent Management section to the final state: drop "being
  migrated" and update the chart versions.

### Step 7: End-to-end verification and cleanup
- Run the full static suite:
  - `helm dependency update` + `helm lint` + `helm unittest` for both charts;
  - `.github/scripts/values-schema.sh --check`;
  - `.github/scripts/check-links.sh`;
  - `mvn -f it/pom.xml test-compile`.
- `grep -rn -i "consentKey\|CONSENT_KEY\|visionstrust\|mongodb-consent\|consent-manager:3000\|prometheus-x"`
  over `charts k3s it doc README.md pom.xml .github` must return only historical mentions
  in superseded ADRs and release notes. Fix any other hit.
- If the plugin tag is published by then, bump the placeholder from Step 4 and run
  `mvn clean deploy -Pconsent` followed by
  `mvn clean integration-test -Ptest,consent,consent-test`, and fix the failures. If it
  is not published, record it as the only open item in the completion comment.
