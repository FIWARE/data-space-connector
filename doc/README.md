# Additional documentation

This directory provides additional and more detailed documentation about the FIWARE Data Space Connector, 
specific flows and its deployment and integration with other frameworks.

<!-- ToC created with: https://github.com/thlorenz/doctoc -->
<!-- Update with: doctoc README.md -->

<details>
<summary><strong>Table of Contents</strong></summary>

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
<!-- param::isNotitle::true:: -->

- [Details about flows and interfaces](#details-about-flows-and-interfaces)
  - [Contract Management](#contract-management)
  - [M2M Service Interaction](#m2m-service-interaction)
  - [Consent Management](#consent-management)
- [Deployment / Integration](#deployment--integration)
  - [Quick Start Guide](#quick-start-guide)
  - [Deployment by Role](#deployment-by-role)
  - [Verifying a deployment](#verifying-a-deployment)
  - [Local Deployment (Maven)](#local-deployment-maven)
  - [Packet Delivery Company (ArgoCD)](#packet-delivery-company-argocd)
  - [Running the EDC identity material in production](#running-the-edc-identity-material-in-production)
  - [Observability](#observability)
  - [eIDAS 2.0 Compliance](#eidas-20-compliance)
  - [Integration with AWS Garnet Framework (formerly AWS Smart Territory Framework)](#integration-with-aws-garnet-framework-formerly-aws-smart-territory-framework)
- [Glossary](#glossary)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->


</details>



## Details about flows and interfaces

### Contract Management

The FIWARE Data Space Connector provides components to perform contract management based on the TMForum APIs. 

More information can be found here:
* [Contract Management](./flows/contract-management) - Information about the Contract Management and its 
  authentication/authorization
* [TMForum contract management example](./flows/contract-management/tmf) - Example requests to interact 
  with the TMForum APIs



### M2M Service Interaction

A detailed description about the steps to be performed in a Machine-To-Machine (M2M) service interaction 
can be found here:
* [Service Interaction (M2M)](./flows/service-interaction-m2m)

### Consent Management

Optional consent management (ISO/IEC TS 27560) - its architecture, trust model and the demo flow charts 
(granting consent, consent-gated data access, authenticated projection) are documented here:
* [Consent Management](./CONSENT_MANAGEMENT.md) - architecture, trust model, participant
  authentication over OID4VP, and the end-to-end demo
* [Architecture Decision Records](./adr/README.md)





## Deployment / Integration

### Quick Start Guide

An automated minimal deployment of a complete data space (trust anchor + provider + consumer) for **learning and development purposes**. Its goal is not to show how to deploy each component individually, but to provide a running environment where you can explore the flows and understand how a FIWARE DSC-based Data Space works.

* [Quick Start Guide](./deployment-integration/local-deployment/LOCAL.MD)

> **Note:** The Quick Start Guide is not intended for production deployments. For deploying in a real environment, see the role-based guides below.

### Deployment by Role

Documentation for deploying the FIWARE DSC according to your organization's role in the data space. Each guide describes the required and optional components, architecture, and production considerations.

* [Deployment by Role — Overview](./deployment-integration/roles/README.md)
  * [Consumer](./deployment-integration/roles/consumer/README.md)
  * [Provider](./deployment-integration/roles/provider/README.md)
  * [Consumer + Provider](./deployment-integration/roles/consumer-provider/README.md)
  * [Operator (Data Space Governance)](./deployment-integration/roles/operator/README.md)

### Verifying a deployment

After installing the connector, [fdsc-verify](https://github.com/SEAMWARE/fdsc-verify) checks a running deployment against its own namespace: whether it is built right (Helm values), wired right (DIDs, JWKS, credentials, certificates, trusted issuers) and whether its flows work (TMForum, EDC negotiation and transfer). It infers the role, reports the cause and the fix for every finding, and `--preflight-only` creates nothing in the cluster.

* [fdsc-verify](https://github.com/SEAMWARE/fdsc-verify#readme)

### Local Deployment (Maven)

For development and testing, a Maven-based local deployment automatically spins up a full data space using k3s and Docker.

* [Local Deployment](./deployment-integration/local-deployment/LOCAL.MD)

### Packet Delivery Company (ArgoCD)

This is an example of a data service provider called Packet Delivery Company (PDC).

The deployment is performed via 
[GitOps pattern](https://www.gitops.tech/) and [ArgoCD](https://argo-cd.readthedocs.io/en/stable/).

The configuration can be found at the 
[fiware-gitops repository](https://github.com/FIWARE-Ops/fiware-gitops/tree/master/aws/dsba/packet-delivery/data-space-connector).

**Note,** that this is currently being reworked and above repository does not contain the latest configuration.

### Running the EDC identity material in production

What the Dataspace-Protocol part of the connector needs beyond a default install: a Vault that
survives a restart, the participant bootstrap, keeping the identityhub's credential copy in step, and
the runbook for rotating the signing key.

* [Production setup for Vault and the participant identity](./deployment-integration/production/VAULT.md)

### Observability

Documentation for enabling distributed tracing across the FIWARE DSC using OpenTelemetry, including how to connect external backends (Jaeger, Tempo, Honeycomb) and per-component configuration notes.

* [Distributed Tracing with OpenTelemetry](./deployment-integration/observability/README.md)
* [Grafana Tempo Backend (In-Cluster)](./deployment-integration/observability/README.md#grafana-tempo-backend-in-cluster) -- Deploy Tempo and Grafana as optional subcharts for a turnkey trace-storage and visualisation stack with zero-config auto-wiring.

### eIDAS 2.0 Compliance

Documentation for enabling eIDAS 2.0 compliance through VCVerifier-native PKIX certificate chain validation against EU Trusted Lists. Covers `did:elsi`, `did:web`, and HTTPS-issued credentials, certificate generation for testing, and per-component configuration.

* [eIDAS 2.0 Compliance](./deployment-integration/eidas/README.md)

### Integration with AWS Garnet Framework (formerly AWS Smart Territory Framework)

This is an example of a data service provider that is integrated with the 
[AWS Garnet Framwork (formerly AWS STF)](https://github.com/aws-samples/aws-stf). 

In general, this example deploys a data service provider based on the Data Space Connector, 
but integrating the FIWARE Context Broker from the STF.

More information can be found here:
* [Integration with AWS Garnet Framework](./deployment-integration/aws-garnet/)

## Glossary

Terms and example names used across this documentation and the examples in [`k3s/`](../k3s).

**Example participants**

| Name | DID | Role in the examples |
|---|---|---|
| M&P Operations ("MP") | `did:web:mp-operations.org` | The **provider**: offers the data service and, in the local deployment, its own marketplace. "MP" is this organization, not "marketplace" |
| Fancy Marketplace | `did:web:fancy-marketplace.biz` | The **consumer**, whose users buy and access M&P Operations' offerings. It also hosts the central marketplace in that scenario |
| Data space authority | `did:web:dataspace-authority.org` | The authority of the consent management scenario |

**Components and terms**

| Term | Meaning |
|---|---|
| BAE | [Business API Ecosystem](https://github.com/FIWARE/business-api-ecosystem), the marketplace portal (`marketplace` in the chart) |
| Central marketplace | A marketplace shared by the data space, where providers publish their offerings, see [CENTRAL_MARKETPLACE.md](./CENTRAL_MARKETPLACE.md) |
| Trust Anchor | The data space's root of trust; its Trusted Issuers Registry lists the participants |
| TIR | Trusted Issuers Registry, the Trust Anchor's list of participants and the credentials they may issue |
| TIL | Trusted Issuers List, each provider's own list of the issuers it trusts for each credential type |
| VCVerifier | Verifies the credentials presented over OID4VP and issues access tokens. It also serves the credentials configuration API (what each service accepts), formerly the separate Credentials Config Service |
| PEP / PDP / PAP | Policy enforcement, decision and administration points: APISIX, OPA and the ODRL-PAP |
| OID4VCI / OID4VP | OpenID for Verifiable Credential Issuance and for Verifiable Presentations: how credentials are issued (by Keycloak) and presented (to the VCVerifier) |
| DSP / DCP | Dataspace Protocol and Decentralized Claims Protocol, used by FDSC-EDC and the Identity Hub, see [DSP_INTEGRATION.md](./DSP_INTEGRATION.md) |
| did-helper | Publishes the organization's `did:web` document |
