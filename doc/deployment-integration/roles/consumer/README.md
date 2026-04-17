# Consumer Role

> **Note:** This guide covers the deployment of an organization acting **exclusively as a consumer**. If your organization also needs to act as a provider (offering data or services to other participants), refer to the [Consumer & Provider guide](../consumer-provider/README.md) instead.

## Overview

A Consumer is an organization that **retrieves data or services from other participants** in the data space. The consumer does not host data services itself — it only needs the ability to authenticate its users and issue Verifiable Credentials that can be presented to providers.

## Before you start

Before deploying, contact the **Data Space Operator** to initiate the onboarding process for your organization. The Operator will guide you through:

- The documentation and requirements needed to join the data space
- Registration of your organization's DID at the Trust Anchor
- The credential types your organization is allowed to issue and their required configuration (claims, attribute mappings, etc.)
- The Trust Anchor's TIR endpoint URL

This information is essential for configuring Keycloak, the DID, and the Verifiable Credentials correctly.

## Required components

### Verifiable Credential Issuer

A consumer organization requires a **Verifiable Credential (VC) Issuer** — a component capable of issuing [Verifiable Credentials](https://www.w3.org/TR/vc-data-model/) to its users so they can authenticate and access services offered by providers in the data space.

The FIWARE Data Space Connector uses [Keycloak](https://github.com/keycloak/keycloak) as the VC issuer. For detailed configuration instructions, see the [Keycloak Configuration](../KEYCLOAK.md) guide.

### DID

Every participant in the data space needs a [Decentralized Identifier (DID)](https://www.w3.org/TR/did-core/) — a globally unique identifier that the organization controls and that is not dependent on a centralized registry. DIDs can be of different types (methods), such as `did:key`, `did:web`, `did:ebsi`, among others.

- **`did:web`** — Recommended for **production** environments. It relies on a domain name controlled by the organization and serves the DID document over HTTPS, providing discoverability and a natural trust anchor tied to DNS.
- **`did:key`** — Suitable only for **test or development** environments. It encodes the public key directly in the identifier itself, which makes it simple to generate but offers no discoverability or key rotation capabilities.

The [DID Helper](https://github.com/SEAMWARE/did-helper) is a lightweight service that creates `did:key` or serves the consumer's [DID document](https://www.w3.org/TR/did-core/).

The DID document contains the consumer's public key, which providers use to verify that credentials were issued by this organization. The DID must be registered at the data space's Trust Anchor following the Data Space onboarding process.


## Optional components

### FDSC-EDC (Dataspace Protocol)

If the data space requires compliance with the [Dataspace Protocol (DSP)](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol), the consumer can enable the Eclipse Dataspace Components connector. This adds support for standardized contract negotiation and data transfer protocols.

Enable this when:
- The data space mandates DSP compliance
- You need to interact with non-FIWARE DSP-compliant connectors
- Contract negotiation must follow the IDSA protocol

See [DSP Integration](../../../DSP_INTEGRATION.md) for configuration details.

## Deployment

The **consumer role does not require deploying a FIWARE DSC**, since it only needs a DID and a VC issuer (Keycloak), and that is not a FIWARE Data Space Connector.

However, for easier deployment of these components, you can use the same `fiware/data-space-connector` Helm chart as the provider, but with most components disabled. You only need to enable Keycloak and the DID Helper as mandatory components. If you prefer not to use the FIWARE DSC Helm chart, you can always deploy these components separately using their own Helm charts or deployment methods.

### Using the FIWARE DSC Helm chart and values

The consumer could use the same `fiware/data-space-connector` chart as the provider, but with most components disabled (since it is not a real connector).

1. Create a helm `values.yaml` file with the key settings to enable only the consumer-related components like this:

```yaml
# Key settings for consumer-only deployment

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

decentralizedIam:
  enabled: true
  vcAuthentication:
    vcverifier:
      enabled: false
    credentials-config-service:
      enabled: false
    trusted-issuers-list:
      enabled: false
    managedPostgres: # Only needed if using the built-in PSQL for the keycloak.
      enabled: true
      config:
        volume:
          storageClass: "" # Use default storage class or specify one if needed
  odrlAuthorization:
    enabled: false
    odrl-pap:
      enabled: false
    apisix:
      enabled: false


contract-management:
  enabled: false

fdsc-edc:
  enabled: false

credentials:
  enabled: false

identityhub:
  enabled: false

vault:
  enabled: false

scorpio:
  enabled: false

tm-forum-api:
  enabled: false
```

2. Add the FIWARE DSC helm chart repository:

```shell
helm repo add fiware-dsc https://fiware.github.io/data-space-connector
````

3. Install the chart with your values:

```shell
helm install consumer fiware-dsc/data-space-connector \
  -n consumer \
  --create-namespace \
  -f consumer-values.yaml
```

#### Other deployment options

A base example for a consumer deployment can be found at [k3s/consumer.yaml](../../../../k3s/consumer.yaml).

> **Note:** These values are intended for **local development and testing** environments. For a real production deployment, they must be customized to meet your infrastructure, security, and availability requirements. See [Production considerations](#production-considerations) below.

For other deployment scenarios, the base `consumer.yaml` can be extended with additional overlay values files (which take precedence over the base). The following overlays are available:

| Overlay | File | Description |
|---------|------|-------------|
| **ELSI** | [k3s/consumer-elsi.yaml](../../../../k3s/consumer-elsi.yaml) | Configuration for deployments compliant with the [ELSI](https://github.com/FIWARE/elsi) trust framework |
| **Gaia-X** | [k3s/consumer-gaia-x.yaml](../../../../k3s/consumer-gaia-x.yaml) | Configuration for [Gaia-X](https://gaia-x.eu/) compliant deployments |
| **TM Forum APIs** | [k3s/consumer-tmf.yaml](../../../../k3s/consumer-tmf.yaml) | Enables [TM Forum APIs](https://www.tmforum.org/oda/open-apis/) on the consumer (required for central marketplace and EDC-related features) |
| **Auth components** | [k3s/consumer-auth.yaml](../../../../k3s/consumer-auth.yaml) | Configures authentication components for the consumer (required for central marketplace and EDC-related features) |
| **DSP** | [k3s/dsp-consumer.yaml](../../../../k3s/dsp-consumer.yaml) | Enables the [Dataspace Protocol (DSP)](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol) connector on the consumer side |

For example, to deploy a consumer with DSP support and TM Forum APIs:

```shell
helm install consumer fiware/data-space-connector \
  -n consumer \
  -f k3s/consumer.yaml \
  -f k3s/consumer-auth.yaml \
  -f k3s/consumer-tmf.yaml \
  -f k3s/dsp-consumer.yaml
```

## Production considerations

### TLS certificates

- All publicly reachable endpoints **must** use HTTPS with valid TLS certificates issued by a trusted CA
- You can enable [cert-manager](https://cert-manager.io/) directly in the FIWARE DSC Helm chart to automate certificate issuance and renewal

### DID and key management

- Use a registered domain name that your organization controls for the `did:web` identifier
- Ensure the DID document is served over HTTPS with a valid certificate
- Protect the private key associated with your DID — it is the foundation of your organization's identity in the data space. If compromised, an attacker could issue credentials on your behalf

### Trust Anchor registration

- Register your organization's DID at the data space's Trust Anchor before deploying
- Coordinate with the Data Space Operator regarding:
  - Which credential types your organization is allowed to issue
  - The Trust Anchor's TIR endpoint URL (needed for provider credential verification)

### Credential configuration

- Configure only the credential types that your organization actually needs to issue
- Define appropriate claims and attribute mappings for each credential type
- Set reasonable credential expiry times
- Review credential scopes to ensure they follow the principle of least privilege

### Keycloak

- Do not hardcode passwords in values files — use Kubernetes Secrets
- Rotate Keycloak admin credentials and the keystore password regularly
- For high availability, deploy at least 2 replicas with Infinispan cache replication enabled

### Upgrade strategy

- Review the FIWARE Data Space Connector [release notes](https://github.com/FIWARE/data-space-connector/releases) before each upgrade for breaking changes
- Keep the connector version aligned across all participants in the data space when possible to avoid protocol incompatibilities
