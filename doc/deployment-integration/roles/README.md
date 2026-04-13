# Deployment by Role

In a FIWARE Data Space, each organization plays one or more roles. Each role requires a different set of components from the FIWARE Data Space Connector. This section provides deployment guidance specific to each role.

If you are new to the FIWARE DSC, start with the [Quick Start Guide](../quick-start/README.md) to understand how all the components work together before deploying for production.

## Roles in a Data Space

| Role | Description |
|------|-------------|
| [**Consumer**](consumer/README.md) | Retrieves data or services from other participants. Needs the ability to issue Verifiable Credentials for its users. |
| [**Provider**](provider/README.md) | Offers data or services to other participants. Needs to verify incoming credentials, enforce access policies, and host data services. |
| [**Consumer + Provider**](consumer-provider/README.md) | Acts as both consumer and provider. This is the most common scenario in real-world data spaces. |
| [**Admin**](admin/README.md) | Operates the shared trust infrastructure of the data space (Trust Anchor, Onboarding services, etc). This role is typically fulfilled by a neutral governance body, not by a data space participant. |

## Component matrix

The following table shows which components are required, optional, or not applicable for each role:

<table>
    <thead>
        <tr>
            <th>Umbrella component</th>
            <th>Sub-umbrella component</th>
            <th>Component</th>
            <th>Role</th>
            <th>Consumer</th>
            <th>Provider</th>
            <th>Consumer + Provider</th>
            <th>Admin</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td rowspan="6"><b><a href="https://github.com/FIWARE/decentralized-iam">decentralized-iam</a></b></td>
            <td rowspan="3"><a href="https://github.com/FIWARE/vc-authentication">vc-authentication</a></td>
            <td><a href="https://github.com/FIWARE/VCVerifier">VCVerifier</a></td>
            <td>Validates VCs and exchanges them for tokens</td>
            <td>-</td>
            <td>Required</td>
            <td>Required</td>
            <td>-</td>
        </tr>
        <tr>
            <td><a href="https://github.com/FIWARE/credentials-config-service">credentials-config-service</a></td>
            <td>Holds the information which VCs are required for accessing a service</td>
            <td>-</td>
            <td>Required</td>
            <td>Required</td>
            <td>-</td>
        </tr>
        <tr>
            <td><a href="https://github.com/FIWARE/trusted-issuers-list">trusted-issuers-list</a></td>
            <td>Acts as Trusted Issuers List by providing an <a href="https://api-pilot.ebsi.eu/docs/apis/trusted-issuers-registry">EBSI Trusted Issuers Registry</a> API</td>
            <td>-</td>
            <td>Required</td>
            <td>Required</td>
            <td>-</td>
        </tr>
        <tr>
            <td rowspan="3"><a href="https://github.com/FIWARE/odrl-authorization">odrl-authorization</a></td>
            <td><a href="https://apisix.apache.org/">APISIX</a></td>
            <td>APISIX as API-Gateway with an OPA plugin</td>
            <td>-</td>
            <td>Required</td>
            <td>Required</td>
            <td>-</td>
        </tr>
        <tr>
            <td><a href="https://www.openpolicyagent.org/">OPA</a></td>
            <td>OpenPolicyAgent as the API Gateway's Sidecar</td>
            <td>-</td>
            <td>Required</td>
            <td>Required</td>
            <td>-</td>
        </tr>
        <tr>
            <td><a href="https://github.com/SEAMWARE/odrl-pap">odrl-pap</a></td>
            <td>Allowing to configure ODRL policies to be used by the OPA</td>
            <td>-</td>
            <td>Required</td>
            <td>Required</td>
            <td>-</td>
        </tr>
        <tr>
            <td>-</td>
            <td>-</td>
            <td><a href="https://www.keycloak.org">Keycloak</a></td>
            <td>Issuer of VCs (OID4VCI)</td>
            <td>Required</td>
            <td>Required</td>
            <td>Required</td>
            <td>-</td>
        </tr>
        <tr>
            <td>-</td>
            <td>-</td>
            <td>DID</td>
            <td>Organization's DID document</td>
            <td>Required</td>
            <td>Required</td>
            <td>Required</td>
            <td>-</td>
        </tr>
        <tr>
            <td>-</td>
            <td>-</td>
            <td><a href="https://github.com/FIWARE/tmforum-api">tmforum-api</a></td>
            <td>Implementation of the <a href="https://www.tmforum.org/oda/open-apis/">TMForum APIs</a> for handling contracts</td>
            <td>-</td>
            <td>Optional</td>
            <td>Optional</td>
            <td>-</td>
        </tr>
        <tr>
            <td>-</td>
            <td>-</td>
            <td><a href="https://github.com/FIWARE/contract-management">contract-management</a></td>
            <td>Notification listener for contract management events out of TMForum</td>
            <td>-</td>
            <td>Optional</td>
            <td>Optional</td>
            <td>-</td>
        </tr>
        <tr>
            <td>-</td>
            <td>-</td>
            <td><a href="https://github.com/SEAMWARE/fdsc-edc">FDSC-EDC</a></td>
            <td>Eclipse Dataspace Components (Dataspace Protocol)</td>
            <td>Optional</td>
            <td>Optional</td>
            <td>Optional</td>
            <td>-</td>
        </tr>
        <tr>
            <td>-</td>
            <td>-</td>
            <td><a href="https://github.com/FIWARE/trusted-issuers-list">Trusted Issuers Registry</a></td>
            <td>Central registry of trusted participants</td>
            <td>-</td>
            <td>-</td>
            <td>-</td>
            <td>Required</td>
        </tr>
        <tr>
            <td>-</td>
            <td>-</td>
            <td><a href="https://www.postgresql.org">PostgreSQL</a></td>
            <td>PostgreSQL Database with <a href="https://postgis.net/">PostGIS extensions</a></td>
            <td>Required</td>
            <td>Required</td>
            <td>Required</td>
            <td>Required</td>
        </tr>
    </tbody>
</table>

### Legend

- **Required**: The component must be deployed for this role to function.
- **Optional**: The component adds capabilities but is not needed for the basic role.
- **-**: The component is not relevant for this role.

## Choosing optional components

### TMForum API + Contract Management + Marketplace

Enable these if your data space uses a **marketplace model** where data products and services are offered, browsed, and ordered through a catalog. The TMForum APIs provide a standardized interface for product lifecycle management, and the Contract Management component automates access control based on orders.

### FDSC-EDC (Dataspace Protocol)

Enable this if your data space requires compliance with the **Dataspace Protocol (DSP)** as defined by the International Data Spaces Association (IDSA) / Eclipse Dataspace Components. This is relevant for interoperability with other DSP-compliant connectors.