#!/usr/bin/env bash
#
# dsp-consume.sh — interactive walk-through of the EDC/DSP consumer flow:
#   catalog -> contract negotiation -> transfer process -> EDR (endpoint + token)
#
# It shows the provider catalog, lets you pick a dataset/offer, negotiates the
# contract (polling until FINALIZED), asks whether to start the transfer, polls
# it until STARTED, and finally prints the data ENDPOINT and ACCESS TOKEN.
#
# Usage:
#   dsp-consume.sh <dsp-endpoint> <management-api> <counter-party-id>
#
#   <dsp-endpoint>      provider DSP address (counterPartyAddress),
#                       e.g. https://dcp-producer.example.org/api/dsp/2025-1
#   <management-api>    local consumer EDC management base,
#                       e.g. http://localhost:8085/api/v1/management/v3
#   <counter-party-id>  provider DID, e.g. did:web:did-producer.example.org:did
#
# Env overrides (optional):
#   TRANSFER_TYPE   transfer type            (default: HttpData-PULL)
#   PROTOCOL        DSP protocol string      (default: dataspace-protocol-http:2025-1)
#   POLL_TIMEOUT    max seconds to poll      (default: 150)
#   POLL_INTERVAL   seconds between polls    (default: 3)
#
# Only the final "ENDPOINT" and "ACCESS_TOKEN" lines go to stdout; everything
# else (prompts, progress) goes to stderr, so the script stays pipe-friendly.
#
set -euo pipefail

TRANSFER_TYPE="${TRANSFER_TYPE:-HttpData-PULL}"
PROTOCOL="${PROTOCOL:-dataspace-protocol-http:2025-1}"
POLL_TIMEOUT="${POLL_TIMEOUT:-150}"
POLL_INTERVAL="${POLL_INTERVAL:-3}"
EDC_CTX='["https://w3id.org/edc/connector/management/v0.0.1"]'

# ---- output helpers (everything to stderr) ----------------------------------
if [ -t 2 ]; then C_B=$'\033[1m'; C_G=$'\033[32m'; C_Y=$'\033[33m'; C_R=$'\033[31m'; C_0=$'\033[0m'
else C_B=""; C_G=""; C_Y=""; C_R=""; C_0=""; fi
log()  { printf '%s\n' "$*" >&2; }
step() { printf '\n%s== %s ==%s\n' "$C_B" "$*" "$C_0" >&2; }
ok()   { printf '%s%s%s\n' "$C_G" "$*" "$C_0" >&2; }
warn() { printf '%s%s%s\n' "$C_Y" "$*" "$C_0" >&2; }
die()  { printf '%sERROR: %s%s\n' "$C_R" "$*" "$C_0" >&2; exit 1; }

usage() { sed -n '3,28p' "$0" >&2; exit "${1:-0}"; }

case "${1:-}" in -h|--help) usage 0;; esac
[ "$#" -eq 3 ] || { warn "Expected 3 arguments, got $#."; usage 1; }

command -v curl >/dev/null 2>&1 || die "curl is required."
command -v jq   >/dev/null 2>&1 || die "jq is required."

DSP_ENDPOINT="${1%/}"
MGMT="${2%/}"
COUNTER_PARTY_ID="$3"

# ---- HTTP helpers -----------------------------------------------------------
# api <METHOD> <path> [json-body] -> prints response body to stdout, dies on non-2xx
api() {
  local method="$1" path="$2" body="${3:-}" resp code out
  if [ -n "$body" ]; then
    resp=$(curl -sS -X "$method" "${MGMT}${path}" \
      -H 'Accept: */*' -H 'Content-Type: application/json' \
      --data-raw "$body" -w $'\n%{http_code}') || die "curl failed: $method $path"
  else
    resp=$(curl -sS -X "$method" "${MGMT}${path}" \
      -H 'Accept: */*' -w $'\n%{http_code}') || die "curl failed: $method $path"
  fi
  code="${resp##*$'\n'}"; out="${resp%$'\n'*}"
  if [ "$code" -lt 200 ] || [ "$code" -ge 300 ]; then
    log "$out"; die "$method $path -> HTTP $code"
  fi
  printf '%s' "$out"
}

