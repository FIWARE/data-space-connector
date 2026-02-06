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

function b64enc() { openssl enc -base64 -A | tr '+/' '-_' | tr -d '='; }

function convert_ec {
    INPUT=$(openssl asn1parse -inform der)
    R=$(echo "$INPUT" | head -2 | tail -1 | cut -d':' -f4)
    S=$(echo "$INPUT" | head -3 | tail -1 | cut -d':' -f4)

    echo -n $R | xxd -r -p
    echo -n $S | xxd -r -p
}

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