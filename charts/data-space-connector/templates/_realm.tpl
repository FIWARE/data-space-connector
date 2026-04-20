{{/*
Build realm attributes object. Resolves issuerDid from elsi/keycloak.issuerDid/fallback,
merges extra attributes (string or map), and appends verifiable credential attributes.
*/}}
{{- define "dsc.realmAttributes" -}}
{{- $issuerDid := "${DID}" -}}
{{- if eq .Values.elsi.enabled true -}}
{{- $issuerDid = .Values.elsi.did -}}
{{- else if .Values.keycloak.issuerDid -}}
{{- $issuerDid = .Values.keycloak.issuerDid -}}
{{- end -}}
{{- $attrs := dict "frontendUrl" (.Values.keycloak.realm.frontendUrl | default "") "issuerDid" $issuerDid -}}
{{- if .Values.keycloak.realm.attributes -}}
{{- $extra := .Values.keycloak.realm.attributes -}}
{{- if kindIs "string" .Values.keycloak.realm.attributes -}}
{{- $extra = printf "{%s}" (.Values.keycloak.realm.attributes | trim) | fromJson -}}
{{- end -}}
{{- $attrs = mergeOverwrite $attrs $extra -}}
{{- end -}}
{{- $vcAttrs := include "dsc.vcAttributes" . | fromJson -}}
{{- $attrs = mergeOverwrite $attrs $vcAttrs -}}
{{- $attrs | toPrettyJson -}}
{{- end -}}

{{/*
Merge defaultRealmRoles and extraRealmRoles into a single list
*/}}
{{- define "dsc.realmRoles" -}}
{{- $default := .Values.keycloak.realm.defaultRealmRoles | default list -}}
{{- $extra := .Values.keycloak.realm.extraRealmRoles | default list -}}
{{- concat $default $extra | toPrettyJson -}}
{{- end -}}

{{/*
Merge defaultGroups and extraGroups into a single list
*/}}
{{- define "dsc.groups" -}}
{{- $default := .Values.keycloak.realm.defaultGroups | default list -}}
{{- $extra := .Values.keycloak.realm.extraGroups | default list -}}
{{- concat $default $extra | toPrettyJson -}}
{{- end -}}

{{/*
Merge defaultDefaultClientScopes and extraDefaultClientScopes, deduplicating.
*/}}
{{- define "dsc.defaultDefaultClientScopes" -}}
{{- concat (.Values.keycloak.realm.defaultDefaultClientScopes | default list) (.Values.keycloak.realm.extraDefaultClientScopes | default list) | uniq | toPrettyJson -}}
{{- end -}}

{{/*
Merge defaultOptionalClientScopes and extraOptionalClientScopes, deduplicating.
*/}}
{{- define "dsc.defaultOptionalClientScopes" -}}
{{- concat (.Values.keycloak.realm.defaultOptionalClientScopes | default list) (.Values.keycloak.realm.extraOptionalClientScopes | default list) | uniq | toPrettyJson -}}
{{- end -}}

{{/*
Merge defaultClientScopes and clientScopes into a single JSON array.
clientScopes supports string (raw JSON elements) or list.
*/}}
{{- define "dsc.clientScopes" -}}
{{- $default := .Values.keycloak.realm.defaultClientScopes | default list -}}
{{- $extra := .Values.keycloak.realm.clientScopes | default list -}}
{{- if kindIs "string" .Values.keycloak.realm.clientScopes -}}
{{- $extra = (printf "{\"list\":[%s]}" (.Values.keycloak.realm.clientScopes | trim) | fromJson).list -}}
{{- end -}}
{{- $vcScopes := (include "dsc.vcClientScopes" . | fromJson).list -}}
{{- concat $default $extra $vcScopes | toPrettyJson -}}
{{- end -}}

{{/*
Render clientRoles as a JSON object. Supports string (raw JSON fields) or map.
Merges defaultClientRoles and clientRoles using mergeOverwrite (clientRoles wins per key).
*/}}
{{- define "dsc.clientRoles" -}}
{{- $default := .Values.keycloak.realm.defaultClientRoles | default dict -}}
{{- $extra := .Values.keycloak.realm.clientRoles | default dict -}}
{{- if kindIs "string" .Values.keycloak.realm.clientRoles -}}
{{- $extra = printf "{%s}" (.Values.keycloak.realm.clientRoles | trim) | fromJson -}}
{{- end -}}
{{- mergeOverwrite $default $extra | toPrettyJson -}}
{{- end -}}

{{/*
Render users as a JSON array. Supports string (raw JSON elements) or list.
*/}}
{{- define "dsc.users" -}}
{{- $users := .Values.keycloak.realm.users | default list -}}
{{- if kindIs "string" .Values.keycloak.realm.users -}}
{{- $users = (printf "{\"list\":[%s]}" (.Values.keycloak.realm.users | trim) | fromJson).list -}}
{{- end -}}
{{- $users | toPrettyJson -}}
{{- end -}}

