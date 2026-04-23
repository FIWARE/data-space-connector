# Implementation Plan: OpenTelemetry Tracing in FIWARE Data Space Connector

## Overview
Introduce end-to-end distributed tracing across the FIWARE Data Space Connector
(DSC) by deploying an OpenTelemetry (OTEL) Collector as an optional subchart of
the `data-space-connector` umbrella chart and wiring every first-party workload
(IdentityHub, fdsc-edc, Rainbow, and the DSC-owned job runners) to export
traces through the OTLP endpoint. Trace configuration is exposed via
`values.yaml` so operators can plug any OTLP-compatible backend (Jaeger,
Tempo, Honeycomb, etc.) and toggle tracing per component without forking
the chart.

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

### Step 4: Instrument Rainbow deployment with OTEL
Goal: trace the Rainbow DSP implementation (`quay.io/wi_stefan/rainbow`).

Files affected:
- `charts/data-space-connector/templates/rainbow-deployment.yaml`
- `charts/data-space-connector/values.yaml` (rainbow section)

Actions:
- Add a `rainbow.tracing` block to `values.yaml`
  (`enabled`, `serviceName`, inherited exporter settings).
- In the deployment template, when tracing is enabled append the
  standard `OTEL_*` env vars to both the `rainbow-init` and `rainbow`
  containers using the `dsc.otel.env` helper. Rainbow is written in
  Rust and honours the upstream OTLP SDK environment variables (no
  Java agent needed).
- Leave the hard-coded `DB_*` env block untouched; add the tracing
  vars after it.

Acceptance criteria:
- With tracing enabled, the rainbow pod spec shows OTEL env vars in both
  containers.
- With tracing disabled the rainbow pod spec is unchanged.

### Step 5: Instrument registration jobs with OTEL
Goal: trace the short-lived batch jobs the chart creates
(`participant-registration-job.yaml`, `tmf-registration-job.yaml`,
`dataplane-registration.yaml`, `rainbow-registration.yaml`,
`participant-registration.yaml`).

Files affected:
- `charts/data-space-connector/templates/participant-registration-job.yaml`
- `charts/data-space-connector/templates/tmf-registration-job.yaml`
- `charts/data-space-connector/templates/dataplane-registration.yaml`
- `charts/data-space-connector/templates/rainbow-registration.yaml`
- `charts/data-space-connector/templates/participant-registration.yaml`
- `charts/data-space-connector/values.yaml`

Actions:
- When `tracing.enabled` is true, inject `OTEL_*` env vars into each
  job's container via the `dsc.otel.env` helper with a
  job-specific `service.name` (e.g. `participant-registration-job`).
- For Java-based jobs that go through the identityhub image, reuse the
  same Java agent init container pattern as Step 2 via a shared partial
  (`dsc.otel.javaInit`).
- Curl-based shell jobs only get the OTLP propagation headers via the
  `W3C traceparent` env (they do not emit spans themselves); document
  this limitation.

Acceptance criteria:
- Each job, when rendered with tracing enabled, carries the expected
  OTEL env vars.
- `helm template` output with tracing disabled is unchanged.

### Step 6: Wire subchart components (keycloak, scorpio, tm-forum-api, contract-management, marketplace, decentralizedIam)
Goal: pass tracing configuration down to the third-party subcharts
that already support OTEL natively, without modifying those charts.

Files affected:
- `charts/data-space-connector/values.yaml`
- `charts/data-space-connector/templates/_helpers.tpl`

Actions:
- For each subchart, add conditional value overrides under its
  namespaced key when `tracing.enabled` is true. Specifically:
  - `keycloak.extraEnvVars` – append `OTEL_*` env vars and enable
    `KC_TRACING_ENABLED=true` (Keycloak 25+ native OTEL support).
  - `scorpio.env` – add `QUARKUS_OTEL_*` vars (`QUARKUS_OTEL_ENABLED`,
    `QUARKUS_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`,
    `QUARKUS_OTEL_SERVICE_NAME`).
  - `tm-forum-api.extraEnv` / `contract-management.extraEnv` /
    `marketplace.extraEnv` – append standard `OTEL_*` vars; note in
    comments that the upstream charts must expose an `extraEnv`-style
    hook (if they do not, raise an upstream issue and leave the block
    commented out with a TODO referencing ticket #28).
  - `decentralizedIam` – propagate `tracing` block as a passthrough
    value so the dependent IAM chart can consume it if it adds
    support.
- Use Helm's nested-values override mechanism: define defaults here and
  let the values merge into the subcharts at install time.
- Provide a single `dsc.otel.extraEnv` helper that returns a YAML list
  suitable for `extraEnv`/`extraEnvVars` blocks.

Acceptance criteria:
- Rendering the umbrella chart with `tracing.enabled=true` produces
  pods (in the subcharts that expose `extraEnv`-like hooks) with the
  correct OTEL env vars.
- With `tracing.enabled=false`, no subchart values are overridden.
- Comments clearly mark any subchart that currently lacks a hook.

### Step 7: Add default OpenTelemetry Collector pipeline and export examples
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

### Step 8: Documentation for OTEL tracing
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
  - Per-component notes (Java agent, Rust SDK, Quarkus, Keycloak).
  - Troubleshooting (checking Collector logs, common env var typos).
- Link the new page from `doc/README.md` under a new "Observability"
  section.
- Add a one-paragraph summary + link to the top-level `README.md`.

Acceptance criteria:
- All links resolve, Markdown passes `markdownlint` (if configured in
  CI).
- No screenshots or external assets added in this iteration (keeps PR
  small).

### Step 9: Integration test / CI verification
Goal: catch regressions in the new tracing surface automatically.

Files affected:
- `.github/workflows/test.yaml`
- `it/` (new helm-unittest or goss test files)

Actions:
- Add a helm-unittest suite under
  `charts/data-space-connector/tests/tracing_test.yaml` that asserts:
  - With `tracing.enabled=false`, the identityhub, rainbow, and fdsc-edc
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

### Step 10: Release notes and chart version bump
Goal: make the change discoverable and version-compliant.

Files affected:
- `charts/data-space-connector/Chart.yaml`
- `doc/release-notes/` (new file, following existing naming)
- `README.md` (values table regeneration if helm-docs is used)

Actions:
- Bump `version` in `Chart.yaml` to the next minor (e.g. `9.1.0`) as
  per semver since functionality is additive.
- Add a release note entry under `doc/release-notes/` following the
  style of existing entries (scan latest file for conventions).
- Regenerate the values table in the top-level `README.md` via
  `helm-docs` (keep diff minimal – run only inside the chart path).

Acceptance criteria:
- `Chart.yaml` version matches the release note filename.
- `README.md` values table reflects the new `tracing.*` keys.
- No existing release notes are modified.
