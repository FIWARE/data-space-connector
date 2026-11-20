# 0017 - Role presets as bundle data, checked against the role guide's matrix

**Status:** Proposed
**Date:** 2026-11-20
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md#role-presets)

## Context

E2 (#122, US-2.1) adds Stage 2 of [`USER_JOURNEY.md`](../config-tool/USER_JOURNEY.md#stage-2--role).
The user picks one of five roles, and the role preselects features from the catalogue. Two sources
already describe which components a role needs, and they disagree:

- the component matrix of the [role guide](../deployment-integration/roles/README.md#component-matrix),
  which operators read when they deploy by hand;
- the preset table of [`FEATURE_MODEL.md`](../config-tool/FEATURE_MODEL.md#role-presets).

Two more facts shape the design. The Operator role deploys the `trust-anchor` chart, not the
data-space-connector, and that chart had no model bundle. And a loaded values file
([ADR 0014](0014-config-tool-load-classification.md)) already back-derives feature toggles, so a
preset can contradict a file the user loaded.

## Decision

### 1. The role guide's matrix is authoritative

Where `FEATURE_MODEL.md` disagreed with the matrix, the bundle follows the matrix, and the design doc
was corrected: `contract.tmforum` and `contract.management` are *Optional* for Provider and
Consumer + Provider, so the presets make them `available`, not `recommended`.

### 2. Cell translation

`Required` → `recommended`, `Optional` → `available`, `-` → `not-offered`. `Built into <X>` takes the
value of row `<X>` in the same column. Any other cell text fails the test of section 9.

### 3. A feature belongs to one chart, and so does a role

A feature has an optional `chart:` that defaults to the bundle's chart. A role may offer
(`recommended` or `available`) only features of its own `chart`, because a feature whose keys do not
exist in the target chart cannot be set there. Operator therefore offers `ta.registry` (recommended),
`ta.onboarding` and `consent.authority`, and nothing else. `dsp.config` and `obs.*` are not offered,
because the trust-anchor chart has neither key.

Custom is fixed to `data-space-connector`. It makes every feature of that chart `available`, recommends
nothing, and does not offer the trust-anchor features. This deviates from `USER_JOURNEY.md`, which lists
Custom as "choose chart". A chart choice inside Custom would duplicate Operator, which already offers
every trust-anchor feature, and would give Custom two catalogues to filter by chart. Picking Custom after
Operator switches the chart back.

### 4. The matrix-to-feature map is bundle data

The top-level `roleMatrix:` of the bundle maps each matrix "Component" cell to a feature id
(`components`). It also lists the rows that are not features (`infrastructure`, today `PostgreSQL`).
Several components may map to one feature (VCVerifier, credentials-config-service and
trusted-issuers-list → `access.verification`). Keeping the map in the bundle means a new matrix row
needs a bundle edit, not a test edit. Features that no row maps to (`data.scorpio`, `market.*`, `obs.*`,
...) cannot be checked against the matrix. Their offers follow `FEATURE_MODEL.md`, adjusted by section 3.

### 5. The trust-anchor chart gets a minimal bundle

`Profile.Validate` refuses a target chart without a shipped bundle, and Stage 1 offers only versions
with a bundle. `config-tool/model/1.3.6/bundle.yaml` is the trust-anchor bundle: `chart: trust-anchor`,
the chart's version, `revision: 1` and `topLevelKeys`, with no `recognize`, `features` or `roles`. Bundle
directories are named by version only, and 1.3.6 does not collide with a data-space-connector version.

### 6. Where presets are read from

`model.RoleCatalogue(bundles, chart, version)` returns the target's own bundle when that declares
`roles:`. Otherwise it falls back to a bundle that does: a `Latest` one first (highest semver if several
are), and the highest semver only when no bundle with roles is `Latest`. A shipped release candidate
therefore never becomes the fallback while a released bundle with roles exists, the same way an rc
never becomes the default version. The rule names no chart in Go code. For trust-anchor, the fallback
is the latest data-space-connector release bundle.

Because the fallback bundle is not the profile's `model`, the profile records it as
`catalogue: {chart, version, revision}`, set only when it differs from `model`. `Validate` reads roles
and features from the recorded bundle, never from whatever is newest at that moment, and applies the
`model` rules to it: not shipped or a newer revision → 422 pointing at US-9.3, an older revision → a
note. A later bundle that renames a `ta.*` feature then cannot silently invalidate a saved Operator
profile. The server owns `catalogue` on save, as it owns `model`.

### 7. Toggles from a loaded file win over the preset

`ApplyPreset` takes the toggles that the loaded file back-derived (`<featureId>.enabled` at stage
`features`). The server reads them from the loaded report, so the client cannot send different ones.

- A loaded toggle for an offered feature is kept as `{enabled: <file value>, source: loaded}`.
- A loaded `false` for a not-offered feature is dropped, like any not-offered feature.
- A loaded `true` for a not-offered feature refuses the role (`ErrPresetConflictsLoaded`, 422),
  naming the features. The file is not a setup of that role, and nothing is cleared silently.

`loaded` is a third feature source next to `preset` and `user`. A re-applied preset treats it like `user`.
Every `recognize` rule at stage `features` must name `<catalogue id>.enabled`, so every loaded toggle
names a catalogue feature.

### 8. HTML parsing in the matrix test

The matrix is an HTML `<table>` inside Markdown, because its cells use `rowspan`.
`internal/model/rolematrix_test.go` parses it with `golang.org/x/net/html`, and only test files import
that package. The module was already an indirect dependency, so it moves to the direct `require` block
and adds no new module. The test expands the rowspans and finds columns by their exact header text. The
table also has `Umbrella component`, `Sub-umbrella component` and `Role` (a description), which a
substring match would confuse with `Component` or a role title. The test compares every role column with
the latest shipped bundle that has roles. Its messages name role, feature and component, for example
`role "provider", feature "identity.issuer": bundle says recommended, matrix says available (component "Keycloak")`.

### 9. Feature `keys` are subtree paths and may be shared

A key is a `roundtrip` path that names that node and its whole subtree, so `FEATURE_MODEL.md`'s
`keycloak.*` is written `keycloak`. No wildcard exists: validation refuses a key segment that contains
`*`, so a copied `keycloak.*` fails instead of naming a literal key `*`. Qualifiers such as "+ ingress"
become concrete paths (`trusted-issuers-list.ingress.til`).

`keys` says what a feature touches, not what it owns exclusively. `vcCredentials`
(`identity.issuer` and `identity.vcoperator`), `grafana` (`obs.traceStore` and `obs.metrics`) and
`contract-management` (`contract.management` and `market.central`) are shared. A shared key is needed
while any feature that lists it is enabled, and that is the meaning US-5.1 inherits. Within one feature,
a key listed twice or a key under another key of the same feature is an error.

## Consequences

- Changing the matrix without the bundle, or the bundle without the matrix, fails `go test ./...`.
- The role guide stays the one place where operators and the tool read role requirements.
- The data-space-connector bundle went to revision 2. Profiles saved against revision 1 open with an
  older-revision note.
- A trust-anchor profile depends on a data-space-connector bundle for its roles, and records which one.
- Custom cannot produce a trust-anchor configuration. Operator does.

## Alternatives

- **`FEATURE_MODEL.md` as the source.** Operators read the role guide, so two tables would keep drifting.
- **Generate the presets from the matrix.** The matrix covers 8 of the 24 features. The rest would still
  need hand-written data, now split across two formats.
- **The matrix map in test code.** A new component would need a Go change and could not be reviewed
  next to the presets it affects.
- **Roles in the trust-anchor bundle as well.** Two copies of the same five roles, which could
  disagree, for a chart that offers one of them.
- **Custom with a chart choice.** It duplicates Operator and needs a second catalogue view (section 3).
- **A regexp over the Markdown instead of an HTML parser.** It breaks on `rowspan` and nested links.
