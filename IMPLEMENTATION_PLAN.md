# Implementation Plan: Integrate Status-List-Server with FDSC-Keycloak

## Overview
Integrate the [Token Status List](https://datatracker.ietf.org/doc/draft-ietf-oauth-status-list/) specification into the FIWARE Data Space Connector to make Verifiable Credentials revocable. This requires deploying the [status-list-server](https://github.com/adorsys/status-list-server) as a new infrastructure component and loading the [token-status-link](https://github.com/ADORSYS-GIS/token-status-link) Keycloak plugin JAR into Keycloak, then configuring the realm to use the `oid4vc-status-list-claim-mapper` protocol mapper for credential types that opt in to status list support.

## Steps

### Step 1: Add status-list-server Helm templates and values

**Goal:** Deploy the status-list-server as a new component within the data-space-connector umbrella chart, with its own PostgreSQL and Redis dependencies.

**Files to create/modify:**
- `charts/data-space-connector/values.yaml` — Add a new top-level `statusListServer` section with default values:
  - `statusListServer.enabled: false` (opt-in)
  - `statusListServer.image.repository: ghcr.io/adorsys/status-list-server`
  - `statusListServer.image.tag: latest`
  - `statusListServer.service.port: 8081` (external), `containerPort: 8000` (internal)
  - `statusListServer.resources` (requests: 256Mi/250m, limits: 512Mi/500m)
  - `statusListServer.env` for `RUST_LOG`, `APP_ENV`, `APP_SERVER__HOST`, `APP_SERVER__PORT`, `APP_CACHE__TTL`, `APP_CACHE__MAX_CAPACITY`
  - `statusListServer.postgresql` — Configuration for the status-list-server's PostgreSQL database (host, port, database name, credentials secret). By default, reuse the existing Zalando Postgres Operator managed instance by adding a new database (`statuslistdb`) and user (`statuslist`) to the `decentralizedIam.vcAuthentication.managedPostgres.config` section.
  - `statusListServer.redis` — Configuration for Redis (host, port). Include a simple Redis deployment option or allow pointing to an external Redis.
  - `statusListServer.ingress` — Optional ingress configuration for external access to the status list endpoint.
- `charts/data-space-connector/templates/status-list-server-deployment.yaml` — New Deployment template for the status-list-server container, conditionally rendered with `{{- if .Values.statusListServer.enabled }}`. Wire environment variables for PostgreSQL and Redis connectivity using the values. Include readiness/liveness probes on the health endpoint.
- `charts/data-space-connector/templates/status-list-server-service.yaml` — New Service template exposing port 8081 -> 8000.
- `charts/data-space-connector/templates/status-list-server-ingress.yaml` — New optional Ingress template (conditional on `statusListServer.ingress.enabled`).
- `charts/data-space-connector/templates/status-list-server-redis.yaml` — A simple Redis Deployment + Service template (conditional on `statusListServer.redis.enabled`, for cases where no external Redis is available). Use the standard `redis:7` image with minimal config.

**Acceptance criteria:**
- `helm template` with `statusListServer.enabled: true` produces valid Deployment, Service, and optional Ingress manifests for the status-list-server.
- `helm template` with `statusListServer.enabled: false` (default) produces no status-list-server resources.
- The status-list-server container receives correct database and Redis connection environment variables.
- The Redis deployment is optional and can be disabled when pointing to an external Redis instance.

### Step 2: Add token-status-link plugin JAR loading to Keycloak

**Goal:** Configure Keycloak to load the `keycloak-token-status-plugin` JAR from the [token-status-link](https://github.com/ADORSYS-GIS/token-status-link) project into its providers directory at startup, using an init container.

**Files to modify:**
- `charts/data-space-connector/values.yaml` — Add a new `keycloak.tokenStatusList` configuration section:
  - `keycloak.tokenStatusList.enabled: false` (opt-in, should be enabled alongside `statusListServer.enabled`)
  - `keycloak.tokenStatusList.plugin.groupId: io.github.adorsys-gis`
  - `keycloak.tokenStatusList.plugin.artifactId: keycloak-token-status-plugin`
  - `keycloak.tokenStatusList.plugin.version: <latest-release-version>` (to be determined from Maven Central)
  - `keycloak.tokenStatusList.plugin.mavenBaseUrl: https://repo1.maven.org/maven2` (configurable Maven repo URL)
  - Document these values with Helm-style comments.
- `charts/data-space-connector/values.yaml` — Update the default `keycloak.extraVolumeMounts` to conditionally include a providers volume mount at `/opt/bitnami/keycloak/providers`.
- `charts/data-space-connector/values.yaml` — Update the default `keycloak.extraVolumes` to conditionally include an `emptyDir` volume for providers.
- `charts/data-space-connector/templates/realm.yaml` — No changes needed in this step.

**Implementation approach:** Since the Bitnami Keycloak chart supports `initContainers`, `extraVolumes`, and `extraVolumeMounts` as values, and because the k3s examples already demonstrate overriding these (e.g., `k3s/consumer.yaml` line 14-36), the plugin loading mechanism will be documented for use in the k3s values files. A new helper template will be created to generate the initContainer spec and volume mounts.

**New file:**
- `charts/data-space-connector/templates/_keycloak-status-list.tpl` — A Helm helper template that generates:
  1. An initContainer spec that downloads the token-status-link JAR from Maven Central using `curlimages/curl` (matching the existing pattern in `k3s/consumer.yaml`) and places it in a shared `emptyDir` volume.
  2. The extraVolume and extraVolumeMount entries for the providers directory.
  
  These helpers can be included in k3s values files or used via the values.yaml defaults when `keycloak.tokenStatusList.enabled` is true.

**Acceptance criteria:**
- When `keycloak.tokenStatusList.enabled: true`, the Keycloak pod starts with an init container that downloads the correct JAR version from Maven Central to `/opt/bitnami/keycloak/providers/`.
- The JAR download URL is configurable (Maven coordinates and base URL).
- When `keycloak.tokenStatusList.enabled: false` (default), no init container is added and Keycloak functions identically to before.
- The existing `extraVolumeMounts` (realm import) is preserved and not broken by the new volume mount.

### Step 3: Configure realm template for status-list integration

**Goal:** Update the Keycloak realm template to support status-list realm attributes and add the `oid4vc-status-list-claim-mapper` protocol mapper to credential client scopes that opt in to status list support.

**Files to modify:**
- `charts/data-space-connector/values.yaml` — Add status-list-specific realm configuration under `keycloak.tokenStatusList`:
  - `keycloak.tokenStatusList.serverUrl: ""` — URL of the status-list-server (e.g., `http://status-list-server:8081`). This is injected as the `status-list-server-url` realm attribute.
  - `keycloak.tokenStatusList.issuerPrefix: ""` — Optional issuer prefix for the token status list (defaults to auto-generated UUID in the plugin if empty).
  - `keycloak.tokenStatusList.mandatory: false` — Whether status list is mandatory for credential issuance.
  - `keycloak.tokenStatusList.maxEntries: 10000` — Maximum entries per status list.
  - `keycloak.tokenStatusList.issuanceTimeout: 10000` — Timeout in ms for status list issuance calls.
  - `keycloak.tokenStatusList.registrationTimeout: 30000` — Timeout in ms for issuer registration.
  - `keycloak.tokenStatusList.registrationRetries: 1` — Number of registration retries.
  - `keycloak.tokenStatusList.registrationCooldown: 60000` — Cooldown in ms between retries.
  - `keycloak.tokenStatusList.circuitBreakerFailureThreshold: 5` — Circuit breaker threshold.
  - `keycloak.tokenStatusList.credentialTypes: []` — List of credential type names (e.g., `["LegalPersonCredential", "OperatorCredential"]`) that should include the status-list-claim-mapper. This controls which credential types are revocable.

- `charts/data-space-connector/templates/realm.yaml` — Add conditional status-list realm attributes inside the `"attributes"` block (after the existing `keycloak.realm.attributes` injection point at line 27-30):
  ```
  {{- if .Values.keycloak.tokenStatusList.enabled }}
  "status-list-enabled": "true",
  "status-list-server-url": "{{ .Values.keycloak.tokenStatusList.serverUrl }}",
  "status-list-mandatory": "{{ .Values.keycloak.tokenStatusList.mandatory }}",
  "status-list-max-entries": "{{ .Values.keycloak.tokenStatusList.maxEntries }}",
  ...
  {{- end }}
  ```

- `charts/data-space-connector/templates/realm.yaml` — Add a conditional `oid4vc-status-list-claim-mapper` protocol mapper entry to the `protocolMappers` array within the OID4VC client definition (the `clients` value). Since clients are injected as raw JSON from `values.yaml`, the mapper will be documented for inclusion in the client definition.

**Alternative approach for protocol mapper injection:** Because the client definition is a raw JSON string in `values.yaml` (line 390-465), and client scopes are also injectable (`keycloak.realm.clientScopes`), the status-list-claim-mapper should be added to the relevant credential type client scopes (e.g., `LegalPersonCredential`, `OperatorCredential`). This aligns with how existing protocol mappers (context-mapper, role-mapper, etc.) are already defined per client scope.

**New helper template:**
- Extend `charts/data-space-connector/templates/_keycloak-status-list.tpl` to include a named template that generates the `oid4vc-status-list-claim-mapper` protocol mapper JSON block for a given credential type. This can be used within client scope definitions.

**Acceptance criteria:**
- When `keycloak.tokenStatusList.enabled: true`, the realm JSON includes all status-list realm attributes with configurable values.
- The `oid4vc-status-list-claim-mapper` protocol mapper is documented and demonstrated for inclusion in credential type client scopes.
- Each configuration parameter (timeout, retries, etc.) maps correctly to the plugin's expected realm attribute names.
- When `keycloak.tokenStatusList.enabled: false`, no status-list attributes appear in the realm JSON.
- The existing realm structure is not broken (all existing attributes, clients, scopes preserved).

### Step 4: Update k3s example deployments with status-list configuration

**Goal:** Update the k3s example values files to demonstrate a complete working configuration of the status-list integration, serving as both documentation and a functional reference deployment.

**Files to modify:**
- `k3s/consumer.yaml` — Add status-list-server and token-status-list configuration:
  - Enable `statusListServer.enabled: true` with appropriate resource limits for k3s.
  - Configure Redis for the status-list-server.
  - Add the PostgreSQL database and user for the status-list-server to the managed Postgres config.
  - Enable `keycloak.tokenStatusList.enabled: true`.
  - Set `keycloak.tokenStatusList.serverUrl` to the in-cluster status-list-server service URL.
  - Update the `keycloak.initContainers` to include the JAR download init container (alongside the existing TIR registration init container).
  - Update `keycloak.extraVolumeMounts` and `keycloak.extraVolumes` to include the providers directory volume.
  - Add `oid4vc-status-list-claim-mapper` to relevant credential type client scopes (e.g., `LegalPersonCredential`, `OperatorCredential`) within `keycloak.realm.clientScopes`.
  - Add status-list realm attributes to `keycloak.realm.attributes`.
  - Configure `keycloak.tokenStatusList.credentialTypes` to list credential types that should be revocable.

- `k3s/provider.yaml` — Add similar status-list configuration for the provider participant (if Keycloak issuance is configured there).

- `k3s/consumer-elsi.yaml` — Update if this deployment variant uses credential issuance.

**Acceptance criteria:**
- `helm template test charts/data-space-connector -f k3s/consumer.yaml` produces a complete deployment with status-list-server, Redis, Keycloak with plugin, and correctly configured realm.
- The consumer k3s example demonstrates all credential types with status-list support.
- The init container correctly downloads the plugin JAR alongside the existing TIR registration init container.
- The status-list-server is accessible at the configured in-cluster URL from Keycloak.
- All existing k3s functionality is preserved (no regressions).

### Step 5: Add Helm chart documentation and values.yaml comments

**Goal:** Ensure all new configuration is well-documented with Helm-style comments in `values.yaml`, and provide usage documentation for the status-list integration.

**Files to modify:**
- `charts/data-space-connector/values.yaml` — Review and ensure all new values have descriptive `# --` Helm-doc comments explaining:
  - What each value controls
  - Default values and their rationale
  - Cross-references to the token-status-link and status-list-server documentation
  - Which values must be set together (e.g., `statusListServer.enabled` and `keycloak.tokenStatusList.enabled` should both be true for a working setup)

- `charts/data-space-connector/Chart.yaml` — Bump the chart version (minor version bump since this adds new functionality without breaking existing deployments).

**Acceptance criteria:**
- Every new value in `values.yaml` has a Helm-doc comment (`# --`).
- The chart version is bumped appropriately.
- `helm lint charts/data-space-connector` passes without errors.
- `helm template` with default values (status-list disabled) produces identical output to the current chart.
- `helm template` with status-list enabled produces valid Kubernetes manifests.
