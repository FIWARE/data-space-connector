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
| **Keycloak** | Issues the `MarketplaceCredential` used by the marketplace to authenticate against each provider's Contract Management when sending order notifications. Uses its own realm and `did:web`. |
| **DID Helper** | Publishes the marketplace's DID document. |
| **TMForum API** | Hosts the shared catalog (products, offerings, orders, parties, agreements). |
| **Scorpio Context Broker** | NGSI-LD backend for the TMForum API. Should not be exposed to participants as a data service. |
| **Contract Management** | Configured in central-marketplace mode: `enableCentralMarketplace: true`, `enableOdrlPap: false`, `oid4vp.enabled: true`. On order completion, it authenticates with its `MarketplaceCredential` and notifies the provider's Contract Management. |
| **Marketplace UI (BAE)** | Business API Ecosystem web portal (Logic Proxy + Charging Backend + APIs). Requires `BAE_LP_DATASPACE_ENABLED=true` and `BAE_LP_PURCHASE_ENABLED=true` for data space purchase flows. |
| **VCVerifier + Credentials Config Service + Trusted Issuers List (local)** | Authenticate the providers (and users) that access the marketplace's catalog and ordering APIs. |

The **ODRL-PAP, OPA and data-service protection** components that a Provider deploys are **not** needed in the Central Marketplace — the marketplace does not expose protected data services.

#### Initial policies

Before providers can publish offerings, the Operator must restrict access to the TMForum APIs of the marketplace so only authenticated users with the appropriate role can interact with them. A typical baseline policy allows only users carrying the `REPRESENTATIVE` role in their `LegalPersonCredential` to access the TMForum endpoints.

See the end-to-end flow and the example script `prepare-central-market-policies.sh` in [CENTRAL_MARKETPLACE.md](../../../CENTRAL_MARKETPLACE.md).

#### Database

The Central Marketplace requires its **own dedicated SQL database**, separate from the Trust Anchor's database. It holds the Keycloak realm, the TMForum catalog (products, offerings, orders, agreements), the Contract Management state (notifications, retries) and the BAE user profiles and purchases.

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
