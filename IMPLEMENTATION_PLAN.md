# Implementation Plan: Update eIDAS 2.0 documentation in Data Space Connector

## Overview

The VCVerifier has overhauled its eIDAS 2.0 support (app version 6.22.0): the old JAdES/DSS-based verification path has been replaced by built-in PKIX certificate chain validation against the EU Trusted Lists (ETSI TS 119 612). The Data Space Connector's documentation, example values, and Helm configuration still reference the old approach (external `dss-validation-service`, `keycloak-jades-vc-issuer` plugin, JAdES envelope validation). This plan updates the connector to the new eIDAS model, bumps the VCVerifier dependency chain, and rewrites the affected documentation.

## Steps

### Step 1: Update decentralized-iam dependency and values.yaml eIDAS config surface

**Goal:** Bump the `decentralized-iam` chart dependency to pick up the VCVerifier version that includes built-in eIDAS 2.0 trust list validation (app version ≥ 6.22.0), and update the `values.yaml` config surface accordingly.

**Pre-requisite check:** Before implementing, verify that the upstream dependency chain has been updated:
- `vcverifier` Helm chart ≥ 4.12.27 (or whichever version includes appVersion 6.22.0) — must add `eidas:` to the configmap template rendering
- `vc-authentication` chart bumps its `vcverifier` dependency to include the above
- `decentralized-iam` chart bumps its `vc-authentication` dependency

If the upstream charts are not yet updated, open tracking issues and document the required upstream versions in this file. Proceed with steps 2-5 against the expected config shape, gating the Chart.yaml version bump on upstream availability.

**Files affected:**
- `charts/data-space-connector/Chart.yaml` — bump `decentralized-iam` version from `2.1.22` to the version that bundles VCVerifier ≥ 6.22.0
- `charts/data-space-connector/values.yaml`:
  - Under the `decentralizedIam.vcAuthentication` section (lines ~123–132), add a deprecation comment to the `dss:` block explaining that the DSS validation service is superseded by VCVerifier's built-in eIDAS validation in chart version ≥ (new version). Keep the block for backward compatibility but mark it deprecated.
  - Add a new `eidas:` configuration block under `decentralizedIam.vcAuthentication.vcverifier.deployment` (or wherever the upstream vcverifier chart exposes it) with helm-docs comments (`# --` format) for the following keys:
    - `enabled` (default: `false`) — master toggle for eIDAS trust list validation
    - `lotlUrl` (default: official EU LOTL URL) — URL of the EU List of Trusted Lists
    - `refreshInterval` (default: `86400`) — trust list refresh interval in seconds
    - `countries` (default: `[]` = all) — ISO 3166-1 alpha-2 country filter
    - `maxWorkers` (default: `5`) — concurrent trust list fetch workers
    - `fetchTimeout` (default: `30`) — HTTP timeout per trust list fetch
    - `allowStaleTrustLists` (default: `false`) — accept expired trust lists
    - `statusEvaluation` (default: `current`) — `current` or `issuance` evaluation mode
    - `revocationCheck` (default: `soft`) — `off`, `soft`, or `hard`
    - `revocationTimeout` (default: `10`) — OCSP/CRL timeout
    - `revocationCacheExpiry` (default: `3600`) — revocation status cache lifetime

**Acceptance criteria:**
- `helm dependency update charts/data-space-connector` succeeds with new version
- `helm lint charts/data-space-connector` passes
- New eIDAS values appear in rendered output when enabled
- DSS block has deprecation notice in comments

### Step 2: Update k3s example values files for new eIDAS approach

**Goal:** Rewrite the k3s overlay values files that demonstrate eIDAS/ELSI deployment to use the new VCVerifier-native trust list validation instead of the external DSS service.

**Files affected:**
- `k3s/provider-elsi.yaml` (371 lines) — major rewrite:
  - Remove the entire `dss:` block (lines ~90–117) — DSS validation service no longer needed
  - Remove `crl:` secret and DSS keystores configuration
  - Remove `additionalContainers` for CRL provider
  - Update the `vcverifier.deployment.verifier.elsi` block: remove `validationEndpoint` (pointing to DSS), since `did:elsi` verification is now automatic when `eidas.enabled: true`
  - Add `vcverifier.deployment.eidas` block with `enabled: true` and appropriate settings for local testing (e.g., `revocationCheck: off` for local envs without real OCSP/CRL)
  - Keep the verifier's `elsi.enabled: true` (the DID method is still supported, just verified differently)

- `k3s/consumer-elsi.yaml` (414 lines) — investigate and update:
  - The `keycloak-jades-vc-issuer` init container (line ~94) may or may not still be needed. Investigation required:
    - If VCVerifier 6.22.0 just needs regular JWTs with `x5c` headers (not full JAdES), and standard Keycloak OID4VCI issuance includes `x5c` when using a java-keystore key provider, the JAdES plugin can be removed
    - If `x5c` inclusion still requires the plugin, keep it but add a comment explaining it's needed for x5c header injection, not JAdES format
  - The `elsi:` block (lines ~404–413) for keystore configuration likely stays unchanged (still needed for issuing credentials with eIDAS certificates)
  - Update any comments referencing the old validation flow

- `pom.xml` — check if the `elsi` Maven profile (line ~718) needs changes for the new deployment pattern (it references `provider-elsi.yaml` and `consumer-elsi.yaml`, which will be updated)

