# Implementation Plan: [E3 Foundations] US-3.6 — Egress proxy and private CA, declared once

## Overview
Add four Stage 3 foundations to `config-tool/model/10.16.0/bundle.yaml`: `proxy.https`, `proxy.http`, `proxy.no` and
`ca.privateSecret`. They fan out to the verifier, Keycloak, the marketplace logic proxy, the vc-operator and APISIX,
each through that component's own mechanism. Most of these targets are elements of env, volume or init-container lists
that other stories also fill, or keys inside the APISIX routes. The render engine therefore first learns keyed list
elements and per-element targets (ADR 0025). After that come the proxy and CA data, a CA-coverage L2 rule, the
Secret requirement that NEXT_STEPS will list, a no-insecure-option test, and the Stage 3 panel with its docs.

## Decisions settled by this plan (record them in ADR 0025)
- **Keyed elements.** A target may write one element of a list, identified by a key field (`name` for env vars,
  volumes, mounts and init containers). It never writes the whole list. Helm replaces lists, so the
  render writes a complete list. If the loaded file holds the list, the result is the loaded elements with the owned
  keys replaced in place, plus the missing owned elements appended. If it does not, the result is the chart's default
  elements, declared as bundle data (`lists:` table, a test pins them against the chart's `values.yaml`), followed by
  the owned elements in bundle order. Leaves keep concrete indices, so `render.Tree`, `Nested()` and the leaf JSON do
  not change. Elements with keys the tool does not own are never touched. Appending beyond a loaded list's length is
  left to US-7.1's writer, which will need it (`roundtrip.Set` creates no sequence items, and `generate` does not write
  yet).
- **Per-element (`each`) targets.** APISIX's `proxy_opts` lives inside each route's `plugins.openid-connect`. Routes
  belong to US-5.6 and stay non-targets (ADR 0020). An `each` target writes a key into every element of a list that
  already has the parent mapping, whether loaded now or rendered by US-5.6 later. It never creates routes. When there
  are no routes, nothing renders for APISIX, and the Stage 3 preview says so.
- **`requires` on a target.** `keycloak.proxy.enabled: true` is a bool that comes from no answer. A `const` outside a
  list is allowed only if the target lists `requires: [<answer id>]`, and it renders only while those answers are
  given or derived. This keeps the rule that an unanswered question emits no key.
- **Reserved refs `target.release` and `target.namespace`** join `target.environment` (Stage 1), so the
  `NO_PROXY` default can name the release's own services.
- **`NO_PROXY` default:** `localhost,127.0.0.1,*.svc,*.cluster.local,` followed by the in-cluster service names that
  the five callers reach, built from `{{ .target.release }}`/`{{ .target.namespace }}` in the bundle.
  **No literal CIDR placeholder** goes into the value, because every client would parse it as a host. The field's help
  and UI placeholder ask the user to append the pod and service CIDRs (for example `10.42.0.0/16,10.43.0.0/16` on
  k3s). The value stays editable as an ordinary given answer.
- **Keycloak** honours the standard `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` env vars for its outgoing HTTP client, so
  the umbrella's `keycloak.proxy.{enabled,httpsProxy,httpProxy,noProxy}` is enough, and `spiProxyMappings` stays
  unset. Step 3 checks this in the Keycloak 26.7 "outgoing HTTP requests" guide.
- **Keycloak CA: correct the PREREQUISITES table.** `tls.truststoreEnabled`/`truststoreExistingSecret` in CloudPirates
  0.21.19 becomes `--https-trust-store-file`, which only validates *inbound* mTLS clients and needs a JKS file plus a
  password. Outgoing trust is `truststore-paths`, which accepts PEM. So one PEM Secret serves every mechanism. Keycloak
  gets keyed elements in `keycloak.extraVolumes` and `keycloak.extraVolumeMounts` (mount under
  `/opt/keycloak/conf/truststores/`) and in `keycloak.extraEnvVars` (`KC_TRUSTSTORE_PATHS`). The umbrella default
  elements (`KC_HEAP_SIZE`, the `realms` mount) are kept through the `lists:` base.
