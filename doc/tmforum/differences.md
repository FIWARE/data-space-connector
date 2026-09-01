# Divergences and incompatibilities

The components do not share a single interpretation of the TMForum model. This page lists every
divergence found in the code, classified by how bad it is:

(The table below covers *shape* divergences. Entities and fields where the producers and consumers do
not line up at all are a different class of problem and are collected in the
[appendix](#appendix-producer-and-consumer-gaps).)

| Severity | Meaning |
|---|---|
| 🟥 **Incompatible** | The two interpretations cannot coexist on the same objects. One of the two must be disabled, or the data must be authored for exactly one of them. |
| 🟧 **Conditionally compatible** | Works only because of a specific configuration value or a lenient comparison; changing an unrelated default breaks it silently. |
| 🟨 **Inconsistent** | Same concept modelled differently; costs correctness only in edge cases, but is a permanent trap for authors. |

Summary:

| # | Divergence | Severity |
|---|---|---|
| 1 | The characteristic discriminator: `id` vs. `name` vs. `valueType` | 🟨 |
| 2 | `relatedParty` roles | 🟧 |
| 3 | BAE rewrites what it proxies | 🟥 |
| 4 | Participant identity: two places for the DID | 🟥 |
| 5 | Consent depends on an `Agreement` only the EDC produces | 🟧 |
| 6 | ODRL: one rule, two authoring slots, two encodings | 🟨 |
| 7 | `ProductOrder`: three incompatible minimum shapes | 🟧 |
| 8 | Null-tolerance: what crashes on a well-formed TMForum object | 🟨 |
| 9 | One `@schemaLocation` slot per object | 🟨 |
| 10 | Pagination and lookup limits | 🟨 |

---

## 1. The characteristic discriminator

**🟨 Inconsistent.**

A `ProductSpecificationCharacteristic` has three identifying fields — `id`, `name`, `valueType` — and
the DSC uses **all three** as the semantic key, depending on which code path reads it.

| Consumer | Discriminator | Characteristics |
|---|---|---|
| contract-management (both resolvers) | `valueType` | `credentialsConfiguration`, `authorizationPolicy` |
| fdsc-edc `TMFEdcMapper` | `valueType` | `endpointUrl`, `endpointDescription`, `transferType`, `transferPath` |
| fdsc-edc `TMFEdcMapper` (same method!) | **`id`** | `upstreamAddress` |
| fdsc-edc transfer provisioners | **`id`** | `upstreamAddress`, `targetSpecification`, `serviceConfiguration` |
| consent-facade | **`name`** | `purpose` |
| BAE | **`name`**, case-insensitive | `asset type`, `media type`, `location` |

The mix inside `TMFEdcMapper.assetFromProductSpec` is the clearest illustration: it checks for the
presence of `upstreamAddress` by scanning `ProductSpecificationCharacteristic::getId`, then reads
`endpointUrl`/`endpointDescription`/`transferPath` by `switch (spec.getValueType())`.

On top of that, `valueType` is being **repurposed**. In TMForum it denotes the *data type* of the
value (`String`, `Number`) — which is how BAE uses it (`lib/auth.js`: `valueType: 'String'`) — while
the DSC uses it as a semantic tag (`valueType: "endpointUrl"`). The consent demo even writes
`valueType: "object"`, i.e. the TMForum meaning, for a characteristic identified by `name`.

**Consequence.** There is no single payload shape that is *guaranteed* to be read by all consumers,
and a characteristic authored through a UI that fills `valueType` with a data type disappears from
contract-management's and fdsc-edc's view.

**Mitigation (what the DSC examples do, and what you should do):** set `id`, `name` **and**
`valueType`, with `id == valueType ==` the semantic key:

```jsonc
{ "id": "upstreamAddress",
  "name": "Address of the upstream serving the data",
  "valueType": "upstreamAddress",
  "productSpecCharacteristicValue": [ { "value": "data-service-scorpio:9090", "isDefault": true } ] }
```

**Fix direction.** Standardise on one discriminator (`valueType`, since two JSON schemas already pin
it with `const`), and make every reader fall back to the other two.

---

## 2. `relatedParty` roles

**🟧 Conditionally compatible.**

Every component invents its own role vocabulary for the same relationships, and matching is sometimes
case-sensitive, sometimes not, sometimes configurable.

| Object | contract-management | fdsc-edc | BAE | consent-facade |
|---|---|---|---|---|
| `ProductSpecification.relatedParty` | reads `provider` — configurable `general.organization.provider.role`, `equalsIgnoreCase` | – | writes `Seller` (`BAE_LP_OAUTH2_SELLER_ROLE`, DSC value `seller`) + `SellerOperator` | – |
| `ProductOffering.relatedParty` | – | – | writes `Seller`/`SellerOperator` (+ `Buyer`/`BuyerOperator`) | – |
| `Quote.relatedParty` | – | writes+reads `Provider`, `Consumer` — hard-coded | – | – |
| `ProductOrder.relatedParty` | reads `Customer` — configurable `general.productOrder.customerRole`, `equalsIgnoreCase` | writes `Consumer` **and** `Customer` (+ `Provider`) | writes `Buyer` upstream / `customer` in the DSC (`config.roles.customer`) + `Seller` + operators | – |
| `Agreement.engagedParty` | – | writes `Provider`, `Consumer` — hard-coded | – | reads `Provider`, `Consumer` — hard-coded, `Objects.equals` |
| `Product.relatedParty` | – | writes `Provider`, `Consumer` | – | – |

### The two live traps

**a) BAE's customer role.** contract-management needs `Customer` (case-insensitively). BAE's
*upstream* default is **`Buyer`** (`config.js`: `config.roles.customer = 'Buyer'`). The DSC's
`business-api-ecosystem` chart sets it from `oauth.customerrole`, whose default is `customer` — which
*does* match case-insensitively. So marketplace orders activate **only because
`oauth.customerrole` happens to be `customer`.** Setting it to anything else (or running BAE with its
own defaults) makes every order fail with
`Exactly one ordering related party is expected.`

**b) BAE's seller role vs. contract-management's provider role.** contract-management looks for role
`provider` on the `ProductSpecification` to find that party's `contractManagement` characteristic;
BAE writes `seller`. So a specification authored through BAE never resolves a remote
contract-management, and the **central-marketplace delegation silently degrades to local handling**
(`ContractManagement(local = true)`). No error is raised — the configuration is just ignored.

