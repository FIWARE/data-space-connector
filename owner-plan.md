# Plan — Contract-Driven Data-Owner & Resource Resolution for Consent Enforcement

## Motivation

The APISIX **consent-filter** plugin used to derive the data subject from the
**caller's** access-token `sub`. That conflates *requestor* with *data owner*: it
only worked because, in the demo, the consumer and the subject were the same DID.

**The requestor has nothing to do with the data owner.** Ownership is resolved
from the *data*, and the governing *resource* from the *contract* — never from who
is asking.

## Invariant

The plugin delegates two questions about the data flowing through it to an
external **OwnerResolver**:

- **(a)** does this data require a consent check?
- **(b)** who is the data owner (the subject)?

The consent decision is then per **(owner × dataResource)**.

**The one legitimate use of the requestor's identity** is identifying *which
contract* governs the exchange (a contract is provider↔consumer by definition).
That is a different question from ownership, and the split must stay explicit:

| derived from | may determine | must never determine |
|---|---|---|
| the data (`dataOwner`) | the **owner** | — |
| the contract (provider + consumer + request URI) | the **resource** | the owner |
| the caller's token | the **consumer** (for contract lookup only) | the owner |

## Architecture

```
                    request (path, method)          data payload + parties
  consumer ─▶ APISIX ─▶ upstream ─▶ [consent-plugin]
                                         │  1. POST /resolve                          ┌────────────────┐
                                         ├──────────────────────────────────────────▶ │  OwnerResolver │
                                         │  ◀── { consentRequired, claims[] } ─────────│  (provider-side)│
                                         │                                             └───────┬────────┘
                                         │                                    contract lookup  │
                                         │                                    ┌────────────────▼────────┐
                                         │                                    │ consent-facade (local)  │
                                         │                                    │ /verify, /bilaterals    │
                                         │                                    └─────────────────────────┘
                                         │  2. per (owner × resource): consent check   ┌────────────────┐
                                         ├───────────────────────────────────────────▶ │ consent-manager│
                                         ▼  ◀── { granted } ─────────────────────────── │    (PDI)       │
                                 allow / deny                                          └────────────────┘
```

Responsibilities:

- **consent-plugin** — intercepts the response, orchestrates, enforces allow/deny.
  Knows nothing about how ownership or resources are determined.
- **OwnerResolver** — the only component that inspects the data and (with the
  contract) answers (a) + (b). Provider-side, stateless, swappable.
- **consent-facade (provider-local instance)** — projects the provider's own
  TMForum agreements into contracts/catalog. A local instance avoids opening the
  authority facade's NetworkPolicy to the provider namespace.
- **consent-manager (PDI)** — given an owner + resource, returns the decision.

## Delivered (live and verified)

- **`consent-owner-resolver`** (`/home/stefanw/git/wistefan/consent-owner-resolver`,
  image `quay.io/seamware/consent-owner-resolver:0.0.7`) — stateless Go service,
  `POST /resolve` + `/health`, format-agnostic matchers (`json` field-pointer,
  `path` regex for opaque files, `static`), JSON config, non-root image, unit tests.
- **consent-plugin** (`:0.0.2`) — `internal/ownerresolver` client; config
  `owner_resolver_url`, `owner_resolver_timeout`, `service`, `consent_api_host`;
  `evaluateWithResolver` reads the upstream body, calls `/resolve`, passes through
  when `!consentRequired`, else enforces **`deny_all`** over the distinct
  `(owner, dataResource)` claims. Resource-scoped consent match against
  `consent.data[].resource`. Legacy JWT-subject path only when no resolver is set.
- **consent-manager** (`:0.0.5`) — null-guard so a subject with no consumer-side
  `UserIdentifier` can consent (`consumerUserIdentifier` is optional per the model).
- **DSC wiring** — `k3s/consent-provider.yaml` deploys the resolver (ConfigMap +
  Deployment + Service, strict securityContext); the consent-filter route is wired
  on **both** `ext-plugin-pre-req` and `ext-plugin-post-resp` (required: the
  request phase stores the context) with `consent_api_host` (the authority routes
  are host-scoped) and `owner_resolver_url`.
- **Live state:** enforcement runs **owner-level** (resolver emits `ownerId` only).
  Verified end-to-end: grant → data access `200`, revoke → `403`.

## Contract A — OwnerResolver `POST /resolve`