- **vc-operator CA:** PREREQUISITES says `SSL_CERT_FILE`, which *replaces* Go's system bundle and so breaks calls to
  publicly signed hosts. Use `SSL_CERT_DIR` instead: Go's `crypto/x509` `root_unix.go` loads the default bundle file
  plus the listed directories. Check that in the Go source before relying on it, then update the PREREQUISITES row.
- **One Secret, one key.** The user gives only the Secret name. The key is the fixed `ca.crt` (a named value in the
  bundle). The tool never reads or stores the bundle itself.
- **Contract Management** has no CA mechanism. It produces a **warning**, not an error, telling the user that the
  hosts it calls need a publicly trusted certificate. An error would block every provider that uses a private CA.
  The **error** is reserved for an enabled component that has a mechanism the final tree does not carry (dropped or
  pinned by a hand edit).
- **No option disables certificate verification** anywhere. The loaded `NODE_TLS_REJECT_UNAUTHORIZED: "0"` in
  `k3s/provider.yaml` stays untouched as a non-owned element. The tool never writes or offers it.

## Steps

### Step 1: ADR 0025 and the render engine for keyed elements, `each` and `requires`
- Write `doc/adr/0025-config-tool-proxy-and-private-ca.md` with the decisions above, and add it to the index in
  `doc/adr/README.md` ("ADRs 0009 to 0025").
- `config-tool/internal/model/foundations.go`: extend the target schema with
  - `element: {key: name, value: <mapping whose string scalars are templates>}` on a path that ends at a list (a
    keyed element);
  - `each: <subpath>` with a `[*]` path segment (a per-element target);
  - `requires: [ids]`, which allows `const` outside a list index.

  Add a top-level `lists:` table (`{path, key, base: [elements]}`) declaring each list that keyed elements write
  into, with the chart's default elements. Add `target.release` and `target.namespace` to `reservedRefs`, seeded
  from the profile's Stage 1 target in `internal/render`. Compile-time checks: a keyed element's list must be in
  `lists:`. The same list may not also be index-targeted. Two targets may not own the same key value. `each` is
  refused on a list that `lists:` declares.
- `config-tool/internal/render/render.go` (and a new `elements.go`): compose keyed lists as the ADR describes, using
  `opts.Overrides` when a file is loaded. Expand `each` targets over the loaded list's elements that carry the parent
  mapping. Gate on `requires`. Provenance records the element key. `pin` treats a loaded element with an owned key
  whose content differs as a hand edit, exactly as a scalar pin.
- Tests are table-driven, with fixture bundles in `internal/model/testdata` and `internal/render/testdata` and no
  `10.16.0` content yet. Cover: compile errors; the composition with and without a loaded list; the base kept; foreign
  elements untouched; append order; `each` with zero, one and many routes; `requires` emitting nothing while
  unanswered; reserved refs.
- Acceptance: `go test ./...` and `make lint` are green. The existing bundle compiles and renders unchanged
  (`provider_test.go` is untouched).

