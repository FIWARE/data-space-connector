{{/* vim: set filetype=mustache: */}}
{{/*
Keycloak token-status-list plugin helpers.
These templates generate the init container, volume, and volume mount specs
needed to load the keycloak-token-status-plugin JAR into Keycloak at startup.

IMPORTANT: These helpers are designed to be evaluated inside the Keycloak
subchart context (via common.tplvalues.render / tpl). In that context,
.Values is scoped to the keycloak subchart, so parent values under
"keycloak.tokenStatusList" become ".Values.tokenStatusList" here.

See https://github.com/ADORSYS-GIS/token-status-link for plugin documentation.
*/}}

{{/*
Construct the Maven Central download URL for the token-status-list plugin JAR.

Usage (within keycloak subchart context):
  {{ include "dsc.keycloak.tokenStatusList.mavenUrl" . }}

Example output:
  https://repo1.maven.org/maven2/io/github/adorsys-gis/keycloak-token-status-plugin/0.2.0/keycloak-token-status-plugin-0.2.0.jar
*/}}
{{- define "dsc.keycloak.tokenStatusList.mavenUrl" -}}
{{- $groupPath := .Values.tokenStatusList.plugin.groupId | replace "." "/" -}}
{{- $artifactId := .Values.tokenStatusList.plugin.artifactId -}}
{{- $version := .Values.tokenStatusList.plugin.version -}}
{{- $baseUrl := .Values.tokenStatusList.plugin.mavenBaseUrl -}}
{{- printf "%s/%s/%s/%s/%s-%s.jar" $baseUrl $groupPath $artifactId $version $artifactId $version -}}
{{- end -}}

{{/*
Generate the init container spec for downloading the token-status-list plugin JAR.
Uses curlimages/curl to fetch the JAR from Maven Central and place it into the
shared providers volume at /providers/.

The init container mounts the "providers" emptyDir volume (see
dsc.keycloak.tokenStatusList.extraVolume) to /providers.

Usage in keycloak values (evaluated by common.tplvalues.render):
  keycloak:
    initContainers: |
      {{- include "dsc.keycloak.tokenStatusList.initContainer" . | nindent 6 }}
*/}}
{{- define "dsc.keycloak.tokenStatusList.initContainer" -}}
- name: download-token-status-plugin
  image: {{ .Values.tokenStatusList.initContainer.image }}
  imagePullPolicy: IfNotPresent
  command:
    - /bin/sh
  args:
    - -ec
    - |
      PLUGIN_URL="{{ include "dsc.keycloak.tokenStatusList.mavenUrl" . }}"
      echo "Downloading token-status-list plugin from ${PLUGIN_URL}"
      curl -sSL -o /providers/{{ .Values.tokenStatusList.plugin.artifactId }}-{{ .Values.tokenStatusList.plugin.version }}.jar "${PLUGIN_URL}"
      echo "Plugin downloaded successfully"
  volumeMounts:
    - name: providers
      mountPath: /providers
{{- end -}}

{{/*
Generate the extra volume spec for the Keycloak providers directory.
Creates an emptyDir volume named "providers" that is shared between the
init container (which downloads the JAR) and the main Keycloak container.

Usage in keycloak values (evaluated by common.tplvalues.render):
  keycloak:
    extraVolumes: |
      - name: realms
        configMap:
          name: {{ include "dsc.fullname" . }}-realm
      {{- if .Values.tokenStatusList.enabled }}
      {{- include "dsc.keycloak.tokenStatusList.extraVolume" . | nindent 6 }}
      {{- end }}
*/}}
{{- define "dsc.keycloak.tokenStatusList.extraVolume" -}}
- name: providers
  emptyDir: {}
{{- end -}}

{{/*
Generate the extra volume mount spec for mounting the providers directory
into the Keycloak container at /opt/keycloak/providers.

Usage in keycloak values (evaluated by common.tplvalues.render):
  keycloak:
    extraVolumeMounts: |
      - name: realms
        mountPath: /opt/bitnami/keycloak/data/import
      {{- if .Values.tokenStatusList.enabled }}
      {{- include "dsc.keycloak.tokenStatusList.extraVolumeMount" . | nindent 6 }}
      {{- end }}
*/}}
{{- define "dsc.keycloak.tokenStatusList.extraVolumeMount" -}}
- name: providers
  mountPath: /opt/keycloak/providers
{{- end -}}
