#!/bin/bash

# Allow self-registration of organizations at TMForum
curl -k -x localhost:8888 -X 'POST' https://pap-consumer.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/allowSelfRegistrationLegalPerson.json)"

# Allow to order at TMForum
curl -k -x localhost:8888 -X 'POST' https://pap-consumer.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/allowProductOrder.json)"

# Allow to offer at TMForum for identified Representatives
curl -k -x localhost:8888 -X 'POST' https://pap-consumer.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/allowProductOfferingCreation.json)"

# Allow to read offers at TMForum
curl -k -x localhost:8888 -X 'POST' https://pap-consumer.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/allowProductOffering.json)"

# Allow creation of product specs
curl -k -x localhost:8888 -X 'POST' https://pap-consumer.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/allowProductSpec.json)"