**Fix direction.** One role vocabulary per relationship, declared in one place, with case-insensitive
matching and multi-role acceptance (`provider | Provider | Seller` all meaning "the party offering
this", `Customer | Buyer | Consumer` all meaning "the party acquiring it").

---

## 3. BAE rewrites what it proxies

**🟥 Incompatible** for the DSP path.

BAE is not a transparent proxy. On every POST/PATCH it passes through, `attachRelatedParty` mutates
the body (`lib/tmfUtils.js`):

**a) `relatedParty` is replaced, not merged.** `attachPartySpec` and `attachParty` both start with
`body.relatedParty = []` and then push their own entries. Any role the DSC components rely on
(`provider`, `Provider`, `Consumer`) is **discarded**.

**b) `@schemaLocation` is overwritten on `ProductOffering`.**

```js
const attachOfferingParty = async function(req, body) {
    const schemaFunction = () => { body['@schemaLocation'] = config.offeringSchema; }   // DOME ExternallyBilled
    await attachParty(req, body, schemaFunction)
}
```

`config.offeringSchema` points at the DOME `ExternallyBilled.schema.json`. Since a
`ProductOffering` has exactly one `@schemaLocation` slot, and `tm-forum-api` validates *all* unknown
properties against it, an offering created or updated through BAE **cannot carry the EDC
`externalId`**: either the DOME schema rejects it, or (if the schema is permissive) the semantic link
to `external-id.json` is lost. The same applies to any future offering-level extension.

