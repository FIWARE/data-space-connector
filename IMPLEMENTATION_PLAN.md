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
  must match what the verifier puts in the token (Step 3 settled on the credential type
  as the role: `MembershipCredential` -> participant, `UserCredential` -> user). The
  consent key (`x-visionstrust-consent-key`, `CONSENT_KEY`) and the authority APISIX
  facade routes that injected it are removed.
- **DB:** the consent MongoDB is removed. The service uses the trust-anchor's Zalando
  `managedPostgres`, with an extra user and database. A plain (non-hook) setup Job copies
  the operator-generated credentials into the Secret named by `secrets.existingSecret`;
  `config.database.url` points at the cluster service. Nothing depends on `lookup`, so
  `helm install` and the `helm template | kubectl apply` path (ADR 0007, CI, GitOps)
  behave the same.
- **consent-facade:** unchanged in role. It is `config.contractServiceUrl` at the
  authority, and the provider-local instance stays as the token service of the plugin.
- **ADRs 0001-0005** are marked *Superseded* by a new ADR `0008`.
- **consent-filter plugin** (`quay.io/seamware/consent-plugin`, source
  `github.com/SEAMWARE/consent-plugin`, not in this repo): release **1.0.0** speaks only the
  dsc-consent-manager API. Here only the image tag (one value, Step 4) and the route config
  change. What 1.0.0 needs from the rest of the scenario:
  - route options: `consent_api_url` + `consent_api_prefix` (default `/v1`), `check_mode`
    (`list` default, or `check`), `owner_resolver_url`, `token_service_url`,
    `token_audience` (default `consent-manager`), `consumer_claim` (default
    `verifiableCredential.issuer`), optional `privacy_notice_id`, `purpose`,
    `require_purpose`, `require_contract`. `consent_api_host` and `consent_key` /
    `CONSENT_KEY` are removed (they only log a warning if still set);
  - the token from the provider consent-facade (`POST /internal/tokens`) must carry the
    `PARTICIPANT` role and the participant-identifier claim: the plugin calls
    `GET /v1/participants/me` for the provider DID it matches consents on;
  - the consumer DID taken from `consumer_claim` must be a registered participant with a
    `selfDescriptionUri` (`GET /v1/participants?identifier=`), else the request is denied;
  - a consent allows only when `consented`, `provider` = provider DID and `consumer` =
    consumer DID, plus `contractUri` / `privacyNoticeId` / purpose when the owner-resolver
    (or the route) names them.

Out of scope: the consent-owner-resolver (unchanged) and the DSC chart's own
`managedMongo`. The marketplace (BAE) still uses the latter, so it stays, as do
`k3s/operators.yaml` and the mongo-operator.

## Steps

### Step 1: trust-anchor chart -- swap to dsc-consent-manager on PostgreSQL
Files: `charts/trust-anchor/Chart.yaml`, `charts/trust-anchor/values.yaml`,
`charts/trust-anchor/templates/{mongodb.yaml,consent-network-policies.yaml,postgres.yaml,_helpers.tpl}`,
new `charts/trust-anchor/templates/{consent-manager-setup-job.yaml,removed-values.yaml}`,
`charts/trust-anchor/tests/consent_management_test.yaml`, `charts/trust-anchor/Chart.lock`
(if present), `charts/trust-anchor/README.md` (if present; regenerate the values table),
the chart-key part of `k3s/consent-trust-anchor-components.yaml` and the authority APISIX
routes in `k3s/consent-trust-anchor.yaml` (see the last two bullets).

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
  `consent-manager.postgres.credentials.postgresql.acid.zalan.do` (keys `username` /
  `password`; cluster `postgres` per `templates/postgres.yaml`, underscores become dashes).
  The user and database name are **hard-coded** to `consent_manager`, not exposed as
  values: `postgres.yaml` renders `managedPostgres.config` verbatim, the Zalando secret
  name the Job reads derives from the user, and `consent-manager.config.database.url` is a
  subchart value the umbrella cannot template, so a configurable name would have three
  places to keep in sync and a mismatch leaves the Job waiting on a secret that never
  appears. Say so in a `{{- /* */}}` comment in the Job and a `# --` line next to the
  `managedPostgres` entries. The operator creates databases only at initial provisioning
  (see the `ensure-ccsdb` comment in `k3s/consent-trust-anchor.yaml`), so an upgrade from
  1.x or a reused volume would lack the database; the setup Job below covers that.
