# Integration of the Dataspace Protocol

In order to be compatible with other connectors, the FIWARE Data Space Connector supports the [Dataspace Protocol](https://eclipse-dataspace-protocol-base.github.io/DataspaceProtocol/2025-1-err1/) and the [Decentralize Claims Protocol with the Dataspace Protocol](https://eclipse-dataspace-dcp.github.io/decentralized-claims-protocol/v1.0.1/).

## Architecture


![DSP Architecture](./img/FDSC_EDC_Arch.jpg)

* [TMForum API](https://github.com/FIWARE/tmforum-api) provides the APIs for creating and managing Products and Offerings
    * integrates with the Contract Management through Events
* [FDSC-EDC](https://github.com/SEAMWARE/fdsc-edc) is an implementation of the Eclipse Data Space Components that
    * provides the  [Transfer Process API](https://eclipse-dataspace-protocol-base.github.io/DataspaceProtocol/2025-1-err1/#transfer-protocol)
    * provides the [Catalog API](https://eclipse-dataspace-protocol-base.github.io/DataspaceProtocol/2025-1-err1/#catalog-protocol)
    * provides the [Contract Negotiation API](https://eclipse-dataspace-protocol-base.github.io/DataspaceProtocol/2025-1-err1/#negotiation-protocol)
    * uses the TMForum API as storage backend
    * uses the FIWARE Data Space Connector as the dataplane to provision transfers
    * provides two flavors:
      * OID4VC - uses the [OpenID for Verifiable Credentials Protocols](https://openid.net/sg/openid4vc/) for authentication between connectors
      * DCP - uses the [Decentralized Claims Protocol](https://eclipse-dataspace-dcp.github.io/decentralized-claims-protocol/v1.0.1/) for authentication between connectors
* Identity Hub - implementation of the EDC-Identity Services, currently using the [Tractus-X IdentityHub](github.com/eclipse-tractusx/tractusx-identityhub)

### Authentication via OID4VP

In order to integrate with [EUDI Wallet ARF](https://eudi.dev/1.1.0/arf/) compatible wallets and better support for human-to-machine interaction, the FDSC-EDC implementation supports the usage of [OpenID 4 Verifiable Presentations](https://openid.net/specs/openid-4-verifiable-presentations-1_0.html) for Connector-to-Connector communication and for authentication of users accessing the actual data and services.
Both sides need to have a full IAM-Framework in place(including Verifier and PEP). The following architecture only shows those components at the provider side, however the flow is the same on call from provider to consumer, thus the consumer needs to have them, too.

![OID4VP Flow](./img/OID4VP-FIWARE-DSC-EDC-Detail.png)

A request uses the flow:

1. Get /.well-known/openid-configuration
2. Extract the [Authorization Endpoint](https://datatracker.ietf.org/doc/html/rfc6749#section-3.1) and call it
3. Verifier will return the Authorization Request, including information about the requested credentials(via [DCQL](https://openid.net/specs/openid-4-verifiable-presentations-1_0.html#name-digital-credentials-query-l))
4. Consumer selects the claims and credentials to be presented, creates and signs a vp_token and sends it to the verifiers [Token Endpoint](https://datatracker.ietf.org/doc/html/rfc6749#section-3.2)
5. Verifier verifies the token against, checks the issuer at the configured trusted lists and returns a [Token Response](https://datatracker.ietf.org/doc/html/rfc6749#section-4.1.4) containing the JWT in the configured(see [Credentials-Config-Service](https://github.com/FIWARE/credentials-config-service)) format
6. Consumer requests the Provider Controlplane through its Policy-Enforcement-Point. PEP will apply policies, controling access to the DSP-API
7. PEP forwards request to the Controlplane, in order to handle it


#### Preparation

In order to authenticate itself, the FDSC-EDC requires access to the Verifiable Credentials it needs to present. Currently, the implementation only supports storage of the credentials inside a folder provided to the FDSC-EDC. The credentials need to be provided in plain fails, using `.jwt` for JWT-VC and `.sd_jwt`for SD-JWT VC as file extension. See the [FDSC-EDC Helm Chart documentation](https://github.com/FIWARE/helm-charts/blob/main/charts/fdsc-edc/values.yaml#L173) for more details on configuring the FDSC-EDC.

### Authentication via DCP

In order to authenticate via DCP, an instance of the Identity Hub has to be provided. The following diagram only includes it on the Consumer side, but its also needed on the Provider side to call back.

![DCP Flow](./img/DCP-FIWARE-DSC-EDC-Detail.png)

1. Connector gets an [Self-Issued ID-Token](https://eclipse-dataspace-dcp.github.io/decentralized-claims-protocol/v1.0.1/#self-issued-id-tokens) from its IdentiyHub(STS-Service), that includes the DID and the Access Token
2. Sends request with both Tokens to the Provider connector
3. Provider resolves the DID, extracted from the ID-Token, requests did-document at the Consumer's IdentityHub(DID-Service)
4. Consumer returns did-document, containing the Credential Service address
5. Provider requests Credentials at the IdentityHub(Credential Service), using the Access Token
6. Consumer returns Verifiable Presentation, containing the Credentials. Provider side verifies them and checks issuers at the TrustedIssuersList.

For configuration details, see the [FDSC-EDC Helm Chart documentation](https://github.com/FIWARE/helm-charts/blob/main/charts/fdsc-edc/values.yaml#L299).

### Provisioning

The FIWARE FDSC-EDC does not come with an explicit implementation of the dataplane, since the FIWARE Data Space Connector itself acts as such. Transfers are provisioned at Apisix and configured depending on the requested Authentication Protocol.

![Provisioning Overview](./img/Provisioning-FIWARE-DSC-EDC.png)

With an agreement in place, connectors can request data transfers using the [Transfer Process Protocol](https://eclipse-dataspace-protocol-base.github.io/DataspaceProtocol/2025-1-err1/#transfer-protocol). When a Transfer Process gets started, the provisioning happens as following

#### OID4VP

1. Add AccessPolicies to the PAP, enforces them on usage of the transfer
2. Add Credentials Configuration to the Credentials-Config-Service if configured
3. Create a <TRANSFER_PROCESS_ID>/.well-known/openid-configuration route at Apisix, to allow OID4VP auth
4. Create a <TRANSFER_PROCESS_ID>/ route to the data-service/data-set
5. Return the created route-endpoint to the counterparty for usage

#### DCP

1. Enforce AccessPolicies
2. Create a <TRANSFER_PROCESS_ID>/ route to the data-service/data-set
3. Generate a JWT, containing the transfer-process-id as scope, signed with the controlplanes key
4. Return the route-endpoint and the jwt to the counterparty for usage

## Usage


To run the local setup:
```shell
  mvn clean deploy -Plocal,dsp
```

The following steps will show how to create a ProductOffering throught the TMForum-APIs, buy access to it via TMForum, DSP+OID4VP and DSP+DCP and access the purchased data-service through all 3 methods. It requires a proper setup of Consumer and Provider identities as expected for the DCP, which also works for OID4VP.


### Setup the consumer

The consumer-identity and key-material needs to be registered in the identityhub, in order to participate in DCP-based DSP interactions. Since all OID4VC base flows do not rely on any propriatary extensions to the did-standard, they can also work with that.

1. Get the private key(managed by cert-manager, used for signing the certificates) as JWK(to allow signing tokens in the Secure Token Service):
```shell
export CONSUMER_JWK=$(./doc/scripts/get-private-jwk-from-k8s-secret.sh consumer fancy-marketplace.biz-tls); echo $CONSUMER_JWK
```

2. Insert the key into Vault:
```shell
curl -k -X POST -x localhost:8888 'https://vault-fancy-marketplace.127.0.0.1.nip.io/v1/secret/data/key-1' \
  --header 'X-Vault-Token: root' \
  --data "$(jq -n --arg content "$CONSUMER_JWK" '{data:{content:$content}}')"
```

3. Insert the participants identity and public key into identity hub.
```shell
export CONSUMER_PARTICIPANT=$(./doc/scripts/get-participant-create.sh "${CONSUMER_JWK}" did:web:fancy-marketplace.biz "https://identityhub-fancy-marketplace.127.0.0.1.nip.io" "key-1"); echo ${CONSUMER_PARTICIPANT} | jq .
curl -k -x localhost:8888 -X POST \
  'https://identityhub-management-fancy-marketplace.127.0.0.1.nip.io/api/identity/v1alpha/participants' \
  --header 'Accept: */*' \
  --header 'x-api-key: c3VwZXItdXNlcg==.random' \
  --header 'Content-Type: application/json' \
  --data "${CONSUMER_PARTICIPANT}"
```

4. Check that the identity(e.g. did-document) is available:
```shell
curl -x localhost:8888 https://fancy-marketplace.biz/.well-known/did.json -k | jq .
```

### Setup the provider

The provider indentity has to be prepared exactly the same way:

1. Get the private key(managed by cert-manager, used for signing the certificates) as JWK(to allow signing tokens in the Secure Token Service):
```shell
export PROVIDER_JWK=$(./doc/scripts/get-private-jwk-from-k8s-secret.sh provider mp-operations.org-tls); echo $PROVIDER_JWK
```

2. Insert the key into Vault:
```shell
curl -x localhost:8888 -k -X POST 'https://vault-mp-operations.127.0.0.1.nip.io/v1/secret/data/key-1' \
  --header 'X-Vault-Token: root' \
  --data "$(jq -n --arg content "$PROVIDER_JWK" '{data:{content:$content}}')"
```

3. Insert the participants identity and public key into identity hub.
```shell
export PROVIDER_PARTICIPANT=$(./doc/scripts/get-participant-create.sh "${PROVIDER_JWK}" "did:web:mp-operations.org" "https://identityhub-mp-operations.127.0.0.1.nip.io" "key-1"); echo ${PROVIDER_PARTICIPANT} | jq .
curl -x localhost:8888 -k -X POST \
  'https://identityhub-management-mp-operations.127.0.0.1.nip.io/api/identity/v1alpha/participants' \
  --header 'Accept: */*' \
  --header 'x-api-key: c3VwZXItdXNlcg==.random' \
  --header 'Content-Type: application/json' \
  --data "${PROVIDER_PARTICIPANT}"
```

4. Check that the identity(e.g. did-document) is available:
```shell
curl -x localhost:8888 https://mp-operations.org/.well-known/did.json -k | jq .
```

### Issue membership-credentials

The demo deployment of the DSP is configured to require every participant to identify itself with a "MembershipCredential". In most use-cases, this credential will be issued by a central data-space authority. In order to keep the local deployment at managable size, we allow self-issuance of such credentials.
The OID4VC based flows are automatically configured to get such credential in the local deployment, thus the Credential only needs to be issued for usage in DCP-based flows. The deployed [Tractus-X Identityhub](https://github.com/eclipse-tractusx/tractusx-identityhub) supports integration with compliant [Issuer Services](https://eclipse-dataspace-dcp.github.io/decentralized-claims-protocol/v1.0.1/#credential-issuance-protocol). However, we are reusing the [default issuer](./deployment-integration/local-deployment/LOCAL.MD#credentials-issuance-at-the-consumer) from the FIWARE Data Space Connector and insert the credential manually into the identity hub:

#### Consumer

1. Get the credential and its decoded content:
```shell
export CONSUMER_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-consumer.127.0.0.1.nip.io membership-credential employee); echo Consumer Credential: ${CONSUMER_CREDENTIAL}
export CONSUMER_CREDENTIAL_CONTENT=$(./doc/scripts/get-payload-from-jwt.sh "${CONSUMER_CREDENTIAL}" | jq -r '.vc'); echo ${CONSUMER_CREDENTIAL_CONTENT} | jq .
```

2. Insert the credential into the identity hub:
```shell
curl -k -x localhost:8888 -X POST \
  'https://identityhub-management-fancy-marketplace.127.0.0.1.nip.io/api/identity/v1alpha/participants/ZGlkOndlYjpmYW5jeS1tYXJrZXRwbGFjZS5iaXo/credentials' \
  --header 'Accept: */*' \
  --header 'x-api-key: c3VwZXItdXNlcg==.random' \
  --header 'Content-Type: application/json' \
  --data-raw "{
    \"id\": \"membership-credential\",
    \"participantContextId\": \"did:web:fancy-marketplace.biz\",
    \"verifiableCredentialContainer\": {
      \"rawVc\": \"${CONSUMER_CREDENTIAL}\",
      \"format\": \"VC1_0_JWT\",
      \"credential\": ${CONSUMER_CREDENTIAL_CONTENT}
    }
  }"
```


#### Provider

1. Get the credential and its decoded content:
```shell
export PROVIDER_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-provider.127.0.0.1.nip.io membership-credential employee); echo Provider Credential: ${PROVIDER_CREDENTIAL}
export PROVIDER_CREDENTIAL_CONTENT=$(./doc/scripts/get-payload-from-jwt.sh "${PROVIDER_CREDENTIAL}" | jq -r '.vc'); echo ${PROVIDER_CREDENTIAL_CONTENT} | jq .
```

2. Insert the credential into the identity hub:
```shell
curl -k -x localhost:8888 -X POST \
  'https://identityhub-management-mp-operations.127.0.0.1.nip.io/api/identity/v1alpha/participants/ZGlkOndlYjptcC1vcGVyYXRpb25zLm9yZw/credentials' \
  --header 'Accept: */*' \
  --header 'x-api-key: c3VwZXItdXNlcg==.random' \
  --header 'Content-Type: application/json' \
  --data-raw "{
    \"id\": \"membership-credential\",
    \"participantContextId\": \"did:web:mp-operations.org\",
    \"verifiableCredentialContainer\": {
      \"rawVc\": \"${PROVIDER_CREDENTIAL}\",
      \"format\": \"VC1_0_JWT\",
      \"credential\": ${PROVIDER_CREDENTIAL_CONTENT}
    }
  }"
```

###  Trusted Issuers List

Both participants are preconfigured to trust each other for membership credentials. All components(VCVerifier, FDSC-EDC Controlplane) use the same local trusted issuers list to verify issuers of credentials(check provider-side list for example):

```shell
curl -k -x localhost:8888 -X GET https://til-provider.127.0.0.1.nip.io/issuer/did:web:fancy-marketplace.biz | jq .
```

### Prepare some data

The Dataservice provided in the Demo Environment is an NGSI-LD Context Broker. In order to have some data available, an entity will be inserted directly. In the demo-scenario, the provider participant "mp-operations.org" offers hosting capabilities and offers detailed data about those machines. As example, we provide an uptime report:

```shell
curl -k -x localhost:8888 -X POST https://scorpio-provider.127.0.0.1.nip.io/ngsi-ld/v1/entities \
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

### Prepare the offering

The Demo - Offering  should be available for negotiation and usage through standard TMForum mechanisms and through DSP.

There for all offerings are managed through TMForum-Standard APIs. In the Demo they are requested directly, while in real-world use-cases they most likely will be used through graphical interfaces like the [BAE Marketplace](https://github.com/FIWARE-TMForum/Business-API-Ecosystem).

#### Using TMForum APIs

1. A category has to be created, in order to assing the offering to a catalog:
```shell
export CATEGORY_ID=$(curl -k -x localhost:8888 -X 'POST' \
  'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/category' \
  -H 'accept: application/json;charset=utf-8' \
  -H 'Content-Type: application/json;charset=utf-8' \
  -d '{
  "description": "Demo Category",
  "name": "Demo Category"
}' | jq .id -r); echo ${CATEGORY_ID}
```

2. Create the catalog, that includes the category:
```shell
export CATALOG_ID=$(curl -k -x localhost:8888 -X 'POST' \
  'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/catalog' \
  -H 'accept: application/json;charset=utf-8' \
  -H 'Content-Type: application/json;charset=utf-8' \
  -d "{
  \"description\": \"Demo Catalog\",
  \"name\": \"Demo Catalog\",
  \"category\": [
    {
        \"id\": \"${CATEGORY_ID}\"
    }
  ]
}" | jq .id -r); echo ${CATALOG_ID}
```

3. Create the product specification:
```shell
export PRODUCT_SPEC_ID=$(curl -k -x localhost:8888 -X 'POST' \
    'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productSpecification' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d '{
           "name": "Demo Spec",
           "externalId": "ASSET-1",
           "@schemaLocation": "https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json",
           "productSpecCharacteristic": [
                {
                   "id": "dcp",
                   "name":"Endpoint, that the service can be negotiated at via DCP.",
                   "description": "Endpoint, that the service can be negotiated at via DCP.",
                   "valueType":"endpointUrl",
                   "productSpecCharacteristicValue": [{
                      "value":"https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
                      "isDefault": true
                   }]
               },{
                   "id": "oid4vc",
                   "name":"Endpoint, that the service can be negotiated at via OID4VC.",
                   "description": "Endpoint, that the service can be negotiated at via OID4VC.",
                   "valueType":"endpointUrl",
                   "productSpecCharacteristicValue": [{
                      "value":"https://dsp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
                      "isDefault": true
                   }]
               },
               {
                   "id": "upstreamAddress",
                   "name":"Address of the upstream serving the data",
                   "valueType":"upstreamAddress",
                   "productSpecCharacteristicValue": [{
                       "value":"data-service-scorpio:9090",
                       "isDefault": true
                   }]
               },
               {
                   "id": "targetSpecification",
                   "name":"Detailed specification of the ODRL target. Allows to over services via OID4VC",
                   "valueType":"targetSpecification",
                   "productSpecCharacteristicValue": [{
                      "value": {
                        "@type": "AssetCollection",
                        "refinement": [
                          {
                            "@type": "Constraint",
                            "leftOperand": "http:path",
                            "operator": "http:isInPath",
                            "rightOperand": "/*/ngsi-ld/v1/entities"
                          }
                        ]
                       },
                       "isDefault": true
                   }]
               },
               {
                 "id": "serviceConfiguration",
                 "name": "Service config to be used in the credentials config service when provisioning transfers through OID4VC",
                 "valueType": "serviceConfiguration",
                 "productSpecCharacteristicValue": [
                   {
                     "isDefault": true,
                     "value": {
                       "defaultOidcScope": "openid",
                       "authorizationType": "DEEPLINK",
                       "oidcScopes": {
                         "openid": {
                           "credentials": [
                             {
                               "type": "OperatorCredential",
                               "trustedParticipantsLists": [
                                {
                                  "type": "ebsi",
                                  "url": "https://tir.127.0.0.1.nip.io"
                                }
                               ],
                               "trustedIssuersLists": ["http://trusted-issuers-list:8080"],
                               "jwtInclusion": {
                                 "enabled": true,
                                 "fullInclusion": true
                               }
                             }
                           ],
                           "dcql": {
                             "credentials": [
                               {
                                 "id": "legal-person-query",
                                 "format": "vc+sd-jwt",
                                 "multiple": false,
                                 "meta": {
                                   "vct_values": ["OperatorCredential"]
                                 }
                               }
                             ]
                           }
                         }
                       }
                     }
                   }
                 ]
               },
               {
                    "id": "credentialsConfig",
                    "name": "Credentials Config",
                    "@schemaLocation": "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/main/schemas/credentials/credentialConfigCharacteristic.json",
                    "valueType": "credentialsConfiguration",
                    "productSpecCharacteristicValue": [
                        {
                        "isDefault": true,
                        "value": {
                            "credentialsType": "OperatorCredential",
                            "claims": [{
                                "name": "roles",
                                "path": "$.roles[?(@.target==\"did:web:mp-operation.org\")].names[*]",
                                "allowedValues": [
                                    "OPERATOR"
                                ]
                            }]
                        }
                    }]
                },
                {
                    "id": "policyConfig",
                    "name": "Policy for reading the entities.",
                    "@schemaLocation": "https://raw.githubusercontent.com/FIWARE/contract-management/refs/heads/policy-support/schemas/odrl/policyCharacteristic.json",
                    "valueType": "authorizationPolicy",
                    "productSpecCharacteristicValue": [
                        {
                        "isDefault": true,
                        "value": {
                            "@context": {
                            "odrl": "http://www.w3.org/ns/odrl/2/"
                            },
                            "@id": "https://mp-operation.org/policy/common/k8s-small",
                            "odrl:uid": "https://mp-operation.org/policy/common/k8s-small",
                            "@type": "odrl:Policy",
                            "odrl:permission": {
                            "odrl:assigner": "https://www.mp-operation.org/",
                            "odrl:target": {
                                "@type": "odrl:AssetCollection",
                                "odrl:source": "urn:asset",
                                "odrl:refinement": [
                                    {
                                        "@type": "odrl:Constraint",
                                        "odrl:leftOperand": "http:path",
                                        "odrl:operator": "http:isInPath",
                                        "odrl:rightOperand": "/ngsi-ld/v1/entities"
                                    }
                                ]
                            },
                            "odrl:assignee": {
                                "@type": "odrl:PartyCollection",
                                "odrl:source": "urn:user",
                                "odrl:refinement": {
                                "@type": "odrl:LogicalConstraint",
                                "odrl:and": [
                                    {
                                    "@type": "odrl:Constraint",
                                    "odrl:leftOperand": "vc:role",
                                    "odrl:operator": "odrl:hasPart",
                                    "odrl:rightOperand": {
                                        "@value": "OPERATOR",
                                        "@type": "xsd:string"
                                    }
                                    },
                                    {
                                    "@type": "odrl:Constraint",
                                    "odrl:leftOperand": "vc:type",
                                    "odrl:operator": "odrl:hasPart",
                                    "odrl:rightOperand": {
                                        "@value": "OperatorCredential",
                                        "@type": "xsd:string"
                                    }
                                    }
                                ]
                                }
                            },
                            "odrl:action": "odrl:read"
                        }
                    }
                }]
        }]
    }' | jq '.id' -r); echo ${PRODUCT_SPEC_ID}
```

4. Create the coresponding offering. It includes the policies required to make it accessible through DSP:
```shell
access_policy_id=$(uuidgen)
contract_policy_id=$(uuidgen)
curl -k -x localhost:8888 -X 'POST' \
    'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productOffering' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d "{
          \"name\": \"Test Offering\",
          \"description\": \"Test Offering description\",
          \"isBundle\": false,
          \"isSellable\": true,
          \"lifecycleStatus\": \"Active\",
          \"@schemaLocation\": \"https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json\",
          \"externalId\": \"OFFER-1\",
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
                \"@context\": [
                  \"http://www.w3.org/ns/odrl.jsonld\",
                  \"https://w3id.org/dspace/2024/1/context.json\"
                ],
                \"odrl:uid\": \"${contract_policy_id}\",
                \"assigner\": \"did:web:mp-operations.org\",
                \"permission\": [
                  {
                      \"action\":  \"use\",
                      \"constraint\": {
                        \"leftOperand\": \"odrl:dayOfWeek\",
                        \"operator\": \"lt\",
                        \"rightOperand\": {
                          \"@value\": 6,
                          \"@type\": \"xsd:integer\"
                        }
                      }
                  },
                  {
                      \"action\":  \"use\",
                      \"constraint\": {
                        \"leftOperand\": \"dspace:membershipType\",
                        \"operator\": \"eq\",
                        \"rightOperand\":\"FullMember\"
                      }
                  }
                ],
                \"@type\":  \"Offer\"
              }
          }]
        }" | jq .
```




#### Using the BAE Marketplace

> :warning: The BAE Marketplace is not enabled in the deployment used for these tests, in order to keep the deployment minimal. The steps below are provided for reference only; in this environment, use the TMForum API approach described in the previous section.

To be able to use the BAE Marketplace for creating DSP-compatible products and offerings, that Marketplace must have been deployed with the configuration required to support the creation of offerings with the characteristics needed for DSP integration (see the [documentation on deployment configuration by role](https://github.com/FIWARE/data-space-connector/blob/main/doc/deployment-integration/roles/README.md) for more information).


Once you have logged into the Marketplace with a user from the provider organization, you simply need to create a new Product Specification, marking DSP compatibility and filling in the mandatory characteristics with the same values shown in the example of creation through APIs in the previous section.

![BAE Marketplace DSP Product Specification Toggle](./img/BAE_MP_ProductSpec_DSP_Toggle.png)
![BAE Marketplace DSP Product Specification Configuration](./img/BAE_MP_ProductSpec_DSP_Config.png)

Afterwards, you must create the new Offer, also marking DSP compatibility and selecting the product specification created in the previous step. In the policies section, you must create two policies, one for access and one for contract, with the same values as those shown in the example of creation through APIs.

![BAE Marketplace DSP Offer Configuration](./img/BAE_MP_Offer_DSP_Config.png)

### Order through TMForum

#### Using TMForum APIs

The TMForum APIs are secured by the PEP(Apisix). In order to allow access, we need to put policies to allow that in place:
```shell
./doc/scripts/prepare-policies.sh
```

In order to interact with the TMForum and buy access, a couple of credentials are required:

1. The LegalPersonCredential for the representative allowed to buy products:
```shell
export REP_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-consumer.127.0.0.1.nip.io user-credential representative); echo ${REP_CREDENTIAL}
```
1. The OperatorCredential for the operator allowed to use the product and access the service:
```shell
export OPERATOR_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-consumer.127.0.0.1.nip.io operator-credential operator); echo ${OPERATOR_CREDENTIAL}
```

1. Register the consumer in the marketplace:

```shell
export CONSUMER_DID="did:web:fancy-marketplace.biz"
export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh https://mp-data-service.127.0.0.1.nip.io $REP_CREDENTIAL default); echo ${ACCESS_TOKEN}
export FANCY_MARKETPLACE_ID=$(curl -k -x localhost:8888 -X POST https://tm-forum-api.127.0.0.1.nip.io/tmf-api/party/v4/organization \
-H 'Accept: */*' \
-H 'Content-Type: application/json' \
-H "Authorization: Bearer ${ACCESS_TOKEN}" \
-d "{
    \"name\": \"Fancy Marketplace Inc.\",
    \"partyCharacteristic\": [
    {
        \"name\": \"did\",
        \"value\": \"${CONSUMER_DID}\"
    }
    ]
}" | jq '.id' -r); echo ${FANCY_MARKETPLACE_ID}
```

4. Buy access through TMForum:
    1. List offerings
    ```shell
        export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh https://mp-data-service.127.0.0.1.nip.io $REP_CREDENTIAL default); echo $ACCESS_TOKEN
        curl -k -x localhost:8888 -X GET https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productOffering -H "Authorization: Bearer ${ACCESS_TOKEN}" | jq .
    ```
    2. Get offer Id and order it:
    ```shell
        export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh https://mp-data-service.127.0.0.1.nip.io $REP_CREDENTIAL default); echo $ACCESS_TOKEN
        export OFFER_ID=$(curl -k -x localhost:8888 -X GET https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productOffering -H "Authorization: Bearer ${ACCESS_TOKEN}" | jq '.[0].id' -r); echo ${OFFER_ID}
        export ORDER_ID=$(curl -k -x localhost:8888 -X POST https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productOrderingManagement/v4/productOrder \
            -H 'Accept: */*' \
            -H 'Content-Type: application/json' \
            -H "Authorization: Bearer ${ACCESS_TOKEN}" \
            -d "{
                \"productOrderItem\": [
                {
                    \"id\": \"random-order-id\",
                    \"action\": \"add\",
                    \"productOffering\": {
                    \"id\" :  \"${OFFER_ID}\"
                    }
                }
                ],
                \"relatedParty\": [
                {
                    \"id\": \"${FANCY_MARKETPLACE_ID}\"
                }
                ]}" | jq '.id' -r); echo ${ORDER_ID}
    ```
    3. Complete the order:
    ```shell
        curl -k -x localhost:8888 -X 'PATCH' \
            -H "Authorization: Bearer ${ACCESS_TOKEN}" \
            https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productOrderingManagement/v4/productOrder/${ORDER_ID} \
            -H 'accept: application/json;charset=utf-8' \
            -H 'Content-Type: application/json;charset=utf-8' \
            -d "{
                    \"state\": \"completed\"
                }" | jq .
    ```

With that, the Data Space Connector will create:
    * an entry in the TrustedIssuers registry, to allow issuance of OperatorCredentials
    * register the policy to allow access for users with OperatorCredential in role "Operator"

5. Access the entity:

```shell
export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh https://mp-data-service.127.0.0.1.nip.io $OPERATOR_CREDENTIAL operator); echo $ACCESS_TOKEN
curl -k -x localhost:8888 -X GET https://mp-data-service.127.0.0.1.nip.io/ngsi-ld/v1/entities/urn:ngsi-ld:UptimeReport:fms-1 \
    --header 'Content-Type: application/json' \
    --header "Authorization: Bearer ${ACCESS_TOKEN}"
```

### Policy Evaluation in DSP Flows

The FDSC-EDC evaluates ODRL policies at multiple points during DSP interactions. Understanding these evaluation points is essential for designing policies that control catalog visibility, contract negotiation, and data transfer independently.

#### Two-Layer Evaluation Architecture

The EDC policy engine evaluates policies at two distinct layers:

**Layer 1 — Pre-Authentication (`request.*` scopes)**

Runs *before* the counter-party's token and Verifiable Credentials are verified. Only DSP message metadata is available (counter-party address, message type, process IDs). This layer determines which VC scopes to request from the counter-party's credential service.

**Layer 2 — Post-Authentication (`catalog`, `contract.negotiation`, `transfer.process` scopes)**

Runs *after* the counter-party's token has been verified and a `ParticipantAgent` has been created from the verified Verifiable Credentials. The ODRL-PAP validator is registered at this layer and receives the full participant identity and all verified credential claims.

```
Incoming DSP Request (e.g., catalog request)
│
├─ Layer 1: Pre-Authentication
│   └─ Determines VC scopes to request (no credentials available yet)
│
├─ Token verification + VC validation
│   └─ Creates ParticipantAgent with identity + verified claims
│
└─ Layer 2: Post-Authentication
    ├─ catalog scope:             Controls asset visibility
    ├─ contract.negotiation scope: Controls whether negotiation succeeds
    └─ transfer.process scope:    Controls whether transfer is allowed
```

#### Access Policy vs. Contract Policy

Each product offering defines two separate ODRL policies through the `productOfferingTerm`:

| Policy | EDC Scope | Purpose |
|--------|-----------|---------|
| **Access Policy** | `catalog` | Controls whether an asset appears in the catalog for a given consumer. If the access policy denies, the consumer will not see the offer at all. |
| **Contract Policy** | `contract.negotiation` | Controls whether a consumer can successfully negotiate a contract. The offer is visible but negotiation will be rejected if the contract policy denies. |

Both policies can reference the consumer's verified credentials (identity, VerifiableCredential claims) because they are evaluated at Layer 2.

#### Policy Context: What the PAP Receives

When the FDSC-EDC calls the ODRL-PAP for policy evaluation, it sends a `ValidationRequest` containing:

- **`policy`**: The ODRL policy in expanded JSON-LD form
- **`jsonInput.payload.scope`**: The evaluation scope (`catalog`, `contract.negotiation`, or `transfer.process`)
- **`jsonInput.subject.identity`**: The authenticated counter-party's DID
- **`jsonInput.subject.claims.vc`**: Array of verified VerifiableCredentials
- **`additionalContexts`**: Optional JSON-LD context overrides for term remapping

This means Rego policies at the PAP can make decisions based on the consumer's actual credentials — credential type, claim values, issuer, expiration, etc. See the [ODRL-PAP Rego documentation](https://github.com/SEAMWARE/odrl-pap/blob/main/doc/REGO.md) for details on implementing policy rules and the mapping between ODRL operands and Rego methods.

#### Configuring Policy Evaluation

The FDSC-EDC provides two configuration mechanisms that control how policies are processed and at which evaluation points they apply.

##### Additional Contexts

The FDSC-EDC can include additional JSON-LD contexts in every PAP validation request. These control how ODRL policy terms are compacted during JSON-LD processing, allowing the PAP to route terms like `odrl:use` to protocol-specific Rego implementations (e.g., `dspace:use`).

The contexts are loaded at startup from a JSON file configured via:

```properties
odrlPap.policy.additionalContextsPath=/etc/edc/additional-contexts.json
```

**Example**: Remap `odrl:use` to `dspace:use` inside `odrl:action`:

```json
[
  {
    "odrl:action": {
      "@id": "http://www.w3.org/ns/odrl/2/action",
      "@type": "@id",
      "@context": {
        "odrl": null,
        "dspace": { "@id": "http://www.w3.org/ns/odrl/2/", "@prefix": true }
      }
    }
  }
]
```

After JSON-LD expand-then-compact, a permission with `"action": "use"` will be compacted to `"action": "dspace:use"`, allowing the PAP to map it to the correct Rego rule via `mapping.json`.

> **Important**: `"odrl": null` is required in the scoped context. Without it, both prefixes map to the same namespace IRI and the compactor may still choose `odrl`. `"@type": "@id"` is also required for prefix compaction to apply to the values.

For more details on the JSON-LD scoped context mechanism, see the [FDSC-EDC additional contexts documentation](https://github.com/SEAMWARE/fdsc-edc/blob/ticket-36/work/policy-extension/additional-contexts.md).

##### Scope Mappings

By default, ODRL permissions that include a constraint are evaluated at all three Layer 2 scopes. Scope mappings allow restricting specific constraints to only the scopes where they are meaningful:

```properties
odrlPap.policy.scopeMappingsPath=/etc/edc/scope-mappings.json
```

**Example**: Evaluate `dayOfWeek` only during transfer, `membershipType` only during negotiation:

```json
{
  "mappings": [
    {
      "match": {
        "http://www.w3.org/ns/odrl/2/leftOperand":
          "http://www.w3.org/ns/odrl/2/dayOfWeek"
      },
      "scopes": ["transfer.process"]
    },
    {
      "match": {
        "http://www.w3.org/ns/odrl/2/leftOperand":
          "https://w3id.org/dspace/2024/1/membershipType"
      },
      "scopes": ["contract.negotiation"]
    }
  ]
}
```

Permissions without a matching scope mapping are sent to the PAP at every Layer 2 evaluation point.

##### Including Scope in the Permission

Instead of relying on external scope-mapping configuration, policy authors can include the scope directly in the ODRL permission using `dspace:scope`. This makes the policy self-contained — the evaluation point is explicit in the policy definition and travels with it.

When the `dspace:scope` property is included in a permission, the FDSC-EDC uses it directly instead of consulting the scope mappings file. The permission is only sent to the PAP when the current evaluation scope matches the declared scope.

```json
{
  "permission": [{
    "action": "use",
    "dspace:scope": "transfer.process",
    "constraint": {
      "leftOperand": "odrl:dayOfWeek",
      "operator": "lt",
      "rightOperand": {
        "@value": 6,
        "@type": "xsd:integer"
      }
    }
  }]
}
```

This approach is useful when:
- The policy needs to be portable between connectors with different scope mapping configurations
- The evaluation point is inherent to the constraint's semantics and should travel with the policy, not be configured externally

For more details on the policy context and what information is available at each evaluation layer, see the [FDSC-EDC policy context documentation](https://github.com/SEAMWARE/fdsc-edc/blob/ticket-36/work/policy-extension/policy-context.md).
#### Using the BAE Marketplace

To acquire the offering through the BAE Marketplace, you need to access the marketplace's graphical interface and simply follow the same steps as for purchasing any offering, but selecting the offering created in the previous step.

### Order through DSP

The same product now can be negotiated through the Dataspace Protocol. The interactions will be controlled through the management API of the FDSC-EDC Controlplane, authentication is done by the connector's and cannot be related to the actual actor.

#### DCP

1. Read the catalog:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/catalog/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "CatalogRequestMessage",
        "protocol": "dataspace-protocol-http:2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "counterPartyAddress": "https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
        "querySpec": {
        }
    }' | jq .
```

2. Start the negotiation by selecting the offer:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/contractnegotiations' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "ContractRequest",
        "counterPartyAddress": "https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "protocol": "dataspace-protocol-http:2025-1",
        "policy": {
            "@context": [
              "http://www.w3.org/ns/odrl.jsonld",
              "https://w3id.org/dspace/2024/1/context.json"
            ],    
            "@type": "Offer",
            "@id": "OFFER-1:ASSET-1:123",
            "assigner": "did:web:mp-operations.org",
            "permission": [{
              "action":  "use",
              "constraint": {
                "leftOperand": "odrl:dayOfWeek",
                "operator": "lt",
                "rightOperand": {
                  "@value": 6,
                  "@type": "xsd:integer"
                }
              }},
              {
                "action": "use",
                  "constraint": {
                    "leftOperand": "dspace:membershipType",
                    "operator": "eq",
                    "rightOperand": "FullMember"
                  }
              }
            ],
            "target": "ASSET-1"
        }
    }' | jq .
```

3. Check the negotiation state:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/contractnegotiations/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' | jq .
```

4. When the state of the negotiation is "finalized", the agreement id can be retrieved:

```shell
export AGREEMENT_ID=$(curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/contractnegotiations/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' | jq -r '.[0].contractAgreementId'); echo ${AGREEMENT_ID}
```

5. With the aggreement, the transfer can be started:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/transferprocesses' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw "{
    \"@context\": [
        \"https://w3id.org/edc/connector/management/v0.0.1\"
    ],
    \"assetId\": \"ASSET-1\",
    \"counterPartyId\": \"did:web:mp-operations.org\",
    \"counterPartyAddress\":  \"https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1\",
    \"connectorId\": \"did:web:mp-operations.org\",
    \"contractId\": \"${AGREEMENT_ID}\",
    \"protocol\": \"dataspace-protocol-http:2025-1\",
    \"transferType\": \"HttpData-PULL\"
}" | jq .
```

6. Retrieve the transfer state:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/transferprocesses/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "@context": ["https://w3id.org/edc/connector/management/v0.0.1"],
    "@type": "QuerySpec"
  }' | jq .
```

