# 0025 - Declare the egress proxy and the private CA once: keyed list elements, per-element targets and gated targets

**Status:** Proposed
**Date:** 2026-11-26
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md#cross-cutting-values)

## Context

US-3.6 (#130) of [`USER_STORIES.md`](../config-tool/USER_STORIES.md) asks for an egress proxy (`proxy.https`,
`proxy.http`, `proxy.no`) and a private CA (`ca.privateSecret`), each declared once in Stage 3's *Egress* panel and fanned
out to every component that calls out: the verifier, Keycloak, the marketplace logic proxy, the vc-operator and APISIX.
Each component takes them through its own mechanism, listed in
[PREREQUISITES](../deployment-integration/roles/PREREQUISITES.md#private-ca).

Most of those targets are not scalars. They are elements of env, volume, mount or init-container lists that other
stories fill too, or keys inside the APISIX routes that US-5.6 owns. Until now a target could only write a scalar at a
fixed path, an index into a list the tool owns completely, and a `const` only below such an index
([ADR 0020](0020-config-tool-foundations-render.md), [ADR 0024](0024-config-tool-resource-profile.md)). Helm replaces
lists as a whole, so writing one element means writing the complete list.

## Decision

### 1. Keyed elements and the `lists:` table

A target may write one element of a list, identified by a key field:
`{path: keycloak.extraEnvVars, element: {key: KC_TRUSTSTORE_PATHS, value: {value: ...}}}`. String scalars inside
`value` are templates over answers. The target never writes the whole list.

Every list that keyed elements write into is declared once in a top-level `lists:` table:
`{path, key, unique: [fields], base: [elements]}`. The render writes the complete list:

- **Loaded file holds the list:** the loaded elements in their order, with each owned key replaced in place, then the
  owned elements that were missing, in bundle order.
- **Otherwise:** the `base` elements (the chart's defaults), then the owned elements in bundle order.

Elements with keys the tool does not own are never changed. A loaded element with an owned key but different content is
a hand edit: it is kept and pinned, with the rendered element as `Rendered`, exactly like a scalar pin. A loaded null
(`key:`, `key: ~`) is an empty loaded list: Helm deletes a key set to null, chart default included, so the base is not
written. Any other loaded value that is not a sequence is a hand edit of the whole list, and every mechanism with an
element there is withheld (see 2). Leaves keep concrete indices, so `render.Tree`, `Nested()` and the leaf JSON
are unchanged. Provenance gains `element` (the key) and `origin` (`base` or `loaded`) for elements the render writes but
does not own. Such a leaf carries the first foundation, in bundle order, whose element makes the list be written, so
the Stage 3 diff files the base under that foundation. Appending beyond a loaded list's length is left to US-7.1's writer.

Loading needs no marker on a sequence item: a loaded element whose key matches an owned keyed target is owned, and so is
an `each` leaf under any element that has its `within` mapping. Their leaves back-derive the answers by inverting the
target's templates (`model.Invert`); the first element in document order sets an answer, and a leaf that disagrees, or
a field the target does not write, stays unknown and pins the element. So does the key leaf of an element that lacks a
field the target writes. An element or `each` leaf is owned only while the render would write it: every `refs` and
`requires` id of its target, and of every target of its mechanism, must be among the derived answers or follow from
them by a default, and the render must not withhold its mechanism (a `unique` collision with a foreign or pinned
element, or a list loaded as a non-sequence). Otherwise its key leaf (or the `each` leaf) stays unknown with a note,
and the load classifies again without the answers that rule had derived. A template field matches only a string
scalar, since the render compares typed values. A reserved ref such as `{{ .target.namespace }}` reads as any text: the load report does not
depend on the Stage 1 target, which may change after the load, so an element rendered for another release or namespace
is reported as owned. The render compares the whole element with what it writes for the current target and pins it
all the same. The Stage 3 diff reports a pinned element once,
under its key (`keycloak.extraEnvVars[name=HTTPS_PROXY]`), with the whole rendered and kept elements.

Compile-time checks: a keyed element's list must be in `lists:`, and no target may also index into it. Two targets may
not own the same key, and an owned key may not be a `base` key. An element that reads no answer needs `requires`.

### 2. Fields that must be unique besides the key, and withheld mechanisms

Kubernetes rejects two `volumeMounts` with the same `mountPath` in one container. `k3s/provider.yaml` mounts its own
`root-ca` at `/etc/ssl/cert.pem`, which is where the verifier CA mechanism mounts `ca-bundle` too. A `lists:` row
therefore declares `unique: [mountPath]`, and keyed targets that only work together carry the same `mechanism: <id>`
(for example `verifier-ca`: volumes, the mount and the init container).

- **Compile time:** two owned elements, or an owned and a `base` element, may not share a `unique` value. `mechanism`
  is allowed on keyed targets only.
- **Render time:** when an owned element would share a `unique` value with a foreign element of the final list, the
  whole mechanism is **withheld**. None of its targets render, and the foreign element stays as it is. A mechanism with
  a target that is not ready (unanswered refs or requires) renders nothing either. A ready mechanism with an element in
  a list the loaded file holds as a scalar or mapping is withheld the same way, so it never writes its other lists.
  `Tree.Withheld` lists each case as `{reason, mechanism, element, list, field, value, foreignKey, conflict}`, with
  reason `collision`, `edited` (without field, value and foreign key) or `conflict` (see below). The result is never an
  invalid manifest.
- **Conflicts with what the chart templates itself:** a `unique` check only sees list elements. The vcverifier chart
  (decentralized-iam 2.1.30, `vcverifier/templates/deployment.yaml` L48-50, L157-163) mounts its own `ca-bundle` at
  `/etc/ssl/cert.pem` while `serverCertificates.clientKey.secretName` and `clientIdentification.certificatePath` are
  set, and renders its `add-root-ca` init container for `serverCertificates.caCert.enabled` as soon as
  `deployment.initContainers` is non-empty, which `add-private-ca` makes it. A top-level `mechanisms:` table therefore
  lists, per mechanism id, `conflicts: [{whenSet: [paths]}]`. While the loaded file sets every path of one entry as a
  Helm `if` reads it (not absent, null, false, zero or empty), the mechanism is withheld with reason `conflict` and the
  paths in `conflict`. No target may write a conflict path, so only a loaded file can set one.
- **Report:** a withheld CA mechanism is a `caCoverage` *warning*, for either reason. The foreign element is most likely a hand-made trust
  setup copied from PREREQUISITES, which the tool cannot inspect, and an error would block `generate` until the user
  deleted a working setup.

### 3. `lists:` bases are pinned against the layer that supplies the default

A base that misses a default element drops it silently once the tool writes the list. Each base is compared with the
umbrella `values.yaml` when it declares the list, and otherwise with a committed fixture of the subchart's defaults
(`internal/render/testdata/subchart-defaults/<chart>-<version>.yaml`, with the `helm pull` command that produced it, and
the effective default after every layer for nested charts). The test also checks that the fixture version equals the
dependency version in `Chart.yaml`, so a subchart bump fails until the fixture is pulled again.

### 4. Per-element (`each`) targets

`{path: apisix.routes, each: plugins.openid-connect.ssl_verify, ...}` writes a key into every element of the loaded list
that already holds the parent mapping. `within` names a shorter prefix of `each` to look for instead, so
`{each: plugins.openid-connect.proxy_opts.https_proxy, within: plugins.openid-connect}` creates the missing
`proxy_opts`. A loaded node between `within` and the leaf that is not a mapping (`proxy_opts: null`, a scalar or a
list) is a hand edit: the leaf is pinned and nothing is written over it. It never creates elements, so routes stay US-5.6's. Without routes nothing renders
for APISIX, and the Stage 3 preview says so. A loaded value that differs is pinned as a hand edit. `each` is refused on a
list the `lists:` table declares.

### 5. `requires` and `defaultWhen: {set: true}`

Any target may list `requires: [<answer id>]` and then renders only while those answers are given or derived. A `const`
outside a list index is allowed with `requires`, so `keycloak.proxy.enabled: true` renders only with `proxy.https`, and
an unanswered question still emits no key. The fan-out a foundation reports (`render.Targets`) counts a
target reached through `requires` as well as through its refs, and lists every target of a mechanism one of whose
targets is reached, because the mechanism renders as a whole.

`defaultWhen` takes either `equals` or `set: true`. `proxy.http` and `proxy.no` carry `defaultWhen: {id: proxy.https,
set: true}` and every one of their targets carries `requires: [proxy.https]`. Without both gates the `proxy.no` default,
which reads only reserved refs, would derive for every profile and render `NO_PROXY` into five components without a
proxy. `proxy.https` is the switch for the whole group; an HTTP-only proxy is not offered. Conversely, every
`proxy.https` and `proxy.http` target also carries `requires: [proxy.no]`: while Stage 1 leaves the release name or
namespace unanswered, the default cannot derive, and a proxy without `NO_PROXY` would send every in-cluster call
through it. The group then renders nothing until the user gives `proxy.no` or completes Stage 1.

### 6. Reserved refs `target.release` and `target.namespace`, and `fullname`

Both join `target.environment` and are seeded from the profile's Stage 1 target. Only defaults may read reserved refs.
The template function `fullname <release> <chart>` mirrors Helm's standard rule (`cloudpirates.fullname`,
`fiwareCommon.names.fullname`): the release alone if it contains the chart name, otherwise `<release>-<chart>`, truncated
to 63 characters with a trailing `-` trimmed.

### 7. `NO_PROXY` is one string that every client reads

The verifier and vc-operator (Go `httpproxy`), Keycloak 26.7 (`ProxyMappings`), the BAE logic proxy (`proxy-from-env`)
and APISIX (`lua-resty-http`) match entries differently, and no wildcard works in all four. The default is the union
`localhost,127.0.0.1,svc,.svc,cluster.local,.cluster.local,<namespace>.svc,` followed by the bare in-cluster service
names. An entry that one client reads differently matches no real host there and does no harm. The names come from where
the chart fixes them (`fullnameOverride`, a service `nameOverride`, a literal in a template). Only the Keycloak and OTel
Collector names use `fullname` over the release. Their charts also rename the Service through `nameOverride`, which
replaces the chart name inside `fullname` rather than the whole name. The rows are bundle data (`noProxyServices:` with
`name`, `overrides:` and an optional `nameOverride:` path; the effective name is the first set `overrides` value, else
`name` rendered with a set `nameOverride` in place of the chart name), so a test and the `noProxyCoverage` warning read
the same table. That warning fires when a loaded file
renames a service the final `proxy.no` does not bypass. A given `proxy.no` is never rewritten. No CIDR placeholder goes
into the value: CIDRs only help the Go clients dialling IP literals, so the field help suggests appending the pod and
service CIDRs, which the vc-operator needs to reach the Kubernetes API directly.

### 8. Per-component mechanisms

- **Keycloak proxy:** `keycloak.proxy.{enabled,httpsProxy,httpProxy,noProxy}`. Keycloak reads these env vars only while
  `proxy-mappings` is unset, so the tool never writes `spiProxyMappings` and warns when a loaded one is non-empty.
- **Keycloak CA:** `truststore-paths` (PEM) via keyed elements in `keycloak.extraVolumes`, `extraVolumeMounts` (under
  `/opt/keycloak/conf/truststores/`) and `extraEnvVars` (`KC_TRUSTSTORE_PATHS`). `tls.truststore*` in the CloudPirates
  chart 0.22.0 configures inbound mTLS only, so the PREREQUISITES table is corrected.
- **vc-operator CA:** `SSL_CERT_DIR`, which Go loads in addition to the system bundle. `SSL_CERT_FILE` replaces it and
  would break calls to publicly signed hosts.
- **Marketplace logic proxy (BAE 1.2.0, Node 18):** axios honours `HTTPS_PROXY`/`NO_PROXY`; the VC login
  (`passport-vc.js`) uses `https-proxy-agent` whenever `HTTPS_PROXY` is set and ignores `NO_PROXY`. Those two calls
  always go through the proxy, even to an in-cluster verifier. This is a known limitation in BAE code.
- **APISIX:** `apisix.ssl.enabled: true`, `existingCASecret` and `certCAFilename: ca.crt` (with `requires`). In 3.17.0
  that makes the Secret's `ca.crt` the *only* trusted file, so a route calling a publicly signed provider needs the
  public roots appended to it. An `each` target writes `ssl_verify: true` into every route's `openid-connect`, and
  `caCoverage` raises an error for a route whose final `ssl_verify` is `false` while a private CA is set. The default for
  routes without a private CA belongs to US-5.6, which should also write `true`.
- **One Secret, one key:** the user gives only the Secret name; the key is the fixed `ca.crt`. The tool never reads or
  stores the bundle.
- **No option disables certificate verification.** A loaded `NODE_TLS_REJECT_UNAUTHORIZED: "0"` stays untouched as a
  foreign element; the tool never writes or offers it.

### 9. Secret requirements

Rendered values may name a Secret the tool never creates. A bundle `requirements:` table lists them as
`{kind: secret, name: <template over answers>, key, purpose}`; `ca.privateSecret` (type `secretName`, a DNS-1123
subdomain) is the first, with key `ca.crt`. The core package `internal/requirements` returns `Secrets(b, p)`: one entry
per row whose name template resolves, that is, only while every answer it reads is given or derived. It yields names
and keys only and never reads, stores or asks for the Secret's content, so US-7.3 can list them in `NEXT_STEPS.md`.

### 10. L2 rules and the no-insecure guard

Three rule kinds join `internal/check`, each with `when` as its precondition; `when` gains `set: true` for an answer
that resolves.
- `caCoverage` (`ca-coverage`, error) runs while `ca.privateSecret` is set. A component's mechanism counts as in
  effect only if each of its final leaves was rendered by the tool or equals the rendered value, so a loaded element
  that keeps the owned key (`KC_TRUSTSTORE_PATHS`, `SSL_CERT_DIR`, ...) but carries other content is an error, not a
  pass. A withheld mechanism is a warning naming the foreign element; `mechanism: none` is a warning; a route whose
  final `ssl_verify` is `false` is an error naming its index and `uri`. A loaded untagged `no`, `off` or `n` counts as
  `false`, because Helm reads values as YAML 1.1.
- `noProxyCoverage` (`no-proxy-coverage`, warning) runs while `proxy.https` resolves and checks each deployed
  `noProxyServices` row (its `enabledKeys`, as in `resources:`) against the final `proxy.no` with the Go matcher
  (`model.NoProxyBypasses`).
- `valueSet` (`keycloak-proxy-mappings`, warning) fires while the final values hold a non-empty value at a path, here
  `keycloak.proxy.spiProxyMappings`.

`internal/model/insecure_test.go` scans every bundle, `ui/src` and the CLI flags for values that disable
verification (`ssl_verify: false`, `insecureSkipVerify: true`, `rejectUnauthorized: false`, `--insecure`) and for
`NODE_TLS_REJECT_UNAUTHORIZED`, and for prose offering to skip TLS or certificate verification. A structural check
also refuses any foundation id, and any target key outside a `const true`, that names verification (`insecure`,
`skip...verify`, `ssl_verify`, ...), so an option whose value comes from an answer is caught too. A legitimate match
is reworded, never allow-listed.

## Deviation from the ticket

The ticket asks for an L2 *error* when an enabled component that calls a public host has no CA mechanism. Contract
Management is such a component, and the tool builds no mechanism for it before Step 7 of the implementation plan. Until
then its `caCoverage` row is `mechanism: none` and yields a **warning** that the hosts it calls need a publicly trusted
certificate, stating that the mechanism is still missing in the tool, not in the chart. The error is reserved for a
component whose mechanism the final tree does not carry in effect and for an APISIX route that does not verify. Step 7
adds the mechanism (contract-management 3.6.3 templates init containers, volumes, mounts and env vars; its image ships
`keytool`), replaces the row with paths and updates this section.

## Consequences

- A proxy or a private CA is one answer, rendered into five components without overwriting the elements other stories
  or the user put into the same lists.
- A hand-made mechanism that collides with the tool's is kept, and the tool's is withheld and reported instead of
  half-written.
- Keyed elements, `each`, `requires`, `defaultWhen: {set: true}` and the new reserved refs are general mechanisms that
  later stories can use as bundle data.
- `lists:` bases tie the bundle to subchart versions; the fixture test makes a bump visible.

## Alternatives

- **Write whole lists as one answer.** It would overwrite the env vars and volumes that other stories and users add.
- **Match elements by index.** Indices shift as soon as a loaded file adds or reorders an element.
- **Match on the key alone.** Two mounts on one `mountPath` would yield a manifest Kubernetes refuses.
- **Raise an error for a withheld mechanism.** It would block `generate` for a setup that most likely works.
- **Create APISIX routes for `ssl_verify`.** Routes are US-5.6's; creating them here would duplicate that story.
- **`spiProxyMappings` for Keycloak.** A second syntax for the same answer, and it disables the env vars the chart sets.
- **`SSL_CERT_FILE` for Go components.** It drops the system roots.