`productOfferingTerm[name="edc:contractDefinition"]` survives, because the term carries its *own*
`@schemaLocation` — but an offering without `externalId` is skipped by
`TMFEdcMapper.fromProductOffer`, so the contract definition is never seen either.

`ProductSpecification` and `Catalog` are safer: `attachPartySpec` does not touch `@schemaLocation`, so
a spec can keep `external-id.json` + `externalId` — but its `relatedParty` is still replaced.

**c) Additional BAE-only requirements** that DSP-authored objects do not satisfy: `billingAccount.id`
on orders (422), `productOrderItem[].product` and `.productOffering` on every item, a
`productSpecification` on non-bundle offerings, lifecycle progression through `Active` → `Launched`
with dependency checks.

**Practical rule today.** DSP-negotiable offerings must be authored **directly against the TMForum
API**, not through BAE. The DSC docs reflect this: `doc/DSP_INTEGRATION.md` uses raw `curl` against
`tm-forum-api`, while `doc/MARKETPLACE_INTEGRATION.md` (the BAE flow) demonstrates the
credential/policy activation path only — no `externalId`, no contract-definition term.

**Fix direction.** Make BAE *merge* rather than replace (`relatedParty`), and make the schema
injection additive/skippable (e.g. only set `@schemaLocation` when the body has none), or move BAE's
DOME-specific extensions into a nested object that carries its own schema location.

---

## 4. Participant identity: two places for the DID

**🟥 Incompatible** between BAE-created and API-created organizations.

| Writer | Where the DID ends up |
|---|---|
| DSC demo flows / provider onboarding | `partyCharacteristic[name="did"].value` |
| fdsc-edc `ParticipantResolver.createOrganization` | `partyCharacteristic[name="did"].value` (and nothing else — no `name`) |
| BAE login (`lib/auth.js` → `buildOrganization`) | `tradingName` **and** `externalReference[externalReferenceType="idm_id"].name` — the credential **issuer** DID for VC logins |

| Reader | Where it looks |
|---|---|
| contract-management `OrganizationResolver` | `externalReference[idm_id].name` first (validated as `did:*:*`), then `partyCharacteristic[did]` — **handles both** |
| fdsc-edc `ParticipantResolver` / `OrganizationApiClient` | `partyCharacteristic[did]` **only** |
| consent-facade | `partyCharacteristic[<facade.party.did-characteristic>]`, default `did` — **only** |

### The failure mode

fdsc-edc resolves DID → TMForum id with
`organizationApi.getByDid(did).orElseGet(() -> createOrganization(did))`. Given a BAE-created
organization for the same participant, `getByDid` does not find it (no `did` characteristic) and
fdsc-edc **creates a second `Organization` for the same DID**. From then on:

* the marketplace's orders reference organization A, the EDC's quotes/agreements/products reference
  organization B;
* `ContractNegotiation` typing depends on resolving the DID back from the quote's related parties
  (`getDidFromOrganization`) — for organization A that returns empty and the participant is logged as
  *"does not have a did"*, leaving the negotiation without a counter-party;
* the consent-facade cannot produce a self-description with a `did` for organization A, so the
  consent-manager cannot resolve that participant.

fdsc-edc-created organizations have the mirror problem: they carry only the `did` characteristic, so
`legalName` is `null` in the consent self-description and BAE has no `externalReference` to key on.

**Fix direction.** One canonical location (`partyCharacteristic[name="did"]`), written by *every*
producer including BAE, with `externalReference[idm_id]` kept as a read-only fallback; plus a
DID-uniqueness guard so a duplicate cannot be created.

---

## 5. Consent depends on an `Agreement` only the EDC produces

**🟧 Conditionally compatible.**

