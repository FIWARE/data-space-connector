# Multi-dataspace operation

Since chart 10.23.0 the DCP controlplane runs [fdsc-edc](https://github.com/SEAMWARE/fdsc-edc) 2.x.
One connector can then take part in several dataspaces, each through its own DSP profiles and with
its own identity if needed. This guide describes what that is for, how to see it work in the local
demo, and how to configure it with the DSC chart. The full key reference is upstream in
[`docs/multi-dataspace.md`](https://github.com/SEAMWARE/fdsc-edc/blob/2.1.1/docs/multi-dataspace.md).
The OID4VC controlplane is not affected by any of this.

<!-- TOC -->
* [Multi-dataspace operation](#multi-dataspace-operation)
  * [Participant contexts and profiles](#participant-contexts-and-profiles)
  * [Use cases](#use-cases)
  * [Demo flows](#demo-flows)
    * [Starting the scenario](#starting-the-scenario)
    * [Prerequisites for the manual flows](#prerequisites-for-the-manual-flows)
    * [Flow 1 - which dataspaces does a context serve?](#flow-1---which-dataspaces-does-a-context-serve)
    * [Flow 2 - routing happens before authentication](#flow-2---routing-happens-before-authentication)
    * [Flow 3 - the same catalog through two dataspaces](#flow-3---the-same-catalog-through-two-dataspaces)
    * [Flow 4 - a full consumer flow through the second dataspace](#flow-4---a-full-consumer-flow-through-the-second-dataspace)
    * [Flow 5 - a second identity, by hand](#flow-5---a-second-identity-by-hand)
  * [Configuring contexts and profiles in the DSC chart](#configuring-contexts-and-profiles-in-the-dsc-chart)
  * [One connector, two profiles](#one-connector-two-profiles)
  * [Two dataspaces, two DIDs](#two-dataspaces-two-dids)
  * [Management API](#management-api)
  * [DSP addresses for counterparties](#dsp-addresses-for-counterparties)
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

## Use cases

Three things the contexts and profiles of fdsc-edc 2.x make possible, from the most common to the
rarest:

- **One identity in several dataspaces.** The connector keeps its DID, its identityhub participant,
  its credentials and its assets, and becomes reachable through one profile per dataspace. Each
  profile brings its own DCP scopes - the credentials the connector asks its STS for and presents to
  that dataspace - and its own trusted issuers list, so the dataspaces stay separate in everything
  that concerns trust, while the catalog, negotiations and transfers behind them are the one
  context's. This is the common case: joining a second dataspace is then a profile, not a second
  deployment. The local `dsp` scenario does exactly this, and the demo below walks through it.
- **An identity per dataspace.** When a dataspace insists on its own identity - a DID it issued, or
  one whose credentials must not be presentable elsewhere - the connector runs a second participant
  context. Contexts share nothing: each has its own DID, STS client, credentials, assets, policies,
  negotiations and transfers, each is addressed separately on the management API, and a TMForum
  offering states which contexts it is offered in. One deployment still, but the second identity has
  to be provisioned by hand; the chart bootstraps exactly one.
- **Two protocol versions side by side.** A profile carries its own DSP version, binding, namespace
  and JSON-LD contexts, so the same context can serve one dataspace through two profiles while its
  counterparties migrate from one DSP version to the next, and drop the old profile when they are
  done. Every example in this repository uses 2025-1 on both profiles.

In all three, counterparties and management clients notice the same thing: a DSP address names a
context and a profile, and a management call names a context. [DSP addresses for
counterparties](#dsp-addresses-for-counterparties) and [Management API](#management-api) have the
shapes.

## Demo flows

Everything below runs against the local `dsp` scenario, the one scenario in this repository whose
connectors are attached to more than one profile: both sides serve the same context on the profiles
`http-dsp-profile-2025-1` and `dsp-fiware-ds2`, the second standing for another dataspace with its
own DCP scopes and trusted issuers list. The values that produce this are in [Configuring contexts
and profiles in the DSC chart](#configuring-contexts-and-profiles-in-the-dsc-chart) and the sections
after it; nothing there needs to be read first to run the flows.

### Starting the scenario

```shell
mvn clean deploy -Plocal,dsp
```

This is all the preparation the flows need. It brings up the trust anchor, the provider and the
consumer, each side with one DCP participant context - `did:web:mp-operations.org` and
`did:web:fancy-marketplace.biz` - attached to the two profiles `http-dsp-profile-2025-1` and
`dsp-fiware-ds2`, and it provisions both identities: `identityhub.bootstrap` registers the
participant with its key and STS client secret, `identityhub.credentialSync` mirrors the membership
credential the vc-operator requests from Keycloak into it
([`k3s/dsp-provider.yaml`](../k3s/dsp-provider.yaml),
[`k3s/dsp-consumer.yaml`](../k3s/dsp-consumer.yaml)). Nothing has to be registered by hand; the
manual procedure in [DSP integration](./DSP_INTEGRATION.md#provisioning-the-participant-identities)
runs the same scripts and is there to follow along with, or to debug one of them.

Wait for the two bootstrap Jobs before starting:

```shell
kubectl wait --for=condition=complete job/identityhub-bootstrap -n provider --timeout=5m
kubectl wait --for=condition=complete job/identityhub-bootstrap -n consumer --timeout=5m
```

Vault runs in dev mode here, so its storage is in memory: after a vault restart the signing key,
the super-user credential and the STS client secret are gone until the next deploy re-runs the Job,
which is why it is a `post-upgrade` hook as well.

What the deploy does *not* create is the thing being traded. Flows 3 and 4 need an offering in the
provider's catalog: follow [prepare some data](./DSP_INTEGRATION.md#prepare-some-data) and
[prepare the offering](./DSP_INTEGRATION.md#prepare-the-offering) once, or let the tests do it -

```shell
mvn clean integration-test -Ptest,dsp,dsp-test
```

runs every `@dsp` feature, among them
[`dsp_multi_profile.feature`](../it/src/test/resources/it/dsp_multi_profile.feature) (flows 1 to 3
in executable form), and leaves the offering behind when it finishes.

### Prerequisites for the manual flows

The local hosts resolve through the squid proxy of the `infra` namespace and serve self-signed
certificates, so every command below carries `-k -x localhost:8888`. Drop the `-k` once the cluster
CA is exported:

```shell
kubectl get secret ca-secret -n operators -o jsonpath='{.data.tls\.crt}' | base64 -d > local-ca.crt
export CURL_CA_BUNDLE=./local-ca.crt
```

Flows 1 and 2 need nothing else - they are answered before any token is checked. Flows 3 and 4 need
the offering from [Starting the scenario](#starting-the-scenario).

### Flow 1 - which dataspaces does a context serve?

The version metadata of a context lists one entry per attached profile and needs no token:

```shell
curl -s -k -x localhost:8888 \
  'https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/did:web:mp-operations.org/.well-known/dspace-version' \
  | jq -r '.protocolVersions[].profile'
```

```
http-dsp-profile-2025-1
dsp-fiware-ds2
```

This is the discovery step a counterparty performs: it names the dataspaces this context is
reachable in, and nothing else. It is the attachment that decides, `fdsc.participants.<key>.profiles`
in [One connector, two profiles](#one-connector-two-profiles): a profile that is declared but not
attached to the context does not appear here.

### Flow 2 - routing happens before authentication

fdsc-edc resolves context and profile from the path first, and only then checks the DCP token. The
two answers are therefore distinguishable without any credential: an attached profile rejects the
request as unauthenticated, an unknown one is not routed at all. Send the request without a body -
a request body is expanded as JSON-LD before the token is looked at, so `-d '{}'` answers
`400 Failed to expand JsonObject` instead:

```shell
for PROFILE in http-dsp-profile-2025-1 dsp-fiware-ds2 dsp-unknown; do
  printf '%-24s %s\n' "${PROFILE}" "$(curl -s -k -x localhost:8888 -o /dev/null -w '%{http_code}' \
    -X POST \
    "https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/did:web:mp-operations.org/${PROFILE}/catalog/request")"
done
```

```
http-dsp-profile-2025-1  401
dsp-fiware-ds2           401
dsp-unknown              404
```

The 401 carries a DSP `CatalogError` body, which is itself the proof that the request reached the
profile's catalog handler; the 404 is the router's, and has no DSP body. For telling the two apart
that is all that matters - a 400 over a malformed body means "routed" just as well as the 401 does.

The same holds for the context id: a DID without a `fdsc.participants.*` entry answers 404 on every
DSP route, with any body. That is the quickest check after a configuration change - 404 where you
expect 401 means the context or the profile did not come up, not that the credentials are wrong.

### Flow 3 - the same catalog through two dataspaces

Every management call addresses one context and carries an OAuth2 token ([Management
API](#management-api)); the consumer's Keycloak has the client-credentials client `edc-management`
for that. The token expires after a few minutes,
so repeat this when calls start returning 401:

```shell
export MANAGEMENT_API=https://dsp-dcp-management.127.0.0.1.nip.io/api/v1/management/v5beta/participants/did:web:fancy-marketplace.biz
export MANAGEMENT_TOKEN=$(curl -s -k -x localhost:8888 -X POST \
  'https://keycloak-consumer.127.0.0.1.nip.io/realms/test-realm/protocol/openid-connect/token' \
  --data grant_type=client_credentials \
  --data client_id=edc-management \
  --data client_secret=edc-management-client-secret | jq -r .access_token)
```

Now read the provider catalog once per profile. `protocol` is a profile of the *calling* context,
the last segment of `counterPartyAddress` is the provider's profile for the same dataspace, and the
two have to match:

```shell
for PROFILE in http-dsp-profile-2025-1 dsp-fiware-ds2; do
  echo "== ${PROFILE}"
  curl -s -k -x localhost:8888 -X POST "${MANAGEMENT_API}/catalog/request" \
    --header "Authorization: Bearer ${MANAGEMENT_TOKEN}" \
    --header 'Content-Type: application/json' \
    --data-raw "{
      \"@context\": [\"https://w3id.org/edc/connector/management/v2\"],
      \"@type\": \"CatalogRequest\",
      \"protocol\": \"${PROFILE}\",
      \"counterPartyId\": \"did:web:mp-operations.org\",
      \"counterPartyAddress\": \"https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/did:web:mp-operations.org/${PROFILE}\",
      \"querySpec\": {\"@type\": \"QuerySpec\"}
    }" | jq -r 'if type == "array" then "request failed: " + (.[0].message // tostring)
                else (.["dcat:dataset"] // .dataset // []) | (if type == "array" then . else [.] end) | .[]["@id"]
                end'
done
```

A management error comes back as a JSON array rather than a catalog, which is what the `if type ==
"array"` branch prints. The common one is

```
request failed: Unable to obtain credentials: Failed to fetch client secret from the vault with alias: did:web:fancy-marketplace.biz-sts-client-secret
```

and it means the participant was never provisioned, or Vault restarted and lost what the bootstrap
wrote: the context asks its STS for a DCP token before it ever contacts the provider. Check that
`job/identityhub-bootstrap` completed in that namespace, and re-run the deploy if Vault restarted.
The alias in the message is the one the context's `edc.iam.sts.oauth.client.secret.alias` names, so
a *different* DID in it means the context configuration and the provisioned identity disagree.

Both profiles return the offering prepared above, because the local setup has one trust anchor and
one context that owns the offering. What differs is how the catalog was obtained: for
`dsp-fiware-ds2` the consumer asked its STS for a DCP token with that profile's scopes
(`dcp.profiles.dsp-fiware-ds2.scopes.catalog`) and the provider validated the presented credential's
issuer against that profile's TIL (`ebsiTir.profiles.dsp-fiware-ds2.tilAddress`, both in
[One connector, two profiles](#one-connector-two-profiles)). Both are
per-profile settings, so pointing the TIL of one profile at a list that does not carry the issuer
and redeploying is the clearest way to see the separation: that profile's catalog request then
fails at credential validation while the other one keeps working.

### Flow 4 - a full consumer flow through the second dataspace

[`dsp-consume.sh`](./scripts/dsp-consume.sh) walks catalog, negotiation, transfer and EDR. It talks
to the management API directly, so expose it first:

```shell
kubectl port-forward svc/consumer-fdsc-edc-dcp -n consumer 8085:8085
```

Its `PROTOCOL` defaults to `http-dsp-profile-2025-1`, so the second dataspace needs it set, next to
the profile in the DSP address:

```shell
MANAGEMENT_TOKEN=${MANAGEMENT_TOKEN} PROTOCOL=dsp-fiware-ds2 ./doc/scripts/dsp-consume.sh \
  https://dcp-mp-operations.127.0.0.1.nip.io/api/dsp/did:web:mp-operations.org/dsp-fiware-ds2 \
  http://localhost:8085/api/v1/management/v5beta/participants/did:web:fancy-marketplace.biz \
  did:web:mp-operations.org
```

The script prints the data endpoint and the access token at the end; use them as in step 8 of the
[DCP walkthrough](./DSP_INTEGRATION.md#dcp-1). Running it twice, once per profile, produces two
independent agreements and transfers - negotiations and transfers belong to the context, not to the
profile, so both show up under the same context's `/contractnegotiations/request`.

### Flow 5 - a second identity, by hand

The local scenario stops at one DID per connector: `identityhub.bootstrap` registers exactly one
participant. To demo the [two-dataspaces-two-DIDs](#two-dataspaces-two-dids) setup on top of the
running `dsp` scenario:

1. Add the second profile and the second context to
   `fdsc-edc.deployment.dcp.config.additionalProperties` of
   [`k3s/dsp-provider.yaml`](../k3s/dsp-provider.yaml), as in [Two dataspaces, two
   DIDs](#two-dataspaces-two-dids), and redeploy the provider release.
2. Provision the second DID: `derive-key.sh` and `register.sh` from
   `charts/data-space-connector/scripts/`, run with `DID`, `KEY_ALIAS`, `KEY_ID`,
   `STS_CLIENT_SECRET` and the identity key of the second DID, exactly as in
   [setup the provider](./DSP_INTEGRATION.md#setup-the-provider). A `did:web` that resolves locally
   needs its own ingress and certificate, which is the laborious part of this demo.
3. Issue that DID a membership credential and insert it into the identityhub, as in
   [issue membership-credentials](./DSP_INTEGRATION.md#issue-membership-credentials), and make sure
   the TIL of the second profile lists its issuer.
4. Tag the TMForum offering with `"participantContextIds": ["<first DID>", "<second DID>"]` to offer
   it in both dataspaces; untagged offerings stay with
   `tmfExtension.defaultParticipantContextId` only. Flow 3 against the second context then shows
   which offerings each dataspace sees.
5. Give the second dataspace its own management client, scoped to its context with the hardcoded
   `sub` mapper from [Management API](#management-api), and repeat flow 3 with that client's token:
   it reaches `.../participants/<second DID>/...` and nothing else.

## Configuring contexts and profiles in the DSC chart

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

These are the values behind the [demo flows](#demo-flows): the provider of the local `dsp` scenario
serving one context on the two profiles `http-dsp-profile-2025-1` and `dsp-fiware-ds2`. The second
profile's DCP scopes and TIL equal the defaults here, because the local setup has a single trust
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
          tmfExtension.schemaBaseUri=https://raw.githubusercontent.com/SEAMWARE/fdsc-edc/2.1.1/schemas/\n    \
          edc.iam.oauth2.issuer=https://keycloak-provider.127.0.0.1.nip.io/realms/test-realm\n    \
          edc.iam.oauth2.jwks.url=http://provider-keycloak.provider.svc.cluster.local:8080/realms/test-realm/protocol/openid-connect/certs"
```

The consumer, [`k3s/dsp-consumer.yaml`](../k3s/dsp-consumer.yaml), is attached to the same two
profiles. `it/src/test/resources/it/dsp_multi_profile.feature` reads the catalog through each of
them.

## Two dataspaces, two DIDs

When the second dataspace expects a different identity, add a second context with its own DID; the
manual steps to try this in the local scenario are [flow 5](#flow-5---a-second-identity-by-hand). The
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
          tmfExtension.schemaBaseUri=https://raw.githubusercontent.com/SEAMWARE/fdsc-edc/2.1.1/schemas/\n    \
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
   [Offering a product in one, several or all dataspaces](https://github.com/SEAMWARE/fdsc-edc/blob/2.1.1/docs/multi-dataspace.md#offering-a-product-in-one-several-or-all-dataspaces)
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

## Migrating from 10.x

The checklist is in the [10.23.0 release notes](./release-notes/10-x.md#warning-required-action-dcp-deployments).
In short: declare a context per DID, add `%s` to the callback, set the two `tmfExtension.*` keys and
the OAuth2 issuer, create a management client and move every management call to v5beta, and hand
counterparties the new DSP address.
