# 0026 - Model Stage 4 dependencies as three modes compiled into mode-gated synthetic foundations

**Status:** Proposed
**Date:** 2026-11-27
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md)

## Context

US-4.1 (#131) of [`USER_STORIES.md`](../config-tool/USER_STORIES.md) opens Stage 4 (*Infrastructure*) with its first
dependency, cert-manager. A dependency can be met in three ways: `bundled` (installed with the connector), `cluster`
(already in the cluster) and `external` (provided by hand, for cert-manager one TLS Secret per host). Each way asks its
own questions and writes its own chart keys, and several ways write the same key with different values
(`cert-manager.enabled` is written by all three). US-4.2 to US-4.6 add the databases, Vault and detection with the same
model.

Stage 3 already has an engine that turns answers into values: foundations with targets, rendered with provenance and
pinning, owned on load, and listed as requirements ([ADR 0020](0020-config-tool-foundations-render.md),
[ADR 0025](0025-config-tool-proxy-and-private-ca.md)). The engine writes each leaf from one place only, and it has no
way to say "only while this mode is chosen".

## Decision

### 1. `infrastructure:` and `ingresses:` in the bundle, compiled into synthetic foundations

A bundle declares its dependencies under `infrastructure:` as `{id, title, help, doc, clusterWide, modes}`. A mode is
`{id, label, note, warning, doc, development, productionDoc, targets, questions, perIngress, perIngressQuestion,
requirements}`, and a question has the same fields as a foundation plus `when: {question, equals}` and `readOnly`. The
bundle compile turns each dependency into ordinary compiled foundations with the reserved prefix `infra.`:

- `infra.<dep>.mode`, a choice of the dependency's modes without a default, so a mode nobody chose renders nothing;
- `infra.<dep>.<question>` for every question of every mode;
- `infra.<dep>.<question>.<ingressId>` for each row of the `ingresses:` table when a mode has a `perIngressQuestion`.

Templates read them fully qualified (`{{ .infra.certManager.issuer }}`), so reference checks, topological order and path
checks run unchanged over the combined list. Render, load ownership, pinning, provenance and requirements therefore
keep working without a second engine.

The synthetic foundations are in `Bundle.CompiledFoundations()`, `FoundationOrder()` and `Foundation(id)`, but never in
the declared `b.Foundations`, which is Stage 3's list. Every caller that ranges over foundations chooses which list it
means.

Ids are one `idSegment` each: dependency (`certManager`, not `cert-manager`), question and ingress. Only the per-ingress
expansion produces a two-segment local id. Uniqueness across modes, the reserved id `mode` and the nesting check run on
the expanded ids, and a plain question that shares an id with a per-ingress question is refused with both named. A
declared foundation id may not start with `infra.`.

### 2. Mode gating with `When`

A compiled target gains `When []TargetCondition`, where `TargetCondition{ID, Equals}` names a compiled answer id. Every
condition must hold over the resolved answers, and an unset answer never holds. Each target of a mode gets
`infra.<dep>.mode equals <mode>`, and a question's `when` (the local id of a question in the same mode) is qualified and added to it.

Two targets may write the same leaf only when they are *exclusive*: both carry a condition on the same id with
different values. `Only` counts as a condition on the target's own foundation. So all three cert-manager modes write
`cert-manager.enabled`, and `bundled` and `cluster` both write the `cert-manager.io/cluster-issuer` annotation.

A top-level `const` target used to need `requires`, because an ungated constant would always render. A non-empty `When`
is a gate too, so mode targets are plain `const` targets. An unconditioned top-level `const` is still refused.

A condition compares a string, so it may only name a string-typed answer (a choice, string, hostname, ...); a `when` on a
`bool` or `credentialTypes` answer is a compile error, because it could never hold. A question `when` is checked on its
own, so a gated question without targets is refused too. A question `when` may not name a question that has a `when` of
its own: conditions do not chain, so a shown question never depends on a hidden one, and two questions cannot gate
each other.

Requirement rows (Secrets listed for NEXT_STEPS.md) get the same `When` gating and an optional `keys` list (exactly one
of `key`/`keys`). A mode declares its own rows under `requirements:`, gated on that mode. A `perIngressQuestion` may
carry one `requirement:` without a `name`: it expands per row, the answer is the name, and it also needs the row's owner.
`requirements.Secrets` lists each Secret once and merges the keys of rows that name the same one. In `external`, each
per-ingress Secret is listed once, with keys `tls.crt` and `tls.key`, and only while that mode is chosen.

In `Render`, the `When` check runs before refs and requires. A target of an inactive mode is skipped without marking
its lists incomplete, so it never drops the TLS blocks of the active render.

### 3. One `ingresses:` table and replacing Secret names

The top-level `ingresses:` table lists every ingress whose TLS the tool owns: `{id, label, path, owner}`. `path` is the
ingress block, and `owner` is the foundation whose `<path>.tls[0].secretName` target exists. Compile checks run both
ways: every row's owner has that target, and every such target has a row. The verifier and DID ingresses are not rows,
because their certificate is the signing key (US-8.11).

A mode's `perIngress` targets (a key relative to `path`) and its `perIngressQuestion` expand per row. Each expanded
target requires the owner target's refs, its own `requires` and the owner foundation id. APISIX's owner target is a
ref-less `const`, so without the owner id its annotation would render while `domain.base` is open.

The `external` mode's Secret question writes `<path>.tls[0].secretName` with `replaces: true`. An unconditional
foundation target must exist at that path. The question's default is copied from that target's template or constant,
so a derived answer equals today's `<host>-tls`. At render time a rendered replacing leaf drops the replaced one.
Switching away from `external` brings `<host>-tls` back on its own. No other target may share a path with an
unconditional one.

### 4. One field-id scheme

`infrastructure.<dep>` names a mode and maps to `infra.<dep>.mode`. `infrastructure.<dep>.<question>` names a question
and maps to `infra.<dep>.<question>`. The split is at the first dot, so the question may be an expanded
`secret.keycloak`. `model` owns the mapping (`InfrastructureFieldPrefix`, `Bundle.InfraFieldAnswer`,
`InfraAnswerField`), and every place that names a field uses it: field errors, open fields, check-rule `field`/`when`
and UI links. An `answers.infra.*` id is refused, because those answers live only under the profile's `infrastructure`
map. A rule naming an unknown dependency or question is a compile error.

### 5. Notes are bundle data

`clusterWide: true` requires the `bundled` mode to carry `warning` and `doc`. `development: true` (suitable for
development only) requires `productionDoc`. The new type `email` (`local@domain`, refusing the RFC 2606 example and
test names) checks the ACME contact, so the chart's `admins@example.com` can never be typed in.

### 6. cert-manager

| Mode | Mode targets | Questions |
|---|---|---|
| `bundled` | `cert-manager.enabled`, `cert-manager.crds.enabled`, `certManagerResources.enabled`: `true` | `type` (`selfsigned` default, `prod`), `acmeEmail` and `ingressClass` while `prod`, read-only `namespace` while `selfsigned`; issuer annotation `<type>-issuer` per ingress |
| `cluster` | `cert-manager.enabled`, `certManagerResources.enabled`: `false` | `issuer`, the ClusterIssuer name, copied to each ingress's issuer annotation |
| `external` | as `cluster` | `secret.<ingressId>`, replacing each ingress's `tls[0].secretName` |

In `bundled` with `prod`, an open `acmeEmail` or `ingressClass` would let the chart defaults apply (`admins@example.com`,
`nginx`), and the issuer would silently never issue. The error rules `acme-email-for-prod-issuer` and
`acme-ingress-class-for-prod-issuer` block `generate` in that case ([ADR 0021](0021-config-tool-l2-rules.md)).

## Consequences

- Stage 4 is bundle data. US-4.2 to US-4.6 add dependencies without new render code.
- The profile stores `infrastructure: {<dep>: {mode, answers}}` and keeps the answers of inactive modes, so switching
  back restores them. Only the active mode renders.
- Load does not derive `infra.*` answers. Several modes write the same paths, so a one-rule-per-path `recognize:` map
  cannot express them, and a per-target inversion would leave even a tool-written file ambiguous. Owned infra leaves
  stay owned and are listed as underived, and the mode comes from the saved profile. Whole-mode inversion is left to
  US-4.6 detection.
- Every `range b.Foundations` site has to choose between the declared and the compiled list. A test pins that Stage 3's
  list does not change.
- In `external`, the host foundations' `cert-manager.io/common-name` and `alt-names` annotations stay. They are inert
  without an issuer annotation.

## Alternatives

- **A second engine for Stage 4.** It would duplicate render, provenance, pinning, ownership and requirements.
- **Store the answers in `profile.answers` under `infra.*`.** The ticket asks for an `infrastructure` map per
  dependency, and two places for one answer would need a precedence rule.
- **Gate with `Only` alone.** `Only` conditions a target on its own foundation's value, while a question's targets
  depend on the mode answer and on another question.
- **Let the last writer win on a shared path.** The outcome would depend on bundle order, and the path check could no
  longer catch two foundations writing one key by mistake.
- **Namespaced `Issuer`s in `cluster` mode.** The one question names a ClusterIssuer. A hand-set
  `cert-manager.io/issuer` stays an override.