7. Get the transfer Id and use it for retrieving endpoint-url and access-token:
```shell
export TRANSFER_ID=$(curl -k -x localhost:8888 -s -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/transferprocesses/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "@context": ["https://w3id.org/edc/connector/management/v0.0.1"],
    "@type": "QuerySpec"
  }' | jq -r '.[]."@id"'); echo Transfer ID: ${TRANSFER_ID}
export ENDPOINT=$(curl -k -x localhost:8888 -s -X GET "https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/edrs/${TRANSFER_ID}/dataaddress" | jq -r .endpoint); echo Endpoint: ${ENDPOINT}
export ACCESS_TOKEN=$(curl -k -x localhost:8888 -s -X GET "https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/edrs/${TRANSFER_ID}/dataaddress" | jq -r .token); echo Access Token: ${ACCESS_TOKEN}
```

8. Access the service:
```shell
curl -L -s -k -x localhost:8888 -X GET ${ENDPOINT}/ngsi-ld/v1/entities/urn:ngsi-ld:UptimeReport:fms-1 \
  --header 'Content-Type: application/json' \
  --header "Authorization: Bearer ${ACCESS_TOKEN}" | jq .
```

#### Advanced Policy Scenarios

The following scenarios demonstrate additional policy capabilities of the FDSC-EDC. They build on the existing demo setup and assume all previous steps (identity setup, credential issuance, data preparation, offering creation) have been completed.

