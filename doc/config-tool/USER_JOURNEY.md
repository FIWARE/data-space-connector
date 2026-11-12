# User journey

How a user gets from "we want to join a data space" to a `values.yaml` that installs, and how
they come back later to change it.

<!-- ToC -->
- [Who walks it](#who-walks-it)
- [The shape of the journey](#the-shape-of-the-journey)
- [Stage 0 — Start](#stage-0--start)
- [Stage 1 — Target](#stage-1--target)
- [Stage 2 — Role](#stage-2--role)
- [Stage 3 — Foundations](#stage-3--foundations)
- [Stage 4 — Infrastructure](#stage-4--infrastructure)
- [Stage 5 — Features](#stage-5--features)
- [Stage 6 — Feature configuration](#stage-6--feature-configuration)
- [Stage 7 — Review and generate](#stage-7--review-and-generate)
- [Stage 8 — Install (iteration 2)](#stage-8--install-iteration-2)
- [Re-entry: changing a configuration later](#re-entry-changing-a-configuration-later)
- [What is asked when, and why](#what-is-asked-when-and-why)
- [Rules the journey obeys](#rules-the-journey-obeys)
<!-- /ToC -->

## Who walks it

| Persona | Knows | Does not know | What they need from the tool |
|---|---|---|---|
| **Pia, platform engineer** at a participant | Kubernetes, Helm, her cluster's ingress/storage/DB | What a VCVerifier is, which components a provider needs | A role preset, and the infrastructure questions in her vocabulary |
| **Dan, data space architect** at a participant | The data space's trust framework, credential types, the DID | Helm values layout | Feature-level language, no YAML |
| **Oleg, data space operator** (governance body) | The trust framework | — | The Operator role on the `trust-anchor` chart, participant onboarding |
| **Eva, evaluator** | Nothing yet | Everything | A "local trial" path that works with defaults in one screen |

Pia and Dan usually sit together for stage 3; the tool must let them leave a question
unanswered, mark it, and come back ([US-6.3](USER_STORIES.md#epic-6--validation-and-review)).

## The shape of the journey

```
  ┌──────────┐
  │ 0 Start  │  new · open values.yaml · open profile · local trial
  └────┬─────┘
       ▼
  ┌──────────┐   ┌──────────┐   ┌──────────────┐   ┌────────────────┐
  │ 1 Target │──▶│ 2 Role   │──▶│ 3 Foundations│──▶│ 4 Infrastructure│
  └──────────┘   └──────────┘   └──────────────┘   └────────┬────────┘
                                                            ▼
  ┌────────────────┐   ┌──────────────────────┐   ┌──────────────────┐
  │ 7 Review+gen   │◀──│ 6 Feature config     │◀──│ 5 Features       │
  └───────┬────────┘   │   (only enabled ones)│   │   (toggles)      │
          │            └──────────────────────┘   └──────────────────┘
          ▼
  ┌────────────────┐
  │ 8 Install      │ (iteration 2)
  └────────────────┘

  Any stage is reachable from the left-hand stage list at any time — the wizard is the
  recommended order, not a cage. Stage 7 is always reachable: a configuration can be
  exported half-finished, with its open questions listed in the file as TODO comments.
```

## Stage 0 — Start

Four entries, one screen ([mockup](MOCKUPS.md#s0--start)):

| Entry | What happens |
|---|---|
| **New configuration** | Stage 1 with an empty answer set |
| **Open an existing `values.yaml`** | The file is parsed, every key classified (see [Round-trip](FEATURE_MODEL.md#round-trip-and-provenance)), recognized answers pre-filled, unknown keys kept as overrides. Lands on Stage 7 with a summary of what was understood — the user is editing, not re-configuring |
| **Open a profile** (`.dscprofile.yaml`) | The tool's own answer set, saved from an earlier session. Lands on Stage 7 |
| **Try it locally** | A one-screen path: pick "local trial", get the equivalent of the [Quick Start](../deployment-integration/local-deployment/LOCAL.MD) — trust anchor + provider + consumer profiles for a k3s on this machine. Skips stages 1–6 |

Opening a values file and opening a profile converge on the same in-memory state; a profile
simply carries more (which questions were answered deliberately vs. left at their default).

## Stage 1 — Target

Where this is going, because it changes what can be detected and what must be asked.

- **Target type:** `File only` (generate YAML, install elsewhere) · `Kubernetes cluster`
  (kubeconfig + context picker). Iteration 1 supports the cluster target read-only — it is used
  for detection in stage 4; installing arrives in iteration 2.
- **Namespace** and **release name** — they appear in generated hostnames, in-cluster service
  URLs and the Trust Anchor registration, so they are needed early.
- **Chart version** — pinned, defaulting to the newest the tool knows. The question model is
  versioned with the chart ([US-9.3](USER_STORIES.md#epic-9--changing-a-configuration-later)).
- **Environment:** `development` · `production`. Drives defaults, not questions: resource
  profile, `did:key` vs `did:web`, self-signed vs ACME issuer, replica counts.

## Stage 2 — Role

A card per role, with the component matrix from
[Deployment by Role](../deployment-integration/roles/README.md) shown as a preview of what the
card enables.

| Card | Chart | Preset |
|---|---|---|
| **Consumer** | `data-space-connector` | Issuance + DID + PostgreSQL |
| **Provider** | `data-space-connector` | Consumer + verification + policy enforcement + data service |
| **Consumer + Provider** | `data-space-connector` | Provider preset (the realistic default) |
| **Operator** | `trust-anchor` | TIR/TIL + PostgreSQL; optional consent authority, onboarding portal |
| **Custom** | choose chart | Nothing enabled; the full catalogue, nothing recommended |

Choosing a role **sets the recommended toggles and nothing else**. Stage 5 shows them,
marked as "recommended for Provider", and every one of them can be turned off; everything
outside the preset can be turned on. Changing the role later re-applies the preset only to
features the user has never touched — a feature the user switched by hand keeps its state and
is listed in the change summary ([US-2.3](USER_STORIES.md#epic-2--role-presets)).

## Stage 3 — Foundations

The cross-cutting values, asked **once**, before anything that consumes them. This is the
stage that earns the tool its keep: these answers fan out to between 2 and 41 places in the
chart (the full table is in
[Cross-cutting values](FEATURE_MODEL.md#cross-cutting-values)).

Grouped into four short panels on one scrollable screen:

1. **Organization** — display name, base domain, DID method and DID, legal identifier
   (for eIDAS/Gaia-X if those are later enabled).
2. **Data space** — Trust Anchor TIR address, the credential types the data space defines,
   whether participant registration is automatic. From the Operator's onboarding mail; the
   tool offers "import from a data space descriptor URL" where the data space publishes
   a `/.well-known/data-space-configuration`.
3. **Network** — host naming scheme (derived from the base domain, each host editable),
   ingress class and annotations, TLS mode, private CA, outbound proxy.
4. **Platform** — storage class, resource profile, image pull secrets, registry mirror.

Everything here has a default that a development environment can accept unchanged. Each
field shows **where it will be used** ("used by 25 values in 9 components") and a link to the
guide that explains it.

> The host naming scheme deserves its own note: the chart needs a public hostname for the DID
> document, the verifier, APISIX, Keycloak, the marketplace, the TMForum API and — when
> enabled — DSP, the status list server and the dashboard. The tool derives them all from the
> base domain with a pattern (`<component>.<base>`, or a single host with path prefixes), shows
> the resulting list, and lets any single host be overridden. This is where
> [hairpin DNS](../deployment-integration/roles/PREREQUISITES.md#public-names-have-to-resolve-inside-the-cluster)
> is warned about, because the generated configuration depends on it.

## Stage 4 — Infrastructure

Every dependency the charts share with the rest of the cluster, each as a three-way choice
(see [Integration modes](FEATURE_MODEL.md#integration-modes)):

```
PostgreSQL        ( ) install with the connector   (•) already in the cluster   ( ) external service
cert-manager      (•) install with the connector   ( ) already in the cluster   ( ) certificates provided as Secrets
MongoDB           ( ) install …                    ( ) already …                ( ) external service      [only if the marketplace is on]
Ingress / DNS     — ingress class, external-dns annotations, or "I create DNS records myself"
Secret store      ( ) Vault deployed by the chart  ( ) existing Vault           ( ) Kubernetes Secrets only
Observability     ( ) deploy the OTel stack        ( ) existing collector       ( ) off
Secret delivery   ( ) let the chart generate them  (•) generate and apply here  ( ) seal for Git  ( ) reference only
```

Secret delivery is the one with five modes rather than three, because a credential can also be
generated by the tool, applied to the connected cluster, or sealed for Bitnami Sealed Secrets
and committed. The default follows the environment and the target, and every individual Secret
can depart from it later on the Secrets screen — see [Secret handling](SECRETS.md).

When the target is a cluster, the tool has already looked: it reports *"cert-manager v1.21
found in `cert-manager`, ClusterIssuers: `letsencrypt-prod`, `selfsigned-issuer`"* and
pre-selects *already in the cluster*. When nothing is found, the tool offers the recommendation
and the exact upstream command from
[Installing the operators once](../deployment-integration/roles/PREREQUISITES.md#installing-the-operators-once) —
it recommends, it does not force.

Each mode asks only what it needs. *Already in the cluster* asks for references (issuer name,
operator namespace, storage class). *External* asks for the endpoint and the name of the
Secret holding the credentials — never the credentials themselves.

> The single most common failure this stage prevents: two releases in one cluster both
> installing the Zalando Postgres Operator or cert-manager. The tool knows an operator is
> cluster-wide, so when it sees one in the cluster — or when the user says a Trust Anchor
> already runs there — it selects *already in the cluster* and explains why.

## Stage 5 — Features

The catalogue, grouped by capability, as a list of toggles with a one-line "what this gives
you" and a "required for Provider / recommended / optional" badge. Not components — the
[features](FEATURE_MODEL.md#the-feature-catalogue) are what a user wants, and each one maps to
one or several chart keys.

Turning a feature on resolves its dependencies and says so in a line under the toggle:
*"Marketplace needs MongoDB and the TMForum API — both enabled."* Turning one off that
another needs asks, and offers to turn both off.

Features that are enabled but not yet configured carry a dot; the counter in the footer reads
`3 features need configuration`.

## Stage 6 — Feature configuration

One page per enabled feature, in dependency order, each with:

- the handful of questions that feature actually needs, everything else defaulted;
- fields already filled from stage 3 and 4, shown as *inherited* with a small override link —
  this is how "configure cross-cutting things early and only override later if needed" is
  made visible rather than implicit;
- an **Advanced** disclosure that exposes the remaining chart keys for that feature, generated
  from [`values.schema.json`](../../charts/data-space-connector/values.schema.json) — so the
  tool is never a dead end for a key nobody wrote a question for;
- the link to the section of the role guide that explains it.

A feature can be left unconfigured: it keeps the chart defaults and is listed at review as
*"using defaults"*.

The heaviest page is **credential issuance**: the Keycloak realm, its clients, the credential
types the data space defined (from stage 3), the claims each one carries, and the users or
groups that get them. It is a table editor with the data space's credential types
pre-seeded, not 40 scalar fields; see the [mockup](MOCKUPS.md#s6b--feature-configuration-credential-issuance).

## Stage 7 — Review and generate

Three panes ([mockup](MOCKUPS.md#s7--review-and-generate)):

- **Summary** — role, features on/off, infrastructure modes, open questions.
- **Validation** — errors (block generation), warnings (do not), information. Four layers:
  schema, cross-field rules, a real `helm template` render, and — with a cluster target —
  cluster preflight. The rules are listed in
  [Validation](FEATURE_MODEL.md#validation-rules).
- **YAML** — the generated file, with a diff against what was loaded, keys the tool owns
  highlighted, and the rest shown as untouched.

Generating writes three things:

1. `values.yaml` — with a provenance header and `# dsc-config:` markers on owned blocks;
2. `.dscprofile.yaml` — the answer set, so the next session starts where this one ended;
3. `NEXT_STEPS.md` — the manual steps the tool cannot do: the Secrets still to create and what
   they must contain, the DNS records, the registration to request from the Operator, and the
   `helm upgrade --install` command line for this configuration;
4. `sealed-secrets.yaml`, when any Secret is in `sealed` mode — encrypted for this cluster's
   controller and safe to commit.

A **Secrets** pane sits beside the three: every credential the configuration needs, its delivery
mode, and whether it exists yet. Generating or applying a Secret happens there, explicitly, and
a generated value is shown once and never written to any of the files above
([Secret handling](SECRETS.md)).

## Stage 8 — Install (iteration 2)

`helm upgrade --install` through the Helm SDK against the selected context, with per-release
progress, the hook jobs' logs streamed, and a failure report that names the release and the
resource. Then `fdsc-verify -n <namespace> --preflight-only`, and the full run on request.
Re-running the tool on an installed release shows a **diff against the deployed values**, not
just against the file.

## Re-entry: changing a configuration later

The requirement that shapes the data model. Four ways back in:

| The user wants to | They do | The tool does |
|---|---|---|
| **Enable a feature later** | Open profile → stage 5 → toggle → configure → review | Renders only the new keys; review shows a diff of ~20 lines, not a new file |
| **Change a cross-cutting value** (domain moved) | Open → stage 3 → edit domain | Re-renders every place the value fans out to, lists them, and flags the overrides that were pinned by hand so the user can decide per case |
| **Change the role** | Open → stage 2 | Re-applies the preset to untouched features only, shows what changed |
| **Upgrade the chart** | Open → stage 1 → chart version | Migrates the answer set to the new question model, reports renamed and removed keys, lists what the new version adds ([US-9.3](USER_STORIES.md#epic-9--changing-a-configuration-later)) |

In all four, the answer set is the thing being edited and `values.yaml` is re-rendered. A
configuration edited by hand between sessions survives because unknown keys were preserved and
hand-edits to owned keys are detected and surfaced as a conflict, never silently overwritten.

## What is asked when, and why

| Asked at | Question | Why there |
|---|---|---|
| Stage 1 | Namespace, release, chart version | Appear inside generated hostnames and service URLs |
| Stage 2 | Role | Sets the defaults everything else starts from |
| Stage 3 | Domain, DID, TIR, TLS, ingress, storage | Consumed by 5–25 keys each; asking later would mean asking repeatedly |
| Stage 4 | Who provides PostgreSQL, certs, Mongo, Vault | Decides whether stage 6 asks for connection details or for nothing |
| Stage 5 | Which features | Decides which stage 6 pages exist |
| Stage 6 | Per-feature detail | Only for what is on, and only what is not inherited |
| Stage 7 | Per-secret delivery and the values of class B credentials | The full list of Secrets is only known once the features are |

## Rules the journey obeys

1. **A disabled feature is never configured.** No page, no validation, no keys in the output.
2. **Nothing is asked twice.** A value asked in stage 3 appears later only as an inherited
   field with an override link.
3. **Every stage can be left and returned to.** Navigation is a list, not a chain; the
   answer set is valid at every point.
4. **Defaults come from the chart, not from the tool.** A question the user never answers
   produces no key in `values.yaml` — the chart's own default applies, so upgrading the chart
   keeps moving the default.
5. **The tool recommends, the user decides.** Every recommendation says who it is for and
   can be overruled; no toggle is locked.
6. **Nothing the tool did not write is changed.** Including comments and key order, as far as
   the YAML round-trip allows.
7. **No secret value is ever written to a file the tool generates.** A generated credential
   goes to the cluster or into a SealedSecret, is shown once, and leaves a fingerprint in the
   profile — never a value.
