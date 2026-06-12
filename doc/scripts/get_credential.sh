#!/bin/bash

format=${4:-jwt_vc} 

access_token=$(curl -s -k -x localhost:8888 -X POST "$1/realms/test-realm/protocol/openid-connect/token" \
  --header 'Accept: */*' \
  --header 'Content-Type: application/x-www-form-urlencoded' \
  --data grant_type=password \
  --data client_id=account-console \
  --data username=$3 \
  --data scope=openid \
  --data password=test | jq '.access_token' -r)

offer_uri=$(curl -s -k -x localhost:8888 -X GET "$1/realms/test-realm/protocol/oid4vc/create-credential-offer?credential_configuration_id=$2&pre_authorized=true" \
  --header "Authorization: Bearer ${access_token}" | jq '"\(.issuer)/\(.nonce)"' -r)

export pre_authorized_code=$(curl -s -k -x localhost:8888 -X GET ${offer_uri} \
  --header "Authorization: Bearer ${access_token}" | jq '.grants."urn:ietf:params:oauth:grant-type:pre-authorized_code"."pre-authorized_code"' -r)

token_response=$(curl -s -k -x localhost:8888 -X POST "$1/realms/test-realm/protocol/openid-connect/token" \
  --header 'Accept: */*' \
  --header 'Content-Type: application/x-www-form-urlencoded' \
  --data grant_type=urn:ietf:params:oauth:grant-type:pre-authorized_code \
  --data pre-authorized_code=${pre_authorized_code})

credential_access_token=$(echo "${token_response}" | jq '.access_token' -r)

# KC main / patched 26.6.2 (keycloak/keycloak#47404) returns authorization_details
# with credential_identifiers — use credential_identifier when available, fall back
# to credential_configuration_id for older Keycloaks.
credential_identifier=$(echo "${token_response}" | jq -r '
  .authorization_details[]? |
  select(.credential_identifiers != null and (.credential_identifiers | length) > 0) |
  .credential_identifiers[0]' | head -n 1)

if [ -n "${credential_identifier}" ] && [ "${credential_identifier}" != "null" ]; then
  credential_request_body="{\"credential_identifier\":\"${credential_identifier}\"}"
else
  credential_request_body="{\"credential_configuration_id\":\"$2\"}"
fi

curl -s -k -x localhost:8888 -X POST "$1/realms/test-realm/protocol/oid4vc/credential" \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --header "Authorization: Bearer ${credential_access_token}" \
  --data "${credential_request_body}" | jq '.credentials[0].credential' -r
