# 0024 - Size it for production: a resource profile from the environment, a sizing table as bundle data, and a memory-limit warning

**Status:** Proposed
**Date:** 2026-11-25
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md#cross-cutting-values)

## Context

US-3.5 (#129) of [`USER_STORIES.md`](../config-tool/USER_STORIES.md) asks for a resource profile in Stage 3's
*Platform* panel. It defaults from Stage 1's environment and sets `requests`/`limits` on the Java components. In
production, any enabled Java component that is left without a memory limit should get a warning, because the JVM then
sizes its heap from the node. The [sizing guidance](../deployment-integration/roles/PREREQUISITES.md#sizing) gives the
figures, measured at rest on chart 10.13.2. [Its next section](../deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components)
reports Scorpio at ~17 GiB and the TM Forum API at ~8.5 GiB on 64 GiB nodes without limits.

Before this story, a foundation's default could reference only other answers ([ADR 0020](0020-config-tool-foundations-render.md)),
a `const` target was allowed only under a list index, two targets on one path failed the load, and every L2 rule kind
except plain `when` dropped `when` ([ADR 0021](0021-config-tool-l2-rules.md), [ADR 0023](0023-config-tool-pinned-hosts.md)).

## Decision

### 1. A `resources:` table in the bundle

The 10.16.0 bundle gets a top-level `resources:` section. Each component has an `id`, a `label`, the `path` of its
`resources` block, `enabledKeys` (every chart switch that must be true for it to deploy, with its chart default) and
optionally `chartMemoryLimit` and `fallbackPaths`:

| `components` (sized) | Path |
|---|---|
| Keycloak | `keycloak.resources` (`chartMemoryLimit: 2Gi`) |
| Scorpio | `scorpio.resources` |
| TMForum API | `tm-forum-api.defaultConfig.resources` |
| Trusted Issuers List | `decentralizedIam.vcAuthentication.trusted-issuers-list.deployment.resources` |
| contract-management | `contract-management.deployment.resources` |

The paths were checked against the subcharts. tm-forum-api 0.19.4 reads `defaultConfig.resources` in both deploy
modes, contract-management 3.6.3 reads `deployment.resources` and scorpio-broker-aaio 0.5.1 reads the top-level
`resources`. The TIL path is the one `k3s/provider.yaml` uses. The umbrella `values.yaml` already sets
`keycloak.resources` (500Mi/0.5 → 2Gi/1), which stays above the chart default because of OOMs on kernel ≥ 6.14, and
`chartMemoryLimit` records it. A default in `enabledKeys` is the umbrella `values.yaml` value. A key the umbrella does
not set (`decentralizedIam.vcAuthentication.enabled`, `fdsc-edc.enabled`) takes the subchart's default, or Helm's
"a missing condition is on". Both are `true`. A test in `internal/model` pins the defaults against `values.yaml`.
Quantities are YAML strings.

`unsized` lists the chart's other JVM workloads. No profile sizes them, and only the rule of §6 reads them. Each one was
found to be JVM by the entrypoint of its default image (`java …`):

| `unsized` | Version | Path |
|---|---|---|
| credentials-config-service | chart 3.4.3 | `decentralizedIam.vcAuthentication.credentials-config-service.deployment.resources` |
| DSS validation service | chart 0.0.1 | `decentralizedIam.vcAuthentication.dss.deployment.resources` |
| consent-facade | chart 0.2.1, image 0.4.2 | `consent-facade.deployment.resources` |
| IdentityHub | umbrella template | `identityhub.resources` (`chartMemoryLimit: 512Mi`, the umbrella default) |
| fdsc-edc | umbrella dependency | `fdsc-edc.deployment.*.deployment.resources`, fallback `fdsc-edc.common.deployment.resources` |

The fdsc-edc chart renders one Deployment per key of `fdsc-edc.deployment` (empty by default), each merged over
`fdsc-edc.common.deployment`. So its path has one `*` segment over the map's keys, and a limit at the fallback path
covers every instance. A `*` is allowed only in `unsized` paths.

Not JVM, and therefore not listed:

- `odrl-pap` 1.7.3: a Quarkus native image, entrypoint `./application`;
- consent-owner-resolver (chart 0.2.0, image 0.0.8): the native `/app/owner-resolver`, and its chart sets a 128Mi limit;
- the verifier, vc-operator and did-helper (Go), Rainbow (Rust), the status-list-server, the marketplace (Node and
  Python) and the dashboard.

The marketplace's `bizEcosystemApis.enabled` and `bizEcosystemRss.enabled` deploy nothing in business-api-ecosystem
1.2.0. That chart has templates only for the charging backend and the logic proxy, which read `bizEcosystemApis` as
legacy TMForum settings. The TMForum APIs come from the umbrella's `tm-forum-api`, which the table sizes. These
verdicts hold for the default images of the versions above. A changed image or a subchart bump has to re-check them.

### 2. The figures

`profiles` holds one row per option (`development`, `production`), each with every sized component:

| Component | development (requests → limits) | production (requests → limits) |
|---|---|---|
| Keycloak | 100m/768Mi → 1/2Gi | 500m/2Gi → 2/2Gi |
| Scorpio | 100m/512Mi → 1500m/1Gi | 250m/1Gi → 2/1Gi |
| TMForum API | 100m/512Mi → 1/1Gi | 250m/1Gi → 2/1Gi |
| Trusted Issuers List | 50m/256Mi → 500m/512Mi | 100m/512Mi → 1/512Mi |
| contract-management | 50m/256Mi → 500m/512Mi | 100m/512Mi → 1/512Mi |

They derive from the memory floors measured at rest on 10.13.2: Keycloak 0.6 GiB, Scorpio 0.4, TMForum API 0.35, TIL
0.2 and contract-management 0.2. A YAML comment next to the table records them.

- *development*: the memory request is the floor rounded up to 256Mi, and the limit is twice the request.
- *production*: the memory request equals the limit, 2.5× the floor rounded up to 512Mi. Because request = limit, the
  scheduler reserves what the JVM may take, and the pod is not the first evicted under node pressure.
- Neither profile lowers the chart's Keycloak limit of 2Gi.
- CPU requests are a fraction of a core, because the components idle at rest. CPU limits leave room for the start-up
  burst, since a throttled JVM misses its probes.
- `custom` has no row. It renders nothing, so the chart's defaults stay in force.

### 3. The profile defaults from Stage 1 through a reserved ref

`resources.profile` is a `choice` in the new group `platform`, with `default: "{{ .target.environment }}"`. A default may
reference the reserved, read-only ref `target.environment` (`model.FieldTargetEnvironment`). It is seeded from
`profile.Target.Environment` and is never a foundation. It never appears in `Answers()`, in the tree, or as an entry of
`HandSet` or `DerivedDiff`, and a target may not reference it. An empty environment leaves it unresolved, so there is
no default and no key. A given answer overrides the default, as every given answer does.

`resolve` builds a `reserved` set (the reserved refs with a non-empty value) next to the answers. The default gate
(`render.defaults`, used by `resolve`, the prefix check and `derived()`) passes when every ref is an answer or reserved.
A *given* profile is hand-set like any other answer with a derived default. `HandSet` lists it, with the environment's
profile as its derived value, and `DerivedDiff` reports it as *pinned, not updated* when a Stage 1 switch changes what
it would derive. The user chose a profile that no longer follows the environment, and *Reset to derived* follows it
again. The UI mirrors the reserved set (`reservedRefs`), so it resolves the same default without the server.

### 4. The table compiles into `Only` targets

The model compiles the table into targets of the `answer` foundation: one `const` target per profile × component ×
leaf (`requests.cpu`, `requests.memory`, `limits.cpu`, `limits.memory`). Each carries `Only: <profile>` and
`Refs: [<answer>]`, which a hand-written `const` cannot. Through the ref, the render skips the targets while the profile
is unresolved, a leaf's provenance names `resources.profile`, and `render.Targets` reaches them. `render.Targets`
lists each path once, so `resources.profile` reports 20 values in 5 components.

The compiled targets join the hand-written ones before `checkRefs`, `checkTargetPaths` and `topoOrder`. Two targets on
one path are accepted only when they belong to the same foundation and carry distinct non-empty `Only` values. Every
other duplicate and every prefix clash is still refused, a hand-written `keycloak.resources` among them. The render
emits a target with `Only` only while the resolved answer equals it, so pinning of hand edits and the leaf diff work
unchanged. "`const` only under a list index" stays a rule for hand-written targets.

### 5. No back-derivation on load

A loaded `resources` block stays unknown and is pinned ([ADR 0014](0014-config-tool-load-classification.md)). The
answer lives in the profile, so re-loading a generated file without its profile lists `resources.profile` as
underived.

### 6. Rule kind `memoryLimit`

`memoryLimit: {components: [<id>...]}` takes ids from `components` and `unsized`. It is the first kind that keeps
`field` and `when`, which act as its precondition. Rule `java-memory-limit` warns on `answers.resources.profile` while
the environment is `production`. It yields one finding that names every listed component which both:

- **deploys**: each of its `enabledKeys` is effectively true. The effective value is the rendered leaf, else the loaded
  file's scalar, else the default the table records. So the default-off components (credentials-config-service, DSS,
  consent-facade, IdentityHub) are named only once a loaded file switches them on. For fdsc-edc, every key of
  `fdsc-edc.deployment` in the tree or the loaded file counts, labelled `fdsc-edc (<key>)`, in key order;
- **has no memory limit**: no `<path>.limits.memory` in the rendered tree (hand edits pinned in), no non-null scalar
  there or at a fallback path in the loaded file, and no `chartMemoryLimit`. Helm's coalesce drops a chart default
  under a `null`, so a loaded `null` at `<path>.limits.memory` or at any ancestor up to `<path>` removes
  `chartMemoryLimit`. A `null` above `<path>` (`keycloak: null`) is not a sizing edit and is ignored.

The message, "Without a memory limit, the JVM sizes its heap from the node: {{ .components }}.", joins the labels with
", " and reads for any count. `check.Evaluate` takes `render.Options`, so the server checks against the loaded
overrides and the CLI passes the zero value. The warning never blocks `generate`.

**Limitation until US-5.3.** The features in the profile do not decide whether a component deploys. The tool renders
no `enabled` key yet, so a feature that is off does not stop the chart from deploying the component. A production
`custom` profile with `data.scorpio` off still runs Scorpio, and the warning names it. Once US-5.3 renders `enabled`
from the features, the rule follows through the rendered leaf without a change. A YAML comment above the rule records
this.

### 7. Table paths are checked against the values schema

A Go test walks every table path (`components`, `unsized` and `fallbackPaths`) through
`charts/data-space-connector/values.schema.json`. It stops at the first node with `additionalProperties: true` and no
named properties, which is every subchart alias here. Each named segment before that node must exist. US-11.6 will
generalize this per-path check.

### 8. The trust-anchor bundle is untouched

`model/1.3.6` gets no table and no profile. The story is scoped to the connector chart.

## Consequences

- A production profile sets requests and limits on the five components the guidance names. The panel previews the
  figures before they are applied.
- A production environment whose Java components end up without a memory limit is warned about, whether the cause is
  `custom`, a loaded file or an enabled component no profile sizes.
- New figures, components or a new profile option are bundle data. A new chart version measures them again and records
  its `measuredOn`.
- The unsized list and the JVM verdicts are tied to the subchart and image versions above.
- `Only` targets and the reserved ref are general mechanisms: a later answer can select rows of a table, or default from
  Stage 1, without new code.

## Alternatives

- **A Go table of figures.** It would put chart knowledge in code, against [ADR 0020](0020-config-tool-foundations-render.md).
- **One foundation per component and leaf.** 20 answers for one choice, and no single place to change a figure.
- **Copy the environment into the answers.** A second source of truth that save and open would have to keep in step
  with Stage 1.
- **Back-derive the profile from a loaded `resources` block.** A block rarely matches a row exactly, and a near match
  would silently overwrite a hand-sized component.
- **Gate the rule on the features.** Until US-5.3 the chart deploys a component whatever its feature says, so the
  warning would miss real pods.
- **Size every JVM component.** The guidance measures only the five. A figure without a measurement would be a guess.
