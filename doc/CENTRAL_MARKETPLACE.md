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

Open:

* notify product to the provider -> handling of cancellation etc.?

1. Register the Provider at the Marketplace:

```shell
  export FANCY_MARKETPLACE_ID=$(curl -X POST http://mp-tmf-api.127.0.0.1.nip.io:8080/tmf-api/party/v4/organization \
    -H 'Accept: */*' \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -d "{
      \"name\": \"Fancy Marketplace Inc.\",
      \"partyCharacteristic\": [
        {
            \"name\": \"did\",
            \"value\": \"${CONSUMER_DID}\" 
        },
        {
            \"name\": \"contract-management\",
            \"value\": {
                \"address\": \"https://contract-management.data-provider.io\",
                \"clientId\":\"contract-management\",
                // maybe
                \"scope\": [\"openid\", \"external-marketplace\"]  
            },
            \"@schemaLocation\": \"to-be-created\"
        }
      ]
    }" | jq '.id' -r); echo ${FANCY_MARKETPLACE_ID} 
```

2. Create specification, referencing the provider:
```shell
   export PRODUCT_SPEC_FULL_ID=$(curl -X 'POST' http://tm-forum-api.127.0.0.1.nip.io:8080/tmf-api/productCatalogManagement/v4/productSpecification \
  -H 'Content-Type: application/json;charset=utf-8' \
  -d "{
    \"brand\": \"M&P Operations\",
    \"version\": \"1.0.0\",
    \"lifecycleStatus\": \"ACTIVE\",
    \"name\": \"M&P K8S\",
    \"relatedParty\": [
        {
            \"id\": \"${FANCY_MARKETPLACE_ID}\",
            \"role\": \"provider\"
        }
    ]
    \"productSpecCharacteristic\": [
      {
        \"id\": \"credentialsConfig\",
        \"name\": \"Credentials Config\",
        \"@schemaLocation\": \"https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/main/schemas/credentials/credentialConfigCharacteristic.json\",
        \"valueType\": \"credentialsConfiguration\",
        \"productSpecCharacteristicValue\": [
          {
            \"isDefault\": true,
            \"value\": {
              \"credentialsType\": \"OperatorCredential\",
              \"claims\": [
                {
                  \"name\": \"roles\",
                  \"path\": \"$.roles[?(@.target==\\\"${PROVIDER_DID}\\\")].names[*]\",
                  \"allowedValues\": [
                    \"OPERATOR\"
                  ]
                }
              ]
            }
          }
        ]
      },
      {
        \"id\": \"policyConfig\",
        \"name\": \"Policy for creation of K8S clusters.\",
        \"@schemaLocation\": \"https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/policy-support/schemas/odrl/policyCharacteristic.json\",
        \"valueType\": \"authorizationPolicy\",
        \"productSpecCharacteristicValue\": [
          {
            \"isDefault\": true,
            \"value\": {
              \"@context\": {
                \"odrl\": \"http://www.w3.org/ns/odrl/2/\"
              },
              \"@id\": \"https://mp-operation.org/policy/common/k8s-full\",
              \"odrl:uid\": \"https://mp-operation.org/policy/common/k8s-full\",
              \"@type\": \"odrl:Policy\",
              \"odrl:permission\": {
                \"odrl:assigner\": \"https://www.mp-operation.org/\",
                \"odrl:target\": {
                  \"@type\": \"odrl:AssetCollection\",
                  \"odrl:source\": \"urn:asset\",
                  \"odrl:refinement\": [
                    {
                      \"@type\": \"odrl:Constraint\",
                      \"odrl:leftOperand\": \"ngsi-ld:entityType\",
                      \"odrl:operator\": \"odrl:eq\",
                      \"odrl:rightOperand\": \"K8SCluster\"
                    }
                  ]
                },
                \"odrl:assignee\": {
                  \"@type\": \"odrl:PartyCollection\",
                  \"odrl:source\": \"urn:user\",
                  \"odrl:refinement\": {
                    \"@type\": \"odrl:LogicalConstraint\",
                    \"odrl:and\": [
                      {
                        \"@type\": \"odrl:Constraint\",
                        \"odrl:leftOperand\": \"vc:role\",
                        \"odrl:operator\": \"odrl:hasPart\",
                        \"odrl:rightOperand\": {
                          \"@value\": \"OPERATOR\",
                          \"@type\": \"xsd:string\"
                        }
                      },
                      {
                        \"@type\": \"odrl:Constraint\",
                        \"odrl:leftOperand\": \"vc:type\",
                        \"odrl:operator\": \"odrl:hasPart\",
                        \"odrl:rightOperand\": {
                          \"@value\": \"OperatorCredential\",
                          \"@type\": \"xsd:string\"
                        }
                      }
                    ]
                  }
                },
                \"odrl:action\": \"odrl:use\"
              }
            }
          }
        ]
      }
    ]
  }" | jq '.id' -r ); echo ${PRODUCT_SPEC_FULL_ID}
```

-> CM extracts spec and provider from the order, extract CM information from org