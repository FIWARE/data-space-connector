#!/usr/bin/env bash
#
# verify_consent_flow.sh -- automated end-to-end check of the consent-gated data
# access flow (roadmap item 8). It drives the exact demo path and asserts the
# HTTP outcomes:
#   grant consent   -> the gated read returns 200
#   revoke consent  -> the gated read returns 403
#   re-grant consent-> the gated read returns 200
# It exits non-zero on the first failed assertion, so it can gate CI or serve as
# a manual smoke test.
#
# Grant/revoke run through the consent-manager's REAL give-consent API (no direct
# database writes), the same flow the demo in doc/CONSENT_MANAGEMENT.md documents:
# register the participants + subject, seed a TM Forum agreement (which the
# consent-facade projects into a privacy notice), then POST /v1/consents with
# event "given" / "revoked". The consent-manager and the provider's TM Forum API
# are reached via `kubectl port-forward`.
#
# Prerequisites (same as the demo in doc/CONSENT_MANAGEMENT.md):
#   * a running consent-enabled cluster (e.g. `mvn ... -Pconsent`) with KUBECONFIG
#     pointing at it, and the squid proxy reachable on :8888,
#   * the consumer holder identity under cert/ (created by the did-helper step:
#     `mkdir -p cert && docker run -v $(pwd)/cert:/cert quay.io/wi_stefan/did-helper:0.1.1`),
#     which get_access_token_oid4vp.sh reads,
#   * `jq` and `python3` on PATH.
#
# Usage: ./doc/scripts/verify_consent_flow.sh [namespace]
#   namespace : kubernetes namespace of the consent-manager (default: trust-anchor)
set -euo pipefail

NS="${1:-trust-anchor}"
PROVIDER_NS="${PROVIDER_NS:-provider}"
HERE="$(cd "$(dirname "$0")" && pwd)"

# Endpoints (overridable) -- defaults match the local demo deployment.
KEYCLOAK_CONSUMER="${KEYCLOAK_CONSUMER:-https://keycloak-consumer.127.0.0.1.nip.io}"
DATA_SERVICE="${DATA_SERVICE:-https://mp-data-service.127.0.0.1.nip.io}"
GATED_URL="${GATED_URL:-https://mp-data-service-consent.127.0.0.1.nip.io/ngsi-ld/v1/entities/urn:ngsi-ld:PersonalProfile:alice}"
PROXY="${PROXY:-localhost:8888}"
# Local ports for the port-forwards to the consent-manager and TM Forum API.
CM_PORT="${CM_PORT:-3001}"
TMF_PORT="${TMF_PORT:-8090}"
# The consent-facade FQDN is DATA (stored in the consent-manager, dereferenced by
# it in-cluster), so it must be the cluster service name, not localhost.
FACADE="${FACADE:-http://consent-facade.${PROVIDER_NS}.svc.cluster.local:8080}"
# Grace period after a grant/revoke write before the read, so the change is visible.
SETTLE_SECONDS="${SETTLE_SECONDS:-2}"

if [ ! -f cert/did.json ]; then
  echo "cert/ holder identity not found -- create it first (see the prerequisites in this script's header)." >&2
  exit 1
fi

CM="http://localhost:${CM_PORT}/v1"
TMF="http://localhost:${TMF_PORT}/tmf-api"

# -- port-forwards (cleaned up on exit) --------------------------------------
kubectl -n "$NS" port-forward svc/consent-manager "${CM_PORT}:3000" >/dev/null 2>&1 &
CM_PF=$!
kubectl -n "$PROVIDER_NS" port-forward svc/tm-forum-api-svc "${TMF_PORT}:8080" >/dev/null 2>&1 &
TMF_PF=$!
trap 'kill "$CM_PF" "$TMF_PF" 2>/dev/null || true' EXIT

wait_up() { # <url> : wait until the port-forward accepts connections
  for _ in $(seq 1 30); do curl -s -o /dev/null "$1" && return 0; sleep 1; done
  echo "timed out waiting for $1" >&2; exit 1
}
wait_up "$CM/participants/me"
wait_up "$TMF/party/v4/organization?limit=1"

pass=0
fail=0

