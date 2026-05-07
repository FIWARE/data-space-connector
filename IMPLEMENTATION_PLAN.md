# Implementation Plan: Grafana Tempo Integration with the OpenTelemetry Integration

## Overview
Build on the existing OpenTelemetry tracing infrastructure (added in 9.1.0 /
ticket-28) to provide a turnkey **Grafana Tempo** trace-storage backend and a
**Grafana** dashboard for trace visualisation, both deployed as optional
subchart dependencies.  When Tempo is enabled the OTEL Collector pipeline is
auto-wired to export spans to Tempo; when Grafana is also enabled it is
auto-provisioned with Tempo as a datasource.  The operator experience is:
```yaml
tracing:
  enabled: true
tempo:
  enabled: true
grafana:
  enabled: true
```
and the full stack (workloads -> Collector -> Tempo -> Grafana) works out of
the box with zero additional configuration.

## Steps

### Step 1: Add Grafana Tempo subchart dependency and values

**Goal:** deploy Grafana Tempo as an optional in-cluster trace backend.

**Files affected:**
- `charts/data-space-connector/Chart.yaml`
- `charts/data-space-connector/values.yaml`

**Actions:**
- Add `grafana/tempo` (chart `tempo`, pinned to `1.24.4`, repo
  `https://grafana.github.io/helm-charts`) as a dependency in `Chart.yaml`,
  gated by `tempo.enabled`.
- Add a `tempo:` values block at the end of `values.yaml` (after the
  `opentelemetry-collector:` section) with:
  - `tempo.enabled: false` (default off).
  - `tempo.tempo.retention: 48h` (sensible default trace retention).
  - `tempo.tempo.receivers.otlp.protocols.grpc.endpoint: "0.0.0.0:4317"` and
    `http.endpoint: "0.0.0.0:4318"` -- Tempo listens for OTLP directly.
  - `tempo.tempo.storage.trace.backend: local` with
    `local.path: /var/tempo/traces` and `wal.path: /var/tempo/wal`
    (single-binary local storage, suitable for dev/test; operators override
    for S3/GCS in production).
  - Minimal resource requests (`cpu: 100m`, `memory: 256Mi`) and limits
    (`cpu: 500m`, `memory: 1Gi`).
  - Disable receivers the DSC does not use (Jaeger, OpenCensus) to keep the
    Tempo surface minimal; only OTLP gRPC + HTTP are enabled.
- Document every new value with a `# --` helm-docs-compatible comment.
- Run `helm dependency update charts/data-space-connector` successfully.

**Acceptance criteria:**
- `helm dependency update` pulls the `tempo` chart.
- `helm template` with `tempo.enabled=false` does not render any Tempo
  resources.
- `helm template` with `tempo.enabled=true` renders a Tempo StatefulSet and
  Service.
- `helm lint` passes.
- No existing values are renamed or removed.

---

### Step 2: Add Grafana subchart dependency and values

**Goal:** deploy Grafana as an optional in-cluster dashboard for trace
visualisation.

**Files affected:**
- `charts/data-space-connector/Chart.yaml`
- `charts/data-space-connector/values.yaml`

**Actions:**
- Add `grafana/grafana` (pinned to `10.5.15`, repo
  `https://grafana.github.io/helm-charts`) as a dependency in `Chart.yaml`,
  gated by `grafana.enabled`.
- Add a `grafana:` values block at the end of `values.yaml` (after the
  `tempo:` section) with:
  - `grafana.enabled: false` (default off).
  - `grafana.sidecar.datasources.enabled: true` -- enable the k8s-sidecar
    that watches for ConfigMaps labelled `grafana_datasource` and
    auto-provisions them as Grafana datasources.
  - `grafana.sidecar.datasources.label: grafana_datasource` (default label).
  - Sensible resource requests (`cpu: 100m`, `memory: 128Mi`) and limits
    (`cpu: 500m`, `memory: 512Mi`).
  - `grafana.persistence.enabled: false` (no PVC by default; operators
    enable for production).
  - `grafana.adminPassword` -- set a documented default (e.g. `"admin"`)
    with a clear `# --` comment instructing operators to override.
- Document every new value with a `# --` helm-docs-compatible comment.
- Run `helm dependency update charts/data-space-connector` successfully.

