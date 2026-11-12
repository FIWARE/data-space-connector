# Mockups

Wireframes of every screen of the [user journey](USER_JOURNEY.md). They fix layout, wording
and the information architecture — not visual design. A clickable version of the same screens
is in [mockups/prototype.html](mockups/prototype.html); open it in any browser.

<!-- ToC -->
- [Window chrome](#window-chrome)
- [S0 — Start](#s0--start)
- [S1 — Target](#s1--target)
- [S2 — Role](#s2--role)
- [S3 — Foundations](#s3--foundations)
- [S4 — Infrastructure](#s4--infrastructure)
- [S5 — Features](#s5--features)
- [S6a — Feature configuration: Dataspace Protocol](#s6a--feature-configuration-dataspace-protocol)
- [S6b — Feature configuration: credential issuance](#s6b--feature-configuration-credential-issuance)
- [S7 — Review and generate](#s7--review-and-generate)
- [S7b — Secrets](#s7b--secrets)
- [S8 — Install](#s8--install-iteration-2)
- [S9 — Enabling a feature later](#s9--enabling-a-feature-later)
- [Recurring elements](#recurring-elements)
<!-- /ToC -->

## Window chrome

Every screen after S0 shares it: a stage rail on the left (jumpable, with per-stage status), a
content pane, and a footer carrying validation state and navigation.

```
┌────────────────────────────────────────────────────────────────────────────────────────────────┐
│  DSC Configurator                        mp-operations · provider · chart 10.16.0    ─  □  ✕    │
├──────────────────────┬─────────────────────────────────────────────────────────────────────────┤
│                      │                                                                         │
│  ✓ 1 Target          │   «content pane»                                                        │
│  ✓ 2 Role            │                                                                         │
│  ✓ 3 Foundations     │                                                                         │
│  ● 4 Infrastructure  │                                                                         │
│    5 Features        │                                                                         │
│    6 Configuration   │                                                                         │
│    7 Review          │                                                                         │
│    8 Install         │                                                                         │
│                      │                                                                         │
│  ──────────────────  │                                                                         │
│  ⌘E  Expert: YAML    │                                                                         │
│  ⌘S  Save profile    │                                                                         │
│                      │                                                                         │
├──────────────────────┴─────────────────────────────────────────────────────────────────────────┤
│  ⚠ 2 warnings   ✖ 0 errors   ○ 3 open questions          [ Back ]          [ Next: Features ]  │
└────────────────────────────────────────────────────────────────────────────────────────────────┘

  ✓ complete   ● current   ○ has open questions   ⚠ has warnings   ✖ has errors
```

## S0 — Start

```
┌────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                                                                     ─  □  ✕    │
│                                                                                                │
│                         FIWARE Data Space Connector — Configurator                             │
│                      Configure and install a connector for your organization                   │
│                                                                                                │
│      ┌──────────────────────────────┐   ┌──────────────────────────────┐                       │
│      │  ＋                          │   │  📂                          │                       │
│      │  New configuration           │   │  Open values.yaml            │                       │
│      │                              │   │                              │                       │
│      │  Start from a role and go    │   │  Read an existing file,      │                       │
│      │  through the wizard.         │   │  change it, keep everything  │                       │
│      │                              │   │  it already contains.        │                       │
│      └──────────────────────────────┘   └──────────────────────────────┘                       │
│      ┌──────────────────────────────┐   ┌──────────────────────────────┐                       │
│      │  ⟲                           │   │  ⚡                          │                       │
│      │  Open profile                │   │  Try it locally              │                       │
│      │                              │   │                              │                       │
│      │  Continue a configuration    │   │  A whole data space on this   │                      │
│      │  saved earlier.              │   │  machine: trust anchor,      │                       │
│      │                              │   │  provider and consumer.      │                       │
│      └──────────────────────────────┘   └──────────────────────────────┘                       │
│                                                                                                │
│      Recent                                                                                    │
│        mp-operations · provider · chart 10.16.0 · 2 days ago                                    │
│        dataspace-authority · operator · chart 10.16.0 · last week                               │
│                                                                                                │
│                                                       Documentation ↗   Release notes ↗        │
└────────────────────────────────────────────────────────────────────────────────────────────────┘
```

## S1 — Target

```
│  Where is this going?                                                                          │
│                                                                                                │
│  Target                                                                                        │
│    ( ) File only — generate values.yaml, install it yourself                                   │
│    (•) Kubernetes cluster                                                                      │
│        Kubeconfig  [ ~/.kube/config                                    ] [ Browse… ]           │
│        Context     [ prod-eu-central                                 ▾ ]                       │
│        ✓ Connected · Kubernetes v1.31.4 · 6 nodes · 184 GiB allocatable                        │
│          The cluster is only read, to pre-fill the next screens. Nothing is installed          │
│          before you ask for it on step 8.                                                      │
│                                                                                                │
│  Release                                                                                       │
│    Namespace       [ provider                                          ]  ( ) create it        │
│    Release name    [ provider                                          ]                       │
│      ⓘ Both appear in generated service URLs and in the Trust Anchor registration.             │
│                                                                                                │
│  Chart                                                                                         │
│    Chart           [ data-space-connector                            ▾ ]                       │
│    Version         [ 10.16.0 (latest)                                 ▾ ]  Release notes ↗      │
│                                                                                                │
│  Environment                                                                                   │
│    ( ) Development — self-signed certificates, did:key, chart-deployed databases, no limits    │
│    (•) Production  — ACME or your CA, did:web, resource limits, replicas                       │
│      ⓘ Only changes defaults. Every one of them stays editable.                                │
```

## S2 — Role

```
│  What does this organization do in the data space?                                             │
│                                                                                                │
│  ┌───────────────────────┐ ┌───────────────────────┐ ┌───────────────────────┐                 │
│  │  Consumer             │ │  Provider             │ │  Consumer + Provider ✓│                 │
│  │                       │ │                       │ │                       │                 │
│  │  Uses data and        │ │  Offers data and      │ │  Both. The common     │                 │
│  │  services offered by  │ │  services to others.  │ │  case in a real data  │                 │
│  │  others.              │ │                       │ │  space.               │                 │
│  │                       │ │                       │ │                       │                 │
│  │  2 features · ~0.8GiB │ │  7 features · ~2.3GiB │ │  7 features · ~2.3GiB │                 │
│  └───────────────────────┘ └───────────────────────┘ └───────────────────────┘                 │
│  ┌───────────────────────┐ ┌───────────────────────┐                                           │
│  │  Operator             │ │  Custom               │                                           │
│  │                       │ │                       │                                           │
│  │  Runs the trust       │ │  Nothing preselected. │                                           │
│  │  infrastructure of    │ │  Pick from the whole  │                                           │
│  │  the data space.      │ │  catalogue yourself.  │                                           │
│  │  chart: trust-anchor  │ │                       │                                           │
│  │  1 feature · ~0.3GiB  │ │                       │                                           │
│  └───────────────────────┘ └───────────────────────┘                                           │
│                                                                                                │
│  Consumer + Provider turns on                                   ▾ what this means              │
│    Credential issuance · DID document · Credential verification · Policy enforcement ·         │
│    NGSI-LD data service · TMForum contract APIs · Contract management                          │
│    Everything else stays available on step 5 — nothing here is locked.                         │
│                                                                                                │
│    Memory and CPU figures are the measured floor per role, without load. Sizing ↗              │
```

## S3 — Foundations

The panel that pays for the tool. Note the *used by* counters and the derived-host table.

```
│  Foundations — asked once, used everywhere                                                     │
│                                                                                                │
│  ╭─ Organization ──────────────────────────────────────────────────────────────────────────╮   │
│  │  Display name    [ MP Operations                      ]                                 │   │
│  │  Base domain     [ mp-operations.org                  ]   used by 41 values ⓘ           │   │
│  │  Identifier      [ DE123456789                        ]   VAT / LEI, optional           │   │
│  │                                                                                         │   │
│  │  DID method      (•) did:web   ( ) did:key                                              │   │
│  │  DID             did:web:mp-operations.org                used by 15 values ⓘ           │   │
│  │                  derived from the base domain            [ override ]                   │   │
│  ╰─────────────────────────────────────────────────────────────────────────────────────────╯   │
│                                                                                                │
│  ╭─ Data space ────────────────────────────────────────────────────────────────────────────╮   │
│  │  Import from    [ https://dataspace.org/.well-known/data-space-configuration ] [Import] │   │
│  │                                                                                         │   │
│  │  Trust Anchor (TIR)  [ https://tir.dataspace.org                 ]  ✓ reachable         │   │
│  │  Credential types    ┌────────────────────────┬─────────────────────────────────────┐   │   │
│  │                      │ MembershipCredential   │ issued to the organization          │   │   │
│  │                      │ UserCredential         │ issued to employees                 │   │   │
│  │                      │ + add                  │                                     │   │   │
│  │                      └────────────────────────┴─────────────────────────────────────┘   │   │
│  │  [x] Register this participant at the Trust Anchor during installation                  │   │
│  ╰─────────────────────────────────────────────────────────────────────────────────────────╯   │
│                                                                                                │
│  ╭─ Network ───────────────────────────────────────────────────────────────────────────────╮   │
│  │  Host scheme    (•) <component>.mp-operations.org   ( ) one host with path prefixes     │   │
│  │                                                                                         │   │
│  │     DID document      mp-operations.org                     [ edit ]                    │   │
│  │     Verifier          verifier.mp-operations.org            [ edit ]                    │   │
│  │     Gateway (APISIX)  api.mp-operations.org                 [ edit ]                    │   │
│  │     Keycloak          keycloak.mp-operations.org            [ edit ]                    │   │
│  │     TMForum API       tmf.mp-operations.org                 [ edit ]                    │   │
│  │     + 3 more once their features are enabled                                            │   │
│  │                                                                                         │   │
│  │  ⚠ These names are called from inside the cluster as well as from outside.              │   │
│  │    Check that they resolve to the ingress from a pod. Why, and how ↗                    │   │
│  │                                                                                         │   │
│  │  Ingress class  [ nginx                ▾ ]  detected in the cluster                     │   │
│  │  DNS records    ( ) external-dns handles them   (•) I create them myself                │   │
│  │  TLS            (•) cert-manager issuer [ letsencrypt-prod  ▾ ]                         │   │
│  │                 ( ) I provide a Secret per host    ( ) no TLS (development only)        │   │
│  │  Private CA     [ none                 ▾ ]                                              │   │
│  │  Outbound proxy [ none                 ▾ ]                                              │   │
│  ╰─────────────────────────────────────────────────────────────────────────────────────────╯   │
│                                                                                                │
│  ╭─ Platform ──────────────────────────────────────────────────────────────────────────────╮   │
│  │  Storage class    [ gp3                ▾ ] default in this cluster                      │   │
│  │  Resource profile [ production         ▾ ] sets requests and limits on the Java parts ⓘ │   │
│  │  Image pull secrets [ none             ▾ ]   Registry mirror [                      ]   │   │
│  ╰─────────────────────────────────────────────────────────────────────────────────────────╯   │
```

The `used by N values ⓘ` popover lists exactly where a value lands, which is how the user
learns that changing it later is one edit:

```
      ┌──────────────────────────────────────────────────────────────────────┐
      │  Base domain is used by 41 values                                    │
      │                                                                      │
      │    decentralizedIam.vcAuthentication.vcverifier.deployment.host      │
      │    decentralizedIam.vcAuthentication.vcverifier.ingress.hosts[0]     │
      │    decentralizedIam.odrlAuthorization.apisix.routes[*].discovery     │
      │    keycloak.ingress.hostname                                         │
      │    did.ingress.hosts[0]                                              │
      │    … 36 more                                   [ show all ]          │
      │                                                                      │
      │  Change it later and all 41 are re-rendered.                         │
      └──────────────────────────────────────────────────────────────────────┘
```

## S4 — Infrastructure

Detection first, choice second.

```
│  Infrastructure — what is already there, what should be installed                              │
│                                                                                                │
│  Found in prod-eu-central:  cert-manager v1.21 · ingress-nginx · storage classes gp3, gp2      │
│  Not found:                 Zalando Postgres Operator · MongoDB operator · OpenTelemetry       │
│                                                                                 [ scan again ] │
│                                                                                                │
│  ╭─ PostgreSQL ───────────────────────────────────── required by 5 components ────────────╮    │
│  │  ( ) Install with the connector                      development · chart-managed       │    │
│  │  ( ) Already in the cluster                          no operator found                 │    │
│  │  (•) An existing database                                                              │    │
│  │                                                                                        │    │
│  │      Host      [ pg.internal.mp-operations.org    ]  Port [ 5432 ]                      │    │
│  │      [x] one database per component   ( ) one shared database                          │    │
│  │                                                                                        │    │
│  │        component            database          user          secret                     │    │
│  │        Keycloak             [ keycloak    ]  [ kc      ]  [ pg-keycloak      ▾ ]        │    │
│  │        Trusted Issuers List [ til         ]  [ til     ]  [ pg-til           ▾ ]        │    │
│  │        VCVerifier           [ verifier    ]  [ verifier]  [ pg-verifier      ▾ ]        │    │
│  │        ODRL-PAP             [ pap         ]  [ pap     ]  [ pg-pap           ▾ ]        │    │
│  │        Scorpio              [ ngb         ]  [ scorpio ]  [ pg-scorpio       ▾ ]        │    │
│  │                                                                                        │    │
│  │      ⚠ Scorpio's database needs the PostGIS extension.                                 │    │
│  │      ⓘ The Secrets are not created by the tool. They will be listed in NEXT_STEPS.md.  │    │
│  ╰────────────────────────────────────────────────────────────────────────────────────────╯    │
│                                                                                                │
│  ╭─ TLS certificates ─────────────────────────────── required by 6 hosts ──────────────────╮   │
│  │  ( ) Install cert-manager with the connector                                            │   │
│  │  (•) Use the cert-manager already in the cluster      ✓ found in namespace cert-manager │   │
│  │        ClusterIssuer  [ letsencrypt-prod   ▾ ]  letsencrypt-staging, selfsigned-issuer  │   │
│  │  ( ) I provide a TLS Secret per host                                                    │   │
│  │                                                                                         │   │
│  │  ⓘ A cluster can only have one cert-manager, so it is not installed again here.         │   │
│  ╰─────────────────────────────────────────────────────────────────────────────────────────╯   │
│                                                                                                │
│  ╭─ MongoDB ─────────────────────── not needed: the local marketplace is off ──── collapsed ╮   │
│  ╰─────────────────────────────────────────────────────────────────────────────────────────╯   │
│                                                                                                │
│  ╭─ Secret store ──────────────────────────────────── required by Dataspace Protocol ──────╮   │
│  │  ( ) HashiCorp Vault, installed with the connector     development                      │   │
│  │  (•) An existing Vault     Address [ https://vault.mp-operations.org ]                  │   │
│  │                            Auth    [ kubernetes            ▾ ]  Role [ dsc        ]     │   │
│  │  ( ) Kubernetes Secrets only                           not recommended in production ↗  │   │
│  ╰─────────────────────────────────────────────────────────────────────────────────────────╯   │
│                                                                                                │
│  ╭─ Observability ─────────────────────────────────────────────────────── optional ────────╮   │
│  │  ( ) Deploy collector, Tempo and Grafana   (•) Send to an existing collector   ( ) off  │   │
│  │        OTLP endpoint [ http://otel-collector.observability:4318 ]                       │   │
│  ╰─────────────────────────────────────────────────────────────────────────────────────────╯   │
```

When nothing is found and the user chooses *already in the cluster*, the panel turns into a
recommendation instead of an error:

```
      ╭─ PostgreSQL ──────────────────────────────────────────────────────────────────────╮
      │  (•) Already in the cluster                                                       │
      │                                                                                   │
      │  ⚠ No Zalando Postgres Operator found in prod-eu-central.                         │
      │                                                                                   │
      │    The charts create `postgresql` resources that the operator turns into          │
      │    databases. Install it once per cluster, outside this release:                  │
      │                                                                                   │
      │    ┌──────────────────────────────────────────────────────────────────────────┐   │
      │    │ helm repo add postgres-operator-charts \                                  │   │
      │    │   https://opensource.zalando.com/postgres-operator/charts/postgres-…      │   │
      │    │ helm install postgres-operator postgres-operator-charts/postgres-operator\│   │
      │    │   --version 1.15.1 --namespace postgres-operator --create-namespace       │   │
      │    └──────────────────────────────────────────────────────────────────────────┘   │
      │                                  [ copy ]  [ run it for me ] (step 8)             │
      │                                                                                   │
      │    Or let this release install it — only correct if this is the cluster's only    │
      │    connector:                                     [ switch to "install with" ]    │
      ╰───────────────────────────────────────────────────────────────────────────────────╯
```

## S5 — Features

```
│  Features                                        [ Search…            ]  [ recommended ▾ ]     │
│                                                                                                │
│  Identity                                                                                      │
│    [■]  Credential issuance                           required for Consumer + Provider    ⚙ 4  │
│         Issue Verifiable Credentials to your users with Keycloak (OID4VCI).                    │
│    [■]  DID document                                  required                             ⚙   │
│         Publish did:web:mp-operations.org so others can verify your signatures.                │
│    [ ]  Credential revocation                         optional                                 │
│         A token status list so issued credentials can be revoked. Needs PostgreSQL.            │
│    [ ]  Wallet presets (Lissi, EUDI)                  optional                                 │
│    [ ]  eIDAS / ELSI certificates                     optional                                 │
│                                                                                                │
│  Access                                                                                        │
│    [■]  Credential verification                       required for Provider                ⚙ 2 │
│         Check incoming credentials against the data space's trusted issuers, hand out tokens.  │
│    [■]  Policy enforcement                            required for Provider                ⚙ 3 │
│         APISIX as the gateway, OPA as the decision point, ODRL policies you define.            │
│                                                                                                │
│  Data                                                                                          │
│    [■]  NGSI-LD data service (Scorpio)                recommended                          ⚙   │
│    [ ]  Protect my own services                       optional                                 │
│                                                                                                │
│  Contracts and marketplace                                                                     │
│    [■]  TMForum contract APIs                         recommended                          ⚙   │
│    [■]  Contract management                           recommended                          ⚙   │
│    [ ]  A marketplace of my own (BAE)                 optional · needs MongoDB                 │
│    [■]  Publish to the central marketplace            optional                             ⚙ 1 │
│                                                                                                │
│  Interoperability                                                                              │
│    [■]  Dataspace Protocol (DSP / EDC)                optional                              ⚙ 3│
│         ├ needs a secret store — set to "an existing Vault" on step 4 ✓                        │
│         └ replaces the DID document component with the Identity Hub ⓘ                          │
│    [ ]  Rainbow (Rust DSP)                            optional                                 │
│    [ ]  Data space configuration endpoint             optional                                 │
│                                                                                                │
│  Consent                                                                                       │
│    [ ]  Consent enforcement                           optional · needs policy enforcement      │
│                                                                                                │
│  Observability                                                                                 │
│    [■]  Distributed tracing                           optional                             ⚙   │
│    [ ]  Metrics and dashboards                        optional                                 │
│                                                                                                │
│  ⚙ n = n questions still open on step 6                                                        │
```

Turning one off that another needs:

```
            ┌────────────────────────────────────────────────────────────────┐
            │  Turn off TMForum contract APIs?                               │
            │                                                                │
            │  Contract management and Publish to the central marketplace    │
            │  need it. Turning it off turns those off too.                  │
            │                                                                │
            │  Their configuration is kept, in case you turn them back on.   │
            │                                                                │
            │                        [ Cancel ]   [ Turn off all three ]     │
            └────────────────────────────────────────────────────────────────┘
```

## S6a — Feature configuration: Dataspace Protocol

Inherited fields carry the value and where it came from. This is how stage 3 stays visible.

```
│  6 · Configuration            Issuance ✓   DID ✓   Verification ✓   Policies ✓   ▸ DSP   Tracing│
│                                                                                                │
│  Dataspace Protocol (DSP / EDC)                            Guide ↗   Chart keys: fdsc-edc, …   │
│  Contract negotiation and data transfer with any DSP-compliant connector.                      │
│                                                                                                │
│    Participant ID       did:web:mp-operations.org       inherited from Organization [override] │
│    Public host          dsp.mp-operations.org           derived from the base domain [override]│
│    Secret store         existing Vault at vault.mp-operations.org   from step 4      [change]  │
│    Database             pg.internal.mp-operations.org / edc          from step 4     [change]  │
│                                                                                                │
│    Identity protocol    (•) DCP — Identity Hub serves the DID document and credentials         │
│                         ( ) OID4VP — keep the DID helper, use the verifier                     │
│                         ⓘ DCP replaces the DID document component. What changes ↗              │
│                                                                                                │
│    Credentials to hold  [x] MembershipCredential   [ ] DataProcessorCredential                 │
│                                                                                                │
│    Counterparties       ┌─────────────────────────────┬──────────────────────────────────┐     │
│                         │ did:web:consumer.example.org│ https://dsp.consumer.example.org │     │
│                         │ + add                       │                                  │     │
│                         └─────────────────────────────┴──────────────────────────────────┘     │
│                         Optional. Leave empty to discover them through the Trust Anchor.       │
│                                                                                                │
│  ▾ Advanced (37 chart keys)                                                                    │
│                                                                                                │
│                                                 [ ‹ Previous: Policies ]  [ Next: Tracing › ]  │
```

Expanded, Advanced is the schema-driven escape hatch — every remaining key of that feature,
typed and documented from `values.schema.json`:

```
│  ▴ Advanced (37 chart keys)                                   [ only changed ]  [ search…   ]  │
│      fdsc-edc.common.deployment.replicaCount          integer   [ 1        ]   default 1       │
│      fdsc-edc.common.deployment.resources             object    { … }          not set         │
│      fdsc-edc.common.config.transfer.proxy.endpoint   string    [          ]   default ""      │
│      identityhub.bootstrap.rotation.instance          string    [ 1        ]   default "1"     │
│      …                                                                                         │
│      Anything you set here is kept exactly as typed and never re-rendered.                     │
```

## S6b — Feature configuration: credential issuance

The heaviest page, built around the data space's credential types rather than around
Keycloak's object model.

```
│  Credential issuance (Keycloak)                            Guide ↗   Chart keys: keycloak.*    │
│                                                                                                │
│    Issuer host        keycloak.mp-operations.org        derived            [override]          │
│    Issuer DID         did:web:mp-operations.org         inherited          [override]          │
│    Realm name         [ mp-operations                 ]                                        │
│    Admin user         [ admin                         ]  password from Secret [ kc-admin  ▾ ]  │
│    Database           pg.internal… / keycloak           from step 4        [change]            │
│                                                                                                │
│    Signing key        (•) issued by cert-manager (letsencrypt-prod)                            │
│                       ( ) from an existing Secret  [                    ▾ ]                    │
│                       ⚠ The keystore must stay at /opt/keycloak/data/mp-operations/ —          │
│                         Keycloak 26.6.4 and later read realm keystores only from there. ↗      │
│                                                                                                │
│    Credentials this organization issues              from the data space, step 3               │
│    ┌─────────────────────────┬──────────┬───────────────────────────┬────────────────────┐     │
│    │ type                    │ format   │ claims                    │ issued to          │     │
│    ├─────────────────────────┼──────────┼───────────────────────────┼────────────────────┤     │
│    │ MembershipCredential    │ jwt_vc   │ did, legalName            │ the organization   │     │
│    │ UserCredential          │ jwt_vc   │ email, firstName, roles ⚙ │ group: employees   │     │
│    │ + add a credential type │          │                           │                    │     │
│    └─────────────────────────┴──────────┴───────────────────────────┴────────────────────┘     │
│                                                                                                │
│    Users and groups   [x] create the groups above                                              │
│                       [ ] create example users   — development only                            │
│                       [x] assign realm credentials to declared users                           │
│                                                                                                │
│    Wallets            [ ] Lissi    [ ] EUDI (needs credential response encryption) ⓘ           │
│                                                                                                │
│  ▾ Advanced (118 chart keys)                                                                   │
```

## S7 — Review and generate

```
│  Review                                                                                        │
│  ┌───────────────────────────────┬──────────────────────────────────────────────────────────┐  │
│  │ Summary                       │  values.yaml          [ generated ] [ diff ] [ as loaded]│  │
│  │                               │                                                          │  │
│  │ Role   Consumer + Provider    │   1  # Generated by dsc-config 0.4.0 — profile 9f2c1a    │  │
│  │ Chart  10.16.0                 │   2  # chart data-space-connector 10.16.0                 │  │
│  │ Namespace / release  provider │   3                                                      │  │
│  │                               │   4  decentralizedIam:                                   │  │
│  │ Features on            11     │   5    enabled: true                                     │  │
│  │ Features off           8      │   6    vcAuthentication:                                 │  │
│  │ Using chart defaults   3      │   7      managedPostgres:                                │  │
│  │                               │   8        enabled: false                                │  │
│  │ Infrastructure                │   9      postgres-operator:                              │  │
│  │   PostgreSQL   external       │  10        enabled: false                                │  │
│  │   TLS          cluster        │  11      trusted-issuers-list:                           │  │
│  │   MongoDB      off            │  12        database:                                     │  │
│  │   Vault        external       │  13          host: pg.internal.mp-operations.org         │  │
│  │   Tracing      external       │  14  +       port: 5432                                  │  │
│  │                               │  15          name: til                                   │  │
│  │ Secrets you must create  6    │  …                                                       │  │
│  │ DNS records you must add 6    │                                                          │  │
│  ├───────────────────────────────┤                                                          │  │
│  │ Validation                    │                                                          │  │
│  │                               │                                                          │  │
│  │ ✖ 0 errors                    │                                                          │  │
│  │ ⚠ 2 warnings                  │                                                          │  │
│  │   · Scorpio's database needs  │                                                          │  │
│  │     PostGIS — verify it is    │                                                          │  │
│  │     enabled on `ngb`.   go ↗  │                                                          │  │
│  │   · No memory limit on the    │                                                          │  │
│  │     TMForum API.        fix ↗ │                                                          │  │
│  │ ○ 1 open question             │                                                          │  │
│  │   · Central marketplace URL   │                                                          │  │
│  │                        go ↗   │                                                          │  │
│  │                               │                                                          │  │
│  │ ✓ schema      ✓ rules         │                                                          │  │
│  │ ✓ helm template               │                                                          │  │
│  │ ✓ cluster preflight           │                                                          │  │
│  └───────────────────────────────┴──────────────────────────────────────────────────────────┘  │
│                                                                                                │
│         [ Save profile ]   [ Export values.yaml ]   [ Export bundle ]   [ Install › ]          │
│                                                                                                │
│   Export bundle writes values.yaml + .dscprofile.yaml + NEXT_STEPS.md into one folder.         │
```

## S7b — Secrets

Reached from the Secrets pane on Review. One row per credential the configuration needs, grouped
by who can create it. Values are never shown in this table — only names, keys and state.

```
│  Secrets                                                            [ global mode: seal for Git ▾ ]│
│                                                                                                │
│  Sealed Secrets  ✓ controller sealed-secrets-controller in kube-system · certificate 41 days   │
│  External Secrets  ✓ ClusterSecretStore: vault-prod                        [ re-fetch cert ]   │
│                                                                                                │
│  Bootstrap credentials                              the chart can generate these itself        │
│  ┌──────────────────────────┬──────────────────┬───────────────────────┬────────────────────┐  │
│  │ secret / key             │ used by          │ delivery              │ state              │  │
│  ├──────────────────────────┼──────────────────┼───────────────────────┼────────────────────┤  │
│  │ issuance-secret          │ Keycloak admin   │ [ seal for Git    ▾ ] │ ● generated, sealed│  │
│  │   keycloak-admin         │                  │                       │   [ reveal once ]  │  │
│  │ mongodb-admin-password   │ marketplace      │ [ let the chart   ▾ ] │ ○ on first install │  │
│  │   password               │                  │                       │                    │  │
│  │ vc-operator signing key  │ vc-operator      │ [ let the chart   ▾ ] │ ○ on first install │  │
│  └──────────────────────────┴──────────────────┴───────────────────────┴────────────────────┘  │
│                                                                                                │
│  ⚠ The admin account is created from this Secret only while the realm has no administrator.    │
│    Changing the Secret later does not change the password.  What to do instead ↗               │
│                                                                                                │
│  External systems                                   only you can know these values             │
│  ┌──────────────────────────┬──────────────────┬───────────────────────┬────────────────────┐  │
│  │ pg-keycloak              │ Keycloak → DB    │ [ reference only  ▾ ] │ ✓ exists in cluster│  │
│  │   username, password     │                  │                       │                    │  │
│  │ pg-scorpio               │ Scorpio → DB     │ [ reference only  ▾ ] │ ✖ not found        │  │
│  │   username, password     │                  │                       │   [ enter values ] │  │
│  │ vault-approle            │ EDC → Vault      │ [ external secret ▾ ] │ ● ExternalSecret   │  │
│  │   roleId, secretId       │                  │   store: vault-prod   │                    │  │
│  └──────────────────────────┴──────────────────┴───────────────────────┴────────────────────┘  │
│                                                                                                │
│  Identity material                                  losing one means re-registering            │
│  ┌──────────────────────────┬──────────────────┬───────────────────────┬────────────────────┐  │
│  │ mp-operations.org-tls    │ DID, Keycloak    │ cert-manager          │ ✓ issued, 67 days  │  │
│  │ edc-identity             │ EDC, IdentityHub │ [ reference only  ▾ ] │ ✓ exists in cluster│  │
│  └──────────────────────────┴──────────────────┴───────────────────────┴────────────────────┘  │
│                                                                                                │
│  3 to generate · 1 missing · 2 will be written to sealed-secrets.yaml                          │
│                                  [ Generate the 3 ]   [ Apply to the cluster ]   [ Seal all ]  │
```

Applying is explicit and lists what it will do, with no values in sight:

```
            ┌──────────────────────────────────────────────────────────────────────┐
            │  Apply 2 Secrets to prod-eu-central / provider                       │
            │                                                                      │
            │    issuance-secret       create   key: keycloak-admin                │
            │    pg-scorpio            replace  keys: username, password           │
            │       ⚠ exists already; the value of `password` differs.             │
            │         ( ) keep what is in the cluster   (•) replace it             │
            │                                                                      │
            │  issuance.generatePasswords.enabled will be set to false in the same  │
            │  change, so the chart does not also create this Secret.              │
            │                                                                      │
            │                          [ Cancel ]   [ Dry run ]   [ Apply ]        │
            └──────────────────────────────────────────────────────────────────────┘
```

And a generated value is shown exactly once:

```
            ┌──────────────────────────────────────────────────────────────────────┐
            │  issuance-secret · keycloak-admin                                    │
            │                                                                      │
            │     ████████████████████████████████            [ show ] [ copy ]    │
            │                                                                      │
            │  This is the only time the tool will show it. It is in the cluster   │
            │  and in sealed-secrets.yaml; it is not in values.yaml, not in the    │
            │  profile and not in NEXT_STEPS.md.                                   │
            │                                                                      │
            │  Read it back later:                                                 │
            │    kubectl -n provider get secret issuance-secret \                  │
            │      -o jsonpath='{.data.keycloak-admin}' | base64 -d                │
            │                                                                      │
            │  ⚠ This is a temporary bootstrap administrator. Create a named       │
            │    admin and delete it — step 4 of NEXT_STEPS.md.                    │
            │                                                                      │
            │                        [ Save to a file… ]   [ I have it ]           │
            └──────────────────────────────────────────────────────────────────────┘
```

## S8 — Install (iteration 2)

```
│  Install into prod-eu-central / provider                                                       │
│                                                                                                │
│   ✓  Preflight                        storage class, ingress class, CRDs, hostnames    0:04    │
│   ✓  helm dependency update           14 subcharts                                     0:21    │
│   ●  helm upgrade --install provider  waiting for 9 of 14 workloads                    3:12    │
│        ✓ postgres                 ✓ keycloak              ● scorpio      pulling image         │
│        ✓ trusted-issuers-list     ● vcverifier   0/1      ○ apisix       waiting               │
│                                                                                                │
│        ▾ logs  provider/scorpio-5c9f…                                                          │
│          2026-11-12T09:14:02 INFO  Starting Scorpio broker …                                   │
│                                                                                                │
│   ○  Register at the Trust Anchor     hook job participant-registration                        │
│   ○  Verify                           fdsc-verify -n provider                                  │
│                                                                                                │
│                                                      [ Cancel ]   [ Show the helm command ]    │
```

## S9 — Enabling a feature later

The re-entry case. Opening a profile lands on Review; the user goes to Features, turns one on,
and the diff is small.

```
│  Review                                     profile mp-operations · opened 2 min ago           │
│  ┌───────────────────────────────┬──────────────────────────────────────────────────────────┐  │
│  │ Changes since the file        │  values.yaml   [ generated ] [▪ diff ] [ as loaded ]      │  │
│  │                               │                                                          │  │
│  │ + Consent enforcement on      │   2854    consent-facade:                                │  │
│  │   · consent-facade            │   2855  -   enabled: false                               │  │
│  │   · consent-owner-resolver    │   2855  +   enabled: true                                │  │
│  │   · 2 APISIX routes           │   2856  +   config:                                      │  │
│  │                               │   2857  +     consentManagerUrl: https://consent.data…   │  │
│  │ Untouched                     │   2858  +     participantId: did:web:mp-operations.org   │  │
│  │   4186 of 4209 lines          │   …                                                      │  │
│  │                               │   2902    consent-owner-resolver:                        │  │
│  │ ⚠ 1 new secret required       │   2903  -   enabled: false                               │  │
│  │   consent-facade-signing      │   2903  +   enabled: true                                │  │
│  │                        see ↗  │                                                          │  │
│  └───────────────────────────────┴──────────────────────────────────────────────────────────┘  │
│                                                                                                │
│                    [ Export values.yaml ]   [ Upgrade the release › ]                          │
```

And the conflict case — the file changed under the tool:

```
            ┌──────────────────────────────────────────────────────────────────────┐
            │  3 values were changed outside this tool                             │
            │                                                                      │
            │  scorpio.resources.limits.memory                                     │
            │     in the file   2Gi                                                │
            │     from profile  1Gi   (resource profile: production)               │
            │     ( ) keep the file's value, stop managing this key                │
            │     (•) take the profile's value                                     │
            │                                                                      │
            │  keycloak.replicaCount … 2 more                                      │
            │                                                                      │
            │                              [ Review each ]   [ Keep all file values ] │
            └──────────────────────────────────────────────────────────────────────┘
```

## Recurring elements

| Element | Appears | Behaviour |
|---|---|---|
| `inherited from X [override]` | Every stage-6 field whose value comes from stage 3 or 4 | Clicking *override* detaches the field; a detached field shows `pinned [reset]` and is excluded from re-renders |
| `used by N values ⓘ` | Every cross-cutting field | Popover listing the target keys |
| `⚠` inline | Anywhere a documented gotcha applies | Links to the paragraph of the guide that explains it, never restates it |
| `▾ Advanced (N chart keys)` | Every feature page | Schema-generated form over the keys of that feature |
| `⌘E Expert: YAML` | Always, in the rail | Full editor on the generated file; edits become overrides and are preserved |
| Footer validation counters | Always | Clicking an error jumps to the field that caused it |
| Secret delivery dropdown | The Secrets screen, every row | `let the chart` · `reference only` · `generate and apply` · `seal for Git` · `external secret`; a mode the cluster cannot support is shown disabled with the reason |
| `[ reveal once ]` | A generated Secret, until the dialog is dismissed | Afterwards the row shows only the fingerprint |
