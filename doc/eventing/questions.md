# Open questions

Everything the [architecture](architecture.md) decided but could reasonably have decided otherwise,
plus what it could not decide at all. Each question says what it blocks, so the ones that have to be
settled before code starts are visible at a glance.

**Blocking implementation:** Q1, Q2, Q3.
**Settle during implementation:** Q4–Q9.
**Can be answered after the first release:** Q10–Q13.

---

## Q1 — One component or two? *(blocks everything)*

**Context.** The architecture puts both ends of the mechanism into `contract-management` (D-1): it
already receives authenticated events on `/order/*`, already sends them over OID4VP, and already
registers callbacks at start-up with retry and a health indicator. The cost is that a component
named "contract management" ends up running at a trust anchor, which manages no contracts, and
carrying a responsibility that has nothing to do with TMForum.

**Options**

| | Pro | Con |
|---|---|---|
| **a) contract-management, two modes** *(recommended)* | no new repository, image, chart, CI or identity; reuses the OID4VP client, the auth handler and the subscriber pattern; the receiving side is where the TIL cleanup has to happen anyway | the name stops describing the component; the trust anchor runs a contract manager |
| b) a new `dataspace-events` service | clean separation; a publisher that a trust anchor can run without pulling in TMForum machinery | a second codebase duplicating the OID4VP sending and auth stack, plus release and chart work; the receiving side still needs the TIL access that CM has |
| c) publisher in the trusted-issuers-list, listener in CM | the publisher sits where the state changes | contradicts D-3 (the TIR is pluggable); splits one mechanism across two codebases; the TIL would need an OID4VP client it has no other use for |

**Question for the group:** is (a) acceptable, or is the naming/separation concern strong enough to
pay for (b)? If (a), is renaming the component (or documenting it as "the participant's integration
service") worth a follow-up?

## Q2 — Best-effort or durable delivery? *(blocks CM-2, CM-3)*

**Context.** D-4 keeps `contract-management` stateless — it has no database at all today — by making
delivery best-effort and holding subscriptions as in-memory leases, and compensates with the
reconciliation sweep against the TIR (§10). That works because the authoritative state for *this*
event type is public and re-readable. It will not work for an event that *is* the state, such as the
future `service.configuration.provided`.

**Options**

- **a) Stateless now, durable when the first non-reconcilable event type arrives** *(recommended)* —
  the API contract does not change when durability is added.
- b) Persisted outbox + subscription registry from the start — adds a database (Micronaut Data +
  Liquibase, as in the trusted-issuers-list), a retention policy and a migration path, in exchange
  for delivery guarantees nobody needs yet.

**Question:** does any party need *auditable* proof that a removal notice was delivered (regulatory,
or contractual between operator and participant)? If yes, (b) is not optional and this merges with
Q11.

## Q3 — What should a removal do by default? *(blocks CM-5, DSC-2)*

**Context.** D-6 makes the reaction configurable along the grant scope and defaults to `unscoped`:
drop the entries that exist because the DID was a participant, keep every order-backed grant. The
alternatives are defensible in opposite directions.

| Default | Argument |
|---|---|
| `none` | an operator action at the trust anchor never changes a participant's state without review; the report alone already fixes the visibility problem |
| **`unscoped`** *(recommended)* | removes exactly what the data space's vouching created, keeps what a contract pays for — the case the requirement calls out |
| `full` | removal from the data space means the DID is no longer vouched for; keeping any grant means vouching for them yourself, which should be an explicit choice |

**Question:** which one, and does the answer change when the removal carries a `reason` of `revoked`
(expulsion for cause) rather than `removed-from-registry`? A per-reason default is cheap to add and
may be the honest answer: `full` on `revoked`, `unscoped` on a plain deregistration.

## Q4 — How does a publisher learn where to send? *(CM-2/CM-3)*

The architecture uses an explicit, authenticated subscription API with leases (§5.2, D-8).

**Alternatives**

- **DID-document discovery** — the subscriber publishes a `service` entry (e.g. type
  `DataSpaceEvents`) in its `did:web` document; the publisher resolves it from the DID it is about
  to notify. No subscription state at all, no callback validation needed, and the audience for a
  broadcast can be derived straight from the TIR. Needs `did-helper` to support service entries and
  makes every participant's event endpoint public.
- **Static configuration at the trust anchor** — an operator-maintained list. Simplest, no API, but
  no self-service and it drifts.

**Question:** is DID-document discovery available (does `did-helper` support service entries?) and
is publishing every participant's event endpoint in its DID document acceptable? If yes, it is
arguably the better design and would remove §5.2 entirely.

## Q5 — Does cleanup stop at the TIL? *(CM-5)*

A removed participant may also be named in ODRL policies at the PAP (created per order), in Rainbow
or EDC agreements, in issued credentials, and in `registration.issuer` values that will re-create
the TIL entry on the next Helm upgrade — that last one is worth noting: **in `unscoped` mode, a
participant listed in `registration.issuer` comes back on every upgrade** unless the operator also
removes it from the values.

**Recommendation:** v1 touches the TIL only and *reports* the rest, because policies and agreements
belong to orders, which the default mode deliberately keeps. **Question:** is a report enough, or
should the PAP be in scope from the start? And should the reconciliation report flag
`registration.issuer` entries that refer to removed participants?

