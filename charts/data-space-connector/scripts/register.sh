#!/bin/sh
# Provision vault and the participant context. Idempotent: safe to re-run on every upgrade.
set -e
# overridable so the scripts can be exercised outside the Job
SCRIPT_DIR="${SCRIPT_DIR:-/scripts}"
. "${SCRIPT_DIR}/lib.sh"
SHARED_DIR="${SHARED_DIR:-/shared}"

: "${DID:?DID is required}"
: "${KEY_ALIAS:?KEY_ALIAS is required}"
: "${VAULT_ADDR:?VAULT_ADDR is required}"
: "${VAULT_TOKEN:?VAULT_TOKEN is required}"
: "${IDENTITY_API:?IDENTITY_API is required}"
: "${API_KEY:?API_KEY is required}"
: "${READINESS_URL:?READINESS_URL is required}"

wait_for "identityhub" "${READINESS_URL}"
wait_for "vault" "${VAULT_ADDR}/v1/sys/health"

PRIVATE_JWK=$(cat "${SHARED_DIR}/private.jwk")
PUBLIC_JWK=$(cat "${SHARED_DIR}/public.jwk")

echo "Storing private key in vault under alias ${KEY_ALIAS}..."
vault_put "${KEY_ALIAS}" "${PRIVATE_JWK}"

# SuperUserSeedExtension seeds the super-user's alias with the bare API key, which has no
# base64(id). prefix, so no presentable token can ever match it and the account is locked out.
# Store the composed token instead. Repeated on every run because the seed extension only
# applies its value once, at first creation.
#
# The alias is derived by the identityhub as <participantContextId>-apikey - NOT the value of
# identityhub.endpoints.identity.authKeyAlias, which drives a different authenticator and is
# not consulted here.
echo "Repairing super-user credential in vault..."
vault_put "${SUPERUSER_ID}-apikey" "$(superuser_token)"

BASE64_DID=$(b64url "${DID}")
CS_ENDPOINT="${CREDENTIAL_SERVICE_URL}/api/credentials/v1/participants/${BASE64_DID}"

PAYLOAD=$(printf '{"role":["admin"],"active":true,"participantId":"%s","did":"%s","serviceEndpoints":[{"type":"CredentialService","serviceEndpoint":"%s","id":"credential-service"}],"key":{"keyId":"%s","privateKeyAlias":"%s","publicKeyJwk":%s}}' \
  "${DID}" "${DID}" "${CS_ENDPOINT}" "${KEY_ID}" "${KEY_ALIAS}" "${PUBLIC_JWK}")

echo "Registering participant ${DID}..."
http_code=$(curl -s -o /tmp/resp.json -w "%{http_code}" -X POST "${IDENTITY_API}/participants" \
  -H "x-api-key: $(superuser_token)" \
  -H "Content-Type: application/json" \
  -d "${PAYLOAD}")
echo "POST ${IDENTITY_API}/participants -> HTTP ${http_code}"
cat /tmp/resp.json
echo
if [ "${http_code}" = "409" ]; then
  echo "Participant already exists."
elif [ "${http_code}" -lt 200 ] || [ "${http_code}" -ge 300 ]; then
  exit 1
fi

# --- STS client secret ------------------------------------------------------------------
# Provisioned unconditionally, and the clientSecret the identityhub returns on creation is
# deliberately ignored:
#
#  * The identityhub reveals that generated secret only once, in the creation response. Any
#    vault restart in dev mode loses it, and on the next run the API answers 409 and the value
#    is gone for good - which is exactly how the DCP lane breaks with "Failed to fetch client
#    secret from the vault with alias: ...".
#  * Neither side ever stores the secret itself: edc_sts_client only keeps a secret_alias, and
#    both the EDC oauth client and the STS resolve that alias from vault and compare. So the
#    value is ours to choose, and a fixed one makes this restart-proof.
#
# It is written under TWO key names on purpose. EDC's hashicorp vault client URL-encodes the
# alias and the HTTP layer then encodes the '%', so a lookup for "<did>-sts-client-secret"
# actually resolves the key literally named with every ':' of the DID replaced by "%3A". The
# plain-colon name is what curl writes here, so storing both keeps whichever form the client
# uses working.
if [ -z "${STS_CLIENT_SECRET}" ]; then
  echo "STS_CLIENT_SECRET is empty - refusing to provision an empty secret"
  exit 1
fi
DID_PCT=$(printf '%s' "${DID}" | sed 's/:/%253A/g')
for alias_path in "${DID}-sts-client-secret" "${DID_PCT}-sts-client-secret"; do
  echo "Writing STS client secret to vault alias ${alias_path}..."
  vault_put "${alias_path}" "${STS_CLIENT_SECRET}"
done
