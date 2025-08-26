![CENTRAL MARKETPLACE](./img/central_market.png)


1. A Provider employee registers the marketplace in the Provider:

    1.1. As a trusted-issuer of credentials("MarketplaceCredential") required to send notifications

    * check if Gaia-X credential for that purpose exists

    1.2. Register policies that allow the marketplace to send order notifications to the Contract-Management

    * should f.e. check the offer-id

2. Provider employee creates the product-offering at the BAE-Marketplace

* needs to contain the Providers Contract-Management address -> Options: "place"/"channel" in offering, "productSpecCharacteristic" in spec

* needs to contain accessible endpoints for the customer(same options)

3. Customer buys access at the BAE-Marketplace

* BAE interacts with TMForum
* Contract-Management(Marketplace) receives notifications from TMForum

4. Contract Management sends notifcation to the Providers Contract-Management

    4.1. Authenticates with VC at the verifier

    * Credential could either be pre-issued, created via library or a helper component -> to be decided
    
    4.2. Send order notifications to the Contract-Management(through PEP)

5. Contract-Management activates the service:
    
    5.1. Adds the customer to the trusted-issuers-list(according to the order)

    5.2. Creates the policies from the order