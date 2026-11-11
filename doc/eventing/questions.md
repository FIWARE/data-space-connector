# Decisions

The questions this design opened were answered in the design reviews of 2026-10-28 (Q1–Q13) and
2026-10-29 (F1–F8). Nothing is open; this file is the record of what was decided and where it
landed.

## Answered

| # | Question | Decision | Where it landed |
|---|---|---|---|
| Q1 | One component or two? | `contract-management` carries both ends for now; a rename is possible later but is **not** part of this epic | architecture D-1, F6 below |
| Q2 | Best-effort or durable delivery? | Best-effort is fine; no party needs proof of delivery | D-4, §7 |
| Q3 | What should a removal do by default? | **Nothing but report.** Cleanup is explicitly configured, per publisher. No `reason` attribute and no per-reason defaults | D-6, §4, §9 |
| Q4 | How does a publisher learn where to send? | The subscription API stays. DID-document discovery is out: `did:web` is one option among several, so nothing may be derived from the identifier | D-8, §5.2 |
| Q5 | Does cleanup stop at the TIL? | Yes — reporting the PAP policies and agreements is enough for now | §9 |
| Q6 | How should the trusted-issuers-list expose "unscoped"? | The scope stays **completely internal**; it may be exposed on the **management** API where needed, never in a TIR projection | TIL-2, TIL-3, §9 |
| Q7 | Does the payload need its own signature? | No | D-7 |
| Q8 | How is the publisher deployed at the trust anchor? | As a **separate release** — the `trust-anchor` chart gains no dependency on something called contract-management — reusing the consent-authority identity | §11, DSC-2 |
| Q9 | Which credential does the publisher present, and who trusts it? | Per-participant configuration, not a data space-wide governance parameter | §6 |
| Q10 | Can the ODRL policy constrain the issuer? | Not planned today; it can be contributed. The application-level check stays either way | D-5, DSC-3 |
| Q11 | Is there a specified protocol to align with? | Yes, and it is used: CloudEvents HTTP Protocol Binding (structured mode) and the CloudEvents HTTP webhook handshake, with the subscription request shaped after the CloudEvents Subscriptions API. The result is written up as an interoperability specification in this repository — see F8 below | D-9, DSC-3 |
| Q12 | Is ~2 minutes detection latency acceptable? | Yes | §8 |
| Q13 | Multi-data-space membership? | **Must be supported.** Local trust is attributed to the data space that caused it, and every reaction and sweep is per publisher | D-10, §9, §10 |

Three review findings changed the design rather than confirming it, and are worth naming:

- **Callbacks cannot be validated against a `did:web` domain.** Replaced by the CloudEvents webhook
  handshake, which proves the sink consents whatever DID method the subscriber uses (§5.2, D-8).
- **A participant may use many registries**, so the first draft's "everything in my TIL that is not
  in the TIR is stale" was simply wrong. Reconciliation is now per publisher over the entries that
  publisher's membership created (§10, D-10).
- **Publishers are runtime configuration.** The list is refreshable from the mounted ConfigMap
  rather than read once at start-up (§6).

## Follow-ups, answered (2026-10-29)

| # | Question | Decision | Where it landed |
|---|---|---|---|
| F1 | How is the data space scope spelled? | **Typed scope.** `scopeType` is added to the trusted-issuers-list beside `scope`, so a membership grant is `scope=<publisher id>`, `scopeType=dataspace`. No reserved prefix. Required to be backwards compatible, which it is: nullable column, optional parameter, and a call without `scopeType` keeps addressing the untyped grants | architecture D-11, §9, §10; **TIL-1** (new), TIL-2, CM-5 |
| F2 | Who else writes the membership scope? | The **onboarding portal is in scope for this epic**. It both creates membership trust (unscoped today) and removes it (by deleting the whole issuer today), so it is the other half of the attribution | **OBP-1** (new), §13, §15 |
| F3 | Should `full` mode exist? | **Yes**, kept as designed, with its warning | §9 |
| F4 | Is `adoptUnattributed` the right migration? | **Yes**, as designed: adopts for a single-publisher deployment, refuses when several are configured | §9, CM-5 |
| F5 | The publisher's DNS name is a second piece of identity | **Acceptable.** The receiver configures the publisher's `origin` beside its `id`; neither is derived from the other, for the same reason D-8 gives | §6, §11 |
| F6 | When does contract-management get a better name? | **Not in this epic** — it keeps its name. Instead the *configuration* is namespaced `dataspace.events` rather than `contract-management.events`, so a later rename costs nothing in deployed configuration | architecture D-12, §11; CM-1, CM-2, DSC-1 |
| F7 | Does the publisher list need a management API? | **Ignored for now**; self-service registration of publishers belongs to a future epic (it needs persistence, D-4) | ticket “out of scope” |
| F8 | What is the specification write-up for? | **Not a submission.** DSBA, Gaia-X and IDSA are unaffected by this change. DSC-3 writes the **interoperability contract** — event types and payloads, the OID4VP authentication binding, the §6 authorization rule, the subscription and handshake profile — as `doc/eventing/specification.md`, so a second implementation can interoperate. It can be carried outward later if anyone asks | DSC-3, §14 |

### What F1 and F2 changed in the design

- **Typed scopes (F1)** removed the one piece of the design that rested on a convention nothing
  enforces. The cost is a schema change in the trusted-issuers-list, which is why TIL-1 exists and
  why TIL-2/TIL-3 now depend on it; the benefit is that an order id can never be mistaken for a
  membership grant, whatever an order id looks like.
- **The onboarding portal (F2)** turned out to be more than a second writer. `unregisterIssuer`
  deletes the whole issuer, which is the `full` reaction applied unconditionally and from the other
  direction — so without OBP-1, removing a participant through the portal would keep destroying
  order-granted and other data spaces' entries no matter what any participant configured. It is
  also the one place that *knows* about a removal at the moment it happens, which makes direct
  publication an optional shortcut with the TIR watcher as the backstop.

## Nothing is open

Every question raised by this design has an answer. Two things are worth re-reading when
implementation starts, not because they are undecided but because they are easy to get wrong:

- **TIL-1's compatibility rule:** absent `scopeType` addresses the *untyped* grants, never “any
  type”. Read as “any type”, a `DELETE /issuer/{did}/credential?scope=<order id>` would start
  reaching membership grants.
- **OBP-1's fallback:** the portal talks to TILs it does not control and whose version it does not
  know, so it has to work against one that predates TIL-1.
