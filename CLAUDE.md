# FIWARE Data Space Connector

## Overview
Umbrella Helm chart repository that bundles all components of the FIWARE Data
Space Connector (DSC) -- identity, DSP (EDC/Rainbow), contract management,
marketplace -- into a single deployable artifact. No application code lives
here; this repo is packaging and integration glue.

## Tech Stack
- Packaging: Helm 3 (umbrella chart with subchart dependencies).
- Components deployed: EDC-based IdentityHub (Java 21), fdsc-edc (Java 21),
  Rainbow DSP (Rust), Keycloak 26.6.x (CloudPirates chart; SEAMWARE-patched
  image `quay.io/seamware/keycloak:26.6.3`), Scorpio Broker (Quarkus/Java),
  TMForum API (Java), Contract Management, Business API Ecosystem, MongoDB,
  HashiCorp Vault, cert-manager, OpenTelemetry Collector.
- Build / Release: GitHub Actions (`.github/workflows/`), helm package +
  publish to `gh-pages`. Top-level `pom.xml` drives local k3s-based
  integration tests (`it/`).
- Test: helm lint, helm template, helm-unittest (`charts/data-space-connector/tests/`),
  k3s integration tests under `it/`.

## Project Structure
```
charts/
  data-space-connector/       # umbrella chart (THE thing)
    Chart.yaml                # dependencies: decentralizedIam, keycloak,
                              # scorpio, tm-forum-api, contract-management,
                              # marketplace, fdsc-edc, vault, did, cert-manager,
                              # opentelemetry-collector
    values.yaml               # ~2930 lines -- every knob lives here
    templates/                # in-chart manifests (see below)
    tests/                    # helm-unittest suites
      tracing_test.yaml       # OTEL tracing tests (33 tests, 361 asserts)
  trust-anchor/               # secondary chart for trust-anchor scenarios

charts/data-space-connector/templates/
  identityhub-deployment.yaml # EDC IdentityHub (Java, tractusx/identityhub)
  rainbow-deployment.yaml     # Rainbow DSP (Rust, quay.io/wi_stefan/rainbow)
  dsconfig-deployment.yaml    # static /.well-known/data-space-configuration
  dsconfig-cm.yaml / -service.yaml
  identityhub-cm.yaml / -datasource-cm.yaml / -logging.yaml / -ingress.yaml
  identityhub-service.yaml
  participant-registration-job.yaml, tmf-registration-job.yaml
  dataplane-registration.yaml, rainbow-registration.yaml,
  participant-registration.yaml
  mongodb.yaml                # internal MongoDB (when not using operator)
  cert-manager-issuer.yaml, elsi-secret.yaml, issuance-secrets.yaml
  bae-did.yaml, bae-prep-cm.yaml, ca-script.yaml, credentials-script.yaml
  extra-manifests.yaml, realm.yaml, tmf-registration-cm.yaml
  _helpers.tpl, _realm.tpl

doc/
  README.md                   # docs index
  deployment-integration/     # role-based and scenario-based guides
    observability/README.md   # OTEL tracing guide (architecture, per-component)
  flows/                      # mermaid sequence diagrams
  release-notes/              # per-version changelogs
    9-x.md                    # 9.0.0 breaking changes + 9.1.0 OTEL tracing

it/                           # k3s-based integration tests (Maven-driven)
k3s/                          # k3s bootstrap manifests for local dev
  provider.yaml               # 1300 lines, no observability section yet
helpers/                      # reference assets (e.g. Gaia-X trust anchors)
schema/                       # JSON schemas used by components
.github/workflows/            # check.yaml, test.yaml, pre-release.yml,
                              # release.yml, stale-issues.yaml
pom.xml                       # drives local-deploy + integration tests
```

## Build & Test
- Dependency fetch: `helm dependency update charts/data-space-connector`
- Lint: `helm lint charts/data-space-connector`
- Render: `helm template test charts/data-space-connector -f <values>.yaml`
- Unit tests: `helm unittest charts/data-space-connector`
  (requires helm-unittest plugin, pinned to v0.6.3 in CI)
- Integration (local k3s, slow): `mvn -f pom.xml verify`
- CI entry points: `.github/workflows/check.yaml` (lint) and
  `.github/workflows/test.yaml` (helm-unittest + integration).

## Key Conventions
- Every `values.yaml` key carries a `# --` comment (helm-docs format) so
  the values table in `README.md` regenerates cleanly. Keep new values
  documented the same way.
- Feature toggles follow `<component>.enabled` and `{{- if ... }}` gates
  wrap the entire template file.
- Container env blocks always end with three extension points in this
  order: `env:` (map), `envValueFrom:` (map), `envFrom:` (secrets/
  configmaps lists). New env vars should respect that pattern and not
  clobber user-provided maps -- append, never overwrite.
