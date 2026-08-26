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
# The same alias spelled the way EDC's vault client leaves it. Its client URL-encodes the alias and
# the HTTP layer then encodes the '%' again, so a lookup for "<did>-<suffix>" resolves a key
# literally named with every ':' of the DID replaced by "%3A". Used by both blocks below.
DID_PCT=$(printf '%s' "${DID}" | sed 's/:/%253A/g')
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

# --- participant api token ----------------------------------------------------------------------
# The identityhub generates this one itself, stores it ONLY in vault, and keeps just the alias in its
# database. Nothing re-provisions it: on a re-run the participant already exists, the call above
# answers 409, and a vault that lost its contents is left with a row pointing at an alias that
# resolves to nothing.
#
# That is not a clean failure. ParticipantServicePrincipalResolver resolves the alias and then
# compares stored.equals(presented), so an absent secret is a NullPointerException - a 500 where a
# 401 belongs.
#
# POST .../token is the identityhub's own repair: it writes a fresh token under the SAME alias and
# touches nothing else. Done only when neither spelling resolves, so a healthy deployment is left
# alone and a token somebody is already using is never rotated behind their back.
#
# A failure here warns rather than aborts, unlike every other step. This repairs something none of
# the chart's own flows use - they all authenticate as the super-user - and an older identityhub
# without the endpoint would answer 404 and take every bootstrap down with it.
if vault_has "${DID}-apikey" || vault_has "${DID_PCT}-apikey"; then
  echo "participant api token already resolves"
else
  echo "participant api token missing from vault - regenerating it..."
  # The body is the token itself, so it is never echoed on success.
  http_code=$(curl -s -o /tmp/apitoken.json -w "%{http_code}" -X POST \
    "${IDENTITY_API}/participants/${BASE64_DID}/token" \
    -H "x-api-key: $(superuser_token)")
  if [ "${http_code}" -ge 200 ] && [ "${http_code}" -lt 300 ]; then
    echo "participant api token regenerated under the existing alias"
  else
    echo "WARNING: could not regenerate the participant api token: HTTP ${http_code}"
    echo "WARNING: authenticating as ${DID} against the identity API will fail until it is repaired"
  fi
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
# It is written under both spellings of the alias (see DID_PCT above), so whichever form the client
# uses resolves.
#
# Only the participant's. The super-user context has an edc_sts_client row of its own, pointing at
# "<superUserId>-sts-client-secret", and that one is deliberately left unresolvable: nothing ever
# authenticates against the STS as the super-user - the connector does so as the participant DID -
# and provisioning it would turn an account that cannot be used into one that can. A reference that
# resolves to nothing is the safer state here.
if [ -z "${STS_CLIENT_SECRET}" ]; then
  echo "STS_CLIENT_SECRET is empty - refusing to provision an empty secret"
  exit 1
fi
for alias_path in "${DID}-sts-client-secret" "${DID_PCT}-sts-client-secret"; do
  echo "Writing STS client secret to vault alias ${alias_path}..."
  vault_put "${alias_path}" "${STS_CLIENT_SECRET}"
done
