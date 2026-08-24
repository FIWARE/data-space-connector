# Running the EDC identity material in production

The Dataspace-Protocol part of the connector keeps the participant's identity in
Vault: the signing key the IdentityHub's Secure Token Service signs with, the
IdentityHub's own api key, the client secret shared with the connector's oauth
client, and the RSA key `fdsc-edc` generates for itself to sign transfer EDRs.

By default the chart deploys Vault in **dev mode**, which stores all of that in
memory. That is deliberate for local deployments and for the integration tests,
and it is not usable in production: every restart of the Vault pod drops the lot,
and the DCP lane stays broken until the bootstrap Job runs again. This page is the
production configuration.

## Persistent, self-unsealing Vault

A Vault with real storage comes back **sealed** after any restart and refuses
every request until it is unsealed, so persistence on its own is not enough.
`vault.production` covers the second half.

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
  production:
    enabled: true
    # 1/1 is fine when the keys live in a Secret anyway; raise both to split the
    # master key across several holders and unseal by hand
    secretShares: 1
    secretThreshold: 1
    autoUnseal: true
    secretName: vault-unseal-keys
```

`vault.production.enabled` refuses to render next to `vault.server.dev.enabled`,
rather than quietly doing nothing.

### What auto-unsealing costs

The unseal keys and the initial root token are written to a Kubernetes Secret in
the release namespace. That is what lets an unattended restart recover on its own,
and it means the cluster's Secret store is as trusted as the Vault it opens. There
is no cloud KMS involved and none is required.

If that trade is not acceptable, set `autoUnseal: false` and unseal by hand after
every start:

```shell
kubectl exec -n <ns> <release>-vault-0 -- vault operator init -key-shares=3 -key-threshold=2
kubectl exec -n <ns> <release>-vault-0 -- vault operator unseal <key>
```

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

Use a periodic, renewable token rather than the root one. The IdentityHub renews
it on its own.

```shell
kubectl exec -n <ns> <release>-vault-0 -- vault token create -period=768h -policy=default -field=token
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
