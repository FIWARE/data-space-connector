#!/bin/bash
# Helm post-renderer for CI: increase progressDeadlineSeconds on Deployments.
#
# On resource-constrained CI runners, init containers that wait for
# dependencies (e.g., keycloak) can consume most of the default 600s
# Kubernetes progress deadline, leaving insufficient time for the main
# container to start and pass readiness checks.

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
'
