#! /bin/bash
# Render the reference profiles (charts/*/examples) as a user installs them, without a cluster.
# helm validates the values against the charts' schemas and runs their `fail` checks, so a key
# that is misspelled, removed or of the wrong type in a profile fails here. The placeholders are
# rendered as they are.
#
# Requires helm on the PATH. Fetches the charts' dependencies first.

set -euo pipefail

cd "$(dirname "$0")/../.."
DSC=charts/data-space-connector
TA=charts/trust-anchor

helm dependency update "$DSC" >/dev/null
helm dependency update "$TA" >/dev/null

render() {
    local name=$1 chart=$2
    shift 2
    local args=()
    for values in "$@"; do
        args+=(-f "$values")
    done
    echo "rendering ${name}"
    helm template "$name" "$chart" --namespace "$name" "${args[@]}" >/dev/null
}

render provider "$DSC" "$DSC/examples/provider.yaml"
render provider "$DSC" "$DSC/examples/provider.yaml" "$DSC/examples/provider-marketplace.yaml"
render consumer "$DSC" "$DSC/examples/consumer.yaml"
render trust-anchor "$TA" "$TA/examples/trust-anchor.yaml"
echo "All reference profiles render."
