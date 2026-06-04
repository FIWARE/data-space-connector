#! /usr/bin/env bash

ALG="RS256"

DID=$1
KEY_ID=$2
CREDENTIAL_CONTENT=$3
KEY=$4

VERIFIABLE_CREDENTIAL=$(echo "{
  \"iss\": \"${DID}\",
  \"sub\": \"${DID}\",
  \"vc\": ${CREDENTIAL_CONTENT},
  \"iat\": 1748844919
}")

JWT_HEADER=$(echo -n "{\"alg\":\"ES256\", \"typ\":\"JWT\", \"kid\":\"${DID}#${KEY_ID}\"}")

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/convert_ec.sh"

function b64enc() { openssl enc -base64 -A | tr '+/' '-_' | tr -d '='; }

function rs_sign() { openssl dgst -binary -sha${1} -sign "$2"; }
function es_sign() { openssl dgst -binary -sha${1} -sign "$2" | convert_ec; }

JWT_HDR_B64="$(echo -n "$JWT_HEADER" | b64enc)"
JWT_PAY_B64="$(echo -n "$VERIFIABLE_CREDENTIAL" | b64enc)" 
UNSIGNED_JWT="$JWT_HDR_B64.$JWT_PAY_B64"

case "$ALG" in
    RS*) SIGNATURE=$(echo -n "$UNSIGNED_JWT" | rs_sign "${ALG#RS}" "$KEY" | b64enc) ;;
    ES*) SIGNATURE=$(echo -n "$UNSIGNED_JWT" | es_sign "${ALG#ES}" "$KEY" | b64enc) ;;
esac

echo "$UNSIGNED_JWT.$SIGNATURE"