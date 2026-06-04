#!/usr/bin/env bash
# Convert a DER-encoded ECDSA signature (stdin) to JWS raw R||S format (stdout).
# OpenSSL produces DER/ASN.1 signatures, but JWS (RFC 7518) requires the raw
# concatenation of R and S values (64 bytes for P-256, 96 bytes for P-384).

convert_ec() {
  local input r s
  input=$(openssl asn1parse -inform der)
  r=$(echo "$input" | head -2 | tail -1 | cut -d':' -f4)
  s=$(echo "$input" | head -3 | tail -1 | cut -d':' -f4)
  echo -n "$r" | xxd -r -p
  echo -n "$s" | xxd -r -p
}
