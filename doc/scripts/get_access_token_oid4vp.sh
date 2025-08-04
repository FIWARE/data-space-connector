#!/bin/bash

token_endpoint=$(curl -s -k -X GET "$1/.well-known/openid-configuration" | jq -r '.token_endpoint')
holder_did=$(cat cert/did.json | jq '.id' -r)

verifiable_presentation="{
  \"@context\": [\"https://www.w3.org/2018/credentials/v1\"],
  \"type\": [\"VerifiablePresentation\"],
  \"verifiableCredential\": [
      \"$2\"
  ],
  \"holder\": \"${holder_did}\"
}"


# Define header and payload JSON
header="{\"alg\":\"ES256\", \"typ\":\"JWT\", \"kid\":\"${holder_did}\"}"
payload="{\"iss\": \"${holder_did}\", \"sub\": \"${holder_did}\", \"vp\": ${verifiable_presentation}}"

# Base64url encode function (works the same on macOS and Ubuntu)
base64url_encode() {
  openssl base64 -A | tr '+/' '-_' | tr -d '='
}

# Encode header
header_b64=$(printf "%s" "$header" | base64url_encode)

# Encode payload
payload_b64=$(printf "%s" "$payload" | base64url_encode)

# Create signature input
signing_input="${header_b64}.${payload_b64}"

# Sign the input using the ES256 private key
signature_b64=$(printf "%s" "$signing_input" \
  | openssl dgst -sha256 -binary -sign cert/private-key.pem \
  | base64url_encode)

# Assemble the final JWT
jwt="${signing_input}.${signature_b64}"

echo $(curl -s -k -x localhost:8888 -X POST $token_endpoint \
      --header 'Accept: */*' \
      --header 'Content-Type: application/x-www-form-urlencoded' \
      --data grant_type=vp_token \
      --data vp_token=${jwt} \
      --data scope=$3 | jq '.access_token' -r )