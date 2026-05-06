{{/*
Resolve the issuer DID from elsi.did / keycloak.issuerDid / "${DID}" fallback.
Used by both the realm-level `issuerDid` attribute and the per-VC `vc.issuer_did`
ClientScope attribute (KC 26.4+ — drives the JWT `iss` claim of issued credentials).
*/}}
{{- define "dsc.issuerDid" -}}
{{- if eq .Values.elsi.enabled true -}}
{{- .Values.elsi.did -}}
{{- else if .Values.keycloak.issuerDid -}}
{{- .Values.keycloak.issuerDid -}}
{{- else -}}
${DID}
{{- end -}}
{{- end -}}

{{/*
Build realm attributes object. Resolves issuerDid from elsi/keycloak.issuerDid/fallback,
merges extra attributes (string or map), and appends verifiable credential attributes.
*/}}
{{- define "dsc.realmAttributes" -}}
{{- $issuerDid := include "dsc.issuerDid" . -}}
{{- $attrs := dict "frontendUrl" (.Values.keycloak.realm.frontendUrl | default "") "issuerDid" $issuerDid -}}
{{- if .Values.keycloak.realm.attributes -}}
{{- $extra := .Values.keycloak.realm.attributes -}}
{{- if kindIs "string" .Values.keycloak.realm.attributes -}}
{{- $extra = printf "{%s}" (.Values.keycloak.realm.attributes | trim) | fromJson -}}
{{- end -}}
{{- $attrs = mergeOverwrite $attrs $extra -}}
{{- end -}}
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
Build a list of client scopes derived from verifiableCredentials.
Each VC entry generates one client scope with protocol "oid4vc" and the VC's
attributes attached to the scope itself (the VC name is the scope name).
Attribute keys are auto-prefixed with `vc.` if the user did not write them
prefixed already. Map / slice attribute values are JSON-encoded.

Defaults applied when the user does not set them explicitly:
  - `vc.issuer_did` is filled from `dsc.issuerDid` (elsi.did / keycloak.issuerDid / "${DID}").
  - `vc.supported_credential_types` is filled with the value of `vc.verifiable_credential_type`
    when only the latter is provided. The two attributes serve different
    purposes in KC 26.4+ (one drives the SD-JWT `vct` claim and the metadata
    `vct` field; the other drives the JWT_VC_JSON `type` array and the
    metadata `credential_definition.type`), but in the typical DSC setup they
    carry the same VCT name, so deriving the second from the first avoids
    repetition. To use different values, set both keys explicitly under
    `verifiableCredentials.<key>.attributes`.

ref: https://github.com/keycloak/keycloak/blob/26.6.1/server-spi-private/src/main/java/org/keycloak/models/oid4vci/CredentialScopeModel.java
Skipped when clientScope.create is explicitly set to false.
*/}}
{{- define "dsc.vcClientScopes" -}}
{{- $scopes := list -}}
{{- $defaultIssuerDid := include "dsc.issuerDid" . -}}
{{- range $vcKey, $vcVal := .Values.keycloak.realm.verifiableCredentials | default dict -}}
{{- $create := dig "clientScope" "create" true $vcVal -}}
{{- if $create -}}
{{- $attrs := dict "include.in.token.scope" "true" "display.on.consent.screen" "false" "vc.issuer_did" $defaultIssuerDid -}}
{{- range $attrKey, $attrVal := $vcVal.attributes | default dict -}}
{{- $val := $attrVal -}}
{{- if or (kindIs "map" $attrVal) (kindIs "slice" $attrVal) -}}
{{- $val = $attrVal | toJson -}}
{{- end -}}
{{- $key := $attrKey -}}
{{- if not (hasPrefix "vc." $attrKey) -}}
{{- $key = printf "vc.%s" $attrKey -}}
{{- end -}}
{{- $_ := set $attrs $key (printf "%v" $val) -}}
{{- end -}}
{{/* Derive vc.supported_credential_types from vc.verifiable_credential_type when not set. See header comment above. */}}
{{- if and (not (hasKey $attrs "vc.supported_credential_types")) (hasKey $attrs "vc.verifiable_credential_type") -}}
{{- $_ := set $attrs "vc.supported_credential_types" (index $attrs "vc.verifiable_credential_type") -}}
{{- end -}}
{{- $scopes = append $scopes (dict
    "name" $vcKey
    "protocol" "oid4vc"
    "description" (printf "Client scope for issuing the %s verifiable credential" $vcKey)
    "attributes" $attrs
    "protocolMappers" ($vcVal.protocolMappers | default list)
) -}}
{{- end -}}
{{- end -}}
{{- dict "list" $scopes | toPrettyJson -}}
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
