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
# Helm hooks that a fresh install never runs are dropped (any `helm.sh/hook`
# whose value does not mention `install`).
#   `helm template` renders hook manifests like any other resource, and this
#   deployment applies them with kubectl - so an upgrade-only hook runs on what
#   is always a fresh install. The etcd subchart's `pre-upgrade` job is one: it
#   asks an etcd that does not exist yet for its members, fails with
#   "Unable to list members, are all members healthy?", and keeps the plugin
#   waiting until its deadline (the very activeDeadlineSeconds set above)
#   before the whole apply fails. `post-install` hooks - the TIL registration,
#   the verifier and route jobs - are kept, because a fresh install does run
#   those.
#
# Usage: ci-post-render.sh <directory>
# Patches all .yaml files in <directory> in-place; a file left with no
# documents is removed, so `kubectl apply --recursive` is not handed an
# empty manifest.

set -euo pipefail

TARGET_DIR="${1:?Usage: $0 <directory>}"

find "$TARGET_DIR" -name '*.yaml' -print0 | while IFS= read -r -d '' file; do
    awk '
    # Buffer each YAML document, so a hook document can still be dropped once
    # its annotations have been seen.
    function emit(   i) {
        if (hook == "" || hook ~ /install/) {
            for (i = 1; i <= n; i++) print buf[i]
        }
        n = 0; hook = ""; is_deploy = 0; is_job = 0; pd = 0; pj = 0
    }
    /^---/ { emit(); buf[++n] = $0; next }
    /^kind: Deployment/ { is_deploy = 1 }
    /^kind: Job/ { is_job = 1 }
    # matches `helm.sh/hook:`, not `helm.sh/hook-delete-policy:`
    /helm\.sh\/hook:/ { hook = $0 }
    /^  ttlSecondsAfterFinished:/ && is_job { $0 = "  ttlSecondsAfterFinished: 86400" }
    /^  activeDeadlineSeconds:/ && is_job { next }
    /^spec:/ && is_deploy && !pd {
        buf[++n] = $0
        buf[++n] = "  progressDeadlineSeconds: 1500"
        pd = 1
        next
    }
    /^spec:/ && is_job && !pj {
        buf[++n] = $0
        buf[++n] = "  activeDeadlineSeconds: 1500"
        pj = 1
        next
    }
    { buf[++n] = $0 }
    END { emit() }
    ' "$file" > "$file.tmp"

    if grep -qvE '^(---)?[[:space:]]*$' "$file.tmp"; then
        mv "$file.tmp" "$file"
    else
        rm -f "$file.tmp" "$file"
    fi
done
