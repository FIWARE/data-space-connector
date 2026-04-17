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

> **Important:** The realm sub-keys (`attributes`, `clientRoles`, `users`, `clientScopes`, `clients`) are **raw JSON strings** inside YAML block scalars (`|`), not YAML objects. The Helm template injects them directly into the realm import JSON file.

Here is the overall structure at a glance:

```yaml
keycloak:
  enabled: true
  ingress: ...              # 1. Ingress & TLS
  initContainers: ...       # 2. Keystore preparation
  extraVolumes: ...         # 3. Volumes (DID material, realm configmap, private key)
  extraVolumeMounts: ...
  issuerDid: ...
  signingKey: ...           # 4. VC signing key
  extraEnvVars: ...         # 5. Environment variables
  realm:
    frontendUrl: ...
    import: true
    name: ...
    attributes: ...         # 6. Credential type definitions
    clientRoles: ...        # 7. Roles per target organization
    users: ...              # 8. Users with role assignments
    clientScopes: ...       # 9. OID4VC protocol mappers
    clients: ...            # 10. Keycloak clients per organization
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
    - mountPath: /opt/bitnami/keycloak/data/import
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

```yaml
  extraEnvVars:
    - name: KEYCLOAK_EXTRA_ARGS
      value: "--import-realm"               # Triggers realm import from the ConfigMap on startup
    - name: KC_FEATURES
      value: "oid4vc-vci"                   # Enables the OID4VCI credential issuance feature
    - name: KC_HEALTH_ENABLED
      value: "true"                         # Enables health check endpoints
    - name: KC_ADMIN_PASSWORD
      valueFrom:
        secretKeyRef:
          name: issuance-secret
          key: keycloak-admin               # Admin password from Kubernetes Secret
    - name: KC_HEAP_SIZE
      value: "1024m"                        # JVM heap size (adjust based on resource limits)
    - name: "STORE_PASS"
      valueFrom:
        secretKeyRef:
          name: keystore-password
          key: password                     # Keystore password (shared with init container)
```

### Realm: credential type definitions

The `realm.attributes` block defines which Verifiable Credential types your organization can issue. Each credential type is configured through a set of attributes following the naming convention `vc.<credential-name>.<property>`.

Example defining one credential type (`user-credential` in JWT format):

```yaml
  realm:
    frontendUrl: https://<your-keycloak-domain>
    import: true
    name: test-realm
    attributes: |
      "issuerDid": "did:web:fancy-marketplace.biz",
      "vc.user-credential.credential_signing_alg_values_supported": "ES256",
      "vc.user-credential.credential_build_config.signing_algorithm": "ES256",
      "vc.user-credential.credential_build_config.token_jws_type": "JWT",
      "vc.user-credential.credential_build_config.proof_types_supported": "{\"jwt\":{\"proof_signing_alg_values_supported\":[\"ES256\"]}}",
      "vc.user-credential.format": "jwt_vc",
      "vc.user-credential.scope": "UserCredential",
      "vc.user-credential.vct": "UserCredential"
```

The available properties for each credential type are:

| Property | Description | Example values |
|----------|-------------|----------------|
| `credential_signing_alg_values_supported` | Algorithms the issuer supports for signing this credential | `ES256` |
| `credential_build_config.signing_algorithm` | Algorithm used when building this credential | `ES256` |
| `credential_build_config.token_jws_type` | JWT format type | `JWT`, `vc+sd-jwt` |
| `credential_build_config.proof_types_supported` | JSON object with supported proof types | `{"jwt":{"proof_signing_alg_values_supported":["ES256"]}}` |
| `format` | Credential format | `jwt_vc`, `vc+sd-jwt` |
| `scope` | OIDC scope that triggers issuance of this credential | Must match a `clientScope` name |
| `vct` | Verifiable Credential Type identifier | Must match the credential type name |
| `credential_build_config.visible_claims` | *(SD-JWT only)* Claims visible without selective disclosure | `roles,email` |
| `credential_build_config.decoys` | *(SD-JWT only)* Number of decoy digests to include | `0`, `3` |

To define additional credential types, repeat the same attribute block with a different `<credential-name>`. For an SD-JWT credential, set `token_jws_type` to `vc+sd-jwt`, `format` to `vc+sd-jwt`, and optionally configure `visible_claims` and `decoys`.

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

### Realm: client scopes (credential type mappers)

Client scopes define the **OID4VC protocol mappers** that determine which claims are included in each Verifiable Credential. The scope `name` must match the `scope` attribute from the [credential type definitions](#realm-credential-type-definitions).

There are four mapper types:

| Mapper type | `protocolMapper` value | Purpose | Key config fields |
|-------------|----------------------|---------|-------------------|
| Context mapper | `oid4vc-context-mapper` | Sets the JSON-LD `@context` for the credential | `context`, `supportedCredentialTypes` |
| User attribute mapper | `oid4vc-user-attribute-mapper` | Maps a Keycloak user attribute to a VC claim | `subjectProperty`, `userAttribute`, `supportedCredentialTypes` |
| Static claim mapper | `oid4vc-static-claim-mapper` | Adds a fixed-value claim to the VC | `subjectProperty`, `staticValue`, `supportedCredentialTypes` |
| Target role mapper | `oid4vc-target-role-mapper` | Maps client roles for a specific target (DID) into the VC | `subjectProperty`, `clientId`, `supportedCredentialTypes` |

Example defining the `UserCredential` scope with its mappers:

```yaml
    clientScopes: |
      {
        "name": "UserCredential",
        "description": "OID4VC scope for user credentials",
        "protocol": "openid-connect",
        "attributes": {},
        "protocolMappers": [
          {
            "name": "context-mapper-uc",
            "protocol": "oid4vc",
            "protocolMapper": "oid4vc-context-mapper",
            "config": {
              "context": "https://www.w3.org/2018/credentials/v1",
              "supportedCredentialTypes": "UserCredential"
            }
          },
          {
            "name": "email-mapper-uc",
            "protocol": "oid4vc",
            "protocolMapper": "oid4vc-user-attribute-mapper",
            "config": {
              "subjectProperty": "email",
              "userAttribute": "email",
              "supportedCredentialTypes": "UserCredential"
            }
          },
          {
            "name": "firstName-mapper-uc",
            "protocol": "oid4vc",
            "protocolMapper": "oid4vc-user-attribute-mapper",
            "config": {
              "subjectProperty": "firstName",
              "userAttribute": "firstName",
              "supportedCredentialTypes": "UserCredential"
            }
          },
          {
            "name": "role-mapper-uc",
            "protocol": "oid4vc",
            "protocolMapper": "oid4vc-target-role-mapper",
            "config": {
              "subjectProperty": "roles",
              "clientId": "<provider-did>",
              "supportedCredentialTypes": "UserCredential"
            }
          }
        ]
      }
```

Key points:
- **Mapper names must be unique** within a client scope (hence the `-uc` suffix convention for UserCredential, `-oc` for OperatorCredential, etc.)
- **`supportedCredentialTypes`** is a comma-separated list — a single mapper can apply to multiple credential types (e.g., `"LegalPersonCredential,OperatorCredential,UserCredential"`)
- Each credential type you define in `realm.attributes` should have a matching client scope here

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
