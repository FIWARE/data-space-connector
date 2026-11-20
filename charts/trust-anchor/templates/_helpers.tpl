{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "trust-anchor.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "trust-anchor.fullname" -}}
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
{{- define "trust-anchor.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "trust-anchor.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "trust-anchor.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "trust-anchor.labels" -}}
app.kubernetes.io/name: {{ include "trust-anchor.name" . }}
helm.sh/chart: {{ include "trust-anchor.chart" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Fixed name of the consent-manager setup Job and its RBAC objects. The Job's name gets the release
revision and a pod-spec hash appended, see templates/consent-manager-setup-job.yaml.
*/}}
{{- define "trust-anchor.consentManagerSetup.name" -}}
consent-manager-setup
{{- end -}}

{{/*
Name of the ServiceAccount the setup Job runs under: `consentManagement.setup.serviceAccount.name`
when set, else the fixed Job name. Resolved in one place, because the Job and its RBAC objects
(templates/consent-manager-setup-rbac.yaml) have to agree on it even when the chart creates neither.
*/}}
{{- define "trust-anchor.consentManagerSetup.serviceAccountName" -}}
{{- $serviceAccount := .Values.consentManagement.setup.serviceAccount | default dict -}}
{{- $serviceAccount.name | default (include "trust-anchor.consentManagerSetup.name" .) -}}
{{- end -}}

{{/*
Name of the Secret holding the consent-manager's environment (DB and SMTP credentials, the log
pseudonym and erasure keys). Written by the setup Job and read by the subchart, so both the Job's
script and its Role resolve it here. Nil-safe: the subchart values are not always coalesced into
the context a template is rendered with.
*/}}
{{- define "trust-anchor.consentManager.envSecretName" -}}
{{- $manager := (index .Values "consent-manager") | default dict -}}
{{- ($manager.secrets | default dict).existingSecret | default "consent-manager-env" -}}
{{- end -}}

{{/*
Name of the Secret holding the consent-manager's receipt signing key. Same contract as
`trust-anchor.consentManager.envSecretName`.
*/}}
{{- define "trust-anchor.consentManager.signingKeysSecretName" -}}
{{- $manager := (index .Values "consent-manager") | default dict -}}
{{- ($manager.signingKeys | default dict).secretName | default "consent-manager-signing-keys" -}}
{{- end -}}