# assert_read <expected-http-code> <label>: perform the gated read and compare.
assert_read() {
  local expected="$1" label="$2" code
  code=$(curl -k -x "$PROXY" -s -o /dev/null -w '%{http_code}' -X GET "$GATED_URL" \
    -H "Authorization: Bearer $ACCESS_TOKEN")
  if [ "$code" = "$expected" ]; then
    echo "  PASS  $label -> HTTP $code"
    pass=$((pass + 1))
  else
    echo "  FAIL  $label -> HTTP $code (expected $expected)"
    fail=$((fail + 1))
  fi
}

echo "== consent flow end-to-end (namespace: $NS) =="

echo "-- issuing the consumer credential + OID4VP access token --"
USER_CREDENTIAL=$("$HERE/get_credential.sh" "$KEYCLOAK_CONSUMER" user-credential employee)
ACCESS_TOKEN=$("$HERE/get_access_token_oid4vp.sh" "$DATA_SERVICE" "$USER_CREDENTIAL" default)
# The data subject the consent is keyed on is the access-token `sub` (see item 1).
DID=$(python3 -c "import sys,base64,json; p=sys.argv[1].split('.')[1]; p+='='*(-len(p)%4); print(json.loads(base64.urlsafe_b64decode(p))['sub'])" "$ACCESS_TOKEN")
echo "  data subject (token sub): $DID"

echo "-- preparing participants, agreement and privacy notice (consent-manager API) --"
CONSENT_KEY=$(kubectl -n "$NS" exec deploy/consent-manager -- printenv X_VISIONSTRUST_CONSENT_KEY)

# provider participant (already registered by the deploy Job): log in, read its SD
PROVIDER_JWT=$(curl -s -X POST "$CM/participants/login" -H 'Content-Type: application/json' \
  -d '{"clientID":"consent-demo-provider","clientSecret":"demo"}' | jq -r .jwt)