The ODRL operands used in these scenarios (`dspace:credentialType`, `dspace:membershipStatus`, etc.) require corresponding Rego implementations in the ODRL-PAP. See the [ODRL-PAP Rego documentation](https://github.com/SEAMWARE/odrl-pap/blob/main/doc/REGO.md) for details on implementing and registering policy rules.

##### Scenario A: Offer Not Visible Due to Access Policy

This scenario creates an offer that is invisible to the consumer because the access policy requires a `PremiumPartnerCredential` that the consumer does not hold. The access policy is evaluated at the `catalog` scope — when the provider builds the catalog response, each asset's access policy is checked against the consumer's verified credentials. Assets that fail are excluded from the response entirely.

1. Create a product specification for a restricted asset:
```shell
export RESTRICTED_SPEC_ID=$(curl -k -x localhost:8888 -X 'POST' \
    'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productSpecification' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d '{
           "name": "Restricted Spec",
           "externalId": "ASSET-2",
           "@schemaLocation": "https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json",
           "productSpecCharacteristic": [
               {
                   "id": "dcp",
                   "name":"Endpoint, that the service can be negotiated at via DCP.",
                   "valueType":"endpointUrl",
                   "productSpecCharacteristicValue": [{
                      "value":"https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
                      "isDefault": true
                   }]
               },
               {
                   "id": "upstreamAddress",
                   "name":"Address of the upstream serving the data",
                   "valueType":"upstreamAddress",
                   "productSpecCharacteristicValue": [{
                       "value":"data-service-scorpio:9090",
                       "isDefault": true
                   }]
               }
           ]
    }' | jq '.id' -r); echo ${RESTRICTED_SPEC_ID}
```

2. Create the offering with a restrictive access policy requiring a `PremiumPartnerCredential`:
```shell
restricted_access_policy_id=$(uuidgen)
restricted_contract_policy_id=$(uuidgen)
curl -k -x localhost:8888 -X 'POST' \
    'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productOffering' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d "{
          \"name\": \"Premium Partner Offering\",
          \"description\": \"Only visible to premium partners\",
          \"isBundle\": false,
          \"isSellable\": true,
          \"lifecycleStatus\": \"Active\",
          \"@schemaLocation\": \"https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json\",
          \"externalId\": \"OFFER-2\",
          \"productSpecification\":
              {
                  \"id\": \"${RESTRICTED_SPEC_ID}\",
                  \"name\":\"Restricted Spec\"
              },
          \"category\": [{
              \"id\": \"${CATEGORY_ID}\"
          }],
          \"productOfferingTerm\": [
          {
              \"name\": \"edc:contractDefinition\",
              \"@schemaLocation\": \"https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/contract-definition.json\",
              \"accessPolicy\": {
                \"@context\": [
                  \"http://www.w3.org/ns/odrl.jsonld\",
                  \"https://w3id.org/dspace/2024/1/context.json\"
                ],
                \"odrl:uid\": \"${restricted_access_policy_id}\",
                \"assigner\": \"did:web:mp-operations.org\",
                \"permission\": [{
                    \"action\":  \"use\",
                    \"constraint\": {
                      \"leftOperand\": \"dspace:credentialType\",
                      \"operator\": \"eq\",
                      \"rightOperand\": \"PremiumPartnerCredential\"
                    }
                }],
                \"@type\":  \"Offer\"
              },
              \"contractPolicy\": {
                \"@context\": \"http://www.w3.org/ns/odrl.jsonld\",
                \"odrl:uid\": \"${restricted_contract_policy_id}\",
                \"assigner\": \"did:web:mp-operations.org\",
                \"permission\": [{
                    \"action\":  \"use\"
                }],
                \"@type\":  \"Offer\"
              }
          }]
        }" | jq .
```

3. Request the catalog from the consumer side. The consumer only holds a `MembershipCredential`, not a `PremiumPartnerCredential`, so ASSET-2 will not appear:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/catalog/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "CatalogRequestMessage",
        "protocol": "dataspace-protocol-http:2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "counterPartyAddress": "https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
        "querySpec": {
        }
    }' | jq .
