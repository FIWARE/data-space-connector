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

## Required components

> **Note:** The data services that the provider wants to expose through the FIWARE Data Space Connector (APIs, web applications, etc.) are not included in this list. These are external to the connector and must be deployed and managed independently.

### Everything from the Consumer role

The provider needs the same base components as a consumer:
- **Keycloak** — VC issuer for the provider's own users. For detailed configuration instructions, see the [Keycloak Configuration](../KEYCLOAK.md) guide.
- **DID Helper** — publishes the provider's DID document
- **Managed PostgreSQL** — database for IAM components

> **Note:** If the data space uses the [Decentralized Claims Protocol (DCP)](https://eclipse-dataspace-dcp.github.io/decentralized-claims-protocol/v1.0.1/) (typically together with the [Dataspace Protocol](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol) via [FDSC-EDC](#fdsc-edc-dataspace-protocol)), the **DID Helper is replaced by the Identity Hub**, which serves the DID document (and the provider's credentials) as required by DCP. See the [Dataspace Protocol Integration Guide](../../../DSP_INTEGRATION.md) for details.

### Authentication: Verifiable Credential verification

| Component | Description |
|-----------|-------------|
| **VCVerifier** | Verifies incoming Verifiable Credentials via [OID4VP](https://openid.net/specs/openid-4-verifiable-presentations-1_0.html). Checks credential signatures, issuer trust (via the Trust Anchor), and credential type acceptance. Returns a JWT access token on success. |
| **Credentials Config Service** | Configures which credential types are accepted for each protected service. Defines which Trusted Issuers Lists and Trusted Participants Lists to check. |
| **Trusted Issuers List** (local) | The provider's own list of trusted issuers, used in combination with the Trust Anchor's registry to determine which organizations can issue specific credential types. |

### Authorization: Policy enforcement

| Component | Description |
|-----------|-------------|
| **APISIX** | API Gateway that acts as the Policy Enforcement Point (PEP). Validates JWT tokens from VCVerifier and delegates authorization decisions to OPA. All external access to protected services goes through APISIX. |
| **OPA** (Open Policy Agent) | Policy Decision Point (PDP). Evaluates authorization policies (in Rego format, generated from ODRL) for each incoming request and returns allow/deny decisions. |
| **ODRL-PAP** | Policy Administration Point. Allows the provider to define access policies using [ODRL](https://www.w3.org/TR/odrl-model/) (Open Digital Rights Language). Translates ODRL policies into Rego for OPA. |


## Optional components

All optional components listed below can be deployed using the `fiware/data-space-connector` Helm chart by enabling them in your values file.

> **Note on Scorpio Context Broker:** The FIWARE implementation of the [TMForum APIs](https://github.com/FIWARE/tmforum-api) is based on [NGSI-LD](https://www.etsi.org/deliver/etsi_gs/CIM/001_099/009/01.08.01_60/gs_CIM009v010801p.pdf), which is why the `fiware/data-space-connector` Helm chart deploys a [Scorpio Context Broker](https://github.com/ScorpioBroker/ScorpioBroker) as its backend. In the provided examples, this same Scorpio instance is also used as the NGSI-LD endpoint that the provider exposes through the connector. However, **in production**, if you want to expose an NGSI-LD endpoint as a data service, you should deploy a **separate Context Broker** for that purpose and not reuse the Scorpio instance that backs the TMForum APIs.

### TMForum API

The [TMForum API](https://github.com/FIWARE/tmforum-api) implements standardized APIs for product catalog management, ordering, inventory, and more. These APIs are used both by the provider's own local marketplace and by the [central marketplace](../../../CENTRAL_MARKETPLACE.md) if one exists in the data space. Enable this when:
- You want to offer data products through a structured catalog
- You need standardized product ordering workflows
- You plan to use contract-based access control
- You want to publish offerings to a central marketplace

### Contract Management

The [Contract Management](https://github.com/FIWARE/contract-management) component automates access control based on product orders. When a consumer purchases a product offering via the TMForum API, Contract Management reacts to the order and, most importantly, **registers the consumer's DID in the provider's local Trusted Issuers List**, so the consumer's credentials are accepted at the provider when accessing the purchased service. Additionally, if the [Credentials Config Service](https://github.com/FIWARE/credentials-config-service) and the [ODRL-PAP](https://github.com/SEAMWARE/odrl-pap) are deployed, Contract Management also creates the matching credentials configuration and the ODRL access policies that encode the contract terms. Enable this when:
- You use TMForum APIs for product ordering
- Access should be granted/revoked automatically based on contracts
- You need audit trails for data access agreements

> **Note:** If the data space uses a [central marketplace](../../../CENTRAL_MARKETPLACE.md), the Contract Management component requires specific configuration to work with it. See the [Central Marketplace documentation](../../../CENTRAL_MARKETPLACE.md) for details on how to configure Contract Management in that scenario.

### Marketplace (BAE)

A provider can offer its products and services through a marketplace in two ways:

- **Local marketplace** — Deploy a marketplace as part of the provider's own connector. The [Business API Ecosystem](https://github.com/FIWARE/business-api-ecosystem) provides a web portal for browsing and ordering product offerings directly from this provider.
- **Central marketplace** — If the data space operates a shared central marketplace, the provider can publish its offerings there instead of (or in addition to) running its own. See the [Central Marketplace documentation](../../../CENTRAL_MARKETPLACE.md) for details.

Enable the local marketplace when:
- You want a user-facing web interface for your own data marketplace
- Consumers should be able to browse and order offerings through a portal hosted by the provider

> **Note:** The local Marketplace requires MongoDB in addition to the existing PostgreSQL. MongoDB can be deployed via the `fiware/data-space-connector` Helm chart using the MongoDB Community Operator.

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
              ssl_verify: false
              discovery: http://verifier:3000/services/contract-management/.well-known/openid-configuration
            opa:
              host: http://localhost:8181
              policy: policy/main
              with_body: true
```

**3. Register the Contract Management as a service in the Credentials Config Service**

The VCVerifier needs to know which credential types to accept on the Contract Management host. Register the service with a dedicated OIDC scope (`external-marketplace`) that accepts the `MarketplaceCredential`:

```yaml
decentralizedIam:
  vcAuthentication:
    credentials-config-service:
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
      hostUrl: "http://<your_did_domain>"
      certPath: "/certs/tls.crt"
  volumes:
    - name: certs
      secret:
        secretName: <your_did_domain>-tls
        items:
          - key: tls.crt
            path: tls.crt
  volumeMounts:
    - name: certs
      mountPath: /certs
  ingress:
    enabled: true
    className: ""
    annotations:
      traefik.ingress.kubernetes.io/service.passhostheader: "true"
      traefik.ingress.kubernetes.io/router.tls: "true"
      cert-manager.io/cluster-issuer: "prod" # Use self-signed for testing
      cert-manager.io/private-key-algorithm: "ECDSA"
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
          traefik.ingress.kubernetes.io/router.tls: "true"
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
        configRepo:
          configEndpoint: http://credentials-config-service:8080

    # Credentials Config Service — defines which credential types are accepted
    credentials-config-service:
      ingress:
        enabled: true
        annotations:
          cert-manager.io/cluster-issuer: "prod"
          cert-manager.io/private-key-algorithm: "ECDSA"
          cert-manager.io/common-name: "<your_ccs_domain>"
        hosts:
          - host: <your_ccs_domain>
            paths:
              - "/"
        tls:
          - secretName: <your_ccs_domain>-tls
            hosts:
              - <your_ccs_domain>
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
              ssl_verify: false
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

# Scorpio Context Broker (TMForum backend — see note on Scorpio above)
scorpio:
  # enabled by default

# --- Disabled by default (enable as needed) ---
tm-forum-api:
  enabled: false        # Set to true for marketplace features
contract-management:
  enabled: false        # Set to true for contract-based access
marketplace:
  enabled: false        # Set to true for web marketplace portal
fdsc-edc:
  enabled: false        # Set to true for DSP compliance

# Components not needed for provider
credentials:
  enabled: false
identityhub:
  enabled: false
vault:
  enabled: false
```

2. Add the FIWARE DSC helm chart repository:

```shell
helm repo add fiware-dsc https://fiware.github.io/data-space-connector
```

3. Install the chart with your values:

```shell
helm install provider fiware-dsc/data-space-connector \
  -n provider \
  --create-namespace \
  -f provider-values.yaml
```

See [quick-start/values/provider.yaml](../../quick-start/values/provider.yaml) for a complete minimal example.

#### Other deployment options

A base example for a provider deployment can be found at [k3s/provider.yaml](../../../../k3s/provider.yaml).

> **Note:** These values are intended for **local development and testing** environments. For a real production deployment, they must be customized to meet your infrastructure, security, and availability requirements. See [Production considerations](#production-considerations) below.

For other deployment scenarios, the base `provider.yaml` can be extended with additional overlay values files (which take precedence over the base). The following overlays are available:

| Overlay | File | Description |
|---------|------|-------------|
| **ELSI** | [k3s/provider-elsi.yaml](../../../../k3s/provider-elsi.yaml) | Configuration for deployments compliant with the [ELSI](https://github.com/FIWARE/elsi) trust framework |
| **Gaia-X** | [k3s/provider-gaia-x.yaml](../../../../k3s/provider-gaia-x.yaml) | Configuration for [Gaia-X](https://gaia-x.eu/) compliant deployments |
| **DSP** | [k3s/dsp-provider.yaml](../../../../k3s/dsp-provider.yaml) | Enables the [Dataspace Protocol (DSP)](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol) connector on the provider side |

For example, to deploy a provider with ELSI compliance:

```shell
helm install provider fiware/data-space-connector \
  -n provider \
  -f k3s/provider.yaml \
  -f k3s/provider-elsi.yaml
```

## Production considerations

### TLS certificates

- All publicly reachable endpoints **must** use HTTPS with valid TLS certificates issued by a trusted CA
- You can enable [cert-manager](https://cert-manager.io/) directly in the FIWARE DSC Helm chart to automate certificate issuance and renewal

### DID and key management

- Use a registered domain name that your organization controls for the `did:web` identifier
- Ensure the DID document is served over HTTPS with a valid certificate
- Protect the private key associated with your DID — it is the foundation of your organization's identity in the data space. If compromised, an attacker could issue credentials on your behalf

### Do not expose internal APIs

The following ingresses are useful for development but must **not** be publicly accessible in production:
- **Scorpio Broker** direct ingress — all access must go through APISIX
- **ODRL-PAP** ingress — policy management should be restricted to authorized operators
- **Trusted Issuers List** management API — restrict to internal access
- **Credentials Config Service** — restrict to internal access

### APISIX configuration

- Change the APISIX `admin` credentials from defaults
- Configure proper error responses (do not leak internal service details)

### VCVerifier

- Configure the `tirAddress` to point to the production Trust Anchor's TIR endpoint
- Ensure the VCVerifier can reach the Trust Anchor over HTTPS with valid certificates
- If using a proxy, configure `HTTPS_PROXY`/`NO_PROXY` appropriately
- The VCVerifier's signing key (used for JWT issuance) must be protected

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
