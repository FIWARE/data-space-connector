# Implementation Plan: OpenTelemetry Tracing in FIWARE Data Space Connector

## Overview
Introduce end-to-end distributed tracing across the FIWARE Data Space Connector
(DSC) by deploying an OpenTelemetry (OTEL) Collector as an optional subchart of
the `data-space-connector` umbrella chart and wiring the first-party workloads
(IdentityHub, fdsc-edc) plus the third-party subcharts that ship with native
OTEL support to export traces through the OTLP endpoint. Trace configuration
is exposed via `values.yaml` so operators can plug any OTLP-compatible backend
(Jaeger, Tempo, Honeycomb, etc.) and toggle tracing per component without
forking the chart.

Rainbow and the short-lived registration jobs are intentionally excluded from
this plan: Rainbow is slated for removal in a future release, and batch jobs
do not need to be traced.

## Steps

### Step 1: Add OpenTelemetry Collector subchart and global tracing values
Goal: give the umbrella chart a single source of truth for tracing
configuration and a self-contained Collector that everything can point at.

Files affected:
- `charts/data-space-connector/Chart.yaml`
- `charts/data-space-connector/values.yaml`
- `charts/data-space-connector/templates/_helpers.tpl` (new or extended)

Actions:
- Add an `opentelemetry-collector` dependency (chart
  `open-telemetry/opentelemetry-collector`, pinned version) to
  `Chart.yaml`, gated by `opentelemetry-collector.enabled`.
- Introduce a top-level `tracing:` block in `values.yaml` with:
  - `tracing.enabled` (default `false`) – global on/off switch.
  - `tracing.exporter.otlp.endpoint` (default
    `http://{{ .Release.Name }}-opentelemetry-collector:4317`).
  - `tracing.exporter.otlp.protocol` (`grpc` | `http/protobuf`, default
    `grpc`).
  - `tracing.exporter.otlp.insecure` (default `true`).
  - `tracing.sampler` (`parentbased_traceidratio`) and
    `tracing.samplerArg` (default `1.0`).
  - `tracing.resourceAttributes` (map merged into `OTEL_RESOURCE_ATTRIBUTES`).
  - `tracing.propagators` (default `tracecontext,baggage`).
- Add a `dsc.otel.env` template helper in `_helpers.tpl` that renders the
  common `OTEL_*` environment variables block from the `tracing.*` values,
  so each deployment template can include it with a single
  `{{- include "dsc.otel.env" (dict "ctx" . "service" "identityhub") | nindent N }}`.
- Add default Collector configuration under
  `opentelemetry-collector:` in `values.yaml` (receivers: OTLP gRPC+HTTP;
  processors: batch, memory_limiter; exporters: debug + OTLP to an
  operator-provided backend; pipelines for traces only in this iteration).
- Document every new value with a `# --` helm-docs compatible comment.

Acceptance criteria:
- `helm template` renders cleanly with `tracing.enabled=false` (no Collector,
  no OTEL env vars on existing pods) and with `tracing.enabled=true`
  (Collector workload rendered, OTEL env vars injected on the workloads
  covered by later steps).
- `helm dependency update` succeeds.
- No existing values are renamed; the change is strictly additive.

### Step 2: Instrument IdentityHub deployment with OTEL
Goal: emit traces from the EDC IdentityHub component.

Files affected:
- `charts/data-space-connector/templates/identityhub-deployment.yaml`
- `charts/data-space-connector/values.yaml` (identityhub section)

Actions:
- Add an `identityhub.tracing` block in `values.yaml` with:
  - `identityhub.tracing.enabled` (default inherits `tracing.enabled`
    via the helper).
  - `identityhub.tracing.javaagent.enabled` (default `true`).
  - `identityhub.tracing.javaagent.image` (default
    `ghcr.io/open-telemetry/opentelemetry-java-instrumentation/autoinstrumentation-java:<pinned>`).
  - `identityhub.tracing.serviceName` (default `identityhub`).
- Extend the deployment template to, when tracing is enabled:
  - Add an init container that copies the OpenTelemetry Java agent JAR
    into a shared `emptyDir` volume (`otel-agent`) mounted at
    `/otel-agent`.
  - Mount that volume read-only into the IdentityHub container.
  - Append `-javaagent:/otel-agent/opentelemetry-javaagent.jar` to
    `JAVA_TOOL_OPTIONS` (env var; preserve any user-provided value by
    concatenating).
  - Inject the `OTEL_*` env vars via the `dsc.otel.env` helper
    (`OTEL_SERVICE_NAME`, `OTEL_EXPORTER_OTLP_ENDPOINT`,
    `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_TRACES_SAMPLER`,
    `OTEL_TRACES_SAMPLER_ARG`, `OTEL_PROPAGATORS`,
    `OTEL_RESOURCE_ATTRIBUTES`, `OTEL_METRICS_EXPORTER=none`,
    `OTEL_LOGS_EXPORTER=none`).
