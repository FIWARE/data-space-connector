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

jwt_header=$(echo -n "{\"alg\":\"ES256\", \"typ\":\"JWT\", \"kid\":\"${holder_did}\"}"| base64 -w0 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//)
payload=$(echo -n "{\"iss\": \"${holder_did}\", \"sub\": \"${holder_did}\", \"vp\": ${verifiable_presentation}}" | base64 -w0 | sed s/\+/-/g |sed 's/\//_/g' |  sed -E s/=+$//)
signature=$(echo -n "${jwt_header}.${payload}" | openssl dgst -sha256 -binary -sign cert/private-key.pem | base64 -w0 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//)
jwt="${jwt_header}.${payload}.${signature}"
vp_token=$(echo -n ${jwt} | base64 -w0 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//)

echo $(curl -s -k -x localhost:8888 -X POST $token_endpoint \
      --header 'Accept: */*' \
      --header 'Content-Type: application/x-www-form-urlencoded' \
      --data grant_type=vp_token \
      --data vp_token=${vp_token} \
      --data scope=$3 | jq '.access_token' -r )