`Agreement` has exactly one writer — fdsc-edc — and the consent-facade is built on precisely that
shape: it reads the `provider-id`, `consumer-id`, `policy`, `asset-id` and `signing-date`
characteristics, the `Provider`/`Consumer` engaged parties, and resolves the catalog graph through
`agreementItem[].productOffering[]` / `agreementItem[].product[]`.

The consequence is a deployment-level coupling that nothing enforces at runtime:

* A deployment that runs the **marketplace path only** (BAE checkout → `ProductOrder` completed →
  contract-management activates credentials and policies) creates **no `Agreement` at all**.
  `/bilaterals/for/{participantId}` then returns an empty list, the consent-manager builds no privacy
  notice, and the consent PIP reads the absence as "no consent" — a silent, total failure of consent
  management with no error anywhere.
* Any future second writer of `Agreement` must reproduce all five characteristics *and* the
  `Provider`/`Consumer` engaged-party roles, or the projection degrades field by field:
  `dataProvider`/`dataConsumer` become `null`, `policy[]` becomes `null` (⇒ privacy notice with no
  `data`), and `status` falls back to mapping `Agreement.status` — absent ⇒ `pending` ⇒ filtered out
  by the consent-manager's `hasSigned=true`.
* `agreementItem[].productItem[]` is resolved against the **Product Inventory API**. A writer that
  puts anything other than a `Product` id there (an order id, an offering id) produces a 404 during
  the catalog-graph walk.

This is consistent with `consent-facade/REQUIREMENTS.md` §0.1, which states the source model is the
EDC-written `Agreement`. It is worth stating explicitly in deployment terms: **consent management
requires fdsc-edc.**

**Fix direction.** Either document the dependency as a hard prerequisite in the consent chart values
(fail fast when no EDC is deployed), or give contract-management's activation path an
`Agreement`-writing step that produces the same shape, so a marketplace-only data space can support
consent too.

---

## 6. ODRL: one rule, two authoring slots, two encodings

**🟨 Inconsistent.**

The same logical rule ("only members holding an OperatorCredential may read `/entities`") has to be
authored **twice**, in two different slots, because two different enforcement points consume it — and
the machine-generated copies use a third and fourth encoding:

| # | Location | Encoding | Consumer | Purpose |
|---|---|---|---|---|
| 1 | `productSpecCharacteristic[valueType="authorizationPolicy"].…value` | compacted, `odrl:`-prefixed, with `@context` and `odrl:uid` | contract-management → ODRL-PAP → OPA | **access control** on the data service |
| 2 | `productOfferingTerm[edc:contractDefinition].accessPolicy` / `.contractPolicy` | compacted, `http://www.w3.org/ns/odrl.jsonld` context, `odrl:uid` required | fdsc-edc | catalog **visibility** / negotiable **terms** |
| 3 | `quoteItem[].policy` | expanded JSON-LD | fdsc-edc | the **negotiated** policy |
| 4 | `Agreement.characteristic[name="policy"]` | expanded JSON-LD | consent-facade | the **agreed** policy, projected into consent |

Slots 1 and 2 are both hand-authored by the provider and **nothing keeps them in sync**: the DSP
contract policy can grant terms the PAP policy does not permit, and vice versa. Slots 3 and 4 are
generated from 2 by the EDC.

Additional transformations along the way:

* fdsc-edc expands on write (`fromEdcPolicy` = transform to JSON-LD then `jsonLd.expand`) and
  compacts again for the PAP (`jsonLd.compact`).
* the transfer provisioner **overwrites `odrl:uid`** with the transfer-process id and **replaces
  `odrl:target`** with the specification's `targetSpecification` characteristic before pushing to the
  PAP.
* the consent-facade **retargets every rule** to its own service-offering URL, and forces
  `permission` and `prohibition` to be present as arrays.

Because expansion turns `odrl:permission` into
`http://www.w3.org/ns/odrl/2/permission`, a compacted policy cannot be copied into slot 3 or 4 (or
read out of them) without re-encoding.

