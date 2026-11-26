# Glossary

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
| Central marketplace | A marketplace shared by the data space, where providers publish their offerings, see [Central Marketplace](./CENTRAL_MARKETPLACE.md) |
| Trust Anchor | The data space's root of trust; its Trusted Issuers Registry lists the participants |
| TIR | Trusted Issuers Registry, the Trust Anchor's list of participants and the credentials they may issue |
| TIL | Trusted Issuers List, each provider's own list of the issuers it trusts for each credential type |
| VCVerifier | Verifies the credentials presented over OID4VP and issues access tokens. It also serves the credentials configuration API (what each service accepts), formerly the separate Credentials Config Service |
| PEP / PDP / PAP | Policy enforcement, decision and administration points: APISIX, OPA and the ODRL-PAP |
| OID4VCI / OID4VP | OpenID for Verifiable Credential Issuance and for Verifiable Presentations: how credentials are issued (by Keycloak) and presented (to the VCVerifier) |
| DSP / DCP | Dataspace Protocol and Decentralized Claims Protocol, used by FDSC-EDC and the Identity Hub, see [DSP Integration](./DSP_INTEGRATION.md) |
| did-helper | Publishes the organization's `did:web` document |
