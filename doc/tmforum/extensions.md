# Extension registry

Every DSC-specific field added to the TMForum model, in one place: what it means, who owns it, how it
is discriminated and which JSON schema validates it.

Two mechanisms are in play (see [`platform.md`](./platform.md#extension-with-schemalocation)):

* **Extension properties** — new top-level JSON properties on an entity, validated against
  `@schemaLocation`.
* **Characteristics** — entries in `productSpecCharacteristic[]` / `characteristic[]` /
  `usageCharacteristic[]` / `partyCharacteristic[]`, which need no schema (they are standard TMForum
  structures) but whose *value* is DSC-specific.

---

## 1. Extension properties

| Entity / object | Property | Type | Meaning | Owner | Schema |
|---|---|---|---|---|---|
| `ProductSpecification` | `externalId` | string | EDC asset id / DSP dataset id | fdsc-edc | `edc-dsc/schemas/external-id.json` |
| `ProductOffering` | `externalId` | string | EDC contract-definition id / DSP offer id | fdsc-edc | `external-id.json` |
| `ProductOfferingTerm` | `accessPolicy` | ODRL object | catalog-visibility policy | fdsc-edc | `contract-definition.json` |
| `ProductOfferingTerm` | `contractPolicy` | ODRL object | negotiable-terms policy | fdsc-edc | `contract-definition.json` |
| `Quote` | `contractNegotiation` | object | EDC negotiation state, lease and control-plane ownership | fdsc-edc | `contract-negotiation.json` |
| `QuoteItem` | `externalId` | string | DSP offer id | fdsc-edc | `quote-item.json` |
| `QuoteItem` | `datasetId` | string | DSP dataset id | fdsc-edc | `quote-item.json` |
| `QuoteItem` | `policy` | object | offered/agreed ODRL policy, *expanded* JSON-LD | fdsc-edc | `quote-item.json` |
| `Agreement` | `externalId` | string | EDC/DSP contract-agreement id | fdsc-edc | `agreement.json` |
| `Agreement` | `negotiationId` | string | EDC negotiation id | fdsc-edc | `agreement.json` |
| `Product` | `externalId` | string | `<agreementId>-<ASSETID>` | fdsc-edc | `external-id.json` |
| `Usage` | `externalId` | string | EDC transfer-process id | fdsc-edc | `usage.json` |
| `Usage` | `transferState` | enum | EDC `TransferProcessStates` name | fdsc-edc | `usage.json` |
| `ProductSpecificationCharacteristic` | `productSpecCharacteristicValue[].value` | object | credential config | contract-management | `credentials/credentialConfigCharacteristic.json` |
| `ProductSpecificationCharacteristic` | `productSpecCharacteristicValue[].value` | object | ODRL authorization policy | contract-management | `odrl/policyCharacteristic.json` |

> ⚠️ **The `externalId` / `negotiationId` properties are DSP back-references, not general model
> requirements.** They are written by `fdsc-edc` so the DSP world can find its own objects again, and
> their presence is what *marks* an entity as DSP-relevant. A consumer must skip entities that lack
> them rather than fail, which is what `fdsc-edc` already does. They must never be required globally —
> see [`plan.md`](./plan.md#phase-0--agree-the-model), decision D12.

> ⚠️ `usage.json` and `contract-negotiation-id.json` / `callback-address.json` are declared but
> currently unused: the `Usage`-backed transfer store is modelled in `fdsc-edc` and not wired.

> ⚠️ An object carries exactly **one** `@schemaLocation`, and `tm-forum-api` validates *all* of its
> unknown properties against that one schema. Two features therefore cannot extend the same object
> independently — see [`differences.md#9-one-schemalocation-slot-per-object`](./differences.md#9-one-schemalocation-slot-per-object).

### Schema locations

fdsc-edc schemas live in [`SEAMWARE/fdsc-edc/schemas`](https://github.com/SEAMWARE/fdsc-edc/tree/main/schemas)
but the **default base URI the code emits** is
`https://raw.githubusercontent.com/wistefan/edc-dsc/refs/heads/init/schemas/`
(`TMFConfig.DEFAULT_SCHEMA_BASE_URI`), overridable via `tmfExtension.schemaBaseUri`. The
`@schemaLocation` is filled in lazily by the `Extendable*VO` classes when the field is not already
set (`getAtSchemaLocation()` → `SchemaBaseUriHolder.get().resolve("<file>.json")`), so callers never
need to set it by hand — but a hand-written payload does.

contract-management schemas live in
[`FIWARE/contract-management/schemas`](https://github.com/FIWARE/contract-management/tree/main/schemas)
and are referenced explicitly in payloads.

| Schema | Declares | Required |
|---|---|---|
| `external-id.json` | `externalId` | `externalId` |
| `contract-definition.json` | `accessPolicy`, `contractPolicy` (both `policy.json`) | both |
| `contract-negotiation.json` | `contractNegotiation{controlplane,isPending,isLeased,correlationId,counterPartyAddress,state}` | – |
| `contract-negotiation-id.json` | `contractNegotiationId` | yes |
| `quote-item.json` | `externalId`, `datasetId`, `policy` | `externalId` |
| `agreement.json` | `externalId`, `negotiationId` | both |
| `usage.json` | `externalId`, `transferState` (enum of 23 states) | both |
| `callback-address.json` | `uri`, `authKey`, `authCodeId` | – |
| `policy.json` | `permissions` (array) | – |
| `odrl/policyCharacteristic.json` | `valueType` **const** `authorizationPolicy` + `productSpecCharacteristicValue[]` | both |
| `credentials/credentialConfigCharacteristic.json` | `valueType` **const** `credentialsConfiguration` + `productSpecCharacteristicValue[]` | both |
| `credentials/credentialConfigObject.json` | `credentialsType`, `claims[]` | both |
| `credentials/claims.json` | `name`, `path`, `allowedValues[]` | `name`, `allowedValues` |

Note that `policyCharacteristic.json` and `credentialConfigCharacteristic.json` pin `valueType` with
a JSON-Schema `const` — so for those two characteristics the discriminator is **contractually**
`valueType`, not a convention.

---

## 2. `productSpecCharacteristic` registry

This is the provider's configuration surface. **The discriminator field differs per consumer** —
always check the column before authoring:

| Characteristic | Discriminated by | Value type | Consumed by | Meaning |
|---|---|---|---|---|
| `endpointUrl` | `valueType` | string | fdsc-edc | DSP/DCAT endpoint. fdsc-edc reads **all** occurrences and emits one `DataService` per entry, using the characteristic's `id` as the DataService id (e.g. `dcp`, `oid4vc`) |
| `endpointDescription` | `valueType` | string | fdsc-edc | `dcat:endpointDescription` on the DSP `DataService` |
| `transferType` | `valueType` | string | fdsc-edc | Catalog distribution format. Only `HttpData-PULL` is supported; **absent ⇒ `HttpData-PULL`**; present-but-unsupported ⇒ the dataset is dropped from the catalog |
| `transferPath` | `valueType` | string | fdsc-edc | Path appended to the transfer endpoint in the EDR, so the consumer receives a ready-to-use URL |
| `upstreamAddress` | **`id`** | string | fdsc-edc (`TMFEdcMapper`, provisioners) | The real backend `host:port`. **Mandatory** — without it neither the EDC asset nor the transfer can be built |
| `targetSpecification` | **`id`** | object (ODRL `AssetCollection` + `refinement`) | fdsc-transfer-extension | Replaces `odrl:target` in the policy pushed to the ODRL-PAP, enabling path/attribute-level enforcement |
| `serviceConfiguration` | **`id`** | object (credentials-config-service `Service`) | fdsc-transfer-extension | Registered at the credentials-config-service when provisioning an OID4VC-secured transfer (`defaultOidcScope`, `oidcScopes`, `dcql`, trusted lists) |
| `credentialsConfiguration` | `valueType` (schema `const`) | array of `{credentialsType, claims[]}` | contract-management → trusted-issuers-list | Which credential types + claim restrictions the customer's issuer may issue once the order completes |
| `authorizationPolicy` | `valueType` (schema `const`) | array of ODRL policies | contract-management → ODRL-PAP | Access-control policies installed when the order completes |
| `purpose` | **`name`** (configurable: `facade.spec.purpose-characteristic`) | object or JSON string | consent-facade | Processing purpose: `{id, name, description, purpose (DPV), legalBasis (DPV)}`. Only `name` is consumed by today's consent-manager |
| `asset type`, `media type`, `location` | **`name`**, case-insensitive | string | BAE | A spec carrying **all three** is a *digital product*: BAE then forbids characteristic changes without a version bump |

### Value selection

The `productSpecCharacteristicValue[]` list is resolved differently, too:

| Component | Rule |
|---|---|
| fdsc-edc `TMFEdcMapper.getValue` | prefer the entry with `isDefault: true`, else the first entry; accepts `value` as a string **or** as `{"value": "…"}` |
| fdsc-edc provisioners `getCharValueSpec` | prefer `isDefault: true`, else the first entry |
| CM `CredentialsConfigResolver` / `PolicyResolver` | takes **every** value of the *first* matching characteristic and flattens them |
| consent-facade `CatalogMapper` | first value of the matching characteristic |

> **Rule of thumb for authors:** always set `isDefault: true` on exactly one value of every
> characteristic, always set `id`, `name` **and** `valueType`, and keep `id` == `valueType` where the
> DSC examples do (`upstreamAddress`, `transferType`, `transferPath`). That payload satisfies every
> consumer. The `doc/DSP_INTEGRATION.md` example follows this pattern.

---

## 3. Name/value characteristic registry

### `Organization.partyCharacteristic`

| Name | Value | Consumed by |
|---|---|---|
| `did` | the participant DID (string) | CM, fdsc-edc, consent-facade |
| `contractManagement` | `{address, clientId, scope[]}` — the participant's own contract-management endpoint and the OID4VP client/scope to authenticate with | CM (`ContractManagement` POJO) |
| `country` | ISO country, normalised | BAE |
| `academicTitle`, `language`, `pictureUrl` | user profile (on `Individual`) | BAE |

`contractManagement` decides *local vs. remote*: if the resolved DID equals the local
`general.did`, or the characteristic is missing, the config is treated as `local = true` and handled
in-process (trusted-issuers-list + ODRL-PAP). Otherwise the order event is forwarded to that
participant's contract-management `POST /order/start` | `/order/stop`.

### `Agreement.characteristic`

| Name | Written by | Meaning |
|---|---|---|
| `policy` | fdsc-edc | agreed ODRL policy (expanded JSON-LD) |
| `asset-id` | fdsc-edc | EDC asset id |
| `provider-id` | fdsc-edc | provider DID |
| `consumer-id` | fdsc-edc | consumer DID |
| `signing-date` | fdsc-edc | epoch seconds; the consent-facade treats its presence as "signed" |

### `Usage.usageCharacteristic`

`asset-id`, `correlation-id`, `protocol`, `counter-party-address`, `transfer-type`, `type`,
`contract-id`, `resource-manifest`, `dataplane-id`, `content-data-address` — all written and read by
fdsc-edc only.

---

## 4. The contract-management order event

Not a TMForum structure, but the projection of one — the payload contract-management sends to a
*remote* contract-management in the central-marketplace scenario
(`contract-management/api/contract-management.yaml`):

```jsonc
POST /order/start   |   POST /order/stop
{
  "orderId":    "<ProductOrder.id>",
  "customerId": "did:web:my-customer.org",     // customer DID, resolved from the Organization
  "policies":   [ /* ODRL policy objects from authorizationPolicy characteristics */ ],
  "credentialsConfig": [
    { "credentialsType": "OperatorCredential",
      "validFor": { "from": "…", "to": "…" },
      "claims": [ { "name": "roles", "path": "$.roles[*].names[*]", "allowedValues": ["OPERATOR"] } ] }
  ]
}
```

The grouping is per target contract-management: a single order whose items belong to several
providers produces one event per provider, each carrying only that provider's policies and credential
configs (`ContractManagementProductOrderHandler.toOrderMap`). Entries whose `ContractManagement` is
`local` are filtered out of the outbound events and handled locally instead.

---

## Adding a new extension

1. **Pick the plane.** Provider-authored configuration of a data product ⇒ a
   `productSpecCharacteristic`. Machine state that needs to be *queried* ⇒ an extension property
   (extension properties are broker-queryable, characteristic values are not).
2. **Write a JSON schema**, host it where the `tm-forum-api` pod can fetch it, and reference it from
   `@schemaLocation` on the object you extend — not on the document root.
3. **Check the object does not already have a `@schemaLocation`.** There is only one slot; if the
   object already carries one you must extend that schema instead of adding a second URL.
4. **If the new property is list-valued, add it to the ODRL/NGSI-LD context**
   (`fdsc-edc/schemas/odrl-context.jsonld`, wired via `tm-forum-api.defaultConfig.contextUrl`) with
   `"@container": "@set"`, or a single-element list will round-trip as an object.
5. **Use the existing discriminator conventions** (`id` == `valueType` == the semantic key, `name`
   human-readable, exactly one `isDefault: true` value).
6. **Document it here** and, if a component now *requires* it, in
   [`differences.md`](./differences.md) — a new mandatory field is a compatibility break for every
   other authoring path, BAE included.