```jsonc
{
  "resource": {
    "service":     "mp-data-service",
    "method":      "GET",
    "path":        "/ngsi-ld/v1/entities/urn:ngsi-ld:PersonalProfile:alice",
    "contentType": "application/ld+json"
  },
  "parties": {                      // for CONTRACT lookup only - never for ownership
    "consumer": "did:web:fancy-marketplace.biz"
  },
  "body": { "encoding": "json", "content": { /* payload verbatim */ } }
}
```

Response:

```jsonc
{
  "consentRequired": true,
  "scheme": "identifier",           // how ownerId is interpreted: identifier | email | did
  "claims": [
    { "selector": { "type": "json-pointer", "value": "" },
      "ownerId": "did:key:zDnae…",
      "dataResource": "…/catalog/dataresources/default~urn:ngsi-ld:product-specification:…" }
  ]
}
```

- `encoding`: `json` | `base64` | `none` (`none` ⇒ decide from `resource` alone —
  opaque files identified by path).
- `selector`: `whole` (opaque/single object) or `json-pointer` (enables per-item
  filtering later).
- `dataResource` is optional; omitted ⇒ owner-level consent (the current live mode).
- Errors: `400` undecodable payload, `422` consent may be required but the owner
  could not be determined (⇒ plugin fail policy). Never silently "no consent needed".

## Contract B — Consent decision

Per resolved claim: `POST /users/identifier/search {selfDescription, email: ownerId}`
→ `GET /consents/participants/{id}?receipt=true` → allow iff a `granted` consent
covers the claim (`dataResource ∈ consent.data[].resource`; empty ⇒ any granted
consent). Evaluated in the plugin; no consent-manager change required.

**Vocabulary fact that makes this work:** a granted consent stores the
**catalog data-resource URL** (verified live:
`…/catalog/dataresources/default~urn:ngsi-ld:product-specification:…`), because the
privacy notice is projected from the contract's service offering. Any resource id
the resolver emits must come from that same vocabulary.

## Decision algorithm

```
resolve(payload, parties) → claims[]
  group claims by ownerId                      // fetch-level dedup: 1 consent fetch per owner
  evaluate each unique (ownerId, dataResource)  // decision-level dedup
  map decisions back to every claim.selector
  enforce deny_all: any unmet claim ⇒ deny the whole response
```

Dedup is two-level and never by owner alone — one owner may have consented to some
data objects but not others (the zip-code-yes / street-name-no case).

## Phase A — resource by URI match (no static rules)  — IN PROGRESS

Replaces the resolver's hard-coded rules with the contract as the source of truth.

**Done (code, unit-tested):** steps 1, 3, 4 below.
**Open:** step 2 (provider-local facade deployment) and step 5 (demo agreement
target), then a live end-to-end run.

1. **consent-facade — stop discarding the ODRL target.** ✅ *done* — `assetTarget`
   added to the `OdrlRule` schema; `preserveAssetTarget()` copies the source target
   before `target` is normalized. 103 tests green.
   `AgreementContractMapper.retargetRules()` currently overwrites every rule's
   `target` with the service-offering URL:
   ```java
   .forEach(rule -> rule.setTarget(serviceOfferingUrl));
   ```
   because the consent-manager only reads a contract's data resources for rules
   whose target is *contained in* its `serviceOffering` (string containment,
   `REQUIREMENTS.md` §3.1) — while the EDC writes **asset URNs**. Keep that
   retargeted value for compatibility, and additionally expose the **original**
   target (e.g. `assetTarget`, or the `DataResource.representation` the API already
   reserves). The consent-manager ignores unknown fields, so this is backward
   compatible.
2. **Provider-local consent-facade instance** in the provider namespace, projecting
   the provider's own agreements. Keeps the authority facade's NetworkPolicy closed.
   *(open)*
3. **Resolver `contract` matcher:** ✅ *done* — `internal/contract` facade client +
   `contract` matcher (config `contractService{url, providerSelfDescription, timeoutMs}`,
   matcher `{type:"contract", owner, uriPointer, items, itemsIsArray}`); fails closed on
   no contract / no matching target / missing owner. Unit-tested incl. collections
   (one contract fetch per request) and the no-PII-resource case.
   - consumer ← `parties.consumer` (the VC issuer DID from the request token);
     provider ← the resolver's own participant SD
   - `GET /verify/{providerSD64}/{consumerSD64}` → the signed contract
   - contract → `policy[].permission[].target` (the preserved asset URI) and
     `serviceOffering` → `dataResources[]` → `containsPII`
   - **match: target URI == the requested data object URI** ⇒ that contract's
     data-resource id becomes `dataResource`
   - `consentRequired` ← `DataResource.containsPII` (no static flag)
   - owner ← unchanged, from the data (`dataOwner`)
   - fail closed when no signed contract matches ⇒ the plugin also enforces
     "access only under a contract"
