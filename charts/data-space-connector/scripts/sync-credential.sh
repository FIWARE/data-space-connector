#!/bin/sh
# Keep the identityhub's copy of the participant's credential in step with the Secret the
# vc-operator maintains.
#
# Without it every DCP flow fails: the connector presents a credential the identityhub does not
# hold. Obtaining and renewing the credential is the vc-operator's job - it speaks OID4VCI to the
# issuer and writes the result to a Secret - and this only mirrors that Secret into the
# identityhub. It therefore has to keep running: a push that happens once at deploy time leaves
# the copy stale after the first unattended renewal, which takes the DCP lane down with nothing
# having changed in the cluster.
#
# Runs as a sidecar in the identityhub pod: re-reads the mounted Secret every RESYNC_SECONDS and
# pushes when it changed. The kubelet refreshes mounted Secrets by itself, so no watch on the
# Kubernetes API and no RBAC are involved.
set -e
# overridable so the scripts can be exercised outside the Job
SCRIPT_DIR="${SCRIPT_DIR:-/scripts}"
. "${SCRIPT_DIR}/lib.sh"

: "${DID:?DID is required}"
: "${IDENTITY_API:?IDENTITY_API is required}"
: "${CREDENTIAL_FILE:?CREDENTIAL_FILE is required}"
: "${CREDENTIAL_ID:?CREDENTIAL_ID is required}"
RESYNC_SECONDS="${RESYNC_SECONDS:-60}"

ONCE=""
[ "$1" = "--once" ] && ONCE="yes"

BASE64_DID=$(b64url "${DID}")
CREDENTIALS_API="${IDENTITY_API}/participants/${BASE64_DID}/credentials"

# Push $1 (a compact JWT) into the identityhub. Returns non-zero instead of exiting, so a
# transient failure in loop mode is retried on the next cycle rather than killing the sidecar.
push_credential() {
  _raw_vc="$1"

  # Refuse a credential signed under a different kid than the one the DID document publishes:
  # inserting it would present the counterparty with an unverifiable credential.
  _header=$(printf '%s' "${_raw_vc}" | cut -d. -f1 | tr '_-' '/+')
  case $((${#_header} % 4)) in 2) _header="${_header}==" ;; 3) _header="${_header}=" ;; esac
  _header_json=$(printf '%s' "${_header}" | base64 -d 2>/dev/null || true)
  if ! printf '%s' "${_header_json}" | grep -qF "${KEY_ID}"; then
    echo "ERROR: the credential in ${CREDENTIAL_FILE} was not signed with ${KEY_ID}"
    echo "header: ${_header_json}"
    echo "force a re-issuance (bump the VerifiableCredentialRequest generation)"
    return 1
  fi

  # Extract the `vc` object from the JWT payload. Done by brace counting rather than with a
  # JSON parser to keep this image dependency-free.
  _payload=$(printf '%s' "${_raw_vc}" | cut -d. -f2 | tr '_-' '/+')
  case $((${#_payload} % 4)) in 2) _payload="${_payload}==" ;; 3) _payload="${_payload}=" ;; esac
  _vc=$(printf '%s' "${_payload}" | base64 -d | awk '
    { line = line $0 }
    END {
      start = index(line, "\"vc\":")
      if (start == 0) { exit 1 }
      rest = substr(line, start + 5)
      depth = 0
      for (i = 1; i <= length(rest); i++) {
        c = substr(rest, i, 1)
        if (c == "{") depth++
        else if (c == "}") { depth--; if (depth == 0) { print substr(rest, 1, i); exit 0 } }
      }
      exit 1
    }') || true
  if [ -z "${_vc}" ]; then
    echo "ERROR: could not extract the vc claim from the credential"
    return 1
  fi

  _body=$(printf '{"id":"%s","participantContextId":"%s","verifiableCredentialContainer":{"rawVc":"%s","format":"VC1_0_JWT","credential":%s}}' \
    "${CREDENTIAL_ID}" "${DID}" "${_raw_vc}" "${_vc}")

  echo "Storing credential ${CREDENTIAL_ID}..."
  _code=$(printf '%s' "${_body}" | curl -s -o /tmp/cred_resp.json -w "%{http_code}" -X POST "${CREDENTIALS_API}" \
    -H "x-api-key: $(superuser_token)" \
    -H "Content-Type: application/json" \
    --data-binary @-) || return 1
  echo "POST ${CREDENTIALS_API} -> HTTP ${_code}"

  # 409 means a copy is already stored. Replace it: on a renewal the stored copy is the stale
  # one, and leaving it in place is how the DCP lane dies without anyone deploying anything.
  if [ "${_code}" = "409" ]; then
    echo "credential ${CREDENTIAL_ID} already stored - replacing it"
    _code=$(curl -s -o /tmp/cred_del.json -w "%{http_code}" -X DELETE "${CREDENTIALS_API}/${CREDENTIAL_ID}" \
      -H "x-api-key: $(superuser_token)") || return 1
    echo "DELETE ${CREDENTIALS_API}/${CREDENTIAL_ID} -> HTTP ${_code}"
    if [ "${_code}" -lt 200 ] || [ "${_code}" -ge 300 ]; then
      cat /tmp/cred_del.json
      return 1
    fi
    _code=$(printf '%s' "${_body}" | curl -s -o /tmp/cred_resp.json -w "%{http_code}" -X POST "${CREDENTIALS_API}" \
      -H "x-api-key: $(superuser_token)" \
      -H "Content-Type: application/json" \
      --data-binary @-) || return 1
    echo "POST ${CREDENTIALS_API} (retry) -> HTTP ${_code}"
  fi

  if [ "${_code}" -lt 200 ] || [ "${_code}" -ge 300 ]; then
    cat /tmp/cred_resp.json
    return 1
  fi
  echo "credential ${CREDENTIAL_ID} stored"
  return 0
}

if [ -n "${ONCE}" ]; then
  if [ ! -s "${CREDENTIAL_FILE}" ]; then
    echo "no credential at ${CREDENTIAL_FILE} yet - nothing to store"
    echo "the vc-operator writes it asynchronously; re-run once it exists"
    exit 1
  fi
  push_credential "$(tr -d '\n' < "${CREDENTIAL_FILE}")"
  exit $?
fi

# Loop mode. The comparison is against the last credential actually stored, so a failed push is
# retried next cycle, and a restart of this container re-pushes once and then goes quiet.
echo "watching ${CREDENTIAL_FILE} every ${RESYNC_SECONDS}s"
LAST_PUSHED=""
while true; do
  if [ -s "${CREDENTIAL_FILE}" ]; then
    RAW_VC=$(tr -d '\n' < "${CREDENTIAL_FILE}")
    if [ "${RAW_VC}" != "${LAST_PUSHED}" ]; then
      if push_credential "${RAW_VC}"; then
        LAST_PUSHED="${RAW_VC}"
      else
        echo "push failed - retrying in ${RESYNC_SECONDS}s"
      fi
    fi
  else
    # optional mount: on a first install the vc-operator has not written the Secret yet. Report
    # it and keep waiting rather than crash-looping the sidecar and with it the pod's events.
    echo "no credential at ${CREDENTIAL_FILE} yet - waiting"
  fi
  sleep "${RESYNC_SECONDS}"
done
