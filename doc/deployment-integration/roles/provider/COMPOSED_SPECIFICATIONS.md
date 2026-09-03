# Authoring a composed ProductSpecification

An access policy protects an API, and in TMForum an API is a `ServiceSpecification`. A provider
selling one product built from three services can therefore author three policies where they belong,
instead of merging them into one policy on the `ProductSpecification`.

`contract-management` reads the configuration of the composed `ServiceSpecification`s when it
activates an order. This page is the provider's view of that: how to switch it on, how to author it,
and which rules decide what a customer ends up being allowed to do.

> The model behind it, including the decisions and their rationale, is in
> [`doc/tmforum/service-policy.md`](../../../tmforum/service-policy.md).

## Switching it on

```yaml
contract-management:
  # read the configuration of the composed ServiceSpecifications as well
  enableSpecificationComposition: true
  # how many specification levels below the ordered product are resolved
  specificationCompositionMaxDepth: 2
```

| Component | Minimum version |
|---|---|
| `contract-management` | chart `3.5.36` (app `3.3.12`) |
| `trusted-issuers-list` | app `0.9.1` |

The connector chart pins the `trusted-issuers-list` image until the chart chain catches up; see the
[10.5.0 release note](../../../release-notes/10-x.md).

Leaving the flag off keeps the previous behaviour exactly: only the ordered `ProductSpecification`
is read.

## Which characteristic belongs where

The split is deliberate: **the enforcement plane moves down to the service, the DSP description plane
stays at the product.**

| Characteristic | Level | Read by |
|---|---|---|
| `authorizationPolicy` | `ServiceSpecification` (product level still works for product-wide rules) | contract-management → ODRL-PAP |
| `purpose` | `ServiceSpecification` | consent-facade |
| `credentialsConfiguration` | `ProductSpecification`, additionally allowed per service | contract-management → trusted-issuers-list |
| `endpointUrl`, `endpointDescription`, `transferType`, `transferPath` | `ProductSpecification` | fdsc-edc (DSP catalog) |
| `upstreamAddress`, `serviceConfiguration`, `targetSpecification` | `ProductSpecification` | fdsc-transfer-extension |

The last two rows exist for the DSP integration, and DSP has no notion of a composed product: one
`ProductSpecification` is one dataset with one set of distributions. Authoring them on a
`ServiceSpecification` would put them where nothing reads them.

## The container names differ

The same concept is spelled differently per API — this is the most common authoring mistake:

| Entity | Characteristic list | Value list |
|---|---|---|
| `ProductSpecification` | `productSpecCharacteristic` | `productSpecCharacteristicValue` |
| `ServiceSpecification` | **`specCharacteristic`** | **`characteristicValueSpecification`** |

`id`, `name` and `valueType` are the same on both, and `valueType` is what the connector
discriminates on. A characteristic without a `valueType` is skipped.

## Worked example

Continuing the "M&P K8S" example of the [local deployment](../../local-deployment/LOCAL.MD): the
product is composed of one service that carries the cluster-creation policy.

**1 — the service specification, with the policy**

```shell
export SERVICE_SPEC_ID=$(curl -k -x localhost:8888 -X 'POST' https://tm-forum-api.127.0.0.1.nip.io/tmf-api/serviceCatalogManagement/v4/serviceSpecification \
  -H 'Content-Type: application/json;charset=utf-8' \
  -d "{
    \"name\": \"M&P K8S Cluster API\",
    \"version\": \"1.0.0\",
    \"lifecycleStatus\": \"ACTIVE\",
    \"specCharacteristic\": [
      {
        \"id\": \"policyConfig\",
        \"name\": \"Policy for creation of K8S clusters.\",
        \"valueType\": \"authorizationPolicy\",
        \"characteristicValueSpecification\": [
          {
            \"isDefault\": true,
            \"value\": {
              \"@context\": { \"odrl\": \"http://www.w3.org/ns/odrl/2/\" },
              \"@id\": \"https://mp-operation.org/policy/common/k8s-small\",
              \"odrl:uid\": \"https://mp-operation.org/policy/common/k8s-small\",
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
                \"odrl:action\": \"odrl:use\"
              }
            }
          }
        ]
      }
    ]
  }" | jq -r '.id')
```

