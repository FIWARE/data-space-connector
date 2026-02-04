#!/usr/bin/env bash
set -euo pipefail

DID="$1"
TYPE="$2"


echo "{
  \"@context\": [
      \"https://www.w3.org/2018/credentials/v1\",
      \"https://w3id.org/security/suites/jws-2020/v1\",
      \"https://www.w3.org/ns/did/v1\"
    ],
    \"id\": \"http://org.yourdataspace.com/credentials/2347\",
    \"type\": [
      \"VerifiableCredential\",
      \"MembershipCredential\"
    ],
    \"issuer\": \"${DID}\",
    \"issuanceDate\": \"2023-08-18T00:00:00Z\",
    \"credentialSubject\": {
      \"id\": \"${DID}\",
      \"membership\": {
        \"membershipType\": \"${TYPE}\",
        \"since\": \"2023-01-01T00:00:00Z\"
      }
    }
}"
