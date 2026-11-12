# Secret handling

Referencing a Secret by name is the safe default, not the only mode. This document covers what
credentials a connector needs, who may create them, what the tool generates, how it applies or
seals them, and what happens to the bootstrap passwords — the Keycloak admin above all.

<!-- ToC -->
- [What a connector needs](#what-a-connector-needs)
- [Delivery modes](#delivery-modes)
- [Choosing a mode](#choosing-a-mode)
- [Generation](#generation)
- [The Keycloak admin password](#the-keycloak-admin-password)
- [Sealed Secrets](#sealed-secrets)
- [External Secrets](#external-secrets)
- [Applying to a cluster](#applying-to-a-cluster)
- [Rules](#rules)
- [Validation](#validation)
<!-- /ToC -->

## What a connector needs

Three classes, which behave differently and must not be treated alike.

### A — Bootstrap credentials the chart can generate itself

These exist only because a component needs *some* password on first start. The chart already
generates them, each with its own stabilisation mechanism — which is exactly why the tool has to
have an opinion about them rather than inventing a parallel one.

| Secret | Keys | Chart switch | How the chart keeps it stable |
|---|---|---|---|
| `issuance-secret` | `keycloak-admin` | `issuance.generatePasswords.enabled` (default **true**) | `lookup` of the existing Secret, else `randAlphaNum 30` |
| `mongodb-admin-password` | `password` | `marketplace.generatePasswords` (default **true**) | rendered on `.Release.IsInstall` only, `helm.sh/resource-policy: keep` |
| `mongodb-charging-password` | per `marketplace.bizEcosystemChargingBackend.db.secretKey` | same | same |
| `mongodb-belp-password` | per `marketplace.bizEcosystemLogicProxy.db.secretKey` | same | same |
| vc-operator signing key | ECDSA private key | `vc-operator` enabled | `lookup`, else `genPrivateKey "ecdsa"` |
| `consent-manager-env`, `consent-manager-signing-keys` *(trust-anchor)* | DB, pseudonym and erasure keys; receipt JWK | `consentManagement.enabled` | a plain setup Job that creates them once with `kubectl create secret` and never rotates |

Two of those mechanisms depend on Helm running against a live cluster. **`lookup` returns
nothing under `helm template`**, and ArgoCD and Flux do not enable it by default — so every
render produces a *new* random value. The chart's own
[secret management guide](../../charts/README.md#secret-management) says to set both
`generatePasswords` flags to `false` in production and in GitOps. The consent setup Job is the
pattern that avoids the problem entirely ([ADR 0007](../adr/0007-test-both-deployment-paths.md)),
which is why it exists.

The tool knows this table, knows which rendering path the user chose, and never leaves the
combination "GitOps target + chart-generated passwords" silently in place.

### B — Credentials of things outside the chart

Nothing can generate these; they belong to systems the connector only talks to.

| For | Typical keys |
|---|---|
| An existing PostgreSQL, per component (Keycloak, TIL, verifier, PAP, Scorpio, IdentityHub, status list) | `username`, `password` |
| An existing MongoDB | connection string or `username`/`password` |
| An existing Vault | token, AppRole id and secret, or a Kubernetes auth role |
| An external OTLP endpoint | authorization header |
| Image registries | a `kubernetes.io/dockerconfigjson` pull secret |
| A private CA | the CA bundle (not secret, but handled on the same screen) |
| SMTP, for the consent authority | `username`, `password` |
| A central marketplace | client id and secret |

For these the tool collects the **name and the key layout**, validates that they exist when a
cluster is connected, and lists the missing ones in `NEXT_STEPS.md`. It offers to create them
from values the user types — in `generate+apply` or `sealed` mode — but it never invents the
value, because the value has to match something that already exists elsewhere.

### C — Identity material

Keys and certificates that *are* the participant's identity: the DID's key, the Keycloak realm
keystore, the EDC/IdentityHub key, the ELSI certificate. These are not passwords and are not
interchangeable — losing one means re-registering at the Trust Anchor, and rotating one has to
preserve the key, see
[Identity key lifecycle](../deployment-integration/roles/provider/README.md#identity-key-lifecycle).

The tool treats them as a separate class: it can generate a development key, it never rotates
one silently, and before any action that would replace one it says what the consequence at the
Trust Anchor is.

## Delivery modes

Every Secret in the configuration carries one of five modes.

| Mode | What the tool does | Where the value ends up | Committable | Needs a cluster |
|---|---|---|---|---|
| `chart` | Leaves it to the chart; sets `*.generatePasswords: true` | Generated in-cluster by the chart | — | no |
| `reference` | Records the name and the keys only | Somebody else creates it | — | no |
| `generate+apply` | Generates the value and applies the Secret to the connected cluster | The cluster, and the reveal-once dialog | no | **yes** |
| `sealed` | Generates the value and encrypts it for the cluster's Sealed Secrets controller | A `SealedSecret` manifest in the export bundle | **yes** | yes, to fetch the certificate (or an offline cert) |
| `external` | Emits an `ExternalSecret` that points at the organization's store | The organization's secret store; the tool never sees it | **yes** | the operator must be installed |

`chart` and `reference` are iteration 1. `generate+apply` and `sealed` arrive with the cluster
connection in iteration 2, as does `external`; the model carries all five from the start so that
a profile written today does not have to be migrated.

## Choosing a mode

A global default on [stage 4](USER_JOURNEY.md#stage-4--infrastructure), overridable per Secret on
the Secrets screen ([mockup](MOCKUPS.md#s7b--secrets)).

| Situation | Default |
|---|---|
| Environment `development`, file-only target | `chart` |
| Environment `development`, cluster target | `generate+apply` |
| Environment `production`, cluster target, Sealed Secrets detected | `sealed` |
| Environment `production`, cluster target, no Sealed Secrets | `generate+apply`, with `reference` one click away |
| Environment `production`, file-only target | `reference` |
| External Secrets Operator detected | offered for class B, never forced |
| Class B secrets, any environment | `reference` unless the user types a value |

Choosing `generate+apply` or `sealed` for a class A secret also sets the matching
`generatePasswords` flag to `false`, so the chart and the tool do not both own the same Secret.
That coupling is part of the render, not something the user has to remember.

## Generation

- Values come from the operating system CSPRNG (`crypto/rand`). Passwords are 32 characters from
  an alphabet that survives shell quoting, connection strings and JDBC URLs; keys are generated
  per algorithm (ECDSA P-256 for the vc-operator's signing key, RSA 4096 where a component
  demands RSA), and JWKs are emitted in the shape the consuming component expects.
- **Nothing generated is written into `values.yaml`, into the profile, or into `NEXT_STEPS.md`.**
  The profile records the mode, the Secret name, the key names and a truncated SHA-256
  fingerprint of each value — enough to detect drift, useless to an attacker.
- A generated value is shown **once**, in a reveal dialog, with copy and "save to a file I
  choose". After that dialog closes, only the cluster (or the SealedSecret) has it. The tool
  says so before generating, not after.
- Re-running a render does not re-generate. A value is regenerated only when the user asks, and
  that is a rotation: the tool states which component has to restart and what has to be changed
  elsewhere before it does anything.
- In `reference` mode generation is refused with the reason — there is nowhere to put the value.

## The Keycloak admin password

**Today, without the tool.** The chart generates it. `issuance.generatePasswords.enabled` is
`true` by default, so `templates/issuance-secrets.yaml` renders the Secret `issuance-secret` with
the key `keycloak-admin` set to `randAlphaNum 30`, reusing the existing value through `lookup`
when one is already in the namespace. Keycloak consumes it as
`auth.existingSecret: issuance-secret` / `secretKeys.adminPasswordKey: keycloak-admin`, which the
subchart turns into `KC_BOOTSTRAP_ADMIN_USERNAME` and `KC_BOOTSTRAP_ADMIN_PASSWORD`. Nobody is
ever shown the password; you read it back with

```shell
kubectl -n <namespace> get secret issuance-secret -o jsonpath='{.data.keycloak-admin}' | base64 -d
```

Two consequences follow from `KC_BOOTSTRAP_ADMIN_*`, and the tool is explicit about both because
each one surprises people:

1. **The bootstrap admin is created once.** Keycloak applies those variables only while the realm
   has no admin account. Changing the Secret afterwards changes nothing in Keycloak — the account
   keeps the password it was created with. "Rotate the Secret" is not a rotation.
2. **`lookup` is not available on every path.** Under `helm template | kubectl apply`, ArgoCD or
   Flux, the lookup finds nothing and a fresh random value is rendered on every single render. The
   Secret churns on each sync while the actual admin password stays at whatever the first install
   produced — so the value in the cluster stops being the password that works.

**What the tool does.**

| | |
|---|---|
| **Development, cluster target** | Generates a 32-character password, applies `issuance-secret`, sets `issuance.generatePasswords.enabled: false`, and shows the password once with the read-back command. |
| **Development, file only** | Leaves `generatePasswords: true` and puts the read-back command in `NEXT_STEPS.md`. |
| **Production** | Generates and either applies or seals it, depending on the mode; marks the account as a **temporary bootstrap admin**, and `NEXT_STEPS.md` carries the follow-up: log in once, create a named administrator with the rights your organization expects, and delete the bootstrap account. |
| **GitOps / `helm template` with `generatePasswords: true`** | A validation **error**, not a warning, with the three ways out: `sealed`, `generate+apply`, or `reference` plus your own Secret. |
| **Rotation requested** | The tool states that changing the Secret does not change the account, and offers the path that does work: a new admin created through the admin API (or `kc.sh bootstrap-admin` on the pod), then the old account removed. It does not pretend to rotate it by editing a Secret. |
| **Admin user name** | Asked on the issuance page (`keycloak.auth.adminUser`, default `keycloak-admin`) rather than left at a default nobody chose. |

The same treatment applies to the three MongoDB passwords of the local marketplace and to the
consent authority's setup-Job Secrets: the tool knows which switch owns them, which rendering
paths break them, and offers to own them instead.

## Sealed Secrets

Supported when the cluster has the controller; absent, the mode is offered greyed out with one
line on what to install.

**Detection.** The CRD `sealedsecrets.bitnami.com/v1alpha1` and a controller Deployment — by
default `sealed-secrets-controller` in `kube-system`, with the name and namespace
overridable because installations differ. The Secrets screen reports the controller's namespace
and the age of its certificate.

**Certificate.** Fetched from the controller's service endpoint (`/v1/cert.pem`) through the
API server proxy, from `kubeseal --fetch-cert` when the binary is on the PATH, or from a PEM file
the user points at — the last one is what makes an air-gapped or a cluster-less workstation work,
since sealing needs only the public key.

**Encryption in process.** Sealed Secrets' scheme is a hybrid: a random AES-256-GCM session key
per value, wrapped with RSA-OAEP under the controller's public key. The tool does it with
`github.com/bitnami-labs/sealed-secrets/pkg/crypto`, so no `kubeseal` binary is required; if one
is present it is still used for `--fetch-cert`.

**Scope.** `strict` by default — the ciphertext is bound to the Secret's name *and* namespace, so
a SealedSecret cannot be moved or renamed and then decrypted. `namespace-wide` and `cluster-wide`
are offered, each with its consequence stated at the point of choice. Because the binding is to
name and namespace, changing either on stage 1 **invalidates every strict SealedSecret**; the tool
detects that and re-seals, and the review diff says why every one of them changed.

**Stable output.** Sealing the same value twice produces different ciphertext, which would make
every export a noisy diff. The tool therefore **keeps the existing sealed value** unless the
plaintext changed, the scope changed, or the name or namespace moved. A re-seal is always
something the user can explain.

**Output.** `sealed-secrets.yaml` in the export bundle, safe to commit, with `NEXT_STEPS.md`
saying to apply it before or with the release. Controller key rotation needs no action: the
controller keeps its old keys and can still decrypt what was sealed with them.

## External Secrets

Where the organization already runs the External Secrets Operator — which the chart's own
[guidance](../../charts/README.md#secret-management) recommends — the tool emits `ExternalSecret`
resources instead of Secrets: the store reference, the remote key per Secret key, and the
refresh interval. It never reads or writes the store itself. Detection is the
`external-secrets.io` CRDs plus the available `ClusterSecretStore`s, which the Secrets screen
offers as a list.

## Applying to a cluster

- Only on an explicit action, never as a side effect of generating or exporting. The screen lists
  every Secret it is about to create, by name and namespace, with the keys and without the values.
- Server-side apply with the field manager `dsc-config`, after a dry-run.
- **An existing Secret is never overwritten silently.** The tool compares key sets and value
  fingerprints and reports *which keys* differ — never the values — then asks per Secret: keep
  what is in the cluster, or replace it.
- The `*.generatePasswords` flags are set to `false` in the same change, so the next `helm
  upgrade` does not fight the applied Secret.
- The RBAC the tool needs is `create`, `get` and `patch` on Secrets in the target namespace, and
  it says so when a permission is missing instead of failing mid-way.
- Uninstalling never deletes a Secret the tool applied; they are listed so the user can.

## Rules

1. A plaintext secret never reaches `values.yaml`, the profile, `NEXT_STEPS.md`, a log line, or
   any file the tool writes without being asked for that file explicitly.
2. The only committable forms are a `SealedSecret`, an `ExternalSecret`, and a reference by name.
3. A generated value is displayed exactly once; the tool says that before it generates.
4. The tool and the chart never both own a Secret: choosing a tool-owned mode flips the chart's
   switch off in the same render.
5. Nothing is applied to a cluster without an explicit action that lists what will be applied.
6. Class C identity material is never replaced as a side effect of anything.
7. The profile stores modes, names, key names and fingerprints — never values.

## Validation

Added to the layer-2 rules in [the feature model](FEATURE_MODEL.md#validation-rules):

| Rule | Severity |
|---|---|
| `generatePasswords: true` with a `helm template` or GitOps target | **error** — `lookup` is unavailable, the value churns on every render |
| A referenced Secret does not exist in the target namespace (cluster target) | error before install, warning while configuring |
| A referenced Secret exists but lacks a key the component reads | error |
| `sealed` selected without a controller or a certificate | error |
| `sealed` with `cluster-wide` scope | warning, with what it permits |
| A strict SealedSecret whose name or namespace changed | warning, re-sealed, listed in the diff |
| Environment `production` with any `chart`-mode class A secret | warning, with the three alternatives |
| Environment `production` with a bootstrap admin that is still the only administrator | warning in `NEXT_STEPS.md` |
| The same Secret name claimed by two modes | error |