**2 — the product specification, referencing it**

The credential configuration stays at the product level: it describes what the *customer's* issuer
may assert, which is a commercial fact about the product rather than about one of its APIs.

```shell
export PRODUCT_SPEC_ID=$(curl -k -x localhost:8888 -X 'POST' https://tm-forum-api.127.0.0.1.nip.io/tmf-api/productCatalogManagement/v4/productSpecification \
  -H 'Content-Type: application/json;charset=utf-8' \
  -d "{
    \"brand\": \"M&P Operations\",
    \"version\": \"1.0.0\",
    \"lifecycleStatus\": \"ACTIVE\",
    \"name\": \"M&P K8S Small\",
    \"serviceSpecification\": [
      { \"id\": \"${SERVICE_SPEC_ID}\" }
    ],
    \"productSpecCharacteristic\": [
      {
        \"id\": \"credentialsConfig\",
        \"name\": \"Credentials Config\",
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
                  \"allowedValues\": [ \"OPERATOR\" ]
                }
              ]
            }
          }
        ]
      }
    ]
  }" | jq -r '.id')
```

The referenced service specification has to exist first — the TMForum API validates the reference and
rejects the product specification otherwise.

**3 — the offering** is unchanged: it references the `ProductSpecification` as before.

Ordering that offering now installs the service's policy at the ODRL-PAP and grants the product's
credential configuration at the trusted-issuers-list.

## The rules that decide what a customer may do

**Union, never override.** The effective configuration of a product is the union over the product
and its services. No level narrows another.

**Credential configurations widen.** The trusted-issuers-list evaluates several configurations of one
credential type as an OR, so a permissive entry relaxes a restrictive one. If a service is part of a
"small" and a "full" product, the small product's customer is *not* restricted by the small product's
entry alone — author the restriction where it is not shared, or split the service.

**Policies are keyed by `odrl:uid`.** Identical policies from several parts are installed once. Two
*different* policies claiming the same `odrl:uid` fail the activation, because the ODRL-PAP keys an
installed policy by that uid plus the order id and could only ever keep one of them. Give every
policy its own uid.

**One provider per composition.** A `ServiceSpecification` whose `relatedParty` names a different
provider than the product fails the activation. A part that names no provider inherits the product's
one, which is what the marketplace produces. Composing across providers is not supported.

**The order is a snapshot.** The configuration is resolved when the order completes, and not
re-resolved when a specification changes afterwards. Editing a shared service specification does not
retroactively change what existing orders granted.

**Revocation is per order.** Credentials are granted at the trusted-issuers-list under the id of the
order that granted them, so cancelling one order leaves what another order granted in place — which
is what makes sharing a service specification safe.

## When something does not activate

`contract-management` says what it did or refused to do. The lines worth grepping for:

| Log line | Meaning |
|---|---|
| `The composition of specification … is deeper than the configured limit of N levels` | references below the limit were **not** applied. Raise `specificationCompositionMaxDepth` or flatten the composition |
| `The composition of specification … contains two different policies claiming the uid …` | two parts use one `odrl:uid` for different policies; nothing was installed |
| `The composition of specification … declares more than one provider` | a part names a different provider party |
| `The … specification … referenced by specification … could not be resolved` | a dangling reference — the catalog is broken, and the activation failed rather than granting a partial configuration |
| `The offering … does not reference a product specification` | a bundled offering; it configures nothing by itself |
| `The trusted-issuers-list rejected the credentials granted by order … with status 404` | the deployed trusted-issuers-list is older than `0.9.1` |

A truncation is the one case that leaves an order **partially** activated, which is why it is logged
as a warning naming the specification and the references it skipped.
