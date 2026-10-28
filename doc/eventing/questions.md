# Decisions and open follow-ups

The questions this design opened were answered in the design review of 2026-10-28. This file keeps
the record, and collects the new questions the rework raised — all of them small enough to settle
during implementation, none of them blocking.

## Answered

| # | Question | Decision | Where it landed |
|---|---|---|---|
| Q1 | One component or two? | `contract-management` carries both ends for now; **the component may have to be renamed** once it is no longer only about contracts | architecture D-1, [F6](#f6--when-does-contract-management-get-a-better-name) |
| Q2 | Best-effort or durable delivery? | Best-effort is fine; no party needs proof of delivery | D-4, §7 |
| Q3 | What should a removal do by default? | **Nothing but report.** Cleanup is explicitly configured, per publisher. No `reason` attribute and no per-reason defaults | D-6, §4, §9 |
| Q4 | How does a publisher learn where to send? | The subscription API stays. DID-document discovery is out: `did:web` is one option among several, so nothing may be derived from the identifier | D-8, §5.2 |
| Q5 | Does cleanup stop at the TIL? | Yes — reporting the PAP policies and agreements is enough for now | §9 |
| Q6 | How should the trusted-issuers-list expose "unscoped"? | The scope stays **completely internal**; it may be exposed on the **management** API where needed, never in a TIR projection | TIL-1, TIL-2, §9 |
| Q7 | Does the payload need its own signature? | No | D-7 |
| Q8 | How is the publisher deployed at the trust anchor? | As a **separate release** — the `trust-anchor` chart gains no dependency on something called contract-management — reusing the consent-authority identity | §11, DSC-2 |
| Q9 | Which credential does the publisher present, and who trusts it? | Per-participant configuration, not a data space-wide governance parameter | §6 |
| Q10 | Can the ODRL policy constrain the issuer? | Not planned today; it can be contributed. The application-level check stays either way | D-5, DSC-3 |
| Q11 | Is there a specified protocol to align with? | Yes, and it is used: CloudEvents HTTP Protocol Binding (structured mode) and the CloudEvents HTTP webhook handshake, with the subscription request shaped after the CloudEvents Subscriptions API. The result is to be written up for the DSBA/Gaia-X/IDSA context | D-9, DSC-3 |
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

## Open follow-ups

### F1 — How is the data space scope spelled, and is a reserved prefix acceptable?

Membership grants are written under `dataspace:<publisher id>` in a scope namespace that otherwise
holds ProductOrder ids. That is a reserved prefix in a free-form field: an order whose id began with
`dataspace:` would collide, and nothing in the trusted-issuers-list enforces the convention.
Alternatives: a typed scope in the TIL (`scope` + `scopeType`), or a scope that is simply the
publisher id with no prefix (then "is this scope a data space?" is answered by the configuration,
not by the string). *Lands in TIL-1/CM-5.*

### F2 — Who else writes the membership scope?

The chart's registration job will (DSC-1), but entries also arrive through the onboarding portal and
by hand. Anything that creates membership-derived trust without the scope leaves an entry the
mechanism cannot act on. Is the onboarding portal in reach for this epic, and should the operator
documentation simply state the convention? *Lands in DSC-1/DSC-3.*

### F3 — Should `full` mode exist?

`full` deletes the issuer and with it the entries of every *other* data space and every
hand-managed one. It is the only mode that can act on unattributed entries, which is why it is
there, but it is also the one mode that contradicts D-10. Keep it with a loud warning, or drop it
and let `adoptUnattributed` plus `dataspace` cover the case? *Lands in CM-5.*

### F4 — Is `adoptUnattributed` the right migration?

For a participant that is a member of exactly one data space it treats every unattributed entry as
that data space's membership grant, which is true in most existing deployments and false in the ones
that trust a partner from outside. It refuses when several publishers are configured. Is that the
migration we want, or should existing entries simply be re-registered with a scope by an operator
script? *Lands in CM-5/DSC-1.*

### F5 — The publisher's DNS name is a second piece of identity

The webhook handshake works on origins — DNS names — while everything else in the mechanism works on
DIDs. A receiver therefore has to know both for the same publisher (`id` for authorization, origin
for `OPTIONS /events`). Acceptable, or should the origin be derived from the registered
subscription instead of configured at the receiver? *Lands in CM-1/CM-2.*

### F6 — When does contract-management get a better name?

The review accepted the component as the home of the mechanism "for now", with a rename expected.
A rename touches the chart, the image, the values and every deployment, so it wants its own ticket
and a deprecation path — and it is cheaper before the eventing values are published than after.
*Needs a decision before DSC-1 freezes the values.*

### F7 — Does the publisher list need a management API?

Refreshable configuration covers "the operator adds a publisher". It does not cover "a data space
registers itself at a participant", which would need persistence and so a database (D-4). Is
self-service registration of publishers ever wanted? *Lands in CM-1.*

### F8 — Who owns the specification write-up, and which body first?

DSC-3 says the write-up happens; it does not say whether DSBA, Gaia-X or IDSA is the first audience,
or who carries it there. *Needs an owner.*
