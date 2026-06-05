# FIWARE Data Space Connector

## Overview
Umbrella Helm chart that packages all components a participant needs to join a FIWARE-based data space. It combines Keycloak-based credential issuance (OID4VC), decentralized IAM (VC verification, trusted issuers lists), NGSI-LD context brokering (Scorpio), contract management, and marketplace components into a single deployable unit.

## Tech Stack
- Language: YAML (Helm templates), Shell scripts for helpers
- Build: Helm 3 (umbrella chart with subchart dependencies)
- Framework: Kubernetes, Bitnami Keycloak 25.2.0, Zalando Postgres Operator
- Test: k3s-based integration tests (see `it/` directory), Maven for test orchestration (`pom.xml`)

## Project Structure
```
charts/
  data-space-connector/       # Main umbrella chart (v8.5.2)
    Chart.yaml                 # Dependencies: keycloak, decentralized-iam, scorpio, tm-forum-api, etc.
    values.yaml                # Default values for all subcharts
    templates/
      _helpers.tpl             # Helm template helpers (labels, names)
      realm.yaml               # Keycloak realm ConfigMap (credentials, protocol mappers, keys)
      issuance-secrets.yaml    # Secret generation for issuance passwords
      identityhub-*.yaml       # Identity Hub deployment/service/ingress
      dsconfig-*.yaml          # Data space config components
      rainbow-*.yaml           # Rainbow service
      participant-registration*.yaml
      credentials-script.yaml
  trust-anchor/                # Minimal trust anchor chart (v0.3.0)
k3s/                           # k3s deployment examples (values override files)
  consumer.yaml                # Consumer participant values
  provider.yaml                # Provider participant values
  consumer-elsi.yaml           # Consumer with ELSI support
  consumer-gaia-x.yaml         # Consumer with Gaia-X support
  dsp-consumer.yaml            # DSP consumer config
  dsp-provider.yaml            # DSP provider config
  infra/                       # Infrastructure (traefik, coredns, squid, gx-registry)
  namespaces/                  # Kubernetes namespace definitions
  certs/                       # Certificate configurations
doc/                           # Documentation and AWS deployment examples
it/                            # Integration test definitions
schema/                        # JSON-LD schemas
helpers/                       # Helper scripts (cert generation)
```

## Build & Test
```bash
# Update Helm dependencies
helm dependency update charts/data-space-connector

# Template locally (lint check)
helm template test charts/data-space-connector -f k3s/consumer.yaml

# Lint the chart
helm lint charts/data-space-connector

# Run integration tests (requires k3s cluster)
mvn verify -f pom.xml
```

## Key Conventions
- Subcharts are declared as dependencies in `Chart.yaml` with condition flags (e.g., `keycloak.enabled`)
- Keycloak realm configuration is a Helm template (`realm.yaml`) that generates a ConfigMap with JSON
- Realm attributes, client roles, users, clients, and client scopes are injected from `values.yaml` as raw JSON strings using Helm's `nindent` and `trim`
- The `realm.yaml` template has extension points: `keycloak.realm.attributes` (line 27-30), `keycloak.realm.clientScopes` (line 600-603) for additional JSON
- k3s YAML files are plain Helm values overrides (used with `helm install -f`)
- Keycloak uses Bitnami's legacy image with `containerSecurityContext.enabled: false`
- PostgreSQL is managed via Zalando Postgres Operator (CRD-based), not in-chart Bitnami postgres
- Database credentials stored in auto-generated secret: `postgres.postgres.credentials.postgresql.acid.zalan.do`
- Bitnami Keycloak chart supports `initContainers`, `extraVolumes`, `extraVolumeMounts`, `extraEnvVars` for extension

## Important Files
- `charts/data-space-connector/Chart.yaml` — Subchart dependency declarations
- `charts/data-space-connector/values.yaml` — Default configuration for all components
- `charts/data-space-connector/templates/realm.yaml` — Keycloak realm template (credentials, mappers, keys)
- `charts/data-space-connector/templates/_helpers.tpl` — Shared Helm template helpers
- `k3s/consumer.yaml` — Reference consumer deployment with full credential config
- `k3s/provider.yaml` — Reference provider deployment