```

The result will show only ASSET-1 (from the original offering). ASSET-2 is filtered out at the `catalog` scope because the access policy evaluation determined the consumer lacks the required credential.

##### Scenario B: Offer Visible but Not Contractable (Contract Policy)

This scenario creates an offer that appears in the catalog (permissive access policy) but cannot be negotiated because the contract policy requires a specific claim value the consumer's credential does not satisfy. This pattern is useful for "browse-only" catalogs where providers want consumers to discover offerings and understand the requirements before acquiring the necessary credentials.

1. Create a product specification:
```shell
export VISIBLE_SPEC_ID=$(curl -k -x localhost:8888 -X 'POST' \
    'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productSpecification' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d '{
           "name": "Visible But Restricted Spec",
           "externalId": "ASSET-3",
           "@schemaLocation": "https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json",
           "productSpecCharacteristic": [
               {
                   "id": "dcp",
                   "name":"Endpoint, that the service can be negotiated at via DCP.",
                   "valueType":"endpointUrl",
                   "productSpecCharacteristicValue": [{
                      "value":"https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
                      "isDefault": true
                   }]
               },
               {
                   "id": "upstreamAddress",
                   "name":"Address of the upstream serving the data",
                   "valueType":"upstreamAddress",
                   "productSpecCharacteristicValue": [{
                       "value":"data-service-scorpio:9090",
                       "isDefault": true
                   }]
               }
           ]
    }' | jq '.id' -r); echo ${VISIBLE_SPEC_ID}
