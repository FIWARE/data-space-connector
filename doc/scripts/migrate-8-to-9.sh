#!/usr/bin/env bash
# migrate-8-to-9.sh
#
# Migrates a data-space-connector values file from 8.5.x to 9.0.x.
# Performs automated renames/moves where possible and prints warnings
# for changes that require manual review.
#
# Requires: yq v4  (https://github.com/mikefarah/yq)
#
# Usage:
#   ./migrate-8-to-9.sh -f my-values.yaml [-o my-values-9.yaml]

set -euo pipefail

SCRIPT_NAME="$(basename "$0")"
WARNINGS=()
INPUT_FILE=""
OUTPUT_FILE=""

# ── portable sed -i ───────────────────────────────────────────────────────────
sedi() {
  if sed --version 2>/dev/null | grep -q GNU; then
    sed -i "$@"
  else
    sed -i '' "$@"
  fi
}

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME -f <input-values.yaml> [-o <output-values.yaml>]

  -f    Input values file (8.5.x format)
  -o    Output values file (9.0.x format). Defaults to <input>-migrated.yaml

Requires yq v4: https://github.com/mikefarah/yq
EOF
  exit 1
}

warn() { WARNINGS+=("$*"); echo "  [WARN] $*"; }
info() { echo "  [OK]   $*"; }
skip() { echo "  [SKIP] $*"; }

while getopts "f:o:h" opt; do
  case $opt in
    f) INPUT_FILE="$OPTARG" ;;
    o) OUTPUT_FILE="$OPTARG" ;;
    *) usage ;;
  esac
done

[[ -z "$INPUT_FILE" ]] && usage
[[ ! -f "$INPUT_FILE" ]] && { echo "ERROR: File not found: $INPUT_FILE"; exit 1; }

if [[ -z "$OUTPUT_FILE" ]]; then
  OUTPUT_FILE="${INPUT_FILE%.yaml}-migrated.yaml"
fi

# Check for yq v4
if ! command -v yq &>/dev/null; then
  echo "ERROR: yq v4 is required. Install from https://github.com/mikefarah/yq"
  exit 1
fi

YQ_MAJOR=$(yq --version 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 | cut -d. -f1)
if [[ "${YQ_MAJOR:-0}" -lt 4 ]]; then
  echo "ERROR: yq v4 is required (found v${YQ_MAJOR:-?})"
  exit 1
fi

cp "$INPUT_FILE" "$OUTPUT_FILE"
echo "Migrating '$INPUT_FILE' → '$OUTPUT_FILE'"
echo ""

# ── helper: check if a key exists (handles hyphenated keys) ──────────────────
has_key() {
  local expr="$1"
  [[ "$(yq "$expr" "$OUTPUT_FILE")" != "null" ]]
}

# ─────────────────────────────────────────────────────────────────────────────
echo "=== 1. Removing deprecated top-level keys ==="

for key in authentication dataplane mysql postgresql postgis opa tpp; do
  if has_key ".$key"; then
    yq -i "del(.$key)" "$OUTPUT_FILE"
    info "Deleted .$key"
  else
    skip ".$key not present"
  fi
done

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== 2. Moving IAM components under decentralizedIam ==="

# vcAuthentication components
for comp in vcverifier credentials-config-service trusted-issuers-list dss; do
  if has_key ".\"$comp\""; then
    yq -i ".decentralizedIam.vcAuthentication.\"$comp\" = .\"$comp\"" "$OUTPUT_FILE"
    yq -i "del(.\"$comp\")"                                            "$OUTPUT_FILE"
    info "Moved .$comp → .decentralizedIam.vcAuthentication.$comp"
  else
    skip ".$comp not present"
  fi
done

# odrlAuthorization components
for comp in odrl-pap; do
  if has_key ".\"$comp\""; then
    yq -i ".decentralizedIam.odrlAuthorization.\"$comp\" = .\"$comp\"" "$OUTPUT_FILE"
    yq -i "del(.\"$comp\")"                                             "$OUTPUT_FILE"
    info "Moved .$comp → .decentralizedIam.odrlAuthorization.$comp"
  else
    skip ".$comp not present"
  fi
done

# apisix: chart changed from Bitnami to Apache — move as reference but warn
if has_key ".apisix"; then
  yq -i ".decentralizedIam.odrlAuthorization.apisix = .apisix" "$OUTPUT_FILE"
  yq -i "del(.apisix)"                                          "$OUTPUT_FILE"
  warn "apisix: Bitnami chart → Apache APISIX chart. The value structure changed completely."
  warn "        Old values copied to .decentralizedIam.odrlAuthorization.apisix for reference."
  warn "        Rewrite them using the Apache APISIX Helm chart format before deploying."
else
  skip ".apisix not present"
fi

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== 3. Renaming did-helper → did ==="

if has_key '."did-helper"'; then
  yq -i '.did = ."did-helper"' "$OUTPUT_FILE"
  yq -i 'del(."did-helper")'   "$OUTPUT_FILE"
  info "Renamed .did-helper → .did"
