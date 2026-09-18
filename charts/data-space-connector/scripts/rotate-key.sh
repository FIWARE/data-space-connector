#!/bin/sh
# Replace the participant's signing key, keeping the same alias and the same DID fragment.
#
# Run explicitly, never on every upgrade: there is no overlap window. The moment the key is
# replaced, everything signed with the old one under this fragment stops verifying - including
# credentials already held in wallets - so re-issuance is part of the operation.
#
# The order below is the only safe one, verified against tractusx/identityhub:
#
#   * `rotate` does NOT replace in place. Called with the same keyId it APPENDS, leaving the DID
#     document with two verificationMethod entries sharing one id, and revoking either one then
#     removes BOTH (removal is by keyId, not by resource), leaving no key at all.
#   * revoke-then-add ends with exactly one entry carrying the new key. The gap between the two
#     calls is a window in which the participant publishes no key, so keep it short.
set -e
# overridable so the scripts can be exercised outside the Job
SCRIPT_DIR="${SCRIPT_DIR:-/scripts}"
. "${SCRIPT_DIR}/lib.sh"
SHARED_DIR="${SHARED_DIR:-/shared}"

: "${DID:?DID is required}"
: "${KEY_ALIAS:?KEY_ALIAS is required}"
: "${KEY_ID:?KEY_ID is required}"
: "${IDENTITY_API:?IDENTITY_API is required}"
: "${VAULT_ADDR:?VAULT_ADDR is required}"
: "${READINESS_URL:?READINESS_URL is required}"

wait_for "identityhub" "${READINESS_URL}"
wait_for "vault" "${VAULT_ADDR}/v1/sys/health"

BASE64_DID=$(b64url "${DID}")
KEYPAIRS_API="${IDENTITY_API}/participants/${BASE64_DID}/keypairs"
PUBLIC_JWK=$(cat "${SHARED_DIR}/public.jwk")
PRIVATE_JWK=$(cat "${SHARED_DIR}/private.jwk")

echo "Looking up the active keypair for ${KEY_ID}..."
curl -fsS -X GET "${KEYPAIRS_API}" -H "x-api-key: $(superuser_token)" > /tmp/keypairs.json

# Every resource carrying this keyId has to go: they all render the same verificationMethod id.
# Split the array into one record per line on `},{` - the embedded serializedPublicKey string
# contains braces, so splitting on a bare `{` would cut records in half. `"id":"` matches only
# the id field; participantContextId and keyId both end in a capital `Id`.
RESOURCE_IDS=$(sed 's/},{/}\n{/g' /tmp/keypairs.json | awk -v kid="${KEY_ID}" '
  index($0, "\"keyId\":\"" kid "\"") > 0 && match($0, /"id":"[^"]*"/) {
    print substr($0, RSTART + 6, RLENGTH - 7)
  }')
if [ -z "${RESOURCE_IDS}" ]; then
  echo "no keypair resource found for ${KEY_ID} - is the participant registered?"
  exit 1
fi

for rid in ${RESOURCE_IDS}; do
  echo "Revoking keypair resource ${rid}..."
  http_code=$(curl -s -o /tmp/revoke.json -w "%{http_code}" -X POST "${KEYPAIRS_API}/${rid}/revoke" \
    -H "x-api-key: $(superuser_token)" -H "Content-Type: application/json")
  echo "POST ${KEYPAIRS_API}/${rid}/revoke -> HTTP ${http_code}"
  if [ "${http_code}" -lt 200 ] || [ "${http_code}" -ge 300 ]; then
    cat /tmp/revoke.json
    exit 1
  fi
done

echo "Overwriting vault alias ${KEY_ALIAS} with the new key..."
vault_put "${KEY_ALIAS}" "${PRIVATE_JWK}"

echo "Publishing the new key under ${KEY_ID}..."
DESCRIPTOR=$(printf '{"keyId":"%s","privateKeyAlias":"%s","publicKeyJwk":%s}' \
  "${KEY_ID}" "${KEY_ALIAS}" "${PUBLIC_JWK}")
http_code=$(printf '%s' "${DESCRIPTOR}" | curl -s -o /tmp/add.json -w "%{http_code}" -X PUT "${KEYPAIRS_API}?makeDefault=true" \
  -H "x-api-key: $(superuser_token)" \
  -H "Content-Type: application/json" \
  --data-binary @-)
echo "PUT ${KEYPAIRS_API} -> HTTP ${http_code}"
if [ "${http_code}" -lt 200 ] || [ "${http_code}" -ge 300 ]; then
  cat /tmp/add.json
  exit 1
fi

echo "key ${KEY_ID} rotated"
echo "REMINDER: re-issue every credential signed with the previous key, restart the workloads"
echo "that read the identity key at startup, and restart apisix so it refetches the JWKS."