```

2. Create the offering with an open access policy but a restrictive contract policy requiring `membershipStatus` to be `"premium"`:
```shell
visible_access_policy_id=$(uuidgen)
visible_contract_policy_id=$(uuidgen)
curl -k -x localhost:8888 -X 'POST' \
    'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productOffering' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d "{
          \"name\": \"Browse-Only Offering\",
          \"description\": \"Visible to all, contractable only by premium members\",
          \"isBundle\": false,
          \"isSellable\": true,
          \"lifecycleStatus\": \"Active\",
          \"@schemaLocation\": \"https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json\",
          \"externalId\": \"OFFER-3\",
          \"productSpecification\":
              {
                  \"id\": \"${VISIBLE_SPEC_ID}\",
                  \"name\":\"Visible But Restricted Spec\"
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
                \"odrl:uid\": \"${visible_access_policy_id}\",
                \"assigner\": \"did:web:mp-operations.org\",
                \"permission\": [{
                    \"action\":  \"use\"
                }],
                \"@type\":  \"Offer\"
              },
              \"contractPolicy\": {
                \"@context\": [
                  \"http://www.w3.org/ns/odrl.jsonld\",
                  \"https://w3id.org/dspace/2024/1/context.json\"
                ],
                \"odrl:uid\": \"${visible_contract_policy_id}\",
                \"assigner\": \"did:web:mp-operations.org\",
                \"permission\": [{
                    \"action\":  \"use\",
                    \"constraint\": {
                      \"leftOperand\": \"dspace:membershipStatus\",
                      \"operator\": \"eq\",
                      \"rightOperand\": \"premium\"
                    }
                }],
                \"@type\":  \"Offer\"
              }
          }]
        }" | jq .