- Keep existing `env:` / `envValueFrom` / `envConfigMapNames` blocks
  intact – the new vars are appended.

Acceptance criteria:
- With `tracing.enabled=true`, `helm template` shows the init container,
  the `otel-agent` volume, and the OTEL env vars on the identityhub pod.
- With `tracing.enabled=false`, the rendered manifest is unchanged
  compared to `main`.
- `helm lint` passes.

### Step 3: Instrument fdsc-edc (EDC control/data plane) with OTEL
Goal: trace the EDC-based connector deployed by the `fdsc-edc` subchart.

Files affected:
- `charts/data-space-connector/values.yaml` (fdsc-edc section)
- `charts/data-space-connector/templates/_helpers.tpl`

Actions:
- Extend the `fdsc-edc` values with tracing configuration that maps the
  upstream subchart's extension points:
  - Use `fdsc-edc.common.additonalEnvVars` (existing list) to inject the
    `OTEL_*` environment variables.
  - Use `fdsc-edc.common.deployment.initContainers` and
    `additionalVolumes` / `additionalVolumeMounts` to mount the
    OpenTelemetry Java agent the same way as Step 2.
  - Add a `fdsc-edc.tracing` toggle documented in `values.yaml` and a
    `dsc.otel.fdscEdc` helper that emits the values diff to be merged
    into the subchart values when tracing is enabled (the umbrella chart
    can only influence the subchart via its values; no template files are
    owned here).
- Document in `values.yaml` that if the operator overrides
  `additonalEnvVars` / `initContainers` they must merge the tracing
  entries manually; provide a ready-to-copy snippet in the comments.
- Verify the EDC runtime in the pinned `fdsc-edc` image picks up
  `JAVA_TOOL_OPTIONS` (add an `EDC_` equivalent override via
  `additonalEnvVars` if the startup script does not forward
  `JAVA_TOOL_OPTIONS`).

Acceptance criteria:
- `helm template` with tracing enabled renders an fdsc-edc pod carrying
  the OTEL agent init container, shared volume, and OTEL env vars.
- With tracing disabled the fdsc-edc rendering is byte-identical to
  `main`.

### Step 4: Wire the keycloak subchart for OTEL
Goal: pass tracing configuration down to the keycloak subchart without
modifying the upstream chart.

Files affected:
- `charts/data-space-connector/values.yaml` (keycloak section)
- `charts/data-space-connector/templates/_helpers.tpl`

Actions:
- Under the `keycloak:` key, add a conditional `extraEnvVars` override
  that is only rendered when `tracing.enabled` is true and appends the
  `OTEL_*` env vars plus `KC_TRACING_ENABLED=true` (Keycloak 25+ native
  OTEL support).
- Set `OTEL_SERVICE_NAME=keycloak` (overridable via
  `keycloak.tracing.serviceName`).
- Provide the `dsc.otel.extraEnv` helper that returns a YAML list
  suitable for `extraEnvVars` blocks so this and the later subchart
  steps can share it.
- Document every new value with a `# --` helm-docs compatible comment.

Acceptance criteria:
- `helm template tracing.enabled=true` renders the keycloak StatefulSet
  with `KC_TRACING_ENABLED=true` plus the standard OTEL env vars.
- With `tracing.enabled=false`, the keycloak values are untouched
  (byte-identical rendering vs `main`).
- `helm lint` passes.

### Step 5: Wire the scorpio subchart for OTEL
Goal: enable Quarkus-native OTEL tracing on the scorpio subchart.

Files affected:
- `charts/data-space-connector/values.yaml` (scorpio section)

Actions:
- Under the `scorpio:` key, add a conditional `env` override gated on
  `tracing.enabled` that appends the Quarkus OTEL variables:
  - `QUARKUS_OTEL_ENABLED=true`
  - `QUARKUS_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` derived from
    `tracing.exporter.otlp.endpoint`.
  - `QUARKUS_OTEL_EXPORTER_OTLP_TRACES_PROTOCOL` derived from
    `tracing.exporter.otlp.protocol`.
  - `QUARKUS_OTEL_SERVICE_NAME=scorpio` (overridable via
    `scorpio.tracing.serviceName`).
- Document every new value with a `# --` helm-docs compatible comment.

