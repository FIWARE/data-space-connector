# 0012 - Attribute local trust to the data space that caused it

**Status:** Accepted
**Date:** 2026-11-20
**Context doc:** [`../eventing/participant-setup.md`](../eventing/participant-setup.md#which-trust-comes-from-the-membership)

## Context

A participant's Trusted Issuers List (TIL) holds entries of several origins: grants made because an
organization is a member of a data space, grants made by contract-management when a product order
is activated, and entries made by hand or before this feature. A participant may be a member of
several data spaces, and may trust issuers that belong to none of them. A removal announced by one
data space therefore cannot be answered by looking at the TIL as a whole.

The TIL already attributes each credential to the **scope** that granted it; the scope holds
ProductOrder ids today and is free-form.

Decided in the design review of 2026-10-28 as D-6, D-10 and D-11
([architecture §12](../eventing/architecture.md#12-decision-register), §9 and §10).

## Decision

- **Attribution through the existing TIL scope, with a type (D-6, D-11).** A membership grant is
  written as `scope=<publisher id>` with `scopeType=dataspace-membership`. The type is a nullable
  column and an optional query parameter; a scoped call without `scopeType` addresses only the
  **untyped** grants, so order grants (`scope=<order id>`, no type) cannot collide with a membership
  grant. The architecture writes the type as `dataspace`; the TIL API enum
  `dataspace-membership` is what is deployed. The chart's `registration.issuer` writes membership
  grants this way as soon as a `participant.removed` publisher is configured.
- **The reaction is configured per publisher and defaults to `report` (D-6).**
  `on-participant-removed.mode` is one of:
  - `report` (default): record and report only; the TIL is not changed;
  - `dataspace`: drop this data space's membership grant;
  - `granted`: `dataspace`, plus every order-scoped grant to that identifier;
  - `full`: drop the issuer entirely, including other data spaces' and unattributed entries.

  `overrides` set the mode per identifier; identifiers on `keep` are never touched by any mode or
  sweep.
- **Multi-data-space membership is supported (D-10).** Every reaction and every reconciliation
  sweep is per publisher and touches only the entries attributable to that data space (except
  `full`, which is documented as blunt).
- **Unattributed entries are never removed automatically** in any mode but `full`; they are listed in
  the report. For a participant that is a member of exactly one data space,
  `adopt-unattributed: true` treats them as that data space's membership grants; it refuses to act
  with more than one publisher configured.
- **The report is part of the feature.** `GET :9090/participants-removed` lists, per publisher and
  removed identifier, what is held, under which scopes, and what was done.

## Consequences

- Installing the feature changes no trust configuration: `report` writes nothing, and the
  integration test asserts that (the report-mode scenario in
  [EVENTING.md](../EVENTING.md#integration-tests)).
- A participant in two data spaces keeps the other data space's grant when one removes the
  organization.
- Existing deployments carry unattributed entries the mechanism cannot act on. They appear in the
  report; `adopt-unattributed` is the migration path for single-membership deployments.
- The marking and the writing modes need a TIL that understands typed scopes (0.10.1 for the
  marking, 0.10.2 for the writing modes and a complete report, 0.10.3 for `adopt-unattributed`)
  and contract-management ≥ 3.5.4, which sends the type as the TIL expects
  ([participant setup](../eventing/participant-setup.md#requirements)).
- ODRL policies, Rainbow/EDC agreements, running access tokens and credentials already issued to
  the organization's users are not touched by any mode. The review found reporting them enough:
  policies and agreements belong to orders, which the default keeps, and tokens are short-lived.
- The onboarding portal removes a participant from a TIL with `DELETE /issuer/{did}`, which also
  drops order and other data spaces' grants; it should register and remove under the membership
  scope instead (OBP-1, outside this repository).

## Alternatives

- **A single on/off switch, or always removing everything.** An action in another namespace must not
  silently delete trust configuration here.
- **A `reason`-driven default.** Rejected in the review together with the switch.
- **Treating the TIL as belonging to one data space, with a `keep` list as the only guard** (first
  draft). No removal could be answered safely for a participant in several data spaces.
- **A reserved `dataspace:` prefix in `scope`** (first draft), or a bare publisher id as the scope
  with the configuration deciding what it means. Nothing enforces a prefix convention, and an order
  id could collide with it.
- **Diffing the TIL as a whole against one TIR** for reconciliation (first draft). A participant
  uses many registries, so "in my TIL but not in that TIR" says nothing about staleness.
