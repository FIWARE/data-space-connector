# 0021 - L2 rules are bundle data evaluated by a core checker into findings addressed by open field

**Status:** Proposed
**Date:** 2026-11-24
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md#validation-rules)

## Context

US-3.2 (#126) of [`USER_STORIES.md`](../config-tool/USER_STORIES.md) adds the participant's DID as a foundation
(`did.method`, `did.value`, ADR 0020) and asks for the first cross-field rule: `production` with a `did:key` gets a
warning. [`FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md) lists L2 rules, the tool's own cross-field checks
between the L1 schema and the L3 render. No rule engine existed: `internal/validate` is the L1 schema stub, and
`model.Rule` / `Bundle.Rules()` already name the `recognize:` rules of ADR 0014.

## Decision

### 1. Rules are bundle data

A bundle declares `rules:`, a list of `{id, severity, stage, field, when: [{field, equals}], message, doc}` in
`model/<version>/bundle.yaml`, not in a separate `rules.yaml`. The Go type is `model.CheckRule`, read by
`Bundle.CheckRules()`. The trust-anchor bundle has none.

### 2. Conjunctive `equals` conditions

A rule holds while every `when` holds. A condition compares one field with one string. Conditions read the
**resolved** answers (`render.Answers`): a derived default counts, so `production` with `did.method` unanswered
does not warn, because the method derives `did:web`. An unanswered field never matches. A rule that needs another
operator adds it to `RuleWhen`, with a load check and an amendment to this ADR.

### 3. Fields are open-field ids

`field` (what the finding points at) and every `when.field` are `target.environment` or
`answers.<declared foundation id>`, the ids the open-question marks and the UI links already use. The addressable
Target fields are a named table in `internal/model`; a test in `internal/check` keeps it in step with
`profile.Target`.

### 4. Validation at bundle load

`readBundle` refuses a rule with a missing or duplicate id, an unknown severity or stage, a missing message or
`when`, a field that is not addressable, a `stage` other than the stage that shows `field`, an `equals` that is not
one of the field's values (a choice's options, the environments), and a `doc` that is not a clean repo-relative path.
A test checks that every shipped rule's `doc` file exists.

### 5. Warning and error

A `warning` is shown and never blocks. An `error` blocks `generate`: `check.Blocking(findings)` is true only for an
error. Findings come back sorted by severity (errors first), stage (wizard order) and rule id.

### 6. A core package, not `internal/validate`

`check.Evaluate(bundle, profile)` lives in core package `internal/check` and does not import `net/http`.
`internal/validate` stays the L1 schema check: it answers "does the chart accept these values", L2 answers "is
this combination a mistake". Keeping them apart lets L1 grow on the Helm SDK without dragging the profile in.

### 7. The DID targets narrow the story

The DID is the first foundation whose target list narrows an acceptance criterion. US-3.2 names the verifier, the
IdentityHub, the issuer, the registration job, `dataSpaceConfig` and APISIX. The bundle comment above the
`did.value` targets carries the same narrowing:

| AC consumer | What US-3.2 does | Reason | Owning story |
|---|---|---|---|
| Registration job: TIL `registration.issuer[0].did` | not a target | Targets may only index into a list the bundle owns whole, and the issuer list also names other participants | US-3.3 |
| Registration: the verifier's TIR curl script (`vcverifier.deployment.initContainers[0].args[0]`) | not a target | A k3s-local shell script inside a list with content no foundation owns | US-3.3 |
| `dataSpaceConfig` | nothing | Chart 10.16.0 has no DID key there | none until the chart adds one |
| APISIX | nothing | Chart 10.16.0 has no DID key there | none until the chart adds one |
| Keycloak signing key `keycloak.signingKey.did` | not a target | Emitting it alone yields a `java-keystore` provider with a null store, password, alias and algorithm; it belongs with the keystore block | US-5.5 |
| IdentityHub `identityhub.bootstrap.did` | not a target | With a bare `holder.kid` it fails the chart's `assertHolderKid`; with a `<did>#<keyAlias>` kid it needs `keyAlias`, which the tool does not own | US-8.11 |
| EDC `fdsc-edc.{common,deployment.<name>}.config.oid4vp.holder.kid` | not a target | A key id whose form (`<did>` or `<did>#<keyAlias>`) depends on the IdentityHub bootstrap | US-8.11 |

The fdsc-edc targets (`participant.id`, `issuerId`, `oauth.clientId`, `oauth.secretAlias`, `oid4vp.holder.id`) sit
under `fdsc-edc.common` only. fdsc-edc 0.7.9 ranges over `.Values.deployment` without an enabled gate and merges
each entry over `common`, so any key under `fdsc-edc.deployment.<name>` would create a controlplane. The deployment
names are the user's, so the tool cannot own them.

## Consequences

- A new rule is a bundle edit and a revision bump; a new operator or addressable field is a code change and an
  amendment to this ADR.
- The UI can label and link a finding from its `stage` and `field` without knowing the rule.
- A loaded hand-written file (such as `k3s/dsp-provider.yaml`) pins every DID leaf, because no `recognize:` rule
  matches a DID key: a `domain.base` change moves none of them, and nothing splits.
- A generated file whose owned block holds the DID targets, with `identityhub.bootstrap.did` and `holder.kid`
  hand-added outside it, drifts on a `domain.base` change: the owned targets move to the new DID, the hand-added pair
  stays on the old one. The pair stays consistent, so `assertHolderKid` passes, but `bootstrap.did` no longer equals
  `fdsc-edc.common.config.edc.participant.id`, as `values.yaml` requires. That is accepted until US-8.11 owns the
  pair; a render test pins it.
- The fdsc-edc targets are not feature-gated (US-5.3), so every render emits them. They are inert while the
  component is off and never add a workload.

## Alternatives

- **A separate `rules.yaml` per version**, as `FEATURE_MODEL.md` sketched. A second file per bundle with no gain:
  the rules reference the foundations, which load validation already has at hand.
- **Rules in Go code.** Not checkable against the bundle and repeated per chart version.
- **An expression language (CEL, templates) for conditions.** An open language cannot be validated at load or
  traced to fields; one operator covers the first rule.
- **Folding L2 into `internal/validate`.** Mixes the chart's schema with the tool's opinions, see §6.