- External Postgres is out of scope for 2.0.0: the setup Job depends on the Zalando
  secrets. `consent-manager-setup-job.yaml` `fail`s with a clear message when
  `consent-manager.enabled` is true and `managedPostgres.enabled` is false.
- `consent-manager:` values, using the new chart's keys: `enabled: false`,
  `nameOverride: consent-manager`, `fullnameOverride: consent-manager`,
  `config.contractServiceUrl: http://consent-facade:8080`,
  `config.database.url: jdbc:postgresql://postgres:5432/consent_manager`,
  `secrets.existingSecret: consent-manager-env` and
  `signingKeys.secretName: consent-manager-signing-keys`. The subchart has no env
  `valueFrom` hook (its `deployment.yaml` renders only `envFrom` + a fixed `env`, and
  `extraEnv` refuses `DB_*`), and `lookup` is empty under `helm template` (the subchart's
  own `secret.yaml` warns the keys then rotate on every render). So neither Secret is a
  Helm-templated object. Both NetworkPolicies select `app.kubernetes.io/name:
  consent-manager`. Helm already renames an aliased dependency's `.Chart.Name` to the
  alias, so `nameOverride` is not strictly needed; set it anyway so the label does not
  depend on the alias, and pin the rendered label in a unit test.
- New `templates/consent-manager-setup-job.yaml`, gated on `consent-manager.enabled`. It
  is the **single owner** of both Secrets, so nothing has two writers:
  - A **plain Job, not a Helm hook** (same as `ensure-ccsdb`, which is a plain Job in
    `extraManifests`): a pre-install hook would deadlock, because the Zalando secret only
    appears after the operator reconciles the `postgresql` CR, a normal release resource.
    `restartPolicy: OnFailure`, `backoffLimit: 30`, `activeDeadlineSeconds: 900`, and
    `ttlSecondsAfterFinished` from the new value `consentManagement.setup.ttlSecondsAfterFinished`
    (default `300`, rendered with `{{- with }}`, so `null` omits the field). GitOps users with self-heal (Argo CD) unset it:
    with a TTL, Argo recreates the Job after every cleanup and the app flaps OutOfSync. The
    script is idempotent, so that loop is noisy, not harmful. The consent-manager Deployment sits in
    `CreateContainerConfigError` until `consent-manager-env` exists and recovers on its own;
    `deploy.sh --wait` covers that window (check its timeout, Step 4).
  - Name it `consent-manager-setup-{{ .Release.Revision }}-<hash>`, where `<hash>` is
    `sha256sum` of the rendered pod spec (script, images, the referenced values), truncated
    to 8 characters. The revision part covers `helm upgrade` (see the rotation-Job note in
    AGENTS.md). The hash covers the `helm template | kubectl apply` path (the pom.xml
    deploys), where the revision is always 1: a changed spec gets a new Job instead of
    `field is immutable`, and an unchanged spec re-applies the same Job, which is a no-op
    while it exists and a harmless idempotent rerun after the short TTL removed it.
  - Init container 1, `create-db` (`postgres:16-alpine`): reads `PGUSER`/`PGPASSWORD` from
    the superuser secret `postgres.postgres.credentials.postgresql.acid.zalan.do` via
    `secretKeyRef` (the kubelet resolves it and the pod waits until it exists, so no RBAC)
    and runs `CREATE DATABASE consent_manager OWNER consent_manager` only when
    `pg_database` lacks it, exactly like `ensure-ccsdb`.
  - Init container 2, `generate-jwk`, only rendered when
    `consentManagement.signingKey.generate` (default `true`): image
    `consentManagement.setup.jwkImage` (default `smallstep/step-cli:0.28.7`, verified: runs
    as uid 1000 and ships `step`). It always writes a fresh candidate key into the shared
    `emptyDir` (env `HOME=/work`, so `step` has a writable config dir under
    `readOnlyRootFilesystem`); the main container decides whether to use it:
    `step crypto jwk create /work/pub.json /work/signing-key.jwk --kty EC --crv P-256
    --use sig --no-password --insecure` (a private JWK with `kid` and `alg: ES256`). No
    hand-rolled openssl/jq JWK encoding.
  - Main container `setup`: image `consentManagement.setup.image` (default
    `alpine/k8s:1.31.4`, verified to ship `kubectl`, `jq`, `base64` and `head` but **not**
    `openssl`; it defaults to root, so set `runAsUser`/`runAsGroup`, for example 1000).
    Set env `HOME=/work`: with uid 1000 and `readOnlyRootFilesystem` there is no writable
    home, so kubectl would print discovery-cache warnings to stderr on every call, and the
    script parses `kubectl get` stderr for `NotFound` (step 1 below).
    `DB_USER`/`DB_PASSWORD` come from the consent user's Zalando secret
    `consent-manager.postgres.credentials.postgresql.acid.zalan.do` (keys `username` /
    `password`) via `secretKeyRef`, and `SMTP_USER`/`SMTP_PASSWORD` from
    `consentManagement.smtp.existingSecret` when it is set (see below), so the script never
    reads those Secrets through the API. The script runs under `set -eu` and:
    1. reads `consent-manager-env` with `kubectl get secret ... -o json`, capturing stderr.
       Only two outcomes lead to generating a key: the call fails **and** stderr reports
       `NotFound` (no Secret yet: generate both), or it succeeds and
       `LOG_PSEUDONYM_KEY` / `ERASURE_VERIFICATION_SECRET` is absent or empty (generate
       only that one). Any other failure (Forbidden, timeout, API error) exits non-zero, so
       the Job retries instead of replacing live keys. Never `2>/dev/null || true`. New
       keys come from `head -c 32 /dev/urandom | base64`; existing keys must never rotate;
    2. applies `consent-manager-env` with server-side apply:
       `kubectl create secret generic --dry-run=client -o yaml | kubectl apply
       --server-side --field-manager=consent-manager-setup --force-conflicts -f -`. Unlike
       client-side apply it does not copy the data (DB password, keys) into the
       `last-applied-configuration` annotation. It needs the same verbs (create/patch), and
       because the field manager owns the keys it wrote, it still drops SMTP keys that are
       no longer configured. The Secret holds `DB_USER`, `DB_PASSWORD`, the two keys and,
       when configured, `SMTP_USER`/`SMTP_PASSWORD`. Without an SMTP secret it does not
       write the SMTP keys;
    3. when the JWK init container ran, checks `consent-manager-signing-keys` the same
       way as in 1: on `NotFound` it creates it from `/work/signing-key.jwk` under the data
       key `consent-manager.signingKeys.signingKeyFile` (default `signing-key.jwk`; the
       subchart mounts exactly that key, any other name fails at pod start), treating an
       `AlreadyExists` from a concurrent run as success; on any other error it exits
       non-zero. An existing Secret is **never** overwritten. `generate: false` means the
       operator provides that Secret.
  - SMTP credentials: new optional values `consentManagement.smtp.existingSecret` (default
    `""`), `consentManagement.smtp.userKey` (default `username`) and
    `consentManagement.smtp.passwordKey` (default `password`). When the secret is set the
    Job copies its two keys into `consent-manager-env`; the SMTP host and sender stay in
    the subchart's `consent-manager.config.smtp.*`. Changed SMTP credentials take effect on
    the next Job run (every `helm upgrade`; on the template path after the TTL or by
    deleting the Job), followed by a consent-manager restart, as the subchart documents.
    `removed-values.yaml` also `fail`s when `consent-manager.secrets.smtpUser` /
    `smtpPassword` are set, pointing to the new option, since the subchart ignores them
    once `existingSecret` is set.
  - Plain release resources (not hooks, so they exist before the Job runs): a
    ServiceAccount, a Role and a RoleBinding. No cluster-wide RBAC. The Role has two rules:
    `get`/`patch` on `secrets` with `resourceNames: [consent-manager-env,
    consent-manager-signing-keys]`, and `create` on `secrets` without `resourceNames`
    (Kubernetes cannot scope `create` by name). The Job cannot read any other Secret in
    the namespace, including the Postgres superuser and TIL credentials.
  - Strict pod security context (runAsNonRoot, readOnlyRootFilesystem with an `emptyDir`
    at `/work` for scratch files, drop ALL) and `priorityClassName` from
    `consentManagement.setup.priorityClassName` (`{{- with }}`, `nindent 8`) per the
    AGENTS.md scheduling rules. Record the "why a plain Job" and "why the hash in the name"
    reasoning in a `{{- /* */}}` block above it.
