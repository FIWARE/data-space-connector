#!/bin/bash
# Deploy the local data space into a running cluster with `helm upgrade --install`.
#
# Called by the Maven build (`mvn clean deploy -Plocal`, and the integration tests in
# the `it` module), but usable on its own against any cluster:
#
#   KUBECONFIG=it/target/k3s.yaml k3s/deploy.sh
#
# Every chart is installed as a Helm release, the same way an operator installs it, so
# hooks run in their phase and order, `lookup` sees the existing secrets, the CRDs of
# each chart's crds/ folder are installed, and running the script again upgrades the
# releases in place.
#
# Order matters:
#   1. namespaces
#   2. the operators (mongo, postgres, cert-manager) - their CRDs and webhooks have to
#      be up before anything uses them
#   3. the manifests that are not part of a chart: the cluster infrastructure (traefik,
#      coredns, squid, the cert-manager issuers) and the participants' additional
#      resources. They contain cert-manager Certificates and ClusterIssuers, so they come
#      after cert-manager.
#   4. the participants: the trust anchor first, because the registration hooks of the
#      other participants call it, then provider and consumer.
#
# Configuration, from the environment (the Maven profiles set these):
#   KUBECONFIG                 cluster to deploy to (required)
#   HELM                       helm binary, default `helm`
#   TIMEOUT                    timeout of every helm operation, default 25m
#   TRUST_ANCHOR_VALUES        comma-separated values files, relative to the repository
#   CONSENT_AUTHORITY_VALUES   root. A release whose values are empty is not installed.
#   PROVIDER_VALUES
#   CONSUMER_VALUES
#   PROMETHEUS_VALUES
#   GAIA_X_INFRA               `true` to also apply k3s/gaia-x-infra
#
# Requires kubectl on the PATH.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

: "${KUBECONFIG:?KUBECONFIG must point to the cluster to deploy to}"
export KUBECONFIG
HELM="${HELM:-helm}"
TIMEOUT="${TIMEOUT:-25m}"

DSC_CHART=charts/data-space-connector
TRUST_ANCHOR_CHART=charts/trust-anchor

log() {
    echo "==> $*"
}

on_error() {
    echo "Deployment failed. Releases:" >&2
    "$HELM" list --all-namespaces --all >&2 || true
}
trap on_error ERR

# install <release> <namespace> <chart> <comma-separated values files> [extra helm args]
install() {
    local release=$1 namespace=$2 chart=$3 values=$4
    shift 4
    if [ -z "$values" ]; then
        return 0
    fi
    local args=() files=() file
    IFS=',' read -ra files <<< "$values"
    for file in "${files[@]}"; do
        args+=(-f "$file")
    done
    log "helm upgrade --install $release ($namespace) with $values"
    "$HELM" upgrade --install "$release" "$chart" \
        --namespace "$namespace" \
        "${args[@]}" \
        --wait --timeout "$TIMEOUT" \
        "$@"
}

log "Namespaces"
kubectl apply -f k3s/namespaces

log "Chart dependencies"
"$HELM" dependency update "$DSC_CHART"
"$HELM" dependency update "$TRUST_ANCHOR_CHART"

log "Operators"
install mongo-operator mongo-operator "$DSC_CHART" k3s/mongo-operator.yaml
install postgres-operator postgres-operator "$DSC_CHART" k3s/postgres-operator.yaml
install cert-manager cert-manager "$DSC_CHART" k3s/cert-manager.yaml

log "Cluster infrastructure"
kubectl apply --recursive -f k3s/infra
kubectl -n cert-manager wait --for=condition=Ready certificate/selfsigned-ca --timeout=5m
kubectl wait --for=condition=Ready clusterissuer/selfsigned-issuer --timeout=5m
kubectl -n kube-system rollout status deployment/coredns --timeout=5m
kubectl -n infra rollout status deployment/traefik --timeout=5m
kubectl -n infra rollout status deployment/squid-proxy --timeout=5m
if [ "${GAIA_X_INFRA:-false}" = "true" ]; then
    kubectl apply --recursive -f k3s/gaia-x-infra
fi

log "Additional resources of the participants"
kubectl apply -f k3s/trust-anchor -f k3s/provider -f k3s/consumer

if [ -n "${PROMETHEUS_VALUES:-}" ]; then
    log "Monitoring"
    install prometheus monitoring "$DSC_CHART" "$PROMETHEUS_VALUES"
fi

log "Participants"
install trust-anchor trust-anchor "$TRUST_ANCHOR_CHART" "${TRUST_ANCHOR_VALUES:-}" --wait-for-jobs
install consent-authority trust-anchor "$DSC_CHART" "${CONSENT_AUTHORITY_VALUES:-}" --wait-for-jobs
install provider provider "$DSC_CHART" "${PROVIDER_VALUES:-}" --wait-for-jobs
install consumer consumer "$DSC_CHART" "${CONSUMER_VALUES:-}" --wait-for-jobs

log "Deployed releases"
"$HELM" list --all-namespaces
