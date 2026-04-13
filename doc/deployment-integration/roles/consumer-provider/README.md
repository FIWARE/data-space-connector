# Consumer + Provider Role

## Overview

In most real-world data spaces, organizations act as **both consumer and provider simultaneously**. They offer their own data or services to the data space while also consuming data from other participants. This is the most common deployment scenario.

> **Important:** To deploy as both consumer and provider, you must follow **both** the [Consumer guide](../consumer/README.md) and the [Provider guide](../provider/README.md). Each guide covers the components, configuration, and deployment steps for its respective role. This document only provides additional recommendations specific to the combined scenario.

## Recommendations

### Do not duplicate DID and Keycloak

An organization has a **single identity** in the data space, regardless of how many roles it plays. When deploying as both consumer and provider:

- **One DID** — The Decentralized Identifier is unique per organization. Do not create separate DIDs for the consumer and provider functions.
- **One Keycloak instance** — A single Keycloak instance issues Verifiable Credentials for both roles: credentials that your users present to other providers (consumer function) and credentials for your own employees/services (provider function). The same realm handles both.

### Evolving from one role to the other

- **Consumer to provider** — If your organization initially deploys as a consumer only and later needs to become a provider, you can add the provider components (VCVerifier, Credentials Config Service, APISIX, OPA, ODRL-PAP, etc.) to the existing deployment. The DID and Keycloak are already in place.
- **Provider to consumer** — If your organization already has a FIWARE Data Space Connector deployed as a provider, it already includes all the components needed to act as a consumer (Keycloak and DID). You only need to configure the Keycloak realm with the appropriate clients, roles, and credential types for the target providers.

## Production considerations

All production considerations from both the [Consumer](../consumer/README.md#production-considerations) and [Provider](../provider/README.md#production-considerations) guides apply. Additionally:

### Shared components

With a single deployment, a failure in shared components (PostgreSQL, Keycloak) affects both consumer and provider functions. Consider deploying Keycloak and PostgreSQL with high availability to mitigate this risk.
