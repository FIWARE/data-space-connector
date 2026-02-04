#!/usr/bin/env bash
set -euo pipefail

DID="$1"
KEY_ID="$2"
CREDENTIAL_CONTENT="$3"

VERIFIABLE_CREDENTIAL=$(echo "{
  \"iss\": \"${DID}\",
  \"sub\": \"${DID}\",
  \"vc\": ${CREDENTIAL_CONTENT},
  \"iat\": 1748844919
}")

JWT_HEADER=$(echo -n "{\"alg\":\"EdDSA\", \"typ\":\"JWT\", \"kid\":\"${DID}#${KEY_ID}\"}"| base64 -w0 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//)

PAYLOAD=$(echo -n ${VERIFIABLE_CREDENTIAL} | base64 -w0 | sed s/\+/-/g |sed 's/\//_/g' |  sed -E s/=+$//)

echo $JWT_HEADER.$PAYLOAD