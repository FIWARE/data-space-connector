# 0012 - Order the model bundles with Masterminds/semver, the library Helm uses

**Status:** Proposed
**Date:** 2026-11-16
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/TECH_STACK.md`](../config-tool/TECH_STACK.md)

## Context

[ADR 0009](0009-config-tool-stack.md) fixes the tool's libraries through the table in
TECH_STACK, and a deviation needs an ADR. CT-1 (#116) orders the bundles under
`config-tool/model/<chart-version>/` and marks the highest release `latest`, which the S1 Version
dropdown defaults to. The table names no semver library, and `golang.org/x/mod/semver` accepts
`10` and `10.16`, which Helm refuses as chart versions.

## Decision

Use `github.com/Masterminds/semver/v3` with `StrictNewVersion`, and add it to the TECH_STACK
library table. A bundle directory that is not a strict `MAJOR.MINOR.PATCH[-pre][+build]` version
is an error.

## Consequences

- The tool and Helm agree on which versions are valid and on their order.
- No new dependency in practice: the Helm SDK in the table already pulls the library in.
- ADR 0009's decision is unchanged; this record only adds a row to its library table.

## Alternatives

- **`golang.org/x/mod/semver` plus a three-component check.** Works, but is a second semver
  implementation next to Helm's, and its ordering would have to be kept in line by hand.
- **Amend ADR 0009 in place.** Rejected to keep the decision log append-only.