4. **Plugin:** ✅ *done* — new `consumer_claim` config (default
   `verifiableCredential.issuer`), dotted-path claim lookup, forwarded as
   `parties.consumer` on `/resolve`; the claim root is added to the decoded claim set.
   *(open: caching contract lookups per (consumer, provider).)*
5. **Demo/agreement:** the 3b agreement must set the ODRL `target` to the data
   object URI instead of the placeholder `urn:asset:profile`. *(open)*

**Limit to accept:** a plain-URI target means one target per data object, i.e. a
contract per object (and, for per-subject data, effectively per subject). Fine for
the demo; it does not scale. Scaling needs Phase B.

## Then: Phase B — what Prometheus-X must support for real ODRL

Requirements to raise upstream (in dependency order):

1. **Target ≠ offering.** Drop the assumption that a rule's `target` is contained
   in the contract's `serviceOffering`. A rule must be able to target an **asset**
   distinct from the bundling offering, and the model must carry it verbatim (no
   retargeting in projection). This is the single change that unblocks everything
   else.
2. **Targets as asset collections, not just URIs.** Support
   `odrl:target: { @type: AssetCollection, odrl:source, odrl:refinement: [Constraint…] }`
   so one rule covers a *class* of objects instead of one URI.
3. **A constraint vocabulary the data plane can evaluate.** The DSC already uses
   and enforces these in odrl-pap/OPA, so they are the natural starting set:
   - `leftOperand`: `ngsi-ld:entityType`, `http:path`, `http:method`
   - `operator`: `odrl:eq`, `odrl:hasPart`, `http:isInPath`
   Prometheus-X needs to *carry* them; the DSC can already *evaluate* them.
4. **Consent keyed on the target.** `Consent.data[].resource` must be able to hold
   the asset target (today it holds catalog data-resource URLs derived from the
   offering), so a consent can be scoped to an asset rather than a whole offering.
5. **Privacy notices must enumerate targets**, so a subject can consent per asset —
   this is what makes true per-resource consent (zip-code yes, street-name no)
   expressible at grant time, not just at enforcement time.
6. **Backward compatibility.** Offering-level targets must remain valid, so existing
   contracts and consents keep working while the richer form is adopted.

With 1–5 in place the resolver's `contract` matcher evaluates a refinement against
the request instead of comparing a single URI, and no per-object contracts are
needed.

## Cross-cutting

- **Requestor independence** — structural: the owner comes only from the data;
  `parties.consumer` is used solely for contract lookup. Keep a test asserting no
  caller identity can reach an owner decision.
- **Fail policy** — resolver, facade or consent-manager error ⇒ `deny`
  (configurable `open`).
- **Caching** — contract lookups per (consumer, provider); resolution per
  (service, path); consent per (owner, participant). Short TTLs.
- **Trust** — resolver and facade are provider-side; `dataOwner` fields must be
  broker-set, not consumer-writable.
- **Body/size** — the plugin buffers the body; a `dataRef` variant is an option for
  very large payloads.

## Deferred

- **Owner id travels as `email`** — the plugin resolves owners via
  `/users/identifier/search {email: ownerId}`, so the stamped `dataOwner` must equal
  the value registered as `UserIdentifier.email`. Matching on `identifier` instead
  would need a consent-manager change.
- **`filter_items`** — drop/null only the unmet claims' selectors instead of denying
  the whole response. The per-claim `selector` already in Contract A exists for this;
  no contract change needed to adopt it.

## Operational notes (bite on every fresh cluster)

- Locally-built images pinned in the values are **not on quay**
  (`consent-plugin:0.0.2`, `consent-owner-resolver:0.0.7`, `consent-manager:0.0.5`):
  push them, or import them into k3s as part of the deploy.
- `ensure-ccsdb` Jobs create the verifier's config-server database, which the
  Zalando operator only provisions on initial cluster creation.
- **squid** resolves its `cache_peer` once at startup and has `never_direct allow all`;
  if it loses the DNS race it forwards nothing until restarted. An initContainer that
  waits for `traefik-loadbalancer-in.infra.svc.cluster.local` would fix it for good.