else
  skip ".did-helper not present"
fi

if has_key ".didJson"; then
  yq -i "del(.didJson)" "$OUTPUT_FILE"
  warn "didJson removed. Configure DID serving via the .did subchart instead."
else
  skip ".didJson not present"
fi

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== 4. credentialType → credentialTypes ==="

# Could appear in multiple locations; handle both old top-level and nested
if grep -q 'credentialType:' "$OUTPUT_FILE"; then
  OLD_VAL=$(grep 'credentialType:' "$OUTPUT_FILE" | head -1 | awk -F': ' '{print $2}' | tr -d '"' | xargs)
  sedi 's/credentialType: \(.*\)/credentialTypes:\n  - \1/' "$OUTPUT_FILE"
  info "Converted credentialType: \"$OLD_VAL\" → credentialTypes: [\"$OLD_VAL\"]"
else
  skip "credentialType not present"
fi

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== 5. Replacing \${DSC_REALM} references ==="

if grep -q '\${DSC_REALM}' "$OUTPUT_FILE"; then
  sedi 's/\${DSC_REALM}/{{ .Values.keycloak.realm.name }}/g' "$OUTPUT_FILE"
  info "Replaced all \${DSC_REALM} → {{ .Values.keycloak.realm.name }}"
else
  skip "\${DSC_REALM} not present"
fi

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== 6. Keycloak keystore configuration ==="

if grep -qE 'STORE_PASS|kc-keystore' "$OUTPUT_FILE"; then
  warn "STORE_PASS / kc-keystore detected. The STORE_PASS env var injection has been removed."
  warn "  Configure keystore credentials via .elsi or .keycloak.signingKey."
  warn "  Use \${STORE_PASS} as a placeholder in values + keycloak.extraEnvVars for the actual secret."
else
  skip "No STORE_PASS / kc-keystore references found"
fi

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== 7. Keycloak realm: deprecated fields ==="

if has_key ".keycloak.realm.realmRoles"; then
  warn "keycloak.realm.realmRoles is no longer used."
  warn "  Chart-managed defaults are in .keycloak.realm.defaultRealmRoles."
  warn "  Add your custom roles to .keycloak.realm.extraRealmRoles and remove .keycloak.realm.realmRoles."
else
  skip ".keycloak.realm.realmRoles not present"
fi

# Warn about legacy raw JSON string fields (string value starting with " or {)
for field in attributes clientRoles users clients clientScopes; do
  VAL=$(yq ".keycloak.realm.$field" "$OUTPUT_FILE" 2>/dev/null || true)
  if [[ "$VAL" != "null" && "$VAL" != "" ]] && echo "$VAL" | grep -qE '^[\"\{]'; then
    warn "keycloak.realm.$field looks like a legacy raw JSON string. Consider converting to native YAML."
  fi
done

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== 8. Database secrets ==="

if grep -qE 'authentication-database-secret|mysql-root-password' "$OUTPUT_FILE"; then
  warn "Old TIL/CCS MySQL secret 'authentication-database-secret' detected."
  warn "  New secret: postgres.postgres.credentials.postgresql.acid.zalan.do / key: password"
  warn "  Also update database host (mysql → postgres) and dialect (MYSQL → POSTGRES)."
fi

if grep -qE '"database-secret"|database-secret' "$OUTPUT_FILE"; then
  warn "Old secret 'database-secret' detected."
  warn "  New secret: postgres.postgres.credentials.postgresql.acid.zalan.do / key: password"
fi

if grep -q 'postgres-admin-password' "$OUTPUT_FILE"; then
  warn "Old key 'postgres-admin-password' detected."
  warn "  The Zalando operator generates one secret per user."
  warn "  Use: postgres.postgres.credentials.postgresql.acid.zalan.do / password"
fi

if grep -q 'authentication-mysql' "$OUTPUT_FILE"; then
  sedi 's/authentication-mysql/postgres/g' "$OUTPUT_FILE"
  info "Replaced database host 'authentication-mysql' → 'postgres'"
fi

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "=== 9. ODRL-PAP database name ==="

if grep -q "name: pap$" "$OUTPUT_FILE" || grep -q "name: 'pap'" "$OUTPUT_FILE"; then
  warn "ODRL-PAP database was renamed from 'pap' to 'papdb'."
  warn "  Migrate existing policy data before applying this change."
  warn "  Then update the database name in .decentralizedIam.odrlAuthorization.odrl-pap."
fi

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "================================================================"
echo "Output: $OUTPUT_FILE"
echo ""

if [[ ${#WARNINGS[@]} -gt 0 ]]; then
  echo "Manual actions required — ${#WARNINGS[@]} warning(s):"
  echo ""
  for w in "${WARNINGS[@]}"; do
    echo "  ⚠  $w"
  done
  echo ""
  echo "Review the output file and address all warnings before deploying."
else
  echo "No manual actions required. Review the output file before deploying."
fi

echo "================================================================"