**Acceptance criteria:**
- `helm dependency update` pulls the `grafana` chart.
- `helm template` with `grafana.enabled=false` does not render any Grafana
  resources.
- `helm template` with `grafana.enabled=true` renders a Grafana Deployment
  and Service with the datasource sidecar container present.
- `helm lint` passes.
- No existing values are renamed or removed.

---

### Step 3: Create Collector ConfigMap template with auto-wired Tempo pipeline

**Goal:** when Tempo is enabled, automatically configure the OTEL Collector
to export spans to the in-cluster Tempo instance -- no manual collector config
required.

**Files affected:**
- `charts/data-space-connector/templates/otel-collector-config-cm.yaml` (new)
- `charts/data-space-connector/templates/_helpers.tpl`
- `charts/data-space-connector/values.yaml`

**Actions:**
- Create `otel-collector-config-cm.yaml`:
  - Renders a ConfigMap (key: `relay`, as required by the OTEL Collector
    subchart) only when `tracing.enabled=true`.
  - Reads the user's `opentelemetry-collector.config` values as the base
    pipeline definition (receivers, processors, exporters, service).
  - When `tempo.enabled=true`, appends an `otlp/tempo` exporter block with
    `endpoint: http://<release>-tempo:4317` (computed via a helper) and
    `tls.insecure: true`, and adds `otlp/tempo` to the
    `service.pipelines.traces.exporters` list.
  - When `tempo.enabled=false`, renders the config as-is (identical to what
    the subchart would produce from values).
  - Include the Apache-2.0 license header.
- Add `dsc.tempo.endpoint` helper in `_helpers.tpl` that returns
  `http://<release>-tempo:4317` (the OTLP gRPC endpoint of the bundled
  Tempo service).
- Update `values.yaml`:
  - Set `opentelemetry-collector.configMap.create: false` so the subchart
    does not create its own ConfigMap.
  - Set `opentelemetry-collector.configMap.existingName` to our custom
    ConfigMap name (template-rendered, e.g.
    `"{{ .Release.Name }}-otel-collector-config"`; the subchart's
    `existingName` field supports template content).
- Update the inline comments in the `opentelemetry-collector.config` block
  to note that the actual ConfigMap is generated by
  `otel-collector-config-cm.yaml` and the `otlp/tempo` exporter is
  auto-injected when `tempo.enabled=true`.

**Acceptance criteria:**
- `helm template` with `tracing.enabled=true, tempo.enabled=false` renders
  a ConfigMap with the same pipeline as before (debug exporter only).
- `helm template` with `tracing.enabled=true, tempo.enabled=true` renders
  a ConfigMap whose `relay` key includes the `otlp/tempo` exporter
  targeting `http://<release>-tempo:4317` and `otlp/tempo` appears in
  `service.pipelines.traces.exporters`.
- `helm template` with `tracing.enabled=false` does NOT render the
  ConfigMap.
- `helm lint` passes.

---

### Step 4: Create Grafana Tempo datasource auto-provisioning template

**Goal:** when both Grafana and Tempo are enabled, automatically provision
Tempo as a Grafana datasource so operators see traces in the Grafana UI
immediately.

**Files affected:**
- `charts/data-space-connector/templates/grafana-tempo-datasource-cm.yaml` (new)
- `charts/data-space-connector/templates/_helpers.tpl`

**Actions:**
- Create `grafana-tempo-datasource-cm.yaml`:
  - Renders a ConfigMap only when both `grafana.enabled=true` AND
    `tempo.enabled=true`.
  - Labels the ConfigMap with `grafana_datasource: "1"` so the Grafana
    sidecar picks it up automatically.
  - Data key `tempo-datasource.yaml` contains:
    ```yaml
    apiVersion: 1
    datasources:
      - name: Tempo
        type: tempo
        access: proxy
        url: http://<release>-tempo:3200
        isDefault: true
        editable: true
    ```
    (URL computed via a helper referencing the Tempo HTTP query endpoint on
    port 3200.)
  - Include the Apache-2.0 license header.
- Add `dsc.tempo.queryEndpoint` helper in `_helpers.tpl` that returns
  `http://<release>-tempo:3200` (the Tempo HTTP API endpoint used by
  Grafana for queries).

**Acceptance criteria:**
- `helm template` with `grafana.enabled=true, tempo.enabled=true` renders
  the datasource ConfigMap with the correct label and Tempo URL.
