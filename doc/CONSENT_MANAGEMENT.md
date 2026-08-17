## Consent Management

Some data spaces require that access to personal data is backed by the **explicit consent** of the data subject, recorded in an auditable way. The Data Space Connector can deploy an optional consent-management layer based on the [Prometheus-X / Visions consent-manager](https://github.com/VisionsOfficial/consent-manager), producing [ISO/IEC TS 27560](https://www.iso.org/standard/80392.html) consent records, and enforce them at the gateway through the existing ODRL/OPA authorization stack. The full design, its blockers and the backlog of deferred fixes are documented in [CONSENT_MANAGEMENT_PLAN.md](../CONSENT_MANAGEMENT_PLAN.md).

> :warning: The consent-management integration is a **proof-of-concept** (see the `UF-1..UF-14` backlog in the plan). The shared consent key and participant token are POC-grade, the consent-manager runs **unmodified** in the trust-anchor (central authority) namespace and is kept cluster-internal (no ingress). Do not use this configuration as-is in production.

### Components

Consent management is deployed across **two namespaces** - a central **authority**
(the trust-anchor, which knows all participants; in real deployments the consent-manager
is run by such a central authority) and the **provider**.

**Trust-anchor (central authority)** - a second release of the DSC umbrella chart
named `consent-authority`, via [`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml):

* **consent-manager** (`consent-manager:3000`) - stores and serves the consent records/receipts (Node/MongoDB), on a managed MongoDB replica set (dedicated `consent` database). It points at the consent-facade (now in the same namespace) via `CONTRACT_SERVICE_BASE_URL` (`consent-facade.trust-anchor.svc.cluster.local`).
* an internal **APISIX facade** in front of the consent-manager (route `/consent-manager/*`) that authenticates callers **per participant** (`jwt-auth`, keyed on the token's `participant_name`) and injects the shared consent key server-side - so callers authenticate with their own participant JWT and never hold the consent key. Not exposed via ingress (roadmap item 1).
* the **consent-facade** (`consent-facade:8080`) - a Micronaut service ([wistefan/consent-facade](https://github.com/wistefan/consent-facade)) that projects a provider's TMForum APIs (agreements, catalog, party) into the contract-service API the consent-manager consumes: bilateral contracts, catalog self-descriptions, and participant self-descriptions at `/participants/{tmforum-org-id}` (party API). It runs at the authority so it can front **multiple providers**, authenticating its outbound TMForum calls to each provider's OID4VP-protected api (as `did:web:dataspace-authority.org`). It replaces the POC node `contract-facade` (plan §10, backlog `UF-10`).
* a **consent secret** carrying the session/JWT/oauth secrets and the shared consent key.

**Provider**:

* the **consent-filter APISIX plugin** on the `mp-data-service-consent` route, which calls the authority's consent-manager (through its facade at `consent-authority-apisix-gateway.trust-anchor`) to gate access. This plugin is the **canonical (and only) consent-enforcement path** - OPA/odrl-pap still authorizes each request on the presented *credential*, but the *consent* decision is made by the plugin.

### Enabling

Consent management is **opt-in** via the dedicated `consent` maven profile. The
provider config lives in [`k3s/consent-provider.yaml`](../k3s/consent-provider.yaml)
(layered on `provider.yaml`) and carries only the **consent-filter plugin**; the
central-authority config - the consent-manager, its APISIX facade, the **consent-facade**
and a managed MongoDB, deployed as the `consent-authority` release into the trust-anchor
namespace - lives in [`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml).
The provider overlay disables both the consent-manager and the consent-facade (they run
at the authority) and keeps the plugin:

```yaml
consentManagement:
  enabled: true
  consentManager:
    enabled: false   # the consent-manager runs at the trust-anchor, not the provider
  consentFacade:
    enabled: false   # the consent-facade also runs at the trust-anchor (it fronts multiple providers)
```

The shared `consentKey` (`X_VISIONSTRUST_CONSENT_KEY`) is set on the authority side in [`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml) (`consentManagement.consentKey`); the consent-manager validates it and the facade route injects it, so callers never hold it.

Deploy the consent scenario with `mvn clean deploy -Pconsent`. That profile builds
and imports the `consent-manager:local` image and deploys the trust-anchor
`consent-authority` release (consent-manager + consent-facade + MongoDB) plus the
provider's consent-filter plugin. A plain `mvn clean deploy -Plocal` brings up the
connector **without** consent management.

> :bulb: The consent-manager, its APISIX facade, the **consent-facade** and MongoDB all run in the `trust-anchor` namespace; the provider runs only the consent-filter plugin (inside its own APISIX). Check with `kubectl -n trust-anchor get pods` (consent-manager, consent-facade, `consent-authority-apisix`, `mongodb`).

### Working with consent records

The consent-manager is intentionally **not exposed via an ingress**; reach it through its authority
**APISIX facade** - the front door that authenticates callers per-participant (`jwt-auth`) and injects
the shared consent key server-side, so you present a participant token (`Authorization: Bearer`) and
never hold the consent key. Port-forward the facade and set the same `$CM` base the
[Demo](#demo-consent-gated-access-to-personal-data) uses:

```shell
  kubectl -n trust-anchor port-forward svc/consent-authority-apisix-gateway 3001:80
  export CM=http://localhost:3001/consent-manager/v1
```

> :key: **Reproducing the plugin's two calls by hand.** The plugin obtains these itself (it logs in with the participant client credentials and reads `/participants/me`), but to run the two calls manually you need the same two values tied to the provider participant: a *participant token* - a JWT signed with the consent-manager's `JWT_SECRET_KEY` (secret `consent-manager-secret`, key `jwtSecret`) whose `sub` is the provider participant's `_id` - and the *provider self-description* URL, which the consent-facade serves at `http://consent-facade.trust-anchor.svc.cluster.local:8080/participants/{tmforum-org-id}` (the facade runs in the trust-anchor namespace, alongside the consent-manager). Mint the token inside the pod (the participant must already exist):
>
> ```shell
>   kubectl -n trust-anchor exec deploy/consent-manager -- node -e '
>     const jwt = require("jsonwebtoken");
>     const mongoose = require("mongoose");
>     const Participant = require("/usr/src/app/dist/src/models/Participant/Participant.model").default;
>     (async () => {
>       await mongoose.connect(process.env.MONGO_URI);
>       const p = await Participant.findOne({ clientID: "consent-demo-provider" });
>       console.log("token:", jwt.sign({ sub: String(p._id) }, process.env.JWT_SECRET_KEY));
>       console.log("providerSd:", p.selfDescriptionURL);
>       await mongoose.disconnect();
>     })();'
> ```

An identity in the consent-manager is a **`UserIdentifier`** - a record that binds an **e-mail** to a **participant** (`attachedParticipant`). The consent-manager keys its lookup and cross-participant matching on that `email` field, so the DSC simply uses the **access-token `sub` as-is** as the value (for the PoC we assume `sub` *is* the identifier - it may be a `did:key:…` or a real e-mail; either is stored verbatim). A consent record is thus tied to whatever `sub` appears in the presented credential's access token. Reconciling on the dedicated `identifier` field instead is `UF-3` (needs consent-manager changes; deferred/upstream).

#### Registering a user identity (must happen before it can be resolved)

```shell
  mkdir cert
  chmod o+rw cert
  docker run -v $(pwd)/cert:/cert quay.io/wi_stefan/did-helper:0.1.1
  # unsecure, only do that for demo
  sudo chmod -R o+rw cert/private-key.pem
```

The `identifier/search` call below only finds a `UserIdentifier` that has been **registered first**. In the consent-manager's own model a participant registers one of its users' identifiers itself, authenticated with a **participant JWT** (`sub` = the participant's id in the consent-manager):

```shell
  curl -s -X POST $CM/users/register \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer <participantToken>" \
    -d '{ "email": "did:key:zDnaeeWrPc6G8GQEpBgLJGwPdFtHK1Mk7JbYhQyFu8waJNLBx" }'
  # -> the created UserIdentifier (attachedParticipant = the participant from the token)
```

The `email` is the holder DID; a throw-away one can be minted with the [did-helper](https://github.com/wistefan/did-helper) (`docker run -v $(pwd)/cert:/cert quay.io/wi_stefan/did-helper:0.1.1`, then read `cert/did.json`). The PDI `User` account itself is created with `POST /v1/users/signup`, and a background matcher links identifiers that share an e-mail across participants.

> :warning: **In the DSC POC nothing registers users automatically.** Identity bootstrapping - creating the provider/consumer participants and registering each holder's DID - is deferred (backlog `UF-1`/`UF-2`/`UF-3`) and the consent-manager is deployed **empty**, so out of the box `identifier/search` returns `userIdentifierExists: false`. The [Demo](#demo-consent-gated-access-to-personal-data) below performs this registration through the consent-manager API - it registers the provider/consumer participants and the subject's `UserIdentifier` (`email` = the holder DID) before recording the granted consent.

Once an identity is registered, the consent-filter plugin that gates access resolves and checks consent with a **two-call chain** against the consent-manager:

1. Resolve the DID to a `UserIdentifier`. The consent-manager requires the shared consent key on this
   endpoint; through the facade you present your **participant JWT** and the facade injects that key.
   `selfDescription` must be the **provider self-description** (`providerSd`) the consent-facade serves
   at `/participants/{tmforum-org-id}` - the value `GET /participants/me` returns for the provider
   participant:
```shell
  curl -s -X POST $CM/users/identifier/search \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer <participantToken>" \
    -d '{
          "selfDescription": "http://consent-facade.trust-anchor.svc.cluster.local:8080/participants/urn:ngsi-ld:organization:<id>",
          "email": "did:key:zDnaeeWrPc6G8GQEpBgLJGwPdFtHK1Mk7JbYhQyFu8waJNLBx"
        }' | jq .
  # -> { "participantExists": true, "userIdentifierExists": true, "userIdentifier": "<id>", ... }
```
2. List that user's consents (auth: a provider participant JWT signed with the consent-manager's `JWT_SECRET_KEY`, `sub` = the provider participant id); the plugin allows only if some consent has `status == "granted"`:
```shell
  curl -s "$CM/consents/participants/<userIdentifier>?receipt=true" \
    -H "Authorization: Bearer <participantToken>" | jq '.consents[] | .status'
```

> :warning: `GET /consents/participants/...` **always builds a receipt** for every consent, which HTTP-fetches each participant's `selfDescriptionURL` and reads `legalPerson.legalAddress` from it. So the participants' `selfDescriptionURL` **must** resolve to a valid self-description - the consent-facade serves these at `/participants/{tmforum-org-id}` (backed by the party API), which is why each participant is backed by a real TMForum organization (created by the deploy-time register Job and, for the consumer, by the Demo below). A participant whose SD URL 404s makes this call return `500`, which the plugin treats as "no consent".

The full give-consent flow additionally needs a bootstrapped privacy notice - the consent-facade projects one from a TM Forum agreement. The [Demo](#demo-consent-gated-access-to-personal-data) below grants and revokes consent end to end through the consent-manager API (registering the participants and subject, seeding the agreement, then `POST /v1/consents`), with no direct database writes.

### Enforcing consent on a concrete service

Consent is enforced on the data path by the **APISIX consent-filter plugin** - the canonical, only enforcement path (used by the [demo below](#demo-consent-gated-access-to-personal-data)). A custom external plugin attached to the `mp-data-service-consent` route runs the **two-call consent check** against the consent-manager on every request - resolve the subject's `userIdentifier` (`POST /v1/users/identifier/search`, authenticated with the `consent_key`), then list its consents (`GET /v1/consents/participants/{id}`, authenticated with the participant token) - and blocks unless a granted consent exists for the credential subject. OPA still authorizes the request on the *credential* first; the plugin adds the *consent* gate on top, keeping the access policy free of consent logic. The plugin authenticates with participant **client credentials** — it reads `client_id`/`client_secret` (the `consent_key` is injected by the facade), logs in via `/participants/login` for a (refreshing) participant token, and derives the provider self-description from `/participants/me`. Those credentials are **stable** (`consent-demo-provider`/`demo`, established by the deploy-time register Job), so no per-seed value is wired into the plugin.

The check runs in the **response phase** (`ext-plugin-post-resp`) rather than pre-request: a personal-data read can return entities belonging to several data subjects, and gating on the response lets the decision be made per subject in the body rather than only on the caller's token. The alternative - the odrl-pap `consent:hasValidConsent` PIP evaluating consent inside OPA - is **not used**; consent is decided by the plugin, and odrl-pap/OPA handles only credential authorization.

The intended end-to-end behaviour is:

1. Authenticate via OID4VP (see the [demo prerequisites](#demo-consent-gated-access-to-personal-data)) and call the service **without** a consent record &rarr; **403**.
2. Grant consent for the holder DID in the consent-manager (see above) &rarr; the same request now returns **200**.
3. Revoke the consent &rarr; the request returns **403** again.

> :bulb: **Status.** The consent-filter plugin implements the two-call check directly and gates the `mp-data-service-consent` route end to end (grant → 200 / revoke → 403, see [`verify_consent_flow.sh`](scripts/verify_consent_flow.sh)); the surrounding user-bootstrap flow it relies on is still POC-grade (`UF-1..UF-14`).

> :warning: A full local bring-up of the provider **with** consent management is resource-hungry; the &ge;24 GB recommendation in the [Requirements](#requirements) applies.

### Access audit log

The consent-filter plugin can record **every access decision** (who accessed what, under which decision) to the OpenTelemetry Collector, kept **separate from traces**. When `audit_enabled` is set in the route conf, the plugin emits one **OTLP/HTTP log record** per request to `audit_otlp_endpoint` (`…/v1/logs`), stamped with resource `service.name=consent-access-audit` and log attributes `event.domain=audit`, `consent.decision` (allow/deny), `consent.reason`, `enduser.id` (the subject), `http.request.method`, `url.path`, `http.request.id`. Emission is **asynchronous and best-effort** - it never blocks or changes the access decision, and drops (with a counter) if the Collector is slow/absent; durability and retention are the Collector's sink's job, not the plugin's.

Enable it in two steps:

1. Deploy the Collector (`opentelemetry-collector.enabled: true`) and set `audit_enabled: true` on the `mp-data-service-consent` route in [`k3s/provider.yaml`](../k3s/provider.yaml) (the endpoint defaults to `http://provider-opentelemetry-collector:4318`). The endpoint may also be supplied out-of-band via the `CONSENT_AUDIT_OTLP_ENDPOINT` env var.
2. Give the Collector a **`logs/audit` pipeline** that routes on the marker to a durable, append-only sink - separate from the traces pipeline:

```yaml
# routes audit records (service.name=consent-access-audit) to their own sink;
# because the plugin is the only logs source today, a plain logs pipeline suffices.
service:
  pipelines:
    logs:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [file/audit]      # or opensearch/audit, kafka/audit, ...
```

If you later ingest other logs too, split them with the `routing` connector on `service.name == "consent-access-audit"` (see the OTEL notes). The plugin only *routes* the audit stream out; immutability/retention is enforced by the chosen sink (WORM volume, OpenSearch audit index with restricted deletes, Kafka→immudb, …). The **consent record** itself (its TS 27560 lifecycle log) is a separate concern that lives in the consent-manager, not here.

### Demo: consent-gated access to personal data

This walkthrough shows the core consent story end to end: a data subject publishes personal data at the provider, a consumer is **denied** access until the subject **grants consent**, after which the identical request **succeeds**.

> :bulb: **Two enforcement layers.** The `mp-data-service-consent` route runs each request through **two** gates: first OPA (fed by odrl-pap) authorizes the call on the presented *credential*, then the custom **consent-filter** APISIX plugin gates it on the data subject's *consent* by calling the consent-manager. This walkthrough exercises exactly that split - OPA must **allow** the `PersonalProfile` read (step 0) so that the **plugin** is the component that denies the access when no consent exists and permits it once consent is granted. The data requests therefore target the plugin-enforced host `mp-data-service-consent.127.0.0.1.nip.io`; the access token is still obtained from `mp-data-service.127.0.0.1.nip.io`, which serves the OIDC discovery.

> :bulb: Step **1** (publishing data) and step **3** (grant/revoke consent, via the consent-manager API) are verified against the live cluster. Steps **0/2/4** exercise the OPA-allow + consent-plugin path; the plugin performs the two-call check against the consent-manager, and the user-bootstrap flow it relies on is POC-grade (`UF-1..UF-14`).

**Prerequisites.** Deploy the data space with consent management enabled (`mvn clean deploy -Pconsent`, see [Enabling](#enabling)). All commands below are run from the repository root. The grant step reaches the consent-manager and TM Forum API through `kubectl port-forward`, so point `KUBECONFIG` at the local cluster:

```shell
  export KUBECONFIG=$(pwd)/target/k3s.yaml
```

Then, on the consumer side, generate a holder identity and issue the credential the demo presents - the same steps as in the [local deployment guide](deployment-integration/local-deployment/LOCAL.MD#the-data-consumer), repeated here so this walkthrough is self-contained.

1. Generate the consumer's holder key material (a `did:key` plus signing key under `cert/`, read by `get_access_token_oid4vp.sh`):

```shell
  mkdir -p cert && chmod o+rw cert
  docker run -v $(pwd)/cert:/cert quay.io/wi_stefan/did-helper:0.1.1
  # unsecure, demo only:
  sudo chmod -R o+rw cert/private-key.pem
```

2. Issue a `UserCredential` for the consumer's `employee` user and keep it in `$USER_CREDENTIAL`:

```shell
  export USER_CREDENTIAL=$(./doc/scripts/get_credential.sh https://keycloak-consumer.127.0.0.1.nip.io user-credential employee); echo ${USER_CREDENTIAL}
```

**0. Allow the read at OPA - consent is left to the plugin.** Register a policy that permits *read* of `PersonalProfile` entities to any credential holder (`vc:any`). It carries **no** consent refinement: OPA authorizes purely on the credential, so the request reaches the consent-filter plugin, which is the component that decides on consent. (Without this policy OPA denies the call outright and the plugin never runs.)

```shell
  curl -k -x localhost:8888 -s -X POST https://pap-provider.127.0.0.1.nip.io/policy \
    -H 'Content-Type: application/json' \
    -d '{
          "@context": { "odrl": "http://www.w3.org/ns/odrl/2/" },
          "@id": "https://mp-operation.org/policy/common/personalProfileRead",
          "odrl:uid": "https://mp-operation.org/policy/common/personalProfileRead",
          "@type": "odrl:Policy",
          "odrl:permission": {
            "odrl:assigner": { "@id": "https://www.mp-operation.org/" },
            "odrl:target": {
              "@type": "odrl:AssetCollection",
              "odrl:source": "urn:asset",
              "odrl:refinement": [
                { "@type": "odrl:Constraint", "odrl:leftOperand": "ngsi-ld:entityType", "odrl:operator": { "@id": "odrl:eq" }, "odrl:rightOperand": "PersonalProfile" }
              ]
            },
            "odrl:assignee": { "@id": "vc:any" },
            "odrl:action": { "@id": "odrl:read" }
          }
        }'
```

**1. The subject publishes personal data.** The data owner creates a `PersonalProfile` entity in the provider's context broker (via the demo scorpio ingress):

```shell
  curl -k -x localhost:8888 -s -X POST https://scorpio-provider.127.0.0.1.nip.io/ngsi-ld/v1/entities \
    -H 'Content-Type: application/json' \
    -d '{
      "id": "urn:ngsi-ld:PersonalProfile:alice",
      "type": "PersonalProfile",
      "email": { "type": "Property", "value": "alice@example.org" },
      "loyaltyPoints": { "type": "Property", "value": 4200 }
    }'
```

**2. The consumer requests the data &rarr; access denied.** Authenticate the consumer via OID4VP - the `get_access_token_oid4vp.sh` helper builds the `vp_token` from the `cert/` identity created in the prerequisites - and read the entity through the plugin-enforced host. OPA now authorizes the read (step 0), but no consent exists yet, so the **consent-filter plugin** denies the request:

```shell
  export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh https://mp-data-service.127.0.0.1.nip.io $USER_CREDENTIAL default); echo ${ACCESS_TOKEN}
  curl -k -x localhost:8888 -s -o /dev/null -w 'HTTP %{http_code}\n' \
    -X GET 'https://mp-data-service-consent.127.0.0.1.nip.io/ngsi-ld/v1/entities/urn:ngsi-ld:PersonalProfile:alice' \
    -H "Authorization: Bearer ${ACCESS_TOKEN}"
  # -> HTTP 403 (denied by the consent-filter plugin, not by OPA)
```

**3. The subject grants consent.** The plugin gates on the **access-token `sub`** (here the holder DID from `cert/`, which the consent-manager stores as the identity) - *not* the credential's `credentialSubject.id`. So extract the subject from the token obtained in step 2, and grant for that value:

```shell
  export SUBJECT_DID=$(echo "${ACCESS_TOKEN}" | cut -d. -f2 | base64 -d 2>/dev/null | jq -r .sub); echo ${SUBJECT_DID}
```

The consent-manager is deployed empty. Rather than seeding Mongo, the demo records consent through
the **real give-consent API** (no direct database writes): it registers the two participants, seeds a
TM Forum **agreement** (which the consent-facade projects into a privacy notice), registers the
subject, then `POST /v1/consents`. Every consent-manager call goes through the authority's **APISIX
facade** (`consent-authority-apisix-gateway`, route `/consent-manager/*`): it authenticates the caller
**per participant** with their JWT (`jwt-auth`) and injects the shared consent key server-side - so you
authenticate with a participant token (`Authorization: Bearer`) and never hold the consent key.
Port-forward that facade and the provider's TM Forum API. `FACADE` is a **cluster** URL on purpose - it
is stored in the consent-manager and dereferenced by it *in-cluster*, so it must be the service FQDN,
not `localhost`:

```shell
  kubectl -n trust-anchor port-forward svc/consent-authority-apisix-gateway 3001:80
  kubectl -n provider     port-forward svc/tm-forum-api-svc 8090:8080
  kubectl -n trust-anchor port-forward svc/consent-authority-apisix-admin 9180:9180   # for the 3a consumer onboarding

  export CM=http://localhost:3001/consent-manager/v1   # -> the authority APISIX facade -> consent-manager
  export TMF=http://localhost:8090/tmf-api
  export FACADE=http://consent-facade.trust-anchor.svc.cluster.local:8080
  export DID=$SUBJECT_DID
```

> :warning: **Every `$CM` call must go through the facade** (`localhost:3001` must be the
> `consent-authority-apisix-gateway` port-forward, **not** the consent-manager). The calls below
> authenticate per-participant with the participant JWT (`Authorization: Bearer`); the facade
> validates it and injects the shared consent key server-side. A common trap: a **leftover
> `svc/consent-manager 3001:3000` port-forward** from an earlier run keeps holding port `3001`, so
> starting the gateway port-forward fails silently (`address already in use`) and `localhost:3001`
> still reaches the consent-manager directly. Symptoms of hitting the consent-manager directly:
>
> - **`3a` login** returns HTML `Cannot POST /consent-manager/v1/participants/login` (Express 404 -
>   the consent-manager has no `/consent-manager` prefix; the facade strips it).
> - other calls return `{"message":"Authorization header missing or invalid"}` (the consent key is
>   never injected).
>
> Fix it by killing the stale forward and re-establishing the gateway one:
>
> ```shell
>   pkill -f 'port-forward.*3001'; sleep 1
>   kubectl -n trust-anchor port-forward svc/consent-authority-apisix-gateway 3001:80 &
>   # confirm you are on the facade (a no-auth request is rejected by APISIX jwt-auth, not Express):
>   curl -s -X POST $CM/users/identifier/search -d '{}'
>   # {"message":"Missing JWT token in request"}  -> facade (correct); the 3a calls add the Bearer token
> ```
>
> The participant token expires after 1 h; if a call starts failing with an *invalid JWT* error,
> re-run the login in 3a to refresh `$PROVIDER_JWT` (and `$CONSUMER_JWT`).

**3a. Participants.** The provider participant already exists (registered at deploy time by the
`register-provider-participant` Job); log in as it and read its own self-description, then create the
consumer (assumed not to exist yet in this test env).

```shell
  # provider login -> participant token, then its own self-description (no DB read)
  export PROVIDER_JWT=$(curl -s -X POST $CM/participants/login -H 'Content-Type: application/json' \
    -d '{"clientID":"consent-demo-provider","clientSecret":"demo"}' | jq -r .jwt)
  export PROVIDER_SD=$(curl -s $CM/participants/me -H "Authorization: Bearer $PROVIDER_JWT" | jq -r .selfDescriptionURL); echo $PROVIDER_JWT
  export PROV_ORG=${PROVIDER_SD##*/participants/}          # the backing TM Forum org id

  # create the consumer TM Forum org -> its self-description
  export CONS_ORG=$(curl -s -X POST $TMF/party/v4/organization -H 'Content-Type: application/json' \
    -d '{"name":"Consent Demo Consumer","tradingName":"Consent Demo Consumer","isLegalEntity":true,
         "organizationType":"company","contactMedium":[{"characteristic":{"country":"DE"}}],
         "partyCharacteristic":[{"name":"did","value":"did:web:fancy-marketplace.biz"}]}' | jq -r .id); echo $CONS_ORG
  export CONSUMER_SD=$FACADE/participants/$CONS_ORG

  # register the consumer participant (201, or 409 if it already exists), then log in as it.
  # POST /participants sits behind the facade's per-participant jwt-auth, so authenticate the
  # call with the existing provider token.
  curl -s -w '%{http_code}\n' -X POST $CM/participants -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $PROVIDER_JWT" \
    -d "{\"legalName\":\"Fancy Marketplace Co.\",\"email\":\"consumer@fancy-marketplace.biz\",
         \"did\":\"did:web:fancy-marketplace.biz\",\"clientID\":\"consent-demo-consumer\",
         \"clientSecret\":\"demo\",\"selfDescriptionURL\":\"$CONSUMER_SD\"}"
  export CONSUMER_JWT=$(curl -s -X POST $CM/participants/login -H 'Content-Type: application/json' \
    -d '{"clientID":"consent-demo-consumer","clientSecret":"demo"}' | jq -r .jwt); echo $CONS_ORG

  # Onboard the consumer to the facade. The provider was onboarded at deploy (the register Job
  # provisions its jwt-auth consumer); the consumer is created here, so provision its facade
  # jwt-auth consumer too - keyed on its participant_name (the legalName), signed with the
  # consent-manager JWT secret - otherwise its token is rejected at the facade with 401. In a
  # real dataspace each participant does this itself at onboarding. Uses the APISIX admin API
  # (port-forwarded in the prerequisites above):
  export JWT_SECRET=$(kubectl -n trust-anchor get secret consent-manager-secret -o jsonpath='{.data.jwtSecret}' | base64 -d)
  curl -s -X PUT http://localhost:9180/apisix/admin/consumers -H 'X-API-KEY: admin' \
    -d '{"username":"Fancy_Marketplace_Co_","plugins":{"jwt-auth":{"key":"Fancy Marketplace Co.","secret":"'"$JWT_SECRET"'","algorithm":"HS256"}}}'
```

**3b. Contract source (the EDC stand-in).** In production the provider↔consumer **agreement** is
written by the Marketplace / EDC contract negotiation; the facade only *projects* it. The demo has no
negotiation, so it creates a product specification (carrying the `purpose` characteristic), an
offering, and the agreement through the TM Forum API:

```shell
  export SPEC_ID=$(curl -s -X POST $TMF/productCatalogManagement/v4/productSpecification \
    -H 'Content-Type: application/json' -d @- <<JSON | jq -r .id
{ "name": "Personal Profile", "description": "The subject's profile",
  "productSpecCharacteristic": [ { "name": "purpose", "valueType": "object",
    "productSpecCharacteristicValue": [ { "value": {
      "id": "profile-service-provision",
      "name": "Personal profile for service provision",
      "description": "Deliver the requested service.",
      "purpose": "https://w3id.org/dpv#ServiceProvision" } } ] } ] }
JSON
  ); echo $SPEC_ID

  export OFFERING_ID=$(curl -s -X POST $TMF/productCatalogManagement/v4/productOffering \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"Personal Profile Offering\",\"productSpecification\":{\"id\":\"$SPEC_ID\"}}" | jq -r .id); echo $OFFERING_ID

  export AGREEMENT_ID=$(curl -s -X POST $TMF/agreementManagement/v4/agreement \
    -H 'Content-Type: application/json' -d @- <<JSON | jq -r .id
{ "name": "Profile sharing agreement", "status": "approved",
  "agreementItem": [ { "productOffering": [ { "id": "$OFFERING_ID" } ] } ],
  "engagedParty": [ { "id": "$PROV_ORG", "role": "Provider" }, { "id": "$CONS_ORG", "role": "Consumer" } ],
  "characteristic": [
    { "name": "policy", "value": { "@type": "Set", "uid": "urn:policy:profile",
        "permission": [ { "target": "urn:asset:profile", "action": "use" } ] } },
    { "name": "provider-id", "value": "$PROVIDER_SD" },
    { "name": "consumer-id", "value": "$CONSUMER_SD" },
    { "name": "signing-date", "value": $(date +%s) } ] }
JSON
  ); echo $AGREEMENT_ID
```

**3c. Register the subject.** A `UserIdentifier` binds the holder DID to a participant; register it on
**both** sides (idempotent - a repeat registration is a no-op):

```shell
  curl -s -o /dev/null -X POST $CM/users/register -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $PROVIDER_JWT" -d "{\"email\":\"$DID\",\"identifier\":\"$DID\"}"
  curl -s -o /dev/null -X POST $CM/users/register -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $CONSUMER_JWT" -d "{\"email\":\"$DID\",\"identifier\":\"$DID\"}"
```

**3d. Resolve the `x-user-key` (search).** The provider-side `UserIdentifier._id` is the `x-user-key`
that selects the subject when granting. Resolve it from the holder DID via `identifier/search` - the
same lookup the consent-filter plugin does, and it works whether the identifier was just registered or
already existed. You authenticate per-participant with the **provider token**; the facade injects the
consent key this endpoint requires. `selfDescription` must be the **provider** self-description
(`$PROVIDER_SD`, the value `GET /participants/me` returns):

```shell
  export USER_KEY=$(curl -s -X POST $CM/users/identifier/search -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $PROVIDER_JWT" \
    -d "{\"selfDescription\":\"$PROVIDER_SD\",\"email\":\"$DID\"}" | jq -r .userIdentifier); echo $USER_KEY
```

A non-empty `userIdentifier` (`userIdentifierExists: true`) is the key; an empty result means the
registration in 3c has not propagated yet - retry.

**3e. Grant.** Fetch the privacy notice the facade projected (it must have non-empty `data` **and**
`purposes`), then give consent for its data. These calls carry the **provider token** (per-participant
auth) *and* the **`x-user-key` header** - the `UserIdentifier._id` from 3d, which selects the subject
(the `:userId` path segment is just a placeholder the header overrides).

```shell
  export PROV_B64=$(printf '%s' "$PROVIDER_SD" | base64 -w0)
  export CONS_B64=$(printf '%s' "$CONSUMER_SD" | base64 -w0)

  export NOTICE=$(curl -s "$CM/consents/$DID/$PROV_B64/$CONS_B64" \
    -H "Authorization: Bearer $PROVIDER_JWT" -H "x-user-key: $USER_KEY")
  echo "$NOTICE" | jq '.[0] | {privacyNoticeId: ._id, data: [.data[].resource], purposes: [.purposes[].purpose]}'

  export PN_ID=$(echo "$NOTICE" | jq -r '.[0]._id')
  export DATA=$(echo "$NOTICE"  | jq -c '[.[0].data[].resource]')

  # the receipt's recordId is the consent's id (kept for the withdraw step)
  export CONSENT_ID=$(curl -s -X POST $CM/consents -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $PROVIDER_JWT" -H "x-user-key: $USER_KEY" \
    -d "{\"privacyNoticeId\":\"$PN_ID\",\"event\":\"given\",\"data\":$DATA}" | jq -r .record.recordId); echo "granted, consent id: $CONSENT_ID"
```

A `201` with a receipt is the grant. The consent-filter plugin now sees it: it authenticates with the
stable client credentials (`consent-demo-provider`/`demo`, already in the `mp-data-service-consent`
route conf in [`k3s/provider.yaml`](../k3s/provider.yaml)) and derives the provider SD from
`/participants/me`, so no per-grant wiring is needed. (The jwt-auth consumer per participant in the
authority APISIX is provisioned by the deploy reconcile Job, not by granting.)

> :warning: **Consistency.** The consumer participant's stored `selfDescriptionURL` (3a), the
> agreement's `consumer-id` (3b), and the `consumerId` passed to the grant (3e) must be **identical**
> (likewise for the provider) - on a fresh deploy the steps satisfy this by construction. Re-running
> against a *different* consumer org while `consent-demo-consumer` already exists leaves the
> participant pinned to its original SD (`POST /v1/participants` is a no-op `409`), which mismatches
> the agreement and fails the grant with `No Matching user found`. The steps also assume a **single**
> agreement between the pair; delete stale ones (`DELETE $TMF/agreementManagement/v4/agreement/{id}`)
> if you re-seed.

**4. The consumer requests again &rarr; access allowed.** With a granted consent in place the plugin now lets the request through, and the identical call succeeds:

```shell
  export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh https://mp-data-service.127.0.0.1.nip.io $USER_CREDENTIAL default)
  curl -k -x localhost:8888 -s -w '\nHTTP %{http_code}\n' \
    -X GET 'https://mp-data-service-consent.127.0.0.1.nip.io/ngsi-ld/v1/entities/urn:ngsi-ld:PersonalProfile:alice' \
    -H "Authorization: Bearer ${ACCESS_TOKEN}"
  # -> the entity + HTTP 200
```

**Withdraw the consent** by terminating it (the `CONSENT_ID` from step 3 is still in scope) - its
status flips to `terminated`, so the plugin stops allowing and the very next request returns `403`
again. Access follows the subject's consent in real time:

```shell
  curl -s -X POST $CM/consents/$CONSENT_ID/terminate -H 'Content-Type: application/json' \
    -H "Authorization: Bearer $PROVIDER_JWT" -H "x-user-key: $USER_KEY" -d '{}' | jq '{recordId: .record.recordId}'
```
(Re-running the grant in step 3e issues a fresh `granted` consent, so access is allowed again.)

> :bulb: To run this whole check in one shot, use [`./doc/scripts/verify_consent_flow.sh`](scripts/verify_consent_flow.sh) - it issues the token, then drives the same give-consent API to grant → assert `200`, revoke → assert `403`, re-grant → assert `200`, and exits non-zero on any failure. Needs the `cert/` holder identity from the prerequisites above.


### Arch

- check consent on response, filter response for data to be included/excluded
  - data needs to be flagged as personal/subject-to-consent
  - 

#### Contract Facade

- provide "Agreement" from TMForum in form Prometheus-X Contract Format
- filter for "Agreement" by participant ID(consumer, provider) - id to ask needs to be did


![consent-arch](./img/consent/consent.png)

## From PoC to production: open steps

The current integration is a **proof-of-concept**. The steps below take it to a production-ready state. They extend the `UF-1..UF-14` backlog in [`CONSENT_MANAGEMENT_PLAN.md`](../CONSENT_MANAGEMENT_PLAN.md) §13.4 (referenced as `UF-n`) with what this integration surfaced. Rough priority: **P0** blocks any non-demo use, **P1** is needed for a real deployment, **P2** is hardening.

### 1. Authentication & subject identity (P0)
- **Real participant auth** instead of the single global `x-visionstrust-consent-key` shared secret and the `x-user-key`/query-param/base64-payload shortcuts in the consent-manager (`UF-1`). The consent-manager stays **unmodified**; the extra auth is added by the **APISIX facade in front of it** at the authority (route `/consent-manager/*`, upstream `consent-manager:3000`, in [`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml)), which callers reach cross-namespace at `consent-authority-apisix-gateway.trust-anchor`.
  - **Done (config-only hardening):** every `/consent-manager/*` call requires a **per-participant JWT** (`jwt-auth`, keyed on the token's `participant_name`; one consumer per participant provisioned by the reconcile Job), and the facade **injects** the `x-visionstrust-consent-key` server-side - so callers authenticate as themselves and never hold the shared consent key. The `x-user-key` subject selector is **forwarded** behind that auth (a participant can name the subject of a grant); replacing it by deriving the subject from the token `sub` is the remaining part of `UF-1`. The consent-filter plugin's `consent_api_url` points at `http://consent-authority-apisix-gateway.trust-anchor.svc.cluster.local/consent-manager`. The facade's APISIX is internal (ingress off), so it is not directly reachable from outside.
  - **Done (participant-JWT validation) — verified live:** the facade requires a valid **participant JWT** on every call except `/participants/login` (APISIX `jwt-auth`, keyed on the token's `participant_name` via a consumer whose HS256 secret is the consent-manager's `JWT_SECRET_KEY`); the plugin now sends the token on call 1 too. Verified end-to-end: missing/tampered token → **401** at the facade (before the consent-manager), login stays open, and the full grant→**200** / revoke→**403** flow works. Baked into [`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml) (login-exempt route + `jwt-auth` on the facade route + a consumer-reconcile Job, one consumer per participant).
  - **Done (secret ref):** the consent key injected by the facade is sourced from `consent-manager-secret` (mounted as env `CONSENT_KEY`, referenced in the route header as `$env://CONSENT_KEY`, resolved by APISIX at runtime) - no literal in git or etcd. Verified live.
  - **Done (credentials out of the route conf):** the plugin's `consent_key` is now **optional** (the facade injects it and overrides anything sent), so it is dropped from the provider route conf; the participant `client_secret` is no longer in the route conf either - the plugin reads it from env `CONSENT_CLIENT_SECRET`. APISIX spawns the ext-plugin runner with a *fixed* environment, so the container env never reaches it: the `consent-plugin-credentials` Secret is instead mounted as a file and the runner launch command `export`s it before `exec`ing the runner. `client_id` (not secret) stays in the conf. Verified live.
  - **Done (consumer per participant):** the authority provisions a jwt-auth **consumer per participant**, not one hardcoded consumer, keyed on the participant's `legalName` (the token's `participant_name`) and signed with the shared `JWT_SECRET_KEY`. The deploy **reconcile Job** ([`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml)) runs the consent-manager image to enumerate *all* participants and PUT a consumer for each - idempotent, so it also re-syncs after an apisix/etcd reset (empty/no-op on a fresh deploy); the deploy-time register Job additionally provisions the provider's consumer when it registers the participant.
  - **Not applicable — no NetworkPolicy:** in a real deployment the consent-manager and its gateway are run by the **central authority in a separate cluster**, reached over the network (with the participant token) - they are not co-located with the provider, so a same-cluster lockdown is meaningless. The in-cluster trust-anchor deployment here is only a PoC stand-in for that external authority.
- **Identity = the access-token `sub`, taken as-is (accepted for the PoC).** The plugin uses the token's `sub` verbatim as the consent-manager identifier (stored in `UserIdentifier.email`), and the consent is seeded for the same value - so **no consent-manager change is needed** and it works whether `sub` is a `did:key:…` or a real e-mail. Revisiting this - reconciling on the `identifier` field so a DID and a real e-mail can coexist (`UF-3`), and setting the receipt `piiPrincipalId` to the DID rather than a Mongo ObjectId (`UF-5`) - *would* require consent-manager changes (the lookup, the matcher and the model all key on `email`), so it is **deferred/upstream**, not a DSC task.
- **Consistent `sub`**: the value the consent is granted for must equal the access-token `sub` the verifier issues. The demo grants for the token's `sub` directly (step 3 of the walkthrough); a real deployment needs the verifier ↔ credential ↔ consent-manager to agree on one canonical subject value.

### 2. Consent lifecycle & data-subject UX (P0/P1)
- **Real give/withdraw flow.** Grant and withdraw now run through the consent-manager's **real give-consent API** driven by a privacy notice the facade projects from a TM Forum agreement (no direct Mongo writes; see the Demo). Remaining: the agreement is seeded for the demo (production has the Marketplace/EDC negotiate it), and the **PDI consent UI** (`/consents/pdi/iframe`) still needs to be exposed to data subjects over **ingress + TLS + auth** (ClusterIP-only today, `UF-1`).
- **Consent expiry** (`UF-6`): honor `validityDuration`, compute expiry, transition `status → expired`; the plugin must then also reject expired (it currently checks only `status == "granted"`). Blocked upstream: the `Consent` model has an `"expired"` status but no `validityDuration`/expiry timestamp, so expiry would have to come from the privacy notice (the facade flow).

### 3. Contract projection & participant registration (P1)
- **Complete the consent-facade** TMForum → Prometheus-X projection (agreements/bilaterals/service-offerings/data-resources); it is currently a scaffold and the consent-manager derives its notices/consents from it (`UF-10`, see [Contract Facade](#contract-facade) and the facade repo's `REQUIREMENTS.md`).
- **Done — register the provider participant at deploy time.** A deploy Job ([`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml), `providerParticipant`) runs the consent-manager image to (1) find-or-create the backing TMForum organization at the provider (so the `selfDescriptionURL` is stable across runs), (2) register the participant via the consent-manager's **real `POST /v1/participants` API** (idempotent: 201 new / 409 exists - no direct Mongo write), and (3) provision its jwt-auth consumer. The participant is now a **deploy artifact**, not a per-run side effect of the grant flow. Verified live: the Job registers the org/participant/consumer and the plugin authenticates as it (grant → 200). `clientID`/`clientSecret` are config (`clientSecret` from the `provider-participant-credentials` Secret) and must match the provider plugin's credentials.
  - **Remaining:** a *fully fixed* `selfDescriptionURL` (independent of the TMForum-generated org id) needs the consent-facade to serve a well-known provider SD - folded into the facade projection (next bullet). Today the SD is stable-by-find-or-create, not a fixed constant.
- **Complete the consent-facade** TMForum → Prometheus-X projection (agreements/bilaterals/service-offerings/data-resources); it is currently a scaffold and the consent-manager derives its notices/consents from it (`UF-10`, see [Contract Facade](#contract-facade) and the facade repo's `REQUIREMENTS.md`).

### 4. Enforcement architecture (P1)
- **Done — canonical enforcement path chosen: the APISIX consent-filter plugin.** The odrl-pap `consent:hasValidConsent` PIP path was never wired (empty `files/consent-pip/`, no template/registration job) and has been removed - the `enforcement` values block is gone, its only real output (the shared `consentKey`) is relocated to `consentManagement.consentKey`. odrl-pap/OPA authorizes the *credential*; the plugin decides *consent*. (The `UF-2` two-call shape stays as-is in the plugin.)
- **Kept in `post-resp` (deliberate):** the check stays in the response phase, not `pre-req` - a personal-data read can return entities for several data subjects, so gating on the response allows a per-subject decision in the body (a `pre-req` block can only see the caller's token). Personal-data routes are **fail-closed** (`fail_open: false` on `mp-data-service-consent`).
- **Per-order wiring.** In a full deployment `contract-management` should create the service, its policy **and** the protected route together per product order; the `mp-data-service-consent` route is static/manual today.
- **Flag what is personal.** Define which entity types/attributes are subject to consent (see [Arch](#arch)) so the gate knows what to enforce (and, if field-level consent is wanted, what to filter).

### 5. Secrets & configuration (P0)
- **Done:** no consent secret sits in an APISIX route conf anymore. (1) The authority facade's consent key is a `$env://CONSENT_KEY` **secret ref** from `consent-manager-secret` (APISIX resolves `$env://` inside `proxy-rewrite` headers). (2) The provider plugin's `consent_key` is gone (the facade injects it). (3) The participant `client_secret` moved out of the `mp-data-service-consent` route conf into env `CONSENT_CLIENT_SECRET`. Note two APISIX quirks that shaped this: `ext-plugin` conf has no `$env://` resolution (unlike `proxy-rewrite`), *and* APISIX spawns the runner with a fixed env, so the container env doesn't reach it - the `consent-plugin-credentials` Secret is mounted as a file and the runner launch command exports it before `exec`ing the runner.
- **Done (external sourcing):** all three consent Secrets can be provided by an external store (Vault / external-secrets) instead of the git literals - set `consentManagement.consentManager.secret.generate: false` (consent-manager-secret) and `consentManagement.pluginCredentials.create: false` / `consentManagement.providerParticipant.credentialsSecretCreate: false` (the credential Secrets), then supply Secrets of the same name/keys externally. The literals remain the **demo** default (`demo` / `changeme-consent-key`).
- **Remaining (ops):** wire the actual external-secrets/Vault sync and rotation in a real deployment, and use non-demo credentials - a deployment/operations task, not chart code.

### 6. Packaging & release (P1) — `UF-11`, `UF-12`, `UF-14`
- **Publish official, version-pinned images** for consent-manager, consent-facade and consent-plugin (today: locally built / `quay.io/wi_stefan/*:0.0.1`, `imagePullPolicy: Always`), with the strict pod `securityContext` and `Secret`-based config (`UF-11`).
- **Done (MongoDB):** the managed mongo is a `MongoDBCommunity` **ReplicaSet** (`type: ReplicaSet`, single member by default), which satisfies the consent-manager's replica-set requirement (`UF-14`) - the CM connects with `?replicaSet=mongodb`. Bump `members` to 3 for production. Standalone is not offered (the CM needs a replica set for transactions/change streams).
- **`contract-agent` dependency**: the paywalled gitpkg dep is repointed to `VisionsOfficial/contract-consent-agent` in the DSC Dockerfile (`UF-12`) — upstream this properly.

### 7. Receipts, audit & compliance (P1/P2) — `UF-4..UF-9`, `UF-13`
- **Fix the ISO/IEC TS 27560 receipt path**: DPV serialization (`UF-4`), `eventState` taxonomy (`UF-7`), remove dead Kantara code (`UF-8`), fix the broken `toReceipt()` (`UF-9`).
- **Clarify the inverted `getUserConsents` `receipt=true` semantics** the plugin relies on to read raw `status` (`UF-13`).
- **Done (access audit log):** the consent-filter plugin emits an **access-decision audit event** per request to the OpenTelemetry Collector (OTLP/HTTP log record, marked `service.name=consent-access-audit`), async + best-effort, routed to a dedicated `logs/audit` pipeline/sink separate from traces (see [Access audit log](#access-audit-log)). Config: `audit_enabled` / `audit_otlp_endpoint` on the route. **Remaining:** the durable/immutable sink itself (WORM / OpenSearch / immudb) is a deployment choice, not baked in; and the **consent-record receipt** persistence/exposure (TS 27560, above) is separate and upstream.

### 8. Plugin hardening & tests (P2)
- **Done:** the mutex-held-across-HTTP login is gone. The token/SD cache is keyed per participant with a per-entry lock; the global map lock is held only to get-or-create the entry, so a token refresh for one participant no longer serializes requests for others, and concurrent first requests for the same participant coalesce onto a single login (double-checked). Covered by a `-race` concurrency test (20 goroutines → 1 login / 1 `/me`).
- **Done:** removed the now-dead `internal/filter` package (superseded by the coarse allow/deny gate).
- **Done (runnable e2e check):** [`doc/scripts/verify_consent_flow.sh`](scripts/verify_consent_flow.sh) drives the full path (issue OID4VP token → grant → assert **200** → revoke → assert **403** → re-grant → assert **200**) and exits non-zero on the first failed assertion, so it can gate CI or serve as a smoke test. Verified live (3/3).
  - **Remaining:** fold it into the `it/` Cucumber suite so it auto-runs in the `-Pconsent` pipeline (incl. `cert/` holder bootstrap), instead of being invoked manually.