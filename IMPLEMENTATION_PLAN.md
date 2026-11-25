# Implementation Plan: Support multi-profile edc in fdsc

## Overview
fdsc-edc 2.0.0 ([SEAMWARE/fdsc-edc#39](https://github.com/SEAMWARE/fdsc-edc/pull/39)) runs
`controlplane-dcp` as EDC 0.18.1 virtual participant contexts, so one connector can take part in
several dataspaces, each through its own DSP profiles. This plan moves the DSC to that release, keeps
the `dsp` integration scenario green, demonstrates a second profile, and documents the feature.
The chart itself ships 2.0.0, not only the k3s scenario (Step 1). `controlplane-oid4vc` has no
functional changes in 2.0.0: it moves to the 2.0.0 image with the DCP controlplane and keeps its
configuration.

## Facts every step relies on
Upstream sources: `docs/multi-dataspace.md`, `config/multi-dataspace/edc.properties` and
`releases/notes/RELEASE_NOTES_MULTI_DATASPACE.md` in SEAMWARE/fdsc-edc (tag `2.0.0`).

- **No legacy mode.** The 2.0.0 DCP image always runs in virtual mode. Without
  `fdsc.participants.*` it starts but has no context, so every DSP route returns 404.
- **Participant keys** `fdsc.participants.<key>.*`:
  - `.id`: the context id. `:` is allowed, `@` is not.
  - `.identity`: the DID.
  - `.profiles`: comma-separated profile names.
  - `.config.edc.iam.sts.oauth.{token.url,client.id,client.secret.alias}`.
  - `.privateConfig.*`: only with `edc.participants.config.encryption.algorithm=aes` and
    `edc.encryption.aes.key.alias`.
  - Startup fails if `.config` sets `edc.participant.id`, `edc.participant.did` or
    `edc.iam.issuer.id` to anything other than `.identity`.
- **Profile keys** `edc.dataspace.profiles.<key>.{name,protocol.version,protocol.binding,protocol.namespace,jsonld.context.urls}`.
  Once any profile is declared, the built-in `http-dsp-profile-2025-1` disappears and must be
  declared explicitly to keep it.
  Per-profile overrides:
  - `dcp.profiles.<profile>.scopes.{catalog,negotiation,transfer}`
  - `ebsiTir.profiles.<profile>.tilAddress`
- **Other required keys:**
  - `edc.dsp.callback.address` must contain `%s` (the context id), e.g. `https://host/api/dsp/%s`.
  - `tmfExtension.defaultParticipantContextId`.
  - `tmfExtension.schemaBaseUri`: a copy of the fdsc-edc `schemas/` that the TMForum API can
    reach, with a trailing `/`.
- **Paths:**
  - DSP: `<protocol path>/<contextId>/<profile>/{catalog|negotiations|transfers}/...`.
  - Version metadata: `<protocol path>/<contextId>/.well-known/dspace-version`.
  - OpenID discovery: `/.well-known/openid-configuration/<base64url(contextId), no padding>`.
    The bare path serves the default context.
- **Management API:** v3 and v4 are removed. Calls go to
  `<management path>/v5beta/participants/<contextId>/{catalog/request,contractnegotiations,transferprocesses,assets/request,...}`.
  - Every call needs an OAuth2 JWT, checked against `edc.iam.oauth2.issuer` and
    `edc.iam.oauth2.jwks.url`. Always set the issuer: if it is unset, only tokens without `iss`
    are accepted.
  - The token `scope` must contain `management-api:read` / `management-api:write`.
    `management-api:admin` additionally grants every context; without it, `sub` must equal the
    context id.
  - In request bodies `protocol` (alias `profile`) is a profile name attached to the calling
    context, e.g. `http-dsp-profile-2025-1`, not `dataspace-protocol-http:2025-1`.
- **Helm chart `fdsc-edc`:**
  - There is no dedicated value for any of the keys above. They go through
    `fdsc-edc.deployment.<name>.config.additionalProperties`, a plain string rendered last into
    `dataspaceconnector-configuration.properties` with no `indent`. Every continuation line must
    therefore be pre-indented by exactly 4 spaces, as `k3s/dsp-provider.yaml` already does
    (`"a=1\n    b=2"`).
  - The deployment's config is `mergeOverwrite` of `common` and the deployment, so a per-deployment
    `additionalProperties` **replaces** the common one. Repeat the common entries (the
    `edc.http.client.timeout.*` lines).
  - `config.edc.dspCallback` is rendered verbatim, so `%s` survives.
  - Chart `0.7.11` (appVersion 2.0.0) exists only on the unmerged FIWARE/helm-charts branch
    `chore/update-fdsc-edc-2.0.0` and was not on quay on 2026-11-25. The umbrella pins `0.7.10`
    (appVersion 1.8.7). Both images, `quay.io/seamware/fdsc-edc-controlplane-dcp:2.0.0` and
    `quay.io/seamware/fdsc-edc-controlplane-oid4vc:2.0.0`, are published (2026-11-20).
  - The image tag is `$cfg.deployment.image.tag | default .Chart.AppVersion`, and
    `deployment.yaml` ranges over `.Values.deployment` (default `{}`). The umbrella therefore
    cannot pin a tag for "the dcp deployment" without creating one in every install. It can pin
    `fdsc-edc.common.deployment.image.tag`, which applies to every deployment that sets no tag
    of its own, exactly like an appVersion bump. A helm-unittest probe (v0.6.3) confirmed that
    a `common` tag reaches both a `dcp` and an `oid4vc` deployment.
- **Context id = DID.** The IdentityHub bootstrap already stores the STS secret under
  `did:web:<host>-sts-client-secret`, which is the upstream convention `<context id>-sts-client-secret`.
  Using the DID as context id reuses the existing identity, STS client and vault alias unchanged.
- **Tag 2.1.0, not 2.0.0.** 2.0.0 serves only v5beta and EDC 0.18.1 has no v5 EDR resource, so a
  consumer cannot read a STARTED transfer's `dataaddress`. 2.1.0 (published on quay for both images,
  `RELEASE_NOTES_EDR_API_V5.md`) adds `POST .../v5beta/participants/<ctx>/edrs/request` and
  `GET .../edrs/<transferId>/dataaddress`. The pin and `schemaBaseUri` therefore use 2.1.0; read
  "2.0.0" below as "2.x, pinned at 2.1.0".
- **CI:** CI runs the `dsp` scenario (`.github/workflows/test.yaml`, profiles
  `test,dsp,dsp-test`), so the image switch and the IT migration must land in the same step.

## Steps

### Step 1: Make fdsc-edc 2.0.0 the chart default, run controlplane-dcp in participant-context mode in the k3s DSP scenario and migrate the DSP ITs
Make 2.0.0 the image the chart deploys, and switch the DCP controlplane of the `dsp` scenario to it, configured as one participant
context per connector with the built-in `http-dsp-profile-2025-1`. Keep every existing DSP IT green.
This step is atomic on purpose: CI runs the scenario.

**Chart default image** (the chart ships 2.0.0, not only the k3s scenario):
- In `charts/data-space-connector/values.yaml`, under `fdsc-edc.common.deployment`, add
  `image.tag: "2.1.0"` with a `# --` comment: it stands in for the fdsc-edc chart's appVersion
  until a chart with appVersion ≥ 2.0.0 is published, and Step 2 drops it then.
- It applies to `oid4vc` too, which is what the 0.7.11 bump would do anyway. Upstream did not
  change the OID4VC controlplane in 2.0.0, and `dsp_oid4vc_flow` in this step's CI run proves it.
  If that flow fails on the oid4vc 2.0.0 image, stop and report it as blocked instead of pinning
  around it.
- Do not add a `deployment.dcp` entry to the umbrella values: that would render a DCP
  Deployment in every install.
- Run `.github/scripts/values-schema.sh --check`. `fdsc-edc` is an open object, so the schema
  should not change; regenerate it if it does.
- The k3s files set no tag: they use the chart default, so CI exercises what users install.

**k3s values**, `dcp` deployment only in `k3s/dsp-provider.yaml` and `k3s/dsp-consumer.yaml`:
- `config.edc.dspCallback`: `https://dcp-<participant>.127.0.0.1.nip.io:443/api/dsp/%s`.
- `config.additionalProperties` (4-space pre-indented continuation lines; repeat the timeout lines)
  with:
  - `fdsc.participants.self.{id,identity}`: the participant DID (`did:web:mp-operations.org` /
    `did:web:fancy-marketplace.biz`).
  - `fdsc.participants.self.profiles=http-dsp-profile-2025-1`.
  - The three `fdsc.participants.self.config.edc.iam.sts.oauth.*` keys, with the values the
    common `config.oauth` already carries.
  - `tmfExtension.defaultParticipantContextId=<DID>`.
  - `tmfExtension.schemaBaseUri=https://raw.githubusercontent.com/SEAMWARE/fdsc-edc/2.1.0/schemas/`.
    Verify that the TMForum API reaches it through the squid proxy. If it doesn't, serve the
    schemas in-cluster.
  - `edc.iam.oauth2.issuer` and `edc.iam.oauth2.jwks.url`.
- Do not touch the `oid4vc` deployment. It shares `common.config`, so nothing that goes into
  `common` may be 2.0.0-only.

**Management OAuth2:**
- Consumer: in the consumer's Keycloak realm `test-realm`, add a confidential client-credentials
  client, e.g. `edc-management` with `serviceAccountsEnabled`. Its default client scopes are
  `management-api:read`, `management-api:write` and `management-api:admin`, each with
  `include.in.token.scope: true`. Use `keycloak.realm.clientScopes` and `keycloak.realm.clients`.
  - `k3s/consumer.yaml:724` already sets `keycloak.realm.clients`, and Helm replaces lists across
    values files. Copy the existing entries into the `dsp-consumer.yaml` override, or add the client
    where it lives today, but do not drop them.
  - `edc.iam.oauth2.issuer` must equal the `iss` Keycloak puts in tokens. That is derived from the
    host the IT requests the token from, so use the same external Keycloak URL in both places.
  - `jwks.url` must use a `*.cluster.local` host, e.g.
    `http://consumer-keycloak.consumer.svc.cluster.local:8080/...`. A bare `consumer-keycloak` host
    goes through the squid proxy, because `JAVA_TOOL_OPTIONS` `nonProxyHosts` only matches
    `*.cluster.local`.
- Provider: set the same two keys against the provider's Keycloak (`k3s/provider.yaml`, realm
  `test-realm`), even though no IT calls the provider's management API.

