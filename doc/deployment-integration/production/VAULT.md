# Running the EDC identity material in production

The Dataspace-Protocol part of the connector keeps the participant's identity in
Vault: the signing key the IdentityHub's Secure Token Service signs with, the
IdentityHub's own api key, the client secret shared with the connector's oauth
client, and the RSA key `fdsc-edc` generates for itself to sign transfer EDRs.

By default the chart deploys Vault in **dev mode**, which stores all of that in
memory. That is deliberate for local deployments and for the integration tests,
and it is not usable in production: every restart of the Vault pod drops the lot.
Note when it comes back — the bootstrap Job runs on install and upgrade, not on
restart, so a Vault that restarts at 3am leaves the DCP lane broken until somebody
deploys again.

There are three postures, and it is worth being blunt about which is which:

| Posture | Survives a restart | Starts unattended | Keys outside the cluster |
|---|---|---|---|
| `dev` (default) | no | yes | n/a |
| `unattended` with `autoUnseal: false` | yes | **no** | **yes** |
| `unattended` with `autoUnseal: true` | yes | yes | no |

**Only the middle row is a production posture.** A Vault that unseals itself has to
keep its unseal keys within reach, which here means a Secret in the same namespace;
that is a convenience for demo and staging, not a security property. Production
means people: several key holders in the operating organisation, each with a share,
presenting them on every start. It cannot be automated away, and this chart does not
pretend otherwise — it just makes the unattended option available, and names it for
what it is.

## Persistent Vault

A Vault with real storage comes back **sealed** after any restart and refuses every
request until it is unsealed, so persistence on its own is not enough. Something has
to present the unseal keys — either the chart, or a person.