- `identityProviders.primary` defaults stay the subchart placeholders. The authority
  values (Step 4) set them.
- `consent-network-policies.yaml`, consent-manager policy:
  - Port: read the subchart's container port, `dig "deployment" "containerPorts" "http"`
    on the nil-safe `$manager` dict, falling back to
    `consentManagement.networkPolicy.consentManagerPort` (default changes 3000 -> 8080).
    Drop the `$manager.port` lookup: the new chart has no top-level `port`. NetworkPolicy
    ports match the container port, not the Service port.
  - Admitted peers, each only on that port:
    1. same-namespace `app.kubernetes.io/name: apisix` pods (the authority gateway that
       publishes `consent-manager.dataspace-authority.org`, see the last bullets of this
       step), as today;
    2. the ingress controller, from the existing
       `consentManagement.networkPolicy.ingressController` (`namespaceSelector` +
       `podSelector`), for setups that publish the manager with a plain Ingress;
    3. a new list `consentManagement.networkPolicy.consentManagerClients` (default `[]`) of
       `{namespace, podLabels}` entries, each rendered as a `namespaceSelector`
       (`kubernetes.io/metadata.name`) + `podSelector` peer. This admits the providers'
       APISIX, whose consent-filter plugin calls
       `consent-manager.trust-anchor.svc.cluster.local:8080` from another namespace
       (Step 4). `k3s/consent-trust-anchor-components.yaml` sets
       `{namespace: provider, podLabels: {app.kubernetes.io/name: apisix}}`.
  - The management port 9090 is never listed, so it stays unreachable for all of them.
  - There is no facade-only path any more: the service authenticates every call itself.
    Rewrite the comments that describe facade-only access and the consent key.
  - The consent-facade policy stays as it is: it admits only the consent-manager pods,
    because the manager calls the facade, not the other way round. Its `podSelector` on
    `app.kubernetes.io/name: consent-manager` depends on the `nameOverride` above.