- Subchart-specific knobs live under the subchart's alias
  (`fdsc-edc:`, `scorpio:`, `keycloak:`, `contract-management:`,
  `marketplace:`, `decentralizedIam:`, `did:`, `vault:`,
  `cert-manager:`, `opentelemetry-collector:`). The umbrella chart only
  overrides values; it does not template the subcharts' manifests.
- Pod security context is strict (runAsNonRoot, readOnlyRootFilesystem,
  drop ALL capabilities). Any new container must match that.
- Chart version (`charts/data-space-connector/Chart.yaml` `version:`)
  follows semver; bump minor for additive features.

## OpenTelemetry Tracing (from ticket-28)
The chart already has a full OTEL tracing integration (chart version 9.1.0):
- Global `tracing:` block in `values.yaml` (line ~2731) controls
  `OTEL_*` env vars on all workloads.
- `opentelemetry-collector` subchart (v0.152.0) deploys an in-cluster
  Collector; its config lives in `values.yaml` under
  `opentelemetry-collector.config:` (line ~2806).
- `_helpers.tpl` has six helpers: `dsc.otel.defaultEndpoint`,
  `dsc.otel.endpoint`, `dsc.otel.resourceAttributes`, `dsc.otel.env`,
  plus per-subchart variants (`dsc.otel.scorpio.envList`,
  `dsc.otel.fdscEdc.envList`, `dsc.otel.keycloak.cmName`, etc.).
- The Collector currently uses the **subchart's built-in ConfigMap**
  (`opentelemetry-collector.configMap.create: true` is the default) with
  a `debug` exporter only. Commented-out examples for Tempo, Jaeger, and
  Honeycomb exporters are in `values.yaml` (line ~2850).
- The subchart supports `configMap.create: false` +
  `configMap.existingName: "<name>"` to use a custom ConfigMap; the
  `existingName` field is tpl-rendered so template expressions work.
  The ConfigMap **must** use the key `relay` for the collector config YAML.
- Tracing tests live in `charts/data-space-connector/tests/tracing_test.yaml`.

## Keycloak / OID4VCI wallet issuance (10.x)
10.x migrated Keycloak to the CloudPirates chart + KC 26.6.x with the post-26.4
OID4VCI model. Hard-won gotchas (full detail in `doc/release-notes/10-x.md`):
- **Image:** use `quay.io/seamware/keycloak:26.6.3` — it carries the Liquibase
  changeset `26.7.0-verifiable-credential` (fixes `column ... version does not
  exist` when reusing a pre-26.4 DB) plus the OID4VCI QR-endpoint fix.
- **Credential encryption:** the issuer advertises request + response encryption.
  Wallets that encrypt must set the JWE `kid` and must not send a top-level `alg`
  in `credential_response_encryption`; the EUDI wallet needs
  `eudi-lib-ios-openid4vci-swift` ≥ 0.40.0. Lissi does not encrypt (unaffected).
  Response encryption is not disableable via config (RSA token key always offers RSA-OAEP).
- **DCQL:** `jwt_vc_json` uses `meta.type_values` (array of string arrays);
  sd-jwt uses `meta.vct_values` (flat array); `credential_sets` must be non-null.
- **Realm import:** KC does not re-import an existing realm — recreate it to apply
  `keycloak.realm.*` changes. Bitnami→CloudPirates upgrades need the keycloak
  StatefulSet deleted (immutable-field change).
- **Wallet preset + per-user VCs:** `keycloak.realm.wallets` (Lissi/EUDI clients);
  `wallets.issueCredentialsToUsers: true` auto-assigns realm VCs to declared users.

## Important Files
- `charts/data-space-connector/Chart.yaml` -- dependency graph (current
  version 9.1.0).
- `charts/data-space-connector/values.yaml` -- ~2930-line config surface;
  top-level sections start at these approximate line numbers:
  `decentralizedIam` (9), `scorpio` (183), `keycloak` (262),
  `tm-forum-api` (1168), `rainbow` (1299), `contract-management` (1351),
  `dataSpaceConfig` (1417), `marketplace` (1450), `fdsc-edc` (1556),
  `identityhub` (1913), `tracing` (2731),
  `opentelemetry-collector` (2776).
- `charts/data-space-connector/templates/_helpers.tpl` -- shared Helm
  partials (`dsc.labels`, `dsc.otel.*`, etc.).
- `charts/data-space-connector/templates/identityhub-deployment.yaml` --
  reference for how env/volumes/initContainers/OTEL agent are wired.
- `charts/data-space-connector/templates/rainbow-deployment.yaml` --
  simpler deployment with init container pattern.
- `pom.xml` + `it/` -- integration test harness.
- `doc/release-notes/9-x.md` -- follow existing format when adding entries.
- `doc/deployment-integration/observability/README.md` -- OTEL tracing docs.
