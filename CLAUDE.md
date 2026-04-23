# FIWARE Data Space Connector

## Overview
Umbrella Helm chart repository that bundles all components of the FIWARE Data
Space Connector (DSC) – identity, DSP (EDC/Rainbow), contract management,
marketplace – into a single deployable artifact. No application code lives
here; this repo is packaging and integration glue.

## Tech Stack
- Packaging: Helm 3 (umbrella chart with subchart dependencies).
- Components deployed: EDC-based IdentityHub (Java 21), fdsc-edc (Java 21),
  Rainbow DSP (Rust), Keycloak 25.x, Scorpio Broker (Quarkus/Java),
  TMForum API (Java), Contract Management, Business API Ecosystem, MongoDB,
  HashiCorp Vault, cert-manager.
- Build / Release: GitHub Actions (`.github/workflows/`), helm package +
  publish to `gh-pages`. Top-level `pom.xml` drives local k3s-based
  integration tests (`it/`).
- Test: helm lint, helm template, k3s integration tests under `it/`.

## Project Structure
```
charts/
  data-space-connector/       # umbrella chart (THE thing)
    Chart.yaml                # dependencies: decentralizedIam, keycloak,
                              # scorpio, tm-forum-api, contract-management,
                              # marketplace, fdsc-edc, vault, did, cert-manager
    values.yaml               # ~2200 lines – every knob lives here
    templates/                # in-chart manifests (see below)
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
  flows/                      # mermaid sequence diagrams
  release-notes/              # per-version changelogs
  CONTRACT_NEGOTIATION.md, DSP_INTEGRATION.md, RAINBOW_INTEGRATION.md,
  CENTRAL_MARKETPLACE.md, MARKETPLACE_INTEGRATION.md

it/                           # k3s-based integration tests (Maven-driven)
k3s/                          # k3s bootstrap manifests for local dev
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
- Integration (local k3s, slow): `mvn -f pom.xml verify`
- CI entry points: `.github/workflows/check.yaml` (lint) and
  `.github/workflows/test.yaml` (integration).

## Key Conventions
- Every `values.yaml` key carries a `# --` comment (helm-docs format) so
  the values table in `README.md` regenerates cleanly. Keep new values
  documented the same way.
- Feature toggles follow `<component>.enabled` and `{{- if ... }}` gates
  wrap the entire template file.
- Container env blocks always end with three extension points in this
  order: `env:` (map), `envValueFrom:` (map), `envFrom:` (secrets/
  configmaps lists). New env vars should respect that pattern and not
  clobber user-provided maps – append, never overwrite.
- Subchart-specific knobs live under the subchart's alias
  (`fdsc-edc:`, `scorpio:`, `keycloak:`, `contract-management:`,
  `marketplace:`, `decentralizedIam:`, `did:`, `vault:`,
  `cert-manager:`). The umbrella chart only overrides values; it does
  not template the subcharts' manifests.
- License header (Apache-2.0, Cofinity-X / LKS Next / Eclipse
  Foundation) is present on every template file – preserve it.
- Pod security context is strict (runAsNonRoot, readOnlyRootFilesystem,
  drop ALL capabilities). Any new container must match that.
- Chart version (`charts/data-space-connector/Chart.yaml` `version:`)
  follows semver; bump minor for additive features.

## Important Files
- `charts/data-space-connector/Chart.yaml` – dependency graph.
- `charts/data-space-connector/values.yaml` – 2200-line config surface;
  top-level sections start at these approximate line numbers:
  `decentralizedIam` (9), `scorpio` (183), `keycloak` (262),
  `tm-forum-api` (1168), `rainbow` (1299), `contract-management` (1351),
  `dataSpaceConfig` (1417), `marketplace` (1450), `fdsc-edc` (1556),
  `identityhub` (1913).
- `charts/data-space-connector/templates/_helpers.tpl` – shared Helm
  partials (`dsc.labels`, etc.).
- `charts/data-space-connector/templates/identityhub-deployment.yaml` –
  reference for how env/volumes/initContainers are wired.
- `charts/data-space-connector/templates/rainbow-deployment.yaml` –
  simpler deployment with init container pattern.
- `pom.xml` + `it/` – integration test harness.
- `doc/release-notes/` – follow existing format when adding entries.
