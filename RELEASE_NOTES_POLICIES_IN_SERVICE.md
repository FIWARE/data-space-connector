# Policies on the ServiceSpecifications a product is composed of

Topic branch `policies-in-service`. An access policy protects an API, and in TMForum an API is a
`ServiceSpecification` — so a provider selling one product built from several services can author
each policy where it belongs instead of merging them into one policy on the `ProductSpecification`.

`contract-management` now reads the policy and credential configuration of the composed
`ServiceSpecification`s when it activates an order, behind a flag that is off by default.

The analysis, the ratified decision register and the per-component work items are in
[`doc/tmforum/service-policy.md`](doc/tmforum/service-policy.md); the provider-facing guide is
[`COMPOSED_SPECIFICATIONS.md`](doc/deployment-integration/roles/provider/COMPOSED_SPECIFICATIONS.md).

## What was decided

The eleven decisions are recorded in full in the plan; the five that shaped the implementation:

| | |
|---|---|
| **D-C1** | Only `ProductSpecification` and `ServiceSpecification` carry configuration. `ResourceSpecification` is not traversed — nothing in the connector consumes a resource-level fact |
| **D-C2** | The `fdsc-edc` integration characteristics (`endpointUrl`, `transferType`, `upstreamAddress`, …) **stay on the `ProductSpecification`**: DSP has no combined product, so authoring them on a service would put them where nothing reads them. `fdsc-edc` therefore needed no change at all |
| **D-C3/D-C4** | Union, never override. Policies de-duplicated by `odrl:uid`, credential configurations by value. Two *different* policies claiming one uid fail the activation, because the ODRL-PAP keys an installed policy by that uid |
| **D-C6** | Single provider. A composition declaring more than one provider fails; a part declaring none inherits the product's |
| **D-C7** | An order's configuration is a **snapshot** taken when it completes. This is a decision against re-activation, not a deferral: a provider who wants a changed policy to apply publishes a new version |

## What changed, per component

### `contract-management` — released as `3.3.12`

| | |
|---|---|
| Tolerant resolution | A specification without any characteristic, or a characteristic without a `valueType`, raised a `NullPointerException`; an offering that bundles others failed the activation of *every other item* in the same order; and an order that legitimately configures nothing resolved to nothing at all, which the listener answered with a 404 and the TMForum hub answered with a redelivery. All three are fixed, and all three could hit a **flat** order |
| Broken references fail loudly | A referenced offering, specification, quote or provider that cannot be read is logged and raised instead of silently activating a partial configuration |
| `CharacteristicValues` | Normalizes both characteristic shapes onto one `(valueType, values)` view — the container names differ per API (`productSpecCharacteristic` vs. `specCharacteristic`, `productSpecCharacteristicValue` vs. `characteristicValueSpecification`) |
| `SpecificationGraphResolver` | Walks the product into its services and, recursively, its bundled products. Visited-set and depth limit, because `bundledProductSpecification` is not cycle-checked by the API; hitting either guard truncates **and warns**, naming the specification and what it skipped |
| Aggregation | The first matching characteristic is read *per specification*, then unioned per D-C3/D-C4 |
| Order-scoped grants | Credentials are granted under the id of the order that granted them, so cancelling one order leaves what another order granted in place. Revocation deletes by that scope and no longer resolves the configuration at all — a specification that changed since the grant cannot block a revocation |
| Settings | `general.enableSpecificationComposition` (default `false`), `general.specificationCompositionMaxDepth` (default `2`) |

Tests: 81, up from 44.

### `trusted-issuers-list` — released as `0.9.1`

A credential entry was identified by its **content**, so revoking one grant removed a credential
another grant still required. Credentials now carry the id of whatever granted them:

* `PUT /issuer/{did}/credential?scope=<id>` replaces the credentials of one scope — idempotent under
  redelivered notifications — and creates the issuer if it is unknown;
* `DELETE /issuer/{did}/credential?scope=<id>` revokes one scope, keeping the issuer;
* `POST /issuer` takes an optional scope, so an issuer created together with its credentials can
  attribute them — without it, the first grant to an issuer would be unrevocable;
* `PUT /issuer/{did}` manages only the directly maintained credentials and no longer wipes grants.

The scope is deliberately **not** in the API models: both registry projections serialize
`CredentialsVO`, and v5 derives attribute ids from those bytes, so a scope there would publish grant
ids in a world-readable registry and change every attribute id. It travels as a query parameter and
lives on the entity only; two regression tests pin the projections. Both projections de-duplicate,
since one credential can now come from several scopes.

Tests: 167, up from 147.

> `0.9.0` carries the API but **cannot start against MySQL**: the composite index it added exceeded
> MySQL's key length, because `trusted_issuer_id` is `varchar(768)` — 3072 bytes under utf8mb4 and
> therefore the whole key budget. `0.9.1` indexes `scope` alone, with new precondition-guarded
> changeset ids so it recovers a database that `0.9.0` left half-migrated. Postgres and H2 were
> unaffected, which is why only the MySQL-based integration tests showed it.

### `helm-charts`

* `contract-management` **3.5.36** (appVersion `3.3.12`) renders the two settings — the configmap
  template lists them explicitly, so a property put under `contract-management.*` by an umbrella
  chart would otherwise never reach the application.
* `trusted-issuers-list` **0.18.7** (appVersion `0.9.1`).

### `data-space-connector` — `10.5.0`

* the two settings exposed under `contract-management`, and the subchart pinned at `3.5.36`;
* `doc/tmforum/` — nine documents describing the TMForum model the connector uses as its system of
  record, including the ten divergences between the components and the plan this topic followed;
* the provider authoring guide, linked from the provider role;
* a composed scenario in the marketplace integration test: the policy is authored on the
  `ServiceSpecification`, and the consumer may then create a cluster with 3 nodes and may not create
  one with 4 — the policy authored on the service is what the PDP enforces;
* `allowServiceSpec.json`, so a seller may author service specifications through the OID4VP-protected
  API. No policy-decision-point change was needed: `tmf:resource` derives the resource from the
  request path generically.

## Interim: the `trusted-issuers-list` image is pinned

```yaml
decentralizedIam:
  vcAuthentication:
    trusted-issuers-list:
      deployment:
        image:
          tag: "0.9.1"
```

Granting under an order's scope needs `0.9.1`, and the chart chain cannot deliver it yet:
`decentralized-iam 2.1.20` depends on `vc-authentication 1.3.6`, which still pins the
`trusted-issuers-list` chart `0.18.4` (app `0.8.1`). Against that version the scope-addressed
endpoints answer `404` — a grant fails and a revocation silently does nothing.

**Remove the override** once `decentralized-iam` ships a `vc-authentication` with the
`trusted-issuers-list` chart `>= 0.18.7`.

## Behaviour change that is not behind the flag

A single `ProductSpecification` declaring two **different** parties in the provider role now fails
the activation instead of picking one arbitrarily. Which contract-management is responsible was never
defined for that shape. Check for specifications with more than one `relatedParty` in the configured
provider role before upgrading.

## Still open

| | |
|---|---|
| BAE authoring UI | offering `authorizationPolicy` and `purpose` on a `ServiceSpecification` in the marketplace frontend, and setting `valueType` on service-spec characteristics |
| Consent granularity | `consent-facade` fixes 1 `ProductSpecification` = 1 `DataResource`; with `purpose` on the service level, 1 `ServiceSpecification` = 1 `DataResource` is the truthful projection |
| The chart chain | the `vc-authentication` hop above |
| Performance | the graph is resolved once per order item, so two items referencing one specification resolve it twice; and `tm-forum-api` could expand references server-side |
