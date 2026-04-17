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
- [Deployment / Integration](#deployment--integration)
  - [Quick Start Guide](#quick-start-guide)
  - [Deployment by Role](#deployment-by-role)
  - [Local Deployment (Maven)](#local-deployment-maven)
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

### Quick Start Guide

An automated minimal deployment of a complete data space (trust anchor + provider + consumer) for **learning and development purposes**. Its goal is not to show how to deploy each component individually, but to provide a running environment where you can explore the flows and understand how a FIWARE DSC-based Data Space works.

* [Quick Start Guide](./deployment-integration/quick-start/README.md)

> **Note:** The Quick Start Guide is not intended for production deployments. For deploying in a real environment, see the role-based guides below.

### Deployment by Role

Documentation for deploying the FIWARE DSC according to your organization's role in the data space. Each guide describes the required and optional components, architecture, and production considerations.

* [Deployment by Role — Overview](./deployment-integration/roles/README.md)
  * [Consumer](./deployment-integration/roles/consumer/README.md)
  * [Provider](./deployment-integration/roles/provider/README.md)
  * [Consumer + Provider](./deployment-integration/roles/consumer-provider/README.md)
  * [Operator (Data Space Governance)](./deployment-integration/roles/operator/README.md)

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

### Integration with AWS Garnet Framework (formerly AWS Smart Territory Framework)

This is an example of a data service provider that is integrated with the 
[AWS Garnet Framwork (formerly AWS STF)](https://github.com/aws-samples/aws-stf). 

In general, this example deploys a data service provider based on the Data Space Connector, 
but integrating the FIWARE Context Broker from the STF.

More information can be found here:
* [Integration with AWS Garnet Framework](./deployment-integration/aws-garnet/)

