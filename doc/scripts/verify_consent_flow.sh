#!/usr/bin/env bash
#
# verify_consent_flow.sh -- automated end-to-end check of the consent-gated data
# access flow (roadmap item 8). It drives the exact demo path and asserts the
# HTTP outcomes:
#   grant consent   -> the gated read returns 200
#   revoke consent  -> the gated read returns 403
#   re-grant consent-> the gated read returns 200
# It exits non-zero on the first failed assertion, so it can gate CI or serve as
# a manual smoke test. It reuses the demo helper scripts rather than duplicating
# their logic.
#
# Prerequisites (same as the demo in doc/CONSENT_MANAGEMENT.md):
#   * a running consent-enabled cluster (e.g. `mvn ... -Pconsent`) with KUBECONFIG
#     pointing at it, and the squid proxy reachable on :8888,
#   * the consumer holder identity under cert/ (created by the did-helper step:
#     `mkdir -p cert && docker run -v $(pwd)/cert:/cert quay.io/wi_stefan/did-helper:0.1.1`),
#     which get_access_token_oid4vp.sh reads.
#
# Usage: ./doc/scripts/verify_consent_flow.sh [namespace]
#   namespace : kubernetes namespace of the consent-manager (default: trust-anchor)
set -euo pipefail

NS="${1:-trust-anchor}"
HERE="$(cd "$(dirname "$0")" && pwd)"

# Endpoints (overridable) -- defaults match the local demo deployment.
KEYCLOAK_CONSUMER="${KEYCLOAK_CONSUMER:-https://keycloak-consumer.127.0.0.1.nip.io}"
DATA_SERVICE="${DATA_SERVICE:-https://mp-data-service.127.0.0.1.nip.io}"
GATED_URL="${GATED_URL:-https://mp-data-service-consent.127.0.0.1.nip.io/ngsi-ld/v1/entities/urn:ngsi-ld:PersonalProfile:alice}"
PROXY="${PROXY:-localhost:8888}"
# Grace period after a grant/revoke write before the read, so the change is visible.
SETTLE_SECONDS="${SETTLE_SECONDS:-2}"

if [ ! -f cert/did.json ]; then
  echo "cert/ holder identity not found -- create it first (see the prerequisites in this script's header)." >&2
  exit 1
fi

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

echo "-- grant consent --"
"$HERE/consent_grant.sh" "$DID" "$NS" >/dev/null
sleep "$SETTLE_SECONDS"
assert_read 200 "granted -> read allowed"

echo "-- revoke consent --"
"$HERE/consent_revoke.sh" "$DID" "$NS" >/dev/null
sleep "$SETTLE_SECONDS"
assert_read 403 "revoked -> read denied"

echo "-- re-grant consent --"
"$HERE/consent_grant.sh" "$DID" "$NS" >/dev/null
sleep "$SETTLE_SECONDS"
assert_read 200 "re-granted -> read allowed"

echo "== $pass passed, $fail failed =="
[ "$fail" -eq 0 ]