- Rewrite `tests/consent_management_test.yaml` with one `it:` per behaviour:
  - the dependency renders a `consent-manager` Deployment on port 8080 whose pod label is
    `app.kubernetes.io/name: consent-manager` (the label both NetworkPolicies select);
  - no MongoDBCommunity is rendered;
  - `managedPostgres` carries the consent user and database;
  - the NetworkPolicy port is 8080, 9090 is not exposed, and each
    `consentManagerClients` entry renders a `namespaceSelector` + `podSelector` peer;
  - each removed value fails;
  - the setup Job, its ServiceAccount/Role/RoleBinding render without hook annotations
    and only when `consent-manager.enabled`; the Job name changes when the script or an
    image changes;
  - the Role grants `get`/`patch` only with `resourceNames` `consent-manager-env` and
    `consent-manager-signing-keys`, and `create` without `resourceNames`;
  - the script runs under `set -eu`, generates keys only on a `NotFound` read or an
    empty key and exits non-zero on any other read error, and applies
    `consent-manager-env` with `--server-side --field-manager=consent-manager-setup`;
  - `ttlSecondsAfterFinished` follows `consentManagement.setup.ttlSecondsAfterFinished`
    and is omitted when that is `null`;
  - the `generate-jwk` init container and the signing-key step are rendered only when
    `consentManagement.signingKey.generate` is true, and the script writes the data key
    named by `consent-manager.signingKeys.signingKeyFile`;
  - `SMTP_USER`/`SMTP_PASSWORD` are wired from `consentManagement.smtp.existingSecret` only
    when it is set;
  - `consent-manager.secrets.existingSecret` / `signingKeys.secretName` match the names
    the Job writes;
  - the `setup` and `generate-jwk` containers set `HOME=/work`;
  - rendering fails when `consent-manager.enabled` and not `managedPostgres.enabled`.
- `k3s/consent-trust-anchor-components.yaml`: the `fail`s above would break the `consent`
  profile (and its CI matrix leg) at render time from this step on, because the file still
  sets every removed key. So this step already drops `managedMongo`,
  `consent-manager.mongo`, `secret.consentKey`, `externalIdp`, `appEndpoint` and
  `contractService`, and sets the new chart's keys that need no later step:
  `config.consentManagerUrl: https://consent-manager.dataspace-authority.org`,
  `config.contractServiceUrl: http://consent-facade.trust-anchor.svc.cluster.local:8080`,
  `identityProviders.primary.{issuer,discoveryUrl,audience}` (Step 4 lists the values) and
  `consentManagement.networkPolicy.consentManagerClients`. It also sets the k3s-only
  verifier reachability variables in `consent-manager.config.extraEnv` (proxy, contract
  service `DIRECT`, trust-all; the list and the reasoning are in Step 4). They need
  nothing from a later step, and without them the consent-manager never gets Ready (its
  readiness probe includes IdP discovery), so `deploy.sh`'s final `wait_ready` and the
  template path's per-apply wait in `pom.xml` would time out before any IT runs. Step 4
  adds the claim mapping and runs the full verification gate.