The values below are the unattended variant, which is what a demo or staging
deployment wants. For production, keep everything except the last block and read
[Unsealing in production](#unsealing-in-production).

```yaml
vault:
  enabled: true
  server:
    # dev mode is in-memory and auto-unsealed; both have to go
    dev:
      enabled: false
    standalone:
      enabled: true
    dataStorage:
      enabled: true
      size: 2Gi
      # storageClass: <your class>
  hashicorp:
    url: "http://{{ .Release.Name }}-vault:8200"
    # keeps the token out of the ConfigMaps, see below
    existingSecret: vault-token
    existingSecretTokenKey: token
  unattended:
    enabled: true
    # 1/1 only makes sense together with autoUnseal: splitting the master key and
    # then leaving every share in one Secret buys nothing
    secretShares: 1
    secretThreshold: 1
    autoUnseal: true
    secretName: vault-unseal-keys
```

`vault.unattended.enabled` refuses to render next to `vault.server.dev.enabled`,
rather than quietly doing nothing.

### What auto-unsealing costs

The unseal keys and the initial root token are written to a Kubernetes Secret in
the release namespace. That is what lets an unattended restart recover on its own,
and it means the cluster's Secret store is as trusted as the Vault it opens: anyone
who can read Secrets in that namespace can open the Vault. No cloud KMS is involved
and none is required — which is exactly why this is not a production posture.

## Unsealing in production

Production means the unseal keys never live in the cluster. The operating
organisation nominates key holders, the master key is split between them, and a
quorum of them presents their shares every time Vault starts. There is no way around
the manual step; that is the point of it.

Values: everything from the block above, with the last part replaced by

```yaml
  unattended:
    enabled: true
    autoUnseal: false
    # split across five holders, three of whom have to show up
    secretShares: 5
    secretThreshold: 3
```

With `autoUnseal: false` the chart deploys no unsealer at all: no Deployment, no
ServiceAccount, no Secret holding keys. Vault is yours to initialise and open.

**Once, at install.** Run this with the key holders present, and distribute the
output there and then — it is the only time Vault ever shows it:

```shell
kubectl exec -n <ns> <release>-vault-0 -- vault operator init -key-shares=5 -key-threshold=3
```

Each holder keeps one unseal key. Store the root token separately (a password
manager, an HSM, an envelope in a safe): it is needed for the steps below and for
recovery, and for nothing else. Do not keep any of it in the cluster.

**On every start**, including every pod restart, upgrade and node drain. Three
different holders run this, each with their own key:

```shell
kubectl exec -it -n <ns> <release>-vault-0 -- vault operator unseal
kubectl exec -n <ns> <release>-vault-0 -- vault status | grep Sealed   # false when the quorum is met
```

Until the quorum is reached the IdentityHub cannot sign anything and the DCP lane is
down. Plan the rota accordingly, and prefer draining Vault deliberately over letting
it be rescheduled at random — a `podDisruptionBudget` and a `nodeSelector` are worth
more here than they look.

**Order matters on install.** The bootstrap Job waits for the IdentityHub to report
ready, and the IdentityHub is not ready while Vault is sealed. It gives up after
`identityhub.bootstrap.waitRetries` polls five seconds apart — 150 seconds by default,
and with `backoffLimit` retries on top of that, still minutes rather than hours. A
manual unseal takes as long as it takes to get the key holders together, so either

* unseal Vault first and deploy afterwards, which is the simple path; or
* deploy, unseal, and re-run the deploy so the `post-upgrade` hook fires again; or
* raise `identityhub.bootstrap.waitRetries` to cover the window you actually expect.

Failing the Job is not destructive — every step in it is idempotent — but it does
leave the participant unprovisioned until it runs to completion.

**The application token** is not minted for you either when `autoUnseal: false`
(`provisionToken` has no unsealer to run in). Create the policy and the token by hand
with the root token, as described in [The Vault token](#the-vault-token) below, and
store it in the Secret named by `vault.hashicorp.existingSecret`.

Two things about the Secret:

* It is annotated `helm.sh/resource-policy: keep`, so it survives `helm uninstall`.
  Deleting it while the storage volume still exists leaves a Vault nobody can
  open.
* Initialisation happens exactly once, against an empty Vault. If the unsealer
  finds Vault uninitialised **and** the Secret already present, it refuses to
  overwrite it and says so: those keys do not match the Vault in front of it, and
  one of the two is stale.

### The Vault token

`vault.hashicorp.token` is rendered into ConfigMaps in clear, so anyone who can
read ConfigMaps in the namespace can read the credential that unlocks the
participant's signing key. `vault.hashicorp.existingSecret` keeps it out: the
IdentityHub receives it as `EDC_VAULT_HASHICORP_TOKEN` and the property is omitted
from the ConfigMap entirely, rather than left empty for EDC to resolve as an empty
token.

With `vault.unattended.provisionToken` (the default) there is nothing to do here:
the unsealer mints that token itself, right after it initialises Vault, and stores
it in the Secret named by `existingSecret`. It is the only component that ever sees
the root token, so it is the only one that can, and it removes the last manual step
from the install — the Secret cannot be created before Vault exists, which used to
leave the IdentityHub in `CreateContainerConfigError` in the middle of every deploy,
fresh installs included.

The token is periodic and scoped by an ACL policy named after
`vault.unattended.tokenPolicyName`:

```hcl
path "secret/data/*"     { capabilities = ["create", "read", "update"] }
path "secret/metadata/*" { capabilities = ["read", "delete"] }
```

`create` as well as `update` because writing an alias that does not exist yet is a
create, and the bootstrap Job is idempotent, so it rewrites aliases that do.
`secret/metadata/*` because that is where KV v2 keeps versions, and the only way
EDC's `Vault#deleteSecret()` removes anything — keypair revocation needs it.

> :warning: **Do not attach `default` and nothing else.** Vault's `default` policy
> grants token self-management and no access to `secret/` at all, and Vault is
> deny-by-default: every read and write returns `403`. A token like that renews
> itself happily and fails the first `vault write` of the bootstrap Job.

**Renewal.** Each runtime renews its own token in process — `auth/token/lookup-self`
then `auth/token/renew-self`, rescheduled from the `lease_duration` Vault returns and
tuned by `edc.vault.hashicorp.token.{ttl,renew-buffer}`. Renewing extends the lease of
the same token: **the string never changes**, so nothing has to be propagated and no
pod has to restart. That is also why the token is periodic — a non-periodic one hits
its max TTL and stops being renewable.

**Rotation**, when the token really has to change (it lapsed, or it was revoked):

```shell
kubectl delete secret vault-token -n <ns>          # the unsealer mints a new one within a cycle
kubectl rollout restart deploy/identityhub -n <ns>
```

The restart is not optional. The token arrives as an environment variable, and env
vars are never updated in a running pod; EDC also reads it once at startup and holds
it immutable. That is why the unsealer creates the Secret and never overwrites it:
rewriting it in place would change nothing visible until some later restart, with the
old token possibly long dead by then.

If you set `autoUnseal: false`, or `provisionToken: false`, you do it by hand — and
then the policy has to exist first:

```shell
kubectl exec -n <ns> <release>-vault-0 -- sh -c \
  'vault policy write dsc-connector - <<EOF
path "secret/data/*"     { capabilities = ["create", "read", "update"] }
path "secret/metadata/*" { capabilities = ["read", "delete"] }
EOF'
kubectl exec -n <ns> <release>-vault-0 -- vault token create -period=768h -policy=dsc-connector -field=token
kubectl create secret generic vault-token -n <ns> --from-literal=token=<the token>
```

`fdsc-edc` takes the same two keys, and they have to be set separately: its subchart
cannot read the parent's values, so `vault.hashicorp` above does not reach the
connector.

```yaml
fdsc-edc:
  common:
    config:
      vault:
        hashicorp:
          existingSecret: vault-token
          existingSecretTokenKey: token
```

Set `existingSecret` rather than emptying `token`. Leaving the property behind with an
empty value is not equivalent: EDC resolves it as an empty token instead of falling
through to the environment variable, so the connector starts with no credentials for
Vault at all. With `existingSecret` the property is omitted from the properties file
entirely and the token arrives as `EDC_VAULT_HASHICORP_TOKEN`.

One lane does not need any of this. `edc.vault.hashicorp.*` is only read where the
`vault-hashicorp` extension is on the classpath, which is the DCP controlplane;
`controlplane-oid4vc` excludes it deliberately. A connector running only OID4VC can set
`vault.hashicorp.enabled: false` for that deployment and drop the block, token included.

## Participant bootstrap

`identityhub.bootstrap` provisions the participant context, writes the signing key
to Vault, repairs the IdentityHub super-user credential, provisions the STS client
secret and publishes the credential into the IdentityHub's credential store. It is
the procedure from [DSP_INTEGRATION.md](../../DSP_INTEGRATION.md), run in-cluster
and idempotently.

```yaml
identityhub:
  enabled: true
  superuser:
    # a Secret, not a plain value: the vc-operator reads the api key from one
    existingSecret: identityhub-secret
    existingSecretKeyKey: superuser
  endpoints:
    identity:
      authKeyAlias: super-user-apikey
  bootstrap:
    enabled: true
    did: did:web:my-connector.org
    credentialServiceUrl: https://my-connector.org
    identityKey:
      secretName: my-connector.org-tls
      algorithm: EC
    stsClientSecret:
      existingSecret: identityhub-secret
    kidCheck:
      enabled: true
      keycloakUrl: http://my-keycloak:8080
      realm: my-realm
    credential:
      secretName: vc-fdsc-edc-credential
```

The api key in `identityhub-secret` must be the random part of a token whose
participant id decodes to exactly `super-user`; the IdentityHub derives the Vault
alias as `<participantContextId>-apikey` and compares the whole presented token
against it.

Keep the identityhub's copy in step by enabling the credential-sync sidecar,
instead of relying on the next deploy:

```yaml
vcCredentials:
  enabled: true
  requests:
    - name: fdsc-edc-credential
      credentialType: membership-credential
      targetSecretName: vc-fdsc-edc-credential
      # hours, not minutes: a five-minute window leaves no room to notice a
      # failure before the credential in use expires
      renewBefore: 24h

identityhub:
  credentialSync:
    enabled: true
    secretName: vc-fdsc-edc-credential
```

The sidecar runs inside the identityhub pod and writes over `localhost`: the
Identity API is component-internal, so the only thing that writes to it is the
pod that owns the store. The vc-operator keeps obtaining and renewing the
credential into the Secret; the sidecar only mirrors it.

## Pin the identity key's lifecycle

The participant's identity is derived from a private key that is usually a
cert-manager TLS Secret. cert-manager's `privateKey.rotationPolicy` defaults to
`Never`, i.e. the key is reused when the certificate is renewed - which is what
keeps the published DID document valid across renewals. That default is not
declared anywhere by default, and upstream intends to change it, so declare it:

```yaml
privateKey:
  algorithm: ECDSA
  rotationPolicy: Never
```

With that in place the key moves only when you decide it should.

## Rotating the signing key

`identityhub.bootstrap.rotation` replaces the key while keeping the same Vault
alias and the same DID fragment. **There is no overlap window**: the moment the key
is replaced, everything signed with the previous one under that fragment stops
verifying, credentials already held in wallets included. Re-issuing them is part of
the operation, not a follow-up.

Keeping the fragment fixed is what makes this practical: the Keycloak realm is not
touched, and one of the places the fragment appears is the `signing_key_id`
attribute of a ClientScope, which cannot be changed by automation on a realm that
already exists.

1. Change the key material in the identity Secret (or let cert-manager reissue with
   `rotationPolicy: Always` once).
2. Run the Job:

   ```yaml
   identityhub:
     bootstrap:
       rotation:
         enabled: true
         # bump for each rotation, so a second run is a new Job and not a failed
         # patch of a completed one
         instance: 1
   ```

3. Watch it: it revokes the current keypair, overwrites the Vault alias and
   publishes the new public key under the same id. Between the revoke and the
   publish, the DID document carries no key at all, so keep an eye on it rather
   than starting a negotiation.

   ```shell
   kubectl logs -n <ns> job/identityhub-key-rotation-1 -f
   curl -s https://my-connector.org/.well-known/did.json | jq '.verificationMethod'
   ```

   Exactly one entry must come back. Two entries sharing an id means something
   called the keypairs `rotate` endpoint instead: that appends rather than
   replaces, and revoking either entry then removes both.

   **The Job is not atomic, and it does not roll back.** If the Vault write fails
   after the revoke, it exits there. Nothing wrong is ever published - the DID
   document is only updated once the new key is in Vault, and the revoke deletes
   the old private key from Vault itself, so there is no stale key left to sign
   with either. But the participant is then left with a revoked keypair and no
   replacement, and can neither sign nor be verified until this is resolved.

   The fix is to run the Job again, with `rotation.instance` bumped so it renders
   as a new Job rather than patching the failed one. A second run picks up cleanly:
   the listing it reads is not filtered by state, so the revoked resource is still
   found, and revoking it again is accepted rather than rejected as an illegal
   transition. Should the underlying cause not be transient, the participant stays
   down until it is fixed - so rotate when someone is around to look at it.

4. Re-issue every credential signed with the old key. For the MembershipCredential
   that means forcing the vc-operator to re-issue - changing the
   `VerifiableCredentialRequest` spec is what triggers it, deleting the Secret is
   not enough:

   ```shell
   kubectl patch vcr fdsc-edc-credential -n <ns> --type merge -p '{"spec":{"renewBefore":"24h1s"}}'
   ```

   Credentials held in wallets have to be requested again by their holders.

5. Restart everything that reads the identity key at startup - typically Keycloak
   (it builds a keystore from it in an init container), both `fdsc-edc`
   controlplanes, and any other workload mounting that Secret - and restart apisix,
   which caches the verifier's JWKS and will not refetch a new key served under an
   unchanged `kid`.

6. Set `rotation.enabled` back to `false`.