**Fix direction.** Derive one from the other where the semantics allow it — e.g. generate the PAP
access-control policy from the offering's `contractPolicy` plus the `targetSpecification` — so the
provider authors the rule once.

---

## 7. `ProductOrder`: three incompatible minimum shapes

**🟧 Conditionally compatible.**

| Producer | Minimum shape it emits |
|---|---|
| BAE | `relatedParty[customer + sellers + operators]`, `productOrderItem[{product{relatedParty}, productOffering}]`, `billingAccount{id}`, state derived from item states |
| fdsc-edc | `relatedParty[Consumer, Customer, Provider]`, `quote[{id}]` — **no `productOrderItem`, no `billingAccount`** |
| direct API (demo) | `relatedParty[{id, role: Consumer}]`, `quote[{id}]` |

| Consumer | Minimum shape it requires |
|---|---|
| BAE validators | `relatedParty`, ≥ 1 `productOrderItem` with `product` + `productOffering`, `billingAccount.id` |
| contract-management | exactly one `relatedParty` in the customer role; either `quote[]` **or** `productOrderItem[].productOffering` to resolve the configuration |
| fdsc-edc | `quote.id` (found via `?quote.id=`), at most one order per quote (`There should only be one order per quote.`) |

An order created by fdsc-edc is therefore **invalid from BAE's point of view** (no items, no billing
account) — harmless today only because fdsc-edc writes straight to `tm-forum-api` and bypasses the
BAE proxy, but it means those orders cannot be viewed or advanced in the marketplace UI, and BAE's
seller back-office cannot be used to process a DSP-negotiated order.

Conversely, a BAE order carries no `quote[]`, so contract-management resolves the configuration from
`productOrderItem[].productOffering` — a code path that exists precisely for this case.

The role mismatch (item 2a) is what actually makes this fragile: change `oauth.customerrole` and every
BAE order stops activating.

---

## 8. Null-tolerance: what crashes on a well-formed TMForum object

**🟨 Inconsistent.** Objects that are perfectly valid TMForum break individual components:

| Object property | Tolerant | Intolerant |
|---|---|---|
| `productSpecCharacteristic[].valueType` absent | fdsc-edc (`if (spec.getValueType() == null) return;`) | contract-management — `psc.getValueType().equals(…)` ⇒ **NPE** in `CredentialsConfigResolver` and `PolicyResolver` |
| `relatedParty[].role` absent | contract-management (skips null roles; a single party with no role is accepted as the customer), fdsc-edc (logs *"Received a related party without a role."* and drops the party) | BAE — `party.role.toLowerCase()` in `hasPartyRole` / `isOrderingCustomer` ⇒ **TypeError** |
| `Quote.externalId` absent | – | fdsc-edc throws `The quote does not contain an external Id.` |
| `characteristicValue` as `{"value": …}` vs. a plain string | fdsc-edc `extractStringValue` handles both | consent-facade casts/`toString`s, contract-management casts to `String` |
| `productSpecCharacteristicValue[].isDefault` absent | fdsc-edc (`Optional.ofNullable(...).orElse(false)`), consent-facade (takes the first value) | – |
| `Agreement.characteristic` absent | consent-facade (falls back) | – |

The `valueType` NPE is the most likely to be hit in practice, because it fires on *any* specification
that has one characteristic without a `valueType` — including specifications that have nothing to do
with the DSC.

---

## 9. One `@schemaLocation` slot per object

**🟨 Inconsistent** — a structural limitation with model-level consequences.

`tm-forum-api` validates all unknown properties of an object against the single schema named in that
object's `@schemaLocation`. There is no schema composition. Therefore:

* **Two features cannot independently extend the same object.** `ProductOffering` currently needs
  `external-id.json` (fdsc-edc) and would need DOME's `ExternallyBilled.schema.json` (BAE) — see item
  3. Adding a third extension means writing a combined schema.
