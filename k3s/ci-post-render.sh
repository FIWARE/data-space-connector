#!/bin/bash
# Increase progressDeadlineSeconds on all rendered Deployment manifests.
#
# On resource-constrained CI runners, init containers that wait for
# dependencies (e.g., keycloak) can consume most of the default 600s
# Kubernetes progress deadline, leaving insufficient time for the main
# container to start and pass readiness checks.
#
# Usage: ci-post-render.sh <directory>
# Patches all .yaml files in <directory> in-place.

set -euo pipefail

TARGET_DIR="${1:?Usage: $0 <directory>}"

find "$TARGET_DIR" -name '*.yaml' -print0 | while IFS= read -r -d '' file; do
    awk '
    /^---/ { is_deployment = 0; patched = 0 }
    /^kind: Deployment/ { is_deployment = 1 }
    /^spec:/ && is_deployment && !patched {
        print
        print "  progressDeadlineSeconds: 1500"
        patched = 1
        next
    }
    { print }
    ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"
done
