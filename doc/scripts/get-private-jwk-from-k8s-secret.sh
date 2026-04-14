#!/usr/bin/env bash
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "Usage: $0 <namespace> <secret-name>" >&2
  echo "Extracts the tls.key from a cert-manager secret and outputs a P-256 JWK." >&2
  exit 1
fi

NAMESPACE="$1"
SECRET_NAME="$2"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

TMP_KEY=$(mktemp)
trap 'rm -f "$TMP_KEY"' EXIT

kubectl get secret "$SECRET_NAME" -n "$NAMESPACE" \
  -o jsonpath='{.data.tls\.key}' \
  | base64 -d > "$TMP_KEY"

"$SCRIPT_DIR/get-private-jwk-p-256.sh" "$TMP_KEY"