### Step 2: Load and back-derivation for keyed and `each` targets
- `config-tool/internal/load` (`classify.go`, `seed.go`): a loaded element whose key matches an owned keyed target is
  *owned*. Its leaves back-derive the answer (for example the verifier's `HTTPS_PROXY` value gives `proxy.https`).
  Other elements of the same list keep their current class. An `each` target's leaf under any matching element is
  owned. When the elements disagree, the first one in document order wins and the others are pinned.
- `render.HandSet`/`DerivedDiff` list a pinned element under its key (`additionalEnvVars[name=NO_PROXY]`).
- Tests: table-driven fixtures, including a list that mixes owned and foreign elements, and a round trip
  load → render → equal tree for the fixture bundle.
- Acceptance: `go test ./...` and `make lint` are green, and loading `k3s/*.yaml` gives the same report as before.
  No `10.16.0` targets exist yet.

### Step 3: Proxy foundations and their fan-out
- `model/10.16.0/bundle.yaml`: add a `lists:` row for every list touched, with the chart's base elements, and the
  foundations below, in a new group `egress`. Bump `revision` and extend the header comment block for the new fields
  and the group.
  - `proxy.https` (type `url`).
  - `proxy.http` (`url`, default `{{ .proxy.https }}`).
  - `proxy.no` (`string`, default the `NO_PROXY` template above).
- Targets, each written while its answer resolves:
  - Verifier: keyed `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` in
    `decentralizedIam.vcAuthentication.vcverifier.deployment.additionalEnvVars`.
  - Keycloak: `keycloak.proxy.enabled` (`const true`, `requires: [proxy.https]`), plus `httpsProxy`, `httpProxy` and
    `noProxy`.
  - Marketplace: keyed env in `marketplace.bizEcosystemLogicProxy.additionalEnvVars`.
  - vc-operator: keyed env in `vc-operator.extraEnvVars`.
  - APISIX: `each` over `decentralizedIam.odrlAuthorization.apisix.routes[*].plugins.openid-connect` writing
    `proxy_opts.{https_proxy,http_proxy,no_proxy}`.

  Check every path against the umbrella `values.yaml` and the subchart defaults (BAE 1.2.0, vc-operator, the
  decentralized-iam apisix chart) before writing it. Confirm in the Keycloak 26.7 docs that it reads the standard
  proxy env vars.
- New test `internal/render/proxy_test.go` pins the proxy key list (all five components) against
  `charts/data-space-connector/values.yaml`: every scalar target path exists there or under a declared open subchart
  key, and every `lists:` base equals the chart's elements. Add a second test rendering a minimal profile with only
  `proxy.https` set: all five components render, `NO_PROXY` holds the release's services, and nothing renders while
  `proxy.https` is unanswered.
- `internal/render/provider_test.go`: answer the proxy foundations from `k3s/provider.yaml`. Add exclusion rows (story
  US-3.6) where k3s tailors `NO_PROXY` per component, and for anything else the fan-out deliberately does not
  reproduce.
- Acceptance: `go test ./...` and `make lint` are green.

### Step 4: Private CA foundation, its mechanisms and the Secret requirement
- `model/10.16.0/bundle.yaml`: add `ca.privateSecret` (type `string`, a DNS-1123 Secret name; reuse `hostname` or add
  a `secretName` type in `internal/model` with a pattern constant). Bump `revision`. Targets, one per PREREQUISITES
  mechanism (key `ca.crt`):
  - APISIX: `decentralizedIam.odrlAuthorization.apisix.apisix.ssl.existingCASecret` and `certCAFilename: ca.crt`
    (check the alias path).
  - Verifier: keyed `additionalVolumes` (`private-ca` Secret volume, plus a `ca-bundle` emptyDir),
    `additionalVolumeMounts` (`ca-bundle` over `/etc/ssl/cert.pem`, subPath `cert.pem`), and the `initContainers`
    element `add-private-ca`. That init container concatenates `/etc/ssl/cert.pem` and the CA into the emptyDir, uses
    an image pinned as a named bundle value, and runs with the strict security context.
  - Marketplace logic proxy: keyed `NODE_EXTRA_CA_CERTS` plus the volume and mount (BAE's `extraContainerVolumes` and
    `extraVolumeMounts`; check the shapes in BAE 1.2.0).
  - vc-operator: keyed `SSL_CERT_DIR` plus `extraVolumes`/`extraVolumeMounts`.
  - Keycloak: keyed `extraVolumes`/`extraVolumeMounts` and `KC_TRUSTSTORE_PATHS` in `extraEnvVars`.
- Secret requirements from the core: add a bundle `requirements:` list (`{kind: secret, name: <template>, key,
  purpose}`) and a core package `internal/requirements` with `Secrets(b, p) []Secret`. It returns only the names and
  keys it can resolve, and never a value, so US-7.3 can write them to `NEXT_STEPS.md`.
- Tests: `internal/render/ca_test.go` (table-driven, one row per PREREQUISITES mechanism, all rendered from a single
  answer; nothing renders while unanswered) and `internal/requirements` tests. Extend `provider_test.go` exclusions
  (US-3.6), because k3s takes the verifier's CA from the signing-key Secret.
- Acceptance: `go test ./...` and `make lint` are green.

### Step 5: CA-coverage rule and the no-insecure guard
- `internal/check`: a new rule kind `caCoverage` in the bundle's `rules:` (document it in the header comment) with
  `components: [{id, label, enabledKeys, paths | mechanism: none}]` and the precondition `when: [{field:
  answers.ca.privateSecret, ...}]`. It needs a precondition form for "set", either a new `set: true` beside `unset` or
  the rule kind's own gate.
  - An **error** names every enabled component whose CA paths are missing from the final tree (rendered plus loaded).
  - A **warning** names the enabled components without a mechanism (contract-management).

  Reuse the `enabledKeys` handling from `memory.go`. Add the two rules to `model/10.16.0/bundle.yaml` and bump
  `revision`.
- No-insecure guard: a Go test (`internal/model/insecure_test.go`) scans every `model/*/bundle.yaml`, `ui/src/**`
  (excluding test fixtures) and the cobra flag definitions in `internal/cli` against one regexp table:
  `insecure`, `skip.?verify`, `tls_verify`, `ssl_verify`, `verify_ssl`, `NODE_TLS_REJECT_UNAUTHORIZED`,
  `rejectUnauthorized`, `InsecureSkipVerify`, `--insecure`. `k3s/` and `config-tool/trial/` are reference files and
  are not scanned. If a legitimate word matches (for example in help text), reword it rather than adding an allow
  list.
- Tests: table-driven `check` cases (CA unset; all covered; one component pinned away; contract-management enabled
  and disabled).
- Acceptance: `go test ./...` and `make lint` are green.

### Step 6: Stage 3 egress panel, API exposure and docs
- UI: `ui/src/stages/EgressPanel.tsx` (+ `EgressPanel.test.tsx`), modelled on `PlatformPanel.tsx`. It shows the proxy
  fields with the derived `NO_PROXY` (editable, with the CIDR hint) and the CA Secret field with the required key
  `ca.crt`. It also previews the fan-out per component, including "no APISIX routes yet" when `each` has nothing to
  write. Wire the `egress` group into `Foundations.tsx`. Every shown text is UI copy or bundle data. The verification
  wording must pass the Step 5 guard.
- Server: expose `requirements.Secrets` (and the new findings, which already flow through `check`) in the
  existing render/review response. No new outbound calls.
- Docs:
  - `config-tool/README.md`: the egress section.
  - `doc/config-tool/FEATURE_MODEL.md#cross-cutting-values`: the final ids `proxy.https`, `proxy.http`, `proxy.no`,
    `ca.privateSecret`.
  - `doc/config-tool/USER_JOURNEY.md` Stage 3, if the group is named there.
  - `doc/deployment-integration/roles/PREREQUISITES.md#private-ca`: correct the Keycloak row to `truststore-paths`
    and the vc-operator row to `SSL_CERT_DIR`, and note that one PEM Secret with key `ca.crt` serves every
    mechanism.
  - `AGENTS.md`: mark US-3.5 shipped and add a US-3.6 line.
  - Run `.github/scripts/check-links.sh` if links changed.
- Acceptance: `make ui`, `make build`, `make test` (`go test ./...` + `npm test`) and `make lint` are green, and the
  Step 3 and Step 4 tests still pass.
