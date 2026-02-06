#!/usr/bin/env bash
set -euo pipefail

JWK=$1
PARTICIPANT_ID=$2
CREDENTIAL_SERVICE_URL=$3
KEY_ID=$4

PUBLIC_JWK=$(echo $JWK | jq --arg did "$PARTICIPANT_ID" --arg key_id "$KEY_ID" 'del(.d) | .kid = ($did + "#" + $key_id)')
BASE_64_PARTICIPANT_ID=$(printf '%s' "$PARTICIPANT_ID" | base64 | tr '+/' '-_' | tr -d '=')

DATA=$(echo "{
      \"role\": [\"admin\"],
      \"serviceEndpoints\": [
          {
            \"type\": \"CredentialService\", 
            \"serviceEndpoint\": \"${CREDENTIAL_SERVICE_URL}/api/credentials/v1/participants/${BASE_64_PARTICIPANT_ID}\",
            \"id\": \"credential-service\"
          }
        ],
      \"active\": true,
      \"participantId\": \"${PARTICIPANT_ID}\",
      \"did\": \"${PARTICIPANT_ID}\",
      \"key\": {
        \"keyId\": \"${PARTICIPANT_ID}#${KEY_ID}\",
        \"privateKeyAlias\": \"${KEY_ID}\",
        \"publicKeyJwk\": {}     
      }
    }")

DATA_RAW=$(echo "${DATA}" | jq --argjson pem "$PUBLIC_JWK" '.key.publicKeyJwk = $pem')

echo $DATA_RAW