**Routes and PEP:**
- Check the APISIX routes for the DCP hosts (`k3s/consumer-auth.yaml` around lines 238-300, and
  the provider's DSP routes in `k3s/provider.yaml` / `k3s/dsp-provider.yaml`).
- Move any no-token exemption for `/.well-known/dspace-version` to
  `/api/dsp/<ctx>/.well-known/dspace-version`.
- Check that `fdscTransfer.dcp.oid.host` discovery still resolves: the bare path serves the
  default context.
- The management route (`dsp-dcp-management` → `:8085`) only proxies; the token travels in the
  `Authorization` header.

**ITs** (`it/src/test/java/org/fiware/dataspace/it/components/`):
- `DSPEnvironment.java`: add public constants (one-line Javadoc each) for:
  - the consumer Keycloak token URL;
  - the management client id and secret;
  - the provider and consumer context ids;
  - the profile name `http-dsp-profile-2025-1`.
- `DSPManagementHelper.java`:
  - DCP calls use `/api/v1/management/v5beta/participants/<ctx>/...` (verify the management path
    prefix against the running pod) with a bearer token from client-credentials. Cache the token
    until it expires.
  - `protocol` in DCP request bodies is the profile name.
  - Assets are listed with `POST assets/request` and body `{}`.
  - Check whether v5beta still exposes EDRs under `edrs/` and adapt `EDRS_PATH_PREFIX`.
  - **OID4VC calls keep v3, no token, `dataspace-protocol-http:2025-1`.** That image did not
    change. Split the path constants per flavour; do not switch them globally.
- `DSPStepDefinitions.java`: the DCP `counterPartyAddress` and any other use of `DSP_ENDPOINT_PATH`
  for the DCP provider (lines ~1171 and ~1623) become `DCP_PROVIDER_ADDRESS + "/api/dsp/<ctx>/<profile>"`.
  The OID4VC ones keep `/api/dsp/2025-1`.
- Offering and policy ids must now be globally unique per controlplane. Check that the IT's
  `OFFER-*` / policy ids are.

**Acceptance:**
- `mvn clean deploy -Plocal,dsp`, then `mvn clean integration-test -Ptest,dsp,dsp-test` passes:
  `dsp_dcp_flow`, `dsp_oid4vc_flow`, `dsp_advanced_policy`, `dsp_identity_setup` and
  `dsp_tmforum_ordering` are all green.
- If no local k3s is available, at least `helm template` the provider and consumer with the
  `dsp` value sets. Inspect the rendered `dataspaceconnector-configuration.properties`: one key
  per line, `%s` intact. Rely on CI for the run.

### Step 2: Pin the multi-dataspace rendering with helm-unittest and move the chart to fdsc-edc 2.0.0
- **Dependency** in `charts/data-space-connector/Chart.yaml`:
  - Run `helm show chart oci://quay.io/fiware/helm-charts/fdsc-edc --version 0.7.11` (or a newer
    version with appVersion ≥ 2.0.0). If one is published, bump the `fdsc-edc` dependency to it,
    run `helm dependency update charts/data-space-connector`, and drop the now-redundant
    `fdsc-edc.common.deployment.image.tag` pin from Step 1.
  - If none is published, keep `0.7.10` and the pin. The chart still deploys 2.0.0 through it.
    Record in the release notes that the pin goes away with the next fdsc-edc chart bump, and
    that anyone who sets their own `image.tag` on a deployment overrides it. Do not change
    FIWARE/helm-charts from here.
- **Chart version:**
  - `Chart.yaml` reads `10.21.2`, the last version on gh-pages. `doc/release-notes/11-x.md`
    describes an 11.0.0 that a bot commit (`8313e4a3`) reverted and that was never published.
  - Either way, after Step 1 the chart's default DCP image is 2.0.0, so every DCP deployment
    changes behaviour. That breaks existing users, so set `version: 11.0.0` in both cases. The
    release notes describe the pin or the bump, whichever applies. Add a new top section
    `## 11.0.0 — fdsc-edc 2.0.0: multi-dataspace controlplane-dcp` in `doc/release-notes/11-x.md`,
    above the consent section, and adjust the file's intro paragraph, which currently says only
    consent users are affected.
  - Point out the 11.0.0 ambiguity in the PR description for the maintainers.
  - `check.yaml`'s `chart-version` job (FIWARE `chart-pr.yml`) bumps the charts a PR changes,
    based on a semver label. Ask for the `major` label in the PR description. Before committing
    11.0.0 by hand, check how that job treats a version that is already bumped, so the chart
    does not land on 12.0.0.
- **Release notes:** a breaking-changes list for DCP users and a `:warning: Required action`
  checklist:
  - `fdsc.participants.*` (context id = DID);
  - callback with `%s`;
  - `tmfExtension.defaultParticipantContextId` / `schemaBaseUri`;
  - management v5beta + OAuth2 issuer/JWKS;
  - new DSP paths for counterparties and PEP exemptions;
  - `testExtension.*` removed;
  - globally unique offering/policy ids.
  - Link the upstream release notes and `doc/MULTI_DATASPACE.md` (created in Step 4; a forward
    link is fine, `check-links.sh` runs in CI, so create the file as a stub here if needed).
- **New suite** `charts/data-space-connector/tests/fdsc-edc-multi-dataspace_test.yaml`. It opens
  with a short header comment and has one `it:` per behaviour. No existing suite renders a
  subchart template, so there is no precedent to copy:
  - List `charts/fdsc-edc/templates/config-map.yaml` (and `deployment.yaml` for the image
    assertion) under `templates:`. The path is relative to the umbrella chart and resolves into
    the `charts/fdsc-edc-<version>.tgz` that `helm dependency update` fetches. CI runs that before
    `helm unittest`.
  - Set `fdsc-edc.deployment.{dcp,oid4vc}` in the suite's `set:` and pick each ConfigMap with
    `documentSelector` on `metadata.name` (`RELEASE-NAME-fdsc-edc-dcp`, `...-oid4vc`,
    `skipEmptyTemplates: true`). Match lines in `data["dataspaceconnector-configuration.properties"]`
    with `matchRegex` and a `(?m)^ *<key>=<value>$` pattern.
  - A throwaway probe built this way passed under helm-unittest v0.6.3 during planning, and it
    failed when the expected value was changed. The resolution itself is not a risk.
  Use a two-participant, two-profile `additionalProperties` value and assert that:
  - each `fdsc.participants.*` / `edc.dataspace.profiles.*` key lands on its own line (the
    4-space indentation contract);
  - the `%s` callback is rendered verbatim;
  - the `oid4vc` deployment's ConfigMap does not get them;
  - both rendered Deployments use image tag `2.0.0` when no tag is set: Step 1's pin, or the
    bumped appVersion.
  Define repeated literals once (YAML anchors).
- `.github/scripts/values-schema.sh --check` must still pass: `fdsc-edc` is an open object, so
  adding or dropping the `common.deployment.image.tag` pin does not change the schema.
- **Acceptance:** `helm lint`, `helm unittest charts/data-space-connector`,
  `.github/scripts/render-examples.sh` and `values-schema.sh --check` all pass.

### Step 3: Demonstrate a second dataspace profile in the k3s DSP scenario
Attach a second profile to both DCP contexts and prove it end to end.
- `k3s/dsp-provider.yaml` and `k3s/dsp-consumer.yaml`, `additionalProperties`:
  - Declare both profiles explicitly through `edc.dataspace.profiles.<key>.*`:
    `http-dsp-profile-2025-1` plus a second one, e.g. `dsp-fiware-ds2` (DSP 2025-1, `HTTPS`,
    namespace `https://w3id.org/dspace/2025/1/`). Declaring any profile removes the built-in one.
  - Set `fdsc.participants.self.profiles` to both.
  - Give the second profile its own `dcp.profiles.dsp-fiware-ds2.scopes.*` and
    `ebsiTir.profiles.dsp-fiware-ds2.tilAddress`. They can point to the same
    MembershipCredential / TIL as the default, because k3s has one trust anchor. Each must be set
    explicitly, to show the per-dataspace knobs.
  - Never configure the same scope through both `dcp.*` and `edc.iam.dcp.scopes.*`: the results
    are unioned.
- New feature `it/src/test/resources/it/dsp_multi_profile.feature`, run under the existing `dsp`
  tags/profile. Step definitions go in `DSPStepDefinitions.java`, or in a new
  `DSPMultiProfileStepDefinitions.java` if that file grows unwieldy. Scenarios:
  - the consumer requests the provider's catalog through each profile and gets the offers;
  - routing before authentication: send raw, tokenless `POST`s to the provider's
    `DCP_PROVIDER_ADDRESS/api/dsp/<ctx>/<profile>/catalog/request`.
    - A profile that is not attached, e.g. `dsp-unknown`, gets **404**.
    - An attached profile without a DCP token gets **401**.
    - The two are distinguishable only because fdsc-edc 2.0.0 resolves the context and profile
      before it checks the token (upstream `docs/multi-dataspace.md`, "Poking at it yourself").
      The provider's APISIX route for `dcp-mp-operations` (`k3s/provider.yaml`, `uri: /*` to
      `provider-fdsc-edc-dcp:8080`) has no auth plugin, so the request reaches EDC unchanged.
    - Assert both exact codes against the running pod. If the pod answers differently, report
      it rather than loosening the assertion to "any 4xx";
  - `/api/dsp/<ctx>/.well-known/dspace-version` answers 200 without a token and lists both
    profiles, and only those.
  Use Cucumber `Scenario Outline` / `Examples` to parameterise over the profiles.
