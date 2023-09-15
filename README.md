# FIWARE Data Space Connector

This repository provides a description of 
the [DSBA-compliant](https://data-spaces-business-alliance.eu/wp-content/uploads/dlm_uploads/Data-Spaces-Business-Alliance-Technical-Convergence-V2.pdf) 
FIWARE Data Space Connector.

If you want to head over directly to the implementation, go to the 
FIWARE-Ops [data-space-connector repository](https://github.com/FIWARE-Ops/data-space-connector).


<!-- ToC created with: https://github.com/thlorenz/doctoc -->
<!-- Update with: doctoc README.md -->

<details>
<summary><strong>Table of Contents</strong></summary>

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Overview](#overview)
- [Components](#components)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->


</details>



## Overview

The FIWARE Data Space Connector is an integrated suite of components every organization participating 
in a data space should deploy to “connect” to a data space. Following the DSBA recommendations, it 
allows to: 

* Interface with Trust Services aligned with [EBSI specifications](https://api-pilot.ebsi.eu/docs/apis)
* Implement authentication based on [W3C DID](https://www.w3.org/TR/did-core/) with 
  [VC/VP standards](https://www.w3.org/TR/vc-data-model/) and 
  [SIOPv2](https://openid.net/specs/openid-connect-self-issued-v2-1_0.html#name-cross-device-self-issued-op) / 
  [OIDC4VP](https://openid.net/specs/openid-4-verifiable-presentations-1_0.html#request_scope) protocols
* Implement authorization based on attribute-based access control (ABAC) following an 
  [XACML P*P architecture](https://www.oasis-open.org/committees/tc_home.php?wg_abbrev=xacml)
* Provide compatibility with [ETSI NGSI-LD](https://www.etsi.org/committee/cim) as data exchange API

**Note:** Although the FIWARE Data Space Connector provides compatibility with NGSI-LD as data exchange 
API, it could be also used for any RESTful API by replacing or extending the PDP component of the 
connector.

Technically, the FIWARE Data Space Connector is a [Helm](https://helm.sh/) chart following the 
[app-of-apps pattern](https://argo-cd.readthedocs.io/en/stable/operator-manual/cluster-bootstrapping/) of 
[ArgoCD](https://argo-cd.readthedocs.io/en/stable/), which bundles charts for all the necessary components 
and simplifies the deployment of all these components. In addition, the connector is also provided 
as an [Umbrella-Chart](https://helm.sh/docs/howto/charts_tips_and_tricks/#complex-charts-with-many-dependencies), 
containing all the sub-charts and their dependencies for deployment via Helm.  
Thus, being provided as Helm chart, the FIWARE Data Space Connector can be deployed on 
[Kubernetes](https://kubernetes.io/) environments.



## Components

The following diagram shows a logical overview of the different components of the FIWARE Data Space 
Connector.

![connector-components](img/Connector_Components.png)

Precisely, the connector bundles the following components:

| Component       | Role            | Link |
|-----------------|-----------------|------|
| VCVerifier      | Verifier        | https://github.com/FIWARE/VCVerifier |
| VCWaltid        | Backend for managing credentials and DIDs, supports the verifier and issuer | https://github.com/FIWARE/VCWaltid |
| credentials-config-service | Credentials Config provider for the verifier | https://github.com/FIWARE/credentials-config-service |
| Keycloak + keycloak-vc-issuer plugin | Issuer of VCs | https://www.keycloak.org / https://github.com/FIWARE/keycloak-vc-issuer |
| Orion-LD        | Context Broker  | https://github.com/FIWARE/context.Orion-LD |
| trusted-issuers-list | Acts as Trusted Issuers List by providing an [EBSI Trusted Issuers Registry](https://api-pilot.ebsi.eu/docs/apis/trusted-issuers-registry) API | https://github.com/FIWARE/trusted-issuers-list |
| Kong + kong-plugins-fiware | Kong API-Gateway with the kong-pep-plugin serving as API-Gateway and PEP | https://konghq.com / https://github.com/FIWARE/kong-plugins-fiware |
| dsba-pdp        | DSBA-compliant PDP | https://github.com/FIWARE/dsba-pdp |
| Keyrock         | Authorization Registry (storing role / ABAC-policy mappings) | https://github.com/ging/fiware-idm |
| tmforum-api     | [TMForum APIs](https://www.tmforum.org/oda/open-apis/) for contract management | https://github.com/FIWARE/tmforum-api |
| MongoDB         | Database | https://www.mongodb.com |
| MySQL           | Database | https://www.mysql.com |
| PostgreSQL      | Database | https://www.postgresql.org |

**Note,** that some of the components shown in the diagram above are not implemented yet.





## Description of flows in a data space

This section provides a description of various flows and interactions in a data space involving the FIWARE 
Data Space Connector.


### Onboarding of an organization in the data space

Before participating in a data space, an organization needs to be onboarded at the data space's 
Participant List Service by registering it as trusted participant. The user invoking the onboarding 
process needs to present a VC issued by the organization to the user itself, a VC containing the self 
description of the organization and a VC issued by a trusted Compliancy Service for the organization 
self description. 

The following displays the different steps during the onboarding.

![flows-onboarding](img/Flows_Onboarding.png)

**Steps**

* The organization validates that the VC containing its description as organization is compliant with 
  Gaia-X specifications using the services of a Gaia-X Digital Clearing House (GXDCH) - as a result, a 
  VC is issued by the GXDCH (steps 1-2). That VC will end stored in the wallet of the LEAR either as part 
  of the same process (once the GXDCH implements the OIDC4VCI) or via an issuer of VCs that exists 
  inside the organization (step 3)
* The API for registering the organization is inspired in the DID-Registry API defined by EBSI but 
  extending it to allow: 
  - creation, update and deletion of entries beyond read(ing) of entries
  - authentication with VCs (including the VC issued by a GXDCH)
* Using an onboarding application (or a web portal) the organization’s LEAR requests the authentication 
  into the Participant Lists service which ultimately translates into a request to the Verifier (step 4-6)
  - a page is accessed where a QR for authentication is displayed (step 4)
  - the QR code is scanned through the wallet (step 5) which translates 
  - into a request to the verifier (step 6)
* The verifier checks in the PRP/PAP what VCs it has to request to the wallet (step 7). In principle it will 
  find the following VCs to be requested: a) the LEAR VC accrediting the user as LEAR of the organization, 
  b) the VC containing the description of the organization, and c) the VC issued by some GXDCH acredditing 
  that the previous VC is Gaia-X compliant
* The verifier responds to the previous request sending a VP request to the wallet which responds with 
  the requested VCs (steps 8-9)
* The verifier checks that the LEAR VC has been signed using proper eIDAS certificates and that the GXDCH VC 
  has been issued by a trusted GXDCH (step 10). It finally produces an access token (steps 11-12) which the 
  onboarding application can then use to invoke the EBSI DID-Registry+ API in order to register the 
  organization as data space participant (step 13)







