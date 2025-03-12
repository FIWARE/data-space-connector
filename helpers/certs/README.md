# Certs

Helper folder to create the certificate chain required for running the local deployment with a valid did:web, configured for usage in a Gaia-X Participants registry.

It creates:
1. A root-ca
2. An intermediate-ca
3. The client certificate
4. Generates the required kubernetes resources and updates them in the local deployments.

Run it with: 
``` 
    ./generate-certs.sh <LOCAL_LOCATION>/data-space-connector/k3s
```