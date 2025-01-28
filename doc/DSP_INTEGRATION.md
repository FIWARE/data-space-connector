# Integration of the Dataspace Protocol

In order to be compatible with other connectors, the FIWARE Data Space Connector supports the [IDSA Dataspace Protocol](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol).

## Architecture


![DSP Architecture](./img/dsp-architecture.png)

* [TMForum API](https://github.com/FIWARE/tmforum-api) provides the APIs for creating and managing Products and Offerings
    * integrates with the Contract Management through Events
* [Rainbow](https://github.com/ging/rainbow)(not yet public) is a RUST-implementation of the [Dataspace Protocol](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol)
    * provides the  [Transfer Process API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/transfer-process/transfer.process.protocol)
    * provides the [Catalog API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/catalog/catalog.protocol)
    * provides CRUD functionality for Agreement objects definde by the [Contract Negotiation API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/contract-negotiation/contract.negotiation.protocol)
    * provides the callback interface for the Consumer
* [Contract Management](https://github.com/FIWARE/contract-management):
    * integrates with the TMForum API and translate its entities to [DCAT Entries](https://www.w3.org/TR/vocab-dcat-3/) in Rainbow([Catalogs](https://www.w3.org/TR/vocab-dcat-3/#Class:Catalog) and [DataServices](https://www.w3.org/TR/vocab-dcat-3/#Class:Data_Service))
    * create Agreements in Rainbow based on the Product Orderings
    * writes back the Agreemnt-IDs to the TMForum API Product Orderings

## Usage

In order to access a Service by using the Transfer Process Protocol, the following steps need to be taken:

> :warning: The example calls are using the [local deployment](./deployment-integration/local-deployment/LOCAL.MD). Make sure its running before trying them out.

### Create the offering

In order to be compatible with the Dataspace Protocol, a Data Service needs to be available at the [Catalog API](https://docs.internationaldataspaces.org/ids-knowledgebase/dataspace-protocol/catalog/catalog.protocol) in the [DCAT-Format](https://www.w3.org/TR/vocab-dcat-3/). To do so, Catalogs, Categories and ProductOfferings from TMForum are mapped to the entries by the Contract Management.
With the following steps, a catalog containing a DataService can be created:

>:warning: For better understandability, the creation happens directly through the TMForum-API, without authentication.

1. Create a Category(to match offering and catalog):

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

2. Create a Catalog:

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

3. Create a product specification. In order to be mapped to DCAT, it needs to contain the ```productSpecCharacteristic``` ```endpointUrl``` and ```endpointDescription```:


```shell
export PRODUCT_SPEC_ID=$(curl -X 'POST' \
  'http://tm-forum-api.127.0.0.1.nip.io:8080/tmf-api/productCatalogManagement/v4/productSpecification' \
  -H 'accept: application/json;charset=utf-8' \
  -H 'Content-Type: application/json;charset=utf-8' \
  -d "{
        \"name\": \"Test Spec\", 
        \"productSpecCharacteristic\": [
            {
                \"id\": \"endpointUrl\",
                \"name\":\"Service Endpoint URL\",
                \"valueType\":\"endpointUrl\",
                \"productSpecCharacteristicValue\": [{
                    \"value\":\"https://the-test-service.org\",
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
            }
        ]
    }" | jq .id -r); echo ${PRODUCT_SPEC_ID}
```

4. Create the Product Offering:


```shell
export PRODUCT_OFFERING_ID=$(curl -X 'POST' \
  'http://tm-forum-api.127.0.0.1.nip.io:8080/tmf-api/productCatalogManagement/v4/productOffering' \
  -H 'accept: application/json;charset=utf-8' \
  -H 'Content-Type: application/json;charset=utf-8' \
  -d "{
        \"name\": \"Test Offering\",
        \"description\": \"Test Offering description\", 
        \"isBundle\": false,
        \"isSellable\": true,
        \"lifecycleStatus\": \"Active\",
        \"productSpecification\": 
            {
                \"id\": \"${PRODUCT_SPEC_ID}\",
                \"name\":\"The Test Spec\"
            },
        \"category\": [{
            \"id\": \"${CATEGORY_ID}\"
        }]
    }" | jq .id -r); echo ${PRODUCT_OFFERING_ID}
```

After those steps, the catalog with the offering(as a ```dcat:service```) is available:

```shell
    curl -X GET 'http://rainbow-provider.127.0.0.1.nip.io:8080/api/v1/catalogs'
```

The result will be similar to the following:

```json
[
    {
        "@context": "https://w3id.org/dspace/2024/1/context.json",
        "@type": "dcat:Catalog",
        "@id": "urn:ngsi-ld:catalog:5b33f5bc-65e7-40b4-a71c-b722da52a919",
        "foaf:homepage": null,
        "dcat:theme": "",
        "dcat:keyword": "",
        "dct:conformsTo": null,
        "dct:creator": null,
        "dct:identifier": "urn:ngsi-ld:catalog:5b33f5bc-65e7-40b4-a71c-b722da52a919",
        "dct:issued": "2025-01-15T07:25:19.779168",
        "dct:modified": null,
        "dct:title": "Test Catalog",
        "dct:description": [],
        "dspace:participantId": null,
        "odrl:hasPolicy": [],
        "dspace:extraFields": null,
        "dcat:dataset": [],
        "dcat:service": [
            {
                "@context": "https://w3id.org/dspace/2024/1/context.json",
                "@type": "dcat:DataService",
                "@id": "urn:ngsi-ld:product-offering:96eaae6d-1615-41b0-b721-91c6a2e36551",
                "dcat:theme": "",
                "dcat:keyword": "",
                "dcat:endpointDescription": "The Test Service",
                "dcat:endpointURL": "https://the-test-service.org",
                "dct:conformsTo": null,
                "dct:creator": null,
                "dct:identifier": "urn:ngsi-ld:product-offering:96eaae6d-1615-41b0-b721-91c6a2e36551",
                "dct:issued": "2025-01-15T07:25:31.220506",
                "dct:modified": null,
                "dct:title": "Test Spec",
                "dct:description": [],
                "odrl:hasPolicy": [],
                "dspace:extraFields": null
            }
        ]
    }
]
```

