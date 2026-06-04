#!/usr/bin/env bash
set -euo pipefail

DID=$1
KEY_ID=$2
CREDENTIAL_CONTENT=$3
KEY_FILE=$4

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/convert_ec.sh"

JWT_HEADER=$(echo -n "{\"alg\":\"ES256\", \"typ\":\"JWT\", \"kid\":\"${DID}#${KEY_ID}\"}"| base64 -w0 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//)

PAYLOAD=$(echo -n "{\"iss\": \"${DID}\", \"sub\": \"${DID}\", \"vc\": ${CREDENTIAL_CONTENT}, \"iat\": 1748844919}" | base64 -w0 | sed s/\+/-/g |sed 's/\//_/g' |  sed -E s/=+$//)

SIGNATURE=$(echo -n "${JWT_HEADER}.${PAYLOAD}" | openssl dgst -sha256 -binary -sign "${KEY_FILE}" | convert_ec | base64 -w0 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//)

echo "${JWT_HEADER}.${PAYLOAD}.${SIGNATURE}"
