#!/usr/bin/env bash
set -euo pipefail

############################################
# Usage check
############################################
if [ "$#" -ne 2 ]; then
  echo "Usage: $0 '<header.payload>' '<jwk-json>'" >&2
  exit 1
fi

DATA="$1"
JWK_JSON="$2"

############################################
# Helpers
############################################
b64url_decode() {
  local input="$1"
  local pad=$(( (4 - ${#input} % 4) % 4 ))
  printf '%s' "${input}$(printf '=%.0s' $(seq 1 $pad))" \
    | tr '_-' '/+' \
    | base64 -d
}

############################################
# Extract private key from JWK
############################################
D_B64URL=$(printf '%s' "$JWK_JSON" | jq -r '.d')

if [ -z "$D_B64URL" ] || [ "$D_B64URL" = "null" ]; then
  echo "Error: JWK does not contain private key 'd'" >&2
  exit 1
fi

############################################
# Decode Ed25519 private key
############################################
PRIV_HEX=$(b64url_decode "$D_B64URL" | xxd -p | tr -d '\n')

# Must be exactly 32 bytes = 64 hex chars
if [ "${#PRIV_HEX}" -ne 64 ]; then
  echo "Error: Ed25519 private key must be 32 bytes" >&2
  exit 1
fi

############################################
# Temp files (REQUIRED for OpenSSL 3)
############################################
MSG_FILE=$(mktemp)
KEY_FILE=$(mktemp)
trap 'rm -f "$MSG_FILE" "$KEY_FILE"' EXIT

printf '%s' "$DATA" > "$MSG_FILE"

############################################
# Build PKCS#8 Ed25519 private key (DER → PEM)
############################################
# PKCS#8 DER header for Ed25519:
# 30 2e 02 01 00 30 05 06 03 2b 65 70 04 22 04 20 <32-byte key>

printf "302e020100300506032b657004220420%s" "$PRIV_HEX" \
 | xxd -r -p \
 | openssl pkcs8 -inform DER -outform PEM -nocrypt \
 > "$KEY_FILE"

############################################
# Sign (this is the ONLY reliable invocation)
############################################
SIGNATURE=$(
  openssl pkeyutl \
    -sign \
    -rawin \
    -inkey "$KEY_FILE" \
    -in "$MSG_FILE" \
  | base64 -w0 \
  | tr '+/' '-_' \
  | sed -E 's/=+$//'
)

############################################
# Output full JWT
############################################
echo "${DATA}.${SIGNATURE}"
