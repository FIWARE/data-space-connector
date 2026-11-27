---
title: Home
hide:
  - navigation
  - toc
---

# FIWARE Data Space Connector

The FIWARE Data Space Connector (FIWARE DSC) is a data space connector built from the integration
of open-source components of the [FIWARE Dataspace Components](https://github.com/FIWARE). Any organization
participating in a data space can deploy it to connect, acting as a provider of data (processing)
services, a consumer of them, or both.

[Get Started](deployment-integration/local-deployment/LOCAL.md){ .md-button .md-button--primary }
[Deploy by Role](deployment-integration/roles/README.md){ .md-button }

## Choose your path

<div class="grid cards" markdown>

-   :material-shield-crown:{ .lg .middle } __Operator__

    ---

    Govern the data space: run the Trust Anchor and the Trusted Issuers Registry that lists
    its participants.

    [:octicons-arrow-right-24: Operator guide](deployment-integration/roles/operator/README.md)

-   :material-database-export:{ .lg .middle } __Provider__

    ---

    Offer data (processing) services to the data space, with an optional local marketplace.

    [:octicons-arrow-right-24: Provider guide](deployment-integration/roles/provider/README.md)

-   :material-database-import:{ .lg .middle } __Consumer__

    ---

    Consume data (processing) services offered by providers across the data space.

    [:octicons-arrow-right-24: Consumer guide](deployment-integration/roles/consumer/README.md)

</div>

## What's inside

<div class="grid cards" markdown>

-   :material-rocket-launch:{ .lg .middle } __Quick Start__

    ---

    A minimal, automated data space (trust anchor + provider + consumer) for learning and
    development.

    [:octicons-arrow-right-24: Quick Start](deployment-integration/local-deployment/LOCAL.md)

-   :material-sitemap:{ .lg .middle } __Concepts & Flows__

    ---

    Contract management, machine-to-machine service interaction, consent management and the
    protocol integrations.

    [:octicons-arrow-right-24: Concepts & Flows](flows/contract-management/README.md)

-   :material-cog:{ .lg .middle } __Operations & Production__

    ---

    Running the connector for real: Vault and identity, observability, eIDAS 2.0 and Gaia-X.

    [:octicons-arrow-right-24: Operations](deployment-integration/production/VAULT.md)

-   :material-book-open-variant:{ .lg .middle } __Reference__

    ---

    Glossary and release notes for the connector and its Helm chart.

    [:octicons-arrow-right-24: Reference](glossary.md)

-   :material-source-pull:{ .lg .middle } __Contributor__

    ---

    Understand the design decisions and ongoing work, or contribute to the project.

    [:octicons-arrow-right-24: Contribute](adr/README.md)

</div>

## The components

The connector bundles several open-source frameworks:

- **[Identity & Trust](https://github.com/FIWARE/data-space-connector#oid4vc-based-authentication-framework)** — OID4VC-based authentication: the VCVerifier, the Trusted Issuers
  List/Registry and Keycloak as credential issuer.
- **[Authorization](https://github.com/FIWARE/data-space-connector#authorization-framework)** — ODRL policies enforced through APISIX (PEP), OPA (PDP) and the ODRL-PAP.
- **[Catalog & Contracting](flows/contract-management/README.md)** — the TMForum-based product catalog and contract management.
- **[Dataspace Protocol (EDC)](DSP_INTEGRATION.md)** — FDSC-EDC and the Identity Hub for DSP/DCP-based data exchange.
- **[Marketplace](MARKETPLACE_INTEGRATION.md)** — the Business API Ecosystem portal for publishing and buying offerings.

See the [full component overview on GitHub](https://github.com/FIWARE/data-space-connector#components).
