# 0016 - The local trial: bundled Quick Start values, a trial descriptor and hard-coded warnings

**Status:** Proposed
**Date:** 2026-11-20
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/USER_JOURNEY.md`](../config-tool/USER_JOURNEY.md)

## Context

CT-5 (#120) adds the fourth start card, *Try it locally* (US-1.5 of
[`USER_STORIES.md`](../config-tool/USER_STORIES.md)). One choice produces the
[Quick Start](../deployment-integration/local-deployment/LOCAL.MD): three releases, trust-anchor,
provider and consumer, for a k3s on this machine. It skips stages 1–6 and lands on Review.

Three facts shape the design:

- The renderer from answers to values (Epics 2, 3 and 5) does not exist yet, so the tool cannot
  derive these values from an answer set.
- A profile ([ADR 0015](0015-config-tool-profile.md)) has exactly one `target`, and there is no
  model bundle for the `trust-anchor` chart.
- Epic 6 validation, which would detect development-only settings, does not exist yet either.

The ticket discussion settled on option 3: ship the Quick Start values and hard-code all
production warnings.

## Decision

### 1. Pre-rendered values, not answer sets

The trial ships the values of `k3s/trust-anchor.yaml`, `k3s/provider.yaml` and
`k3s/consumer.yaml` as they are, rather than answer sets fed through a renderer. Once Epics 2, 3
and 5 render, real answer sets replace them, and the trial then goes through the same path as
every other profile.

### 2. Byte-identical copies, kept in sync by a test

`go:embed` cannot reach above the module directory, so `config-tool/trial/values/` holds copies
of the three `k3s/` files. They are byte-identical, trailing newline or not, and a test compares
them with the originals. `make trial-sync` refreshes them. The CI workflow also runs on changes
under `k3s/**` and `charts/**`, so a drift fails the build of the change that caused it.

### 3. A trial descriptor, not a profile

`config-tool/trial/trial.yaml` lists the releases in install order, as `k3s/deploy.sh` installs
them. Each entry has a `role`, the `source` file in the repository, the embedded `values` path
and a `target` with the keys of `profile.Target`, always `type: file` and
`environment: development`. The descriptor is not a `.dscprofile.yaml`: profile format 1 keeps a
single target and is unchanged. The trial is not saved as a profile.

### 4. Production warnings are a hard-coded list

Until Epic 6 validates values, the trial's production warnings are a fixed list in code. Each
warning names the releases it applies to and carries evidence, a string that must occur in the
bundled values of each of those releases. A test checks the evidence, so a warning cannot outlive
the setting it warns about. The warnings are always shown and cannot be dismissed.

### 5. Outputs never overwrite

The trial writes one values file per release, all or nothing. If any target file exists, it
writes none of them. This is the rule of ADR 0015 for `generate`, applied to several files.

### 6. Only the data-space-connector outputs can be reopened

Each output starts with the generated header of [ADR 0013](0013-config-tool-roundtrip-byte-patches.md)
and a development banner. The header has no chart name, and loading
([ADR 0014](0014-config-tool-load-classification.md)) reads its chart version as a
`data-space-connector` version. The provider and consumer outputs therefore carry their chart
version and open in the tool like any other values file. The trust-anchor output is not meant to
be reopened: there is no trust-anchor model bundle. Its header carries no chart version, and the
banner names the chart and its version instead.

*Update (#122):* a trust-anchor bundle (`model/1.3.6/`) now exists, so the remaining reason is the
header: loading still reads its chart version as a `data-space-connector` one.

## Consequences

- The trial works before the renderer exists and yields exactly what the Quick Start deploys.
- Each change to a `k3s/` file also needs `make trial-sync`; the sync test says so.
- A trial cannot be edited stage by stage and saved as a profile until the renderer exists.
- A new development-only setting in the Quick Start values gets no warning until someone adds
  one; removing a setting fails the evidence test.
- The tool does not install anything, detect a cluster, or apply the namespaces, operators and
  infrastructure that `k3s/deploy.sh` sets up first.

## Alternatives

- **Answer sets through `internal/render`.** Needs the renderer of Epics 2, 3 and 5, which would
  block this ticket on them.
- **Read the `k3s/` files at run time.** A release binary has no repository next to it.
- **Profile format 2 with several targets.** Changes the saved format for a path that does not
  save, and needs a trust-anchor model bundle that does not exist.
- **Warnings derived from the values.** That is Epic 6 validation; a partial version here would
  be replaced there.
- **Overwrite or merge existing outputs.** Breaks the rule that the tool never touches a file it
  did not write.
