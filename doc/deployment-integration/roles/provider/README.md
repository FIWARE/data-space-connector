# Provider Role

## Overview

A Provider is an organization that **offers data services to other participants** in the data space (e.g. APIs, data services web applications, etc). The provider hosts one or more services and protects them with a full decentralized identity and access management stack. Incoming requests must present valid Verifiable Credentials, which are verified against the data space's trust framework and evaluated against ODRL access policies.

These services can be:
- **APIs** — RESTful APIs, NGSI-LD endpoints or any other HTTP-based service
- **Web applications** — applications that integrate with Verifiable Credentials via [OID4VC](https://openid.net/specs/openid-4-verifiable-credential-issuance-1_0.html) for user authentication and authorization

A provider may also needs the ability to issue its own Verifiable Credentials (for its employees or services), so it includes all the components of a [Consumer](../consumer/README.md) plus additional authentication, authorization, and data service components.

## Before you start

Before deploying, contact the **Data Space Operator** to initiate the onboarding process for your organization. The Operator will guide you through:

- The documentation and requirements needed to join the data space
- Registration of your organization's DID at the Trust Anchor
- The credential types your organization is allowed to issue and their required configuration (claims, attribute mappings, etc.)
- The Trust Anchor's TIR endpoint URL

This information is essential for configuring Keycloak, the DID, the VCVerifier, and the Verifiable Credentials correctly.

Check also that the cluster meets the [Cluster Prerequisites](../PREREQUISITES.md): its databases, the network requirements and the resources the provider needs.

## Required components

> **Note:** The data services that the provider wants to expose through the FIWARE Data Space Connector (APIs, web applications, etc.) are not included in this list. These are external to the connector and must be deployed and managed independently.

### Everything from the Consumer role

The provider needs the same base components as a consumer:
- **Keycloak** — VC issuer for the provider's own users. For detailed configuration instructions, see the [Keycloak Configuration](../KEYCLOAK.md) guide.
- **DID Helper** — publishes the provider's DID document
- **PostgreSQL** — database for Keycloak and the IAM components: an existing one, or one the chart deploys (see [Databases](#databases))

> **Note:** If the data space uses the [Decentralized Claims Protocol (DCP)](https://eclipse-dataspace-dcp.github.io/decentralized-claims-protocol/v1.0.1/) (typically together with the [Dataspace Protocol](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol) via [FDSC-EDC](#fdsc-edc-dataspace-protocol)), the **DID Helper is replaced by the Identity Hub**, which serves the DID document (and the provider's credentials) as required by DCP. See the [Dataspace Protocol Integration Guide](../../../DSP_INTEGRATION.md) for details.

### Authentication: Verifiable Credential verification

| Component | Description |
|-----------|-------------|
| **VCVerifier** | Verifies incoming Verifiable Credentials via [OID4VP](https://openid.net/specs/openid-4-verifiable-presentations-1_0.html). Checks credential signatures, issuer trust (via the Trust Anchor), and credential type acceptance. Returns a JWT access token on success. |
| **Credentials configuration** | Which credential types are accepted for each protected service, and which Trusted Issuers Lists and Trusted Participants Lists to check. Served by the VCVerifier itself, on its configuration API (port 8090); the services are registered there from the values with `vcverifier.registration.services`. The standalone [Credentials Config Service](https://github.com/FIWARE/credentials-config-service) is no longer needed and is disabled by default. |
| **Trusted Issuers List** (local) | The provider's own list of trusted issuers, used in combination with the Trust Anchor's registry to determine which organizations can issue specific credential types. |

### Authorization: Policy enforcement

| Component | Description |
|-----------|-------------|
| **APISIX** | API Gateway that acts as the Policy Enforcement Point (PEP). Validates JWT tokens from VCVerifier and delegates authorization decisions to OPA. All external access to protected services goes through APISIX. |
| **OPA** (Open Policy Agent) | Policy Decision Point (PDP). Evaluates authorization policies (in Rego format, generated from ODRL) for each incoming request and returns allow/deny decisions. |
| **ODRL-PAP** | Policy Administration Point. Allows the provider to define access policies using [ODRL](https://www.w3.org/TR/odrl-model/) (Open Digital Rights Language). Translates ODRL policies into Rego for OPA. |


### Databases

Keycloak, the Trusted Issuers List, the VCVerifier, the ODRL-PAP and Scorpio store their data in PostgreSQL; the local marketplace also needs MongoDB (see [Running a local marketplace](./LOCAL_MARKETPLACE.md#mongodb)). Use databases you already run, or let the chart deploy them:

- **A PostgreSQL deployed by the chart** (`decentralizedIam.vcAuthentication.managedPostgres`, the default): a single cluster named `postgres` in the connector's namespace, with a database for each component, created by the [Zalando Postgres Operator](https://github.com/zalando/postgres-operator). The chart installs that operator by default too (`decentralizedIam.vcAuthentication.postgres-operator.enabled`); if one already runs in the cluster - for example installed by another connector or by the Trust Anchor - disable it in this release, since the operator's cluster-wide resources can only belong to one release.
- **An existing PostgreSQL**: disable both, create a database and a user for each component, store the passwords in Secrets of the connector's namespace, and point each component to them:

```yaml
decentralizedIam:
  vcAuthentication:
    managedPostgres:
      enabled: false
    postgres-operator:
      enabled: false
    trusted-issuers-list:
      database:
        host: <your_postgres_host>
        port: 5432
        name: <til_database>
        username: <til_user>
        existingSecret:
          enabled: true
          name: <til_secret>
          key: password
    vcverifier:
      deployment:
        database:
          host: <your_postgres_host>
          name: <verifier_database>
          existingSecret:
            name: <verifier_secret>               # holds both the user and the password
            usernameKey: username
            passwordKey: password
  odrlAuthorization:
    odrl-pap:
      database:
        url: jdbc:postgresql://<your_postgres_host>:5432/<pap_database>
        username: <pap_user>
        existingSecret:
          enabled: true
          name: <pap_secret>
          key: password
keycloak:
  database:
    host: <your_postgres_host>
    name: <keycloak_database>
    existingSecret: <keycloak_secret>             # holds both the user and the password
    secretKeys:
      usernameKey: username
      passwordKey: password
scorpio:
  db:
    dbhost: <your_postgres_host>
    user: <scorpio_user>
    existingSecret:
      enabled: true
      name: <scorpio_secret>
      key: password
```

Scorpio needs the PostGIS extension in its database; the examples in [`k3s/`](../../../../k3s) create it, as a database named `ngb`, through the operator's `preparedDatabases`. The Identity Hub and the status list server, when enabled, take their database settings under `identityhub.postgresql` and `statusListServer.postgresql`.

## Optional components

All optional components listed below can be deployed using the `fiware/data-space-connector` Helm chart. Some of them - Scorpio, the TMForum API and Contract Management - are **enabled by default**; set them to `false` explicitly if you do not need them. The [values example](#using-the-fiware-dsc-helm-chart-and-values) below lists the default of each one.

> **Note on Scorpio Context Broker:** The FIWARE implementation of the [TMForum APIs](https://github.com/FIWARE/tmforum-api) is based on [NGSI-LD](https://www.etsi.org/deliver/etsi_gs/CIM/001_099/009/01.08.01_60/gs_CIM009v010801p.pdf), which is why the `fiware/data-space-connector` Helm chart deploys a [Scorpio Context Broker](https://github.com/ScorpioBroker/ScorpioBroker) as its backend. In the provided examples, this same Scorpio instance is also used as the NGSI-LD endpoint that the provider exposes through the connector. However, **in production**, if you want to expose an NGSI-LD endpoint as a data service, you should deploy a **separate Context Broker** for that purpose and not reuse the Scorpio instance that backs the TMForum APIs.

### TMForum API

The [TMForum API](https://github.com/FIWARE/tmforum-api) implements standardized APIs for product catalog management, ordering, inventory, and more. These APIs are used both by the provider's own local marketplace and by the [central marketplace](../../../CENTRAL_MARKETPLACE.md) if one exists in the data space. Enable this when:
- You want to offer data products through a structured catalog
- You need standardized product ordering workflows
- You plan to use contract-based access control
- You want to publish offerings to a central marketplace

### Contract Management

The [Contract Management](https://github.com/FIWARE/contract-management) component automates access control based on product orders. When a consumer purchases a product offering via the TMForum API, Contract Management reacts to the order and, most importantly, **registers the consumer's DID in the provider's local Trusted Issuers List**, so the consumer's credentials are accepted at the provider when accessing the purchased service. The credential types granted to the consumer are taken from the ordered product. Additionally, if the [ODRL-PAP](https://github.com/SEAMWARE/odrl-pap) is deployed, Contract Management also creates the ODRL access policies that encode the contract terms. Enable this when:
- You use TMForum APIs for product ordering
- Access should be granted/revoked automatically based on contracts
- You need audit trails for data access agreements

> **Note:** If the data space uses a [central marketplace](../../../CENTRAL_MARKETPLACE.md), the Contract Management component requires specific configuration to work with it. See the [Central Marketplace documentation](../../../CENTRAL_MARKETPLACE.md) for details on how to configure Contract Management in that scenario.

> **Note:** The policies and the credential configuration are read from the ordered `ProductSpecification` by default. A product can instead be composed of `ServiceSpecification`s that carry their own policies - an access policy protects an API, and in TMForum an API is a `ServiceSpecification`. See [Authoring a composed ProductSpecification](./COMPOSED_SPECIFICATIONS.md).

### Marketplace (BAE)

A provider can offer its products and services through a marketplace in two ways:

- **Local marketplace** — Deploy a marketplace as part of the provider's own connector. The [Business API Ecosystem](https://github.com/FIWARE/business-api-ecosystem) provides a web portal for browsing and ordering product offerings directly from this provider. See [Running a local marketplace](./LOCAL_MARKETPLACE.md).
- **Central marketplace** — If the data space operates a shared central marketplace, the provider can publish its offerings there instead of (or in addition to) running its own. See the [Central Marketplace documentation](../../../CENTRAL_MARKETPLACE.md) for details.

Enable the local marketplace when:
- You want a user-facing web interface for your own data marketplace
- Consumers should be able to browse and order offerings through a portal hosted by the provider

> **Note:** The local Marketplace requires MongoDB in addition to the existing PostgreSQL, deployed by the chart through the MongoDB Community Operator. [Running a local marketplace](./LOCAL_MARKETPLACE.md) covers the whole setup: the operator, the values, the login with Verifiable Credentials and the security considerations.

> **Note:** To create **DSP-compatible** products and offerings (i.e. offerings whose data is exchanged over the Dataspace Protocol — see [FDSC-EDC (Dataspace Protocol)](#fdsc-edc-dataspace-protocol)) through the local marketplace UI, the BAE Logic Proxy must be deployed with `BAE_LP_DSP_ENABLED=true`, set under `marketplace.bizEcosystemLogicProxy.additionalEnvVars`. This flag exposes the DSP-specific fields in the offering-creation forms; omit it for a marketplace that only brokers classic FIWARE offerings. See [Using the BAE Marketplace](../../../DSP_INTEGRATION.md#using-the-bae-marketplace) in the DSP Integration documentation.

#### Participating in a Central Marketplace

If the data space runs a shared [Central Marketplace](../../../CENTRAL_MARKETPLACE.md) hosted by another actor (typically the Data Space Operator), the provider publishes its offerings there and does **not** deploy the marketplace itself. However, the provider's DSC needs specific configuration so the Central Marketplace can reach the local Contract Management and trigger the activation flow after each purchase.

> **Minimum versions**
> - `fiware/data-space-connector` Helm chart ≥ **9.0.1**
> - `fiware/contract-management` Helm chart ≥ **3.5.22**
> - `contract-management` container image ≥ **3.3.8**

The following configuration is required on the provider side. The example in [`k3s/provider.yaml`](../../../../k3s/provider.yaml) already includes all of the pieces below and can be used as a reference.

**1. Enable and configure Contract Management in central-marketplace mode**

```yaml
contract-management:
  enabled: true
  enableCentralMarketplace: true          # react to order notifications coming from an external marketplace
  enableOdrlPap: true                     # write ODRL policies to the local PAP on contract activation
  did: <your-organization-DID>
  til:
    credentialType: <YourCredentialType>  # e.g. OperatorCredential — registered in the local TIL on activation
  services:
    odrl:
      url: http://odrl-pap:8080
    # ... TMForum URLs, trusted-issuers-list, etc.
  notification:
    enabled: true
    host: contract-management
  deployment:
    image:
      tag: 3.3.8                          # minimum image required for central marketplace mode
```

- `enableCentralMarketplace` activates the flow that reacts to order notifications coming from an external marketplace (instead of reacting to local orders only).
- `enableOdrlPap: true` is required so that the ODRL policies attached to each purchased offering are installed in the provider's PAP when the contract is activated.
- `til.credentialType` defines the credential type that Contract Management will register for each buyer in the local Trusted Issuers List when a purchase is completed.

**2. Expose Contract Management externally through APISIX**

The Central Marketplace sends order notifications to the provider's Contract Management over HTTPS, authenticating with its `MarketplaceCredential`. APISIX must expose the Contract Management on a dedicated host with the standard OIDC + OPA protection:

```yaml
decentralizedIam:
  odrlAuthorization:
    apisix:
      ingress:
        hosts:
          - host: <your_contract_management_domain>    # e.g. provider-cm.example.org
            paths: ["/"]
      routes:
        # Well-known endpoint for Contract Management — proxied to the verifier
        - uri: /*/.well-known/openid-configuration
          host: <your_contract_management_domain>
          upstream:
            nodes:
              verifier:3000: 1
            type: roundrobin
          plugins:
            proxy-rewrite:
              uri: /services/contract-management/.well-known/openid-configuration
        # Catch-all route to Contract Management, protected by VCVerifier + OPA
        - uri: /*
          host: <your_contract_management_domain>
          upstream:
            nodes:
              contract-management:8080: 1
            type: roundrobin
          plugins:
            openid-connect:
              bearer_only: true
              use_jwks: true
              client_id: contract-management
              client_secret: unused
              ssl_verify: true            # see "Trusting a private CA" below if your certificates are not publicly trusted
              discovery: http://verifier:3000/services/contract-management/.well-known/openid-configuration
            opa:
              host: http://localhost:8181
              policy: policy/main
              with_body: true
```

**3. Register the Contract Management as a service at the VCVerifier**

The VCVerifier needs to know which credential types to accept on the Contract Management host. Register the service with a dedicated OIDC scope (`external-marketplace`) that accepts the `MarketplaceCredential`:

```yaml
decentralizedIam:
  vcAuthentication:
    vcverifier:
      registration:
        enabled: true
        services:
          - id: contract-management
            defaultOidcScope: "external-marketplace"
            authorizationType: "DEEPLINK"
            oidcScopes:
              "external-marketplace":
                credentials:
                  - type: MarketplaceCredential
                    trustedParticipantsLists:
                      - <trust_anchor_tir_url>     # Trust Anchor's TIR endpoint
                    trustedIssuersLists:
                      - "*"                        # any issuer registered at the Trust Anchor
                    jwtInclusion:
                      enabled: true
                      fullInclusion: true
```

Add the entry to the same `registration.services` list as the provider's other services (such as the `data-service` of the [values example](#using-the-fiware-dsc-helm-chart-and-values)): Helm replaces lists instead of merging them, so a list in a second values file drops the first one.

`trustedIssuersLists: "*"` is intentional here: the marketplace's credential is issued by the marketplace itself, and the trust decision is delegated entirely to the Trust Anchor's TIR.

**4. Register the `allowContractManagement` policy at the PAP**

Before receiving the first notification, the provider must install a policy at its PAP granting the Central Marketplace access to the Contract Management endpoint. See [Prepare the provider (Step 1)](../../../CENTRAL_MARKETPLACE.md#prepare-the-provider-step-1) in the Central Marketplace documentation for the exact policy and the `curl` command.

**5. Register the provider as an Organization at the Central Marketplace**

The Central Marketplace needs to know the provider's Contract Management endpoint and the OIDC client to authenticate with. This registration can be done through the TMForum API (`partyCharacteristic.contractManagement`) or through the Marketplace UI (Profile form). See [Prepare the provider (Step 1)](../../../CENTRAL_MARKETPLACE.md#prepare-the-provider-step-1) and [Using the Marketplace UI](../../../CENTRAL_MARKETPLACE.md#using-the-marketplace-ui) in the Central Marketplace documentation for both variants.

### FDSC-EDC (Dataspace Protocol)

Enable the Eclipse Dataspace Components connector for DSP compliance. See [DSP Integration](../../../DSP_INTEGRATION.md).

## Deployment

### Using the FIWARE DSC Helm chart and values

The provider uses the `fiware/data-space-connector` chart with authentication and authorization components enabled.

1. Create a helm `values.yaml` file with the key settings for a provider deployment:

   The Ingresses in the example only carry the cert-manager annotations and use the cluster's default ingress class unless `className` is set. Add the class and the annotations your ingress controller needs; the [local deployment](../../local-deployment/LOCAL.MD) uses Traefik, for example.

```yaml
# Key settings for provider deployment

# Enable cert-manager for prod
cert-manager:
  enabled: true
  crds:
    enabled: true

certManagerResources:
  enabled: true
  type: "prod"

# Keycloak — see ../KEYCLOAK.md for detailed configuration
keycloak:


# DID of type web using cert-manager for TLS and for the DID document public key
did:
  enabled: true
  config:
    server:
      # https: did-helper also publishes <hostUrl>/.well-known/tls.crt as the x5u of the key
      hostUrl: "https://<your_did_domain>"
      certPath: "/certs/tls.crt"
  volumes:
    - name: certs
      secret:
        # the DID's key, issued by cert-manager for the Ingress below and never rotated: see
        # "Identity key lifecycle"
        secretName: <your_did_domain>-tls
        items:
          - key: tls.crt
            path: tls.crt
  volumeMounts:
    - name: certs
      mountPath: /certs
  # with certPath did-helper writes nothing, so it runs on a read-only root filesystem
  securityContext:
    readOnlyRootFilesystem: true
    allowPrivilegeEscalation: false
    capabilities:
      drop: ["ALL"]
  ingress:
    enabled: true
    className: <your-ingress-class>   # add the annotations your ingress controller needs
    annotations:
      cert-manager.io/cluster-issuer: "prod" # Use self-signed for testing
      cert-manager.io/private-key-algorithm: "ECDSA"
      # the Secret also holds the DID's key: renewals must keep it (cert-manager >= 1.18
      # generates a new one by default)
      cert-manager.io/private-key-rotation-policy: "Never"
      cert-manager.io/common-name: "<your_did_domain>"
    hosts:
      - host: <your_did_domain>
        paths:
          - path: /
            pathType: ImplementationSpecific
    tls:
      - secretName: <your_did_domain>-tls
        hosts:
          - <your_did_domain>

# --- Authentication and Authorization ---

decentralizedIam:
  enabled: true
  vcAuthentication:
    # VCVerifier — verifies incoming Verifiable Credentials via OID4VP
    vcverifier:
      ingress:
        enabled: true
        annotations:
          cert-manager.io/cluster-issuer: "prod"
          cert-manager.io/private-key-algorithm: "ECDSA"
          cert-manager.io/common-name: "<your_verifier_domain>"
        tls:
          - hosts:
              - <your_verifier_domain>
            secretName: <your_verifier_domain>-tls
        hosts:
          - host: <your_verifier_domain>
            paths:
              - "/"
      deployment:
        verifier:
          tirAddress: <trust_anchor_tir_url>     # Trust Anchor's TIR endpoint
          did: <your-organization's-DID>
        server:
          host: https://<your_verifier_domain>
      # Services and the credentials they accept, registered at the VCVerifier's configuration
      # API on install and upgrade. A service that is already registered is left as it is: change
      # it through that API (`PUT http://verifier:8090/service/<id>`).
      registration:
        enabled: true
        services:
          - id: data-service                       # ID for the protected service
            defaultOidcScope: "default"
            authorizationType: "DEEPLINK"
            oidcScopes:
              "default":
                credentials:
                  - type: UserCredential           # Accepted credential type
                    trustedParticipantsLists:
                      - <trust_anchor_tir_url>     # Trust Anchor's TIR endpoint
                    trustedIssuersLists:
                      - http://trusted-issuers-list:8080  # Local TIL

    # Trusted Issuers List — local list of trusted issuers
    trusted-issuers-list:
      ingress:
        til:
          enabled: true
          annotations:
            cert-manager.io/cluster-issuer: "prod"
            cert-manager.io/private-key-algorithm: "ECDSA"
            cert-manager.io/common-name: "<your_til_domain>"
          hosts:
            - host: <your_til_domain>
              paths:
                - /
          tls:
            - secretName: <your_til_domain>-tls
              hosts:
                - <your_til_domain>

    # Managed PostgreSQL for IAM components
    managedPostgres:
      enabled: true
      config:
        volume:
          storageClass: "" # Use default storage class or specify one if needed

  # --- Authorization: APISIX (PEP) + OPA (PDP) + ODRL-PAP ---
  odrlAuthorization:
    apisix:
      apisix:
        admin:
          credentials:
            admin: <your_apisix_admin_password>    # Change from default!
      ingress:
        enabled: true
        annotations:
          cert-manager.io/cluster-issuer: "prod"
          cert-manager.io/private-key-algorithm: "ECDSA"
        hosts:
          - host: <your_data_service_domain>       # Domain for the protected data service
            paths: ["/"]
        tls:
          - secretName: <your_apisix_tls_secret>
            hosts:
              - <your_data_service_domain>
      routes:
        # Route: OpenID Configuration for the data service (VCVerifier)
        - uri: /.well-known/openid-configuration
          host: <your_data_service_domain>
          upstream:
            nodes:
              verifier:3000: 1
            type: roundrobin
          plugins:
            proxy-rewrite:
              uri: /services/data-service/.well-known/openid-configuration
        # Route: Data service requests (authenticated + authorized)
        - uri: /*
          host: <your_data_service_domain>
          upstream:
            nodes:
              <your_backend_service>: 1            # Upstream to your actual data service
            type: roundrobin
          plugins:
            openid-connect:
              bearer_only: true
              use_jwks: true
              client_id: data-service
              client_secret: unused
              ssl_verify: true            # see "Trusting a private CA" below if your certificates are not publicly trusted
              discovery: https://<your_verifier_domain>/services/data-service/.well-known/openid-configuration
            opa:
              host: http://localhost:8181
              policy: policy/main

    # ODRL-PAP — Policy Administration Point
    odrl-pap:
      additionalEnvVars:
        - name: GENERAL_ORGANIZATION_DID
          value: <your-organization's-DID>
      ingress:
        enabled: true
        annotations:
          cert-manager.io/cluster-issuer: "prod"
          cert-manager.io/private-key-algorithm: "ECDSA"
          cert-manager.io/common-name: "<your_pap_domain>"
        hosts:
          - host: <your_pap_domain>
            paths:
              - "/"
        tls:
          - secretName: <your_pap_domain>-tls
            hosts:
              - <your_pap_domain>

# --- Enabled by default: set to false if you do not need them ---
# Scorpio Context Broker (TMForum backend — see note on Scorpio above)
scorpio:
  enabled: true
tm-forum-api:
  enabled: true         # TMForum APIs: product catalog, ordering, ...
contract-management:
  enabled: true         # contract-based access, needs the TMForum API

# --- Disabled by default (enable as needed) ---
marketplace:
  enabled: false        # local marketplace portal (BAE), needs MongoDB
fdsc-edc:
  enabled: false        # Dataspace Protocol connector
identityhub:
  enabled: false        # only needed with the Decentralized Claims Protocol
vault:
  enabled: false
```

A provider without any of the optional components sets `scorpio`, `tm-forum-api` and `contract-management` to `false`: leaving them out of the values file deploys them.

2. Install the chart with your values, pinning its version (see the [release notes](../../../release-notes) for what changes between versions):

```shell
helm install provider oci://quay.io/fiware/helm-charts/data-space-connector \
  --version <version> \
  -n provider \
  --create-namespace \
  -f provider-values.yaml
```

   The chart is also available from the Helm repository `https://fiware.github.io/data-space-connector`:

```shell
helm repo add fiware-dsc https://fiware.github.io/data-space-connector
helm install provider fiware-dsc/data-space-connector --version <version> -n provider --create-namespace -f provider-values.yaml
```

See [quick-start/values/provider.yaml](../../quick-start/values/provider.yaml) for a complete minimal example.

#### Other deployment options

A base example for a provider deployment can be found at [k3s/provider.yaml](../../../../k3s/provider.yaml).

> **Note:** These values are intended for **local development and testing** environments. For a real production deployment, they must be customized to meet your infrastructure, security, and availability requirements. See [Production considerations](#production-considerations) below.

For other deployment scenarios, the base `provider.yaml` can be extended with additional overlay values files (which take precedence over the base). The following overlays are available:

| Overlay | File | Description |
|---------|------|-------------|
| **eIDAS / ELSI** | [k3s/provider-eidas.yaml](../../../../k3s/provider-eidas.yaml) | Configuration for deployments compliant with the [ELSI](https://alastria.github.io/did-method-elsi/) trust framework, including [eIDAS 2.0 trust list validation](../../eidas/README.md) via VCVerifier's built-in PKIX chain verification against the EU Trusted Lists |
| **Gaia-X** | [k3s/provider-gaia-x.yaml](../../../../k3s/provider-gaia-x.yaml) | Configuration for [Gaia-X](https://gaia-x.eu/) compliant deployments |
| **DSP** | [k3s/dsp-provider.yaml](../../../../k3s/dsp-provider.yaml) | Enables the [Dataspace Protocol (DSP)](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol) connector on the provider side |

For example, to deploy a provider with ELSI compliance:

```shell
helm install provider oci://quay.io/fiware/helm-charts/data-space-connector --version <version> \
  -n provider \
  -f k3s/provider.yaml \
  -f k3s/provider-eidas.yaml
```

## Production considerations

### TLS certificates

- All publicly reachable endpoints **must** use HTTPS with valid TLS certificates issued by a trusted CA
- You can enable [cert-manager](https://cert-manager.io/) directly in the FIWARE DSC Helm chart to automate certificate issuance and renewal

### DID and key management

- Use a registered domain name that your organization controls for the `did:web` identifier
- Ensure the DID document is served over HTTPS with a valid certificate
- Protect the private key associated with your DID — it is the foundation of your organization's identity in the data space. If compromised, an attacker could issue credentials on your behalf

#### Identity key lifecycle

The DID's private key is what did-helper publishes and what Keycloak signs credentials with. If it changes, the DID document changes with it, every credential signed with the previous key stops verifying, and the participants that resolved the DID see a different key without notice.

Since cert-manager 1.18, a certificate's `privateKey.rotationPolicy` defaults to `Always`: every renewal - every 60 days with Let's Encrypt - generates a new key. Whatever certificate holds the DID's key must therefore set `Never`. There are two ways to do it:

- **On the DID's Ingress**, as the example above does: the `cert-manager.io/private-key-rotation-policy: "Never"` annotation makes cert-manager keep the key of the certificate it issues for that Ingress, and the same Secret serves TLS and holds the identity key. It is the smallest change, but ties the identity to that Ingress: changing its controller, host or issuer touches the identity key too.
- **A dedicated `Certificate`** for the identity, separate from any Ingress, which only cert-manager renews and which keeps its key. Mount its Secret in did-helper and in the Keycloak keystore init container ([Keycloak guide](../KEYCLOAK.md)) instead of the Ingress' one; the Ingresses keep their own certificates, so a TLS rotation or compromise does not reach the identity:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: identity-key
  namespace: <your_namespace>
spec:
  secretName: <your_identity_key_secret>
  commonName: <your_did_domain>
  dnsNames:
    - <your_did_domain>
  issuerRef:
    name: <your_cluster_issuer>
    kind: ClusterIssuer
  privateKey:
    algorithm: ECDSA
    size: 256
    rotationPolicy: Never      # renewals keep the key, and with it the DID
```

Either way, every component that holds a copy of the key has to follow it: restarting did-helper and Keycloak on a changed Secret keeps them consistent, but credentials signed with a previous key still stop verifying, and a component that stored the key elsewhere - such as the identity hub, in Vault - keeps the old one. Hence `Never`.

Use separate keys for everything else that signs, such as the verifier's request objects (`clientIdentification`) and tokens (see [VCVerifier](#vcverifier)): sharing the identity key spreads it across components that do not need it. The local marketplace needs no key of its own when it logs users in through the verifier's OpenID Connect discovery, as [Running a local marketplace](./LOCAL_MARKETPLACE.md#login-with-verifiable-credentials) describes; its SIOP mode, instead, has to sign with the identity key itself, since the verifier checks those signatures against the DID.

Keep a backup of the identity key outside the cluster: losing it means a new DID. Rotating it on purpose works the same way as losing it - did-helper publishes a single key - so plan it as a re-issuance of the credentials signed with the old one, as described for the identity hub in [Vault](../../production/VAULT.md#rotating-the-signing-key).

### Do not expose internal APIs

The following ingresses are useful for development but must **not** be publicly accessible in production:
- **Scorpio Broker** direct ingress — all access must go through APISIX
- **ODRL-PAP** ingress — policy management should be restricted to authorized operators
- **Trusted Issuers List** management API — restrict to internal access
- **VCVerifier configuration API** (`vcverifier.configIngress`, port 8090) — it defines which credentials each service accepts; keep it internal, and only publish it, restricted, if services have to be managed from outside the cluster

### APISIX configuration

- Change the APISIX `admin` credentials from defaults
- Configure proper error responses (do not leak internal service details)
- Keep `ssl_verify: true` in the `openid-connect` plugin. Even when `discovery` points to the verifier inside the cluster, the discovery document announces the verifier's public `jwks_uri`, so APISIX fetches the keys that validate every token over HTTPS. Without verification, whoever can intercept that call can make APISIX accept forged tokens.

#### Trusting a private CA

If the verifier's certificate is issued by a private CA, give APISIX that CA instead of disabling the verification. Put the CA certificate (PEM) in a Secret, for example `private-ca` with the key `ca.crt`, and point the apisix chart to it:

```yaml
decentralizedIam:
  odrlAuthorization:
    apisix:
      apisix:
        ssl:
          enabled: true              # the chart only mounts the CA together with its HTTPS listener
          existingCASecret: private-ca
          certCAFilename: ca.crt
```

The apisix chart only mounts the CA, and sets it as APISIX's `ssl_trusted_certificate`, when `apisix.ssl.enabled` is set, which also opens its HTTPS listener (port 9443); that listener needs no further configuration when TLS is terminated at the Ingress.

### VCVerifier

- Configure the `tirAddress` to point to the production Trust Anchor's TIR endpoint
- Ensure the VCVerifier can reach the Trust Anchor over HTTPS with valid certificates
- If using a proxy, configure `HTTPS_PROXY`/`NO_PROXY` appropriately
- The VCVerifier's signing key (used for JWT issuance) must be protected

#### Metrics

The verifier serves its Prometheus metrics, `/metrics`, on port 9090 of the `verifier` Service (`metrics`), apart from port 3000, which its Ingress publishes, so the metrics are not reachable from outside the cluster. Scrape them from inside the cluster, at `http://verifier.<your_namespace>.svc.cluster.local:9090/metrics`. The port is set with `deployment.server.metricsPort`; setting it to the main port (3000) serves the metrics there again, and publishes them with the Ingress.

#### Request object modes

`deployment.verifier.supportedModes` defaults to `urlEncoded`. With `byValue` or `byReference` the verifier signs the request objects it sends to wallets, so it also needs `clientIdentification`; without its `keyPath` the verifier does not start (`Failed to initialize verifier`):

```yaml
decentralizedIam:
  vcAuthentication:
    vcverifier:
      deployment:
        verifier:
          supportedModes: ["byValue", "byReference"]
          clientIdentification:
            id: x509_san_dns:<your_verifier_domain>   # how the verifier identifies itself to wallets
            keyPath: /signing-key/tls.key            # key that signs the request objects
            certificatePath: /signing-key/tls.crt    # certificate embedded in their header
            requestKeyAlgorithm: ES256               # algorithm of that key
        additionalVolumes:
          - name: signing-key
            secret:
              secretName: <your_verifier_domain>-tls # e.g. a cert-manager certificate for the verifier's domain
        additionalVolumeMounts:
          - name: signing-key
            mountPath: /signing-key
```

The verifier reads the algorithm from `requestKeyAlgorithm`; the `keyAlgorithm` listed under `clientIdentification` in the vcverifier chart's values has no effect.

#### Token signing algorithm

The access tokens the verifier issues are validated by APISIX's `openid-connect` plugin, whose library ([lua-resty-openidc](https://github.com/zmartzone/lua-resty-openidc)) only accepts tokens signed with `RS*` or `HS*` algorithms, in every APISIX version up to 3.19 at least. With an `ES256` token it does not look up any key - neither in the JWKS nor in an `x5c` chain - and rejects every request with `401` and `jwt signature verification failed: No trusted certs loaded`. Keep `deployment.verifier.keyAlgorithm` at its default `RS256` for every verifier whose tokens APISIX validates. This only concerns the verifier's tokens; Keycloak can keep signing credentials with `ES256`.

By default (`generateKey: true`) the verifier generates that key at startup and only keeps it in memory, so the tokens issued before a restart stop validating and several replicas do not share a key. In production, provide an RSA key instead, with `generateKey: false` and `keyPath`.

### Policy management

- Define ODRL policies following the principle of least privilege
- Consider versioning your policies (store them in version control and apply via CI/CD)

### Central Marketplace integration

If the provider participates in a [Central Marketplace](../../../CENTRAL_MARKETPLACE.md):

- The Contract Management host exposed through APISIX must be reachable from the Central Marketplace over HTTPS with a valid certificate. The `contractManagement.address` registered in the provider's Organization at the marketplace must match this host exactly — no trailing paths, and the scheme/port used when registering must remain valid from the marketplace's network.
- Coordinate version upgrades with the Central Marketplace operator: changes in the `contract-management` notification contract require both sides to be aligned. Keep the minimum versions noted in [Participating in a Central Marketplace](#participating-in-a-central-marketplace) as a floor.
- Keep the local Trusted Issuers List aligned with the Central Marketplace's `MarketplaceCredential` issuer. If the marketplace rotates its DID, the provider will stop accepting notifications until the TIL and the Trust Anchor's TIR are updated.

### Upgrade strategy

- Review the FIWARE Data Space Connector [release notes](https://github.com/FIWARE/data-space-connector/releases) before each upgrade for breaking changes
- Keep the connector version aligned across all participants in the data space when possible to avoid protocol incompatibilities