# ---- Step 1: catalog --------------------------------------------------------
step "1/4  Requesting catalog from ${DSP_ENDPOINT}"
CAT_REQ=$(jq -n --argjson ctx "$EDC_CTX" --arg p "$PROTOCOL" \
  --arg cpid "$COUNTER_PARTY_ID" --arg cpa "$DSP_ENDPOINT" '{
    "@context":$ctx, "@type":"CatalogRequestMessage", protocol:$p,
    counterPartyId:$cpid, counterPartyAddress:$cpa, querySpec:{}
  }')
CATALOG=$(api POST /catalog/request "$CAT_REQ")

# datasets (handle dcat:dataset | dataset, object or array)
DATASETS=$(printf '%s' "$CATALOG" | jq -c '
  (.["dcat:dataset"] // .dataset // []) | if type=="array" then . else [.] end')
N=$(printf '%s' "$DATASETS" | jq 'length')
[ "$N" -gt 0 ] || die "Catalog has 0 datasets. Common cause: offerings/specs without an externalId (the catalog drops them)."

ok "Found ${N} dataset(s):"
log ""
printf '%s' "$DATASETS" | jq -r '
  to_entries[] |
  "[\(.key)] \(.value["@id"])",
  "     type: \(.value["@type"] // "Dataset")",
  ( .value | to_entries[]
      | select(.key | test("^@|hasPolicy|distribution") | not)
      | select(.value | type=="string" or type=="number")
      | "     \(.key): \(.value)" ),
  ( (.value.distribution // .value["dcat:distribution"] // [])
      | (if type=="array" then . else [.] end)[]
      | "     endpoint[\(.format // "?")]: \(.accessService.endpointURL // "?")"
        + ((.accessService.endpointDescription // "") | if .=="" then "" else "  (\(.))" end) ),
  ( (.value["odrl:hasPolicy"] // .value.hasPolicy // [])
      | (if type=="array" then . else [.] end)
      | "     offers: \(length)" ),
  ( (.value["odrl:hasPolicy"] // .value.hasPolicy // [])
      | (if type=="array" then . else [.] end)[]
      | "       - \(.["@id"])  [perm: "
        + ( ((.permission // []) | (if type=="array" then . else [.] end))
            | map( ((.action // .["odrl:action"] // "?") | if type=="object" then (.["@id"]//.id//"?") else . end) as $a
                   | ((.constraint // .["odrl:constraint"] // []) | (if type=="array" then . else [.] end)
                      | map("\(.leftOperand) \(.operator) \(.rightOperand)") | join(", ")) as $c
                   | if $c=="" then $a else "\($a) {\($c)}" end )
            | join("; ") ) + "]" ),
  ""
' >&2

# select dataset (or 'v <idx>' to dump its full JSON)
DS_IDX=""
while :; do
  read -r -p "Select dataset index [0-$((N-1))], or 'v <idx>' to view full JSON: " sel <&0
  if [[ "$sel" =~ ^[vV?][[:space:]]*([0-9]+)$ ]]; then
    vi="${BASH_REMATCH[1]}"
    if [ "$vi" -ge 0 ] && [ "$vi" -lt "$N" ]; then
      printf '%s' "$DATASETS" | jq --argjson i "$vi" '.[$i]' >&2
    else warn "Index out of range (0-$((N-1)))."; fi
    continue
  fi
  if [[ "$sel" =~ ^[0-9]+$ ]] && [ "$sel" -ge 0 ] && [ "$sel" -lt "$N" ]; then
    DS_IDX="$sel"; break
  fi
  warn "Enter a number 0-$((N-1)), or 'v <idx>' to inspect."
done
DATASET=$(printf '%s' "$DATASETS" | jq -c --argjson i "$DS_IDX" '.[$i]')
ASSET_ID=$(printf '%s' "$DATASET" | jq -r '.["@id"]')

# offers within the dataset
OFFERS=$(printf '%s' "$DATASET" | jq -c '
  (.["odrl:hasPolicy"] // .hasPolicy // []) | if type=="array" then . else [.] end')
NO=$(printf '%s' "$OFFERS" | jq 'length')
[ "$NO" -gt 0 ] || die "Selected dataset has no offers (odrl:hasPolicy)."
if [ "$NO" -eq 1 ]; then
  OF_IDX=0
else
  ok "Dataset has ${NO} offers:"
  printf '%s' "$OFFERS" | jq -r 'to_entries[] | "  [\(.key)] \(.value["@id"])"' >&2
  OF_IDX=""
  while :; do
    read -r -p "Select offer index [0-$((NO-1))], or 'v <idx>' to view full JSON: " sel <&0
    if [[ "$sel" =~ ^[vV?][[:space:]]*([0-9]+)$ ]]; then
      vi="${BASH_REMATCH[1]}"
      if [ "$vi" -ge 0 ] && [ "$vi" -lt "$NO" ]; then
        printf '%s' "$OFFERS" | jq --argjson i "$vi" '.[$i]' >&2
      else warn "Index out of range (0-$((NO-1)))."; fi
      continue
    fi
    if [[ "$sel" =~ ^[0-9]+$ ]] && [ "$sel" -ge 0 ] && [ "$sel" -lt "$NO" ]; then
      OF_IDX="$sel"; break
    fi
    warn "Enter a number 0-$((NO-1)), or 'v <idx>' to inspect."
  done
fi
OFFER=$(printf '%s' "$OFFERS" | jq -c --argjson i "$OF_IDX" '.[$i]')
OFFER_ID=$(printf '%s' "$OFFER" | jq -r '.["@id"]')
log "  asset (target): ${ASSET_ID}"
log "  offer  (@id):   ${OFFER_ID}"

# ---- Step 2: contract negotiation -------------------------------------------
step "2/4  Starting contract negotiation"
# Build the offer policy from the catalog offer verbatim, forcing the required fields.
POLICY=$(printf '%s' "$OFFER" | jq -c --arg oid "$OFFER_ID" --arg asset "$ASSET_ID" --arg assigner "$COUNTER_PARTY_ID" '
  .["@context"]="http://www.w3.org/ns/odrl.jsonld"
  | .["@type"]="Offer"
  | .["@id"]=$oid
  | .target=$asset
  | .assigner=(.assigner // $assigner)')
CN_REQ=$(jq -n --argjson ctx "$EDC_CTX" --arg p "$PROTOCOL" \
  --arg cpa "$DSP_ENDPOINT" --arg cpid "$COUNTER_PARTY_ID" --argjson policy "$POLICY" '{
    "@context":$ctx, "@type":"ContractRequest",
    counterPartyAddress:$cpa, counterPartyId:$cpid, protocol:$p, policy:$policy
  }')
CN_ID=$(api POST /contractnegotiations "$CN_REQ" | jq -r '.["@id"]')
[ -n "$CN_ID" ] && [ "$CN_ID" != "null" ] || die "No contract negotiation @id returned."
log "  negotiation id: ${CN_ID}"

AGREEMENT_ID=""
deadline=$(( $(date +%s) + POLL_TIMEOUT )); last=""
while :; do
  cn=$(api GET "/contractnegotiations/${CN_ID}")
  state=$(printf '%s' "$cn" | jq -r '.state')
  [ "$state" != "$last" ] && { log "  state: ${state}"; last="$state"; }
  case "$state" in
    FINALIZED) AGREEMENT_ID=$(printf '%s' "$cn" | jq -r '.contractAgreementId'); break;;
    TERMINATED) die "Negotiation TERMINATED: $(printf '%s' "$cn" | jq -r '.errorDetail // "no detail"')";;
  esac
  [ "$(date +%s)" -lt "$deadline" ] || die "Timed out (${POLL_TIMEOUT}s) waiting for FINALIZED (last: ${state})."
  sleep "$POLL_INTERVAL"
done
ok "Negotiation FINALIZED. Agreement: ${AGREEMENT_ID}"

# ---- Step 3: transfer process (interactive gate) ----------------------------
read -r -p "Start the transfer process now? [y/N]: " ANS <&0
case "$ANS" in
  y|Y|yes|YES|s|S|si|SI) ;;
  *) warn "Stopped after negotiation. Agreement id: ${AGREEMENT_ID}"; exit 0;;
esac

step "3/4  Starting transfer process"
TP_REQ=$(jq -n --argjson ctx "$EDC_CTX" --arg p "$PROTOCOL" --arg tt "$TRANSFER_TYPE" \
  --arg asset "$ASSET_ID" --arg cpid "$COUNTER_PARTY_ID" --arg cpa "$DSP_ENDPOINT" --arg aid "$AGREEMENT_ID" '{
    "@context":$ctx, assetId:$asset, counterPartyId:$cpid, counterPartyAddress:$cpa,
    connectorId:$cpid, contractId:$aid, protocol:$p, transferType:$tt
  }')
TP_ID=$(api POST /transferprocesses "$TP_REQ" | jq -r '.["@id"]')
[ -n "$TP_ID" ] && [ "$TP_ID" != "null" ] || die "No transfer process @id returned."
log "  transfer id: ${TP_ID}"

deadline=$(( $(date +%s) + POLL_TIMEOUT )); last=""
while :; do
  tp=$(api GET "/transferprocesses/${TP_ID}")
  state=$(printf '%s' "$tp" | jq -r '.state')
  [ "$state" != "$last" ] && { log "  state: ${state}"; last="$state"; }
  case "$state" in
    STARTED) break;;
    TERMINATED) die "Transfer TERMINATED: $(printf '%s' "$tp" | jq -r '.errorDetail // "no detail"')";;
  esac
  [ "$(date +%s)" -lt "$deadline" ] || die "Timed out (${POLL_TIMEOUT}s) waiting for STARTED (last: ${state})."
  sleep "$POLL_INTERVAL"
done
ok "Transfer STARTED."

# ---- Step 4: EDR (endpoint + token) -----------------------------------------
step "4/4  Fetching EDR data address"
EDR=$(api GET "/edrs/${TP_ID}/dataaddress")
ENDPOINT=$(printf '%s' "$EDR" | jq -r '.endpoint // empty')
ACCESS_TOKEN=$(printf '%s' "$EDR" | jq -r '.token // .authorization // empty')
[ -n "$ENDPOINT" ] || die "EDR has no endpoint."
[ -n "$ACCESS_TOKEN" ] || die "EDR has no token."

# Clear, spaced summary to stderr ...
LINE="────────────────────────────────────────────────────────────"
{
  printf '\n%s%s%s\n' "$C_G" "$LINE" "$C_0"
  printf '%s ACCESS GRANTED — use the endpoint + token below%s\n' "$C_B" "$C_0"
  printf '%s%s%s\n\n' "$C_G" "$LINE" "$C_0"
  printf '%sEndpoint%s\n  %s\n\n' "$C_B" "$C_0" "$ENDPOINT"
  printf '%sAccess token%s\n  %s\n\n' "$C_B" "$C_0" "$ACCESS_TOKEN"
  printf '%sExample request%s\n  curl -H "Authorization: Bearer <token>" \\\n       "%s/ngsi-ld/v1/entities?type=<YourType>"\n' "$C_B" "$C_0" "$ENDPOINT"
  printf '%s%s%s\n\n' "$C_G" "$LINE" "$C_0"
} >&2
# ... and machine-readable lines to stdout (for piping/eval):
printf 'ENDPOINT=%s\n' "$ENDPOINT"
printf 'ACCESS_TOKEN=%s\n' "$ACCESS_TOKEN"
