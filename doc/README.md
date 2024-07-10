# Additional documentation

This directory provides additional and more detailed documentation about the FIWARE Data Space Connector, 
specific flows and its deployment and integration with other frameworks.

<!-- ToC created with: https://github.com/thlorenz/doctoc -->
<!-- Update with: doctoc README.md -->

<details>
<summary><strong>Table of Contents</strong></summary>

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Details about flows and interfaces](#details-about-flows-and-interfaces)
  - [Contract Management](#contract-management)
  - [M2M Service Interaction](#m2m-service-interaction)
- [Deployment / Integration](#deployment--integration)
  - [Local deployment of Minimal Viable Dataspace (helm/k3s)](#local-deployment-of-minimal-viable-dataspace-helmk3s)
  - [Packet Delivery Company (ArgoCD)](#packet-delivery-company-argocd)
  - [Integration with AWS Garnet Framework (formerly AWS Smart Territory Framework)](#integration-with-aws-garnet-framework-formerly-aws-smart-territory-framework)

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





## Deployment / Integration

### Local deployment of Minimal Viable Dataspace (helm/k3s)

This is an example of a "Minimal Viable Dataspace", consisting of a fictitious data service 
provider called M&P Operations Inc. (using the FIWARE Data Space Connector), a data service consumer 
called Fancy Marketplace Co. and the 
data space's trust anchor.

The service is provided by the Scorpio Context via the NGSI-LD API, offering access to 
energy report entities.

The example uses [k3s](https://k3s.io/) and helm for deployment on a local machine.

More information can be found here:
* [Local Deployment](./deployment-integration/local-deployment/LOCAL.MD)



### Packet Delivery Company (ArgoCD)

This is an example of a data service provider called Packet Delivery Company (PDC).

The deployment is performed via 
[GitOps pattern](https://www.gitops.tech/) and [ArgoCD](https://argo-cd.readthedocs.io/en/stable/).

The configuration can be found at the 
[fiware-gitops repository](https://github.com/FIWARE-Ops/fiware-gitops/tree/master/aws/dsba/packet-delivery/data-space-connector).

**Note,** that this is currently being reworked and above repository does not contain the latest configuration.




### Integration with AWS Garnet Framework (formerly AWS Smart Territory Framework)

This is an example of a data service provider that is integrated with the 
[AWS Garnet Framwork (formerly AWS STF)](https://github.com/aws-samples/aws-stf). 

In general, this example deploys a data service provider based on the Data Space Connector, 
but integrating the FIWARE Context Broker from the STF.

More information can be found here:
* [Integration with AWS Garnet Framework](./deployment-integration/aws-garnet/)

