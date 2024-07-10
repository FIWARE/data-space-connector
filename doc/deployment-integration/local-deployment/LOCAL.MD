# Local Deployment

In order to support development and exploration of the FIWARE Data Space Connector, a "Minimal Viable Dataspace" is
provided as part of this repo.

## Quick Start

> :warning: The local deployment uses [k3s](https://k3s.io/) and is currently only tested on linux.

To start the Data Space, just use:

```shell
    mvn clean deploy -Plocal
```

Depending on the machine, it should take between 5 and 10min to spin up the complete data space. You can connect to the
running k3s-cluster via:

```shell
    export  KUBECONFIG=$(pwd)/target/k3s.yaml
    # get all deployed resources
    kubectl get all --all-namespaces
```

## The Data Space

![Overview](../../img/local/overview.jpg)

The locally deployed Data Space consists of 2 Participants, connected through a Trust Anchor.

### The Trust Anchor

Every Data Spaces requires a framework that ensures trust between the participants. Depending on the requirements of the
concrete Data Space,
this can become a rather complex topic. Various trust-providers exist(
f.e. [Gaia-X Digital Clearing Houses](https://gaia-x.eu/gxdch)) and could be reused, as long as
they provide an implementation of
the [EBSI-Trusted Issuers Registry](https://hub.ebsi.eu/apis/pilot/trusted-issuers-registry/v4) to the participants.

The local Data Spaces comes with the [FIWARE Trusted Issuers List](https://github.com/FIWARE/trusted-issuers-list) as a
rather simple implementation of that API, providing CRUD functionality
for Issuers and storage in an MySQL Database. After deployment, the API is available
at ```http://tir.127.0.0.1.nip.io:8080```. Both participants
are automatically registered as "Trusted Issuers" in the registry with their did's.

Get a list of the issuers:

```shell
    curl -X GET http://tir.127.0.0.1.nip.io:8080/v4/issuers
```

A new issuer could for example be registered via:

```shell
  curl -X POST http://til.127.0.0.1.nip.io:8080/issuer \
    --header 'Content-Type: application/json' \
    --data '{
      "did": "did:key:myKey",
      "credentials": []
    }'
```

For more information about the API, see
its [OpenAPI Spec](https://github.com/FIWARE/trusted-issuers-list/blob/main/api/trusted-issuers-list.yaml)

## The Participants

The minimal Data Space should provide an easy-to-understand introduction to the FIWARE Data Space. Therefor the roles of
the
participants are clearly seperated into "Data Consumer" and "Data Provider". However, in most real-world Data Spaces the
participants
will have both roles. They are not restricted to either consume or provide.

In our scenario, the Data Provider(`M&P Operations Inc.`) is a company offering solutions to host and operate digital
services for other companies. The Data Consumer(`Fancy Marketplace Co.`)
provides a marketplace solution, listing offers from other companies. To fulfill their roles, they need different
components of the FIWARE Data Space Connector.

### The Data Consumer

![Consumer](../../img/local/consumer.jpg)

Since the Data Consumer in our example is only retrieving data, it requires very few components:

* [Keycloak](https://github.com/keycloak/keycloak) - to issue VerifiableCredentials
* [did-helper](https://github.com/wistefan/did-helper) - a small helper application, providing the decentralized
  identity to be used for the local Data Space

After deployment, Keycloak can be used to issue VerifiableCredentials for users or services, to be used for
authorization at other participants of the Data Space.
It comes with 2 preconfigured users:

* the `keycloak-admin` - has a password generated during deployment, it can be retrieved
  via ```kubectl get secret -n consumer -o json issuance-secret | jq '.data."keycloak-admin"' -r | base64 --decode```
* the `test-user` - it has a fixed password, set to "test"

The admin-console of keycloak is available at: ```http://keycloak-consumer.127.0.0.1.nip.io:8080```, login with
the `keycloak-admin`
The credentials issuance in the account-console is available
at: ```http://keycloak-consumer.127.0.0.1.nip.io:8080/realms/test-realm/account/oid4vci```, login with the `test-user`

In order to retrieve an actual credential two ways are available:

* Use the account-console and retrieve the credential with a wallet. Currently, we cannot recommend any for a local use
  case.
* Get the credential via http-requests through the `SameDevice-Flow`:

> :warning: The pre-authorized code and the offer expire within 30s for security reasons. Be fast.

> :bulb: In case you did the demo before, you can use the following snippet to unset the env-vars:
> ```shell
>           unset ACCESS_TOKEN; unset OFFER_URI; unset PRE_AUTHORIZED_CODE; \
>           unset CREDENTIAL_ACCESS_TOKEN; unset VERIFIABLE_CREDENTIAL; unset HOLDER_DID; \
>           unset VERIFIABLE_PRESENTATION; unset JWT_HEADER; unset PAYLOAD; unset SIGNATURE; unset JWT; \
>           unset VP_TOKEN; unset DATA_SERVICE_ACCESS_TOKEN;
> ```

Get an AccessToken from Keycloak:

```shell
    export ACCESS_TOKEN=$(curl -s -X POST http://keycloak-consumer.127.0.0.1.nip.io:8080/realms/test-realm/protocol/openid-connect/token \
      --header 'Accept: */*' \
      --header 'Content-Type: application/x-www-form-urlencoded' \
      --data grant_type=password \
      --data client_id=admin-cli \
      --data username=test-user \
      --data password=test | jq '.access_token' -r); echo ${ACCESS_TOKEN}
```

(Optional, since in the local case we know all of the values in advance)
Get the credentials issuer information:

```shell
    curl -X GET http://keycloak-consumer.127.0.0.1.nip.io:8080/realms/test-realm/.well-known/openid-credential-issuer
```

Get a credential offer uri(for the `user-credential), using the retrieved AccessToken:

```shell
    export OFFER_URI=$(curl -s -X GET 'http://keycloak-consumer.127.0.0.1.nip.io:8080/realms/test-realm/protocol/oid4vc/credential-offer-uri?credential_configuration_id=user-credential' \
      --header "Authorization: Bearer ${ACCESS_TOKEN}" | jq '"\(.issuer)\(.nonce)"' -r); echo ${OFFER_URI}
```

Use the offer uri(e.g. the `issuer`and `nonce` fields), to retrieve the actual offer:

```shell
    export PRE_AUTHORIZED_CODE=$(curl -s -X GET ${OFFER_URI} \
            --header "Authorization: Bearer ${ACCESS_TOKEN}" | jq '.grants."urn:ietf:params:oauth:grant-type:pre-authorized_code"."pre-authorized_code"' -r); echo ${PRE_AUTHORIZED_CODE}
```

Exchange the pre-authorized code from the offer with an AccessToken at the authorization server:

```shell
    export CREDENTIAL_ACCESS_TOKEN=$(curl -s -X POST http://keycloak-consumer.127.0.0.1.nip.io:8080/realms/test-realm/protocol/openid-connect/token \
      --header 'Accept: */*' \
      --header 'Content-Type: application/x-www-form-urlencoded' \
      --data grant_type=urn:ietf:params:oauth:grant-type:pre-authorized_code \
      --data code=${PRE_AUTHORIZED_CODE} | jq '.access_token' -r); echo ${CREDENTIAL_ACCESS_TOKEN}
```

Use the returned access token to get the actual credential:

```shell
    export VERIFIABLE_CREDENTIAL=$(curl -s -X POST http://keycloak-consumer.127.0.0.1.nip.io:8080/realms/test-realm/protocol/oid4vc/credential \
      --header 'Accept: */*' \
      --header 'Content-Type: application/json' \
      --header "Authorization: Bearer ${CREDENTIAL_ACCESS_TOKEN}" \
      --data '{"credential_identifier":"user-credential", "format":"jwt_vc"}' | jq '.credential' -r); echo ${VERIFIABLE_CREDENTIAL}
```

You will receive a jwt-encoded credential to be used within the data space.

### The Data Provider

![Provider](../../img/local/provider.jpg)

The Data Provider requires a couple of more components, in order to provide secure access to its data. It needs
essentially 3 building blocks:

* Data-Service: In our example case, the data is provided by
  an [NGSI-LD Context Broker](https://github.com/ScorpioBroker/ScorpioBroker)
* Authentication:
    * [VCVerifier](https://github.com/FIWARE/VCVerifier): Verifies incoming VerifiableCredentials
      through [OID4VP](https://openid.net/specs/openid-4-verifiable-presentations-1_0.html) and returns a JWT token
    * [CredentialsConfigService](https://github.com/FIWARE/credentials-config-service): Allows to configure the Trusted
      Lists to be used for certain credentials
    * [TrustedIssuersList](https://github.com/FIWARE/trusted-issuers-list): Allows to specify the capabilities of
      certain issuers, f.e. what credentials they are allowed to issue
* Authorization:
    * [Apisix](https://apisix.apache.org/): An Api-Gateway that verifies the incoming JWT(provided by the verifier) and
      acts as Policy Enforcement Point to authorize requests based on the PDP's decision
    * [Open Policy Agent](https://www.openpolicyagent.org/): Acts as the Policy Decision Point, to evaluate existing
      policies for requests and either allows or denies them.
    * [ODRL-PAP](https://github.com/wistefan/odrl-pap): Policy Administration & Information Point, that allows to
      configure policies in ODRL and provides them to the Open Policy Agent(translated into rego). It can be used to
      offer additional infromation to be taken into account.
* [did-helper](https://github.com/wistefan/did-helper) - a small helper application, providing the decentralized
  identity to be used for the local Data Space

After the deployment, the provider can create a policy to allow access to its data. An example policy can be found in
the [test-resources](../../../it/src/test/resources/policies/energyReport.json)
It allows every participant to access entities of type ```EnergyReport```.

> :warning: The PAP and Scorpio APIs are only published to make demo interactions easier.
> In real environments, they should never be public without any authentication/authorization framework in front of them.

The policy can be created at the PAP via:

```shell
  curl -s -X 'POST' http://pap-provider.127.0.0.1.nip.io:8080/policy \
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
            "@id": "https://mp-operation.org/policy/common/type",
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
                    "odrl:leftOperand": "ngsi-ld:entityType",
                    "odrl:operator": {
                      "@id": "odrl:eq"
                    },
                    "odrl:rightOperand": "EnergyReport"
                  }
                ]
              },
              "odrl:assignee": {
                "@id": "odrl:any"
              },
              "odrl:action": {
                "@id": "odrl:read"
              }
            }
          }'
```

Data can be created through the NGSI-LD API itself. In order to make interaction easier, its directly available through
an ingress at ```http://scorpio-provider.127.0.0.1.nip.io/ngsi-ld/v1```. In
real environments, no endpoint should be publicly available without beeing protected by the authorization framework.
Create an entity via:

```shell
  curl -s -X POST http://scorpio-provider.127.0.0.1.nip.io:8080/ngsi-ld/v1/entities \
    -H 'Accept: application/json' \
    -H 'Content-Type: application/json' \
    -d '{
      "id": "urn:ngsi-ld:EnergyReport:fms-1",
      "type": "EnergyReport",
      "name": {
        "type": "Property",
        "value": "Standard Server"
      },
      "consumption": {
        "type": "Property",
        "value": "94"
      }
    }'
```

## Demo Interactions

Once everything is deployed and configured(e.g. the consumer received a credential -
see [The Data Consumer](#the-data-consumer) - and policy/entity are setup - see [The Data Provider](#the-data-provider))
,
the consumer can access the data as following:

### Authenticate via OID4VP

> :warning: Those steps assume that interaction with consumer and provider already happend, e.g. a VerifiableCredential
> is available
> and policy/entity are created.

The credential needs to be presented for authentication
through [OID4VP]((https://openid.net/specs/openid-4-verifiable-presentations-1_0.html).
Every required information for that flow can be retrieved via the standard endpoints.

If you try to request the provider api without authentication, you will receive an 401:

```shell
   curl -s -X GET 'http://mp-data-service.127.0.0.1.nip.io:8080/ngsi-ld/v1/entities/urn:ngsi-ld:EnergyReport:fms-1'
```

The normal flow is now to request the oidc-information at the well-known endpoint:

```shell
  export TOKEN_ENDPOINT=$(curl -s -X GET 'http://mp-data-service.127.0.0.1.nip.io:8080/.well-known/openid-configuration' | jq -r '.token_endpoint'); echo $TOKEN_ENDPOINT
```

In the response, the grant type `vp_token` will be present, indicating the support for the OID4VP authentication flow:

```json
{
  "issuer": "http://provider-verifier.127.0.0.1.nip.io:8080",
  "authorization_endpoint": "http://provider-verifier.127.0.0.1.nip.io:8080",
  "token_endpoint": "http://provider-verifier.127.0.0.1.nip.io:8080/services/data-service/token",
  "jwks_uri": "http://provider-verifier.127.0.0.1.nip.io:8080/.well-known/jwks",
  "scopes_supported": [
    "default"
  ],
  "response_types_supported": [
    "token"
  ],
  "response_mode_supported": [
    "direct_post"
  ],
  "grant_types_supported": [
    "authorization_code",
    "vp_token"
  ],
  "subject_types_supported": [
    "public"
  ],
  "id_token_signing_alg_values_supported": [
    "EdDSA",
    "ES256"
  ]
}
```

With that information, the authentication flow at the verifier(e.g.`"http://provider-verifier.127.0.0.1.nip.io:8080`)
can be started.
First, the credential needs to be encoded into a vp_token. If you want to do that manually, first a did and the
corresponding key-material is required.
You can create such via:

```shell
  docker run -v $(pwd):/cert quay.io/wi_stefan/did-helper:0.1.1
```

This will produce the files cert.pem, cert.pfx, private-key.pem, public-key.pem and did.json, containing all required
information for the generated did:key.
Find the did here:

```shell
  export HOLDER_DID=$(cat did.json | jq '.id' -r); echo ${HOLDER_DID}
```

As a next step, a VerifiablePresentation, containing the Credential has to be created:

```shell
  export VERIFIABLE_PRESENTATION="{
    \"@context\": [\"https://www.w3.org/2018/credentials/v1\"],
    \"type\": [\"VerifiablePresentation\"],
    \"verifiableCredential\": [
        \"${VERIFIABLE_CREDENTIAL}\"
    ],
    \"holder\": \"${HOLDER_DID}\"
  }"; echo ${VERIFIABLE_PRESENTATION}
```

Now, the presentation has to be embedded into a signed JWT:

Setup the header:

```shell
  export JWT_HEADER=$(echo -n "{\"alg\":\"ES256\", \"typ\":\"JWT\", \"kid\":\"${HOLDER_DID}\"}"| base64 -w0 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//); echo Header: ${JWT_HEADER}
```

Setup the payload:

```shell
  export PAYLOAD=$(echo -n "{\"iss\": \"${HOLDER_DID}\", \"sub\": \"${HOLDER_DID}\", \"vp\": ${VERIFIABLE_PRESENTATION}}" | base64 -w0 | sed s/\+/-/g |sed 's/\//_/g' |  sed -E s/=+$//); echo Payload: ${PAYLOAD};   
```

Create the signature:

```shell
  export SIGNATURE=$(echo -n "${JWT_HEADER}.${PAYLOAD}" | openssl dgst -sha256 -binary -sign private-key.pem | base64 -w0 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//); echo Signature: ${SIGNATURE}; 
```

Combine them to the JWT:

```shell
  export JWT="${JWT_HEADER}.${PAYLOAD}.${SIGNATURE}"; echo The Token: ${JWT}
```

The JWT representation of the JWT has to be Base64-encoded(no padding!):

```shell
  export VP_TOKEN=$(echo -n ${JWT} | base64 -w0 | sed s/\+/-/g | sed 's/\//_/g' | sed -E s/=+$//); echo ${VP_TOKEN}
```

The vp_token can then be exchanged for the access-token

```shell
    export DATA_SERVICE_ACCESS_TOKEN=$(curl -s -X POST $TOKEN_ENDPOINT \
      --header 'Accept: */*' \
      --header 'Content-Type: application/x-www-form-urlencoded' \
      --data grant_type=vp_token \
      --data vp_token=${VP_TOKEN} \
      --data scope=default | jq '.access_token' -r ); echo ${DATA_SERVICE_ACCESS_TOKEN}
```

With that token, try to access the data again:

```shell
  curl -s -X GET 'http://mp-data-service.127.0.0.1.nip.io:8080/ngsi-ld/v1/entities/urn:ngsi-ld:EnergyReport:fms-1' \
    --header 'Accept: application/json' \
    --header "Authorization: Bearer ${DATA_SERVICE_ACCESS_TOKEN}"
```

## Deployment details

In order to make the setup properly working locally and usable for development and try out, some adaptions have been
made and will be explained here.

### Ingress

To have the local environment as close to reality as possible, all interaction happens
through [Ingress](https://kubernetes.io/docs/concepts/services-networking/ingress/). The ingress is provided via the
[Traefik-IngressController](https://doc.traefik.io/traefik/providers/kubernetes-ingress/) and configured
here: [k3s/infra/traefik]. Additionally, to ensure access to public endpoints happens
equally from inside the cluster and outside of it, [CoreDNS](https://coredns.io/)(deployed on default with k3s) is
instructed to resolve the ingresses(e.g. *.127.0.0.1.nip.io) directly to the loadbalancer-service of Traefik.

Available ingresses:

| URL                                             | Component                                                                  | Participant  | Comment                                                                             | Only for Demo                                        |
|-------------------------------------------------|----------------------------------------------------------------------------|--------------|-------------------------------------------------------------------------------------|------------------------------------------------------|
| http://tir.127.0.0.1.nip.io:8080/               | [Trusted Issuers Registry](https://github.com/FIWARE/trusted-issuers-list) | Trust Anchor | Provides the list of trusted issuers                                                | no                                                   |
| http://til.127.0.0.1.nip.io:8080/               | [Trusted Issuers List API](https://github.com/FIWARE/trusted-issuers-list) | Trust Anchor | Create,Update,Delete functionality for the Trusted Issuers Registry                 | should not be publicly available in real data spaces |
| http://keycloak-consumer.127.0.0.1.nip.io:8080/ | [Keycloak](https://github.com/keycloak/keycloak)                           | Consumer     | Issues credentials on behalf of the consumer                                        | no                                                   |
| http://did-consumer.127.0.0.1.nip.io:8080/      | [did-helper](https://github.com/wistefan/did-helper)                       | Consumer     | Helper to provide access to the consumers did and key material                      | yes, should never be public                          |
| http://mp-data-service.127.0.0.1.nip.io:8080/   | [Apisix](https://apisix.apache.org/)                                       | Provider     | ApiGateway to be used as entry point to all secured services, f.e. the data-service | no                                                   |
| http://provider-verifier.127.0.0.1.nip.io:8080/ | [VCVerifier](https://github.com/FIWARE/VCVerifier)                         | Provider     | Authentication endpoint, used for authenticating through VerifiableCredentials      | no                                                   |
| http://did-provider.127.0.0.1.nip.io:8080/      | [did-helper](https://github.com/wistefan/did-helper)                       | Provider     | Helper to provide access to the providers did and key material                      | yes, should never be public                          |
| http://scorpio-provider.127.0.0.1.nip.io:8080/  | [Scorpio ContextBroker](https://github.com/ScorpioBroker/ScorpioBroker)    | Provider     | Provides direct access to the context broker, to be used for test setup.            | yes, should only be available through the PEP        |
| http://pap-provider.127.0.0.1.nip.io:8080/      | [ODRL-PAP](https://github.com/wistefan/odrl-pap)                           | Provider     | Allows configuration of the access policie, used for authorization                  | yes, should only be available to authorized users    |


### Participant Identity

In a Data Space, every participant requires an identity. The FIWARE Data Space relies on [Decentralized Identifiers](https://www.w3.org/TR/did-core/)
to identify its participants. While the concrete scheme to be used for a Data Space needs to be decided by its requirements, 
the local installation uses [did:key](https://w3c-ccg.github.io/did-method-key/). While the did's are not well readable for humans,
they are well-supported, can be resolved without any external interaction and can easily be generated within the deployment. That makes
them a perfect fit for the local use-case. 
All participants(e.g. the consumer and the participant) get a did generated on installation, by using the [did-helper](https://github.com/wistefan/did-helper). 
The identities and connected key-material is automatically distributed in the cluster and set in the componets that require it.

In real world data spaces, the participants should rather use stabled identities, which can be [did:key](https://w3c-ccg.github.io/did-method-key/), but also 
more organization-focused once like [did:web](https://w3c-ccg.github.io/did-method-web/) or [did:elsi](https://alastria.github.io/did-method-elsi/).

### Deployment

The deployment leverages the [k3s-maven-plugin](https://github.com/kokuwaio/k3s-maven-plugin) to stay as close to the real deployments as possible,
while providing integration with the [integration tests](../../../it/src/test).
In order to build a concrete deployment, [maven](https://maven.apache.org/) executes the following steps, that would also be done by a `normal` deployment process:

1. Copy the required charts([charts/](../../../charts)) to the target folder, e.g. `target/charts`
2. Copy additionally required resources([k3s/infra & k3s/namespaces](../../../k3s/)) to the target folder, e.g. `target/k3s`
3. Execute `helm template` on the charts, with the local values provided for each participant(e.g. [trust-anchor](../../../k3s/trust-anchor.yaml), [provider](../../../k3s/provider.yaml) and [consumer](../../../k3s/consumer.yaml)) and copy the manifests to the target folder(e.g. `target/k3s`)
4. Spin up the cluster
5. Apply the infrastructure resources to the cluster, via `kubectl apply`
6. Apply the charts to the cluster, via `kubectl apply`