* **The DSC works around it by extending nested objects instead**: the contract definition lives on
  `productOfferingTerm` (own slot), the credential and policy configuration on
  `productSpecificationCharacteristic` (own slots), the negotiation state on `Quote`, the offer state
  on `quoteItem`. This is why the payloads in `doc/DSP_INTEGRATION.md` carry four different
  `@schemaLocation` values in one document.
* **Schemas must be network-reachable from the API pod at write time.** All of them currently live on
  `raw.githubusercontent.com`, and two of them point at *branches* rather than tags:
  `wistefan/edc-dsc@init` (the fdsc-edc default base URI) and
  `FIWARE/contract-management@policy-support`. A branch move silently changes validation for every
  deployment. Pin them, and mirror them for restricted networks
  (`tmfExtension.schemaBaseUri` exists for exactly this).

---

## 10. Pagination and lookup limits

**🟨 Inconsistent.**

| Lookup | Limit |
|---|---|
| consent-facade `findAgreements()` | **first 100 agreements only**, then filtered by party in memory (`REQUIREMENTS.md` §8 lists full pagination + server-side filtering as open) |
| fdsc-edc `OrganizationApiClient.getByDid` | pages until found — TMForum cannot filter a characteristic by name *and* value, so every page is filtered client-side |
| fdsc-edc `ProductCatalogApiClient.getByPolicyId` | pages `productOffering?productOfferingTerm.name=edc:contractDefinition` with `limit=100` |
| fdsc-edc `ProductOrderApiClient.findByQuoteId` | throws if more than one order references a quote |
| contract-management `getCredentialsConfig` / `getAuthorizationPolicy` | takes the **first** matching characteristic only; a second `credentialsConfiguration` characteristic on the same spec is silently ignored |

None of these are wrong per se, but they define the *effective* cardinality of the model: one
credential config and one policy config per specification, one order per quote, and — for consent — no
more than 100 agreements per provider.

---

## Appendix: producer and consumer gaps

The ten divergences above are all *shape* mismatches — the same fact written differently. A second,
structurally different class of problem is an entity or field where the set of producers and the set
of consumers simply do not line up. These are not incompatibilities between two interpretations;
they are holes.