Acceptance criteria:
- `helm template tracing.enabled=true` shows the scorpio deployment
  carrying the `QUARKUS_OTEL_*` env vars.
- With `tracing.enabled=false`, the scorpio rendering is unchanged.

### Step 6: Wire the tm-forum-api subchart for OTEL
Goal: expose OTEL configuration to the tm-forum-api subchart via its
`extraEnv` hook.

Files affected:
- `charts/data-space-connector/values.yaml` (tm-forum-api section)

Actions:
- Under the `tm-forum-api:` key, add a conditional `extraEnv` override
  gated on `tracing.enabled` that appends the standard `OTEL_*` env
  vars (service name defaulting to `tm-forum-api`, overridable via
  `tm-forum-api.tracing.serviceName`).
- Reuse the `dsc.otel.extraEnv` helper introduced in Step 4.
- Verify the upstream tm-forum-api chart exposes an `extraEnv`-style
  hook; if it does not, raise an upstream issue and leave the block
  commented out with a TODO referencing ticket #28.
- Document every new value with a `# --` helm-docs compatible comment.

Acceptance criteria:
- `helm template tracing.enabled=true` shows the tm-forum-api pod(s)
  carrying the standard `OTEL_*` env vars (or, if the upstream hook is
  missing, a clearly commented TODO in `values.yaml`).
- With `tracing.enabled=false`, the tm-forum-api rendering is unchanged.

### Step 7: Wire the contract-management subchart for OTEL
Goal: expose OTEL configuration to the contract-management subchart via
its `extraEnv` hook.

Files affected:
- `charts/data-space-connector/values.yaml` (contract-management section)

Actions:
- Under the `contract-management:` key, add a conditional `extraEnv`
  override gated on `tracing.enabled` that appends the standard
  `OTEL_*` env vars (service name defaulting to `contract-management`,
  overridable via `contract-management.tracing.serviceName`).
- Reuse the `dsc.otel.extraEnv` helper.
- Verify the upstream contract-management chart exposes an
  `extraEnv`-style hook; if it does not, raise an upstream issue and
  leave the block commented out with a TODO referencing ticket #28.
- Document every new value with a `# --` helm-docs compatible comment.

Acceptance criteria:
- `helm template tracing.enabled=true` shows the contract-management
  pod(s) carrying the standard `OTEL_*` env vars (or, if the upstream
  hook is missing, a clearly commented TODO in `values.yaml`).
- With `tracing.enabled=false`, the contract-management rendering is
  unchanged.

### Step 8: Wire the marketplace subchart for OTEL
Goal: expose OTEL configuration to the marketplace subchart via its
`extraEnv` hook.

Files affected:
- `charts/data-space-connector/values.yaml` (marketplace section)

Actions:
- Under the `marketplace:` key, add a conditional `extraEnv` override
  gated on `tracing.enabled` that appends the standard `OTEL_*` env
  vars (service name defaulting to `marketplace`, overridable via
  `marketplace.tracing.serviceName`).
- Reuse the `dsc.otel.extraEnv` helper.
- Verify the upstream marketplace chart exposes an `extraEnv`-style
  hook; if it does not, raise an upstream issue and leave the block
  commented out with a TODO referencing ticket #28.
- Document every new value with a `# --` helm-docs compatible comment.

Acceptance criteria:
- `helm template tracing.enabled=true` shows the marketplace pod(s)
  carrying the standard `OTEL_*` env vars (or, if the upstream hook is
  missing, a clearly commented TODO in `values.yaml`).
- With `tracing.enabled=false`, the marketplace rendering is unchanged.

### Step 9: Wire the decentralizedIam subchart for OTEL
Goal: propagate tracing configuration to the decentralizedIam subchart
as a passthrough so the dependent IAM chart can consume it.

Files affected:
- `charts/data-space-connector/values.yaml` (decentralizedIam section)

Actions:
- Under the `decentralizedIam:` key, add a `tracing` passthrough block
  that mirrors the global `tracing.*` values when `tracing.enabled` is
  true. If the upstream chart adds direct support for tracing, the
  values flow through without further work; if not, the passthrough
  has no effect.
- Document in `values.yaml` that this is a forward-compatible
  passthrough and reference the upstream chart version that needs to
  land tracing support.

Acceptance criteria:
- `helm template tracing.enabled=true` shows the decentralizedIam
  values carrying the tracing passthrough.
- With `tracing.enabled=false`, the decentralizedIam rendering is
  unchanged.

### Step 10: Add default OpenTelemetry Collector pipeline and export examples
Goal: ship a working default Collector pipeline and documented export
examples for common backends.

Files affected:
- `charts/data-space-connector/values.yaml`