- `k3s/consent-trust-anchor.yaml`, authority APISIX (~lines 566-720): the new chart
  renders no `consent-manager` Secret (`secrets.existingSecret` is set), so the
  `extraEnvVars` entry `CONSENT_KEY` from `secretKeyRef: {name: consent-manager, key:
  consentKey}` would leave the APISIX pod in `CreateContainerConfigError`. Delete that
  entry, the facade routes to `consent-manager:3000` that inject
  `x-visionstrust-consent-key`, and their comments. The gateway keeps only the published
  host `consent-manager.dataspace-authority.org`, routed to `consent-manager:8080` (path
  `/v1/*`, plus `/.well-known/jwks.json` for receipt verification), with no header
  injection and no `openid-connect` plugin. Alternatively replace it with a plain Ingress
  if APISIX is no longer needed at the authority (then the `ingressController` peer of
  the NetworkPolicy applies). The verifier/keycloak parts of the file stay for Step 3.
- Transition state: from this step on, the consent leg renders, deploys and gets Ready.
  Its IT is expected to fail until Steps 3-5 land (plugin 0.0.12 against the new API, no
  claim mapping yet); that is accepted, so do not chase those IT failures here.
- Acceptance: `helm dependency update charts/trust-anchor && helm lint charts/trust-anchor
  && helm unittest charts/trust-anchor` pass. `helm template` renders both with
  `-f k3s/trust-anchor.yaml` and with
  `-f k3s/trust-anchor.yaml -f k3s/consent-trust-anchor-components.yaml`, and
  `helm template consent-authority charts/data-space-connector -f
  k3s/consent-trust-anchor.yaml` renders with no `consentKey`/`CONSENT_KEY` left. If a
  cluster is available, `mvn clean deploy -Pconsent` completes (all pods Ready).

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
  `CONSENT_KEY`, `visionstrust`, `consent-user/`, `:3000`. On the plan branch this finds
  nothing: the chart's consent values are only `enabled` flags, the facade/resolver
  overrides and their docs, and none of them is removed. So this step changes comments
  and docs only and adds no `removed-values.yaml` entry.
- Regenerate the schema (`.github/scripts/values-schema.sh`) and the README values table.
  Bump `Chart.yaml` `version` to `10.16.0`: minor, because no DSC value is removed or
  changes meaning. The breaking changes live in the trust-anchor chart (2.0.0) and the
  provider's consent route / plugin config. If the implementer does have to remove a
  value that then `fail`s, bump to `11.0.0` instead and say so in the release note.
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
  role `consent-participant` at the roles claim, which `roleMapping` turns into
  `PARTICIPANT`. consent-plugin 1.0.0 depends on this: with the provider consent-facade's
  token it calls `GET /v1/participants/me` and `GET /v1/participants?identifier=`, which
  both require `PARTICIPANT` and the participant identifier.
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
- The authority APISIX facade routes and `CONSENT_KEY` are already gone (Step 1).
- Acceptance: `helm template consent-authority charts/data-space-connector -f
  k3s/consent-trust-anchor.yaml` renders and validates against the schema. A rendered
  verifier config shows both scopes with the claim mapping.
- Outcome: neither credential carries a role claim, so neither 1. as written nor 2. was
  used. VCVerifier 6.27.0 (`decentralizedIam` 2.1.28) copies the credential's `type` array
  to `roles` (`claimsToInclude`, scope `flatClaims: true`), and the consent-manager's
  `roleMapping` names the credential types instead of `consent-*` strings. The verifier
  already enforces the type per scope, so no participant's issuer can grant itself a
  consent role, and the credentials issued in `k3s/provider.yaml` / `k3s/consumer.yaml`
  stay unchanged. `participant_id` is the MembershipCredential's `issuer` (the DID the TIR
  check covers and the plugin's default `consumer_claim` reads), not `credentialSubject.id`,
  which an issuer sets freely; in k3s both participants self-issue, so the two are equal.
  The openid scope also maps `email`, `given_name` and `family_name` from the
  UserCredential. Both DCQL queries ask for one credential (`multiple: false`) because the
  verifier lets the last credential's claims win; it does not enforce `multiple`.
  Residual gap for Step 6 docs: VCVerifier 6.27.0 does not compare `vc.issuer` with the
  signed `iss` for jwt_vc_json, so a hand-crafted VC can still name another DID until
  upstream rejects that mismatch (it already does on the `vc+jwt` path).

