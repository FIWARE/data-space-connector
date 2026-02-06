# Integration of the Dataspace Protocol

In order to be compatible with other connectors, the FIWARE Data Space Connector supports the [IDSA Dataspace Protocol](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol).

## Architecture


![DSP Architecture](./img/dsp-architecture.png)

* [TMForum API](https://github.com/FIWARE/tmforum-api) provides the APIs for creating and managing Products and Offerings
    * integrates with the Contract Management through Events
* [Rainbow](https://github.com/ging/rainbow)(not yet public) is a RUST-implementation of the [Dataspace Protocol](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol)
    * provides the  [Transfer Process API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/transfer-process/transfer.process.protocol)
    * provides the [Catalog API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/catalog/catalog.protocol)
    * provides CRUD functionality for Agreement objects defined by the [Contract Negotiation API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/contract-negotiation/contract.negotiation.protocol)
    * provides the callback interface for the Consumer
* [Contract Management](https://github.com/FIWARE/contract-management):
    * integrates with the TMForum API and translate its entities to [DCAT Entries](https://www.w3.org/TR/vocab-dcat-3/) in Rainbow([Catalogs](https://www.w3.org/TR/vocab-dcat-3/#Class:Catalog) and [DataServices](https://www.w3.org/TR/vocab-dcat-3/#Class:Data_Service))
    * create Agreements in Rainbow based on the Product Orderings
    * writes back the Agreemnt-IDs to the TMForum API Product Orderings

## Usage

In order to access a Service by using the Transfer Process Protocol, the following steps need to be taken:

> :warning: The example calls are using the [local deployment](./deployment-integration/local-deployment/LOCAL.MD). Make sure its running before trying them out.

### Preparation

To have some demo data available, create the following entity directly at the context broker:

```shell
curl -X POST http://scorpio-provider.127.0.0.1.nip.io:8080/ngsi-ld/v1/entities \
  -H 'Accept: application/json' \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "urn:ngsi-ld:UptimeReport:fms-1",
    "type": "UptimeReport",
    "name": {
      "type": "Property",
      "value": "Standard Server"
    },
    "uptime": {
      "type": "Property",
      "value": "99.9"
    }
  }'
```

### Create the offering

In order to be compatible with the Dataspace Protocol, a Data Service needs to be available at the [Catalog API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/catalog/catalog.protocol) in the [DCAT-Format](https://www.w3.org/TR/vocab-dcat-3/). To do so, Catalogs, Categories and ProductOfferings from TMForum are mapped to the entries by the Contract Management.
With the following steps, a catalog containing a DataService can be created:

>:warning: For better understandability, the creation happens directly through the TMForum-API, without authentication.

Prepare the offering on the provider side: 

1. Create the category:
```shell
export CATEGORY_ID=$(curl -X 'POST' \
  'http://tm-forum-api.127.0.0.1.nip.io:8080/tmf-api/productCatalogManagement/v4/category' \
  -H 'accept: application/json;charset=utf-8' \
  -H 'Content-Type: application/json;charset=utf-8' \
  -d '{
  "description": "Test Category",
  "name": "Test Category"
}' | jq .id -r); echo ${CATEGORY_ID}
```

2. Create the catalog:
```shell
export CATALOG_ID=$(curl -X 'POST' \
  'http://tm-forum-api.127.0.0.1.nip.io:8080/tmf-api/productCatalogManagement/v4/catalog' \
  -H 'accept: application/json;charset=utf-8' \
  -H 'Content-Type: application/json;charset=utf-8' \
  -d "{
  \"description\": \"Test Catalog\",
  \"name\": \"Test Catalog\",
  \"category\": [
    {
        \"id\": \"${CATEGORY_ID}\"
    }
  ]
}" | jq .id -r); echo ${CATALOG_ID}
```

3. Create the product-specifction(e.g. the dataset) - pointing towards the NGSI-LD based data service. In order to work with the DSP, the following specCharacteristics have to be provided:
* endpointUrl: host address that the service will be made available at
* endpointDescription: a description of the service to be used in the catalog
* upstreamAddress: internal address of the service, required for provisioning - will not be published through the catalog api
* (optional) targetSpecification: An odrl:AssetCollection to be used as target of the policies. If not provided, the AssetId will be enforced as the target
* serviceConfiguration: Configuration to be provided at the credentials-config-service for OID4VP support

```shell
export PRODUCT_SPEC_ID=$(curl -X 'POST' \
    'http://tm-forum-api.127.0.0.1.nip.io:8080/tmf-api/productCatalogManagement/v4/productSpecification' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d "{
           \"name\": \"Test Spec\",
           \"externalId\": \"ASSET-1\",
           \"@schemaLocation\": \"https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json\",
           \"productSpecCharacteristic\": [
               {
                   \"id\": \"endpointUrl-dcp\",
                   \"name\":\"Endpoint, that the service can be negotiated at via DCP.\",
                   \"valueType\":\"endpointUrl\",
                   \"productSpecCharacteristicValue\": [{
                      \"value\":\"http://dcp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1\",
                      \"isDefault\": true
                   }]
               },{
                   \"id\": \"endpointUrl-oid4vc\",
                   \"name\":\"Endpoint, that the service can be negotiated at via OID4VC.\",
                   \"valueType\":\"endpointUrl\",
                   \"productSpecCharacteristicValue\": [{
                      \"value\":\"http://dsp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1\",
                      \"isDefault\": true
                   }]
               },
               {
                   \"id\": \"upstreamAddress\",
                   \"name\":\"Address of the upstream serving the data\",
                   \"valueType\":\"upstreamAddress\",
                   \"productSpecCharacteristicValue\": [{
                       \"value\":\"data-service-scorpio:9090\",
                       \"isDefault\": true
                   }]
               },
               {
                   \"id\": \"endpointDescription\",
                   \"name\":\"Service Endpoint Description\",
                   \"valueType\":\"endpointDescription\",
                   \"productSpecCharacteristicValue\": [{
                       \"value\":\"The Test Service\"
                   }]
               },
               {
                   \"id\": \"targetSpecification\",
                   \"name\":\"Detailed specification of the ODRL target\",
                   \"valueType\":\"targetSpecification\",
                   \"productSpecCharacteristicValue\": [{
                      \"value\": {
                        \"@type\": \"AssetCollection\",
                        \"refinement\": [
                          {
                            \"@type\": \"Constraint\",
                            \"leftOperand\": \"http:path\",
                            \"operator\": \"http:isInPath\",
                            \"rightOperand\": \"/*/ngsi-ld/v1/entities\"
                          }
                        ]
                       },
                       \"isDefault\": true
                   }]
               },
               {
                 \"id\": \"serviceConfiguration\",
                 \"name\": \"Service config to be used in the credentials config service\",
                 \"valueType\": \"serviceConfiguration\",
                 \"productSpecCharacteristicValue\": [
                   {
                     \"isDefault\": true,
                     \"value\": {
                       \"defaultOidcScope\": \"openid\",
                       \"authorizationType\": \"DEEPLINK\",
                       \"oidcScopes\": {
                         \"openid\": {
                           \"credentials\": [
                             {
                               \"type\": \"LegalPersonCredential\",
                               \"trustedParticipantsLists\": [
                                {
                                  \"type\": \"ebsi\",
                                  \"url\": \"http://tir.127.0.0.1.nip.io\"
                                }
                               ],
                               \"trustedIssuersLists\": [\"http://trusted-issuers-list:8080\"],
                               \"jwtInclusion\": {
                                 \"enabled\": true,
                                 \"fullInclusion\": true
                               }
                             }
                           ],
                           \"dcql\": {
                             \"credentials\": [
                               {
                                 \"id\": \"legal-person-query\",
                                 \"format\": \"dc+sd-jwt\",
                                 \"multiple\": false,
                                 \"claims\": [
                                   {
                                     \"id\": \"name-claim\",
                                     \"path\": [\"firstName\"]
                                   }
                                 ],
                                 \"meta\": {
                                   \"vct_values\": [\"LegalPersonCredential\"]
                                 }
                               }
                             ]
                           }
                         }
                       }
                     }
                   }
                 ]
               }
           ]
       }" | jq '.id' -r); echo ${PRODUCT_SPEC_ID}
```

4. Create the corresponding ProductOffering. It will include the policies to be used for the offering. As of now, only access-policies are supported(for compatibility reasons, the contract policy still needs an id).

```shell
access_policy_id=$(uuidgen)
contract_policy_id=$(uuidgen)
curl -X 'POST' \
    'http://tm-forum-api.127.0.0.1.nip.io:8080/tmf-api/productCatalogManagement/v4/productOffering' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d "{
          \"name\": \"Test Offering\",
          \"description\": \"Test Offering description\",
          \"isBundle\": false,
          \"isSellable\": true,
          \"lifecycleStatus\": \"Active\",
          \"@schemaLocation\": \"https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json\",
          \"externalId\": \"OFFER-2\",
          \"productSpecification\":
              {
                  \"id\": \"${PRODUCT_SPEC_ID}\",
                  \"name\":\"The Test Spec\"
              },
          \"category\": [{
              \"id\": \"${CATEGORY_ID}\"
          }],
          \"productOfferingTerm\": [
          {
              \"name\": \"edc:contractDefinition\",
              \"@schemaLocation\": \"https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/contract-definition.json\",
              \"accessPolicy\": {
                \"@context\": \"http://www.w3.org/ns/odrl.jsonld\",
                \"odrl:uid\": \"${access_policy_id}\",
                \"assigner\": \"did:web:mp-operations.org\",
                \"permission\": [{
                    \"action\":  \"use\"
                }],
                \"@type\":  \"Offer\"  
              },
              \"contractPolicy\": {
                \"@context\": \"http://www.w3.org/ns/odrl.jsonld\",
                \"odrl:uid\": \"${contract_policy_id}\",
                \"assigner\": \"did:web:mp-operations.org\",
                \"permission\": [{
                    \"action\":  \"use\",
                    \"constraint\": {
                      \"leftOperand\": \"odrl:dayOfWeek\",
                      \"operator\": \"lt\",
                      \"rightOperand\": 6
                    }
                }],
                \"@type\":  \"Offer\"
              }
          }]
        }" | jq .
```

5. The catalog now can be read at the Providers DSP Catalog(needs to be authenticated):
```shell
export LEGAL_PERSON_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-consumer.127.0.0.1.nip.io legal-person-credential employee)
export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh http://dsp-mp-operations.127.0.0.1.nip.io $LEGAL_PERSON_CREDENTIAL openid)
curl  -X POST \
  'http://dsp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1/catalog/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --header "Authorization: Bearer ${ACCESS_TOKEN}" \
  --data-raw '{
        "@context": [
            "https://w3id.org/dspace/2025/1/context.jsonld"
        ],
        "@type": "CatalogRequestMessage",
        "querySpec": {
        }
    }' | jq .
```

### Interact with the FDSC-EDC Through DCP secured endpoints


All following interactions will happen through the Managment-API of the consumer-side connector. To ease its usage, the api is made available at ```dsp-management.127.0.0.1.nip.io```. In productive environments, this needs to be protected.

6. The catalog can be read through the local connector. The connector will authenticate itself at the provider side, configured in ```counterPartyAddress```.
```shell
curl  -X POST \
  'http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/catalog/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "CatalogRequestMessage",
        "protocol": "dataspace-protocol-http:2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "counterPartyAddress": "http://dcp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1",
        "querySpec": {
        }
    }' | jq .
```

7. In order to start a negotiation, an offer has to be selected(for the demo flow, only one is configured):

```shell
curl  -X POST \
  'http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/contractnegotiations' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "ContractRequest",
        "counterPartyAddress": "http://dcp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "protocol": "dataspace-protocol-http:2025-1",
        "policy": {
            "@context": "http://www.w3.org/ns/odrl.jsonld",
            "@type": "Offer",
            "@id": "OFFER-2:ASSET-1:123",
            "assigner": "did:web:mp-operations.org",
            "permission": [{
              "action":  "use",
              "constraint": {
                "leftOperand": "odrl:dayOfWeek",
                "operator": "lt",
                "rightOperand": 6
              }
            }],
            "target": "ASSET-1"
        }
    }' | jq .
```

8. Negotiation state can be retrieved at the consumer:

```shell
curl  -X POST \
  'http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/contractnegotiations/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' | jq .
```

9. When the state of the negotiation is "finalized", the agreement id can be retrieved: 

```shell
export AGREEMENT_ID=$(curl  -X POST \
  'http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/contractnegotiations/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' | jq -r .[].contractAgreementId); echo ${AGREEMENT_ID}
```

10. With a successfull agreement in place, a transfer process can be started:

```shell
curl  -X POST \
  'http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/transferprocesses' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw "{
    \"@context\": [
        \"https://w3id.org/edc/connector/management/v0.0.1\"
    ],
    \"assetId\": \"ASSET-1\",
    \"counterPartyId\": \"did:web:mp-operations.org\",
    \"counterPartyAddress\":  \"http://dcp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1\",
    \"connectorId\": \"did:web:mp-operations.org\",
    \"contractId\": \"${AGREEMENT_ID}\",
    \"dataDestination\": {
        \"type\": \"HttpProxy\"
    },
    \"protocol\": \"dataspace-protocol-http:2025-1\",
    \"transferType\": \"HttpData-PULL\"
}" | jq .
```

11. The transfer state can be retrieved at the consumer:

```shell
curl  -X POST \
  'http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/transferprocesses/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "@context": ["https://w3id.org/edc/connector/management/v0.0.1"],
    "@type": "QuerySpec"
  }' | jq .
```

12. When state is "started", the transfer Id can be retrieved:

```shell
export TRANSFER_ID=$(curl  -X POST \
  'http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/transferprocesses/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "@context": ["https://w3id.org/edc/connector/management/v0.0.1"],
    "@type": "QuerySpec"
  }' | jq -r '.[]."@id"'); echo ${TRANSFER_ID}
```



13. For the started transfer, now the provisioned endpoint has to be retrieved:
```shell
export ENDPOINT=$(curl -X GET "http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/edrs/${TRANSFER_ID}/dataaddress" | jq -r .endpoint); echo ${ENDPOINT}
```

14. The token can be retrieved the same way: 
```shell
export ACCESS_TOKEN=$(curl -X GET "http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/edrs/${TRANSFER_ID}/dataaddress" | jq -r .token); echo ${ACCESS_TOKEN}
```

15. Get the data:
```shell
curl -x localhost:8888 -X GET ${ENDPOINT}/ngsi-ld/v1/entities/urn:ngsi-ld:UptimeReport:fms-1 \
  --header 'Content-Type: application/json' \
  --header "Authorization: Bearer ${ACCESS_TOKEN}"
```

16. Once interaction has finished, the transfer can be stopped:

```shell
curl -X POST "http://dsp-dcp-management.127.0.0.1.nip.io:8080/api/v1/management/v3/transferprocesses/${TRANSFER_ID}/terminate" \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw "{
      \"@context\": [
          \"https://w3id.org/edc/connector/management/v0.0.1\"
      ],
      \"counterPartyAddress\":  \"http://dsp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1\",
      \"reason\" : \"finished\"
  }" | jq .
```

--------

[OID4VC]
6. The catalog can be read through the local connector. The connector will authenticate itself at the provider side, configured in ```counterPartyAddress```.
```shell
curl  -X POST \
  'http://dsp-oid4vc-management.127.0.0.1.nip.io:8080/api/v1/management/v3/catalog/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "CatalogRequestMessage",
        "protocol": "dataspace-protocol-http:2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "counterPartyAddress": "http://dsp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1",
        "querySpec": {
        }
    }' | jq .
```

7. In order to start a negotiation, an offer has to be selected(for the demo flow, only one is configured):

```shell
curl  -X POST \
  'http://dsp-oid4vc-management.127.0.0.1.nip.io:8080/api/v1/management/v3/contractnegotiations' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "ContractRequest",
        "counterPartyAddress": "http://dsp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "protocol": "dataspace-protocol-http:2025-1",
        "policy": {
            "@context": "http://www.w3.org/ns/odrl.jsonld",
            "@type": "Offer",
            "@id": "OFFER-2:ASSET-1:123",
            "assigner": "did:web:mp-operations.org",
            "permission": [{
              "action":  "use",
              "constraint": {
                "leftOperand": "odrl:dayOfWeek",
                "operator": "lt",
                "rightOperand": 6
              }
            }],
            "target": "ASSET-1"
        }
    }' | jq .
```

8. Negotiation state can be retrieved at the consumer:

```shell
curl  -X POST \
  'http://dsp-oid4vc-management.127.0.0.1.nip.io:8080/api/v1/management/v3/contractnegotiations/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' | jq .
```

9. When the state of the negotiation is "finalized", the agreement id can be retrieved: 

```shell
export AGREEMENT_ID=$(curl  -X POST \
  'http://dsp-oid4vc-management.127.0.0.1.nip.io:8080/api/v1/management/v3/contractnegotiations/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' | jq -r .[].contractAgreementId); echo ${AGREEMENT_ID}
```

10. With a successfull agreement in place, a transfer process can be started:

```shell
curl  -X POST \
  'http://dsp-oid4vc-management.127.0.0.1.nip.io:8080/api/v1/management/v3/transferprocesses' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw "{
    \"@context\": [
        \"https://w3id.org/edc/connector/management/v0.0.1\"
    ],
    \"assetId\": \"ASSET-1\",
    \"counterPartyId\": \"did:web:mp-operations.org\",
    \"counterPartyAddress\":  \"http://dsp-mp-operations.127.0.0.1.nip.io:8080/api/dsp/2025-1\",
    \"connectorId\": \"did:web:mp-operations.org\",
    \"contractId\": \"${AGREEMENT_ID}\",
    \"dataDestination\": {
        \"type\": \"HttpProxy\"
    },
    \"protocol\": \"dataspace-protocol-http:2025-1\",
    \"transferType\": \"HttpData-PULL\"
}" | jq .
```

11. The transfer state can be retrieved at the consumer:

```shell
curl  -X POST \
  'http://dsp-oid4vc-management.127.0.0.1.nip.io:8080/api/v1/management/v3/transferprocesses/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "@context": ["https://w3id.org/edc/connector/management/v0.0.1"],
    "@type": "QuerySpec"
  }' | jq .
```

12. When state is "started", the transfer Id can be retrieved:

```shell
export TRANSFER_ID=$(curl  -X POST \
  'http://dsp-oid4vc-management.127.0.0.1.nip.io:8080/api/v1/management/v3/transferprocesses/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "@context": ["https://w3id.org/edc/connector/management/v0.0.1"],
    "@type": "QuerySpec"
  }' | jq -r '.[]."@id"'); echo ${TRANSFER_ID}
```


13. For the started transfer, now the provisioned endpoint has to be retrieved:
```shell
export ENDPOINT=$(curl -X GET "http://dsp-oid4vc-management.127.0.0.1.nip.io:8080/api/v1/management/v3/edrs/${TRANSFER_ID}/dataaddress" | jq -r .endpoint); echo ${ENDPOINT}
```

14. Since OID4VP is configured to be used for authentication, a 401 should be returned for that endpoint(the proxy has to be used in the local deployment, to allow proper dns resolution):
```shell
curl -x localhost:8888 -X GET ${ENDPOINT}/ngsi-ld/v1/entities/urn:ngsi-ld:UptimeReport:fms-1
```

15. Get the OpenId Configuration for the provisioned endpoint:
```shell
curl -x localhost:8888 -X GET ${ENDPOINT}/.well-known/openid-configuration | jq .
```

16. With the OpenId-Configuration, a proper OID4VP flow can be executed. The user identifies itself with a LegalPersonCredential and gets an AccessToken back from the verifier. With that, the entity can be accessed:
```shell
export LEGAL_PERSON_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-consumer.127.0.0.1.nip.io legal-person-credential employee)
export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh ${ENDPOINT} $LEGAL_PERSON_CREDENTIAL openid)
curl -x localhost:8888 -X GET ${ENDPOINT}/ngsi-ld/v1/entities/urn:ngsi-ld:UptimeReport:fms-1 \
  --header 'Content-Type: application/json' \
  --header "Authorization: Bearer ${ACCESS_TOKEN}"
```

## DCP 


Consumer:

Get client key as jwk
```shell
export JWK=$(./doc/scripts/get-private-jwk-p-256.sh ./helpers/certs/out/client-consumer/private/client-pkcs8.key.pem); echo $JWK
```

Create key in vault:
```shell
curl -X POST 'http://vault-mp-operations.127.0.0.1.nip.io:8080/v1/secret/data/key-1' \
  --header 'X-Vault-Token: root' \
  --data "$(jq -n --arg content "$JWK" '{data:{content:$content}}')"
```

```shell
export PUBLIC_JWK=$(echo $JWK | jq 'del(.d)' | jq '. += {"kid":"did:web:fancy-marketplace.biz#key-1"}')
export DATA=$(echo '{
      "role": ["admin"],
      "serviceEndpoints": [
          {
            "type": "CredentialService", 
            "serviceEndpoint": "http://identityhub-fancy-marketplace.127.0.0.1.nip.io/api/credentials/v1/participants/ZGlkOndlYjpmYW5jeS1tYXJrZXRwbGFjZS5iaXo",
            "id": "credential-service"
          }
        ],
      "active": true,
      "participantId": "did:web:fancy-marketplace.biz",
      "did": "did:web:fancy-marketplace.biz",
      "key": {
        "keyId": "did:web:fancy-marketplace.biz#key-1",
        "privateKeyAlias": "key-1",
        "publicKeyJwk": {}     
      }
    }')

export DATA_RAW=$(echo "${DATA}" | jq --argjson pem "$PUBLIC_JWK" '.key.publicKeyJwk = $pem'); echo $DATA_RAW | jq .
curl -X POST \
  'http://identityhub-management-fancy-marketplace.127.0.0.1.nip.io:8080/api/identity/v1alpha/participants' \
  --header 'Accept: */*' \
  --header 'x-api-key: c3VwZXItdXNlcg==.random' \
  --header 'Content-Type: application/json' \
  --data "${DATA_RAW}"
```

Provider:

Get client key as jwk
```shell
export JWK=$(./doc/scripts/get-private-jwk-p-256.sh ./helpers/certs/out/client-provider/private/client-pkcs8.key.pem); echo $JWK
```

Create key in vault:
```shell
kubectl port-forward provider-vault-0 8200:8200 -n provider
curl -X POST 'http://localhost:8200/v1/secret/data/key-1' \
  --header 'X-Vault-Token: root' \
  --data "$(jq -n --arg content "$JWK" '{data:{content:$content}}')"
```

```shell
export PUBLIC_JWK=$(echo $JWK | jq 'del(.d)' | jq '. += {"kid":"did:web:mp-operations.org#key-1"}')
export DATA=$(echo '{
      "role": ["admin"],
      "serviceEndpoints": [
          {
            "type": "CredentialService", 
            "serviceEndpoint": "http://identityhub-mp-operations.127.0.0.1.nip.io/api/credentials/v1/participants/ZGlkOndlYjptcC1vcGVyYXRpb25zLm9yZw",
            "id": "credential-service"
          }
        ],
      "active": true,
      "participantId": "did:web:mp-operations.org",
      "did": "did:web:mp-operations.org",
      "key": {
        "keyId": "did:web:mp-operations.org#key-1",
        "privateKeyAlias": "key-1",
        "publicKeyJwk": {}     
      }
    }')

export DATA_RAW=$(echo "${DATA}" | jq --argjson pem "$PUBLIC_JWK" '.key.publicKeyJwk = $pem'); echo $DATA_RAW | jq .
curl -X POST \
  'http://identityhub-management-mp-operations.127.0.0.1.nip.io:8080/api/identity/v1alpha/participants' \
  --header 'Accept: */*' \
  --header 'x-api-key: c3VwZXItdXNlcg==.random' \
  --header 'Content-Type: application/json' \
  --data "${DATA_RAW}"
```

Add credential:

```shell
export CONSUMER_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-consumer.127.0.0.1.nip.io membership-credential employee); echo Consumer Credential: ${CONSUMER_CREDENTIAL}
```

```shell
export CONSUMER_CREDENTIAL_CONTENT=$(./doc/scripts/get-payload-from-jwt.sh "${CONSUMER_CREDENTIAL}" | jq -r '.vc'); echo Content: ${CONSUMER_CREDENTIAL_CONTENT}
```

Put credential:

```shell
curl  -X POST \
  'http://identityhub-management-fancy-marketplace.127.0.0.1.nip.io:8080/api/identity/v1alpha/participants/ZGlkOndlYjpmYW5jeS1tYXJrZXRwbGFjZS5iaXo/credentials' \
  --header 'Accept: */*' \
  --header 'x-api-key: c3VwZXItdXNlcg==.random' \
  --header 'Content-Type: application/json' \
  --data-raw "{
    \"id\": \"my-credential\",
    \"participantContextId\": \"did:web:fancy-marketplace.biz\",
    \"verifiableCredentialContainer\": {
      \"rawVc\": \"${CONSUMER_CREDENTIAL}\",
      \"format\": \"VC1_0_JWT\",
      \"credential\": ${CONSUMER_CREDENTIAL_CONTENT}
    }
  }"
```

Register Membership Credential at provider til:

```shell
curl -X PUT http://til-provider.127.0.0.1.nip.io:8080/issuer/did:web:fancy-marketplace.biz \
  -H 'accept: */*' \
  -H 'Content-Type: application/json' \
  -d '{
  "did": "did:web:fancy-marketplace.biz",
  "credentials": [
    {
      "credentialsType": "LegalPersonCredential"
    },
    {
      "credentialsType": "UserCredential"
    },
    {
      "credentialsType": "MembershipCredential"
    }
  ]
}'
```

Credentials for provider:

Prepare credential:

```shell
export PROVIDER_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-provider.127.0.0.1.nip.io membership-credential employee); echo Provider Credential: ${PROVIDER_CREDENTIAL}
```

```shell
export PROVIDER_CREDENTIAL_CONTENT=$(./doc/scripts/get-payload-from-jwt.sh "${PROVIDER_CREDENTIAL}" | jq -r '.vc'); echo Content: ${PROVIDER_CREDENTIAL_CONTENT}
```

Put credential:

```shell
curl  -X POST \
  'http://identityhub-management-mp-operations.127.0.0.1.nip.io:8080/api/identity/v1alpha/participants/ZGlkOndlYjptcC1vcGVyYXRpb25zLm9yZw/credentials' \
  --header 'Accept: */*' \
  --header 'x-api-key: c3VwZXItdXNlcg==.random' \
  --header 'Content-Type: application/json' \
  --data-raw "{
    \"id\": \"my-credential\",
    \"participantContextId\": \"did:web:mp-operations.org\",
    \"verifiableCredentialContainer\": {
      \"rawVc\": \"${PROVIDER_CREDENTIAL}\",
      \"format\": \"VC1_0_JWT\",
      \"credential\": ${PROVIDER_CREDENTIAL_CONTENT}
    }
  }"
```

Add issuer to consumer til(port forward 8080):

```shell
curl  -X POST \
  'http://localhost:7080/issuer' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
  "did": "did:web:mp-operations.org",
  "credentials": [
    {
      "credentialsType": "MembershipCredential"
    }
  ]
}'
```