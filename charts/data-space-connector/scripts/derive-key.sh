#!/bin/sh
# Derive a JWK pair from the participant's identity private key.
#
# Writes ${SHARED_DIR}/private.jwk and ${SHARED_DIR}/public.jwk so that every later step is
# algorithm-agnostic.
#
# Getting the algorithm wrong is not a loud failure: extracting an RSA modulus out of an EC key
# produces a syntactically valid JWK full of garbage, and the mismatch only surfaces later as a
# counterparty rejecting every signature. Hence the explicit IDENTITY_KEY_ALGORITHM and the
# sanity checks on the extracted lengths.
set -e

KEY_FILE="${IDENTITY_KEY_FILE:?IDENTITY_KEY_FILE is required}"
ALG="${IDENTITY_KEY_ALGORITHM:?IDENTITY_KEY_ALGORITHM is required}"
KEY_ID="${KEY_ID:?KEY_ID is required}"
# overridable so the script can be run outside the Job (the chart's unit tests do)
SHARED_DIR="${SHARED_DIR:-/shared}"

KEY_TEXT=$(openssl pkey -in "${KEY_FILE}" -text -noout)

# `openssl pkey -text` prints the key material as colon-separated hex between labelled
# sections; pull out everything between two labels and strip the formatting.
extract_hex() {
  echo "$KEY_TEXT" | awk -v s="$1" -v e="$2" '$0 ~ s {flag=1; next} $0 ~ e {flag=0} flag {print}' | tr -d ' :\n'
}
# ASN.1 INTEGERs carry a leading 00 octet when the high bit is set; base64url JWK members must
# not, or the value decodes one byte too long.
strip_leading_zero() {
  case "$1" in
    00*) printf '%s' "${1#00}" ;;
    *) printf '%s' "$1" ;;
  esac
}
hex_to_b64url() {
  printf '%s' "$1" | xxd -r -p | base64 | tr '+/' '-_' | tr -d '=\n'
}

case "${ALG}" in
  EC)
    CURVE=$(echo "$KEY_TEXT" | awk '/ASN1 OID:/ {print $3}')
    case "${CURVE}" in
      prime256v1) CRV="P-256"; COORD_HEX=64 ;;
      secp384r1)  CRV="P-384"; COORD_HEX=96 ;;
      *) echo "unsupported EC curve '${CURVE}' (expected prime256v1 or secp384r1)"; exit 1 ;;
    esac

    D_HEX=$(extract_hex '^priv:' '^pub:')
    PUB_HEX=$(extract_hex '^pub:' 'ASN1 OID')
    # strip the uncompressed-point prefix, then split into the two coordinates
    PUB_HEX=${PUB_HEX#04}
    X_HEX=$(printf '%s' "$PUB_HEX" | cut -c1-${COORD_HEX})
    Y_HEX=$(printf '%s' "$PUB_HEX" | cut -c$((COORD_HEX + 1))-$((COORD_HEX * 2)))

    if [ ${#X_HEX} -ne "${COORD_HEX}" ] || [ ${#Y_HEX} -ne "${COORD_HEX}" ] || [ -z "$D_HEX" ]; then
      echo "unexpected EC key material (x=${#X_HEX} y=${#Y_HEX} d=${#D_HEX} hex chars, expected ${COORD_HEX})"
      exit 1
    fi

    X=$(hex_to_b64url "$X_HEX")
    Y=$(hex_to_b64url "$Y_HEX")
    D=$(hex_to_b64url "$(strip_leading_zero "$D_HEX")")

    printf '{"kty":"EC","crv":"%s","x":"%s","y":"%s","d":"%s"}' "$CRV" "$X" "$Y" "$D" > "${SHARED_DIR}/private.jwk"
    printf '{"kty":"EC","crv":"%s","x":"%s","y":"%s","kid":"%s"}' "$CRV" "$X" "$Y" "$KEY_ID" > "${SHARED_DIR}/public.jwk"
    ;;
  RSA)
    # `e` is not extracted: openssl prints publicExponent in decimal rather than as hex bytes,
    # so it is asserted instead. Every cert-manager-issued RSA key uses 65537.
    EXPONENT=$(echo "$KEY_TEXT" | awk '/^publicExponent:/ {print $2}')
    if [ "${EXPONENT}" != "65537" ]; then
      echo "unsupported RSA public exponent '${EXPONENT}' (only 65537 is handled)"
      exit 1
    fi

    N_HEX=$(strip_leading_zero "$(extract_hex '^modulus:' '^publicExponent:')")
    D_HEX=$(strip_leading_zero "$(extract_hex '^privateExponent:' '^prime1:')")

    if [ ${#N_HEX} -lt 256 ] || [ -z "$D_HEX" ]; then
      echo "unexpected RSA key material (n=${#N_HEX} d=${#D_HEX} hex chars)"
      exit 1
    fi

    N=$(hex_to_b64url "$N_HEX")
    D=$(hex_to_b64url "$D_HEX")

    printf '{"kty":"RSA","e":"AQAB","n":"%s","d":"%s"}' "$N" "$D" > "${SHARED_DIR}/private.jwk"
    printf '{"kty":"RSA","e":"AQAB","n":"%s","kid":"%s"}' "$N" "$KEY_ID" > "${SHARED_DIR}/public.jwk"
    ;;
  *)
    echo "unsupported IDENTITY_KEY_ALGORITHM '${ALG}' (expected EC or RSA)"
    exit 1
    ;;
esac

echo "derived ${ALG} JWK for ${KEY_ID}"
