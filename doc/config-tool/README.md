# DSC Configurator — design documents

Design for a graphical tool that configures and installs the FIWARE Data Space Connector.

The connector is a Helm umbrella chart with **30 top-level feature toggles**, a **4187-line
`values.yaml`**, a second chart for the Operator role, and a handful of cluster-wide
prerequisites that may or may not already exist in the target cluster. Today an operator
assembles that by reading five role guides and copying from the `k3s/` example profiles. The
scenario files show the cost: [`k3s/provider.yaml`](../../k3s/provider.yaml) is 1868 lines and
names the same domain on **41** of them, the same DID on **15**.

The tool replaces that assembly step with a guided configuration that produces the same
artifact — a `values.yaml` — and, from the second iteration on, installs it.

## Scope of iteration 1

In: a cross-platform desktop/local tool that **generates and edits a `values.yaml`** for the
`data-space-connector` and `trust-anchor` charts, with role presets, individually
selectable features, and reuse of infrastructure that already exists in the target
environment. State is held in memory while configuring; the file is the persisted artifact.

Out (planned, designed for): installing into a cluster, reading the cluster to pre-fill
answers, running [`fdsc-verify`](https://github.com/SEAMWARE/fdsc-verify) afterwards,
GitOps export. The architecture in [TECH_STACK.md](TECH_STACK.md) keeps those additive.

The tool lives **in this repository**, under `config-tool/`, with its question model next to
`values.yaml` and its own release tags — see
[Where the code lives](TECH_STACK.md#where-the-code-lives). It exists only to configure this
chart and reads this chart's schema, so the two change together.

## Documents

| Document | What it answers |
|---|---|
| [USER_JOURNEY.md](USER_JOURNEY.md) | Where the user starts, what is asked when, and how a configuration is changed later |
| [FEATURE_MODEL.md](FEATURE_MODEL.md) | The feature catalogue, the role presets, the cross-cutting values and the integration modes — the data model the UI renders |
| [MOCKUPS.md](MOCKUPS.md) | Wireframe of every screen of the journey |
| [mockups/prototype.html](mockups/prototype.html) | Clickable prototype of the wizard — open it in a browser |
| [TECH_STACK.md](TECH_STACK.md) | Which stack, why, and how the pieces fit |
| [SECRETS.md](SECRETS.md) | Which credentials a connector needs, who creates them, and how the tool generates, applies, seals or merely references them |
| [USER_STORIES.md](USER_STORIES.md) | The cases to be solved, as stories with acceptance criteria, cut into iterations |
| [ADR 0009](../adr/0009-config-tool-stack.md) | The stack decision as an ADR |
| [ADR 0010](../adr/0010-config-tool-secret-handling.md) | The secret-handling decision as an ADR |

## The four ideas the design rests on

1. **The answer set, not the values file, is the tool's state.** A small normalized profile
   (role, cross-cutting values, enabled features, per-feature answers) is what the user edits;
   `values.yaml` is rendered from it. That is what makes "enable a feature later" a small diff
   instead of a re-run.
2. **Cross-cutting values are asked once, early, and fan out.** Domain, DID, TLS, ingress class,
   storage class, Trust Anchor address. Every later screen pre-fills from them and only asks
   again where the user overrides.
3. **Every infrastructure dependency has three modes**, never a boolean: *install it with the
   connector*, *it already exists in the cluster*, or *it is managed outside Kubernetes and I
   give you the endpoint*. See [Integration modes](FEATURE_MODEL.md#integration-modes).
4. **Anything the tool did not write, it does not touch.** A loaded `values.yaml` keeps every
   key the tool has no question for, verbatim. See
   [Round-trip](FEATURE_MODEL.md#round-trip-and-provenance).
5. **A secret has a delivery mode, not a checkbox.** Let the chart generate it, reference one
   somebody else creates, have the tool generate and apply it to the connected cluster, seal it
   for Bitnami Sealed Secrets, or point at the External Secrets Operator — whichever fits the
   environment, per secret. See [Secret handling](SECRETS.md).

## Non-goals

- Not a replacement for the role guides; the tool links into them for every question.
- Not a cluster manager: it configures and installs one connector, it does not operate it.
- Not a secret store, and not a place secrets live: generated values go to the cluster or into a
  SealedSecret, never into the generated `values.yaml` or the profile. The tool does generate
  credentials and, on a connected cluster, create them — see [Secret handling](SECRETS.md) and
  [US-7.4](USER_STORIES.md#epic-7--output-and-handover).