### Step 4: k3s scenario and build wiring
Files: `k3s/consent-trust-anchor-components.yaml`, `k3s/consent-provider.yaml`,
`k3s/provider.yaml` (consent route ~lines 500-560, `CONSENT_SELF_DESCRIPTION_BASE_URL`
comment ~850), `k3s/deploy.sh` (comments only, unless ordering changes), `pom.xml`
(`consent` profile ~line 510), `.github/workflows/test.yaml` (comment ~line 40).

- `consent-trust-anchor-components.yaml` (Step 1 already removed the old keys and set the
  URLs and `consentManagerClients`). The `consent-manager` values end up as:
  - `config.consentManagerUrl: https://consent-manager.dataspace-authority.org` (Step 1);
  - `config.contractServiceUrl: http://consent-facade.trust-anchor.svc.cluster.local:8080`
    (Step 1);
  - `identityProviders.primary.issuer: https://verifier.dataspace-authority.org`,
    `discoveryUrl: https://verifier.dataspace-authority.org/services/consent-manager/.well-known/openid-configuration`,
    `audience: consent-manager` (Step 1). They stay https: the app compares the issuer
    byte-for-byte with the discovery document's `issuer`, and the verifier publishes its
    `jwks_uri` under the public host, so an in-cluster http `discoveryUrl` with
    `allowInsecureTransport` would still fail at the JWKS fetch;
  - claim paths and `roleMapping` from Step 3: `claims.userIdentifier: [sub]`,
    `claims.participantIdentifier: [participant_id]`, `claims.roles: [roles]`,
    `roleMapping.user: UserCredential`, `roleMapping.participant: MembershipCredential`.
    `roleMapping.catalog` keeps the default `consent-catalog`: no credential carries it;
  - `deployment.replicaCount: 1`, smaller resources for k3s.
  Postgres comes from Step 1. Leave SMTP empty, which disables notifications.
- Verifier reachability in k3s. `verifier.dataspace-authority.org` resolves to 127.0.0.1
  inside pods (CoreDNS only maps `*.127.0.0.1.nip.io`), and its ingress cert is signed by
  the cluster CA (`selfsigned-ca`). The old chart solved both with `externalIdp.proxy` and
  `externalIdp.caSecret`. dsc-consent-manager 0.1.0 has no proxy, CA or extra-volume
  option, but the app (Micronaut) builds its discovery client from the global HTTP-client
  settings (`IdentityProviderRegistry.createDiscoveryClient` copies the global proxy and
  SSL config), and the JWKS fetch uses the default client too. `config.extraEnv` refuses
  none of the variables below. Step 1 already sets them there, k3s only, with a comment;
  this step verifies them:
  - `MICRONAUT_HTTP_CLIENT_PROXY_TYPE: HTTP` and
    `MICRONAUT_HTTP_CLIENT_PROXY_ADDRESS: squid-proxy.infra.svc.cluster.local:8888`: the
    squid forward proxy routes the public hosts to traefik, as for every other component;
  - `MICRONAUT_HTTP_SERVICES_CONTRACT_SERVICE_PROXY_TYPE: DIRECT`: the global proxy would
    also catch the contract-service client (`@Client(id = "contract-service")`), and the
    consent-facade NetworkPolicy admits only consent-manager pods, not squid;
  - `MICRONAUT_HTTP_CLIENT_SSL_INSECURE_TRUST_ALL_CERTIFICATES: "true"`: there is no way to
    mount the cluster CA. This matches the `ssl_verify: false` the authority APISIX already
    uses on the same hop through squid. Tokens are still verified against the fetched JWKS;
    only the in-cluster TLS peer check is skipped. Production verifiers have a publicly
    trusted cert and need none of these variables. Step 6 documents that.
  Verification gate, part of this step's acceptance: the consent-manager pod gets Ready
  (its readiness probe includes the identity providers, so it stays unready while
  discovery fails), and a participant-scope token from the authority verifier gets a
  non-401 answer from `GET /v1/participants/me`, which proves the JWKS fetch. If either
  fails because the settings do not reach those clients, do not work around it in this
  repo: report the step as blocked and name the missing upstream option (a CA/truststore
  volume and an IdP proxy setting in the dsc-consent-manager chart). The fallback is a
  chart release with those options and a dependency bump.
