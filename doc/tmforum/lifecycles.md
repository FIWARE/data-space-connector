# Lifecycles and flows

How the entities come into existence and move through their states. Four flows:
catalog publication, contract negotiation, order activation, transfer.

---

## 1. Catalog publication

The provider authors the product. Two authoring paths exist and they produce **different** objects.

```mermaid
flowchart TB
    subgraph api["Path A — direct TMForum API (DSP-capable)"]
        direction TB
        A1["POST /productSpecification<br/>@schemaLocation external-id.json<br/>externalId = ASSET-1<br/>+ endpointUrl(s), upstreamAddress,<br/>transferType, transferPath,<br/>targetSpecification, serviceConfiguration,<br/>credentialsConfiguration, authorizationPolicy"]
        A1 --> A2["POST /productOffering<br/>@schemaLocation external-id.json<br/>externalId = OFFER-1<br/>productSpecification ref<br/>productOfferingTerm[edc:contractDefinition]<br/>{accessPolicy, contractPolicy} with odrl:uid"]
    end
    subgraph bae["Path B — BAE marketplace UI"]
        direction TB
        B1["create Category + Catalog"] --> B2["create ProductSpecification<br/>+ ProductOfferingPrice → Launched"]
        B2 --> B3["create ProductOffering<br/>(catalog + spec + price) → Launched"]
        B3 --> B4["BAE replaces relatedParty<br/>with Seller/SellerOperator<br/>and overwrites the offering's<br/>@schemaLocation with the DOME schema"]
    end
    A2 --> EDCCAT["<b>fdsc-edc DSP catalog</b><br/>needs externalId on both objects<br/>+ the contract-definition term"]
    B4 -.->|"externalId cannot survive<br/>(differences.md #3)"| EDCCAT
    B4 --> UI["marketplace catalog UI<br/>+ checkout"]
```

There is no publication *event* to react to: the DSP catalog is not materialised anywhere. fdsc-edc
computes it on every DSP catalog request, so a change to a specification or offering is visible
immediately, and an object that stops satisfying the requirements simply disappears from the catalog.

