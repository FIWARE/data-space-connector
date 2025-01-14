#!/bin/bash

access_token=$(curl -s -X POST "$1/realms/test-realm/protocol/openid-connect/token" \
  --header 'Accept: */*' \
  --header 'Content-Type: application/x-www-form-urlencoded' \
  --data grant_type=password \
  --data client_id=admin-cli \
  --data username=test-user \
  --data password=test | jq '.access_token' -r)

offer_uri=$(curl -s -X GET "$1/realms/test-realm/protocol/oid4vc/credential-offer-uri?credential_configuration_id=$2" \
  --header "Authorization: Bearer ${access_token}" | jq '"\(.issuer)\(.nonce)"' -r)

export pre_authorized_code=$(curl -s -X GET ${offer_uri} \
  --header "Authorization: Bearer ${access_token}" | jq '.grants."urn:ietf:params:oauth:grant-type:pre-authorized_code"."pre-authorized_code"' -r)

credential_access_token=$(curl -s -X POST "$1/realms/test-realm/protocol/openid-connect/token" \
  --header 'Accept: */*' \
  --header 'Content-Type: application/x-www-form-urlencoded' \
  --data grant_type=urn:ietf:params:oauth:grant-type:pre-authorized_code \
  --data pre-authorized_code=${pre_authorized_code} | jq '.access_token' -r)

curl -s -X POST "$1/realms/test-realm/protocol/oid4vc/credential" \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --header "Authorization: Bearer ${credential_access_token}" \
  --data "{\"credential_identifier\":\"$2\", \"format\":\"jwt_vc\"}" | jq '.credential' -r