- Out of scope: a second DID (a second participant context). The chart's
  `identityhub.bootstrap` provisions exactly one participant; Step 4 documents that path.
- **Acceptance:** the whole `dsp` IT suite passes locally or in CI, including the new feature.

### Step 4: Document multi-dataspace operation
- **New guide `doc/MULTI_DATASPACE.md`**, linking the upstream `docs/multi-dataspace.md` for the
  full key reference:
  - What a participant context and a profile are, and how they map to DSC values: everything goes
    through `fdsc-edc.deployment.dcp.config.additionalProperties`, plus the indentation and
    replace-not-merge gotchas.
  - A complete single-connector, two-profile values example taken from Step 3.
  - A two-dataspace / two-DID example. It covers what the DSC chart provides and what it doesn't:
    a second IdentityHub participant context via the Identity API (`/api/identity/v1beta`), its
    STS secret alias `<ctx>-sts-client-secret`, membership credentials from the second
    dataspace's issuer, the per-profile TIL, and TMForum tagging with
    `defaultParticipantContextId`.
  - The management API: v5beta paths, the Keycloak client and scopes from Step 1, and a
    `sub` = context id mapper for non-admin tokens.
  - DSP URLs for counterparties.
  - The migration checklist from the release notes.
  - Verification with `curl`: dspace-version, catalog per profile.
- **`doc/DSP_INTEGRATION.md`:** update the DCP walkthrough (`#### DCP`, line ~937, and the setup
  sections ~123-238) to the v5beta management calls with a bearer token and the new
  `counterPartyAddress`. Add a short "Multiple dataspaces" subsection that links the guide. Keep the
  OID4VC examples as they are.
- **Links:** `README.md` (DSP section ~line 499 and the docs list ~line 633) and `doc/README.md`
  (index/glossary ~line 182).
- **ADR:** `doc/adr/0009-fdsc-edc-multi-dataspace-via-additional-properties.md` (MADR-lite like
  0008) plus a row in `doc/adr/README.md`. Decision: configure participant contexts through the
  subchart's `additionalProperties`, not by templating or forking the fdsc-edc chart, and use the
  DID as context id.
- **`AGENTS.md`:** update the fdsc-edc section with what Steps 1-3 learned. Fix the
  `Chart.yaml` version under Important Files.
- **Acceptance:** `.github/scripts/check-links.sh` passes, and every values snippet in the guide
  is copied from a values file that CI renders or deploys.