**Acceptance criteria:**
- `helm template test charts/data-space-connector -f k3s/provider.yaml -f k3s/provider-elsi.yaml` renders without errors
- `helm template test charts/data-space-connector -f k3s/consumer.yaml -f k3s/consumer-elsi.yaml` renders without errors
- No DSS-related resources in provider template output
- VCVerifier configmap includes `eidas:` section with `enabled: true` in provider output

### Step 3: Create standalone eIDAS documentation and rewrite LOCAL.MD section

**Goal:** Split the eIDAS documentation from `LOCAL.MD` into a standalone document (as the ticket suggests) and rewrite it to cover the new VCVerifier-native trust list validation approach.

**Files affected:**

- **New file:** `doc/deployment-integration/eidas/README.md` — standalone eIDAS documentation covering:
  1. **Overview**: What eIDAS 2.0 compliance means for the Data Space Connector — `did:elsi` DID method for organization identification, PKIX certificate chain validation against EU Trusted Lists
  2. **Architecture**: How VCVerifier validates `did:elsi` credentials (extract x5c → match organizationIdentifier OID 2.5.4.97 → verify JWT signature → validate chain against trust store built from EU LOTL)
  3. **Configuration — Provider (Verifier)**: New `eidas:` config block, what each setting does, recommended values for production vs. test environments. Explain that `eidas.enabled: true` must be set when using `did:elsi`, that the `dss-validation-service` is no longer needed
  4. **Configuration — Consumer (Issuer)**: How to configure Keycloak to issue `did:elsi` credentials (eIDAS certificate in java-keystore, `elsi:` block in values.yaml), whether keycloak-jades-vc-issuer is still needed or can be replaced
  5. **Preparation — Generating test certificates**: Updated instructions for generating eIDAS certificate chains (same FIWARE/eIDAS tool, but emphasize the certificate requirements for the new validation path)
  6. **Per-credential eIDAS validation (SD-JWT)**: Document the `eidasConfig` per-credential-type option for SD-JWT validation
  7. **Certificate revocation**: Explain `revocationCheck` modes (off/soft/hard), OCSP and CRL endpoint handling
  8. **Trust list freshness**: Explain `refreshInterval`, `allowStaleTrustLists`, rollback protection
  9. **Local deployment**: Updated `mvn clean deploy -Plocal,elsi` instructions with new values
  10. **Migration from DSS-based approach**: Step-by-step migration guide from old (chart ≤ 10.7.0) to new approach
  11. **Troubleshooting**: Common error scenarios from VCVerifier (`eidas_trust_store_required_for_did_elsi`, `eidas_issuer_not_trusted_by_trust_list`, `certificate_revoked`, etc.)

- `doc/deployment-integration/local-deployment/LOCAL.MD`:
  - Replace the eIDAS section (lines 1326–1512) with a brief summary and a cross-reference to the new standalone document
  - Keep it to ~10 lines: mention that eIDAS support is available, link to the new doc

**Acceptance criteria:**
- New `doc/deployment-integration/eidas/README.md` exists and covers all sections
- `LOCAL.MD` eIDAS section replaced with cross-reference
- All internal links (to k3s files, templates, etc.) are valid
- No references to `dss-validation-service` or JAdES validation in new docs (except in migration section)

### Step 4: Update cross-references, role documentation, and release notes

**Goal:** Update all documentation files that reference the old eIDAS approach, and add release notes for the eIDAS 2.0 update.

**Files affected:**
- `doc/deployment-integration/roles/provider/README.md` (line ~481):
  - Update the ELSI overlay table entry description to reflect the new approach (no longer mentions DSS)
  
- `doc/deployment-integration/roles/consumer/README.md` (line ~189):
  - Update the ELSI overlay table entry description if needed

- `doc/release-notes/` — add a new release notes file (or append to existing `10-x.md`):
  - Document the eIDAS 2.0 verification change
  - List breaking changes: `dss` block deprecated, `eidas` block required for `did:elsi`
  - Migration guide summary with link to standalone doc
  - New VCVerifier version and capabilities

- `CLAUDE.md` — update the "Keycloak / OID4VCI" section or add a new "eIDAS 2.0" section documenting the new config paths and gotchas for future agent sessions

**Acceptance criteria:**
- All provider/consumer doc references updated
- Release notes entry added with breaking changes and migration path
- No stale references to DSS-based eIDAS validation in documentation
- CLAUDE.md updated with eIDAS 2.0 context

### Step 5: Verification — linting, template rendering, and unit tests

**Goal:** Verify that all changes are consistent and the chart renders correctly.

**Tasks:**
1. Run `helm dependency update charts/data-space-connector`
2. Run `helm lint charts/data-space-connector`
3. Run `helm template` with the updated provider-elsi and consumer-elsi overlays:
   - `helm template test charts/data-space-connector -f k3s/provider.yaml -f k3s/provider-elsi.yaml`
   - `helm template test charts/data-space-connector -f k3s/consumer.yaml -f k3s/consumer-elsi.yaml`
4. Run `helm unittest charts/data-space-connector` — verify existing tests still pass
5. If applicable, add new test cases in `charts/data-space-connector/tests/` to verify:
   - eIDAS config is rendered in the VCVerifier configmap when enabled
   - DSS block still works when explicitly enabled (backward compatibility)
   - The `elsi-secret.yaml` template still renders correctly
   - The `_realm.tpl` elsi key provider configuration still works
6. Verify all documentation links resolve correctly
7. Verify `helm template` output with the `elsi` profile matches expected structure (no DSS resources, has eIDAS config in verifier)

**Acceptance criteria:**
- `helm lint` passes without warnings
- `helm template` renders cleanly for all value file combinations
- All existing unit tests pass
- New tests (if added) pass
- No broken documentation links
