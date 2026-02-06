#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <jwt>" >&2
  exit 1
fi

JWT="$1"

# Extract payload (2nd part)
PAYLOAD=$(echo "$JWT" | cut -d '.' -f2)

# Base64url → base64 → JSON
echo "$PAYLOAD" \
  | tr '_-' '/+' \
  | awk '{print $0 "=="}' \
  | base64 -d 2>/dev/null \
  | jq .