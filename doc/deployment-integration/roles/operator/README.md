# Operator Role: Data Space Governance

## Overview

The Data Space Operator operates the **shared trust infrastructure** of the data space. Unlike consumer or provider roles, the Operator is not a participant in data exchange — it is the **neutral governance authority** that maintains the trust framework enabling all participants to interact securely.

In a FIWARE Data Space, the Operator's primary responsibility is operating the **Trust Anchor**: the central registry where all trusted participants and their credentials are registered.

## Onboarding new participants

### Participant onboarding

The Operator is also responsible for the **onboarding of new organizations** into the data space. This process includes validating the required documentation and registering the organization in the Trust Anchor.

1. Verify the organization's identity and legitimacy (out-of-band process)
2. Register the organization's DID at the Trust Anchor
3. Define which credential types the organization is allowed to issue
4. Communicate the Trust Anchor's TIR endpoint to the new participant

To support this task, an [Onboarding Portal](#onboarding-portal) is available as an optional component.

## Required components

### Trusted Issuers Registry (TIR)

The [Trusted Issuers Registry](https://github.com/FIWARE/trusted-issuers-list) implements the [EBSI Trusted Issuers Registry API](https://hub.ebsi.eu/apis/pilot/trusted-issuers-registry/v4), providing a standardized way to query which organizations are trusted in the data space.

- Exposes a read-only API (`/v4/issuers`) for participants to verify trust
- Backed by a management API (`/issuer`) for registering and managing trusted issuers

> **Note:** The FIWARE [Trusted Issuers Registry](https://github.com/FIWARE/trusted-issuers-list) is the default implementation shipped with the FIWARE Data Space Connector, but it is not the only option. Any component that exposes the [EBSI Trusted Issuers Registry API](https://hub.ebsi.eu/apis/pilot/trusted-issuers-registry/v4) can be used as the Trust Anchor's registry — including **blockchain-backed implementations** such as the [EBSI Trusted Issuers Registry](https://hub.ebsi.eu/apis/pilot/trusted-issuers-registry/v4) itself. This allows data spaces to align the trust registry with the governance and decentralization model that best fits their requirements.

### Trusted Issuers List (TIL) management API

The management interface allows the Operator to:
- Register new participants (by their DID) as trusted issuers
- Define which credential types each issuer is allowed to issue
- Remove participants that are no longer trusted

### Database (SQL)

The Trust Anchor requires an SQL database to persist the list of trusted issuers.

**Important:** The Trust Anchor must have its own **dedicated database instance**, completely separate from any participant's infrastructure. Since the Operator is a neutral entity independent of any provider or consumer, sharing database infrastructure would compromise the neutrality and security of the trust registry.

## Optional components

### Onboarding Portal

The [Onboarding Portal](https://github.com/SEAMWARE/On-Boarding-Portal/tree/main) is a web application that streamlines the process of onboarding new organizations into the data space. It provides a user interface for:

- Submitting and reviewing the documentation required to join the data space
- Validating the organization's identity and DID
- Registering the organization in the Trust Anchor upon approval

The Onboarding Portal simplifies the Operator's work by replacing manual registration processes with a guided workflow. See the [Onboarding Portal documentation](https://github.com/SEAMWARE/On-Boarding-Portal/tree/main) for deployment and configuration details.

### Central Marketplace

Some data spaces run a **shared Central Marketplace** where providers publish offerings and consumers browse and purchase access. In a neutral governance model, the Operator is the natural host for such a marketplace, since it is already the non-participant entity trusted by every organization in the data space.

> **Note:** The Central Marketplace is **not a Provider**. It does not host data services or protect them with a PAP; it only provides a catalog, an ordering API and a UI. Its role is to broker purchases and notify the actual data providers so they can enforce access on their own infrastructure.

Hosting the Central Marketplace is **optional** and independent from operating the Trust Anchor. Both responsibilities can be concentrated in the same actor or split among different ones. In either case, the Central Marketplace must be deployed with its **own DID, its own database and its own Kubernetes namespace** — never sharing infrastructure with the Trust Anchor's TIR.

#### Required components

The Central Marketplace is deployed using the same `fiware/data-space-connector` Helm chart as a Provider, but with a specific subset of components enabled:

| Component | Purpose |
|-----------|---------|
| **Keycloak** | Issues the credential used by the marketplace to authenticate against each provider's Contract Management when sending order notifications. Uses its own realm and `did:web`. In this documentation and in the shipped examples this credential is a `MarketplaceCredential`, but **the credential type is not fixed** — any type can be used as long as the marketplace's Keycloak issues it and the providers accept it (see [Authentication credential type](#authentication-credential-type)). |
| **DID Helper** | Publishes the marketplace's DID document. |
| **TMForum API** | Hosts the shared catalog (products, offerings, orders, parties, agreements). All interactions with the marketplace (organization registration, product specification, offering, ordering and order completion) can be performed directly through this API — see the end-to-end flow in [CENTRAL_MARKETPLACE.md](../../../CENTRAL_MARKETPLACE.md). |
| **Scorpio Context Broker** | NGSI-LD backend for the TMForum API. Should not be exposed to participants as a data service. |
| **Contract Management** | Configured in central-marketplace mode: `enableCentralMarketplace: true`, `enableOdrlPap: false`, `oid4vp.enabled: true`. On order completion, it authenticates with the marketplace's credential and notifies the provider's Contract Management. |
| **VCVerifier + Credentials Config Service + Trusted Issuers List (local)** | Authenticate the providers (and users) that access the marketplace's catalog and ordering APIs. |

The **ODRL-PAP, OPA and data-service protection** components that a Provider deploys are **not** needed in the Central Marketplace — the marketplace does not expose protected data services.

#### Optional components

| Component | Purpose |
|-----------|---------|
| **Marketplace UI (BAE)** | Business API Ecosystem web portal (Logic Proxy + Charging Backend + APIs) on top of the TMForum API. Offers a user-friendly interface for participants to register, publish offerings and purchase access — all the same actions that can be performed directly through the TMForum APIs. Requires `BAE_LP_DATASPACE_ENABLED=true` and `BAE_LP_PURCHASE_ENABLED=true` for data space purchase flows, plus its own MongoDB. |

> **Note:** The Marketplace UI is **not strictly required** for the Central Marketplace to be operational — a headless deployment (TMForum API + Contract Management only) is valid, and all the flows documented in [CENTRAL_MARKETPLACE.md](../../../CENTRAL_MARKETPLACE.md) can be driven exclusively through the TMForum APIs.
> However, for **production deployments it is highly recommended** to deploy the UI, since it provides a usable experience for end-users (sellers publishing offerings, customers browsing and buying). A UI-less deployment is a reasonable choice only when the scenario is **purely machine-to-machine** and participants integrate directly with the TMForum APIs from their own systems.

#### Database

The Central Marketplace requires its **own dedicated SQL database**, separate from the Trust Anchor's database. It holds the Keycloak realm, the TMForum catalog (products, offerings, orders, agreements) and the Contract Management state (notifications, retries).

If the [Marketplace UI (BAE)](#optional-components) is also deployed, its Charging Backend additionally requires a **MongoDB** instance (used for the BAE user profiles and purchases). MongoDB can be deployed alongside the chart using the MongoDB Community Operator via `mongo-operator.enabled: true` and `managedMongo.enabled: true`. If the UI is not deployed, MongoDB is not needed.

#### Deployment values

The Central Marketplace uses the `fiware/data-space-connector` Helm chart with a different subset of components enabled than a Provider. The snippets below show the key values required on top of a standard deployment — the full list of components (ingresses, TLS, Keycloak realm, credential definitions, etc.) follows the same patterns documented for the [Provider role](../provider/README.md#deployment). See also the local demo overlays in [`k3s/consumer.yaml`](../../../../k3s/consumer.yaml), [`k3s/consumer-auth.yaml`](../../../../k3s/consumer-auth.yaml) and [`k3s/consumer-tmf.yaml`](../../../../k3s/consumer-tmf.yaml) for a complete example.

**1. Enable Contract Management in central-marketplace mode**

This is the component that reacts to order completions in the marketplace and sends notifications to the provider's Contract Management authenticating with the marketplace's credential.

```yaml
contract-management:
  enabled: true
  did: <marketplace-DID>                    # e.g. did:web:did-central-marketplace.example.org
  enableCentralMarketplace: true            # drive the flow from marketplace orders, not from local ones
  enableOdrlPap: false                      # the marketplace does not run a local PAP
  enableTrustedIssuersList: false           # the marketplace does not manage a TIL for participants
  organization:
    provider:
      role: seller                          # see note below
  oid4vp:
    enabled: true                           # authenticate against providers using the marketplace's credential
    credentialsFolder: /credential-repo
    holder:
      holderId: <marketplace-DID>
      keyType: EC
      keyPath: /app/resources/signing-key/tls.key
      signatureAlgorithm: ECDH-ES
  services:
    product-order:    { url: http://tm-forum-api-svc:8080 }
    party:            { url: http://tm-forum-api-svc:8080 }
    product-catalog:  { url: http://tm-forum-api-svc:8080 }
    service-catalog:  { url: http://tm-forum-api-svc:8080 }
    tmforum-agreement-api: { url: http://tm-forum-api-svc:8080 }
    quote:            { url: http://tm-forum-api-svc:8080 }
  notification:
    enabled: true
    host: contract-management
  deployment:
    image:
      tag: 3.3.8                            # minimum image required for central marketplace mode
```

When a TMForum `ProductOrder` is completed, Contract Management inspects the `relatedParty` list of the underlying `ProductSpecification` to determine which participant is the provider — and therefore to which Contract Management it must send the activation notification. `organization.provider.role` configures the `relatedParty.role` value that Contract Management looks for. The chart default is `provider`, which matches the value written by clients that call the TMForum API directly (as in the [demo flow](../../../CENTRAL_MARKETPLACE.md#create-the-offering-step-2)). When the Marketplace UI (BAE) is deployed instead, it tags the provider with the role `seller`, so **in that case this value must be overridden to `seller`**. If the configured value does not match the role that clients write into `relatedParty`, Contract Management will not be able to resolve the provider from the order and no notification will be sent.

**2. Enable the TMForum API and its NGSI-LD backend**

The TMForum API hosts the shared catalog, orders and parties. It runs in `allInOne` mode on top of a Scorpio Context Broker that is not exposed publicly.

```yaml
tm-forum-api:
  enabled: true
  allInOne:
    enabled: true
  defaultConfig:
    ngsiLd:
      url: http://data-service-scorpio:9090

scorpio:
  enabled: true
  fullnameOverride: data-service-scorpio
  ingress:
    enabled: false                          # NGSI-LD backend must not be publicly reachable
```

**3. (Optional) Enable the Marketplace UI (BAE)**

> The Marketplace UI is an [optional component](#optional-components): it is not required for the Central Marketplace to work, but highly recommended in production so that end-users get a usable web interface. For purely machine-to-machine scenarios, this whole block can be omitted and participants can interact directly with the TMForum API.

The Business API Ecosystem is the web portal that participants use to publish offerings and purchase access. The Logic Proxy must be started with the data-space flags so SIOP login and purchase flows are enabled.

```yaml
marketplace:
  enabled: true
  externalUrl: https://<marketplace-domain>
  siop:
    clientId: <marketplace-DID>
    verifier:
      host: https://<verifier-domain>
      qrCodePath: /api/v2/loginQR
      tokenPath: /token
      jwksPath: /.well-known/jwks
    allowedRoles: [seller, customer, admin]
  bizEcosystemApis:
    tmForum:
      catalog:     { host: tm-forum-api-svc, port: 8080, path: /tmf-api/productCatalogManagement/v4 }
      inventory:   { host: tm-forum-api-svc, port: 8080, path: /tmf-api/productInventory/v4 }
      ordering:    { host: tm-forum-api-svc, port: 8080, path: /tmf-api/productOrderingManagement/v4 }
      party:       { host: tm-forum-api-svc, port: 8080, path: /tmf-api/party/v4 }
      # ... other TMForum sub-APIs (billing, usage, customer, resources, services, ...)
  bizEcosystemLogicProxy:
    ingress:
      enabled: true
      hosts:
        - host: <marketplace-domain>
          paths: ["/"]
      tls:
        - secretName: <marketplace-domain>-tls
          hosts: [<marketplace-domain>]
    additionalEnvVars:
      - { name: BAE_LP_SIOP_IS_REDIRECTION, value: "true" }
      - { name: BAE_LP_PURCHASE_ENABLED,     value: "true" }
      - { name: BAE_LP_DATASPACE_ENABLED,    value: "true" }
      - { name: BAE_LP_SIOP_PRIVATE_KEY_PEM, value: /certs-did/tls.key }
      - { name: BAE_LP_BILLING_ENGINE_URL,   value: "http://<release>-biz-ecosystem-charging-backend.<namespace>.svc.cluster.local:8006/charging/api/orderManagement/orders/preview/" }
```

When this block is enabled, remember to also override `contract-management.organization.provider.role` to `seller` (see step 1), since BAE tags the provider with that role in the `ProductSpecification`'s `relatedParty`.

**4. Issue the marketplace's authentication credential from Keycloak**

The Contract Management sends the notifications authenticated with a Verifiable Credential issued by the marketplace's own Keycloak. In this documentation and in the shipped examples this credential is a `MarketplaceCredential` (`format: jwt_vc`, `scope: MarketplaceCredential`, `vct: MarketplaceCredential`), and the `credentials` helper below fetches it into the pod so Contract Management can present it at each provider.

```yaml
credentials:
  enabled: true
  keycloak:
    address: http://<release>-keycloak:8080
    realm: <realm>
    clientId: account-console
    username: <user>
    scope: openid
    password: <password>
  configurations:
    - id: marketplace-credential
      format: jwt_vc
      targetFile: marketplace-credential.jwt
```

##### Authentication credential type

The `MarketplaceCredential` type is **not mandatory** — it is simply the type used throughout this documentation, the [local demo](../../../CENTRAL_MARKETPLACE.md) and the shipped examples (see [`k3s/consumer.yaml`](../../../../k3s/consumer.yaml) Keycloak realm and [`k3s/provider.yaml`](../../../../k3s/provider.yaml) policies). Any credential type can be used as long as the three sides agree on it:

1. **Keycloak (this realm)** must issue the chosen type — adjust `scope` and `vct` in the `verifiableCredentials.<id>` entry, the `id` under `credentials.configurations`, and the `targetFile` that Contract Management will read.
2. **Central Marketplace CCS + PAP** uses that same type to let its Contract Management present it to providers. No extra configuration is typically needed here.
3. **Each provider's CCS + PAP** must accept that type on the Contract Management endpoint (see step 3 and step 4 of [Participating in a Central Marketplace](../provider/README.md#participating-in-a-central-marketplace) — the `type` field in the `credentials-config-service` scope and the `allowContractManagement` policy at the PAP).

If the type is changed, it must be changed consistently on all three sides or provider-side authentication will fail.

**5. Authentication stack for the marketplace's own APIs**

Providers and users accessing the catalog and ordering APIs authenticate via OID4VP against the marketplace's own VCVerifier + Credentials Config Service + local Trusted Issuers List. The values mirror those of a Provider ([see Provider role](../provider/README.md#deployment)), but the `credentials-config-service.registration` must expose a `LegalPersonCredential`-based scope for the catalog/ordering endpoints (so any registered participant can log in to publish or buy offerings).

> **Note:** The Central Marketplace does **not** deploy the ODRL-PAP + OPA data-service-protection stack. Its only protected surface is the TMForum API behind APISIX with OID4VP auth.

#### k3s/ reference values

The local demo (`mvn clean deploy -Plocal,central`) deploys the Central Marketplace by reusing the consumer base chart together with two overlays that add the marketplace functionality. The same set of files can be used as a reference when preparing a production values file:

| File | Description |
|------|-------------|
| [k3s/consumer.yaml](../../../../k3s/consumer.yaml) | Base values for the organization hosting the Central Marketplace (Keycloak, DID, database, etc.) |
| [k3s/consumer-auth.yaml](../../../../k3s/consumer-auth.yaml) | Authentication stack (VCVerifier, Credentials Config Service, Trusted Issuers List, APISIX routes) required to protect the marketplace APIs |
| [k3s/consumer-tmf.yaml](../../../../k3s/consumer-tmf.yaml) | TMForum API, Scorpio backend and Contract Management configured in central-marketplace mode (`enableCentralMarketplace: true`, `oid4vp.enabled: true`) |

Deploying the Central Marketplace with the same layout:

```shell
helm install central-marketplace fiware/data-space-connector \
  -n central-marketplace \
  --create-namespace \
  -f k3s/consumer.yaml \
  -f k3s/consumer-auth.yaml \
  -f k3s/consumer-tmf.yaml
```

#### Reference

- Architecture and end-to-end demo flow: [CENTRAL_MARKETPLACE.md](../../../CENTRAL_MARKETPLACE.md)
- Local demo deployment: `mvn clean deploy -Plocal,central`

## Helm chart

The Trust Anchor uses a dedicated Helm chart, separate from the participant chart:

```
fiware/trust-anchor
```

See the [quickstart values](../../quick-start/values/trust-anchor.yaml) for a minimal example.

## Production deployment

### Dedicated infrastructure

The Trust Anchor should be deployed on **infrastructure managed independently** from any participant:
- Separate Kubernetes cluster, or at minimum a dedicated namespace with strict RBAC
- Dedicated database instance (not shared with provider or consumer databases)
- Independent operations team with its own access controls

### High availability

- Deploy the Trusted Issuers Registry with multiple replicas behind a load balancer
- Ensure the TIR API remains available even during rolling updates

### Persistence and backups

- Enable persistence for the SQL database with durable storage
- Configure automated backups with a recovery point objective (RPO) appropriate for your data space
- Test restore procedures regularly
- The Trust Anchor's data is critical — if it's lost, all participants lose their trust relationships

### TLS and network security

- Use valid TLS certificates from a real CA (not self-signed)
- The TIR read API (`/v4/issuers`) should be publicly accessible (participants need to query it)
- The TIL management API (`/issuer`) must be **strictly restricted** — only authorized operators should be able to register or remove issuers

### Integration with trust frameworks

For production data spaces, consider integrating with established trust frameworks:
- [Gaia-X Digital Clearing Houses (GXDCH)](https://gaia-x.eu/gxdch) — for Gaia-X compliance. See [Gaia-X Integration](../../../GAIA_X.MD)
- [EBSI](https://ec.europa.eu/digital-building-blocks/sites/display/EBSI) — for European Blockchain Services Infrastructure compatibility
- Custom trust frameworks defined by the data space governance body

## Monitoring

Key metrics to monitor:
- TIR API availability and response times
- Database connections and storage usage
- Number of registered issuers (unexpected changes may indicate unauthorized access)
- Certificate expiry dates
- Audit logs of all management API operations (registrations, removals)
