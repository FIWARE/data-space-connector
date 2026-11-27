#!/bin/bash

# Allow read-access to the Rainbow Catalog API
curl -k -x localhost:8888 -X 'POST' https://pap-provider.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/allowCatalogRead.json)"


# Allow self-registration of organizations at TMForum
curl -k -x localhost:8888 -X 'POST' https://pap-provider.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/allowSelfRegistration.json)"


# Allow to order at TMForum
curl -k -x localhost:8888 -X 'POST' https://pap-provider.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/allowProductOrder.json)"

# Allow operators to read uptime-reports
curl -k -x localhost:8888 -X 'POST' https://pap-provider.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/uptimeReport.json)"

# Allow operators to request data transfers at Rainbow
curl -k -x localhost:8888 -X 'POST' https://pap-provider.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/transferRequest.json)"

# Allow the consumer to read its agreements
curl -k -x localhost:8888 -X 'POST' https://pap-provider.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d "$(cat ./it/src/test/resources/policies/allowTMFAgreementRead.json)"
