# Shared helpers, sourced by the other scripts in this directory.
# Wait until a URL answers, then return. Fails the step rather than letting a later call
# produce a confusing error against a component that is not up yet.
# Retries default to 30 x 5s = 150s. That is ample for a vault the chart unseals itself, and
# nowhere near enough for one unsealed by hand: raise identityhub.bootstrap.waitRetries, or run
# the Job after the key holders have opened it.
wait_for() {
  _name="$1"; _url="$2"; _tries="${3:-${WAIT_RETRIES:-30}}"
  echo "Waiting for ${_name}..."
  _i=0
  until curl -fsS "${_url}" >/dev/null 2>&1; do
    _i=$((_i + 1))
    if [ "${_i}" -gt "${_tries}" ]; then echo "timeout waiting for ${_name} at ${_url}"; exit 1; fi
    sleep 5
  done
}

# base64url without padding, the encoding the Identity API expects for the {participantId}
# path parameter. A raw DID there fails as `Illegal base64 character 3a` behind a bare 400.
b64url() {
  printf '%s' "$1" | base64 | tr '+/' '-_' | tr -d '=\n'
}

# Write a value into vault's KV v2 store under the given alias. The value is stored as a JSON
# string under `content`, which is what EDC's hashicorp vault client reads.
# Does an alias resolve to anything? Used to tell a secret that was never provisioned from one
# that is already in place, so a repair step only runs when it is actually needed.
vault_has() {
  _code=$(curl -s -o /dev/null -w '%{http_code}' -X GET "${VAULT_ADDR}/v1/secret/data/$1" \
    -H "X-Vault-Token: ${VAULT_TOKEN}")
  [ "${_code}" = "200" ]
}

vault_put() {
  _alias="$1"; _value="$2"
  _body=$(printf '{"data":{"content":%s}}' "$(printf '%s' "${_value}" | sed 's/\\/\\\\/g; s/"/\\"/g; s/^/"/; s/$/"/')")
  _code=$(curl -s -o /tmp/vault_resp.json -w "%{http_code}" -X POST "${VAULT_ADDR}/v1/secret/data/${_alias}" \
    -H "X-Vault-Token: ${VAULT_TOKEN}" \
    -d "${_body}")
  echo "vault write ${_alias} -> HTTP ${_code}"
  if [ "${_code}" -lt 200 ] || [ "${_code}" -ge 300 ]; then
    cat /tmp/vault_resp.json
    exit 1
  fi
}

# The identityhub authenticates the Identity API by parsing the presented x-api-key as
# base64(participantContextId).secret, looking that participant up, reading the alias in its
# participant_context.api_token_alias column and comparing the WHOLE presented token against
# the value stored in vault under that alias.
superuser_token() {
  printf '%s.%s' "$(printf '%s' "${SUPERUSER_ID}" | base64 | tr -d '\n')" "${API_KEY}"
}
