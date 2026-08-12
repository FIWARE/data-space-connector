## Consent Management

Some data spaces require that access to personal data is backed by the **explicit consent** of the data subject, recorded in an auditable way. The Data Space Connector can deploy an optional consent-management layer based on the [Prometheus-X / Visions consent-manager](https://github.com/VisionsOfficial/consent-manager), producing [ISO/IEC TS 27560](https://www.iso.org/standard/80392.html) consent records, and enforce them at the gateway through the existing ODRL/OPA authorization stack. The full design, its blockers and the backlog of deferred fixes are documented in [CONSENT_MANAGEMENT_PLAN.md](../CONSENT_MANAGEMENT_PLAN.md).

> :warning: The consent-management integration is a **proof-of-concept** (see the `UF-1..UF-14` backlog in the plan). The shared consent key and participant token are POC-grade, the consent-manager runs **unmodified** in the trust-anchor (central authority) namespace and is kept cluster-internal (no ingress). Do not use this configuration as-is in production.

### Components

Consent management is deployed across **two namespaces** - a central **authority**
(the trust-anchor, which knows all participants; in real deployments the consent-manager
is run by such a central authority) and the **provider**.

**Trust-anchor (central authority)** - a second release of the DSC umbrella chart
named `consent-authority`, via [`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml):

* **consent-manager** (`consent-manager:3000`) - stores and serves the consent records/receipts (Node/MongoDB), on a managed MongoDB replica set (dedicated `consent` database). It points at the provider's consent-facade via `CONTRACT_SERVICE_BASE_URL` (cross-namespace, `consent-facade.provider.svc.cluster.local`).
* an internal **APISIX facade** in front of the consent-manager (route `/consent-manager/*`) that injects the shared consent key server-side and strips the `x-user-key` shortcut. Not exposed via ingress (roadmap item 1).
* a **consent secret** carrying the session/JWT/oauth secrets and the shared consent key.

**Provider**:

* **consent-facade** (`consent-facade:8080`) - a Micronaut service ([wistefan/consent-facade](https://github.com/wistefan/consent-facade)) that projects the DSC's TMForum APIs (agreements, catalog, party) into the contract-service API the consent-manager consumes: bilateral contracts, catalog self-descriptions, and participant self-descriptions at `/participants/{tmforum-org-id}` (party API). It replaces the POC node `contract-facade` (plan §10, backlog `UF-10`).
* the **consent-filter APISIX plugin** on the `mp-data-service-consent` route, which calls the authority's consent-manager (through its facade at `consent-authority-apisix-gateway.trust-anchor`) to gate access. This plugin is the **canonical (and only) consent-enforcement path** - OPA/odrl-pap still authorizes each request on the presented *credential*, but the *consent* decision is made by the plugin.

### Enabling

Consent management is **opt-in** via the dedicated `consent` maven profile. The
provider config lives in [`k3s/consent-provider.yaml`](../k3s/consent-provider.yaml)
(layered on `provider.yaml`), and the central-authority config - the consent-manager
+ its APISIX facade + a managed MongoDB, deployed as the `consent-authority` release
into the trust-anchor namespace - lives in
[`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml). The provider
overlay enables the facade only (`consentManager.enabled: false`):

```yaml
consentManagement:
  enabled: true
  consentManager:
    enabled: false   # the consent-manager runs at the trust-anchor, not the provider
```

The shared `consentKey` (`X_VISIONSTRUST_CONSENT_KEY`) is set on the authority side in [`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml) (`consentManagement.consentKey`); the consent-manager validates it and the facade route injects it, so the provider never holds it.

Deploy the consent scenario with `mvn clean deploy -Pconsent`. That profile builds
and imports the `consent-manager:local` image and deploys **both** the provider
consent-facade and the trust-anchor `consent-authority` release. A plain
`mvn clean deploy -Plocal` brings up the connector **without** consent management.

> :bulb: The consent-facade runs in the `provider` namespace; the consent-manager, its APISIX facade and MongoDB run in the `trust-anchor` namespace. Check with `kubectl -n provider get pods` (consent-facade) and `kubectl -n trust-anchor get pods` (consent-manager, `consent-authority-apisix`, `mongodb`).

### Working with consent records

The consent-manager is intentionally **not exposed via an ingress**. Reach it through a port-forward:

```shell
  kubectl -n trust-anchor port-forward svc/consent-manager 3000:3000
```

> :key: **Reproducing the plugin's two calls by hand.** The plugin obtains these itself (it logs in with the participant client credentials and reads `/participants/me`), but to run the two calls manually you need the same two values tied to the provider participant: a *participant token* - a JWT signed with the consent-manager's `JWT_SECRET_KEY` (secret `consent-manager-secret`, key `jwtSecret`) whose `sub` is the provider participant's `_id` - and the *provider self-description* URL, which the consent-facade serves at `http://consent-facade.provider.svc.cluster.local:8080/participants/{tmforum-org-id}` (cross-namespace, since the consent-manager is at the trust-anchor). Mint the token inside the pod (the participant must already exist):
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
  curl -s -X POST http://localhost:3000/v1/users/register \
    -H 'Content-Type: application/json' \
    -H "Authorization: Bearer <participantToken>" \
    -d '{ "email": "did:key:zDnaeeWrPc6G8GQEpBgLJGwPdFtHK1Mk7JbYhQyFu8waJNLBx" }'
  # -> the created UserIdentifier (attachedParticipant = the participant from the token)
```

The `email` is the holder DID; a throw-away one can be minted with the [did-helper](https://github.com/wistefan/did-helper) (`docker run -v $(pwd)/cert:/cert quay.io/wi_stefan/did-helper:0.1.1`, then read `cert/did.json`). The PDI `User` account itself is created with `POST /v1/users/signup`, and a background matcher links identifiers that share an e-mail across participants.

> :warning: **In the DSC POC nothing registers users automatically.** Identity bootstrapping - creating the provider/consumer participants and registering each holder's DID - is deferred (backlog `UF-1`/`UF-2`/`UF-3`) and the consent-manager is deployed **empty**, so out of the box `identifier/search` returns `userIdentifierExists: false`. The [`consent_grant.sh`](scripts/consent_grant.sh) helper performs this registration for you: it upserts the provider/consumer participants, a `User`, and the provider-side `UserIdentifier` (`email` = the holder DID) before recording the granted consent - which is why the demo below needs no separate registration step.

Once an identity is registered, the consent-filter plugin that gates access resolves and checks consent with a **two-call chain** against the consent-manager:

1. Resolve the DID to a `UserIdentifier` (auth: the shared consent key). `selfDescription` must be the **provider self-description** (`providerSd`) the consent-facade serves at `/participants/{tmforum-org-id}` - the value `consent_grant.sh` prints:
```shell
  curl -s -X POST http://localhost:3000/v1/users/identifier/search \
    -H 'Content-Type: application/json' \
    -H 'x-visionstrust-consent-key: changeme-consent-key' \
    -d '{
          "selfDescription": "http://consent-facade:8080/participants/urn:ngsi-ld:organization:<id>",
          "email": "did:key:zDnaeeWrPc6G8GQEpBgLJGwPdFtHK1Mk7JbYhQyFu8waJNLBx"
        }' | jq .
  # -> { "participantExists": true, "userIdentifierExists": true, "userIdentifier": "<id>", ... }
```
2. List that user's consents (auth: a provider participant JWT signed with the consent-manager's `JWT_SECRET_KEY`, `sub` = the provider participant id); the plugin allows only if some consent has `status == "granted"`:
```shell
  curl -s "http://localhost:3000/v1/consents/participants/<userIdentifier>?receipt=true" \
    -H "Authorization: Bearer <participantToken>" | jq '.consents[] | .status'
```

> :warning: `GET /consents/participants/...` **always builds a receipt** for every consent, which HTTP-fetches each participant's `selfDescriptionURL` and reads `legalPerson.legalAddress` from it. So the participants' `selfDescriptionURL` **must** resolve to a valid self-description - the consent-facade serves these at `/participants/{tmforum-org-id}` (backed by the party API), which is why `consent_grant.sh` creates a real TMForum organization per participant. A participant whose SD URL 404s makes this call return `500`, which the plugin treats as "no consent".

The full give-consent flow additionally needs a bootstrapped privacy notice (backlog `UF-1..UF-14`). For the demo, grant and revoke consent with the [`consent_grant.sh`](scripts/consent_grant.sh) / [`consent_revoke.sh`](scripts/consent_revoke.sh) helper scripts below, which seed this state (including the identity registration above) and verify it live.

### Enforcing consent on a concrete service

Consent is enforced on the data path by the **APISIX consent-filter plugin** - the canonical, only enforcement path (used by the [demo below](#demo-consent-gated-access-to-personal-data)). A custom external plugin attached to the `mp-data-service-consent` route runs the **two-call consent check** against the consent-manager on every request - resolve the subject's `userIdentifier` (`POST /v1/users/identifier/search`, authenticated with the `consent_key`), then list its consents (`GET /v1/consents/participants/{id}`, authenticated with the participant token) - and blocks unless a granted consent exists for the credential subject. OPA still authorizes the request on the *credential* first; the plugin adds the *consent* gate on top, keeping the access policy free of consent logic. The plugin authenticates with participant **client credentials** — it reads `client_id`/`client_secret` (the `consent_key` is injected by the facade), logs in via `/participants/login` for a (refreshing) participant token, and derives the provider self-description from `/participants/me`. Those credentials are **stable** (`consent-demo-provider`/`demo`, created by `consent_grant.sh`), so no per-seed value is wired into the plugin.

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

> :bulb: Step **1** (publishing data) and step **3** (grant/verify/revoke consent, via the scripts) are verified against the live cluster. Steps **0/2/4** exercise the OPA-allow + consent-plugin path; the plugin performs the two-call check against the consent-manager, and the user-bootstrap flow it relies on is POC-grade (`UF-1..UF-14`).

**Prerequisites.** Deploy the data space with consent management enabled (`mvn clean deploy -Pconsent`, see [Enabling](#enabling)). All commands below are run from the repository root. The grant/revoke helpers talk to the cluster through `kubectl`, so point `KUBECONFIG` at the local cluster:

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

**3. The subject grants consent.** Consent is tied to the DID carried in the consumer's credential (the plugin reads `credentialSubject.id`). Extract it:

```shell
  export SUBJECT_DID=$(echo "${USER_CREDENTIAL}" | cut -d. -f2 | base64 -d 2>/dev/null | jq -r '.vc.credentialSubject.id // .credentialSubject.id'); echo ${SUBJECT_DID}
```

The consent-manager is deployed empty and the full give-consent API needs a bootstrapped user + privacy notice (backlog `UF-1..UF-14`), so the demo records the consent with a helper that seeds the minimal state the plugin needs - a provider/consumer participant, a user, a provider `UserIdentifier` whose `email` is `${SUBJECT_DID}`, and a **granted** `Consent` - directly through the consent-manager pod, then verifies it with the exact two calls the plugin makes:

```shell
  ./doc/scripts/consent_grant.sh ${SUBJECT_DID}
```

```
granted consent for did:key:...
  provider org (TMForum): urn:ngsi-ld:organization:<id>
  jwt-auth consumers (authority apisix): M_P_Operations_Inc_, Fancy_Marketplace_Co_
  POST /v1/users/identifier/search      -> HTTP 200  (userIdentifier 6a71e3567917ddaef2e2c866)
  GET  /v1/consents/participants/<id>   -> HTTP 200  statuses=["granted"]
  (the consent-filter plugin authenticates with clientID consent-demo-provider / clientSecret demo and derives the provider SD from /participants/me)
```

The output verifies the exact two calls the plugin makes against the consent-manager and provisions the participants' jwt-auth consumers at the authority facade - no extra wiring is needed, the plugin logs in with the stable client credentials.

The **consent-filter plugin** (the path this demo exercises) does **not** need them: it's configured with the stable participant **client credentials** (`client_id`/`client_secret`) already in the `mp-data-service-consent` route conf in [`k3s/provider.yaml`](../k3s/provider.yaml), logs in for a token, and derives the provider SD from `/participants/me`. So a reseed needs no plugin re-wiring — just re-run `consent_grant.sh` (it recreates the same participant + client credentials).

**4. The consumer requests again &rarr; access allowed.** With a granted consent in place the plugin now lets the request through, and the identical call succeeds:

```shell
  export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh https://mp-data-service.127.0.0.1.nip.io $USER_CREDENTIAL default)
  curl -k -x localhost:8888 -s -w '\nHTTP %{http_code}\n' \
    -X GET 'https://mp-data-service-consent.127.0.0.1.nip.io/ngsi-ld/v1/entities/urn:ngsi-ld:PersonalProfile:alice' \
    -H "Authorization: Bearer ${ACCESS_TOKEN}"
  # -> the entity + HTTP 200
```

Withdraw the consent again with `./doc/scripts/consent_revoke.sh ${SUBJECT_DID}` - it sets the record's `status` to `revoked`, so the plugin stops allowing and the very next request returns `403` again. Access follows the subject's consent in real time.

> :bulb: To run this whole check in one shot, use [`./doc/scripts/verify_consent_flow.sh`](scripts/verify_consent_flow.sh) - it issues the token, then grants → asserts `200`, revokes → asserts `403`, re-grants → asserts `200`, and exits non-zero on any failure. Needs the `cert/` holder identity from the prerequisites above.


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
  - **Done (config-only hardening):** the facade `proxy-rewrite` **injects** the `x-visionstrust-consent-key` server-side (so callers no longer hold it) and **strips** the `x-user-key` shortcut header; the consent-filter plugin's `consent_api_url` points at `http://consent-authority-apisix-gateway.trust-anchor.svc.cluster.local/consent-manager`. The facade's APISIX is internal (ingress off), so it is not directly reachable from outside.
  - **Done (participant-JWT validation) — verified live:** the facade requires a valid **participant JWT** on every call except `/participants/login` (APISIX `jwt-auth`, keyed on the token's `participant_name` via a consumer whose HS256 secret is the consent-manager's `JWT_SECRET_KEY`); the plugin now sends the token on call 1 too. Verified end-to-end: missing/tampered token → **401** at the facade (before the consent-manager), login stays open, and the full grant→**200** / revoke→**403** flow works. Baked into [`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml) (login-exempt route + `jwt-auth` on the facade route + a consumer-reconcile Job, one consumer per participant).
  - **Done (secret ref):** the consent key injected by the facade is sourced from `consent-manager-secret` (mounted as env `CONSENT_KEY`, referenced in the route header as `$env://CONSENT_KEY`, resolved by APISIX at runtime) - no literal in git or etcd. Verified live.
  - **Done (credentials out of the route conf):** the plugin's `consent_key` is now **optional** (the facade injects it and overrides anything sent), so it is dropped from the provider route conf; the participant `client_secret` is no longer in the route conf either - the plugin reads it from env `CONSENT_CLIENT_SECRET`. APISIX spawns the ext-plugin runner with a *fixed* environment, so the container env never reaches it: the `consent-plugin-credentials` Secret is instead mounted as a file and the runner launch command `export`s it before `exec`ing the runner. `client_id` (not secret) stays in the conf. Verified live.
  - **Done (consumer per participant):** the authority provisions a jwt-auth **consumer per participant**, not one hardcoded consumer. Two paths, both keyed on the participant's `legalName` (the token's `participant_name`) and signed with the shared `JWT_SECRET_KEY`: (a) `consent_grant.sh` registers the consumer for each participant it registers ("provision when a participant registers"); (b) the deploy **reconcile Job** ([`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml)) runs the consent-manager image to enumerate *all* participants and PUT a consumer for each - idempotent, so it also re-syncs after an apisix/etcd reset (empty/no-op on a fresh deploy).
  - **Not applicable — no NetworkPolicy:** in a real deployment the consent-manager and its gateway are run by the **central authority in a separate cluster**, reached over the network (with the participant token) - they are not co-located with the provider, so a same-cluster lockdown is meaningless. The in-cluster trust-anchor deployment here is only a PoC stand-in for that external authority.
- **Identity = the access-token `sub`, taken as-is (accepted for the PoC).** The plugin uses the token's `sub` verbatim as the consent-manager identifier (stored in `UserIdentifier.email`), and the consent is seeded for the same value - so **no consent-manager change is needed** and it works whether `sub` is a `did:key:…` or a real e-mail. Revisiting this - reconciling on the `identifier` field so a DID and a real e-mail can coexist (`UF-3`), and setting the receipt `piiPrincipalId` to the DID rather than a Mongo ObjectId (`UF-5`) - *would* require consent-manager changes (the lookup, the matcher and the model all key on `email`), so it is **deferred/upstream**, not a DSC task.
- **Consistent `sub`**: the value the consent is seeded for must equal the access-token `sub` the verifier issues. The demo seeds for the token's `sub` directly (`consent_grant.sh <sub>`); a real deployment needs the verifier ↔ credential ↔ consent-manager to agree on one canonical subject value.

### 2. Consent lifecycle & data-subject UX (P0/P1)
- **Real give/withdraw flow.** Today `consent_grant.sh` / `consent_revoke.sh` write Mongo directly (participant, user, `UserIdentifier`, `Consent`). Replace with the consent-manager's real give-consent API driven by privacy notices tied to contracts, and expose the **PDI consent UI** (`/consents/pdi/iframe`) to data subjects over **ingress + TLS + auth** (it is ClusterIP-only today, `UF-1`).
- **Consent expiry** (`UF-6`): honor `validityDuration`, compute expiry, transition `status → expired`; the plugin must then also reject expired (it currently checks only `status == "granted"`). Blocked upstream: the `Consent` model has an `"expired"` status but no `validityDuration`/expiry timestamp, so expiry would have to come from the privacy notice (the facade flow).

### 3. Contract projection & participant registration (P1)
- **Complete the consent-facade** TMForum → Prometheus-X projection (agreements/bilaterals/service-offerings/data-resources); it is currently a scaffold and the consent-manager derives its notices/consents from it (`UF-10`, see [Contract Facade](#contract-facade) and the facade repo's `REQUIREMENTS.md`).
- **Done — register the provider participant at deploy time.** A deploy Job ([`k3s/consent-trust-anchor.yaml`](../k3s/consent-trust-anchor.yaml), `providerParticipant`) runs the consent-manager image to (1) find-or-create the backing TMForum organization at the provider (so the `selfDescriptionURL` is stable across runs), (2) register the participant via the consent-manager's **real `POST /v1/participants` API** (idempotent: 201 new / 409 exists - no direct Mongo write), and (3) provision its jwt-auth consumer. The participant is now a **deploy artifact**, not a per-run side effect of `consent_grant.sh` (which stays idempotent). Verified live: the Job registers the org/participant/consumer and the plugin authenticates as it (grant → 200). `clientID`/`clientSecret` are config (`clientSecret` from the `provider-participant-credentials` Secret) and must match the provider plugin's credentials.
  - **Remaining:** a *fully fixed* `selfDescriptionURL` (independent of the TMForum-generated org id) needs the consent-facade to serve a well-known provider SD - folded into the facade projection (next bullet). Today the SD is stable-by-find-or-create, not a fixed constant.
- **Complete the consent-facade** TMForum → Prometheus-X projection (agreements/bilaterals/service-offerings/data-resources); it is currently a scaffold and the consent-manager derives its notices/consents from it (`UF-10`, see [Contract Facade](#contract-facade) and the facade repo's `REQUIREMENTS.md`).

### 4. Enforcement architecture (P1)
- **Done — canonical enforcement path chosen: the APISIX consent-filter plugin.** The odrl-pap `consent:hasValidConsent` PIP path was never wired (empty `files/consent-pip/`, no template/registration job) and has been removed - the `enforcement` values block is gone, its only real output (the shared `consentKey`) is relocated to `consentManagement.consentKey`. odrl-pap/OPA authorizes the *credential*; the plugin decides *consent*. (The `UF-2` two-call shape stays as-is in the plugin.)
- **Kept in `post-resp` (deliberate):** the check stays in the response phase, not `pre-req` - a personal-data read can return entities for several data subjects, so gating on the response allows a per-subject decision in the body (a `pre-req` block can only see the caller's token). Personal-data routes are **fail-closed** (`fail_open: false` on `mp-data-service-consent`).
- **Per-order wiring.** In a full deployment `contract-management` should create the service, its policy **and** the protected route together per product order; the `mp-data-service-consent` route is static/manual today.
- **Flag what is personal.** Define which entity types/attributes are subject to consent (see [Arch](#arch)) so the gate knows what to enforce (and, if field-level consent is wanted, what to filter).

### 5. Secrets & configuration (P0)
- **Done:** no consent secret sits in an APISIX route conf anymore. (1) The authority facade's consent key is a `$env://CONSENT_KEY` **secret ref** from `consent-manager-secret` (APISIX resolves `$env://` inside `proxy-rewrite` headers). (2) The provider plugin's `consent_key` is gone (the facade injects it). (3) The participant `client_secret` moved out of the `mp-data-service-consent` route conf into env `CONSENT_CLIENT_SECRET`. Note two APISIX quirks that shaped this: `ext-plugin` conf has no `$env://` resolution (unlike `proxy-rewrite`), *and* APISIX spawns the runner with a fixed env, so the container env doesn't reach it - the `consent-plugin-credentials` Secret is mounted as a file and the runner launch command exports it before `exec`ing the runner.
- **Remaining:** the Secret *values* themselves (`consentKey`, `clientSecret`) are still literal placeholders in git/values - source them from Vault / an external secret store, use non-demo credentials (`consent-demo-provider`/`demo` are demo values), and support rotation.

### 6. Packaging & release (P1) — `UF-11`, `UF-12`, `UF-14`
- **Publish official, version-pinned images** for consent-manager, consent-facade and consent-plugin (today: locally built / `quay.io/wi_stefan/*:0.0.1`, `imagePullPolicy: Always`), with the strict pod `securityContext` and `Secret`-based config (`UF-11`).
- **MongoDB**: a replica set is required (`UF-14`); document/optionally support standalone for dev.
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