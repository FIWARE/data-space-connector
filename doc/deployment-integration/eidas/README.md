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
| `lotUrl` | string | `""` (official EU LOTL) | URL of the EU List of Trusted Lists. When empty, defaults to `https://ec.europa.eu/tools/lotl/eu-lotl.xml`. |
| `refreshInterval` | int | `86400` | Interval in seconds between background LOTL refreshes. |
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
| `lotUrl` | default (official EU LOTL) | default or a mock URL for isolated testing |

See the [k3s/provider-elsi.yaml](../../../k3s/provider-elsi.yaml) overlay
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
plugin is required to inject this header, which VCVerifier's PKIX chain
validation reads.

> **Note:** Despite its name, the plugin is used for `x5c` header
> injection, not for full JAdES envelope signing. The credential format
> remains a standard JWT — only the `x5c` header is added.

Add the plugin via an init container in the Keycloak configuration:

```yaml
keycloak:
  keycloak:
    extraInitContainers:
      - name: install-jades-issuer
        image: quay.io/fiware/keycloak-jades-vc-issuer:1.2.0
        imagePullPolicy: IfNotPresent
        volumeMounts:
          - name: providers
            mountPath: /target
```

See the [k3s/consumer-elsi.yaml](../../../k3s/consumer-elsi.yaml) overlay
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
   # Signing keystore (for consumer-elsi.yaml elsi.keystore)
   kubectl create secret generic t --dry-run=client -o json \
     --from-file output/keystore.p12 | jq -r '.data."keystore.p12"'
   ```

> **Note:** VCVerifier builds its trust store directly from the EU
> Trusted Lists. You do not need to extract the CA trust store
> (`ca-store.jks`) or the CRL (`crl.pem`) for the provider side.

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
| `refreshInterval` | `86400` (24 hours) | How often (in seconds) to re-fetch the LOTL and rebuild the trust store. |
| `allowStaleTrustLists` | `false` | Whether to accept trust lists whose `NextUpdate` timestamp is in the past. When `false`, an expired trust list causes the affected certificates to be untrusted until a fresh list is obtained. |
| `lotUrl` | `""` (official EU LOTL) | Override the LOTL URL. Useful for testing with a mock trust list. |

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
mvn clean deploy -Plocal,elsi
```

This uses the Maven `elsi` profile, which applies the
[k3s/provider-elsi.yaml](../../../k3s/provider-elsi.yaml) and
[k3s/consumer-elsi.yaml](../../../k3s/consumer-elsi.yaml) overlay files on
top of the base configuration.

The interaction is the same as the standard local deployment (see
[Demo Interactions](../local-deployment/LOCAL.MD#demo-interactions) in
`LOCAL.MD`). The visible difference is the credential format: the JWT's
`iss` claim is a `did:elsi` DID, and the `x5c` header contains the
certificate chain.

Obtain a credential:
```shell
export ELSI_CREDENTIAL=$(./doc/scripts/get_credential.sh \
  https://keycloak-consumer.127.0.0.1.nip.io user-credential)
echo "${ELSI_CREDENTIAL}"
```

Inspect it on [jwt.io](https://jwt.io/) to verify that:
- The `iss` claim is `did:elsi:VATDE-1234567`.
- The `x5c` header contains the certificate chain.

---

## Troubleshooting

### Common Error Scenarios

| Error | Cause | Resolution |
|-------|-------|------------|
| Credential rejected with no trust store | `eidas.enabled` is `false` or the LOTL was unreachable at startup | Set `eidas.enabled: true` and verify network connectivity to the EU LOTL URL |
| `organizationIdentifier` mismatch (`did:elsi` only) | The `did:elsi` identifier does not match OID 2.5.4.97 in the leaf certificate | Regenerate the certificate with the correct `ORGANISATION_IDENTIFIER` matching the DID |
| Certificate chain validation failed | The issuing CA is not in the EU Trusted Lists, or the `countries` filter excludes it | Check the `countries` setting; for test certificates, this is expected (test CAs are not in the EU LOTL) |
| Revocation check failed | OCSP/CRL endpoint unreachable with `revocationCheck: "hard"` | Switch to `"soft"` or ensure OCSP/CRL endpoints are reachable; for local testing use `"off"` |
| Stale trust list rejected | A national trust list's `NextUpdate` is in the past and `allowStaleTrustLists: false` | Set `allowStaleTrustLists: true` or investigate why the trust list is not being refreshed |
| Slow VCVerifier startup | Fetching all EU national trust lists takes time | Use `countries` filter to limit to relevant countries; increase `maxWorkers` for faster parallel fetching |
| Missing `x5c` header in credential | JAdES plugin not installed on consumer Keycloak | Add the `install-jades-issuer` init container (see [JAdES Plugin](#jades-plugin-for-x5c-header-injection)) |

### Debugging Tips

- Check VCVerifier logs for trust store build status at startup.
- Use `helm template` with your values to verify the `eidas` block appears
  in the VCVerifier configmap.
- Decode the credential JWT at [jwt.io](https://jwt.io/) to verify the
  `x5c` header is present and the `iss` claim matches the expected DID or
  issuer URL.
- For test certificates (self-signed), always use `revocationCheck: "off"`.
