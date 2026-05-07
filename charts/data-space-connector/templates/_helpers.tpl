{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "dsc.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "dsc.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "dsc.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "dsc.serviceAccountName" -}}
{{- if .Values.did.serviceAccount.create -}}
    {{ default (include "dsc.fullname" .) .Values.did.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.did.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "dsc.labels" -}}
app.kubernetes.io/name: {{ include "dsc.name" . }}
helm.sh/chart: {{ include "dsc.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Default OTLP endpoint used when `tracing.exporter.otlp.endpoint` is empty.
Targets the in-cluster OpenTelemetry Collector deployed by the bundled
subchart. The service name follows the upstream chart convention of
`<release>-opentelemetry-collector`.
*/}}
{{- define "dsc.otel.defaultEndpoint" -}}
{{- printf "http://%s-opentelemetry-collector:4317" .Release.Name -}}
{{- end -}}

{{/*
Effective OTLP endpoint: operator override wins, otherwise fall back to
the in-cluster collector service. Always call with the root context (`.`).
*/}}
{{- define "dsc.otel.endpoint" -}}
{{- $tracing := .Values.tracing | default dict -}}
{{- $exporter := (get $tracing "exporter") | default dict -}}
{{- $otlp := (get $exporter "otlp") | default dict -}}
{{- $endpoint := get $otlp "endpoint" -}}
{{- if $endpoint -}}
{{- $endpoint -}}
{{- else -}}
{{- include "dsc.otel.defaultEndpoint" . -}}
{{- end -}}
{{- end -}}

{{/*
Render the `OTEL_RESOURCE_ATTRIBUTES` value for a given service.
Merges `service.name=<service>` with the user-supplied
`tracing.resourceAttributes` map. Call with
  (dict "ctx" . "service" "<name>")
*/}}
{{- define "dsc.otel.resourceAttributes" -}}
{{- $ctx := .ctx -}}
{{- $service := .service -}}
{{- $tracing := $ctx.Values.tracing | default dict -}}
{{- $attrs := get $tracing "resourceAttributes" | default dict -}}
{{- $pairs := list (printf "service.name=%s" $service) -}}
{{- range $k, $v := $attrs -}}
{{- $pairs = append $pairs (printf "%s=%s" $k $v) -}}
{{- end -}}
{{- join "," $pairs -}}
{{- end -}}

{{/*
Render the standard OpenTelemetry environment-variable block as a list of
`name:` / `value:` entries suitable for splicing into a container's
`env:` list. The helper renders nothing when tracing is disabled, so the
call site stays a no-op for untraced deployments.

By default the gate is the global `tracing.enabled` value. Callers that
carry a per-component override (e.g. `identityhub.tracing.enabled`) can
pass an explicit `enabled` key in the argument dict; when present, its
value is authoritative and the global flag is ignored. This keeps the
"default inherits the global toggle" contract while still letting a
component opt in or out independently.

Usage:
  {{- include "dsc.otel.env" (dict "ctx" . "service" "identityhub") | nindent 12 }}
  {{- include "dsc.otel.env" (dict "ctx" . "service" "identityhub" "enabled" true) | nindent 12 }}

The helper emits these vars (per OpenTelemetry SDK environment spec):
  OTEL_SERVICE_NAME
  OTEL_EXPORTER_OTLP_ENDPOINT
  OTEL_EXPORTER_OTLP_PROTOCOL
  OTEL_EXPORTER_OTLP_INSECURE
  OTEL_TRACES_SAMPLER
  OTEL_TRACES_SAMPLER_ARG
  OTEL_PROPAGATORS
  OTEL_RESOURCE_ATTRIBUTES
  OTEL_METRICS_EXPORTER  (forced to `none` in this iteration)
  OTEL_LOGS_EXPORTER     (forced to `none` in this iteration)
*/}}
{{- define "dsc.otel.env" -}}
{{- $ctx := .ctx -}}
{{- $service := .service -}}
{{- $tracing := $ctx.Values.tracing | default dict -}}
{{- $enabled := get . "enabled" -}}
{{- if kindIs "invalid" $enabled -}}
{{- $enabled = get $tracing "enabled" -}}
{{- end -}}
{{- if $enabled -}}
{{- $exporter := get $tracing "exporter" | default dict -}}
{{- $otlp := get $exporter "otlp" | default dict -}}
{{- $protocol := get $otlp "protocol" | default "grpc" -}}
{{- $insecure := get $otlp "insecure" -}}
{{- if kindIs "invalid" $insecure -}}{{- $insecure = true -}}{{- end -}}
{{- $sampler := get $tracing "sampler" | default "parentbased_traceidratio" -}}
{{- $samplerArg := get $tracing "samplerArg" | default "1.0" -}}
{{- $propagators := get $tracing "propagators" | default "tracecontext,baggage" -}}
- name: OTEL_SERVICE_NAME
  value: {{ $service | quote }}
- name: OTEL_EXPORTER_OTLP_ENDPOINT
  value: {{ include "dsc.otel.endpoint" $ctx | quote }}
- name: OTEL_EXPORTER_OTLP_PROTOCOL
  value: {{ $protocol | quote }}
- name: OTEL_EXPORTER_OTLP_INSECURE
  value: {{ ternary "true" "false" $insecure | quote }}
- name: OTEL_TRACES_SAMPLER
  value: {{ $sampler | quote }}
- name: OTEL_TRACES_SAMPLER_ARG
  value: {{ $samplerArg | quote }}
- name: OTEL_PROPAGATORS
  value: {{ $propagators | quote }}
- name: OTEL_RESOURCE_ATTRIBUTES
  value: {{ include "dsc.otel.resourceAttributes" (dict "ctx" $ctx "service" $service) | quote }}
- name: OTEL_METRICS_EXPORTER
  value: "none"
- name: OTEL_LOGS_EXPORTER
  value: "none"
{{- end -}}
{{- end -}}

{{/*
Render the same OpenTelemetry environment-variable block as a YAML list
suitable for subchart `extraEnv` / `extraEnvVars` hooks (keycloak,
tm-forum-api, contract-management, marketplace). Behaviour matches
`dsc.otel.env`; kept separate so call sites stay explicit about the
surface they are wiring into.

Usage:
  extraEnv:
    {{- include "dsc.otel.extraEnv" (dict "ctx" . "service" "keycloak") | nindent 4 }}
*/}}
{{- define "dsc.otel.extraEnv" -}}
{{- include "dsc.otel.env" . -}}
{{- end -}}

{{/*
Name of the ConfigMap that carries the OTEL tracing env vars for the
Keycloak StatefulSet. Used by the `keycloak-tracing-cm.yaml` template
and referenced from the umbrella chart's `keycloak.extraEnvVarsCM`
value (which the Bitnami chart processes through `tpl`).

Usage (in templates):
  {{ include "dsc.otel.keycloak.cmName" . }}
*/}}
{{- define "dsc.otel.keycloak.cmName" -}}
{{- printf "%s-keycloak-tracing" .Release.Name -}}
{{- end -}}

{{/*
Render the OpenTelemetry environment-variable block as a YAML list for
the `fdsc-edc` subchart. Because fdsc-edc is a third-party subchart
whose templates are not owned by this umbrella chart, the env vars
cannot be injected via template includes – they must live in the static
`values.yaml` under `fdsc-edc.common.deployment.additionalEnvVars`.

This helper exists as a *reference implementation* so operators and CI
scripts can generate the correct env-var block dynamically.  For
example, to produce the block and inspect it during development:

  helm template . --show-only templates/_helpers.tpl \
    --set tracing.enabled=true \
    -x <(echo '{{- include "dsc.otel.fdscEdc.envList" . -}}')

The output is a list of `name:` / `value:` entries identical to those
emitted by `dsc.otel.env` for the IdentityHub workload, prefixed with
the `JAVA_TOOL_OPTIONS` entry that loads the Java agent.

Usage (from values tooling, not from a template):
  {{- include "dsc.otel.fdscEdc.envList" . }}
*/}}
{{- define "dsc.otel.fdscEdc.envList" -}}
{{- $tracing := .Values.tracing | default dict -}}
{{- $fdscTracing := (index .Values "fdsc-edc").tracing | default dict -}}
{{- $enabled := get $fdscTracing "enabled" -}}
{{- if kindIs "invalid" $enabled -}}
{{- $enabled = get $tracing "enabled" -}}
{{- end -}}
{{- if $enabled -}}
{{- $serviceName := default "fdsc-edc" (get $fdscTracing "serviceName") -}}
- name: JAVA_TOOL_OPTIONS
  value: "-javaagent:/otel-agent/opentelemetry-javaagent.jar"
{{- include "dsc.otel.env" (dict "ctx" . "service" $serviceName "enabled" true) }}
{{- end -}}
{{- end -}}

{{/*
Render the OpenTelemetry environment-variable block as a YAML list for
the `scorpio` subchart. Because scorpio is a Quarkus application, it
uses `QUARKUS_OTEL_*` configuration properties rather than the standard
`OTEL_*` SDK env vars. The helper reads the global `tracing` block and
the per-component `scorpio.tracing` overrides.

This helper exists as a *reference implementation* so operators and CI
scripts can generate the correct env-var block dynamically. For example,
to produce the block and inspect it during development:

  helm template . --show-only templates/_helpers.tpl \
    --set tracing.enabled=true \
    -x <(echo '{{- include "dsc.otel.scorpio.envList" . -}}')

The output is a list of Quarkus-specific `name:` / `value:` entries that
map to the standard OpenTelemetry SDK configuration.

Usage (from values tooling, not from a template):
  {{- include "dsc.otel.scorpio.envList" . }}
*/}}
{{- define "dsc.otel.scorpio.envList" -}}
{{- $tracing := .Values.tracing | default dict -}}
{{- $scorpioTracing := .Values.scorpio.tracing | default dict -}}
{{- $enabled := get $scorpioTracing "enabled" -}}
{{- if kindIs "invalid" $enabled -}}
{{- $enabled = get $tracing "enabled" -}}
{{- end -}}
{{- if $enabled -}}
{{- $serviceName := default "scorpio" (get $scorpioTracing "serviceName") -}}
{{- $exporter := get $tracing "exporter" | default dict -}}
{{- $otlp := get $exporter "otlp" | default dict -}}
{{- $protocol := get $otlp "protocol" | default "grpc" -}}
- name: QUARKUS_OTEL_ENABLED
  value: "true"
- name: QUARKUS_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
  value: {{ include "dsc.otel.endpoint" . | quote }}
- name: QUARKUS_OTEL_SERVICE_NAME
  value: {{ $serviceName | quote }}
- name: QUARKUS_OTEL_EXPORTER_OTLP_TRACES_PROTOCOL
  value: {{ $protocol | quote }}
{{- end -}}
{{- end -}}

{{/*
Render the OpenTelemetry environment-variable block as a YAML list for
the `tm-forum-api` subchart. tm-forum-api is a Java application that
supports the standard `OTEL_*` SDK environment variables. Because
tm-forum-api is a third-party subchart whose templates are not owned by
this umbrella chart, the env vars must live in the static `values.yaml`
under `tm-forum-api.defaultConfig.additionalEnvVars`.

This helper exists as a *reference implementation* so operators and CI
scripts can generate the correct env-var block dynamically. For example,
to produce the block and inspect it during development:

  helm template . --show-only templates/_helpers.tpl \
    --set tracing.enabled=true \
    -x <(echo '{{- include "dsc.otel.tmForumApi.envList" . -}}')

The output is a list of standard `OTEL_*` `name:` / `value:` entries
suitable for appending to `defaultConfig.additionalEnvVars`.

Usage (from values tooling, not from a template):
  {{- include "dsc.otel.tmForumApi.envList" . }}
*/}}
{{- define "dsc.otel.tmForumApi.envList" -}}
{{- $tracing := .Values.tracing | default dict -}}
{{- $tmfTracing := (index .Values "tm-forum-api").tracing | default dict -}}
{{- $enabled := get $tmfTracing "enabled" -}}
{{- if kindIs "invalid" $enabled -}}
{{- $enabled = get $tracing "enabled" -}}
{{- end -}}
{{- if $enabled -}}
{{- $serviceName := default "tm-forum-api" (get $tmfTracing "serviceName") -}}
- name: OTEL_SDK_DISABLED
  value: "false"
{{- include "dsc.otel.env" (dict "ctx" . "service" $serviceName "enabled" true) }}
{{- end -}}
{{- end -}}

{{/*
Render the OpenTelemetry environment-variable block as a YAML list for
the `contract-management` subchart. contract-management is a Micronaut
(Java) application that supports the standard `OTEL_*` SDK environment
variables. Because contract-management is a third-party subchart whose
templates are not owned by this umbrella chart, the env vars must live in
the static `values.yaml` under `contract-management.additionalEnvVars`.

This helper exists as a *reference implementation* so operators and CI
scripts can generate the correct env-var block dynamically. For example,
to produce the block and inspect it during development:

  helm template . --show-only templates/_helpers.tpl \
    --set tracing.enabled=true \
    -x <(echo '{{- include "dsc.otel.contractManagement.envList" . -}}')

The output is a list of standard `OTEL_*` `name:` / `value:` entries
suitable for appending to `additionalEnvVars`.

Usage (from values tooling, not from a template):
  {{- include "dsc.otel.contractManagement.envList" . }}
*/}}
{{- define "dsc.otel.contractManagement.envList" -}}
{{- $tracing := .Values.tracing | default dict -}}
{{- $cmTracing := (index .Values "contract-management").tracing | default dict -}}
{{- $enabled := get $cmTracing "enabled" -}}
{{- if kindIs "invalid" $enabled -}}
{{- $enabled = get $tracing "enabled" -}}
{{- end -}}
{{- if $enabled -}}
{{- $serviceName := default "contract-management" (get $cmTracing "serviceName") -}}
- name: OTEL_SDK_DISABLED
  value: "false"
{{- include "dsc.otel.env" (dict "ctx" . "service" $serviceName "enabled" true) }}
{{- end -}}
{{- end -}}

{{/*
Render the OpenTelemetry environment-variable block as a YAML list for
the `marketplace` subchart. The marketplace (business-api-ecosystem) is
composed of two main components – the Charging Backend (Python/Django)
and the Logic Proxy (Node.js) – each with its own env-var extension
point (`extraEnvVars` and `additionalEnvVars` respectively). Both
components support the standard `OTEL_*` SDK environment variables.

Because marketplace is a third-party subchart whose templates are not
owned by this umbrella chart, the env vars must live in the static
`values.yaml` under `marketplace.bizEcosystemChargingBackend.extraEnvVars`
and `marketplace.bizEcosystemLogicProxy.additionalEnvVars`.

This helper exists as a *reference implementation* so operators and CI
scripts can generate the correct env-var block dynamically. For example,
to produce the block and inspect it during development:

  helm template . --show-only templates/_helpers.tpl \
    --set tracing.enabled=true \
    -x <(echo '{{- include "dsc.otel.marketplace.envList" . -}}')

The output is a list of standard `OTEL_*` `name:` / `value:` entries
suitable for appending to the marketplace subcomponent env-var hooks.

Usage (from values tooling, not from a template):
  {{- include "dsc.otel.marketplace.envList" . }}
*/}}
{{- define "dsc.otel.marketplace.envList" -}}
{{- $tracing := .Values.tracing | default dict -}}
{{- $mktTracing := .Values.marketplace.tracing | default dict -}}
{{- $enabled := get $mktTracing "enabled" -}}
{{- if kindIs "invalid" $enabled -}}
{{- $enabled = get $tracing "enabled" -}}
{{- end -}}
{{- if $enabled -}}
{{- $serviceName := default "marketplace" (get $mktTracing "serviceName") -}}
- name: OTEL_SDK_DISABLED
  value: "false"
{{- include "dsc.otel.env" (dict "ctx" . "service" $serviceName "enabled" true) }}
{{- end -}}
{{- end -}}

{{/*
OTLP gRPC endpoint of the bundled Grafana Tempo service. Used by the
otel-collector-config-cm.yaml template to auto-wire the Collector→Tempo
pipeline when `tempo.enabled=true`. The service name follows the upstream
Tempo chart convention of `<release>-tempo`.

Usage:
  {{ include "dsc.tempo.endpoint" . }}  → "http://<release>-tempo:4317"
*/}}
{{- define "dsc.tempo.endpoint" -}}
{{- printf "http://%s-tempo:4317" .Release.Name -}}
{{- end -}}

{{/*
Name of the ConfigMap that carries the OTEL Collector pipeline
configuration (key: `relay`). Referenced by the opentelemetry-collector
subchart via `configMap.existingName` and rendered by
otel-collector-config-cm.yaml.

Usage:
  {{ include "dsc.otel.collectorConfigName" . }}
*/}}
{{- define "dsc.otel.collectorConfigName" -}}
{{- printf "%s-otel-collector-config" .Release.Name -}}
{{- end -}}
