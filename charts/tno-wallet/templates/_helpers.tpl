{{/*
Expand the name of the chart.
*/}}
{{- define "tsg.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "tsg.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "tsg.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "tsg.labels" -}}
helm.sh/chart: {{ include "tsg.chart" . }}
{{ include "tsg.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "tsg.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tsg.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "tsg.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "tsg.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{- define "tsg.subPath" -}}
{{- printf "/%s" (trimPrefix "/" .Values.subPath) | trimSuffix "/"}}
{{- end }}

{{- define "tsg.recurseSecretConfig" -}}
{{- $map := first . -}}
{{- $label := last . -}}
{{- range $key, $val := $map -}}
  {{- $sublabel := snakecase $key | upper -}}
  {{- if not (empty $label) -}}
    {{- $sublabel = printf "TSG__%s__%s" $label $sublabel -}}
  {{- end -}}
  {{- if kindOf $val | eq "map" -}}
    {{- if and (hasKey $val "name") (hasKey $val "key")}}
- name: {{ $sublabel | quote }}
  valueFrom:
    secretKeyRef:
      name: {{ $val.name }}
      key: {{ $val.key }}
    {{- else }}
    {{- list $val $sublabel | include "tsg.recurseSecretConfig" -}}
    {{- end }}
  {{- else if kindOf $val | eq "slice" -}}
    {{- range $idx, $elem := $val }}
      {{- list $elem (printf "%s__%d" $sublabel $idx) | include "tsg.recurseSecretConfig" -}}
    {{- end }}
  {{- else -}}
- name: {{ $sublabel | quote }}
  value: {{ $val | quote }}
{{ end -}}
{{- end -}}
{{- end -}}