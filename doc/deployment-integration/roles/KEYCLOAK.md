# Keycloak Configuration

## Overview

[Keycloak](https://github.com/keycloak/keycloak) is the **Verifiable Credential (VC) Issuer** used by the FIWARE Data Space Connector. It is responsible for issuing [Verifiable Credentials](https://www.w3.org/TR/vc-data-model/) to the organization's users so they can authenticate and access services offered by other participants in the data space.

Since version 24, Keycloak provides native support for [OpenID for Verifiable Credentials (OID4VC)](https://www.keycloak.org/docs/latest/server_admin/#_oid4vc), including the [OID4VCI](https://openid.net/specs/openid-4-verifiable-credential-issuance-1_0.html) protocol for credential issuance. For more details, see the [Keycloak OID4VC documentation](https://www.keycloak.org/docs/latest/server_admin/#_oid4vc).

## What Keycloak does in the data space

Keycloak:
- **Authenticates** the organization's users (employees, services)
- **Issues Verifiable Credentials (VCs)** via the [OID4VCI](https://openid.net/specs/openid-4-verifiable-credential-issuance-1_0.html) protocol
- **Manages the realm** with users, roles, and credential configurations

Each user in the organization's Keycloak can obtain VCs that encode their roles and attributes. These VCs are then presented to providers to gain access to data services.

## Realm configuration

The Keycloak realm is where the organization defines its identity structure for the data space. A properly configured realm includes clients, roles, and credential types.

### Clients

Create one **client** for each organization you interact with in the data space. Each client is identified by the target organization's [DID](https://www.w3.org/TR/did-core/). This allows Keycloak to scope credentials and roles per partner organization.

### Client roles

Define **client roles** that specify what your users are allowed to do at each provider. These roles are embedded in the Verifiable Credentials issued to your users and are evaluated by the provider's authorization policies.

### Credential types (client scopes)

Credential types are configured as **client scopes** in Keycloak. Each scope maps to a type of Verifiable Credential that your organization can issue. When configuring credential types:

- Define appropriate **claims and attribute mappings** for each credential type
- Set reasonable **credential expiry times**
- Review credential scopes to ensure they follow the **principle of least privilege**

For detailed information on configuring OID4VC credential types in Keycloak, refer to the [Keycloak OID4VC documentation](https://www.keycloak.org/docs/latest/server_admin/#_oid4vci).

## Role-specific considerations

### Consumer

When acting as a **consumer only**, Keycloak issues VCs for the organization's users to present to external providers. No verification of external credentials is needed on the consumer side — that is handled by the provider.

See the [Consumer deployment guide](consumer/README.md) for details.

### Provider

When acting as a **provider**, Keycloak also issues VCs, but additionally the provider deployment includes components that verify credentials presented by external consumers (e.g., VCVerifier, Credentials Config Service).

See the [Provider deployment guide](provider/README.md) for details.

### Consumer & Provider (combined)

In a combined deployment, a **single Keycloak instance** serves both purposes:

1. **As consumer**: Issues VCs that the organization's users present to other providers in the data space
2. **As provider**: Issues VCs for the organization's own employees/services (e.g., for internal access management or for accessing other data spaces)

The same realm handles credential issuance for both roles. The realm should include:
- **Clients** for each external organization (identified by their DID)
- **Client roles** defining what your users are allowed to do at each provider
- **Credential types** (client scopes) mapping to the VCs your organization issues

See the [Consumer & Provider deployment guide](consumer-provider/README.md) for details.

## Helm chart values

Keycloak is enabled by default in the `fiware/data-space-connector` Helm chart. The configuration has two layers:

1. **Infrastructure** — ingress, TLS, init containers, volumes, signing key, environment variables
2. **Realm import** — credential type definitions, clients, roles, users, and credential scopes

The following sections walk through each part of a typical configuration. For a complete working example, see [k3s/consumer.yaml](../../../k3s/consumer.yaml).

> **Important:** Most realm sub-keys (`clientRoles`, `users`, `clients`, `clientScopes`) accept either YAML objects or **raw JSON strings** inside YAML block scalars (`|`); the Helm template injects them directly into the realm import JSON file. The dedicated `verifiableCredentials:` map is YAML-only and renders one ClientScope per entry.

Here is the overall structure at a glance:

```yaml
keycloak:
  enabled: true
  ingress: ...                  # 1. Ingress & TLS
  extraInitContainers: ...      # 2. Keystore preparation (was `initContainers` in 9.x)
  extraVolumes: ...             # 3. Volumes (DID material, realm configmap, private key)
  extraVolumeMounts: ...
  issuerDid: ...
  signingKey: ...               # 4. VC signing key
  extraEnvVars: ...             # 5. Environment variables
  realm:
    frontendUrl: ...
    import: true
    name: ...
    attributes: ...             # 6. Realm-level attributes (issuerDid, etc.)
    clientRoles: ...             # 7. Roles per target organization
    users: ...                  # 8. Users with role assignments
    verifiableCredentials: ...  # 9. VCs (each entry renders a ClientScope with mappers)
    clients: ...                # 10. Keycloak clients per organization
```

### Signing key and DID: a fundamental requirement

The private key used by Keycloak to sign Verifiable Credentials **must** be the same key whose public counterpart is published in the organization's [DID Document](https://www.w3.org/TR/did-core/#verification-methods). This is a fundamental requirement of the trust model:

1. Keycloak signs each VC using the private key from its PKCS12 keystore
2. When a verifier receives a VC, it resolves the issuer's DID to obtain the public key from the DID Document
3. The verifier uses that public key to validate the VC signature

If the keystore contains a different key pair than the one referenced in the DID Document, **signature verification will fail** and the issued credentials will be rejected by other participants in the data space.

This requirement is the reason why the init container, volumes, and signing key configuration sections below all reference the same DID TLS secret — they must all work with the **same key material** that backs the organization's DID.

### Ingress and TLS

Expose Keycloak via an Ingress with TLS terminated by cert-manager:

```yaml
keycloak:
  enabled: true
  ingress:
    enabled: true
    hostname: <your-keycloak-domain>
    annotations:
      traefik.ingress.kubernetes.io/router.tls: "true"         # Enable TLS on the ingress router
      cert-manager.io/cluster-issuer: "selfsigned-issuer"       # Cert-manager issuer (use "prod" for production)
      cert-manager.io/private-key-algorithm: "ECDSA"            # Key algorithm for the TLS certificate
      cert-manager.io/common-name: "<your-keycloak-domain>"
    pathType: Prefix
    tls: true
```

Replace the hostname with your actual domain. In production, use a real cluster issuer (i.e. `prod`) instead of `selfsigned-issuer`.

### Init container: keystore preparation

Keycloak needs a PKCS12 keystore to sign Verifiable Credentials. An init container converts the DID TLS certificate (PEM format) into the required keystore format before Keycloak starts:

```yaml
  initContainers:
    - name: prepare-keystore
      image: alpine/openssl:3.5.5
      command: ["/bin/sh", "-c"]
      args:
        - |
          openssl pkcs12 -export \
            -in /certs-did/tls.crt \
            -inkey /certs-did/tls.key \
            -certfile /certs-did/ca.crt \
            -out /did-material/cert.pfx \
            -name "didPrivateKey" \
            -passout env:STORE_PASS
          chmod 644 /did-material/cert.pfx
      env:
      - name: "STORE_PASS"
        valueFrom:
          secretKeyRef:
            name: keystore-password
            key: password
      volumeMounts:
        - name: did-material
          mountPath: /did-material
        - name: did-priv-key
          mountPath: /certs-did
```

This reads the DID private key and certificate from a TLS secret and produces a `cert.pfx` keystore in the shared `did-material` volume.

### Volumes

Three volumes connect the init container, the Helm-generated realm ConfigMap, and the DID private key to Keycloak:

```yaml
  extraVolumeMounts:
    - mountPath: /did-material
      name: did-material                # PKCS12 keystore (written by init container)
    - mountPath: /opt/keycloak/data/import
      name: realms                      # Realm JSON (generated by Helm template)
  extraVolumes:
    - emptyDir: {}
      name: did-material                # Shared between init container and Keycloak
    - configMap:
        name: test-realm-realm          # ConfigMap name = <realm-name>-realm
      name: realms
    - name: did-priv-key
      secret:
        secretName: <your-keycloak-domain>-tls   # TLS secret with the DID private key
```

Replace `<your-keycloak-domain>-tls` with the name of the TLS secret that contains your organization's DID private key.

### Signing key

The signing key configuration tells Keycloak where to find the keystore and which key to use for signing VCs:

```yaml
  issuerDid: <your-organization's-DID>              # Must match the DID in the DID Document
  signingKey:
    storePath: /did-material/cert.pfx       # Path to the PKCS12 keystore (from init container)
    storePassword: "${STORE_PASS}"          # Password (resolved from env var at runtime)
    keyAlias: didPrivateKey                 # Alias set during keystore creation
    keyPassword: "${STORE_PASS}"
    did: <your-organization's-DID>       # The organization's DID
    keyAlgorithm: ES256                     # ECDSA with P-256 curve
```

The `did` and `issuerDid` must match your organization's registered DID. `ES256` is the recommended algorithm.

### Environment variables

> :warning: **Changed in 10.x.** The Bitnami-only `KEYCLOAK_EXTRA_ARGS` is gone, and several settings that used to live in `extraEnvVars` are now first-class chart values:
>
> - `--import-realm` is auto-added by the umbrella when `realm.import: true`.
> - `KC_FEATURES` → `keycloak.features.enabled` (must include both `oid4vc-vci` and `oid4vc-vci-preauth-code` since KC 26.4+).
> - `KC_HEALTH_ENABLED` is enabled automatically by any active probe (or `metrics.enabled: true`).
> - `KC_ADMIN_PASSWORD` → handled via `keycloak.keycloak.existingSecret` and `keycloak.keycloak.secretKeys.adminPasswordKey`.
>
> Use `extraEnvVars` for genuinely custom env vars only (heap tuning, secrets consumed by templated realm config, etc.).

```yaml
  extraEnvVars:
    - name: KC_HEAP_SIZE
      value: "1024m"                        # JVM heap size (adjust based on resource limits)
    - name: "STORE_PASS"
      valueFrom:
        secretKeyRef:
          name: keystore-password
          key: password                     # Keystore password (shared with init container)
                                            # Consumed by the realm signingKey block via ${STORE_PASS}
```

### Realm: credential type definitions

> :warning: **Changed in 10.x (Keycloak 26.4+).** The legacy `realm.attributes` flat
> scheme `vc.<name>.<property>` and the `realm.credentialBuilder` block are no
> longer recognised. Each credential type is now declared once under
> `realm.verifiableCredentials.<name>`, and the chart renders one ClientScope
> (with `protocol: "oid4vc"`) per entry. See
> [`doc/release-notes/10-x.md`](../../release-notes/10-x.md) for the full
> migration guide.

Example defining one credential type (`user-credential` in JWT-VC JSON format):

```yaml
  realm:
    frontendUrl: https://<your-keycloak-domain>
    import: true
    name: test-realm
    attributes:
      issuerDid: "did:web:fancy-marketplace.biz"
    verifiableCredentials:
      user-credential:
        attributes:
          format: "jwt_vc_json"                       # was: jwt_vc
          verifiable_credential_type: "UserCredential"
          credential_signing_alg: "ES256"             # was: credential_signing_alg_values_supported
          credential_build_config.token_jws_type: "JWT"
          binding_required: "true"
          binding_required_proof_types: "jwt"
        protocolMappers: []                            # see "Realm: protocol mappers per VC" below
```

The most relevant attributes per credential are:

| Attribute | Description | Example values |
|----------|-------------|----------------|
| `format` | Credential format | `jwt_vc_json`, `dc+sd-jwt` |
| `verifiable_credential_type` | Verifiable Credential Type. Maps to the `vct` claim of SD-JWT and to the `vct` field of the issuer metadata. The chart auto-derives `vc.supported_credential_types` from this value (driving the `type` array of JWT-VC JSON credentials) unless you set it explicitly. |
| `credential_signing_alg` | Signing algorithm | `ES256` |
| `credential_build_config.token_jws_type` | `typ` of the JWS header | `JWT`, `vc+sd-jwt`, `dc+sd-jwt` |
| `binding_required` | Whether the holder must provide a key binding proof | `true`, `false` |
| `binding_required_proof_types` | CSV of allowed proof types | `jwt` |
| `credential_build_config.sd_jwt.visible_claims` | *(SD-JWT only)* CSV of claims that stay disclosed. **Must include the KC red-listed claims** `iss,iat,nbf,exp,cnf,vct,status` if you override this list, otherwise issuance fails with `UndisclosedClaims contains red listed claim names`. | `iss,iat,nbf,exp,cnf,vct,status,roles,email` |
| `sd_jwt.number_of_decoys` | *(SD-JWT only)* Number of decoy digests to include | `0`, `3` |

To define additional credential types, add more entries under `verifiableCredentials`. The key of each entry is the ClientScope name — i.e. the value the wallet sends as `credential_configuration_id`.

See [k3s/consumer.yaml](../../../k3s/consumer.yaml) for a complete example with multiple credential types (LegalPersonCredential, UserCredential, OperatorCredential, MarketplaceCredential, MembershipCredential, etc.).

### Realm: client roles

Client roles define what your users are allowed to do at each target organization. The key is the **DID of the target organization** (the provider), and the value is an array of roles:

```yaml
    clientRoles: |
      "<provider-did>": [
        {
          "name": "customer",
          "description": "Is allowed to see offers",
          "clientRole": true
        },
        {
          "name": "READER",
          "description": "Is allowed to read data",
          "clientRole": true
        }
      ]
```

The role names must match what the target provider's access policies expect. Add additional DID keys to define roles for multiple target organizations.

### Realm: users

Each user definition includes credentials and role assignments:

```yaml
    users: |
      {
        "username": "employee",
        "enabled": true,
        "email": "employee@consumer.org",
        "firstName": "Test",
        "lastName": "User",
        "credentials": [
          {
            "type": "password",
            "value": "test"
          }
        ],
        "clientRoles": {
          "<provider-did>": [
            "customer",
            "READER"
          ],
          "account": [
            "view-profile",
            "manage-account"
          ]
        },
        "groups": [
          "/consumer"
        ]
      }
```

The `clientRoles` section assigns the roles defined above to this user. The `account` roles are standard Keycloak roles for account self-management. To define multiple users, separate them with commas in the JSON string.

> **Production note:** Do not hardcode passwords in values files. Use Kubernetes Secrets or an external identity provider for user management.

### Realm: protocol mappers per VC

Each entry under `verifiableCredentials` carries its own `protocolMappers` list. The DSC chart renders a ClientScope (`protocol: "oid4vc"`) per VC and attaches those mappers to it; you no longer declare the scope by hand. The mapper-type catalogue:

| Mapper type | `protocolMapper` value | Purpose | Key config fields |
|-------------|----------------------|---------|-------------------|
| Context mapper | `oid4vc-context-mapper` | Sets the JSON-LD `@context` for the credential | `context` |
| User attribute mapper | `oid4vc-user-attribute-mapper` | Maps a Keycloak user attribute to a VC claim | `claim.name`, `userAttribute` |
| Static claim mapper | `oid4vc-static-claim-mapper` | Adds a fixed-value claim to the VC | `claim.name`, `staticValue` |
| Target role mapper | `oid4vc-target-role-mapper` | Maps client roles for a specific target (DID) into the VC | `claim.name`, `clientId` |

Example defining the `user-credential` VC with its mappers:

```yaml
    verifiableCredentials:
      user-credential:
        attributes:
          format: "jwt_vc_json"
          verifiable_credential_type: "UserCredential"
          credential_signing_alg: "ES256"
          credential_build_config.token_jws_type: "JWT"
        protocolMappers:
          - name: context-mapper-uc
            protocol: oid4vc
            protocolMapper: oid4vc-context-mapper
            config:
              context: https://www.w3.org/2018/credentials/v1
          - name: email-mapper-uc
            protocol: oid4vc
            protocolMapper: oid4vc-user-attribute-mapper
            config:
              claim.name: email
              userAttribute: email
          - name: firstName-mapper-uc
            protocol: oid4vc
            protocolMapper: oid4vc-user-attribute-mapper
            config:
              claim.name: firstName
              userAttribute: firstName
          - name: role-mapper-uc
            protocol: oid4vc
            protocolMapper: oid4vc-target-role-mapper
            config:
              claim.name: roles
              clientId: "<provider-did>"
```

Key points:
- **Mapper `name` must be unique within a VC's `protocolMappers` list** (the suffix convention `-uc` / `-oc` / `-mc` makes greps easier).
- **As of KC 26.4+, each ClientScope corresponds to exactly one credential.** A mapper's membership in a specific VC entry replaces the legacy `supportedCredentialTypes` CSV — to apply the same mapper logic to multiple VCs, declare it once under each VC.
- **`oid4vc-target-role-mapper`** does not allow two mappers with the same `claim.name` in the same scope. Pick one target per VC, or use distinct claim names; downstream verifiers (FIWARE vcverifier, ODRL/OPA) read `credentialSubject.roles[*].target` by default.
- For the full mapper API reference (properties, defaults, examples), see [`doc/keycloak/oid4vc-protocol-mappers.md`](../../keycloak/oid4vc-protocol-mappers.md).

For a complete example with all credential types, see [k3s/consumer.yaml](../../../k3s/consumer.yaml).

### Realm: clients

Keycloak clients represent the organizations your users can interact with. Each external organization is identified by its **DID as the `clientId`**:

```yaml
    clients: |
      {
        "clientId": "<provider-did>",
        "enabled": true,
        "description": "Client to connect mp-operations.org",
        "surrogateAuthRequired": false,
        "alwaysDisplayInConsole": false,
        "clientAuthenticatorType": "client-secret",
        "defaultRoles": [],
        "redirectUris": [],
        "webOrigins": [],
        "notBefore": 0,
        "bearerOnly": false,
        "consentRequired": false,
        "standardFlowEnabled": true,
        "implicitFlowEnabled": false,
        "directAccessGrantsEnabled": false,
        "serviceAccountsEnabled": false,
        "publicClient": false,
        "frontchannelLogout": false,
        "protocol": "openid-connect",
        "attributes": {
          "client.secret.creation.time": "1675260539"
        },
        "protocolMappers": [],
        "authenticationFlowBindingOverrides": {},
        "fullScopeAllowed": true,
        "nodeReRegistrationTimeout": -1,
        "defaultClientScopes": [],
        "optionalClientScopes": []
      }
```

Most fields are Keycloak defaults that should be kept as-is. The key field to customize is `clientId`, which must be the DID of the target organization. Add one client block per external organization, separated by commas.

In addition to the DID-based clients, include the standard Keycloak clients (`account`, `account-console`, `admin-cli`). The `account-console` client's `optionalClientScopes` must list your VC credential scope names (e.g., `LegalPersonCredential`, `UserCredential`) for the Keycloak console to support credential issuance.

For a complete example including all standard clients, see [k3s/consumer.yaml](../../../k3s/consumer.yaml).

### PostgreSQL database

Keycloak requires a PostgreSQL database. If you do not have an external PostgreSQL instance, you can enable the built-in managed PostgreSQL:

```yaml
decentralizedIam:
  enabled: true
  vcAuthentication:
    managedPostgres:
      enabled: true
      config:
        volume:
          storageClass: "" # Use default storage class or specify one
```

### Complete example

A complete working example with multiple credential types, users, and clients is available at [k3s/consumer.yaml](../../../k3s/consumer.yaml). This file is intended for local development and testing. For production deployments, replace test values (passwords, DIDs, domains) with your real configuration.

### Production considerations

- Use an **external PostgreSQL** instance with proper backups and high availability instead of the built-in managed PostgreSQL
- Configure **TLS** for Keycloak's endpoints
- Set strong **admin credentials** and rotate them regularly
