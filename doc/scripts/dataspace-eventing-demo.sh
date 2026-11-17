#!/usr/bin/env bash
# Drives the data space eventing demo of the local deployment (`mvn clean deploy -Peventing`).
#
#   remove  <did>                         remove a participant from the trust anchor's TIL
#   restore <did>                         register it again, without credentials
#   report  [provider|consumer]           the removed-participants report of a participant
#   health  [publisher|provider|consumer] the health details of a contract-management
#   publish <did>                         announce a removal at once, through the publisher
#
# The management port 9090 is cluster-internal (the Services expose 8080 only), so `report`,
# `health` and `publish` port-forward to the Deployment. Needs curl, jq and kubectl with
# KUBECONFIG pointing at the cluster (e.g. it/target/k3s.yaml), and python3 to pick a free port.
set -euo pipefail

readonly PROXY="localhost:8888"
readonly TIL_URL="https://til.127.0.0.1.nip.io"
readonly REMOVED_TYPE="org.fiware.dataspace.participant.removed"
readonly CM_DEPLOYMENT="contract-management"
readonly MANAGEMENT_PORT=9090
readonly PUBLISHER_NAMESPACE="trust-anchor"
readonly PROVIDER_NAMESPACE="provider"
readonly CONSUMER_NAMESPACE="consumer"
readonly FORWARD_STARTUP_ATTEMPTS=50
readonly FORWARD_STARTUP_INTERVAL_S=0.2

usage() {
  sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
  exit 1
}

namespace_of() {
  case "$1" in
    publisher) echo "$PUBLISHER_NAMESPACE" ;;
    provider) echo "$PROVIDER_NAMESPACE" ;;
    consumer) echo "$CONSUMER_NAMESPACE" ;;
    *) echo "unknown participant: $1" >&2; exit 1 ;;
  esac
}

# Sends `curl <args...>` to path $2 of the management port of the contract-management in
# namespace $1, through a port-forward that lives as long as the request.
management() {
  local namespace=$1 path=$2 local_port pid
  shift 2
  local_port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
  kubectl port-forward -n "$namespace" "deploy/$CM_DEPLOYMENT" "$local_port:$MANAGEMENT_PORT" >/dev/null &
  pid=$!
  trap 'kill "$pid" 2>/dev/null || true' RETURN
  for _ in $(seq "$FORWARD_STARTUP_ATTEMPTS"); do
    if curl -s -o /dev/null "http://localhost:$local_port/health"; then
      break
    fi
    sleep "$FORWARD_STARTUP_INTERVAL_S"
  done
  curl -s "$@" "http://localhost:$local_port$path"
}

[ $# -ge 1 ] || usage
command=$1
shift

case "$command" in
  remove)
    [ $# -eq 1 ] || usage
    date -u +"removing $1 at %Y-%m-%dT%H:%M:%SZ"
    curl -s -k -x "$PROXY" -X DELETE -w '%{http_code}\n' "$TIL_URL/issuer/$1"
    ;;
  restore)
    [ $# -eq 1 ] || usage
    curl -s -k -x "$PROXY" -X POST -w '%{http_code}\n' "$TIL_URL/issuer" \
      -H 'Content-Type: application/json' \
      -d "$(jq -cn --arg did "$1" '{did: $did, credentials: []}')"
    ;;
  report)
    management "$(namespace_of "${1:-provider}")" /participants-removed | jq .
    ;;
  health)
    management "$(namespace_of "${1:-publisher}")" /health | jq .
    ;;
  publish)
    [ $# -eq 1 ] || usage
    management "$PUBLISHER_NAMESPACE" /events -X POST -H 'Content-Type: application/json' \
      -d "$(jq -cn --arg type "$REMOVED_TYPE" --arg subject "$1" '{type: $type, subject: $subject}')" | jq .
    ;;
  *)
    usage
    ;;
esac