{{/*
Transform defaultClients (map keyed by clientId) into a list, injecting clientId from the key.
Also extends each client's optionalClientScopes with all clientScope names.
*/}}
{{- define "dsc.defaultClients" -}}
{{- $defaultScopes := .Values.keycloak.realm.defaultClientScopes | default list -}}
{{- $extraScopes := .Values.keycloak.realm.clientScopes | default list -}}
{{- if kindIs "string" .Values.keycloak.realm.clientScopes -}}
{{- $extraScopes = (printf "{\"list\":[%s]}" (.Values.keycloak.realm.clientScopes | trim) | fromJson).list -}}
{{- end -}}
{{- $vcScopes := (include "dsc.vcClientScopes" . | fromJson).list -}}
{{- $scopeNames := list -}}
{{- range (concat $defaultScopes $extraScopes $vcScopes) -}}
{{- $scopeNames = append $scopeNames (index . "name") -}}
{{- end -}}
{{- $clients := list -}}
{{- range $key, $val := .Values.keycloak.realm.defaultClients | default dict -}}
{{- $clientDefaultScopes := $val.defaultClientScopes | default list -}}
{{- $optScopes := $val.optionalClientScopes | default list -}}
{{- range $scopeNames -}}
{{- if not (has . $clientDefaultScopes) -}}
{{- $optScopes = append $optScopes . -}}
{{- end -}}
{{- end -}}
{{- $optScopes = $optScopes | uniq -}}
{{- $clients = append $clients (merge (dict "clientId" $key "optionalClientScopes" $optScopes) $val) -}}
{{- end -}}
{{- dict "list" $clients | toPrettyJson -}}
{{- end -}}

{{/*
Render clients as a JSON array. Supports string (raw JSON elements) or list.
Merges defaultClients and clients. Extra clients override defaults with the same clientId.
*/}}
{{- define "dsc.clients" -}}
{{- $default := (include "dsc.defaultClients" . | fromJson).list -}}
{{- $extra := .Values.keycloak.realm.clients | default list -}}
{{- if kindIs "string" .Values.keycloak.realm.clients -}}
{{- $extra = (printf "{\"list\":[%s]}" (.Values.keycloak.realm.clients | trim) | fromJson).list -}}
{{- end -}}
{{- $extraIds := list -}}
{{- range $extra -}}
{{- $extraIds = append $extraIds .clientId -}}
{{- end -}}
{{- $merged := list -}}
{{- range $default -}}
{{- if not (has .clientId $extraIds) -}}
{{- $merged = append $merged . -}}
{{- end -}}
{{- end -}}
{{- concat $merged $extra | toPrettyJson -}}
{{- end -}}

{{/*
Build a flat object of verifiable credential attributes from verifiableCredentials.
Each entry is flattened as "vc.{key}.{attr}" = value.
If the value is a map or list, it is serialized as a JSON string.
*/}}
{{- define "dsc.vcAttributes" -}}
{{- $result := dict -}}
{{- range $vcKey, $vcVal := .Values.keycloak.realm.verifiableCredentials | default dict -}}
{{- range $attrKey, $attrVal := $vcVal.attributes | default dict -}}
{{- $val := $attrVal -}}
{{- if or (kindIs "map" $attrVal) (kindIs "slice" $attrVal) -}}
{{- $val = $attrVal | toJson -}}
{{- end -}}
{{- $_ := set $result (printf "vc.%s.%s" $vcKey $attrKey) $val -}}
{{- end -}}
{{- end -}}
{{- $result | toPrettyJson -}}
{{- end -}}

{{/*
Build a list of client scopes derived from verifiableCredentials.
Each VC with a defined attributes.scope generates one client scope entry:
  - name            → attributes.scope
  - protocol        → openid-connect
  - description     → "Client scope for the {vcKey} verifiable credential"
  - protocolMappers → protocolMappers list (or empty list if not defined)
Skipped when clientScope.create is explicitly set to false.
*/}}
{{- define "dsc.vcClientScopes" -}}
{{- $scopes := list -}}
{{- range $vcKey, $vcVal := .Values.keycloak.realm.verifiableCredentials | default dict -}}
{{- $scope := ($vcVal.attributes | default dict).scope -}}
{{- $create := dig "clientScope" "create" true $vcVal -}}
{{- if and $scope $create -}}
{{- $scopes = append $scopes (dict
    "name" $scope
    "protocol" "openid-connect"
    "description" (printf "Client scope for the %s verifiable credential" $vcKey)
    "protocolMappers" ($vcVal.protocolMappers | default list)
) -}}
{{- end -}}
{{- end -}}
{{- dict "list" $scopes | toPrettyJson -}}
{{- end -}}

{{/*
Transform credentialBuilder (map keyed by builder name) into a list for Keycloak components.
Each entry gets id and name from the key, providerId from .name.
*/}}
{{- define "dsc.credentialBuilders" -}}
{{- $builders := list -}}
{{- range $key, $val := .Values.keycloak.realm.credentialBuilder | default dict -}}
{{- $builders = append $builders (dict "id" $key "name" $key "providerId" $val.name) -}}
{{- end -}}
{{- $builders | toPrettyJson -}}
{{- end -}}