```

3. Request the catalog — ASSET-3 will appear alongside ASSET-1:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/catalog/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "CatalogRequestMessage",
        "protocol": "dataspace-protocol-http:2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "counterPartyAddress": "https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
        "querySpec": {
        }
    }' | jq .
```

4. Attempt to negotiate ASSET-3 — the negotiation will be rejected:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/contractnegotiations' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "ContractRequest",
        "counterPartyAddress": "https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "protocol": "dataspace-protocol-http:2025-1",
        "policy": {
            "@context": [
              "http://www.w3.org/ns/odrl.jsonld",
              "https://w3id.org/dspace/2024/1/context.json"
            ],
            "@type": "Offer",
            "@id": "OFFER-3:ASSET-3:456",
            "assigner": "did:web:mp-operations.org",
            "permission": [{
              "action": "use",
              "constraint": {
                "leftOperand": "dspace:membershipStatus",
                "operator": "eq",
                "rightOperand": "premium"
              }
            }],
            "target": "ASSET-3"
        }
    }' | jq .
```

5. Check the negotiation state — it will show `TERMINATED` (rejected):
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v3/contractnegotiations/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' | jq '.[] | select(.state == "TERMINATED")'
```

The consumer can browse the offer (access policy allows it) but cannot form a contract because the contract policy requires `membershipStatus == "premium"`, which the consumer's `MembershipCredential` does not satisfy.

