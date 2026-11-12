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

A **feature** is a capability a user wants, not a component. One feature may enable several
subcharts (`market.local` turns on the BAE, MongoDB and the TMForum API); one subchart may
serve several features (`keycloak` serves issuance, and the wallet preset, and eIDAS). The
feature is the unit of *enable/disable*, of *recommendation*, and of *one configuration page*.

A feature declares:

| Field | Meaning |
|---|---|
| `id`, `title`, `summary` | `dsp.edc`, "Dataspace Protocol (DSP)", one line of what it gives you |
| `group` | Identity · Access · Data · Contracts · Marketplace · Interop · Consent · Observability |
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

### Identity

| Feature | Gives you | Chart keys | Needs |
|---|---|---|---|
| `identity.issuer` | Issue Verifiable Credentials to your users (Keycloak, OID4VCI) | `keycloak.*`, `vcCredentials` | postgres, certs |
| `identity.did` | Publish the organization's DID document (`did:web`/`did:key`) | `did.*` | certs |
| `identity.statuslist` | Revoke issued credentials (token status list) | `statusListServer.*` | postgres |
| `identity.vcoperator` | Manage credential definitions as Kubernetes resources | `vc-operator.*`, `vcCredentials.*` | — |
| `identity.wallets` | Preset clients for the Lissi and EUDI wallets | `keycloak.realm.wallets.*` | `identity.issuer` |
| `identity.eidas` | eIDAS/ELSI certificate-based identity and validation | `elsi.*`, `decentralizedIam.vcAuthentication.dss.*` | `identity.issuer` |

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

`R` recommended on, `o` available off, `–` not offered.

| Feature | Consumer | Provider | Consumer + Provider | Operator |
|---|---|---|---|---|
| `identity.issuer` | **R** | **R** | **R** | – |
| `identity.did` | **R** | **R** | **R** | – |
| `identity.statuslist` | o | o | o | – |
| `identity.vcoperator` | o | o | o | – |
| `identity.wallets` | o | o | o | – |
| `identity.eidas` | o | o | o | – |
| `access.verification` | – | **R** | **R** | – |
| `access.policies` | – | **R** | **R** | – |
| `data.scorpio` | – | **R** | **R** | – |
| `data.external` | – | o | o | – |
| `contract.tmforum` | – | **R** | **R** | – |
| `contract.management` | – | **R** | **R** | – |
| `market.local` | – | o | o | – |
| `market.central` | – | o | o | – |
| `dsp.edc` | o | o | o | – |
| `dsp.rainbow` | o | o | o | – |
| `dsp.config` | o | o | o | o |
| `consent.provider` | – | o | o | – |
| `consent.authority` | – | – | – | o |
| `ta.registry` | – | – | – | **R** |
| `ta.onboarding` | – | – | – | o |
| `obs.*` | o | o | o | o |

The preset is derived from the component matrix in
[roles/README.md](../deployment-integration/roles/README.md); that table and this one must be
changed together, and a test in the tool's repository asserts they agree.

## Cross-cutting values