PROVIDER_SD=$(curl -s "$CM/participants/me" -H "Authorization: Bearer $PROVIDER_JWT" | jq -r .selfDescriptionURL)
PROV_ORG=${PROVIDER_SD##*/participants/}

# consumer TM Forum org (find-or-create by name so its SD is stable across re-runs)
CONS_ORG=$(curl -s "$TMF/party/v4/organization?limit=1000" \
  | jq -r '[.[] | select(.name=="Consent Demo Consumer")][0].id // empty')
[ -z "$CONS_ORG" ] && CONS_ORG=$(curl -s -X POST "$TMF/party/v4/organization" -H 'Content-Type: application/json' \
  -d '{"name":"Consent Demo Consumer","tradingName":"Consent Demo Consumer","isLegalEntity":true,
       "organizationType":"company","contactMedium":[{"characteristic":{"country":"DE"}}],
       "partyCharacteristic":[{"name":"did","value":"did:web:fancy-marketplace.biz"}]}' | jq -r .id)
CONSUMER_SD="$FACADE/participants/$CONS_ORG"

# consumer participant (idempotent 201/409), then its token
curl -s -o /dev/null -X POST "$CM/participants" -H 'Content-Type: application/json' \
  -d "{\"legalName\":\"Fancy Marketplace Co.\",\"email\":\"consumer@fancy-marketplace.biz\",\"did\":\"did:web:fancy-marketplace.biz\",\"clientID\":\"consent-demo-consumer\",\"clientSecret\":\"demo\",\"selfDescriptionURL\":\"$CONSUMER_SD\"}"
CONSUMER_JWT=$(curl -s -X POST "$CM/participants/login" -H 'Content-Type: application/json' \
  -d '{"clientID":"consent-demo-consumer","clientSecret":"demo"}' | jq -r .jwt)

# single agreement between the pair: purge stale ones, then seed spec/offering/agreement
for a in $(curl -s "$TMF/agreementManagement/v4/agreement?limit=1000" | jq -r '.[].id'); do
  curl -s -o /dev/null -X DELETE "$TMF/agreementManagement/v4/agreement/$a"
done
SPEC_ID=$(curl -s -X POST "$TMF/productCatalogManagement/v4/productSpecification" -H 'Content-Type: application/json' -d @- <<JSON | jq -r .id
{ "name": "Personal Profile", "description": "The subject's profile",
  "productSpecCharacteristic": [ { "name": "purpose", "valueType": "object",
    "productSpecCharacteristicValue": [ { "value": {
      "id": "profile-service-provision", "name": "Personal profile for service provision",
      "description": "Deliver the requested service.", "purpose": "https://w3id.org/dpv#ServiceProvision" } } ] } ] }
JSON
)
OFFERING_ID=$(curl -s -X POST "$TMF/productCatalogManagement/v4/productOffering" -H 'Content-Type: application/json' \
  -d "{\"name\":\"Personal Profile Offering\",\"productSpecification\":{\"id\":\"$SPEC_ID\"}}" | jq -r .id)
curl -s -o /dev/null -X POST "$TMF/agreementManagement/v4/agreement" -H 'Content-Type: application/json' -d @- <<JSON
{ "name": "Profile sharing agreement", "status": "approved",
  "agreementItem": [ { "productOffering": [ { "id": "$OFFERING_ID" } ] } ],
  "engagedParty": [ { "id": "$PROV_ORG", "role": "Provider" }, { "id": "$CONS_ORG", "role": "Consumer" } ],
  "characteristic": [
    { "name": "policy", "value": { "@type": "Set", "uid": "urn:policy:profile", "permission": [ { "target": "urn:asset:profile", "action": "use" } ] } },
    { "name": "provider-id", "value": "$PROVIDER_SD" },
    { "name": "consumer-id", "value": "$CONSUMER_SD" },
    { "name": "signing-date", "value": $(date +%s) } ] }
JSON

# register the subject on both sides (idempotent), then resolve the provider-side
# UserIdentifier id (the x-user-key) via identifier/search - works whether new or existing
curl -s -o /dev/null -X POST "$CM/users/register" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $PROVIDER_JWT" -d "{\"email\":\"$DID\",\"identifier\":\"$DID\"}"
curl -s -o /dev/null -X POST "$CM/users/register" -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $CONSUMER_JWT" -d "{\"email\":\"$DID\",\"identifier\":\"$DID\"}"
USER_KEY=$(curl -s -X POST "$CM/users/identifier/search" -H 'Content-Type: application/json' \
  -H "x-visionstrust-consent-key: $CONSENT_KEY" \
  -d "{\"selfDescription\":\"$PROVIDER_SD\",\"email\":\"$DID\"}" | jq -r .userIdentifier)

# the privacy notice the facade projected (must carry data + purposes); retry, since the
# just-written agreement may take a moment to become visible to the facade.
PROV_B64=$(printf '%s' "$PROVIDER_SD" | base64 -w0)
CONS_B64=$(printf '%s' "$CONSUMER_SD" | base64 -w0)
PN_ID=""; DATA="[]"
for _ in $(seq 1 20); do
  NOTICE=$(curl -s "$CM/consents/$DID/$PROV_B64/$CONS_B64" -H "x-user-key: $USER_KEY")
  PN_ID=$(echo "$NOTICE" | jq -r '.[0]._id // empty')
  DATA=$(echo "$NOTICE" | jq -c '[.[0].data[].resource]')
  { [ -n "$PN_ID" ] && [ "$DATA" != "[]" ]; } && break
  sleep 1
done
if [ -z "$PN_ID" ] || [ "$DATA" = "[]" ]; then
  echo "no privacy notice with data was projected (user_key=$USER_KEY) -- check participant/agreement consistency." >&2
  exit 1
fi

# grant consent (event "given"); prints the consent id (the receipt's recordId)
grant() {
  curl -s -X POST "$CM/consents" -H 'Content-Type: application/json' -H "x-user-key: $USER_KEY" \
    -d "{\"privacyNoticeId\":\"$PN_ID\",\"event\":\"given\",\"data\":$DATA}" | jq -r '.record.recordId'
}
# withdraw a consent by terminating it (status -> "terminated"; the plugin allows only "granted")
terminate() {
  curl -s -o /dev/null -X POST "$CM/consents/$1/terminate" -H 'Content-Type: application/json' \
    -H "x-user-key: $USER_KEY" -d '{}'
}

echo "-- grant consent --"
CONSENT_ID=$(grant)
sleep "$SETTLE_SECONDS"
assert_read 200 "granted -> read allowed"

echo "-- revoke consent --"
terminate "$CONSENT_ID"
sleep "$SETTLE_SECONDS"
assert_read 403 "revoked -> read denied"

echo "-- re-grant consent (a fresh 'given' consent) --"
grant >/dev/null
sleep "$SETTLE_SECONDS"
assert_read 200 "re-granted -> read allowed"

echo "== $pass passed, $fail failed =="
[ "$fail" -eq 0 ]
