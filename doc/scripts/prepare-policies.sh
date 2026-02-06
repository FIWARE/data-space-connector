#! /usr/bin/env bash

set -e

curl -s -f -X 'POST' http://pap-provider.127.0.0.1.nip.io:8080/policy \
    -H 'Content-Type: application/json' \
    -d  '{
          "@context": {
            "dc": "http://purl.org/dc/elements/1.1/",
            "dct": "http://purl.org/dc/terms/",
            "owl": "http://www.w3.org/2002/07/owl#",
            "odrl": "http://www.w3.org/ns/odrl/2/",
            "rdfs": "http://www.w3.org/2000/01/rdf-schema#",
            "skos": "http://www.w3.org/2004/02/skos/core#"
          },
          "@id": "https://mp-operation.org/policy/common/offering",
          "odrl:uid": "https://mp-operation.org/policy/common/offering",
          "@type": "odrl:Policy",
          "odrl:permission": {
            "odrl:assigner": {
              "@id": "https://www.mp-operation.org/"
            },
            "odrl:target": {
              "@type": "odrl:AssetCollection",
              "odrl:source": "urn:asset",
              "odrl:refinement": [
                {
                  "@type": "odrl:Constraint",
                  "odrl:leftOperand": "tmf:resource",
                  "odrl:operator": {
                    "@id": "odrl:eq"
                  },
                  "odrl:rightOperand": "productOffering"
                }
              ]
            },
            "odrl:assignee": {
              "@id": "vc:any"
            },
            "odrl:action": {
              "@id": "odrl:read"
            }
          }
        }'

curl -s -f -X 'POST' http://pap-provider.127.0.0.1.nip.io:8080/policy \
    -H 'Content-Type: application/json' \
    -d  '{
          "@context": {
            "dc": "http://purl.org/dc/elements/1.1/",
            "dct": "http://purl.org/dc/terms/",
            "owl": "http://www.w3.org/2002/07/owl#",
            "odrl": "http://www.w3.org/ns/odrl/2/",
            "rdfs": "http://www.w3.org/2000/01/rdf-schema#",
            "skos": "http://www.w3.org/2004/02/skos/core#"
          },
          "@id": "https://mp-operation.org/policy/common/selfRegistration",
          "odrl:uid": "https://mp-operation.org/policy/common/selfRegistration",
          "@type": "odrl:Policy",
          "odrl:permission": {
            "odrl:assigner": {
              "@id": "https://www.mp-operation.org/"
            },
            "odrl:target": {
              "@type": "odrl:AssetCollection",
              "odrl:source": "urn:asset",
              "odrl:refinement": [
                {
                  "@type": "odrl:Constraint",
                  "odrl:leftOperand": "tmf:resource",
                  "odrl:operator": {
                    "@id": "odrl:eq"
                  },
                  "odrl:rightOperand": "organization"
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
                    "odrl:leftOperand": {
                      "@id": "vc:role"
                    },
                    "odrl:operator": {
                      "@id": "odrl:hasPart"
                    },
                    "odrl:rightOperand": {
                      "@value": "REPRESENTATIVE",
                      "@type": "xsd:string"
                    }
                  },
                  {
                    "@type": "odrl:Constraint",
                    "odrl:leftOperand": {
                      "@id": "vc:type"
                    },
                    "odrl:operator": {
                      "@id": "odrl:hasPart"
                    },
                    "odrl:rightOperand": {
                      "@value": "UserCredential",
                      "@type": "xsd:string"
                    }
                  }
                ]
              }
            },
            "odrl:action": {
              "@id": "tmf:create"
            }
          }
        }'

curl -s -f -X 'POST' http://pap-provider.127.0.0.1.nip.io:8080/policy \
    -H 'Content-Type: application/json' \
    -d  '{
          "@context": {
            "dc": "http://purl.org/dc/elements/1.1/",
            "dct": "http://purl.org/dc/terms/",
            "owl": "http://www.w3.org/2002/07/owl#",
            "odrl": "http://www.w3.org/ns/odrl/2/",
            "rdfs": "http://www.w3.org/2000/01/rdf-schema#",
            "skos": "http://www.w3.org/2004/02/skos/core#"
          },
          "@id": "https://mp-operation.org/policy/common/ordering",
          "odrl:uid": "https://mp-operation.org/policy/common/ordering",
          "@type": "odrl:Policy",
          "odrl:permission": {
            "odrl:assigner": {
              "@id": "https://www.mp-operation.org/"
            },
            "odrl:target": {
              "@type": "odrl:AssetCollection",
              "odrl:source": "urn:asset",
              "odrl:refinement": [
                {
                  "@type": "odrl:Constraint",
                  "odrl:leftOperand": "tmf:resource",
                  "odrl:operator": {
                    "@id": "odrl:eq"
                  },
                  "odrl:rightOperand": "productOrder"
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
                    "odrl:leftOperand": {
                      "@id": "vc:role"
                    },
                    "odrl:operator": {
                      "@id": "odrl:hasPart"
                    },
                    "odrl:rightOperand": {
                      "@value": "REPRESENTATIVE",
                      "@type": "xsd:string"
                    }
                  },
                  {
                    "@type": "odrl:Constraint",
                    "odrl:leftOperand": {
                      "@id": "vc:type"
                    },
                    "odrl:operator": {
                      "@id": "odrl:hasPart"
                    },
                    "odrl:rightOperand": {
                      "@value": "UserCredential",
                      "@type": "xsd:string"
                    }
                  }
                ]
              }
            },
            "odrl:action": {
              "@id": "tmf:create"
            }
          }
        }'