# 0010 - The configuration tool creates secrets, but never stores their values

**Status:** Proposed
**Date:** 2026-11-12
**Applies to:** the configuration tool (`dsc-config`), `FIWARE/data-space-connector`
**Context doc:** [`../config-tool/SECRETS.md`](../config-tool/SECRETS.md)

## Context

A connector needs three kinds of credential: bootstrap passwords that exist only because a
component wants one on first start (the Keycloak admin, the marketplace's MongoDB users, the
vc-operator's signing key), credentials of systems outside the chart (an existing PostgreSQL, an
existing Vault, a registry, SMTP), and identity material that *is* the participant (the DID key,
the realm keystore, the EDC key).

The chart already generates the first kind, with two mechanisms that both depend on Helm running
against a live cluster: `lookup` with a random fallback (`issuance.generatePasswords`,
vc-operator) and render-on-install plus `helm.sh/resource-policy: keep`
(`marketplace.generatePasswords`). Under `helm template | kubectl apply`, ArgoCD or Flux,
`lookup` returns nothing, so every render produces a new random value while the account keeps the
password it was created with. The chart's own
[secret management guide](../../charts/README.md#secret-management) therefore tells production
and GitOps users to set both flags to `false` and supply the Secrets from External Secrets,
Sealed Secrets or a prior CI step. The consent setup Job in the trust-anchor chart avoids the
problem by creating its Secrets itself, once, in a way that behaves identically on both
deployment paths ([ADR 0007](0007-test-both-deployment-paths.md)).

A tool that only ever emits a Secret *name* leaves the user exactly where they are today: with a
manual step per credential, and with the chart's defaults quietly doing the wrong thing on a
GitOps path. A tool that writes values into `values.yaml` would be worse — that file is meant to
be reviewed and committed.

## Decision

- Every Secret in a configuration carries a **delivery mode**, not a boolean: `chart` (leave it
  to the chart), `reference` (name only), `generate+apply` (the tool creates it in the connected
  cluster), `sealed` (the tool encrypts it for the cluster's Bitnami Sealed Secrets controller
  and writes a committable `SealedSecret`), or `external` (the tool emits an `ExternalSecret` for
  the organization's store). The default follows the environment and the target; every Secret can
  depart from it.
- **No secret value is written to any file the tool generates** — not `values.yaml`, not the
  profile, not `NEXT_STEPS.md`. A generated value goes to the Kubernetes API of the selected
  context or into a `SealedSecret`, and is shown to the user exactly once. The profile keeps the
  mode, the Secret name, the key names and a truncated SHA-256 fingerprint, which is enough to
  detect drift.
- **The chart and the tool never both own a Secret.** Choosing a tool-owned mode sets the
  matching `generatePasswords` flag to `false` in the same render.
- **`generatePasswords: true` with a `helm template` or GitOps target is an error**, with the
  three ways out, because that combination silently re-randomises the Secret on every sync.
- **Sealing happens in process** with `bitnami-labs/sealed-secrets/pkg/crypto`; `kubeseal` is
  optional and used only to fetch a certificate. Since sealing needs only the controller's public
  key, it also works from a workstation with no cluster access. Ciphertext is non-deterministic,
  so an unchanged plaintext keeps its existing sealed value and a re-seal is always explainable.
- **Nothing is applied to a cluster except through an explicit action** that first lists the
  Secrets by name, namespace and keys, never values, and never overwrites an existing Secret
  without asking.
- **Identity material is a class of its own**, never replaced as a side effect of anything.
- The tool is **honest about `KC_BOOTSTRAP_ADMIN_*`**: that account is created only while the
  realm has no administrator, so changing the Secret afterwards does not change the password. A
  rotation request is answered with the path that works, not with a Secret edit.

## Consequences

- The tool becomes useful on the two paths where the chart's defaults are wrong today — GitOps
  and production — instead of reproducing them.
- The tool needs a Kubernetes write path and cryptography, which is why the security rules above
  are invariants with tests behind them rather than guidance.
- A user who picks `generate+apply` or `sealed` and loses the reveal-once dialog has to read the
  value back from the cluster. That is the cost of not persisting values, and the read-back
  command is in `NEXT_STEPS.md` for every such Secret.
- The tool has to track which chart version owns which Secret under which switch. That lives in
  the model bundle next to `values.yaml`
  ([ADR 0009](0009-config-tool-stack.md)), so a chart change that moves a Secret is caught by the
  same CI check as any other key.
- Sealed Secrets support is optional: absent a controller, the mode is offered disabled. No
  dependency is added to the charts themselves.

## Alternatives

- **Reference only, never create.** The original scope. Rejected: it leaves one manual step per
  credential, and it does not let the tool fix the GitOps failure mode it can detect.
- **Write generated values into `values.yaml` under a flag.** Rejected: that file is written to be
  reviewed, committed and attached to tickets.
- **Keep a local encrypted vault of generated values.** Rejected for iteration 1: it makes the
  tool a secret store with a master secret, a backup story and a sync story, to save a `kubectl
  get secret`.
- **Shell out to `kubeseal`.** Rejected as the primary path: it would make sealing depend on a
  binary that is usually absent on a workstation, for crypto that is a library call.
- **Let the chart keep generating everything.** Rejected: the chart's mechanisms are correct only
  on the `helm install` path, which is precisely the case where the user needs the least help.