##### Scenario C: Explicit Scope Declaration in Policy

The original demo offering uses external scope-mapping configuration (see [Scope Mappings](#scope-mappings)) to control which permissions are evaluated at which scope. This scenario demonstrates the alternative approach: including the scope directly in the ODRL permission using `dspace:scope`.

This makes the policy self-documenting — the evaluation point is part of the policy itself rather than an external configuration concern.

1. Create a product specification:
```shell
export SCOPED_SPEC_ID=$(curl -k -x localhost:8888 -X 'POST' \
    'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productSpecification' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d '{
           "name": "Scoped Policy Spec",
           "externalId": "ASSET-4",
           "@schemaLocation": "https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json",
           "productSpecCharacteristic": [
               {
                   "id": "dcp",
                   "name":"Endpoint, that the service can be negotiated at via DCP.",
                   "valueType":"endpointUrl",
                   "productSpecCharacteristicValue": [{
                      "value":"https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
                      "isDefault": true
                   }]
               },
               {
                   "id": "upstreamAddress",
                   "name":"Address of the upstream serving the data",
                   "valueType":"upstreamAddress",
                   "productSpecCharacteristicValue": [{
                       "value":"data-service-scorpio:9090",
                       "isDefault": true
                   }]
               }
           ]
    }' | jq '.id' -r); echo ${SCOPED_SPEC_ID}
```

2. Create the offering with explicit scopes in the contract policy permissions. Note how each permission declares its own `dspace:scope` instead of relying on the scope-mappings configuration:
```shell
scoped_access_policy_id=$(uuidgen)
scoped_contract_policy_id=$(uuidgen)
curl -k -x localhost:8888 -X 'POST' \
    'https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productOffering' \
    -H 'accept: application/json;charset=utf-8' \
    -H 'Content-Type: application/json;charset=utf-8' \
    -d "{
          \"name\": \"Scoped Policy Offering\",
          \"description\": \"Uses explicit scope in permissions\",
          \"isBundle\": false,
          \"isSellable\": true,
          \"lifecycleStatus\": \"Active\",
          \"@schemaLocation\": \"https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/external-id.json\",
          \"externalId\": \"OFFER-4\",
          \"productSpecification\":
              {
                  \"id\": \"${SCOPED_SPEC_ID}\",
                  \"name\":\"Scoped Policy Spec\"
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
                \"odrl:uid\": \"${scoped_access_policy_id}\",
                \"assigner\": \"did:web:mp-operations.org\",
                \"permission\": [{
                    \"action\":  \"use\"
                }],
                \"@type\":  \"Offer\"
              },
              \"contractPolicy\": {
                \"@context\": [
                  \"http://www.w3.org/ns/odrl.jsonld\",
                  \"https://w3id.org/dspace/2024/1/context.json\"
                ],
                \"odrl:uid\": \"${scoped_contract_policy_id}\",
                \"assigner\": \"did:web:mp-operations.org\",
                \"permission\": [
                  {
                      \"action\":  \"use\",
                      \"dspace:scope\": \"transfer.process\",
                      \"constraint\": {
                        \"leftOperand\": \"odrl:dayOfWeek\",
                        \"operator\": \"lt\",
                        \"rightOperand\": {
                          \"@value\": 6,
                          \"@type\": \"xsd:integer\"
                        }
                      }
                  },
                  {
                      \"action\":  \"use\",
                      \"dspace:scope\": \"contract.negotiation\",
                      \"constraint\": {
                        \"leftOperand\": \"dspace:membershipType\",
                        \"operator\": \"eq\",
                        \"rightOperand\":\"FullMember\"
                      }
                  }
                ],
                \"@type\":  \"Offer\"
              }
          }]
        }" | jq .
```

Compared to the original offering (OFFER-1), this achieves the same behavior:
- `dayOfWeek` is only checked during transfer
- `membershipType` is only checked during negotiation

The difference is that these constraints are explicit in the policy itself, making the policy portable. A different FDSC-EDC instance does not need matching entries in its `scope-mappings.json` to correctly evaluate this policy.

3. The negotiation and transfer flow follows the same steps as the original DCP flow (steps 1–8 above), substituting `OFFER-4`, `ASSET-4`, and the corresponding offer ID from the catalog response.

### OID4VC

The exact same process can be done through connectors authenticating via OID4VC. In order to keep it simple, we will not run the full negotiation process again, since an agreement can be used independetly from the protocol it was negotiated under. However, the catalog will be read and the transfer process will be setup using OID4VC.
In order to use OID4VC, the management API of the OID4VC instance of the controlplane has to be used.

1. Read the catalog:
```shell
curl -k -x localhost:8888 -s -X POST \
  'https://dsp-oid4vc-management.127.0.0.1.nip.io/api/v1/management/v3/catalog/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
        "@context": [
            "https://w3id.org/edc/connector/management/v0.0.1"
        ],
        "@type": "CatalogRequestMessage",
        "protocol": "dataspace-protocol-http:2025-1",
        "counterPartyId": "did:web:mp-operations.org",
        "counterPartyAddress": "https://dsp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1",
        "querySpec": {
        }
    }' | jq .
```

2. Request the transfer with the Agreement created via DCP:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-oid4vc-management.127.0.0.1.nip.io/api/v1/management/v3/transferprocesses' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw "{
    \"@context\": [
        \"https://w3id.org/edc/connector/management/v0.0.1\"
    ],
    \"assetId\": \"ASSET-1\",
    \"counterPartyId\": \"did:web:mp-operations.org\",
    \"counterPartyAddress\":  \"https://dsp-mp-operations.127.0.0.1.nip.io/api/dsp/2025-1\",
    \"connectorId\": \"did:web:mp-operations.org\",
    \"contractId\": \"${AGREEMENT_ID}\",
    \"dataDestination\": {
        \"type\": \"HttpProxy\"
    },
    \"protocol\": \"dataspace-protocol-http:2025-1\",
    \"transferType\": \"HttpData-PULL\"
}" | jq .
```

3. Get the transfer state:
```shell
curl -k -x localhost:8888 -X POST \
  'https://dsp-oid4vc-management.127.0.0.1.nip.io/api/v1/management/v3/transferprocesses/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "@context": ["https://w3id.org/edc/connector/management/v0.0.1"],
    "@type": "QuerySpec"
  }' | jq .
