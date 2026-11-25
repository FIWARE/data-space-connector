# 0023 - Override one host: a pin is a given answer, an answer-level diff reports it, and hosts stay sub-domains

**Status:** Proposed
**Date:** 2026-11-25
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md#cross-cutting-values)

## Context

US-3.4 (#128) of [`USER_STORIES.md`](../config-tool/USER_STORIES.md) makes each derived host editable. An edited host
is pinned: it is not re-derived when the base domain changes, and Review says so. US-3.1
([ADR 0020](0020-config-tool-foundations-render.md)) shipped 22 `host.*` foundations, read-only, each derived as
`<component>.<base>`. Two public names were missing, the verifier and the DID document host, because
`k3s/provider.yaml` puts both on `mp-operations.org` and not on the reference base domain. ADR 0020 §7 already pins
*leaves* that a loaded file edited by hand, but a pinned host leaves its leaves unchanged, so that leaf diff never
mentions it. The ticket also asked for a second naming scheme, one host with path prefixes.

## Decision

### 1. A pin is a given answer

A pinned host is a given `host.<component>` answer in `profile.Answers`, and a derived host has none. The render
already lets a given answer override its default, so the profile needs no new field, keeps `format: 1`, and the pin
survives save and open. *Edit* pins only a text that differs from the derived value, and *Reset to derived* drops
the answer. Review reuses `render.HandSet` (the given answers that override a derived default, with the value they
would derive) and labels each entry *pinned, not updated*.

### 2. An answer-level diff next to the leaf diff

`render.DerivedDiff(b, prev, next)` walks the foundations whose default derives from other answers, in
`FoundationOrder`, and reports two lists:

- `rederived` (`{id, before, after}`): not given in `next`, and the resolved value changed. A reset counts here;
- `pinned` (`{id, kept, before, after}`): given in `next`, and the *derived* value moved from `before` to `after`.

It sits next to the leaf diff of [ADR 0020 §7-8](0020-config-tool-foundations-render.md#7-hand-edits-are-pinned-by-load-classification),
which it does not replace: the leaf diff tells what changes in values.yaml, the answer diff tells which answers
followed the base and which did not. `POST /api/render` returns it as `derived`. Its baseline is the previous render's
profile, and it is dropped on a load, a profile open, a source change and a model change. A `prev` on another chart or
version is no baseline: `resolve` refuses answers the bundle does not declare, so the diff is empty instead.

### 3. Two rule kinds and a `related` field

The `when` language of [ADR 0021](0021-config-tool-l2-rules.md) only compares answers with constants. Two kinds join
it, as bundle data, each instead of `field` and `when`:

- `distinct: {prefix}`: an error per pair of foundations under the prefix that resolve to the same host, ignoring
  case and a trailing dot. The message may use only `{{ .first }}`, `{{ .second }}` and `{{ .host }}`, validated at
  load. Rule `host-collision` uses `prefix: host.`, because the chart routes by host and one of the pair stops being
  reachable;
- `didWebHost: {did, host}`: an error when a `did:web` DID's host (up to the first `:`, percent-decoded, without port)
  is not the resolved host foundation. Rule `did-web-host` compares `did.value` with `host.did`.

A finding of either kind points `field` at one answer (for `distinct`, the pinned one when exactly one of the pair is
given, else the later one) and names the other in `related`, so Stage 3 and Review link both.

### 4. `features` on a foundation is display only

A foundation may list `features`. Stage 3 hides it while any of them is off, unless it is pinned or a finding names
it, so a pin can still be reset and a Review link lands on a field. A host lists the feature whose keys cover its
ingress, plus, behind APISIX, the feature that deploys the route's upstream; a drift test checks that mapping.
Render gating stays US-5.3 (ADR 0020, consequences): every host renders whatever the role, so the rules of §3 ignore
`features` too, and a profile with no role or a sparse `custom` set is still checked. US-5.3 gates render and checks
together.

### 5. Two more hosts, without their certificates

`host.verifier` (`verifier.<base>`) reaches the verifier's ingress host, its `deployment.server.host` and the
marketplace's SIOP verifier URL. `host.did` (the base itself) reaches the `did:` ingress host and `config.server.hostUrl`.
`did.value` now derives `did:web:{{ .host.did }}`, so the chain is base → `host.did` → DID: pinning `host.did`
re-derives the DID, and a pinned DID off its document host is the `did-web-host` error. The rendered values are
unchanged while `host.did` is derived.

The TLS block of both ingresses stays with US-8.11 (identity material): the `tls` list, its `secretName`, the
cert-manager annotations, the verifier's `clientIdentification.id` and the volumes that mount the secret. The secret
is the verifier's signing key or the DID key, mounted by other workloads. Deriving its name from the host would make
cert-manager issue it under a new name while the excluded mounts keep the old one, and the pods could no longer
start. A list is owned whole ([ADR 0020 §4](0020-config-tool-foundations-render.md#4-lists-are-owned-whole)), so
`secretName` cannot leave its list alone.

The cost: pinning `host.verifier` or `host.did`, or re-deriving them on a base change, moves the ingress host and the
URLs but not the certificate. A loaded file keeps its TLS block, so the pods keep their keys, but until US-8.11 the
certificate still names the old host. A fresh profile emits no TLS block for these two ingresses. The help of both
foundations says so.

### 6. No path-prefix scheme

Every host is a sub-domain of the base domain, `<component>.<base>`, and there is no scheme selector. wistefan decided
this in his review of the US-3.4 plan (PR #14, 2026-11-24): "do not use path-prefixes, use sub-domains to the
base-domain instead". So `distinct` compares hosts only, and there is no `host.scheme` or `path.*` foundation.

## Consequences

- A domain move with a pinned verifier is visible at every step: the diff, the `pinned` badge and Review.
- The pin state is plain profile data; a CLI `generate` honours it, and refuses a profile whose hosts collide.
- Hidden hosts are still checked, so a feature that is off cannot hide a collision.
- US-8.11 has to move the verifier's and the DID's certificate with their host.
- A new derived answer joins the answer diff and Review without code, as long as its default references other answers.

## Alternatives

- **A `pinned` list in the profile.** It would duplicate what a given answer already says, and the two could disagree.
- **Report pinned hosts through the leaf diff.** Their leaves do not change, so there is nothing to report there.
- **A general expression language for rules.** Two closed kinds cover the cases, validate at load and keep messages
  to fixed keys.
- **Gate the rules by `features` now.** The render is not gated, so the hidden host still lands in values.yaml and
  still collides.
- **Own the verifier's and the DID's TLS block here.** Without the mounts, which belong to US-8.11, a renamed secret
  breaks the pods.
- **A path-prefix scheme.** Declined in review (§6).
