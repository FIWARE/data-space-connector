#!/bin/sh
# Verify that the issuer signs credentials with ${DID}#${KEY_ALIAS}, and stop the bootstrap if
# it does not.
#
# This only CHECKS - it cannot fix it, and neither can anything else automated:
#
#  * keycloak.signingKey.did is rendered into the realm as the `kid` config of a java-keystore
#    KeyProvider component, and the chart applies the realm only through `start --import-realm`,
#    which Keycloak SKIPS for a realm that already exists. On a pre-existing realm the kid stays
#    whatever the first import set.
#  * It cannot be repaired through the Admin API either: `java-keystore` declares exactly
#    priority, enabled, active, algorithm, keystore, keystorePassword, keystoreType, keyAlias,
#    keyPassword and keyUse (GET /admin/serverinfo). `kid` is NOT a declared property, so a PUT
#    on the component answers 204 and silently drops it. The realm import is the only path that
#    writes component config verbatim.
#
# A fresh install gets this right on its own and the check just passes. An existing realm needs
# the one-off repair printed below. Aborting here is deliberate: inserting a credential that a
# counterparty cannot verify is worse than not inserting it.
set -e
# overridable so the scripts can be exercised outside the Job
SCRIPT_DIR="${SCRIPT_DIR:-/scripts}"
. "${SCRIPT_DIR}/lib.sh"

: "${KEYCLOAK_URL:?KEYCLOAK_URL is required}"
: "${KEYCLOAK_REALM:?KEYCLOAK_REALM is required}"
: "${KEY_ID:?KEY_ID is required}"

wait_for "keycloak realm ${KEYCLOAK_REALM}" "${KEYCLOAK_URL}/realms/${KEYCLOAK_REALM}/.well-known/openid-configuration"

CERTS=$(curl -fsS "${KEYCLOAK_URL}/realms/${KEYCLOAK_REALM}/protocol/openid-connect/certs")
if echo "${CERTS}" | grep -qF "\"${KEY_ID}\""; then
  echo "OK: realm ${KEYCLOAK_REALM} publishes a key with kid ${KEY_ID}"
  exit 0
fi

cat <<EOF
ERROR: realm '${KEYCLOAK_REALM}' does not publish a key with kid '${KEY_ID}'.

Credentials signed by this realm would carry a kid that counterparties cannot resolve in the
DID document, which publishes the signing key under that fragment only.

On a fresh install this means keycloak.signingKey.did is misconfigured - fix it in values.

On a realm that already exists, Keycloak skipped the import and the kid cannot be changed
through the Admin API. Repair it once, directly in the Keycloak database:

  update component_config set value = '${KEY_ID}'
   where name = 'kid'
     and component_id = (
       select c.id from component c
         join realm r on r.id = c.realm_id
        where r.name = '${KEYCLOAK_REALM}'
          and c.provider_id = 'java-keystore');

then restart Keycloak so it reloads the realm's key providers.
EOF
exit 1
