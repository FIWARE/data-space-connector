# eIDAS 2.0 Compliance: EU Trusted List Validation

The FIWARE Data Space Connector supports
[eIDAS 2.0](https://digital-strategy.ec.europa.eu/en/policies/eidas-regulation)
compliance through PKIX certificate chain validation against the
[EU Trusted Lists (ETSI TS 119 612)](https://ec.europa.eu/digital-building-blocks/sites/display/DIGITAL/EU+Trusted+Lists).
Starting with **decentralized-iam >= 2.1.23 / VCVerifier >= 6.22.0**,
this validation is performed natively by VCVerifier.

eIDAS 2.0 verification works with any DID method or credential issuance
approach that includes an `x5c` certificate chain header in issued JWTs:

- **`did:elsi`** -- the
  [`did:elsi` DID method](https://alastria.github.io/did-method-elsi/)
  ties organization identity directly to an eIDAS-qualified certificate
  via the `organizationIdentifier` (OID 2.5.4.97) in the Subject DN.
- **`did:web`** -- credentials issued under a `did:web` DID can carry
  an `x5c` header with an eIDAS certificate chain, enabling the same
  PKIX trust list validation.
- **HTTPS-issued credentials** -- any credential that includes the `x5c`
  JWT header can be validated against the EU Trusted Lists, regardless
  of the issuer identifier scheme.

In all cases, VCVerifier extracts the `x5c` header, validates the
certificate chain against the trust store built from the EU List of
Trusted Lists (LOTL), and (for `did:elsi`) additionally verifies that
the certificate's `organizationIdentifier` matches the DID.

eIDAS support is **disabled by default** and is strictly opt-in.

---

<details>
<summary><strong>Table of Contents</strong></summary>

- [Architecture](#architecture)
  - [Supported DID Methods and Issuance Approaches](#supported-did-methods-and-issuance-approaches)
  - [Validation Flow](#validation-flow)
- [Configuration -- Provider (Verifier)](#configuration----provider-verifier)
  - [Enabling eIDAS Validation](#enabling-eidas-validation)
  - [Configuration Reference](#configuration-reference)
  - [Production vs. Test Settings](#production-vs-test-settings)
- [Configuration -- Consumer (Issuer)](#configuration----consumer-issuer)
  - [Keycloak elsi Block](#keycloak-elsi-block)
  - [JAdES Plugin for x5c Header Injection](#jades-plugin-for-x5c-header-injection)
  - [Keystore Volume Mount](#keystore-volume-mount)
- [Preparation -- Generating Test Certificates](#preparation----generating-test-certificates)
- [Per-Credential eIDAS Validation (SD-JWT)](#per-credential-eidas-validation-sd-jwt)
- [Certificate Revocation](#certificate-revocation)
- [Trust List Freshness](#trust-list-freshness)
- [Local Deployment](#local-deployment)
- [Mocking the EU Trusted List (local only)](#mocking-the-eu-trusted-list-local-only)
- [Demo Flow](#demo-flow)
- [Migration from DSS-based Approach](#migration-from-dss-based-approach)
- [Troubleshooting](#troubleshooting)

</details>

---

## Architecture

eIDAS 2.0 support in the Data Space Connector involves two roles:

| Role | Component | Responsibility |
|------|-----------|----------------|
| **Consumer (Issuer)** | Keycloak + JAdES plugin | Issues Verifiable Credentials signed with an eIDAS certificate. The `x5c` JWT header carries the full certificate chain. The credential's `iss` claim can use `did:elsi`, `did:web`, or an HTTPS identifier. |
| **Provider (Verifier)** | VCVerifier | Validates incoming credentials that carry an `x5c` header by verifying the JWT signature against the certificate chain, then validating that chain against a trust store built from the EU List of Trusted Lists (LOTL). For `did:elsi`, the certificate's `organizationIdentifier` is additionally matched against the DID. |

### Supported DID Methods and Issuance Approaches

VCVerifier's eIDAS trust list validation is not limited to a single DID
method. Any credential that carries an `x5c` JWT header with a valid eIDAS
certificate chain can be validated:

| Approach | Identifier Example | Additional Check |
|----------|-------------------|------------------|
| `did:elsi` | `did:elsi:VATDE-1234567` | `organizationIdentifier` (OID 2.5.4.97) in the leaf certificate must match the DID |
| `did:web` | `did:web:example.com` | Standard `did:web` resolution + `x5c` PKIX chain validation |
| HTTPS | `https://issuer.example.com` | `x5c` PKIX chain validation only |

The examples in this document use `did:elsi` because it is the most
tightly coupled with eIDAS certificates, but the PKIX validation itself
applies to all approaches.

### Validation Flow

When VCVerifier receives a credential with an `x5c` header, it performs
the following steps:

```
Credential (JWT with x5c header)
        |
        v
  1. Extract x5c certificate chain from JWT header
        |
        v
  2. Verify JWT signature against the leaf certificate
        |
        v
  3. (did:elsi only) Extract organizationIdentifier (OID 2.5.4.97)
     from the leaf certificate's Subject DN and match against the
     did:elsi identifier (e.g., did:elsi:VATDE-1234567 -> VATDE-1234567)
        |
        v
  4. Validate the certificate chain (PKIX) against the trust store
     built from the EU LOTL
        |
        v
  5. (Optional) Check certificate revocation status via OCSP/CRL
        |
        v
  Credential accepted or rejected
```

The trust store is built at startup by fetching the EU LOTL, parsing the
national Trusted Lists it references, and extracting the certificates of
Qualified Trust Service Providers. It is refreshed periodically in the
background (see [Trust List Freshness](#trust-list-freshness)).

---

## Configuration -- Provider (Verifier)

The provider runs VCVerifier, which validates incoming credentials
carrying eIDAS certificate chains. Two configuration blocks are relevant:

1. `decentralizedIam.vcAuthentication.vcverifier.deployment.elsi` --
   enables the `did:elsi` DID method in the verifier (only needed when
   using `did:elsi`; not required for `did:web` or HTTPS issuers).
2. `decentralizedIam.vcAuthentication.vcverifier.deployment.eidas` --
   configures the built-in PKIX trust list validation (required for all
   eIDAS approaches).

### Enabling eIDAS Validation

Minimal `values.yaml` overlay to enable eIDAS validation with `did:elsi`:

```yaml
decentralizedIam:
  vcAuthentication:
    vcverifier:
      deployment:
        elsi:
          # Enable did:elsi DID method support (omit if using did:web or HTTPS)
          enabled: true
        eidas:
          # Enable built-in eIDAS 2.0 trust list validation
          enabled: true
```

When `eidas.enabled` is `true`, VCVerifier builds a trust store from the
EU LOTL at startup and validates every credential's `x5c` certificate
chain against it. No external validation service is needed.

> **Note:** The `elsi.enabled` toggle is only needed when the issuer uses
> the `did:elsi` DID method. For `did:web` or HTTPS-issued credentials,
> only the `eidas` block is required.

### Configuration Reference

All values live under `decentralizedIam.vcAuthentication.vcverifier.deployment.eidas`:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `enabled` | bool | `false` | Master toggle for eIDAS trust list validation. Must be `true` for any eIDAS-based credential verification (`did:elsi`, `did:web` with `x5c`, or HTTPS-issued). |
| `lotlUrl` | string | `""` (official EU LOTL) | URL of the EU List of Trusted Lists. When empty, defaults to `https://ec.europa.eu/tools/lotl/eu-lotl.xml`. Note the spelling -- VCVerifier reads `lotlUrl`; the `lotUrl` the vcverifier subchart declares is silently ignored. |
| `refreshInterval` | int | `86400` | Interval in seconds between background LOTL refreshes. Clamped to `[3600, 604800]` -- a smaller value is silently raised to one hour. A failed fetch is not retried before the next interval, so this also bounds how long a verifier that started without a reachable trust list stays unusable. |
| `countries` | list | `[]` (all EU) | ISO 3166-1 alpha-2 country filter. Empty list means all EU member states. Example: `["DE", "ES", "FR"]`. |
| `maxWorkers` | int | `5` | Number of concurrent workers for fetching national trust lists. |
| `fetchTimeout` | int | `30` | HTTP timeout in seconds for each individual trust list fetch. |
| `allowStaleTrustLists` | bool | `false` | Accept trust lists past their `NextUpdate` timestamp. Set to `true` only in environments with unreliable internet connectivity. |
| `statusEvaluation` | string | `"current"` | When to evaluate a trust service's status: `"current"` (at verification time) or `"issuance"` (at credential issuance time). |
| `revocationCheck` | string | `"soft"` | OCSP/CRL revocation checking mode. See [Certificate Revocation](#certificate-revocation). |
| `revocationTimeout` | int | `10` | HTTP timeout in seconds for OCSP/CRL requests. |
| `revocationCacheExpiry` | int | `3600` | Cache lifetime in seconds for revocation check results. |

### Production vs. Test Settings

| Setting | Production | Local / Test |
|---------|-----------|--------------|
| `revocationCheck` | `"soft"` or `"hard"` | `"off"` (test certs lack real OCSP/CRL endpoints) |
| `allowStaleTrustLists` | `false` | `true` (acceptable when testing offline) |
| `countries` | filter to relevant countries | `[]` (all) |
| `lotlUrl` | default (official EU LOTL) | the in-cluster mock, see [Demo Flow](#demo-flow) |

See the [k3s/provider-eidas.yaml](../../../k3s/provider-eidas.yaml) overlay
for a complete local-deployment example.

---

## Configuration -- Consumer (Issuer)

The consumer runs Keycloak, which issues Verifiable Credentials signed with
an eIDAS certificate. The `x5c` JWT header carries the full certificate
chain. The credential's `iss` claim can use `did:elsi`, `did:web`, or an
HTTPS identifier -- the examples below use `did:elsi`.

### Keycloak elsi Block

The `elsi` block in `values.yaml` (top-level, alongside `keycloak`) configures
the keystore and DID used for `did:elsi` credential issuance:

```yaml
elsi:
  # Enable did:elsi credential issuance
  enabled: true
  # Path inside the Keycloak container where the signing keystore is located.
  # Since KC 26.6.4, the java-keystore provider only reads from
  # /opt/keycloak/data/<realm-name>/
  storePath: /opt/keycloak/data/test-realm/keystore.p12
  # Keystore password
  storePassword: password
  # Alias of the signing key inside the keystore
  keyAlias: test-keystore
  # Password of the signing key
  keyPassword: password
  # Signing algorithm (RS256 for RSA keys)
  keyAlgorithm: RS256
  # The did:elsi DID — the organisationIdentifier must match the certificate
  did: did:elsi:VATDE-1234567
  # The keystore content (base64-encoded PKCS#12)
  keystore:
    keystore.p12: <BASE64_ENCODED_KEYSTORE>
```

This creates a Kubernetes Secret with the keystore and configures the
Keycloak [realm template](../../../charts/data-space-connector/templates/realm.yaml)
to use `did:elsi` with the corresponding java-keystore key provider.

> **Important:** Since Keycloak 26.6.4, the `java-keystore` realm key
> provider only reads keystores from `/opt/keycloak/data/<realm-name>/`.
> Both `storePath` and the container's volume mount must point inside that
> directory.

### JAdES Plugin for x5c Header Injection

Standard Keycloak OID4VCI issuance does **not** include the `x5c`
certificate chain header in issued JWTs. The
[keycloak-jades-vc-issuer](https://github.com/FIWARE/keycloak-jades-vc-issuer)
plugin supplies it, and VCVerifier's PKIX chain validation reads it.

Since **1.3.0** the plugin signs credentials as JAdES baseline-B
(ETSI TS 119 182-1), not merely as a JWT with an extra header: the protected
header carries the signing time and the certificate digest alongside `x5c`.
It replaces Keycloak's own builder and signer for the formats it serves, and
delegates back to them for credentials that switch JAdES off.

| Format | Default | Why |
|---|---|---|
| `jwt_vc_json` | JAdES | Keycloak's JWT signer writes no `x5c` at all, so eIDAS validation would have nothing to check |
| `dc+sd-jwt` | Keycloak | Keycloak already adds `x5c` to SD-JWT itself, so JAdES adds only the signing time |

Set `jades.enabled` on a credential's client scope to override the default
per credential type:

```yaml
keycloak:
  realm:
    verifiableCredentials:
      user-credential:
        attributes:
          format: "jwt_vc_json"
          # "false" hands this credential back to Keycloak's own signer
          jades.enabled: "true"
```

> **Note:** 1.3.0 requires Keycloak 26.7.x. The OID4VCI signing SPI was
> rewritten after 26.4, and the plugin's `provided` dependency set is pinned to
> what that server ships -- a second copy of a library the server already has
> breaks the Quarkus build at startup.

Add the plugin via an init container in the Keycloak configuration:

```yaml
keycloak:
  keycloak:
    extraInitContainers:
      - name: install-jades-issuer
        image: quay.io/fiware/keycloak-jades-vc-issuer:1.3.0
        imagePullPolicy: IfNotPresent
        volumeMounts:
          - name: providers
            mountPath: /target
```

See the [k3s/consumer-eidas.yaml](../../../k3s/consumer-eidas.yaml) overlay
for the complete init container, volume, and volume-mount configuration.

### Keystore Volume Mount

The eIDAS signing keystore must be mounted into the Keycloak container.
The chart's
[elsi-secret.yaml](../../../charts/data-space-connector/templates/elsi-secret.yaml)
template creates the Secret from the `elsi.keystore` values. It is then
mounted via Keycloak's `extraVolumeMounts` and `extraVolumes`:

```yaml
keycloak:
  keycloak:
    extraVolumeMounts:
      - name: elsi-trust-store
        mountPath: /opt/keycloak/data/test-realm
    extraVolumes:
      - name: elsi-trust-store
        secret:
          secretName: elsi-secret
          defaultMode: 0755
```

> **Important:** The `mountPath` must match your realm name --
> `/opt/keycloak/data/<your-realm-name>/`. The example above uses
> `test-realm`. If your realm is named differently (e.g., `my-realm`),
> change the path to `/opt/keycloak/data/my-realm`. This is a KC 26.6.4
> restriction -- the `java-keystore` provider only reads from that path.

---

## Preparation -- Generating Test Certificates

For local or test environments, a valid eIDAS certificate chain can be
generated using the [FIWARE/eIDAS tool](https://github.com/FIWARE/eIDAS).

### Steps

1. Create a working directory:
   ```shell
   mkdir output
   ```

2. Create a config file:
   ```shell
   cat > output/config << 'EOF'
   COUNTRY="DE"
   LOCALITY="Berlin"
   STATE="Berlin"
   # The organisation identifier must match the did:elsi identifier
   ORGANISATION_IDENTIFIER="VATDE-1234567"
   ORGANISATION="Test org"
   COMMON_NAME="Test"
   EMAIL="test@test.org"
   # For local testing, point to a reachable CRL endpoint.
   # In the k3s local deployment, the CRL is not needed (revocationCheck: "off").
   CRL_URI=http://localhost:3000/crl.pem
   EOF
   ```

3. Generate the key material:
   ```shell
   docker run \
     -v "$(pwd)/output:/out" \
     -v "$(pwd)/output/config:/config/config" \
     quay.io/fiware/eidas:1.3.2
   ```

4. Base64-encode the outputs for use in Helm values files:
   ```shell
   # Signing keystore (for consumer-eidas.yaml elsi.keystore)
   kubectl create secret generic t --dry-run=client -o json \
     --from-file output/keystore.p12 | jq -r '.data."keystore.p12"'
   ```

5. Extract the root CA for the mock trust list (local deployments only):
   ```shell
   openssl x509 -in output/ca/certs/cacert.pem -outform DER | base64 -w0
   ```

> **Note:** In production VCVerifier builds its trust store from the official
> EU Trusted Lists and needs nothing from this tool on the provider side. A
> local deployment is different: the generated CA is self-signed and therefore
> in no national trust list, so the root certificate has to be placed in a mock
> trust list -- see [Mocking the EU Trusted List](#mocking-the-eu-trusted-list-local-only).
> The CRL (`crl.pem`) is still not needed, since local deployments run with
> `revocationCheck: "off"`.

### Certificate Requirements

For VCVerifier's PKIX validation to succeed, the leaf certificate must:

- Be signed by a CA that appears in one of the EU Trusted Lists (or, for
  testing, be self-signed with `revocationCheck: "off"`).
- Include the full certificate chain in the `x5c` JWT header when the
  credential is issued.
- When using `did:elsi`: contain the `organizationIdentifier` attribute
  (OID 2.5.4.97) in the Subject DN, matching the DID identifier.

---

## Per-Credential eIDAS Validation (SD-JWT)

VCVerifier supports per-credential-type eIDAS validation for SD-JWT
credentials through the `eidasConfig` option in the credentials
configuration. This allows requiring eIDAS validation only for specific
credential types rather than applying it globally.

When `eidasConfig` is set on a credential type in the
[Credentials Config Service](https://github.com/FIWARE/credentials-config-service),
VCVerifier applies the eIDAS trust list validation specifically to that
credential type's issuer certificate chain, even if the global `eidas`
block is disabled.

This is useful when:
- Only certain credential types require eIDAS-level assurance.
- Different credential types need different trust list configurations.

---

## Certificate Revocation

The `revocationCheck` setting controls how VCVerifier handles OCSP and CRL
revocation checking for certificates in the `x5c` chain:

| Mode | Behavior |
|------|----------|
| `"off"` | No revocation checking. Use only for local testing with self-signed certificates that lack OCSP/CRL endpoints. |
| `"soft"` | Best-effort revocation checking. If OCSP/CRL endpoints are unreachable or time out, the certificate is **accepted**. This is the default and is recommended for production. |
| `"hard"` | Fail-closed revocation checking. If OCSP/CRL endpoints are unreachable or time out, the certificate is **rejected**. Use when strict compliance is required and OCSP/CRL infrastructure is guaranteed to be available. |

Related settings:

- `revocationTimeout` -- HTTP timeout in seconds for OCSP/CRL requests
  (default: `10`).
- `revocationCacheExpiry` -- How long (in seconds) to cache a revocation
  check result before re-checking (default: `3600`).

---

## Trust List Freshness

VCVerifier fetches the EU List of Trusted Lists (LOTL) at startup and
refreshes it periodically. The following settings control this behavior:

| Setting | Default | Description |
|---------|---------|-------------|
| `refreshInterval` | `86400` (24 hours) | How often (in seconds) to re-fetch the LOTL and rebuild the trust store. Values below `3600` are clamped to one hour. |
| `allowStaleTrustLists` | `false` | Whether to accept trust lists whose `NextUpdate` timestamp is in the past. When `false`, an expired trust list causes the affected certificates to be untrusted until a fresh list is obtained. |
| `lotlUrl` | `""` (official EU LOTL) | Override the LOTL URL. Useful for testing with a mock trust list. |

**Operational considerations:**

- The initial trust store build happens during VCVerifier startup. If the
  LOTL or national lists are unreachable, startup will succeed but the
  trust store will be empty, causing all eIDAS-validated credentials to be
  rejected.
- The `countries` filter (e.g., `["DE", "ES"]`) reduces startup time and
  memory usage by only fetching trust lists for the specified countries.
- `maxWorkers` controls how many national trust lists are fetched in
  parallel (default: `5`). Increase for faster startup in production;
  decrease if the network has strict rate limits.

---

## Local Deployment

Once the provider and consumer are configured (see sections above), deploy
the eIDAS-enabled data space locally:

```shell
mvn clean deploy -Plocal,eidas
```

This uses the Maven `eidas` profile, which applies the
[k3s/provider-eidas.yaml](../../../k3s/provider-eidas.yaml) and
[k3s/consumer-eidas.yaml](../../../k3s/consumer-eidas.yaml) overlay files on
top of the base configuration.

All commands below go through the squid proxy the local deployment brings
up (`-x localhost:8888`), so that in-cluster and host-side name resolution
match. `-k` is needed because the local certificates are issued by the
cluster's self-signed CA.

---

## Mocking the EU Trusted List (local only)

PKIX validation only accepts a certificate chain that terminates in a CA the
trust store knows. The trust store is built from the EU List of Trusted Lists,
and a locally generated test CA is -- by definition -- not in it. Against the
official LOTL, every locally issued credential is therefore rejected, no matter
how the deployment is configured.

A local demo consequently needs a mock LOTL that declares the test CA a granted
qualified CA. The eIDAS overlay ships one:

| File | Role |
|---|---|
| [k3s/eidas-mock/lotl.xml](../../../k3s/eidas-mock/lotl.xml) | List of Trusted Lists, pointing at a single national list |
| [k3s/eidas-mock/tl-de.xml](../../../k3s/eidas-mock/tl-de.xml) | National list (territory `DE`) carrying the test CA as `CA/QC`, status `granted` |

Both are served in-cluster by a small static web server that
[k3s/provider-eidas.yaml](../../../k3s/provider-eidas.yaml) adds through
`extraManifests`, and the verifier is pointed at it:

```yaml
decentralizedIam:
  vcAuthentication:
    vcverifier:
      deployment:
        eidas:
          enabled: true
          # spelled lotlUrl, not lotUrl - see the configuration reference
          lotlUrl: "http://eidas-trust-list-mock:3000/lotl.xml"
```

After regenerating the test material, the certificate in `tl-de.xml` has to be
replaced with the new root CA:

```shell
openssl x509 -in output/ca/certs/cacert.pem -outform DER | base64 -w0
```

VCVerifier does not verify the XMLDSig signature on trust lists -- a documented
limitation of its `eidas` package -- so the mock is served unsigned.

> **Never point a production deployment at a mock trust list.** It declares an
> arbitrary CA qualified, which removes the entire assurance that eIDAS
> validation exists to provide. Production leaves `lotlUrl` unset so the
> official EU LOTL is used.

---

## Demo Flow

This walks an eIDAS credential end to end: the consumer's Keycloak issues a
credential signed with an eIDAS certificate, and the provider's VCVerifier
accepts it after validating the `x5c` chain. It is the standard OID4VP flow
from [LOCAL.MD](../local-deployment/LOCAL.MD#demo-interactions) -- the
eIDAS-specific parts are steps 3 and 4.

### 1. Confirm the verifier started

VCVerifier builds its trust store at startup. If it cannot start, nothing
downstream works:

```shell
kubectl logs -n provider deployment/verifier | tail -20
```

A healthy verifier reports what it loaded into the trust store:

```
TrustListFetcher: starting initial fetch from http://eidas-trust-list-mock:3000/lotl.xml
TrustListFetcher: found 1 national TLs (1 after country filter)
TrustListFetcher: refresh complete, 1 countries loaded, 1 total services
```

`1 total services` is the mock's test CA. If it says `0 total services`, the
trust store is empty and every credential will be rejected in step 6 -- the
verifier starts perfectly happily either way, which is why this is worth
checking first. These lines are logged at `INFO`; at the default `WARN` the
startup is silent.

Two startup failures are also worth recognising:

| Log message | Cause |
|---|---|
| `Failed to initialize verifier: no_did_configured` | `verifier.did` is empty or unresolved |
| `Failed to initialize verifier: request_mode_not_in_supported_modes` | `requestMode` is not one of `supportedModes` |

### 2. Confirm the protected endpoint rejects anonymous access

```shell
curl -k -x localhost:8888 -s -o /dev/null -w '%{http_code}\n' \
  'https://mp-data-service.127.0.0.1.nip.io/ngsi-ld/v1/entities/urn:ngsi-ld:EnergyReport:fms-1'
```

Expected: `401`. A `404` with `{"error_msg":"404 Route Not Found"}` means
APISIX loaded no routes; a plain `404 page not found` comes from Traefik and
means the ingress has no HTTPS router.

The endpoint advertises how to authenticate:

```shell
export TOKEN_ENDPOINT=$(curl -k -x localhost:8888 -s \
  'https://mp-data-service.127.0.0.1.nip.io/.well-known/openid-configuration' \
  | jq -r '.token_endpoint'); echo ${TOKEN_ENDPOINT}
```

```
https://provider-verifier.127.0.0.1.nip.io:8080/services/data-service/token
```

### 3. Issue a credential from the consumer

`get_credential.sh` runs the full OID4VCI pre-authorized-code flow: it logs
the user in, creates a credential offer, redeems the pre-authorized code and
requests the credential.

```shell
export ELSI_CREDENTIAL=$(./doc/scripts/get_credential.sh \
  https://keycloak-consumer.127.0.0.1.nip.io user-credential test-user)
echo "${ELSI_CREDENTIAL}"
```

The third argument is the user the credential is issued to; it is required.
`user-credential`, `operator-credential` and `verifiable-credential` are the
three configurations the eIDAS overlay declares.

> If this answers `400 invalid_credential_offer_request` with
> *"User 'test-user' does not have verifiable credential 'user-credential'"*,
> the realm was imported without
> `keycloak.realm.wallets.issueCredentialsToUsers: true`. Keycloak does not
> re-import an existing realm, so the realm has to be recreated after adding
> it.

### 4. Verify the eIDAS properties of the credential

This is what distinguishes the eIDAS flow from the standard one. Decode the
JWT header and check that the certificate chain travelled with the
credential:

```shell
echo "${ELSI_CREDENTIAL}" | cut -d '.' -f1 | tr '_-' '/+' \
  | awk '{print $0 "=="}' | base64 -d 2>/dev/null \
  | jq '{alg, typ, iat, x5c_chain_length: (.x5c | length)}'
```

```json
{
  "alg": "RS256",
  "typ": "JWT",
  "iat": 1790171496,
  "x5c_chain_length": 3
}
```

`x5c` carries the leaf, intermediate and root certificates that VCVerifier
validates against the trust list. If `x5c` is absent, the JAdES plugin is not
installed on the consumer's Keycloak -- see
[JAdES Plugin for x5c Header Injection](#jades-plugin-for-x5c-header-injection).

`iat` is the signing time, which is what distinguishes a JAdES baseline-B
signature from a plain JWS -- together with `x5c` and the `x5t#o` certificate
digest. Older JAdES carried the signing time as `sigT` and listed it in `crit`;
since ETSI TS 119 182-1 v1.2 it is the registered `iat` claim and needs no
`crit` entry, so nothing in the header forces a verifier to understand JAdES.
`alg` follows `elsi.keyAlgorithm`, and `typ` comes from the credential's
`credential_build_config.token_jws_type`.

To check the chain against the trust list without involving the verifier,
extract the three certificates and the trust list's CA, then verify:

```shell
for i in 0 1 2; do
  echo "${ELSI_CREDENTIAL}" | cut -d '.' -f1 | tr '_-' '/+' | awk '{print $0 "=="}' \
    | base64 -d 2>/dev/null | jq -r ".x5c[$i]" | base64 -d \
    | openssl x509 -inform DER -out chain$i.pem
done

sed -n '/<X509Certificate>/,/<\/X509Certificate>/p' k3s/eidas-mock/tl-de.xml \
  | sed '1d;$d' | tr -d ' \n' | base64 -d | openssl x509 -inform DER -out tl-ca.pem

openssl verify -CAfile tl-ca.pem -untrusted chain1.pem chain0.pem
```

```
chain0.pem: OK
```

A simpler equivalent: the root in the credential's `x5c[-1]` must be
byte-identical to the certificate in
[k3s/eidas-mock/tl-de.xml](../../../k3s/eidas-mock/tl-de.xml). If they differ,
the trust list was built from a different run of the certificate tool than the
keystore in `k3s/consumer-eidas.yaml`, and the verifier rejects the credential
with `certificate does not chain to any trusted service`.

Then check that the issuer is the `did:elsi` DID:

```shell
./doc/scripts/get-payload-from-jwt.sh "${ELSI_CREDENTIAL}" | jq '{iss, vc_type: .vc.type}'
```

```json
{
  "iss": "did:elsi:VATDE-1234567",
  "vc_type": ["VerifiableCredential", "UserCredential"]
}
```

The identifier after `did:elsi:` must equal the `organizationIdentifier`
(OID 2.5.4.97) in the leaf certificate's Subject DN -- that is the match
VCVerifier performs in step 3 of the
[Validation Flow](#validation-flow). To read it from the certificate:

```shell
echo "${ELSI_CREDENTIAL}" | cut -d '.' -f1 | tr '_-' '/+' \
  | awk '{print $0 "=="}' | base64 -d 2>/dev/null | jq -r '.x5c[0]' \
  | base64 -d | openssl x509 -inform DER -noout -subject
```

```
subject=C=DE, ST=Berlin, L=Berlin, O=Test org, CN=Test, .../organizationIdentifier=VATDE-1234567
```

### 5. Create a holder DID

The credential is presented in a Verifiable Presentation signed by the
holder, so a holder key is needed. This is the consumer-side wallet identity
and has nothing to do with the eIDAS certificate:

```shell
mkdir -p cert
chmod o+rw cert
docker run -v $(pwd)/cert:/cert quay.io/wi_stefan/did-helper:0.1.1
# unsecure, only do that for demo
sudo chmod -R o+rw cert/private-key.pem
export HOLDER_DID=$(jq -r '.id' cert/did.json); echo ${HOLDER_DID}
```

### 6. Exchange the credential for an access token

`get_access_token_oid4vp.sh` wraps the credential in a Verifiable
Presentation, signs it with the holder key and posts it to the verifier as a
`vp_token` grant. It is here that VCVerifier performs the eIDAS validation:

```shell
export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh \
  https://mp-data-service.127.0.0.1.nip.io "${ELSI_CREDENTIAL}" default)
echo ${ACCESS_TOKEN}
```

If the token comes back `null`, the verifier rejected the credential. Its log
says why:

```shell
kubectl logs -n provider deployment/verifier --tail=50
```

See [Troubleshooting](#troubleshooting) for the common causes -- with test
certificates the usual one is that the issuing CA is not in the EU Trusted
Lists.

### 7. Access the protected data

```shell
curl -k -x localhost:8888 -s \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  'https://mp-data-service.127.0.0.1.nip.io/ngsi-ld/v1/entities/urn:ngsi-ld:EnergyReport:fms-1' \
  | jq
```

The same request returned `401` in step 2, so a `200` here means the whole
chain held: the credential was signed with an eIDAS certificate, its `x5c`
chain validated against the trust store, the `organizationIdentifier`
matched the `did:elsi` issuer, and the provider's policy granted access.

> The entity and the access policy have to exist for this step. Creating them
> is not eIDAS-specific -- see
> [The Data Provider](../local-deployment/LOCAL.MD#the-data-provider) in
> `LOCAL.MD`.

### 8. Confirm that validation actually bites

A demo that only shows the happy path proves little. Present the same
credential with the trust list validation turned off and then on, and
compare -- or, more simply, present a credential from an issuer that is not
registered at the provider's trusted-issuers list:

```shell
export OTHER_CREDENTIAL=$(./doc/scripts/get_credential.sh \
  https://keycloak-consumer.127.0.0.1.nip.io verifiable-credential test-user)
./doc/scripts/get_access_token_oid4vp.sh \
  https://mp-data-service.127.0.0.1.nip.io "${OTHER_CREDENTIAL}" default
```

`VerifiableCredential` is not among the `credentialTypes` registered for
`did:elsi:VATDE-1234567` in the eIDAS overlay's `registration` block, so the
verifier refuses it and no access token is issued.

---

## Migration from DSS-based Approach

If your deployment previously used the external `dss-validation-service` for
eIDAS certificate validation (chart versions < 10.7.0), follow these steps to
migrate to VCVerifier-native trust list validation.

### Prerequisites

- **decentralized-iam >= 2.1.23** (VCVerifier >= 6.22.0) — included in chart
  version 10.7.0 and later.
- Familiarity with your existing `dss:` configuration block and any custom
  overlays that deploy the DSS sidecar.

### Migration Steps

1. **Enable VCVerifier-native eIDAS validation.** Add the `eidas` block to
   your values overlay:

   ```yaml
   decentralizedIam:
     vcAuthentication:
       vcverifier:
         deployment:
           eidas:
             enabled: true
   ```

   See [Enabling eIDAS Validation](#enabling-eidas-validation) for the full
   minimal overlay and [Configuration Reference](#configuration-reference)
   for all available settings.

2. **Remove or leave the `dss:` block.** The
   `decentralizedIam.vcAuthentication.dss` block is deprecated. When
   `eidas.enabled` is `true`, VCVerifier ignores the DSS configuration
   entirely. You may remove the block now or leave it for a transitional
   period — it has no effect.

3. **Remove DSS infrastructure from overlays.** Delete the following from
   your custom values files and deployment overlays:
   - `dss-validation-service` Deployment / container definitions
   - DSS-related Secrets and ConfigMaps (keystores, CRL stores)
   - CRL-update sidecar or CronJob (if present)
   - Any `Service` or `Ingress` exposing the DSS endpoint

4. **Remove `verifier.elsi.validationEndpoint`.** This setting previously
   pointed VCVerifier at the external DSS service. It is no longer needed —
   validation is built-in when `eidas.enabled` is `true`.

5. **Keep the `keycloak-jades-vc-issuer` init container (consumer side).**
   The JAdES plugin is still required on the consumer's Keycloak to inject
   the `x5c` certificate chain header into issued credentials. See
   [JAdES Plugin for x5c Header Injection](#jades-plugin-for-x5c-header-injection).

6. **Tune environment-specific settings.** Adjust these values for your
   deployment:
   - `revocationCheck` — `"soft"` for production, `"off"` for local testing
     (see [Certificate Revocation](#certificate-revocation))
   - `refreshInterval` — how often to re-fetch the LOTL (default: 24 hours)
   - `countries` — filter to only the EU member states you need

### Before / After Comparison

| Before (< 10.7.0) | After (>= 10.7.0) |
|---|---|
| `decentralizedIam.vcAuthentication.dss` block with DSS endpoint URL | `decentralizedIam.vcAuthentication.vcverifier.deployment.eidas` block |
| External `dss-validation-service` Deployment + Service | No external service — VCVerifier validates natively |
| CRL-update sidecar or CronJob for revocation data | Built-in OCSP/CRL checks via `revocationCheck` setting |
| `verifier.elsi.validationEndpoint` pointing to DSS | Removed — automatic when `eidas.enabled: true` |
| `keycloak-jades-vc-issuer` init container | Retained — still needed for `x5c` header injection |

### Rollback

If you need to revert to the DSS-based approach:

1. Set `eidas.enabled: false` (or remove the `eidas:` block).
2. Restore the `dss:` block and `verifier.elsi.validationEndpoint`.
3. Re-deploy the `dss-validation-service` and CRL infrastructure.

---

## Troubleshooting

### Common Error Scenarios

| Error | Cause | Resolution |
|-------|-------|------------|
| Credential rejected with no trust store | `eidas.enabled` is `false` or the LOTL was unreachable at startup | Set `eidas.enabled: true` and verify network connectivity to the EU LOTL URL |
| `organizationIdentifier` mismatch (`did:elsi` only) | The `did:elsi` identifier does not match OID 2.5.4.97 in the leaf certificate | Regenerate the certificate with the correct `ORGANISATION_IDENTIFIER` matching the DID |
| Certificate chain validation failed | The issuing CA is not in the trust store, or the `countries` filter excludes it | Check the `countries` setting. With locally generated certificates the CA is never in the real EU LOTL -- point `lotlUrl` at the mock trust list and make sure it carries the current root CA (see [Mocking the EU Trusted List](#mocking-the-eu-trusted-list-local-only)) |
| Revocation check failed | OCSP/CRL endpoint unreachable with `revocationCheck: "hard"` | Switch to `"soft"` or ensure OCSP/CRL endpoints are reachable; for local testing use `"off"` |
| Stale trust list rejected | A national trust list's `NextUpdate` is in the past and `allowStaleTrustLists: false` | Set `allowStaleTrustLists: true` or investigate why the trust list is not being refreshed |
| Slow VCVerifier startup | Fetching all EU national trust lists takes time | Use `countries` filter to limit to relevant countries; increase `maxWorkers` for faster parallel fetching |
| Missing `x5c` header in credential | JAdES plugin not installed on consumer Keycloak | Add the `install-jades-issuer` init container (see [JAdES Plugin](#jades-plugin-for-x5c-header-injection)) |
| `TrustListFetcher: ... lookup eidas-trust-list-mock ... no such host` | The mock trust list server is not in the verifier's namespace | Its manifests need `metadata.namespace: {{ .Release.Namespace }}`; without it they are applied to `default` |
| `Was not able to read the key file from .` | `verifier.clientIdentification.keyPath` is unset | Harmless in `urlEncoded` request mode, which needs no request-signing key. Only `byValue` and `byReference` require one |

### Debugging Tips

- Check VCVerifier logs for trust store build status at startup.
- Use `helm template` with your values to verify the `eidas` block appears
  in the VCVerifier configmap.
- Decode the credential JWT at [jwt.io](https://jwt.io/) to verify the
  `x5c` header is present and the `iss` claim matches the expected DID or
  issuer URL.
- For test certificates (self-signed), always use `revocationCheck: "off"`.