- Self-descriptions: the app fetches `selfDescriptionUri` only for
  `GET /v1/privacy-notices/{id}?resolve=true`, through a separate client with no proxy. So
  `SELF_DESCRIPTION_ALLOWED_HOSTS` stays unset, the IT does not use `resolve=true`, and
  Step 6 notes that `resolve=true` does not work in the local k3s scenario.
- `consentManagement.networkPolicy.consentManagerClients` is the provider APISIX
  (`{namespace: provider, podLabels: {app.kubernetes.io/name: apisix}}`, set in Step 1),
  else the plugin's in-cluster calls to
  `consent-manager.trust-anchor.svc.cluster.local:8080` are dropped on a
  NetworkPolicy-enforcing CNI.
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
  - add `check_mode: list`; leave `consumer_claim` at its default
    (`verifiableCredential.issuer`) unless the consumer token names the consumer DID
    elsewhere -- verify against the provider verifier's token;
  - leave `require_purpose` / `require_contract` off unless the owner-resolver emits a
    purpose and a `contractUri`/`privacyNoticeId` per claim (check what it returns for
    `mp-data-service`); note the choice in the route comment.
  Both phases (`ext-plugin-pre-req`, `ext-plugin-post-resp`) carry the same JSON. Rewrite
  the route comment (~lines 506-520): the plugin resolves the provider via
  `/participants/me`, the consumer via the participant registry, then checks each owner's
  consents.
- The plugin image tag: the single definition is the `install-consent-plugin` init
  container in `k3s/consent-provider.yaml` (~line 45, currently `0.0.12`). Set it to
  `quay.io/seamware/consent-plugin:1.0.0`. Grep `k3s`, `pom.xml`, `it` and `.github` for
  other `consent-plugin:` references and bump them too.
- Update the `.github/workflows/test.yaml` comment (no "managed mongo").
- Acceptance: every `helm template` call the `consent` profile makes renders, which you
  can check by running the commands from `deploy.sh` with `--dry-run=client` or
  `helm template`, and the verifier-reachability gate above passes. Since plugin 1.0.0 is published, the CI `consent` matrix leg
  (`test,consent,consent-test`) is expected to deploy; it may still fail on the IT until
  Step 5 lands, which is accepted for this step.
- Outcome: the gate passed on a local `mvn deploy -Pconsent` run. The consent-manager got
  Ready ("Identity provider 'primary' resolved on attempt 1", through squid with
  trust-all). A token from the provider consent-facade (`POST /internal/tokens`, audience
  `consent-manager`) carries `participant_id: did:web:mp-operations.org` (a plain DID, so
  `vc.issuer` is a string and no `issuer.id` path is needed) and
  `roles: [VerifiableCredential, MembershipCredential]`. With it,
  `GET /v1/participants/me` answers 403 "not registered" (401 without a token), so the
  JWKS fetch and the claim mapping work. The 1.0.0 route keeps `require_purpose` /
  `require_contract` off: owner-resolver 0.2.0 returns only `ownerId` and `dataResource`
  per claim, so either option would deny every request. `consumer_claim` stays at its
  default, because the provider verifier's `data-service` scope uses `fullInclusion`.
  `deploy.sh` needed no change.
  Findings for Step 5:
  - the consent-manager looks contract parties up by participant identifier (the token's
    DID, `findByIdentifier`), while the consent-facade passes the agreement's
    `provider-id`/`consumer-id` through verbatim. Contract-management writes self-description
    URLs there (`CONSENT_SELF_DESCRIPTION_BASE_URL`). An agreement the IT seeds must
    therefore name the participant DIDs in `provider-id`/`consumer-id`, or notice
    generation by contract fails with "No active participant is registered under '<url>'".
    The purchase-flow path (contract-management) has the same mismatch: a marketplace
    purchase no longer yields a privacy notice. **Open decision for the ticket owner:** the
    fix is upstream, not in this repo (contract-management or the consent-facade emitting or
    mapping to the participant DID, or the consent-manager resolving parties by
    `selfDescriptionUri`), followed by an image/chart bump here. Until it is decided, Step 6
    documents the gap in `doc/CONSENT_MANAGEMENT.md`;
  - k3s enforces NetworkPolicies (kube-router), and a freshly started pod is rejected
    for its first seconds until its IP is synced. Retry the first in-cluster call;
  - running k3s nested in a k3s pod needs `-Dk3s.clusterCidr=10.152.0.0/16
    -Dk3s.serviceCidr=10.153.0.0/16 -Dk3s.clusterDns=10.153.0.10`, else the outer DNS
    (10.43.0.10) falls into k3s's own service range and no image pulls.

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
  - register **both** the provider and the consumer with `POST /v1/participants`, each
    with a `selfDescriptionUri`: plugin 1.0.0 denies when the provider is not found at
    `/participants/me` or the consumer DID is unknown / has no self-description;
  - the granted consent must name the provider DID as `provider` and the consumer DID
    (the value the plugin reads from `consumer_claim`) as `consumer`, plus the contract the
    owner-resolver reports, otherwise the plugin does not match it;
  - privacy notices come from `GET /v1/privacy-notices` (filter by the contract URI; there
    is no GET by contract);
  - grant with `POST /v1/consents`, withdraw via the consent's revoke operation;
  - assert with `GET /v1/consents?userIdentifier=` and `GET /v1/consents/check`.
  Remove the `identifier/search` and `consents/participants` calls. Do not use
  `?resolve=true` (see Step 4, self-descriptions).