- `helm template` with `grafana.enabled=false` OR `tempo.enabled=false`
  does NOT render the datasource ConfigMap.
- `helm lint` passes.

---

### Step 5: Helm unittest tests for Tempo and Grafana integration

**Goal:** catch regressions in the new Tempo/Grafana surface automatically.

**Files affected:**
- `charts/data-space-connector/tests/tempo_test.yaml` (new)

**Actions:**
- Create a helm-unittest suite that asserts:
  - **Collector ConfigMap tests:**
    - With `tracing.enabled=false`: the `otel-collector-config-cm` is NOT
      rendered.
    - With `tracing.enabled=true, tempo.enabled=false`: the ConfigMap IS
      rendered and does NOT contain `otlp/tempo` in the `relay` key.
    - With `tracing.enabled=true, tempo.enabled=true`: the ConfigMap IS
      rendered and DOES contain `otlp/tempo` exporter with the correct
      Tempo endpoint.
  - **Grafana datasource ConfigMap tests:**
    - With `grafana.enabled=false, tempo.enabled=true`: the datasource
      ConfigMap is NOT rendered.
    - With `grafana.enabled=true, tempo.enabled=false`: the datasource
      ConfigMap is NOT rendered.
    - With `grafana.enabled=true, tempo.enabled=true`: the datasource
      ConfigMap IS rendered with `grafana_datasource` label and correct
      Tempo query URL.
- Use table/parameterised test inputs where practical (per repo code
  quality rules).
- Verify tests pass locally via `helm unittest charts/data-space-connector`.

**Acceptance criteria:**
- All new tests pass locally.
- Tests are idempotent and do not depend on external state.
- Existing `tracing_test.yaml` tests continue to pass unchanged.

---

### Step 6: Update observability documentation

**Goal:** document the Grafana Tempo integration end-to-end.

**Files affected:**
- `doc/deployment-integration/observability/README.md`
- `doc/README.md`

**Actions:**
- Expand `doc/deployment-integration/observability/README.md` with a new
  section **"Grafana Tempo Backend (In-Cluster)"** covering:
  - Architecture diagram update (Mermaid): add Tempo and Grafana to the
    existing flow (`Collector -> Tempo -> Grafana`).
  - How to enable the full stack:
    ```yaml
    tracing:
      enabled: true
    tempo:
      enabled: true
    grafana:
      enabled: true
    ```
  - Explanation that the Collector->Tempo and Tempo->Grafana wiring is
    automatic when the respective subcharts are enabled.
  - How to use an **external Tempo** instance (disable the subchart, point
    the collector at the external endpoint -- same as existing docs but with
    a clear distinction).
  - Production considerations: Tempo storage backends (S3, GCS, Azure),
    Tempo retention, Grafana persistence, ingress configuration.
  - Troubleshooting: checking Tempo readiness, verifying traces appear in
    Grafana.
- Add a new ToC entry in the observability doc.
- Link the Tempo section from `doc/README.md` under the existing
  "Observability" entry.

**Acceptance criteria:**
- All internal links resolve.
- Existing documentation sections are unmodified except for new additions.
- No screenshots or external assets added (keeps PR small).

---

### Step 7: Release notes and chart version bump

**Goal:** make the Grafana Tempo integration discoverable and version-
compliant.

**Files affected:**
- `charts/data-space-connector/Chart.yaml`
- `doc/release-notes/9-x.md`
- `CLAUDE.md`

**Actions:**
- Bump `version` in `Chart.yaml` from `9.1.0` to `9.2.0` (semver minor
  for additive functionality).
- Append a "Grafana Tempo Integration (9.2.0)" section to
  `doc/release-notes/9-x.md` following the existing convention. Cover:
  - New optional dependencies (`tempo`, `grafana`).
  - Auto-wired Collector->Tempo pipeline.
  - Auto-provisioned Grafana datasource.
  - New values keys added.
  - No breaking changes; fully opt-in.
- Update `CLAUDE.md` to reflect the new subchart dependencies, new template
  files, new test file, and the updated chart version.

**Acceptance criteria:**
- `Chart.yaml` version is `9.2.0`.
- `doc/release-notes/9-x.md` contains the new Tempo integration section.
- No existing release notes sections are modified.
- `helm lint` passes.