```

4. Retrieve the endpoint:
```shell
export TRANSFER_ID=$(curl -k -x localhost:8888 -X POST \
  'https://dsp-oid4vc-management.127.0.0.1.nip.io/api/v1/management/v3/transferprocesses/request' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "@context": ["https://w3id.org/edc/connector/management/v0.0.1"],
    "@type": "QuerySpec"
  }' | jq -r '.[]."@id"'); echo ${TRANSFER_ID}
export ENDPOINT=$(curl -k -x localhost:8888 -X GET "https://dsp-oid4vc-management.127.0.0.1.nip.io/api/v1/management/v3/edrs/${TRANSFER_ID}/dataaddress" | jq -r .endpoint); echo ${ENDPOINT}
```

5. Request without token fails:
```shell
curl -k -x localhost:8888 -X GET ${ENDPOINT}/ngsi-ld/v1/entities/urn:ngsi-ld:UptimeReport:fms-1
```

6. Request openid-configuration:
```shell
curl -k -x localhost:8888 -X GET ${ENDPOINT}/.well-known/openid-configuration | jq .
```

7. Access via OID4VP:
```shell
export OPERATOR_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-consumer.127.0.0.1.nip.io operator-credential operator)
export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh ${ENDPOINT} $OPERATOR_CREDENTIAL openid); echo Access Token: $ACCESS_TOKEN
curl -k -x localhost:8888 -X GET ${ENDPOINT}/ngsi-ld/v1/entities/urn:ngsi-ld:UptimeReport:fms-1 \
  --header 'Content-Type: application/json' \
  --header "Authorization: Bearer ${ACCESS_TOKEN}" | jq .
```