{{/*
Render the complete Keycloak realm as a JSON string.
Values that contain Helm template expressions (e.g. {{ .Values.keycloak.realm.name }})
are resolved by the caller via tpl.
*/}}
{{- define "dsc.realmJson" -}}
{
  "id": "{{ .Values.keycloak.realm.name }}",
  "realm": "{{ .Values.keycloak.realm.name }}",
  "displayName": "{{ .Values.keycloak.realm.name }}",
  "displayNameHtml": "{{ .Values.keycloak.realm.displayNameHtml | default (printf "<div class=\\\"kc-logo-text\\\"><span>%s</span></div>" .Values.keycloak.realm.name) }}",
  "verifiableCredentialsEnabled": true,
  "enabled": true,
  "attributes": {{ include "dsc.realmAttributes" . | indent 4 | trim }},
  "sslRequired": {{ .Values.keycloak.realm.sslRequired | default "none" | quote }},
  "roles": {
    "realm": {{ include "dsc.realmRoles" . | indent 6 | trim }},
    "client": {{ include "dsc.clientRoles" . | indent 6 | trim }}
  },
  "groups": {{ include "dsc.groups" . | indent 4 | trim }},
  "users": {{ include "dsc.users" . | indent 4 | trim }},
  "clients": {{ include "dsc.clients" . | indent 4 | trim }},
  "clientScopes": {{ include "dsc.clientScopes" . | indent 4 | trim }},
  "defaultDefaultClientScopes": {{ include "dsc.defaultDefaultClientScopes" . | indent 4 | trim }},
  "defaultOptionalClientScopes": {{ include "dsc.defaultOptionalClientScopes" . | indent 4 | trim }},
  "components": {
    "org.keycloak.protocol.oid4vc.issuance.credentialbuilder.CredentialBuilder": {{ include "dsc.credentialBuilders" . | indent 6 | trim }},
    "org.keycloak.keys.KeyProvider": {{ include "dsc.keyProviders" . | indent 6 | trim }}
  }
}
{{- end -}}

{{/*
Build the KeyProvider list from defaultKeyProviders + extraKeyProviders
and appends the signing key (elsi / signingKey / test-key fallback).
*/}}
{{- define "dsc.keyProviders" -}}
{{- $providers := concat (.Values.keycloak.realm.defaultKeyProviders | default list) (.Values.keycloak.realm.extraKeyProviders | default list) -}}
{{- $signingKey := dict -}}
{{- if eq .Values.elsi.enabled true -}}
{{- $signingKey = dict
    "id" "a4589e8f-7f82-4345-b2ea-ccc9d4366600"
    "name" .Values.elsi.keyAlias
    "providerId" "java-keystore"
    "subComponents" (dict)
    "config" (dict
        "keystore" (list .Values.elsi.storePath)
        "keystorePassword" (list .Values.elsi.storePassword)
        "keyAlias" (list .Values.elsi.keyAlias)
        "keyPassword" (list .Values.elsi.keyPassword)
        "kid" (list .Values.elsi.did)
        "active" (list "true")
        "priority" (list "0")
        "enabled" (list "true")
        "algorithm" (list .Values.elsi.keyAlgorithm)
    )
-}}
{{- else if .Values.keycloak.signingKey -}}
{{- $signingKey = dict
    "id" "a4589e8f-7f82-4345-b2ea-ccc9d4366600"
    "name" "signing-key"
    "providerId" "java-keystore"
    "subComponents" (dict)
    "config" (dict
        "keystore" (list .Values.keycloak.signingKey.storePath)
        "keystorePassword" (list .Values.keycloak.signingKey.storePassword)
        "keyAlias" (list .Values.keycloak.signingKey.keyAlias)
        "keyPassword" (list .Values.keycloak.signingKey.keyPassword)
        "kid" (list (.Values.keycloak.signingKey.did | default "${DID}"))
        "active" (list "true")
        "priority" (list "0")
        "enabled" (list "true")
        "algorithm" (list .Values.keycloak.signingKey.keyAlgorithm)
    )
-}}
{{- else -}}
{{- $signingKey = dict
    "id" "a4589e8f-7f82-4345-b2ea-ccc9d4366600"
    "name" "test-key"
    "providerId" "java-keystore"
    "subComponents" (dict)
    "config" (dict
        "keystore" (list "/did-material/cert.pfx")
        "keystorePassword" (list "${STORE_PASS}")
        "keyAlias" (list "didPrivateKey")
        "keyPassword" (list "${STORE_PASS}")
        "kid" (list "${DID}")
        "active" (list "true")
        "priority" (list "0")
        "enabled" (list "true")
        "algorithm" (list "ES256")
    )
-}}
{{- end -}}
{{- append $providers $signingKey | toPrettyJson -}}
{{- end -}}
