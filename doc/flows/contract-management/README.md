# Contract Management

With the [tmforum-api](https://github.com/FIWARE/tmforum-api) and the 
[contract-management](https://github.com/FIWARE/contract-management) notification listener, the FIWARE 
Data Space Connector provides components to perform contract management based on the TMForum APIs. 

Via the TMForum APIs, providers can create product specifications and offerings. Consumer organisations 
can register as a party and place product orders. In the case of a product order, the TMForum APIs will 
send a notification to the contract-management notification listener, which will create an entry at the 
Trusted Issuers List for the consumer organisation. Also compare to the flow descriptions 
of [Consumer registration](#consumer-registration) and [Contract management](#contract-management). 


## Authorization and authentication

The current implementation of the FIWARE Data Space Connector uses the decentralised version of the [i4Trust framework](https://i4trust.github.io/building-blocks/docs/i4Trust-BuildingBlocks_v4.0_UnderReview.pdf) with addtional support for http-path based decisions. This required, since the FIWARE Data Space Connector provides the [TMForum APIs](https://github.com/FIWARE/tmforum-api), instead of NGSI-LD APIs.

> :construction: A more powerful security framework, also supporting attribute-based policies for the TMForum API, is currently build on top of the existing framework within the [DOME Marketplace Project](https://dome-marketplace.eu/).

### Roles

The [example-instance](https://github.com/FIWARE-Ops/fiware-gitops/tree/master/aws/dsba/packet-delivery/data-space-connector) of the Data Space Connector supports two roles:

* the ```PROVIDER```, which is able to create Product Specifications and Offerings on the TMForum-API
* the ```CONSUMER```, which is able to order products through the api.
* the ```LEGAL_REPRESENTATIVE```, which is able to register an organization through the Parties API.

The roles are connected to policies inside the [Authorization Registry](https://github.com/FIWARE-Ops/data-space-connector/tree/main/applications/keyrock). Those policies define the paths and operations available for each role at the TMForum API. In this case its:

PROVIDER: "GET","POST","PUT","DELETE" on all deployed TMForum APIs
CONSUMER: "GET" on the ProductCatalogManagement-API and "GET","POST","PUT" on the ProductOrderingManagement-API
LEGAL_REPRESENTATIVE: "GET", "POST" and "PUT" on the Parties-API 

### Credentials and Role-Assingment

The VerifiableCredentials containing the ```PROVIDER```,```CONSUMER``` and ```LEGAL_REPRESENTATIVE``` roles are issued through the standard mechanisms of the framework, e.g. by each individual participant. The example-instance of the Data Space Connector is in the role of an ```PROVIDER```. Therefore, the user ```standard-employee```(see the [documentation](https://github.com/FIWARE-Ops/fiware-gitops/tree/master/aws/dsba#credentials) for credentials) can request a ```NaturalPersonCredential``` at [PacketDeliveries Keycloak](https://packetdelivery-kc.dsba.fiware.dev/realms/fiware-server/account/#/) with a compliant wallet (as of now, the [demo-wallet.fiware.dev](https://demo-wallet.fiware.dev) can be used).

For the Consumer-Participant, two users are provided:
- the ```legal-representative```(see the [documentation](https://github.com/FIWARE-Ops/fiware-gitops/tree/master/aws/dsba#credentials)) with the role ```LEGAL_REPRESENTATIVE```
- the ```standard-user```(see the [documentation](https://github.com/FIWARE-Ops/fiware-gitops/tree/master/aws/dsba#credentials)) with the role ```CONSUMER```
In order to get the credentials, use the [HappyPets Keycloak](https://happypets-kc.dsba.fiware.dev/realms/fiware-server/account/#/).

> :bulb: The framework would allow users that have both roles already. We decided to split them in order to have a clear flow. 

### Example flow 

> :construction: The [Business API Ecosystem](https://github.com/FIWARE-TMForum/Business-API-Ecosystem) as a Marketplace solution build on top of the TMForum-APIs is currently beeing updated to the latest TMForum-APIs and the security framework. Once its finished, it can be used a Marketplace, including UI, for the Data Space Connector.


To provide a working example for Contract Management, using the Data Space Connector, a [postman colletion is provided](./examples/tmf/).
It includes the following steps of the aquisition process:

> :bulb: Since frontend-solutions are still under construction, plain REST-calls are used for the flow. Since all calls require a valid JWT, the [demo-portal](https://packetdelivery-portal.dsba.fiware.dev/) for the provider has a link to get a plain token in exchange for the Verifiable Credential. Log-in either as CONSUMER or PROVIDER(see [Credentials and Role-Assignemnt](#credentials-and-role-assingment)) to get tokens. 

0. In order to have the consumer registered, it has to be created as an ```Organization``` through the [TMForum Party-API](https://github.com/FIWARE/tmforum-api/tree/main/party-catalog). The registration needs to happen with a direct api-call to the Parties-API, with a token in Role ```LEGAL_REPRESENTATIVE```: [POST /organization](./examples/tmf/tmf.postman_collection.json#l80)

1. Creating an offer as the PROVIDER (use a JWT retrieved for user ```standard-employee``` of [PacketDelivery](https://packetdelivery-kc.dsba.fiware.dev/realms/fiware-server/account/#/)):
    1. Create the product specification
    2. Create the product offering

2.  Create a product order (e.g. buy the product) as the CONSUMER (use a JWT retrieved for user ```standard-user``` of [HappyPets](https://happypets-kc.dsba.fiware.dev/realms/fiware-server/account/#/))

After the product was ordered, a notfication will be triggered towards the [Contract Management Service](https://github.com/FIWARE/contract-management). The service will use the information provided as part of the notification, to add the ```CONSUMER``` Organization as a Trusted Issuer to the [Trusted Issuers List](https://github.com/FIWARE/trusted-issuers-list) of PacketDelivery and therefore allow Happy Pets (as the ```CONSUMER``` Organization) issue credentials to its customers to access Packet Deliveries (e.g. the ```PROVIDER```) Services(see [description of Service Usage in the -Data Space](https://github.com/FIWARE-Ops/fiware-gitops/tree/master/aws/#service-usage)). 