| Object / field | Consumed by | Produced by | Gap |
|---|---|---|---|
| `productSpecCharacteristic[name="purpose"]` | consent-facade (→ the consent purpose recorded in every receipt) | **nobody** — no component and no UI writes it; only the hand-written `curl` in `doc/CONSENT_MANAGEMENT.md` | 🟥 **Required, no producer.** The facade falls back to the specification `name`, so every consent silently records the product name as its processing purpose. The BAE DSP form does not offer the field. |
| `Agreement` | consent-facade | fdsc-edc only | 🟧 see [item 5](#5-consent-depends-on-an-agreement-only-the-edc-produces) |
| `ProductOrder.billingAccount` | BAE validators (422 without it) | BAE only | 🟧 see [item 7](#7-productorder-three-incompatible-minimum-shapes) |
| `Usage` | nobody | **nobody** (modelled in fdsc-edc, not wired; the BAE charging backend is separately configured against the Usage API) | 🟨 **Dead surface.** `fdsc-edc` ships the `ExtendableUsage*VO` classes, `usage.json`, the `fromTransferProcess` mapper and a *required* `usageManagementApi` config value, but no `UsageApiClient` and no registered `TransferProcessStore`, so transfer state never reaches TMForum. Usage-based billing has no data to work from. |
| `Product` | consent-facade (optional path), fdsc-edc, BAE inventory UI | fdsc-edc **and** the BAE charging backend (which writes straight to `/tmf-api/productInventory/v4`, bypassing the logic-proxy — the proxy returns `methodNotAllowed` on `POST`) | 🟨 **Two producers, shapes unverified.** The charging backend is outside the five components analysed here; whether its `Product` carries `productSpecification` and which `relatedParty` roles it uses has not been checked. If it omits `productSpecification`, the consent catalog-graph walk via `agreementItem.product[]` fails — harmless today only because the `productOffering[]` path also resolves. |
| `Category`, `Catalog` | BAE UI only | BAE, API | 🟨 **Producer with no backend consumer.** No DSC component reads them; see [`entities.md`](./entities.md#category-and-catalog). |
| `ProductOfferingPrice` | BAE charging path | BAE | 🟨 No DSP consumer: neither the catalog, the negotiation nor the agreement carries price information. |
| `ProductOrder.agreement[]` | nobody | nobody | 🟨 The standard back-reference is unused; `fdsc-edc` links the two the other way round, via `Agreement.agreementItem`. |

### Should contract-management create `Product`s?

**No** — and the asymmetry is correct rather than a gap. `contract-management` is a *reactor*: it
observes `ProductOrder` state and turns it into IAM state. Creating inventory is the job of whoever
*fulfils* the order — the BAE charging backend for a marketplace checkout, `fdsc-edc` for a DSP
negotiation. Adding a third `Product` writer would produce duplicate entitlements for the same order
and give the entity three shapes instead of two.

The apparent gap dissolves once the consent projection is examined: `consent-facade` resolves the
`ProductSpecification` through **either** `agreementItem[].productOffering[]` **or**
`agreementItem[].product[]`, de-duplicated by specification id. The offering path alone is sufficient,
so a canonical `Agreement` written by `contract-management`
([plan.md phase 4](./plan.md#phase-4--canonical-agreement-producer)) needs no `Product` at all.

What *is* worth aligning is the reverse: `fdsc-edc`'s `Product` and the charging backend's `Product`
should agree on `productSpecification` and on the `relatedParty` roles, so that one inventory view can
show both marketplace and DSP acquisitions. That is a verification task, not a new producer.

---

## Compatibility matrix

Which feature combinations are safe on one TMForum backend:

| | fdsc-edc DSP | CM → TIL/PAP | CM → central marketplace | BAE authoring | consent-facade |
|---|---|---|---|---|---|
| **fdsc-edc DSP** | — | ✅ by design (order `completed` hand-off) | 🟧 needs role `provider` on the spec | 🟥 BAE cannot author `externalId` (item 3) | ✅ |
| **CM → TIL/PAP** | ✅ | — | ✅ | 🟧 depends on `oauth.customerrole` (item 2a) | n/a |
| **CM → central marketplace** | 🟧 | ✅ | — | 🟥 BAE replaces `relatedParty`, so `provider` is lost (item 2b) | n/a |
| **BAE authoring** | 🟥 | 🟧 | 🟥 | — | 🟥 no `Agreement` is produced (item 5) |
| **consent-facade** | ✅ | n/a | n/a | 🟥 | — |

---

## Recommended consolidation

In rough order of value per effort:

1. **One DID location.** Write `partyCharacteristic[name="did"]` from every producer (including BAE),
   keep `externalReference[idm_id]` as a fallback, add a uniqueness guard. Removes the duplicate-party
   class of failures (item 4).
2. **Role vocabulary.** Declare one role per relationship, match case-insensitively, and accept the
   known synonyms (`provider|Provider|Seller`, `Customer|Buyer|Consumer`) in every reader. Removes the
   silent-degradation class (item 2).
3. **One characteristic discriminator.** Standardise on `valueType`; make readers fall back to `id`
   and `name`; null-guard everywhere. Removes items 1 and most of 8.
4. **Make BAE merge instead of replace** (`relatedParty`, `@schemaLocation`), unblocking marketplace
   authoring of DSP offerings (item 3) — the single change that would let one catalog serve both the
   human and the machine path.
5. **State the consent prerequisite** (fdsc-edc required) in the chart, or add an `Agreement`-writing
   step to the activation path (item 5).
6. **Pin and mirror the extension schemas**, and publish them from one place — ideally this repo —
   rather than from personal/feature branches (item 9).
7. **Derive the PAP policy from the offering's contract policy** so an access rule is authored once
   (item 6).
