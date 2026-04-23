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