- Keep the three scenarios in `consent_management.feature` (deny without consent, allow
  after grant, deny after revoke). Rename steps only where the wording names the old API.
- Acceptance: `mvn -f it/pom.xml test-compile` passes. The `@consent` scenario passes
  with `mvn clean integration-test -Ptest,consent,consent-test` (plugin 1.0.0 from Step 4).

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
  - the plugin 1.0.0 config (`check_mode`, `consent_api_prefix`, `consumer_claim`,
    `require_purpose`/`require_contract`, the removed `consent_api_host`/`consent_key`),
    the matching rules and the provider/consumer registration it relies on;
  - deployment values for both charts and the k3s `consent` profile, including the setup
    Job (what it owns, the never-rotate rule and its error handling, the signing-key
    options), SMTP credentials via `consentManagement.smtp.existingSecret`, and
    `networkPolicy.consentManagerClients` for providers in other namespaces;
  - GitOps: unset `consentManagement.setup.ttlSecondsAfterFinished` under Argo CD self-heal,
    else the Job is recreated after every TTL cleanup. `helm uninstall` leaves
    `consent-manager-env` and `consent-manager-signing-keys` in place: they are not release
    objects, which is what keeps the keys from rotating on a reinstall. Delete them by hand
    to start from scratch;
  - the k3s-only verifier reachability settings (`config.extraEnv` proxy, contract-service
    `DIRECT`, trust-all TLS) from Step 4, why production does not need them, and that
    `GET /v1/privacy-notices/{id}?resolve=true` does not work in the local scenario;
  - migration from the old consent-manager. There is no data migration from Mongo:
    consents must be re-granted. Upgrading to trust-anchor 2.0.0 on an existing Postgres
    volume relies on the setup Job creating the `consent_manager` database; the plugin
    must be upgraded to 1.0.0 together with the route config. The consent database and
    user are fixed to `consent_manager` on the chart's managed Postgres; external
    Postgres is not supported in 2.0.0 (the chart fails on that combination).
- New ADR 0008 in the existing ADR format: context, decision, consequences. ADRs
  0001-0005 get `Status: Superseded by 0008`. Update the index in `doc/adr/README.md`.
- Release notes: in `doc/release-notes/10-x.md`, a 10.16.0 entry. State plainly that the
  DSC chart itself only changes docs and comments (no value removed, hence the minor
  bump), and that the breaking changes for consent users are in the trust-anchor chart
  2.0.0 (Mongo and the consent key removed, Postgres, new subchart values) and in the
  provider's consent route / plugin 1.0.0 config.
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
- Also grep for `consent_api_host` and `consent-plugin:0.` -- no hit may remain.
- Run `mvn clean deploy -Pconsent` followed by
  `mvn clean integration-test -Ptest,consent,consent-test` (plugin 1.0.0), and fix the
  failures. Redeploy once on the same cluster to confirm the setup Job is idempotent and
  the generated keys and signing key do not change, and that `consent-manager-env` carries
  no `kubectl.kubernetes.io/last-applied-configuration` annotation.
