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

**Note,** that some of the components shown in the diagram above are not implemented yet.

