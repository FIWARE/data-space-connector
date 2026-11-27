# Feature model

The data the tool is made of: what a *feature* is, which ones exist, what a *role preset*
selects, which values are *cross-cutting*, how an infrastructure dependency can be provided,
and what is validated. The UI in [MOCKUPS.md](MOCKUPS.md) is a rendering of this model; the
stories in [USER_STORIES.md](USER_STORIES.md) are the behaviour around it.

<!-- ToC -->
- [Concepts](#concepts)
- [The feature catalogue](#the-feature-catalogue)
- [Role presets](#role-presets)
- [Cross-cutting values](#cross-cutting-values)
- [Integration modes](#integration-modes)
- [Round-trip and provenance](#round-trip-and-provenance)
- [Validation rules](#validation-rules)
- [Where the model lives](#where-the-model-lives)
<!-- /ToC -->

## Concepts

```
Profile (the answer set, the tool's whole state)
├── target        chart + version, namespace, release, kubeconfig context, environment
├── role          consumer | provider | consumer-provider | operator | custom
├── foundations   the cross-cutting values, answered once
├── infrastructure  one integration mode + its references per dependency
├── features      { featureId: { enabled, source: preset|user, answers: {...} } }
├── secrets       { secretName: { mode, keys, fingerprints } } — modes, never values
└── overrides     raw values keys the tool does not own (hand-edited or loaded)

values.yaml  =  render(profile)  ⊕  overrides        (⊕ = deep merge, overrides win)
```

`overrides` is referenced, not copied: the `.dscprofile.yaml` records only the values file's name,
root and SHA-256 under `source`, because unowned keys may hold plaintext secrets. Reopening reads the
file next to the profile again ([ADR 0015](../adr/0015-config-tool-profile.md)).

A **feature** is a capability a user wants, not a component. One feature may enable several
subcharts (`market.local` turns on the BAE, MongoDB and the TMForum API); one subchart may
serve several features (`keycloak` serves issuance, and the wallet preset, and eIDAS). The
feature is the unit of *enable/disable*, of *recommendation*, and of *one configuration page*.

A feature declares:

| Field | Meaning |
|---|---|
| `id`, `title`, `summary` | `dsp.edc`, "Dataspace Protocol (DSP)", one line of what it gives you |
| `group` | Identity · Access · Data · Contracts · Marketplace · Interop · Consent · Observability · Operator (the trust-anchor features `ta.*`) |
| `requires` / `suggests` / `conflicts` | Other features, resolved when toggling |
| `needs` | Infrastructure dependencies (`postgres`, `mongo`, `certs`, `vault`) |
| `keys` | The chart keys it owns, with the render function per key |
| `secrets` | The Secrets it needs: name, keys, class (bootstrap / external / identity) and the chart switch that owns them, see [Secret handling](SECRETS.md#what-a-connector-needs) |
| `questions` | The ordered question list of its configuration page |
| `docs` | Deep link into the role guides |
| `since` / `removedIn` | Chart versions it exists in |

## The feature catalogue

Toggle defaults differ from the **chart** defaults on purpose: the chart turns several
components on by default for the local demo, the tool starts from the role.

The bundle writes `x.*` as `x`, because a key names its whole subtree, and lists concrete paths instead of
qualifiers such as "+ ingress". Features may share keys (`vcCredentials`, `grafana`, `contract-management`).
A shared key is needed while any feature that lists it is enabled ([ADR 0017](../adr/0017-config-tool-role-presets.md)).

### Identity

| Feature | Gives you | Chart keys | Needs |
|---|---|---|---|
| `identity.issuer` | Issue Verifiable Credentials to your users (Keycloak, OID4VCI) | `keycloak.*`, `vcCredentials` | postgres, certs |
| `identity.did` | Publish the organization's DID document (`did:web`/`did:key`) | `did.*` | certs |
| `identity.statuslist` | Revoke issued credentials (token status list) | `statusListServer.*` | postgres |
| `identity.vcoperator` | Manage credential definitions as Kubernetes resources | `vc-operator.*`, `vcCredentials.*` | — |
| `identity.wallets` | Preset clients for the Lissi and EUDI wallets | `keycloak.realm.wallets.*` | `identity.issuer` |
| `identity.eidas` | eIDAS/ELSI certificate-based identity and validation | `elsi.*`, `decentralizedIam.vcAuthentication.vcverifier.deployment.eidas` | `identity.issuer` |

### Access

| Feature | Gives you | Chart keys | Needs |
|---|---|---|---|
| `access.verification` | Verify incoming credentials, hand out access tokens (VCVerifier + local Trusted Issuers List) | `decentralizedIam.vcAuthentication.*` | postgres, certs |
| `access.policies` | Enforce ODRL policies on every request (APISIX + OPA + ODRL-PAP) | `decentralizedIam.odrlAuthorization.*` | postgres, certs |

### Data

| Feature | Gives you | Chart keys | Needs |
|---|---|---|---|
| `data.scorpio` | An NGSI-LD context broker as the data service | `scorpio.*` | postgres (PostGIS) |
| `data.external` | Protect data services you run yourself | `decentralizedIam.odrlAuthorization.apisix.routes` | `access.policies` |

### Contracts and marketplace

| Feature | Gives you | Chart keys | Needs |
|---|---|---|---|
| `contract.tmforum` | TMForum product/contract APIs | `tm-forum-api.*` | postgres |
| `contract.management` | Turn orders into access policies automatically | `contract-management.*` | `contract.tmforum`, `access.policies` |
| `market.local` | A marketplace of your own (BAE) | `marketplace.*` | mongo, `contract.tmforum` |
| `market.central` | Publish into the data space's central marketplace | `contract-management.*` (central endpoints) | `contract.tmforum` |

### Interoperability

| Feature | Gives you | Chart keys | Needs |
|---|---|---|---|
| `dsp.edc` | Dataspace Protocol via EDC, with DCP identity (IdentityHub) | `fdsc-edc.*`, `identityhub.*`, `registration.*` | postgres, vault, certs |
| `dsp.rainbow` | The Rust DSP implementation (Rainbow) | `rainbow.*` | postgres |
| `dsp.config` | Serve `/.well-known/data-space-configuration` | `dataSpaceConfig.*` | certs |

### Consent

| Feature | Gives you | Chart keys | Needs |
|---|---|---|---|
| `consent.provider` | Enforce data-subject consent on provider APIs | `consent-facade.*`, `consent-owner-resolver.*` | `access.policies` |
| `consent.authority` *(Operator)* | Run the data space's consent manager | trust-anchor `consentManagement.*`, `consent-manager.*`, `consent-facade.*` | postgres |

### Operator role (trust-anchor chart)

| Feature | Gives you | Chart keys |
|---|---|---|
| `ta.registry` | Trusted Issuers Registry + Trusted Issuers List API | `trusted-issuers-list.*` |
| `ta.onboarding` | Participant onboarding | `trusted-issuers-list.*` + ingress |

### Observability

| Feature | Gives you | Chart keys |
|---|---|---|
| `obs.tracing` | Distributed traces from every component | `tracing.*`, `opentelemetry-collector.*`, `opentelemetry-operator.*` |
| `obs.traceStore` | Store and browse traces (Tempo + Grafana) | `tempo.*`, `grafana.*` |
| `obs.metrics` | Metrics and dashboards | `prometheus.*`, `grafana.*`, `fdsc-dashboard.*` |

## Role presets

`R` recommended on, `o` available off, `–` not offered (off, but the user may turn it on when the feature belongs to
the role's chart, [ADR 0018](../adr/0018-config-tool-deliberate-choices.md)).

| Feature | Consumer | Provider | Consumer + Provider | Operator | Custom |
|---|---|---|---|---|---|
| `identity.issuer` | **R** | **R** | **R** | – | o |
| `identity.did` | **R** | **R** | **R** | – | o |
| `identity.statuslist` | o | o | o | – | o |
| `identity.vcoperator` | o | o | o | – | o |
| `identity.wallets` | o | o | o | – | o |
| `identity.eidas` | o | o | o | – | o |
| `access.verification` | – | **R** | **R** | – | o |
| `access.policies` | – | **R** | **R** | – | o |
| `data.scorpio` | – | **R** | **R** | – | o |
| `data.external` | – | o | o | – | o |
| `contract.tmforum` | – | o | o | – | o |
| `contract.management` | – | o | o | – | o |
| `market.local` | – | o | o | – | o |
| `market.central` | – | o | o | – | o |
| `dsp.edc` | o | o | o | – | o |
| `dsp.rainbow` | o | o | o | – | o |
| `dsp.config` | o | o | o | – | o |
| `consent.provider` | – | o | o | – | o |
| `consent.authority` | – | – | – | o | – |
| `ta.registry` | – | – | – | **R** | – |
| `ta.onboarding` | – | – | – | o | – |
| `obs.tracing` | o | o | o | – | o |
| `obs.traceStore` | o | o | o | – | o |
| `obs.metrics` | o | o | o | – | o |

The component matrix in [roles/README.md](../deployment-integration/roles/README.md#component-matrix) is
authoritative: `Required` is **R**, `Optional` is o, `-` is –, and `Built into X` takes row X's value.
`internal/model/rolematrix_test.go` fails when the bundle disagrees with it. Rows that no matrix component
maps to (`data.scorpio`, `market.*`, `obs.*`, ...) follow this document.

A role offers only features of the chart it deploys. Operator deploys `trust-anchor`, so it offers the three
trust-anchor features and nothing else, `dsp.config` and `obs.*` included. Custom deploys `data-space-connector`:
it offers all of that chart's features and recommends none ([ADR 0017](../adr/0017-config-tool-role-presets.md)).

Changing the role re-applies the new preset only to features whose state came from the old preset: a toggle set by
the user or by a loaded values file keeps its state, and answers stay ([ADR 0019](../adr/0019-config-tool-role-change.md)).

## Cross-cutting values

Asked in [Stage 3](USER_JOURNEY.md#stage-3--foundations). The "fans out to" count is measured
against [`k3s/provider.yaml`](../../k3s/provider.yaml), which is what a fully configured
provider looks like today.

The rows down to the data space's, and the proxy and private CA rows, are shipped as `foundations:` in the bundle ([ADR 0020](../adr/0020-config-tool-foundations-render.md)),
and `internal/render/provider_test.go` measures their counts. That test accounts for every leaf of provider.yaml that
carries the domain: it is either a target or a row of an exclusion table naming the owning story. The TIR address is
the `dataspace.tirAddress` target, which carries the domain without deriving from it. The excluded rows are the
verifier's trusted participant lists (8, US-5.6, [ADR 0022](../adr/0022-config-tool-data-space-parameters.md#1-only-the-tir-address-renders)),
the APISIX `routes[*].host` (18, US-5.6), the realm clients (2, US-5.5), the dashboard's provider issuer and TPP's
in-cluster transfer host (1 each, US-5.3). The other rows are design.

| Value | Example | Fans out to |
|---|---|---|
| `org.displayName` | `MP Operations` | **1 key** shipped: the Keycloak realm's `displayNameHtml` (provider.yaml sets none) |
| `domain.base` | `mp-operations.org` | **76 keys in 7 components** of the provider profile, through the 22 derived hosts on it: every ingress host and its TLS `secretName` and host list, every `cert-manager.io/*` annotation, public URLs (Keycloak `frontendUrl`, the status list, the dashboard). The other 31 of the 107 leaves that carry the domain are the TIR address and leaves of later stories (see above). The verifier and DID document hosts sit on `mp-operations.org` in provider.yaml; through them and the DID the UI counts 89 keys in 10 components |
| `host.<component>` | `keycloak.mp-operations.org` | derived from `domain.base` as `<component>.<base>` (`host.did` as the base itself); each can be overridden, which pins it: a base change re-derives the others and lists the pinned ones as *pinned, not updated* ([ADR 0023](../adr/0023-config-tool-pinned-hosts.md)). Two hosts on one name are an error; **6 keys** for Keycloak. `host.verifier` and `host.did` leave their TLS certificate to US-8.11 |
| `did.method` / `did.value` | `did:web:mp-operations.org` | **8 keys in 4 components** shipped: verifier `did`, contract-management `did`, Keycloak `issuerDid`, and under `fdsc-edc.common.config` the participant id, `issuerId`, the STS `oauth.clientId` and `secretAlias` (`<did>-sts-client-secret`) and `oid4vp.holder.id`. `did.value` derives `did:web:<host.did>` while the method is `did:web` and must start with `<method>:`. The DID keys left to other stories are listed below the table |
| `dataspace.tirAddress` | `https://tir.dataspace.org` | **1 key** shipped: the verifier's `tirAddress`. A loaded file's value is recognized and back-derives the answer. The service registration's `trustedParticipantsLists` are US-5.6's; the verifier's k3s-only TIR registration script has no chart key |
| `dataspace.credentialTypes` | `MembershipCredential`, `UserCredential` | shipped as an **answer only**: the chart key, `...registration.services[*].oidcScopes.*.credentials[*].type`, sits in the service registration list that US-5.6 owns and pre-seeds from it. Also read for Keycloak client scopes and protocol mappers, local TIL entries (US-5.5) and ODRL policies |
| `dataspace.autoRegistration` | `true` | shipped as an **answer only**: chart 10.16.0 has no key for registering at the Trust Anchor (`registration.*` fills the participant's own TIL). Read by US-5.5 and the next-steps page |
| `tls.mode` + `tls.issuer` | cert-manager `ClusterIssuer letsencrypt-prod` | every Ingress `tls`, every `cert-manager.io/*` annotation, the identity keystore |
| `ingress.className` + `ingress.annotations` | `nginx` | every Ingress of the chart and of 6 subcharts |
| `dns.mode` | external-dns annotations, or manual | Ingress annotations |
| `storage.className` | `gp3` | PostgreSQL, MongoDB, Vault, APISIX etcd |
| `resources.profile` | `production` | `resources` of Scorpio, the TMForum API, the TIL, contract-management, Keycloak — the [sizing guidance](../deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components) applied, not re-explained. Shipped ([ADR 0024](../adr/0024-config-tool-resource-profile.md)): defaults to the Stage 1 environment (`development` or `production`); `custom` writes no key and leaves the chart's defaults |
| `images.pullSecrets`, `images.registryMirror` | | `imagePullSecrets` of every component, `keycloak.tokenStatusList.plugin.mavenBaseUrl` |
| `proxy.https`, `proxy.http`, `proxy.no` | `http://proxy.infra.svc.cluster.local:3128` | shipped ([ADR 0025](../adr/0025-config-tool-proxy-and-private-ca.md)): **8, 5 and 8 targets** in 5 components: the verifier, BAE logic proxy and vc-operator `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY` env elements, Keycloak `proxy.*`, and APISIX `proxy_opts` in each existing openid-connect route. `proxy.https` switches the group; `proxy.http` follows it; `proxy.no` derives from the release and namespace and lists every in-cluster form the five clients read |
| `ca.privateSecret` | `corp-ca` (key `ca.crt`) | shipped: **22 targets** in 6 components, one PEM Secret for all: the verifier CA init container, volumes and mount, APISIX `ssl.enabled`/`existingCASecret`/`certCAFilename` plus `ssl_verify: true` on each route, Keycloak `KC_TRUSTSTORE_PATHS` with its volume and mount, BAE `NODE_EXTRA_CA_CERTS`, vc-operator `SSL_CERT_DIR`, Contract Management `JDK_JAVA_OPTIONS` with an init container that imports `ca.crt` into a copy of the JDK trust store |

`provider.yaml` names the DID on 15 lines, one a comment. The DID keys that are not `did.value` targets (US-3.2, [ADR 0021](../adr/0021-config-tool-l2-rules.md#7-the-did-targets-narrow-the-story)):

- the TIL issuer list `registration.issuer[0].did`: US-5.5, the local TIL entries, which also name other participants;
- the verifier's TIR registration script (`initContainers[*].args[*]`): US-3.3 leaves it out, because it is a
  k3s-local registration at the in-cluster TIR with no chart key ([ADR 0022](../adr/0022-config-tool-data-space-parameters.md#1-only-the-tir-address-renders));
- `dataSpaceConfig` and APISIX: none, chart 10.16.0 has no DID key there;
- `keycloak.signingKey.did`: US-5.5, the keystore block; emitted alone it breaks the realm key provider;
- `identityhub.bootstrap.did` and `fdsc-edc.*.config.oid4vp.holder.kid`: US-8.11, because the chart's
  `assertHolderKid` ties the kid to the bootstrap DID and its `keyAlias`;
- the realm's clients, users, client roles and credential mappers, and the env lists of the ODRL-PAP, the BAE proxy and
  the dashboard's providers: US-5.3, US-5.5 and US-5.6.

Each one renders through a declared function, so a change at stage 3 re-renders every
dependent key and the review diff shows all of them. A key the user pinned by hand (an
override) is **not** re-rendered; it is listed as *pinned, not updated* so the user can decide.

## Integration modes

Every dependency the chart shares with the cluster carries a mode instead of a boolean.

| Dependency | `bundled` — installed with the connector | `cluster` — already in the cluster | `external` — outside Kubernetes |
|---|---|---|---|
| **PostgreSQL** | `…managedPostgres.enabled: true` + `postgres-operator.enabled: true` | operator off, `managedPostgres` on — the operator is someone else's release | both off; host, port, database, user and existing Secret per component (Keycloak, TIL, verifier, PAP, Scorpio, IdentityHub, status list) |
| **MongoDB** | `mongo-operator.enabled: true` + `managedMongo.enabled: true` | operator off, `managedMongo` on | both off; connection string + Secret for the BAE |
| **cert-manager** | `cert-manager.enabled: true` + `certManagerResources` issuers | subchart off, issuer name referenced | both off; the name of a TLS Secret per host |
| **Ingress / DNS** | — | ingress class + annotations; external-dns annotations if present | hostnames point somewhere the user manages; the tool only emits the hosts |
| **Vault** | `vault.enabled: true` + unsealer | address of the existing in-cluster Vault | external address, auth method, Secret |
| **Observability** | OTel collector (+ Tempo, Grafana) | endpoint of an existing collector | an external OTLP endpoint and its headers Secret |
| **Keycloak** | deployed by the chart | — | an existing issuer: its URL, realm and the credential endpoints |

**Secret delivery** is the sixth dimension of the same idea and has five modes rather than
three — `chart`, `reference`, `generate+apply`, `sealed` and `external`. It is chosen on the
same screen and overridable per Secret; [Secret handling](SECRETS.md#delivery-modes) carries
the table.

Rules the tool enforces:

- An **operator is cluster-wide**: if the tool detects one, or the user says another release
  of the connector or a Trust Anchor runs in this cluster, `bundled` is refused with the
  reason and `cluster` is selected ([PREREQUISITES](../deployment-integration/roles/PREREQUISITES.md#installing-the-operators-once)).
- `external` **never stores a credential**. It asks for the name of a Secret, and lists that
  Secret in `NEXT_STEPS.md` with the keys it must contain.
- `bundled` is marked *suitable for development* for the databases and Vault, with a link to
  the production guidance.

When a dependency is missing and the user picks `cluster`, the tool shows the upstream
install command and offers to run it (iteration 2) — it never silently installs.

cert-manager's `cluster` mode references a ClusterIssuer by name; a namespaced `Issuer` is not
offered. How the modes compile into chart keys is recorded in
[ADR 0026](../adr/0026-config-tool-infrastructure-modes.md).

## Round-trip and provenance

Loading an arbitrary `values.yaml` and writing it back without damage is the single property
the tool is trusted or distrusted for.
[ADR 0014](../adr/0014-config-tool-load-classification.md) fixes the marker's scope and the
precedence *owned* > *recognized* > *unknown* that loading applies.

1. **Parse** into a comment-preserving YAML document.
2. **Classify** every leaf: *owned* (a key some enabled feature's render function produces),
   *recognized* (a known toggle or cross-cutting value — its answer is back-derived),
   *unknown* (kept in `overrides` verbatim, with its comments and position).
3. **Render** on save: owned keys from the answer set, overrides merged on top, unknown keys
   written back in their original place.
4. **Mark** owned blocks so a later load can tell them apart:

   ```yaml
   # Generated by dsc-config 0.4.0 for chart 10.16.0 from .dscprofile.yaml — profile hash 9f2c1a
   # Keys under a "dsc-config:owned" marker are re-rendered on every save.

   decentralizedIam:
     vcAuthentication:
       vcverifier:
         # dsc-config:owned domain.base,did.value
         did: did:web:mp-operations.org
         host: https://verifier.mp-operations.org
   ```

5. **Detect conflicts**: an owned key whose value no longer matches what the profile renders
   was hand-edited between sessions. The tool does not overwrite it; it shows both values and
   asks, per key, *keep mine* or *take the file's* (which converts it into an override).

A file with no markers — someone else's `values.yaml` — loads as *recognized + unknown*: the
tool understands the toggles and the cross-cutting values, and carries the rest untouched.

## Validation rules

Four layers, each a separate pass so a failure says which one failed.

**L1 — schema.** [`values.schema.json`](../../charts/data-space-connector/values.schema.json)
(111 KiB, generated from `values.yaml`) as-is. Types, unknown keys, required fields. The chart
already refuses a key that is not in `values.yaml`, so this is the same gate CI applies.

**L2 — cross-field rules**, the tool's own. The current ones, each from a documented failure:

| Rule | Why |
|---|---|
| `access.verification` on ⇒ `dataspace.tirAddress` set (shipped: `tir-address-for-verification`, an error) | The verifier cannot check issuers without it |
| Public hostnames must differ per component, and resolve inside the cluster | [Hairpin DNS](../deployment-integration/roles/PREREQUISITES.md#public-names-have-to-resolve-inside-the-cluster) |
| Only one release per cluster may own `postgres-operator`, `mongo-operator` or `cert-manager` | Cluster-wide resources belong to one release |
| `identity.issuer`: the keystore path must be under `/opt/keycloak/data/<realm>/` | KC ≥ 26.6.4 reads realm keystores only from there |
| `market.local` on ⇒ MongoDB mode ≠ *none* | The BAE has no other store |
| `data.scorpio` with an existing PostgreSQL ⇒ PostGIS required | Scorpio's queries need it |
| `dsp.edc` on ⇒ a secret store chosen, and `identity.did` replaced by the IdentityHub | DCP serves the DID document from the IdentityHub |
| `environment: production` ⇒ `did:web`, a non-self-signed issuer, memory limits on the Java components | [Sizing](../deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components) |
| A private CA ⇒ every component that calls a public host must trust it (shipped: `ca-coverage`) | The per-component table in PREREQUISITES |
| Every `external` dependency's Secret appears in `NEXT_STEPS.md` | The install fails without it |
| `generatePasswords: true` with a `helm template` or GitOps target | `lookup` is unavailable there, so the value is re-randomised on every render |
| A referenced Secret is missing, or lacks a key the component reads | The component crash-loops on start |
| `sealed` without a Sealed Secrets controller or a certificate | Nothing could decrypt it |
| The same Secret claimed by both the chart and the tool | Two owners, one name |

The L2 rules live in the bundle's `rules:` list (`model/<version>/bundle.yaml`), not in a separate
`rules.yaml`: each is a warning or an error on one open field, raised while all of its `field equals value`
conditions hold over the resolved answers. Only errors block `generate`
([ADR 0021](../adr/0021-config-tool-l2-rules.md)).

The secret-specific rules are listed in full in
[Secret handling](SECRETS.md#validation).

Errors block generation; warnings do not. Every message names the stage and field it came
from and links back to it.

**L3 — render.** `helm template` through the Helm SDK with the generated values, against the
pinned chart version. Catches what no rule anticipated.

**L4 — cluster preflight** (iteration 2). Storage class exists, ingress class exists, CRDs
present, hostnames resolve from inside the cluster, the referenced Secrets exist, enough
allocatable memory for the role's [floor](../deployment-integration/roles/PREREQUISITES.md#sizing).

## Where the model lives

The catalogue, the presets, the cross-cutting fan-out and the L2 rules are **data, not code**:
one bundle per chart version, embedded in the binary and loadable from a file for development.

```
config-tool/model/
  10.16.0/
    features.yaml        catalogue, dependencies, keys, questions
    presets.yaml         the role matrix
    foundations.yaml     cross-cutting values and their render functions
    infrastructure.yaml  the dependencies and their three modes
    secrets.yaml         the Secrets per feature, their keys and the chart switch that owns them
    rules.yaml           the L2 rules (shipped as bundle.yaml rules:, ADR 0021)
    migrations.yaml      how answers move from 10.x to 10.16.0
```

That is what makes the tool survive chart releases: a new chart version ships a new bundle,
and [US-9.3](USER_STORIES.md#epic-9--changing-a-configuration-later) migrates existing
profiles into it.

The bundle sits in this repository next to `values.yaml`, and so does the tool that reads it
([Where the code lives](TECH_STACK.md#where-the-code-lives)). That is what keeps it honest: a
chart change and the question that configures it go in one pull request, and a CI check in the
same checkout asserts that every key the bundle names exists in the chart's schema and that the
role presets still match the component matrix in
[roles/README.md](../deployment-integration/roles/README.md).