## Q6 — How should the trusted-issuers-list expose "unscoped"? *(TIL-1/TIL-2)*

`DELETE /issuer/{did}/credential` requires `scope` and answers `400` without it.

- a) Make the omitted parameter mean "the unscoped credentials". Consistent with the existing
  wording ("without a scope the credentials are managed directly through the issuer endpoints"), but
  it turns a current `400` into a destructive success — a client that forgets the parameter now
  deletes something.
- **b) An explicit, separate addressing of the unscoped set** *(recommended)*, e.g.
  `DELETE /issuer/{did}/credential?scope=` with an empty value rejected and a distinct
  `unscoped=true`, or a dedicated sub-path. Unambiguous and safe against a forgotten parameter.

**Also:** should `GET /issuer/{did}` gain an opt-in scope projection instead of the separate
`GET /issuer/{did}/scope` of TIL-2? The spec is emphatic that the scope is "never exposed through
the trusted-issuers-registry" — the management API is a different matter, but the choice should be
deliberate.

## Q7 — Does the event payload need its own signature? *(CM-1)*

Today the event is trusted because of the transport: OID4VP token, PEP policy, and the publisher×type
check. A JWS over the envelope by the publisher's DID would additionally give non-repudiation and
would let events be relayed or stored and forwarded.

**Recommendation:** not in v1 — it is additive (a `signature` extension attribute) and nothing in
the design relays events. **Question:** is non-repudiation of a removal notice required by anybody's
governance framework? (See also Q2.)

## Q8 — How is the publisher deployed at the trust anchor, and what identity does it use? *(DSC-2)*

The trust anchor has no OID4VP identity today: it needs a `did:web`, a signing key and a credential
the participants accept. Two proven paths exist in this repository:

- a second `data-space-connector` release in the trust anchor's namespace providing keycloak,
  did-helper and a vc-operator — the `consent-authority` pattern (`k3s/consent-trust-anchor.yaml`);
- mounting an externally issued credential and key from secrets, as the consent-facade does
  (`k3s/consent-trust-anchor-components.yaml`, `oid4vp.credentials[].secretName`).

**Question:** which one does the `trust-anchor` chart adopt as its documented path, and does the
publisher become a dependency of `charts/trust-anchor` or stay a separate release?

## Q9 — Which credential does the publisher present, and who has to trust it? *(DSC-1/DSC-2)*

The architecture leaves the type open (`TrustAnchorCredential` in the examples), as the central
marketplace leaves `MarketplaceCredential` open. But every participant must then (a) admit that type
to the events path in its ODRL policy and (b) have the trust anchor's DID in its local TIL for that
type — which is itself a trust entry that has to get there somehow, probably through
`registration.issuer` at deployment.

**Question:** is the trust anchor's own entry in every local TIL part of the standard deployment
values, and is the credential type a data space-wide governance parameter (documented in the
operator guide) or per-participant configuration? Note the circularity: the mechanism that announces
removals depends on a trust entry that the mechanism itself must never remove — the trust anchor's
DID belongs on the `keep` list by default.

## Q10 — Can the ODRL policy constrain the issuer DID? *(nice to have)*

D-5 puts the publisher×type check in the application because a PEP policy constrains the credential
*type*, and in a typical data space many organizations hold the type that would admit them to the
events path. If the ODRL-PAP/OPA stack can express a constraint on the credential's issuer or holder
DID (`vc:issuer`-style left operand), the transport layer could reject a spoofed publisher before it
reaches the application, which is strictly better defence in depth. The application check stays
either way.

**Question for the PAP maintainers:** is such a left operand available, or planned?

## Q11 — Is this mechanism meant to become a specification? *(roadmap)*

The connector's cross-participant interfaces are otherwise standards (DSP, DCP, OID4VP, EBSI TIR,
TMForum). A data space event channel is a new interface between organizations, and a FIWARE-specific
one will be hard to use with non-FIWARE participants.

**Question:** should the envelope and the two APIs be written up for the DSBA/Gaia-X/IDSA context,
and is there prior art to align with (IDSA has no notification protocol; EDC and Rainbow have
internal callbacks only)? The answer affects naming (`org.fiware.dataspace.*` vs. a neutral
namespace) more than the design.

## Q12 — Is a ~2 minute detection latency acceptable? *(operations)*

The watcher's default `interval × confirmations` is about two minutes, and the push path adds
seconds. Faster means polling a registry harder or instrumenting it (D-3's rejected alternative).
Slower is free.

**Question:** does the governance framework promise a removal takes effect within a stated time? If
so, that number sets the defaults — and if it is short, a webhook from the FIWARE
trusted-issuers-list as a latency optimisation on top of the watcher becomes worth the work.

## Q13 — What about a participant that is in several data spaces? *(design, post-v1)*

A local TIL entry records *that* an issuer is trusted, not *why* or *under which data space*. A
participant in two data spaces that receives a removal from one cannot tell which of its entries
belong to that data space, so the `keep` list is doing work that the data model should do. Scoped
grants are attributable (an order belongs to a marketplace that belongs to a data space); unscoped
ones are not.

**Question:** is multi-data-space membership a case the connector intends to support? If yes, the
honest fix is to attribute unscoped TIL entries to a data space as well — a bigger change to the
trusted-issuers-list than TIL-1/TIL-2, and better decided before the `keep` list becomes the
documented workaround.