Actions:
- Populate the `opentelemetry-collector:` values block with:
  - `mode: deployment`, `replicaCount: 1`.
  - `config.receivers.otlp` on `0.0.0.0:4317` (gRPC) and
    `0.0.0.0:4318` (HTTP).
  - `config.processors.batch`, `memory_limiter`, `resource`.
  - `config.exporters.debug` (verbosity `basic`) and a commented-out
    `otlp/backend` block with placeholders for Tempo/Jaeger/Honeycomb.
  - `config.service.pipelines.traces` wiring receivers → processors →
    exporters.
  - Resource limits suitable for small deployments
    (`requests: 100m/128Mi`, `limits: 500m/512Mi`).
- Add inline examples (commented YAML) showing how to:
  - Point at an external Tempo via `otlp/tempo`.
  - Enable Jaeger via an additional `jaeger` receiver.
  - Switch to HTTP protocol by overriding
    `tracing.exporter.otlp.protocol=http/protobuf`.

Acceptance criteria:
- `helm dependency update && helm template` with
  `tracing.enabled=true` yields a Collector Deployment that starts
  and logs `"Everything is ready. Begin running and processing data."`.
- Defaults only write to the `debug` exporter so no external network
  calls happen out of the box.

### Step 11: Documentation for OTEL tracing
Goal: describe how to enable tracing and connect a backend.

Files affected:
- `doc/README.md`
- `doc/deployment-integration/` (new `observability/README.md`)
- `README.md` (top-level, short mention + link)

Actions:
- Create `doc/deployment-integration/observability/README.md` covering:
  - Architecture diagram (text-based, using existing Mermaid
    conventions from `doc/flows/`).
  - How to enable tracing (`--set tracing.enabled=true`).
  - How to point at an external backend
    (`tracing.exporter.otlp.endpoint`, TLS, auth header).
  - Per-component notes (Java agent for IdentityHub/fdsc-edc, Quarkus
    for scorpio, Keycloak native OTEL, subchart `extraEnv` hooks).
  - Troubleshooting (checking Collector logs, common env var typos).
- Link the new page from `doc/README.md` under a new "Observability"
  section.
- Add a one-paragraph summary + link to the top-level `README.md`.

Acceptance criteria:
- All links resolve, Markdown passes `markdownlint` (if configured in
  CI).
- No screenshots or external assets added in this iteration (keeps PR
  small).

### Step 12: Integration test / CI verification
Goal: catch regressions in the new tracing surface automatically.

Files affected:
- `.github/workflows/test.yaml`
- `it/` (new helm-unittest or goss test files)

Actions:
- Add a helm-unittest suite under
  `charts/data-space-connector/tests/tracing_test.yaml` that asserts:
  - With `tracing.enabled=false`, the identityhub and fdsc-edc
    manifests do NOT contain `OTEL_EXPORTER_OTLP_ENDPOINT`.
  - With `tracing.enabled=true`, each of those manifests DOES contain
    `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_SERVICE_NAME`.
  - With `tracing.enabled=true`, a Deployment named
    `*-opentelemetry-collector` is rendered.
- Extend `.github/workflows/test.yaml` to run `helm unittest` on the
  chart (install via the official action) as a new job step.
- Keep the existing integration tests untouched.

Acceptance criteria:
- The new unittest suite passes locally via
  `helm unittest charts/data-space-connector`.
- The CI job defined in `test.yaml` passes on a clean checkout.
- Tests use table/parameterised inputs where practical (per repo code
  quality rules).

### Step 13: Release notes and chart version bump
Goal: make the change discoverable and version-compliant.

Files affected:
- `charts/data-space-connector/Chart.yaml`
- `doc/release-notes/9-x.md` (appended to existing 9.x release notes)
- `README.md` (release information section updated)

Actions:
- Bump `version` in `Chart.yaml` to `9.1.0` (semver minor for additive
  functionality).
- Append an "OpenTelemetry Distributed Tracing (9.1.0)" section to the
  existing `doc/release-notes/9-x.md` following the convention of one
  file per major version series.
- Add a 9.1.0 sub-bullet under the 9.x.x release entry in the top-level
  `README.md` Release Information section.

Note: The plan originally called for helm-docs regeneration, but the
repository does not use helm-docs and has no chart-level README.md with
a generated values table. Instead, the new `tracing.*` keys are
documented directly in the release notes and in the observability guide
created in Step 11.

Acceptance criteria:
- `Chart.yaml` version is `9.1.0`.
- `doc/release-notes/9-x.md` contains the new tracing section.
- Top-level `README.md` references the new version.
- No existing release notes sections are modified.