Asked in [Stage 3](USER_JOURNEY.md#stage-3--foundations). The "fans out to" count is measured
against [`k3s/provider.yaml`](../../k3s/provider.yaml), which is what a fully configured
provider looks like today.

| Value | Example | Fans out to |
|---|---|---|
| `org.displayName` | `MP Operations` | Keycloak realm display name, BAE organization, chart labels |
| `domain.base` | `mp-operations.org` | **41 lines** of the provider profile: every ingress host, every TLS secret name, every cert-manager annotation, the DID, the verifier's `host`, the APISIX routes' `discovery` URLs, the dsconfig document |
| `host.<component>` | `verifier.mp-operations.org` | derived from `domain.base`, individually overridable; **15 keys** for the verifier alone |
| `did.method` / `did.value` | `did:web:mp-operations.org` | **15 lines**: verifier `did`, Keycloak `issuerDid`, the TIR registration job, fdsc-edc, IdentityHub, the dsconfig document, APISIX, contract-management |
| `dataspace.tirAddress` | `https://tir.dataspace.org` | verifier `tirAddress`, registration job, trust framework checks |
| `dataspace.credentialTypes` | `MembershipCredential`, `UserCredential` | Keycloak client scopes and protocol mappers, verifier service registration, local TIL entries, ODRL policies |
| `tls.mode` + `tls.issuer` | cert-manager `ClusterIssuer letsencrypt-prod` | every Ingress `tls`, every `cert-manager.io/*` annotation, the identity keystore |
| `ingress.className` + `ingress.annotations` | `nginx` | every Ingress of the chart and of 6 subcharts |
| `dns.mode` | external-dns annotations, or manual | Ingress annotations |
| `storage.className` | `gp3` | PostgreSQL, MongoDB, Vault, APISIX etcd |
| `resources.profile` | `production` | `resources` of Scorpio, the TMForum API, the TIL, contract-management, Keycloak — the [sizing guidance](../deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components) applied, not re-explained |
| `images.pullSecrets`, `images.registryMirror` | | `imagePullSecrets` of every component, `keycloak.tokenStatusList.plugin.mavenBaseUrl` |
| `proxy.https`, `proxy.no` | | verifier env, Keycloak `proxy.*`, BAE, vc-operator, APISIX `proxy_opts` |
| `ca.privateSecret` | | APISIX `ssl.existingCASecret`, the verifier CA init container, `NODE_EXTRA_CA_CERTS`, Keycloak truststore |

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

## Round-trip and provenance

Loading an arbitrary `values.yaml` and writing it back without damage is the single property
the tool is trusted or distrusted for.

1. **Parse** into a comment-preserving YAML document.
2. **Classify** every leaf: *owned* (a key some enabled feature's render function produces),
   *recognized* (a known toggle or cross-cutting value — its answer is back-derived),
   *unknown* (kept in `overrides` verbatim, with its comments and position).
3. **Render** on save: owned keys from the answer set, overrides merged on top, unknown keys
   written back in their original place.
4. **Mark** owned blocks so a later load can tell them apart:

   ```yaml
   # Generated by dsc-config 0.4.0 from .dscprofile.yaml — profile hash 9f2c1a
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
| `access.verification` on ⇒ `dataspace.tirAddress` set | The verifier cannot check issuers without it |
| Public hostnames must differ per component, and resolve inside the cluster | [Hairpin DNS](../deployment-integration/roles/PREREQUISITES.md#public-names-have-to-resolve-inside-the-cluster) |
| Only one release per cluster may own `postgres-operator`, `mongo-operator` or `cert-manager` | Cluster-wide resources belong to one release |
| `identity.issuer`: the keystore path must be under `/opt/keycloak/data/<realm>/` | KC ≥ 26.6.4 reads realm keystores only from there |
| `market.local` on ⇒ MongoDB mode ≠ *none* | The BAE has no other store |
| `data.scorpio` with an existing PostgreSQL ⇒ PostGIS required | Scorpio's queries need it |
| `dsp.edc` on ⇒ a secret store chosen, and `identity.did` replaced by the IdentityHub | DCP serves the DID document from the IdentityHub |
| `environment: production` ⇒ `did:web`, a non-self-signed issuer, memory limits on the Java components | [Sizing](../deployment-integration/roles/PREREQUISITES.md#set-memory-limits-on-the-java-components) |
| A private CA ⇒ every component that calls a public host must trust it | The per-component table in PREREQUISITES |
| Every `external` dependency's Secret appears in `NEXT_STEPS.md` | The install fails without it |
| `generatePasswords: true` with a `helm template` or GitOps target | `lookup` is unavailable there, so the value is re-randomised on every render |
| A referenced Secret is missing, or lacks a key the component reads | The component crash-loops on start |
| `sealed` without a Sealed Secrets controller or a certificate | Nothing could decrypt it |
| The same Secret claimed by both the chart and the tool | Two owners, one name |

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
    rules.yaml           the L2 rules
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
