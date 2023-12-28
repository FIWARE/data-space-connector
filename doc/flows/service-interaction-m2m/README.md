# Service Interaction (M2M)

This document gives a detailed description of the steps in the 
Machine-To-Machine (M2M) service interaction flow for both client 
and server/provider side. 

This flow applies to any kind of M2M interaction, e.g., for a service 
consumer's client to authenticate at the service provider or for 
a verifier component to authenticate at the trusted issuers registry.



## Client side


Before interacting with the server/provider, the client application first needs to obtain an 
access token from the `/token` endpoint of the server/provider. 

Given a specific service endpoint `https://<SERVICE_HOST>/<SERVICE_ENDPOINT>` the client wants to interact with, 
a call to this endpoint without 
an Authorization header 
```shell
curl -X GET https://<SERVICE_HOST>/<SERVICE_ENDPOINT>
```
will most probably return an `Unauthorized` error. 

In this case the client application needs to obtain an access (bearer) token at the 
`/token` endpoint of the server/provider. In order to obtain the address of this endpoint, 
the client application needs to call the well-known 
[EBSI OpenID Provider Metadata](https://hub.ebsi.eu/apis/pilot/authorisation/v3/get-well-known-openid-config) endpoint 
in order to retrieve this endpoint. The address can be built with the known host of the server/provider:
```shell
curl -X GET https://<SERVICE_HOST>/.well-known/openid-configuration
```

The response is a JSON object that should contain (at least) the following content:
```json
{
  "grant_types_supported": [
    "vp_token"
  ],
  "token_endpoint": "https://<SERVICE_TOKEN_HOST>/path/to/token",
}
```
One might also need to require specific entries in the field `"scopes_supported"`, but this depends on the actual 
service.  
The actual endpoint to be used can be found in the parameter `"token_endpoint"`.

In order to obtain the access token, the client application needs to send a `vp_token` to the server/provider's 
`/token` endpoint. The `vp_token` is a Signed Verifiable Presentation (VP) which includes the Verifiable 
Credential (VC) of the client/consumer and needs to be prepared by the client. 
Below is an example of the JWT content of such VP.
* Header:
```json
{
  "typ": "JWT",
  "alg": "ES256",
  "kid": "did:ebsi:zdPj1GPXjfERXxXPE1YTYdJ#7j3TpaNdPNTOzOtouOOknlOLQk3JP-ykTfraWtY3GME"
}
```
* Payload:
```json
{
  "iss": "did:ebsi:zdPj1GPXjfERXxXPE1YTYdJ",
  "aud": "https://api-conformance.ebsi.eu/conformance/v3/auth-mock",
  "sub": "did:ebsi:zdPj1GPXjfERXxXPE1YTYdJ",
  "iat": 1589699260,
  "nbf": 1589699260,
  "exp": 1589699260,
  "nonce": "FgkeErf91kfl",
  "jti": "urn:uuid:0706061a-e2ca-4614-9de7-9c1451935f02",
  "vp": {
    "@context": [
      "https://www.w3.org/2018/credentials/v1"
    ],
    "id": "urn:uuid:0706061a-e2ca-4614-9de7-9c1451935f02",
    "type": [
      "VerifiablePresentation"
    ],
    "holder": "did:ebsi:zdPj1GPXjfERXxXPE1YTYdJ",
    "verifiableCredential": [
      "eyJ0eX....aEhOOXcifQ.eyJpj....J9fX0.9Drky3pj....lzTK3_-Q"
    ]
  }	
}
```
where `"verifiableCredential"` holds the actual VC.

The `vp_token` is then send to the [EBSI Token](https://hub.ebsi.eu/apis/pilot/authorisation/v3/post-token) compliant 
endpoint of the server/provider, together with additional parameters:
```shell
curl -X POST 'https://<SERVICE_TOKEN_HOST>/path/to/token' \
-H 'Content-Type: application/x-www-form-urlencoded' \
-H 'Accept: application/json' \
-d "vp_token=<SIGNED_VP_TOKEN>&grant_type=vp_token&presentation_submission=<PRESENTATION_SUBMISSION>&scope=<SCOPE>"
```
The parameter `grant_type` must be equal to `vp_token` and the parameter `presentation_submission` can be left empty at the 
moment.  
Depending on the actual service to be accessed one might need to request the token for a specific scope provided 
with the parameter `scope`. This would have been also listed in the Metadata of the service above.

As a response the client receives a JSON object containing the access token:
```json
{
  "access_token": "<ACCESS_TOKEN>",
  "token_type": "Bearer",
  "expires_in": 7200,
  "scope": "openid did_write",
  "id_token": "eyJ0eXAiOiJ....zI1NksifQ.eyJpYXQiOjE2N....VRURhZyJ9.YIdjUCinbG...Qyv14A"
}
```
The access token can be a simple string or a JWT containing information required by the service provider, e.g., containing the 
actual VC of the consumer. 

The call to the service endpoint can now be retried with the access token being added to the Authorization header:
```shell
curl -X GET https://<SERVICE_HOST>/<SERVICE_ENDPOINT> \
-H 'Authorization: Bearer <ACCESS_TOKEN>'
```








## Server side

On the server side, service providers need to implement 
the [EBSI OpenID Provider Metadata endpoint](https://hub.ebsi.eu/apis/pilot/authorisation/v3/get-well-known-openid-config). 
This is needed, so that consumers can obtain the actual `/token` endpoint needed to retrieve an access token before accessing 
the service.  
Given a specific endpoint of the actual service provided, e.g., 
`https://<SERVICE_HOST>/<SERVICE_ENDPOINT>`, the well-known metadata endpoint should be accessible 
under `https://<SERVICE_HOST>/.well-known/openid-configuration`.  
It should return a JSON object that should contain (at least) the following content:
```json
{
  "grant_types_supported": [
    "vp_token"
  ],
  "token_endpoint": "https://<SERVICE_TOKEN_HOST>/path/to/token",
}
```
One might also need to require specific entries in the field `"scopes_supported"`, but this depends on the actual 
service.  
When using the [VCVerifier component](https://github.com/FIWARE/VCVerifier), this already implements 
such [endpoint](https://github.com/FIWARE/VCVerifier/blob/4318d2afb9ef15f6feb2134557f2fa68d86d7253/api/api.yaml#L139), providing the necessary data for the 
service endpoints configured in the [Credentials Config Service component](https://github.com/FIWARE/credentials-config-service), 
and just needs a corresponding routing for the actual service host. The VCVerifier is providing the service-specific 
OpenID configuration under following endpoint: `https://<VERIFIER_HOST>/services/<SERVICE_IDENTIFIER>/.well-known/openid-configuration`.


* When receiving vp_token:
  - check 1
  - check 2
  - check 3
  
