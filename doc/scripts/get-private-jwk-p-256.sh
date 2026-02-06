#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <ec-private-key.pem>" >&2
  exit 1
fi

PEM_FILE="$1"

if [ ! -f "$PEM_FILE" ]; then
  echo "File not found: $PEM_FILE" >&2
  exit 1
fi

# Extract EC key parameters in hex
EC_TEXT=$(openssl ec -in "$PEM_FILE" -no_public -text 2>/dev/null)

D_HEX=$(echo "$EC_TEXT" | awk '/priv:/{flag=1;next}/pub:/{flag=0}flag' | tr -d ' :\n')
PUB_HEX=$(openssl ec -in "$PEM_FILE" -pubout -text 2>/dev/null \
  | awk '/pub:/{flag=1;next}/ASN1 OID/{flag=0}flag' | tr -d ' :\n')

# Remove uncompressed point prefix (04)
PUB_HEX="${PUB_HEX#04}"

# Split x and y (each 32 bytes = 64 hex chars)
X_HEX="${PUB_HEX:0:64}"
Y_HEX="${PUB_HEX:64:64}"

# Helper: hex → base64url
hex_to_b64url () {
  echo "$1" \
    | xxd -r -p \
    | base64 \
    | tr '+/' '-_' \
    | tr -d '='
}

X_B64=$(hex_to_b64url "$X_HEX")
Y_B64=$(hex_to_b64url "$Y_HEX")
D_B64=$(hex_to_b64url "$D_HEX")

# Output JWK
jq -n \
  --arg kty "EC" \
  --arg crv "P-256" \
  --arg x "$X_B64" \
  --arg y "$Y_B64" \
  --arg d "$D_B64" \
  '{
    kty: $kty,
    crv: $crv,
    x: $x,
    y: $y,
    d: $d
  }'
