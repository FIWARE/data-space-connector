# Multi-dataspace operation

Since chart 11.0.0 the DCP controlplane runs [fdsc-edc](https://github.com/SEAMWARE/fdsc-edc) 2.x.
One connector can then take part in several dataspaces, each through its own DSP profiles and with
its own identity if needed. This guide shows how to configure that with the DSC chart. The full key
reference is upstream in
[`docs/multi-dataspace.md`](https://github.com/SEAMWARE/fdsc-edc/blob/2.1.0/docs/multi-dataspace.md).
The OID4VC controlplane is not affected by any of this.

<!-- TOC -->
* [Multi-dataspace operation](#multi-dataspace-operation)
  * [Participant contexts and profiles](#participant-contexts-and-profiles)
  * [Configuring them in the DSC chart](#configuring-them-in-the-dsc-chart)
  * [One connector, two profiles](#one-connector-two-profiles)
  * [Two dataspaces, two DIDs](#two-dataspaces-two-dids)
  * [Management API](#management-api)
  * [DSP addresses for counterparties](#dsp-addresses-for-counterparties)
  * [Verification](#verification)
  * [Migrating from 10.x](#migrating-from-10x)
<!-- TOC -->

## Participant contexts and profiles

- A **participant context** is one identity inside the connector: an id, a DID, an STS client to
  get DCP tokens with, and the profiles it is reachable through. Its assets, policies, negotiations
  and transfers are its own. The DSC uses the DID as context id. The IdentityHub bootstrap already
  stores the STS client secret under `<DID>-sts-client-secret`, which is the upstream convention
  `<context id>-sts-client-secret`, so the existing identity, STS client and vault alias are reused
  unchanged. A context id may contain `:` but not `@`.
- A **profile** is one way into the connector: a name, a DSP version and binding, its JSON-LD
  contexts, and optionally its own DCP scopes and trusted issuers list (TIL). A dataspace is
  usually one profile. One context can be attached to several profiles.

There is no legacy mode. A DCP controlplane without a `fdsc.participants.*` entry starts, but every
DSP route answers 404.

## Configuring them in the DSC chart

The fdsc-edc subchart has no value of its own for any of the new keys. They all go into
`fdsc-edc.deployment.dcp.config.additionalProperties`, which is copied into
`dataspaceconnector-configuration.properties` (why: [ADR 0009](./adr/0009-fdsc-edc-multi-dataspace-via-additional-properties.md)).
Three things to watch:

- **Indentation.** The string is rendered without `indent`, so every continuation line must carry
  exactly 4 leading spaces of its own. In a double-quoted YAML scalar that is `\n    \` at the end
  of each line, as in the examples below.
- **Replace, not merge.** A deployment's config is `common` overwritten by the deployment, so a
  per-deployment `additionalProperties` replaces the one in `fdsc-edc.common`. Repeat the common
  entries, i.e. the `edc.http.client.timeout.*` lines.
- **Callback.** `config.edc.dspCallback` must contain `%s`, which fdsc-edc replaces with the context
  id. The chart renders it verbatim.

Keys per context, `fdsc.participants.<key>.*`:

| Key | Value |
|---|---|
| `id`, `identity` | The DID, in both. |
| `profiles` | Comma-separated profile names. |
| `config.edc.iam.sts.oauth.token.url` | The IdentityHub STS token endpoint. |
| `config.edc.iam.sts.oauth.client.id` | The DID. |
| `config.edc.iam.sts.oauth.client.secret.alias` | `<DID>-sts-client-secret`. |

Startup fails if a context's `config` sets `edc.participant.id`, `edc.participant.did` or
`edc.iam.issuer.id` to anything but its `identity`.

Keys per profile: `edc.dataspace.profiles.<key>.{name,protocol.version,protocol.binding,protocol.namespace,jsonld.context.urls}`.
Once one profile is declared, the built-in `http-dsp-profile-2025-1` disappears, so declare it too
if you keep it. A profile can override the DCP scopes with
`dcp.profiles.<profile name>.scopes.{catalog,negotiation,transfer}` and the TIL with
`ebsiTir.profiles.<profile name>.tilAddress`. Never configure the same scope through both
`dcp.*` and `edc.iam.dcp.scopes.*`: the two are unioned.

Connector-wide keys:

- `tmfExtension.defaultParticipantContextId`: the context that owns TMForum offerings that name
  none.
- `tmfExtension.schemaBaseUri`: a copy of the fdsc-edc `schemas/` that the TMForum API can reach,
  with a trailing `/`.
- `edc.iam.oauth2.issuer` and `edc.iam.oauth2.jwks.url`, see [Management API](#management-api).

## One connector, two profiles

The provider of the local `dsp` scenario serves one context on two profiles:
`http-dsp-profile-2025-1` and `dsp-fiware-ds2`. The second stands for another dataspace with its own
DCP scopes and TIL. They equal the defaults here, because the local setup has a single trust
anchor. This is [`k3s/dsp-provider.yaml`](../k3s/dsp-provider.yaml), which CI deploys:

```yaml
fdsc-edc:
  enabled: true
  deployment:
    dcp:
      deployment:
        image:
          repository: quay.io/seamware/fdsc-edc-controlplane-dcp
      service: {}
      config:
        edc:
          hostname: dcp-mp-operations.127.0.0.1.nip.io
          dspCallback: https://dcp-mp-operations.127.0.0.1.nip.io:443/api/dsp/%s
        additionalProperties: "edc.http.client.timeout.connect=60\n    \
          edc.http.client.timeout.read=120\n    \
          edc.dataspace.profiles.default.name=http-dsp-profile-2025-1\n    \
          edc.dataspace.profiles.default.protocol.version=2025-1\n    \
          edc.dataspace.profiles.default.protocol.binding=HTTPS\n    \
          edc.dataspace.profiles.default.protocol.namespace=https://w3id.org/dspace/2025/1/\n    \
          edc.dataspace.profiles.default.jsonld.context.urls=https://w3id.org/dspace/2025/1/context.jsonld,https://w3id.org/edc/dspace/v0.0.1\n    \
          edc.dataspace.profiles.ds2.name=dsp-fiware-ds2\n    \
          edc.dataspace.profiles.ds2.protocol.version=2025-1\n    \
          edc.dataspace.profiles.ds2.protocol.binding=HTTPS\n    \
          edc.dataspace.profiles.ds2.protocol.namespace=https://w3id.org/dspace/2025/1/\n    \
          edc.dataspace.profiles.ds2.jsonld.context.urls=https://w3id.org/dspace/2025/1/context.jsonld,https://w3id.org/edc/dspace/v0.0.1\n    \
          dcp.profiles.dsp-fiware-ds2.scopes.catalog=org.eclipse.tractusx.vc.type:MembershipCredential:read\n    \
          dcp.profiles.dsp-fiware-ds2.scopes.negotiation=org.eclipse.tractusx.vc.type:MembershipCredential:read\n    \
          dcp.profiles.dsp-fiware-ds2.scopes.transfer=org.eclipse.tractusx.vc.type:MembershipCredential:read\n    \
          ebsiTir.profiles.dsp-fiware-ds2.tilAddress=http://trusted-issuers-list.provider.svc.cluster.local:8080\n    \
          fdsc.participants.self.id=did:web:mp-operations.org\n    \
          fdsc.participants.self.identity=did:web:mp-operations.org\n    \
          fdsc.participants.self.profiles=http-dsp-profile-2025-1,dsp-fiware-ds2\n    \
          fdsc.participants.self.config.edc.iam.sts.oauth.token.url=http://identityhub-service.provider.svc.cluster.local:8087/api/sts/token\n    \
          fdsc.participants.self.config.edc.iam.sts.oauth.client.id=did:web:mp-operations.org\n    \
          fdsc.participants.self.config.edc.iam.sts.oauth.client.secret.alias=did:web:mp-operations.org-sts-client-secret\n    \
          tmfExtension.defaultParticipantContextId=did:web:mp-operations.org\n    \
          tmfExtension.schemaBaseUri=https://raw.githubusercontent.com/SEAMWARE/fdsc-edc/2.1.0/schemas/\n    \
          edc.iam.oauth2.issuer=https://keycloak-provider.127.0.0.1.nip.io/realms/test-realm\n    \
          edc.iam.oauth2.jwks.url=http://provider-keycloak.provider.svc.cluster.local:8080/realms/test-realm/protocol/openid-connect/certs"
```

The consumer, [`k3s/dsp-consumer.yaml`](../k3s/dsp-consumer.yaml), is attached to the same two
profiles. `it/src/test/resources/it/dsp_multi_profile.feature` reads the catalog through each of
them.

## Two dataspaces, two DIDs

When the second dataspace expects a different identity, add a second context with its own DID. The
values below are rendered by the chart's helm-unittest suite
[`fdsc-edc-multi-dataspace_test.yaml`](../charts/data-space-connector/tests/fdsc-edc-multi-dataspace_test.yaml).
Context `a` takes part in dataspace A through the default profile; context `b` takes part in
dataspace B through `dsp-dataspace-b`, which requires its own membership credential and trusts its
own TIL:

```yaml
fdsc-edc:
  enabled: true
  deployment:
    dcp:
      deployment:
        image:
          repository: quay.io/seamware/fdsc-edc-controlplane-dcp
      config:
        edc:
          dspCallback: https://dcp.example.org/api/dsp/%s
        additionalProperties: "edc.http.client.timeout.connect=60\n    \
          edc.http.client.timeout.read=120\n    \
          edc.dataspace.profiles.default.name=http-dsp-profile-2025-1\n    \
          edc.dataspace.profiles.default.protocol.version=2025-1\n    \
          edc.dataspace.profiles.default.protocol.binding=HTTPS\n    \
          edc.dataspace.profiles.default.protocol.namespace=https://w3id.org/dspace/2025/1/\n    \
          edc.dataspace.profiles.default.jsonld.context.urls=https://w3id.org/dspace/2025/1/context.jsonld,https://w3id.org/edc/dspace/v0.0.1\n    \
          edc.dataspace.profiles.b.name=dsp-dataspace-b\n    \
          edc.dataspace.profiles.b.protocol.version=2025-1\n    \
          edc.dataspace.profiles.b.protocol.binding=HTTPS\n    \
          edc.dataspace.profiles.b.protocol.namespace=https://w3id.org/dspace/2025/1/\n    \
          edc.dataspace.profiles.b.jsonld.context.urls=https://w3id.org/dspace/2025/1/context.jsonld,https://w3id.org/edc/dspace/v0.0.1\n    \
          dcp.profiles.dsp-dataspace-b.scopes.catalog=org.eclipse.tractusx.vc.type:DataspaceBMembership:read\n    \
          dcp.profiles.dsp-dataspace-b.scopes.negotiation=org.eclipse.tractusx.vc.type:DataspaceBMembership:read\n    \
          dcp.profiles.dsp-dataspace-b.scopes.transfer=org.eclipse.tractusx.vc.type:DataspaceBMembership:read\n    \
          ebsiTir.profiles.dsp-dataspace-b.tilAddress=https://til.dataspace-b.example.org\n    \
          fdsc.participants.a.id=did:web:dataspace-a.example.org\n    \
          fdsc.participants.a.identity=did:web:dataspace-a.example.org\n    \
          fdsc.participants.a.profiles=http-dsp-profile-2025-1\n    \
          fdsc.participants.a.config.edc.iam.sts.oauth.token.url=http://identityhub-service:8087/api/sts/token\n    \
          fdsc.participants.a.config.edc.iam.sts.oauth.client.id=did:web:dataspace-a.example.org\n    \
          fdsc.participants.a.config.edc.iam.sts.oauth.client.secret.alias=did:web:dataspace-a.example.org-sts-client-secret\n    \
          fdsc.participants.b.id=did:web:dataspace-b.example.org\n    \
          fdsc.participants.b.identity=did:web:dataspace-b.example.org\n    \
          fdsc.participants.b.profiles=dsp-dataspace-b\n    \
          fdsc.participants.b.config.edc.iam.sts.oauth.token.url=http://identityhub-service:8087/api/sts/token\n    \
          fdsc.participants.b.config.edc.iam.sts.oauth.client.id=did:web:dataspace-b.example.org\n    \
          fdsc.participants.b.config.edc.iam.sts.oauth.client.secret.alias=did:web:dataspace-b.example.org-sts-client-secret\n    \
          tmfExtension.defaultParticipantContextId=did:web:dataspace-a.example.org\n    \
          tmfExtension.schemaBaseUri=https://raw.githubusercontent.com/SEAMWARE/fdsc-edc/2.1.0/schemas/\n    \
          edc.iam.oauth2.issuer=https://keycloak.example.org/realms/dsc\n    \
          edc.iam.oauth2.jwks.url=http://keycloak.dsc.svc.cluster.local:8080/realms/dsc/protocol/openid-connect/certs"
```

The controlplane side is complete with that. The rest of the second identity is **not** provisioned
by the chart: `identityhub.bootstrap` registers exactly one participant, the connector's own DID.
For the second DID:

1. **DID document.** Publish `did:web:dataspace-b.example.org`, with the IdentityHub's
   CredentialService endpoint for that DID and the public key of its signing key.
2. **IdentityHub participant context and STS secret.** Register the DID in the IdentityHub through
   the Identity API, `POST <identityhub>/api/identity/v1beta/participants`, and write its STS client
   secret to the vault alias `did:web:dataspace-b.example.org-sts-client-secret`. The chart's own
   bootstrap script does both, idempotently, and runs from a workstation as well as from the Job. Run
   it once more with the second DID's settings, as in the manual procedure in
   [DSP integration](./DSP_INTEGRATION.md#setup-the-consumer) (`derive-key.sh`, then `register.sh`
   with `DID`, `KEY_ALIAS`, `KEY_ID`, `STS_CLIENT_SECRET` and the identity key of the second DID).
   The script also writes the alias with every `:` replaced by `%3A`, the key EDC's vault client
   actually resolves.
3. **Membership credential.** Have the issuer of dataspace B issue its membership credential
   (`DataspaceBMembership` in the example) to the second DID, and store it in the IdentityHub under
   that participant, `POST .../api/identity/v1beta/participants/<URL-encoded DID>/credentials`, like
   the credential insertion in [DSP integration](./DSP_INTEGRATION.md).
4. **Trust.** The second dataspace's TIL, set per profile with `ebsiTir.profiles.<profile>.tilAddress`,
   must list that issuer. The counterparties in dataspace B must trust it as well.
5. **TMForum offerings.** Untagged offerings belong to `tmfExtension.defaultParticipantContextId`
   only, so adding a context never exposes existing offers in it. Tag an offering with the contexts
   it is made in through `participantContextIds`, e.g.
   `"participantContextIds": ["did:web:dataspace-a.example.org", "did:web:dataspace-b.example.org"]`,
   or `["*"]` for every context. Upstream
   [Offering a product in one, several or all dataspaces](https://github.com/SEAMWARE/fdsc-edc/blob/2.1.0/docs/multi-dataspace.md#offering-a-product-in-one-several-or-all-dataspaces)
   has the details.

## Management API

The management API v3 and v4 are gone. Every call goes to one context:

```
<management path>/v5beta/participants/<context id>/{catalog/request,contractnegotiations,transferprocesses,edrs/request,assets/request,...}
```

and carries an OAuth2 bearer token. The token is checked against `edc.iam.oauth2.issuer` and
`edc.iam.oauth2.jwks.url`:

- Always set the issuer, and set it to the exact `iss` the identity provider puts in its tokens.
  Keycloak derives that from the host the token is requested from, so use the external URL. If the
  issuer is unset, only tokens without `iss` are accepted.
- Use a `*.cluster.local` host in `jwks.url`. Other hosts go through the egress proxy if one is
  configured, because the `JAVA_TOOL_OPTIONS` `nonProxyHosts` only matches `*.cluster.local`.
- The `scope` claim must contain `management-api:read` and/or `management-api:write`.
  `management-api:admin` additionally grants every context. Without it, `sub` must equal the context
  id.
- `protocol` (alias `profile`) in a request body is a profile name attached to the calling context,
  e.g. `http-dsp-profile-2025-1`, not `dataspace-protocol-http:2025-1`.

The local consumer has an admin client, from [`k3s/consumer.yaml`](../k3s/consumer.yaml):

```yaml
keycloak:
  realm:
    clientScopes:
      - name: management-api:read
        protocol: openid-connect
        attributes:
          include.in.token.scope: "true"
      - name: management-api:write
        protocol: openid-connect
        attributes:
          include.in.token.scope: "true"
      - name: management-api:admin
        protocol: openid-connect
        attributes:
          include.in.token.scope: "true"
    clients:
      - clientId: edc-management
        enabled: true
        description: Client calling the fdsc-edc management API
        clientAuthenticatorType: client-secret
        secret: edc-management-client-secret
        redirectUris: []
        webOrigins: []
        bearerOnly: false
        consentRequired: false
        standardFlowEnabled: false
        implicitFlowEnabled: false
        directAccessGrantsEnabled: false
        serviceAccountsEnabled: true
        publicClient: false
        protocol: openid-connect
        fullScopeAllowed: true
        defaultClientScopes: ["management-api:read", "management-api:write", "management-api:admin"]
        optionalClientScopes: []
```

`keycloak.realm.clients` is a list, and Helm replaces lists across values files. Keep the other
clients of the realm in the same list.

To give an operator of one dataspace access to its context only, leave out the `admin` scope and
make `sub` the context id. Keycloak puts the service account's user id into `sub`; a hardcoded-claim
mapper replaces it. From the chart's
[`keycloak-realm_test.yaml`](../charts/data-space-connector/tests/keycloak-realm_test.yaml):

```yaml
keycloak:
  realm:
    clients:
      - clientId: edc-management-dataspace-b
        enabled: true
        clientAuthenticatorType: client-secret
        secret: change-me
        standardFlowEnabled: false
        directAccessGrantsEnabled: false
        serviceAccountsEnabled: true
        publicClient: false
        protocol: openid-connect
        defaultClientScopes: ["management-api:read", "management-api:write"]
        optionalClientScopes: []
        protocolMappers:
          - name: sub-participant-context
            protocol: openid-connect
            protocolMapper: oidc-hardcoded-claim-mapper
            config:
              claim.name: sub
              claim.value: did:web:dataspace-b.example.org
              jsonType.label: String
              access.token.claim: "true"
              id.token.claim: "false"
              userinfo.token.claim: "false"
              introspection.token.claim: "true"
```

Checked against Keycloak 26.7.0: a client-credentials token from this client carries the DID as
`sub`, also with `basic` (whose `oidc-sub-mapper` normally sets `sub`) added to its default scopes.

Keycloak does not re-import an existing realm: add the client in the admin console or recreate the
realm. A token from this client and a catalog request of the context then look like this:

```shell
export MANAGEMENT_TOKEN=$(curl -s -X POST https://keycloak.example.org/realms/dsc/protocol/openid-connect/token \
  --data grant_type=client_credentials \
  --data client_id=edc-management-dataspace-b \
  --data client_secret=change-me | jq -r .access_token)

curl -s -X POST \
  'https://dcp-management.example.org/api/v1/management/v5beta/participants/did:web:dataspace-b.example.org/catalog/request' \
  --header "Authorization: Bearer ${MANAGEMENT_TOKEN}" \
  --header 'Content-Type: application/json' \
  --data-raw '{
    "@context": ["https://w3id.org/edc/connector/management/v2"],
    "@type": "CatalogRequest",
    "protocol": "dsp-dataspace-b",
    "counterPartyId": "did:web:provider-b.example.org",
    "counterPartyAddress": "https://dcp.provider-b.example.org/api/dsp/did:web:provider-b.example.org/dsp-dataspace-b",
    "querySpec": {"@type": "QuerySpec"}
  }' | jq .
```

The complete consumer flow (catalog, negotiation, transfer, EDR) against the v5beta API is in the
[DCP walkthrough](./DSP_INTEGRATION.md#dcp-1) and in [`dsp-consume.sh`](./scripts/dsp-consume.sh).

## DSP addresses for counterparties

A counterparty addresses one context and one profile:

```
https://<dcp host>/api/dsp/<context id>/<profile>
```

e.g. `https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/did:web:mp-operations.org/http-dsp-profile-2025-1`.
That is the `counterPartyAddress` in management requests and the endpoint to publish in TMForum
product specifications. The DSP messages hang below it (`/catalog/request`, `/negotiations/...`,
`/transfers/...`). Other paths:

| Path | Serves |
|---|---|
| `/api/dsp/<context id>/.well-known/dspace-version` | The context's profiles and versions. No token required. |
| `/.well-known/openid-configuration/<base64url(context id), no padding>` | OpenID discovery of one context, used to verify EDR tokens. The bare path serves the default context. |

A gateway in front of the controlplane that exempts the version metadata from authentication must
exempt the per-context path above.

## Verification

Against the local `dsp` scenario, through its proxy:

```shell
# profiles of the provider context: lists http-dsp-profile-2025-1 and dsp-fiware-ds2, and nothing else
curl -s -k -x localhost:8888 \
  'https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/did:web:mp-operations.org/.well-known/dspace-version' | jq .

# routing comes before authentication: an attached profile without a DCP token answers 401 ...
curl -s -k -x localhost:8888 -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -d '{}' \
  'https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/did:web:mp-operations.org/dsp-fiware-ds2/catalog/request'
# ... and a profile that is not attached answers 404
curl -s -k -x localhost:8888 -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -d '{}' \
  'https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/did:web:mp-operations.org/dsp-unknown/catalog/request'
```

The catalog per profile is read through the consumer's management API: run step 1 of the
[DCP walkthrough](./DSP_INTEGRATION.md#dcp-1) once with `http-dsp-profile-2025-1` and once with
`dsp-fiware-ds2`, as `protocol` and as the last segment of `counterPartyAddress`. Both return the
provider's offers. `dsp_multi_profile.feature` checks the same in CI.

## Migrating from 10.x

The checklist is in the [11.0.0 release notes](./release-notes/11-x.md#warning-required-action-dcp-deployments).
In short: declare a context per DID, add `%s` to the callback, set the two `tmfExtension.*` keys and
the OAuth2 issuer, create a management client and move every management call to v5beta, and hand
counterparties the new DSP address.
