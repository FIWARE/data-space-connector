#!/bin/bash
# Patch rendered manifests for CI resilience.
#
# Deployments: injects progressDeadlineSeconds: 1500.
#   Default 600s is too short when init containers wait for dependencies
#   (e.g., keycloak) on resource-constrained CI runners.
#
# Jobs: sets ttlSecondsAfterFinished: 86400 and activeDeadlineSeconds: 1500.
#   Short TTLs (e.g., 300s) cause Jobs to be garbage-collected before the
#   k3s-maven-plugin can verify completion, resulting in NotFound errors.
#
# Usage: ci-post-render.sh <directory>
# Patches all .yaml files in <directory> in-place.

set -euo pipefail

TARGET_DIR="${1:?Usage: $0 <directory>}"

find "$TARGET_DIR" -name '*.yaml' -print0 | while IFS= read -r -d '' file; do
    awk '
    /^---/ { is_deploy = 0; pd = 0; is_job = 0; pj = 0 }
    /^kind: Deployment/ { is_deploy = 1 }
    /^kind: Job/ { is_job = 1 }
    /^  ttlSecondsAfterFinished:/ && is_job { $0 = "  ttlSecondsAfterFinished: 86400" }
    /^  activeDeadlineSeconds:/ && is_job { next }
    /^spec:/ && is_deploy && !pd {
        print
        print "  progressDeadlineSeconds: 1500"
        pd = 1
        next
    }
    /^spec:/ && is_job && !pj {
        print
        print "  activeDeadlineSeconds: 1500"
        pj = 1
        next
    }
    { print }
    ' "$file" > "$file.tmp" && mv "$file.tmp" "$file"
done