Categories and catalogs matter only for the marketplace UI — see
[`entities.md#category-and-catalog`](./entities.md#category-and-catalog).

---

## 2. Contract negotiation

The EDC state machine drives; TMForum stores. `Quote` is the negotiation record, and the
`contractNegotiation` extension object carries the authoritative state.

```mermaid
sequenceDiagram
    autonumber
    participant CN as EDC state machine
    participant Q as Quote API
    participant AG as Agreement API
    participant PO as ProductOrder API
    participant PI as Product Inventory
    participant CM as contract-management

    CN->>Q: REQUESTED/OFFERED → create Quote<br/>externalId=<negotiationId><br/>contractNegotiation{state, controlplane, lease}<br/>quoteItem[{externalId=offerId, datasetId, policy}]<br/>relatedParty[Provider, Consumer]
    Note over Q: TMForum state inProgress / approved<br/>a superseded quote is cancelled first
    CN->>Q: ACCEPTED → PATCH state=accepted
    CN->>AG: AGREED → create Agreement<br/>externalId=<contractId>, negotiationId,<br/>agreementType=dspContract, status=inProcess,<br/>characteristics policy/asset-id/provider-id/<br/>consumer-id/signing-date
    CN->>PO: VERIFIED → create ProductOrder<br/>quote[{id}], relatedParty[Consumer, <b>Customer</b>, Provider]
    Note over PO,CM: ProductOrderCreateEvent — quote present ⇒<br/>contract-management takes the negotiation branch,<br/>i.e. does not activate yet
    CN->>Q: FINALIZED → PATCH state=accepted
    CN->>PI: create Product<br/>externalId=<agreementId>-<ASSETID>, status=ACTIVE,<br/>relatedParty[Provider, Consumer], productOffering (provider side)
    CN->>AG: PATCH status=agreed,<br/>agreementItem.productItem = <productId>
    CN->>PO: PATCH state=completed
    Note over PO,CM: ProductOrderStateChangeEvent(completed) ⇒<br/>trusted-issuers-list entry + ODRL-PAP policies
```

State mapping — the EDC state name is stored verbatim in `contractNegotiation.state`, and mirrored
onto the TMForum `Quote.state`:

| EDC `ContractNegotiationStates` | `Quote.state` | Other TMForum effect |
|---|---|---|
| `INITIAL`, `REQUESTING`, `REQUESTED` | `inProgress` | – |
| `OFFERING`, `OFFERED` | `inProgress` → `approved` | – |
| `ACCEPTING`, `ACCEPTED` | `accepted` | – |
| `AGREEING`, `AGREED` | `accepted` | Agreement created (`inProcess`) |
| `VERIFYING`, `VERIFIED` | `accepted` | ProductOrder created |
| `FINALIZING`, `FINALIZED` | `accepted` | Product created; Agreement `agreed`; Order `completed` |
| `TERMINATING`, `TERMINATED` | `cancelled` | Order `cancelled`; Agreement `rejected` |

Two properties of this mapping are worth remembering:

* **A negotiation can have several quotes.** A re-offer cancels the superseded quote and creates a new
  one with the same `externalId`; the quote with the newest `quoteDate` is authoritative
  (`TMFEdcMapper.getNewest`). Do not assume `?externalId=<id>` returns a single object.
* **Every step is compensated.** Each transition touches two to four entities, and TMForum has no
  transactions, so `TMFTransactionContext` registers a compensating action per write: a created quote
  is cancelled, an agreement's status reverted, an order's state restored
  (`fdsc-edc: TRANSACTION_README.md`).

Worked example with the corresponding DSP calls: [`../DSP_INTEGRATION.md`](../DSP_INTEGRATION.md).

---

## 3. Order activation

The `ProductOrder` is the universal activation trigger, regardless of who created it.

```mermaid
flowchart TB
    SRC1["BAE checkout<br/>(items + billingAccount,<br/>state computed from items)"] --> PO
    SRC2["fdsc-edc at VERIFIED<br/>(quote ref, no items,<br/>no billingAccount)"] --> PO
    SRC3["direct API<br/>(demo flows)"] --> PO
    PO["<b>ProductOrder</b>"] -->|"Create / StateChange / Delete event"| CMH["contract-management<br/>ProductOrderEventHandler"]
    CMH --> CHK1{"exactly one customer<br/>relatedParty whose role matches<br/>productOrder.customerRole?"}
    CHK1 -->|no| ERR["<b>400</b> Exactly one ordering<br/>related party is expected"]
    CHK1 -->|yes| CHK2{"quote[] present<br/>on a create event?"}
    CHK2 -->|yes| NEG["negotiation branch —<br/>no activation on create"]
    CHK2 -->|no| CHK3{"state == completed?"}
    CHK3 -->|no| IGN["create: ignored<br/>state change / delete:<br/>handleProductOrderStop"]
    CHK3 -->|yes| ACT["handleProductOrderComplete<br/>on every enabled handler"]
    ACT --> R1["<b>TIL</b>: resolve credentialsConfiguration →<br/>add/merge credentials for the customer DID"]
    ACT --> R2["<b>PAP</b>: resolve authorizationPolicy →<br/>create policies scoped to the order id"]
    ACT --> R3["<b>remote CM</b>: POST /order/start per<br/>non-local contractManagement"]
```

Deactivation is symmetric: any state change away from `completed`, or a delete event, calls
`handleProductOrderStop` → remove the credentials from the trusted-issuers-list, delete the PAP
policies for that order id, `POST /order/stop` to remote contract-managements.

Where the configuration comes from — the resolution walk:

```mermaid
flowchart LR
    PO2["ProductOrder"] --> HASQ{"quote[]?"}
    HASQ -->|yes| QQ["Quote (state=accepted) →<br/>quoteItem[state=accepted, action≠delete] →<br/>productOffering"]
    HASQ -->|no| OI["productOrderItem[action∈{add,modify}] →<br/>productOffering"]
    QQ --> SPEC["ProductSpecification"]
    OI --> SPEC
    SPEC --> C1["productSpecCharacteristic<br/>[valueType=credentialsConfiguration]"]
    SPEC --> C2["productSpecCharacteristic<br/>[valueType=authorizationPolicy]"]
    SPEC --> RP{"relatedParty[role=provider]?"}
```

`local` vs. `remote` is decided per resolved configuration, and each handler filters accordingly:

```mermaid
flowchart LR
    RP["ProductSpecification.relatedParty[role=provider]"] -->|absent| LOC["ContractManagement(local=true)"]
    RP -->|present| ORG["Organization"] --> DID{"DID == general.did?"}
    DID -->|yes| LOC
    DID -->|no| PC{"partyCharacteristic<br/>[contractManagement]?"}
    PC -->|absent| LOC
    PC -->|present| REM["ContractManagement(local=false,<br/>address, clientId, scope)"]
    LOC --> H1["TIL + PAP handled in-process"]
    REM --> H2["order event forwarded<br/>(OID4VP-authenticated)"]
```

---

## 4. Transfer

Once an agreement exists, the consumer starts a DSP transfer. TMForum's role here is `Usage` as the
transfer-process store plus the `ProductSpecification` as the provisioning descriptor.

```mermaid
sequenceDiagram
    autonumber
    participant TP as EDC transfer state machine
    participant U as Usage API
    participant PS as Product Catalog API
    participant PAP as ODRL-PAP
    participant CCS as credentials-config-service
    participant GW as APISIX

    TP->>U: create Usage<br/>externalId=<transferProcessId>, transferState=INITIAL,<br/>usageType=dspTransfer, status=received,<br/>usageCharacteristic[asset-id, contract-id, protocol,<br/>counter-party-address, transfer-type, type]
    TP->>PS: GET /productSpecification?externalId=<assetId>
    PS-->>TP: characteristics (by <b>id</b>): upstreamAddress,<br/>targetSpecification, serviceConfiguration
    TP->>PAP: createService(<transferProcessId>) + createPolicy<br/>(agreement policy, odrl:uid replaced by the process id,<br/>odrl:target replaced by targetSpecification)
    TP->>GW: add service route (upstream = upstreamAddress,<br/>policy path from the PAP) + well-known route
    TP->>CCS: createService(serviceConfiguration) — OID4VP variant only
    TP->>U: PATCH transferState=STARTED,<br/>status=rated, ratedProductUsage.productRef=<productId>
```

Note the deliberate re-identification: the agreement's policy is installed in the PAP under the
**transfer-process id**, not its original `odrl:uid`, so several concurrent transfers of the same
asset do not collide.

> ⚠️ The two `Usage` steps above are the **intended** design, not current behaviour: `fdsc-edc` has
> the `Usage` model, schema and mapper but no API client and no registered `TransferProcessStore`
> implementation, so transfer state is not persisted to TMForum today. Everything else in this
> sequence — the specification lookup, the PAP service and policy, the APISIX routes and the
> credentials-config-service entry — is wired.
