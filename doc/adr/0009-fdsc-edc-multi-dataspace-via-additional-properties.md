# 0009 - Configure fdsc-edc participant contexts through `additionalProperties`, with the DID as context id

**Status:** Accepted
**Date:** 2026-11-26
**Applies to:** `FIWARE/data-space-connector`, [fdsc-edc](https://github.com/SEAMWARE/fdsc-edc)
**Context doc:** [`../MULTI_DATASPACE.md`](../MULTI_DATASPACE.md)

## Context

fdsc-edc 2.x runs `controlplane-dcp` as EDC virtual participant contexts: one connector can take
part in several dataspaces, each through its own DSP profiles and, if needed, its own identity.
There is no legacy mode, so the DCP controlplane needs at least one `fdsc.participants.*` entry, the
declared profiles, a `%s` DSP callback and an OAuth2 issuer for the v5beta management API.

The fdsc-edc Helm chart (`0.7.10`, appVersion 1.8.7 at the time of writing) has no value for any of
these keys. Its only free-form hook is `deployment.<name>.config.additionalProperties`, a string
rendered last into `dataspaceconnector-configuration.properties`. The keys are an open-ended set
(`fdsc.participants.<key>.*`, `edc.dataspace.profiles.<key>.*`, per-profile `dcp.profiles.*` and
`ebsiTir.profiles.*`), and fdsc-edc may add more.

Each context needs an id, a DID and an STS client whose secret sits in vault under
`<context id>-sts-client-secret`. The IdentityHub bootstrap already registers the connector's DID
and stores its STS secret under `<DID>-sts-client-secret`.

## Decision

- The DSC chart configures participant contexts and profiles through
  `fdsc-edc.deployment.dcp.config.additionalProperties`. It neither templates the fdsc-edc manifests
  nor forks the fdsc-edc chart, in line with the rule that the umbrella only overrides subchart
  values.
- The context id is the participant's DID. The existing identity, STS client and vault alias are
  reused unchanged, and DSP addresses, management paths and token `sub` all name the same string.
- Until an fdsc-edc chart with a 2.x appVersion is published, the image is pinned through
  `fdsc-edc.common.deployment.image.tag`.

## Consequences

- No change to the fdsc-edc chart is needed, and any key a later fdsc-edc release adds is usable
  at once.
- The string is error-prone: every continuation line needs exactly 4 leading spaces, and a
  per-deployment `additionalProperties` replaces the `common` one instead of merging with it, so
  common entries must be repeated. Helm-unittest suites pin the rendered result for the shipped
  values, and the guide documents both traps.
- Nothing validates the keys at render time. A typo shows up as a startup failure or a 404 on the
  DSP routes.
- A context id with `@` is not allowed upstream; DIDs (`did:web:...`) contain only `:`, which is.
- A second dataspace with a second DID is not provisioned by the chart: its IdentityHub participant
  context, STS secret and membership credentials are created through the Identity API, as the guide
  describes.

## Alternatives considered

- **Add dedicated values to the fdsc-edc chart.** Cleaner to use, but it couples every new fdsc-edc
  key to a chart release in another repository and would still need the free-form string for
  per-profile overrides.
- **Template the properties in the umbrella chart** from a structured `participants`/`profiles`
  value. That templates a subchart's configuration from outside and duplicates its rendering logic.
- **Use a separate, short context id** (e.g. `provider`). It needs a second vault alias for the STS
  secret and a mapping between context id and DID in every token and DSP address.
