# User stories

The cases the tool has to solve, grouped into epics. Each story carries acceptance criteria
that are testable, a priority (**M**ust / **S**hould / **C**ould) and the iteration it belongs
to ([roadmap](TECH_STACK.md#iterations)).

Personas are the ones from the [user journey](USER_JOURNEY.md#who-walks-it): **Pia** (platform
engineer), **Dan** (data space architect), **Oleg** (data space operator), **Eva**
(evaluator).

<!-- ToC -->
- [Epic 1 — Getting started and round-trip](#epic-1--getting-started-and-round-trip)
- [Epic 2 — Role presets](#epic-2--role-presets)
- [Epic 3 — Cross-cutting foundations](#epic-3--cross-cutting-foundations)
- [Epic 4 — Integrating with what is already there](#epic-4--integrating-with-what-is-already-there)
- [Epic 5 — Features and their configuration](#epic-5--features-and-their-configuration)
- [Epic 6 — Validation and review](#epic-6--validation-and-review)
- [Epic 7 — Output and handover](#epic-7--output-and-handover)
- [Epic 8 — Secrets](#epic-8--secrets)
- [Epic 9 — Changing a configuration later](#epic-9--changing-a-configuration-later)
- [Epic 10 — Installing into a cluster](#epic-10--installing-into-a-cluster)
- [Epic 11 — Non-functional](#epic-11--non-functional)
- [Release cut](#release-cut)
<!-- /ToC -->

## Epic 1 — Getting started and round-trip

**US-1.1 — Start from nothing** · M · it1
As Pia, I want to start a configuration without knowing the chart, so that I can get to a
working `values.yaml` without reading 4000 lines first.
- The first screen offers new, open a values file, open a profile, and a local trial.
- A new configuration asks for the target before anything else.
- No question is asked that the selected role and features do not need.

**US-1.2 — Open an existing values.yaml** · M · it1
As Pia, I want to open the `values.yaml` we already run, so that I can change it with the tool
instead of by hand.
- Every key is classified as owned, recognized or unknown.
- Recognized toggles and cross-cutting values appear filled in on their stages.
- Unknown keys are preserved and shown under Expert / Overrides.
- The load report says what was understood and what was carried over untouched.

**US-1.3 — Never lose anything** · M · it1
As Pia, I want to be sure the tool does not eat the parts of the file it does not understand,
so that I dare use it on our production values.
- Loading and saving a file without changes produces a byte-identical file apart from the
  generated header.
- Comments, key order and anchors survive the round-trip.
- This is tested against every profile in [`k3s/`](../../k3s).

**US-1.4 — Save and resume** · M · it1
As Dan, I want to save a half-finished configuration and continue tomorrow, so that I can go
ask the Operator for the missing values.
- A profile holds answers, which questions are deliberately unanswered, and the chart version.
- Reopening it restores the exact state, including the open-question markers.
- Open questions are listed on Review with a link to the field.

**US-1.5 — Try it locally** · S · it1
As Eva, I want to see a whole data space running on my laptop, so that I can judge whether
this is what we need.
- One choice on the start screen produces the Quick Start configuration (trust anchor,
  provider, consumer).
- The tool states its prerequisites (a local cluster, resources) before generating.
- It links to the [Quick Start guide](../deployment-integration/local-deployment/LOCAL.MD)
  rather than duplicating it.

## Epic 2 — Role presets

**US-2.1 — Pick a role** · M · it1
As Dan, I want to pick what our organization does in the data space, so that the right
components are selected without me mapping them myself.
- Consumer, Provider, Consumer + Provider, Operator, Custom.
- Each card shows what it enables and the measured memory and CPU floor for that role.
- Operator switches the target chart to `trust-anchor`.
- The preset matches the component matrix in
  [roles/README.md](../deployment-integration/roles/README.md), asserted by a test.

**US-2.2 — Overrule the preset** · M · it1
As Pia, I want to turn a recommended feature off and an unrecommended one on, so that the
preset is a starting point and not a cage.
- Every feature can be toggled regardless of the role.
- A feature that differs from the preset is marked as a deliberate choice.
- Turning off something the role requires warns, with the consequence, and allows it.

**US-2.3 — Change the role later** · S · it1
As Dan, I want to go from Consumer to Consumer + Provider, so that we can start offering data
without redoing the configuration.
- The preset is re-applied only to features the user never touched.
- Features the user set by hand keep their state and are listed as kept.
- The change summary names every feature that went on or off.
- Configuration of features that stay on is untouched.

## Epic 3 — Cross-cutting foundations

**US-3.1 — One domain, everywhere** · M · it1
As Pia, I want to enter our domain once, so that I do not have to find 41 places in a values
file.
- The base domain fans out to every host, TLS secret name, certificate annotation and
  public URL.
- The field shows how many values it feeds and lists them on demand.
- Changing it re-renders all of them and shows the diff.

**US-3.2 — Our identity, once** · M · it1
As Dan, I want to give the organization's DID once, so that every component that needs it
agrees.
- `did:web` derived from the base domain by default, `did:key` offered for development.
- Used by the verifier, Keycloak, the registration job, EDC, the IdentityHub and the data
  space configuration document.
- Production environments warn when `did:key` is selected.

**US-3.3 — The data space's parameters** · M · it1
As Dan, I want to enter what the Operator sent us, so that we are configured for *this* data
space.
- Trust Anchor TIR address, the credential types, whether registration is automatic.
- The TIR address is checked for reachability, with the check skippable.
- Where a data space publishes `/.well-known/data-space-configuration`, the values can be
  imported from that URL.
- Credential types entered here pre-seed the issuance and verification pages.

**US-3.4 — Override one host** · S · it1
As Pia, I want the verifier on a different domain than the rest, so that our DNS layout is
respected.
- Each derived host is individually editable.
- An edited host is pinned and not re-derived when the base domain changes; the review says so.

**US-3.5 — Size it for production** · S · it1
As Pia, I want resource requests and limits that are not nonsense, so that the JVM components
do not size their heap from a 64 GiB node.
- A resource profile sets requests and limits on Scorpio, the TMForum API, the Trusted
  Issuers List, contract-management and Keycloak.
- The production profile warns about any enabled Java component without a memory limit.
- The guidance is linked, not restated.

**US-3.6 — Proxy and private CA** · C · it1
As Pia, I want to declare our egress proxy and our private CA once, so that every component
that calls out is configured for them.
- The proxy fans out to the verifier, Keycloak, the marketplace, the vc-operator and APISIX,
  with `NO_PROXY` pre-filled with the in-cluster names.
- The private CA fans out to each component's own mechanism, per the table in the
  [prerequisites](../deployment-integration/roles/PREREQUISITES.md#private-ca).
- Disabling certificate verification is not offered.

## Epic 4 — Integrating with what is already there

**US-4.1 — Reuse an existing cert-manager** · M · it1
As Pia, I want the connector to use the cert-manager we already run, so that two releases do
not fight over cluster-wide resources.
- Three modes per dependency: install with the connector, already in the cluster, external.
- *Already in the cluster* asks only for the issuer to reference.
- The subchart is disabled accordingly in the generated values.

**US-4.2 — Reuse an existing database** · M · it1
As Pia, I want to point the components at our managed PostgreSQL, so that we do not run
databases in Kubernetes.
- Per-component database, user and Secret reference, with one-database-per-component or a
  shared database.
- The operator and the managed cluster are both disabled.
- Scorpio's PostGIS requirement is stated at the point of entry.
- The same for MongoDB when the local marketplace is enabled.

**US-4.3 — Detect what is in the cluster** · S · it1
As Pia, I want the tool to look at our cluster, so that I do not have to tell it what it can
see for itself.
- With a kubeconfig, the tool reports cert-manager and its issuers, the Postgres and MongoDB
  operators, ingress classes, storage classes, OTel CRDs and existing connector releases.
- Detection pre-selects a mode; every pre-selection stays editable.
- Detection is read-only and the screen says so.
- Without a kubeconfig, every mode is offered with nothing pre-selected.

**US-4.4 — Help me install what is missing** · S · it1 (recommend) / it2 (run)
As Pia, I want a recommendation when a prerequisite is missing, so that I know what to do
next.
- A missing dependency shows the upstream chart, the pinned version and the exact command,
  copyable.
- It explains why it belongs outside this release (cluster-wide ownership).
- Iteration 2 offers to run it as part of the install.

**US-4.5 — Everything external** · S · it1
As Pia, I want to run the connector against infrastructure that is not in this cluster at all,
so that it fits our platform.
- Endpoints and Secret names for PostgreSQL, MongoDB, Vault, the OTLP collector and an
  existing Keycloak.
- No credential is stored by the tool.
- Every referenced Secret appears in the handover document.

**US-4.6 — One cluster, several participants** · C · it1
As Pia, I want to configure a second connector in a cluster that already has one, so that our
test environment holds provider and consumer.
- The tool detects the existing release and refuses to let the new one own the cluster-wide
  operators, with the reason.
- Namespace and release name are suggested free.

## Epic 5 — Features and their configuration

**US-5.1 — Turn features on and off** · M · it1
As Dan, I want to see what the connector can do in my language, so that I can choose without
knowing component names.
- Features are grouped by capability, each with one line of what it gives you.
- Each shows whether it is required, recommended or optional for the chosen role.
- The chart keys behind a feature are visible on demand, not by default.

**US-5.2 — Dependencies resolved for me** · M · it1
As Dan, I want the tool to pull in what a feature needs, so that I do not produce a
configuration that cannot start.
- Enabling a feature enables what it requires and says which and why.
- Disabling one that others need asks and offers to disable them too.
- Configuration of a disabled feature is kept, in case it comes back.
- A feature needing an infrastructure dependency that is off sends the user to stage 4.

**US-5.3 — Only configure what is on** · M · it1
As Pia, I want to be asked nothing about features we do not use, so that the configuration
stays short.
- Stage 6 has a page per enabled feature and none for the rest.
- A disabled feature contributes no keys to the output.
- A feature left unconfigured uses chart defaults and is listed as such on Review.

**US-5.4 — See what is inherited** · M · it1
As Pia, I want to see which fields come from the foundations, so that I know what one edit
changes.
- Inherited fields show their value, their source and an override link.
- An overridden field is marked and excluded from later re-renders.
- Overrides can be reset to inherited.

**US-5.5 — Configure credential issuance without learning Keycloak** · M · it1
As Dan, I want to declare which credentials we issue and to whom, so that the realm is right.
- A table of credential types, pre-seeded from the data space's types, with format, claims
  and who receives them.
- Groups, users and wallet presets as options, not as a realm JSON.
- The keystore-path constraint of Keycloak ≥ 26.6.4 is enforced, not explained after it fails.

**US-5.6 — Policies and protected services** · S · it1
As Pia, I want to declare the services APISIX protects and the credentials each requires, so
that the gateway routes and the verifier registration agree.
- One table of services; each row produces the APISIX route and the verifier's service
  registration.
- Scorpio and the TMForum API appear automatically when their features are on.
- Own services can be added by in-cluster address.

**US-5.7 — The expert escape hatch** · M · it1
As Pia, I want to set a chart key the tool has no question for, so that it is never a dead
end.
- Each feature page has an Advanced section generated from `values.schema.json`, typed and
  documented.
- A global Expert YAML editor edits the generated file directly.
- Anything set there becomes an override: preserved verbatim, never re-rendered.

**US-5.8 — The Operator role** · S · it1/it3
As Oleg, I want to configure the trust infrastructure of the data space, so that participants
can be onboarded.
- The Operator role configures the `trust-anchor` chart: TIR/TIL, its database, its hosts.
- Consent authority and onboarding are features of that role.
- The same foundations, infrastructure modes and validation apply.

## Epic 6 — Validation and review

**US-6.1 — Tell me it is wrong before I install** · M · it1
As Pia, I want the configuration checked, so that I do not learn about a mistake from a
CrashLoopBackOff.
- Layer 1 validates against the chart's `values.schema.json`.
- Layer 2 applies the cross-field rules of the
  [model](FEATURE_MODEL.md#validation-rules).
- Layer 3 renders with `helm template` against the pinned chart version.
- Errors block generation; warnings do not.

**US-6.2 — Take me to the problem** · M · it1
As Pia, I want a finding to tell me what to change, so that I am not hunting in YAML.
- Every finding names the stage and field that caused it and links there.
- Every finding states the consequence, and links to the guide instead of restating it.
- A rendering failure names the template and the line.

**US-6.3 — Leave a question open** · S · it1
As Dan, I want to mark a question as "ask the Operator", so that I can finish the rest.
- A field can be marked open; the stage shows a count.
- Open questions are listed on Review and written into the handover document.
- Generating with open questions is allowed and the file carries them as TODO comments.

**US-6.4 — Preflight against the cluster** · S · it2
As Pia, I want the cluster checked against the configuration, so that an install does not fail
on something knowable.
- Storage class, ingress class, CRDs, referenced Secrets, allocatable memory against the
  role's floor.
- A check that the public hostnames resolve to the ingress **from inside a pod**.
- Every check is skippable, with a recorded reason.

## Epic 7 — Output and handover

**US-7.1 — A values.yaml we can put in git** · M · it1
As Pia, I want a readable file, so that it survives review.
- Grouped and commented, deterministic ordering, stable between runs with equal input.
- A header naming the tool, its version, the chart version and the profile hash.
- No key emitted for a question the user never answered — the chart's default stays in force.

**US-7.2 — The profile** · M · it1
As Pia, I want the answers saved next to the values, so that whoever edits it next has the
context.
- `.dscprofile.yaml`, human-readable, git-friendly, no secrets.
- It records which chart version and which model bundle it was built with.

**US-7.3 — Tell me what is left to do** · M · it1
As Pia, I want the manual steps written down, so that nothing is forgotten between the file
and a running connector.
- `NEXT_STEPS.md` lists the Secrets to create with the exact keys, the DNS records, the
  registration to request from the Operator, the prerequisites to install, and the
  `helm upgrade --install` command for this configuration.
- Ordered, each step verifiable.

**US-7.4 — Never write a secret value** · M · it1
As Pia, I want credentials to stay out of the generated files, so that they can be committed.
- The generated `values.yaml`, the profile and `NEXT_STEPS.md` carry Secret names, key names
  and fingerprints — never values.
- A generated value reaches the cluster or a `SealedSecret` and the reveal-once dialog, nothing
  else; how it is created is [Epic 8](#epic-8--secrets).
- The profile and the values file are asserted free of secret material by a test.

**US-7.5 — Export for GitOps** · C · it4
As Pia, I want the output as a change to our deployment repository, so that it goes through
our normal process.
- Values per environment, a branch and a pull request instead of a local file.

## Epic 8 — Secrets

The model behind these stories is in [Secret handling](SECRETS.md).

**US-8.1 — Choose how a secret gets there** · M · it1
As Pia, I want to decide per credential who creates it, so that the tool fits the way our
platform handles secrets.
- Five delivery modes: let the chart generate it, reference one by name, generate and apply it,
  seal it for Git, or point at the External Secrets Operator.
- A global default from the environment and the target, overridable per Secret.
- A mode the environment cannot support is offered disabled, with the reason.
- Iteration 1 implements `chart` and `reference`; the others are in the model from the start.

**US-8.2 — One owner per secret** · M · it1
As Pia, I want the chart and the tool never to create the same Secret, so that an upgrade does
not fight what I applied.
- Choosing a tool-owned mode sets the matching `generatePasswords` flag to `false` in the same
  render.
- The same Secret name claimed twice is an error.

**US-8.3 — Warn me where chart-generated passwords break** · M · it1
As Pia, I want to be told that `lookup` does not work on my deployment path, so that I do not
ship a Secret that is re-randomised on every sync.
- `generatePasswords: true` with a `helm template`, ArgoCD or Flux target is an error, not a
  warning.
- The message names the three ways out and links to the chart's
  [secret management guide](../../charts/README.md#secret-management).
- The same check covers the marketplace's three MongoDB passwords.

**US-8.4 — Generate a credential** · M · it2
As Pia, I want the tool to invent the passwords nobody needs to choose, so that I do not paste
`changeme` into a production cluster.
- Values come from a CSPRNG; passwords are 32 characters from a shell- and URL-safe alphabet;
  keys are generated per algorithm.
- A generated value is shown exactly once, with copy and save-to-a-file-I-choose, and the dialog
  says so before generating.
- Re-rendering never re-generates; regeneration is an explicit rotation with its consequences
  named.

**US-8.5 — Apply secrets to the connected cluster** · M · it2
As Pia, I want the tool to create the Secrets in our cluster, so that the connector can start
without a second tool.
- An explicit action that first lists every Secret by name, namespace and keys — never values.
- Server-side apply after a dry run, field manager `dsc-config`.
- An existing Secret is never overwritten silently: the tool reports which keys differ and asks
  per Secret.
- A missing RBAC permission is reported before anything is applied, not half-way through.

**US-8.6 — Seal secrets for Git** · S · it2
As Pia, I want encrypted Secrets I can commit, so that our GitOps repository stays the source of
truth.
- Sealed Secrets is detected: the CRD, the controller and its certificate, with the namespace
  and the controller name overridable.
- The certificate is fetched from the controller, from `kubeseal --fetch-cert`, or from a PEM
  file for air-gapped use.
- Sealing happens in process; no `kubeseal` binary is required.
- `strict` scope by default; the other scopes are offered with their consequence stated.
- Output is `sealed-secrets.yaml` in the export bundle.

**US-8.7 — Quiet diffs** · S · it2
As Pia, I want a re-export not to churn every sealed value, so that a diff shows what actually
changed.
- An unchanged plaintext keeps its existing ciphertext.
- A re-seal happens only when the value, the scope, the Secret name or the namespace changed —
  and the review says which of those it was.
- Changing the namespace or the Secret name invalidates strict SealedSecrets; the tool detects
  it, re-seals, and says why.

**US-8.8 — Use our secret store** · C · it2
As Pia, I want `ExternalSecret` resources instead of Secrets, so that Vault stays the only place
our credentials live.
- `external-secrets.io` CRDs and the available `ClusterSecretStore`s are detected and offered.
- The tool emits the store reference, the remote key per Secret key and a refresh interval.
- The tool never reads or writes the store itself.

**US-8.9 — The Keycloak admin, honestly** · M · it1
As Pia, I want to know what the Keycloak admin password is and what it is good for, so that I am
not locked out and not falsely reassured.
- The tool states that the chart generates it today, and shows the command that reads it back.
- It states that `KC_BOOTSTRAP_ADMIN_*` applies only while the realm has no administrator, so
  changing the Secret later does not change the password.
- Asking to rotate it offers the path that works — a new admin through the admin API, then the
  old account removed — instead of editing the Secret.
- The admin user name is asked, not defaulted silently.
- In a production environment the account is marked temporary, and `NEXT_STEPS.md` carries the
  follow-up to create a named administrator and delete it.

**US-8.10 — Tell me which secrets are missing** · M · it1
As Pia, I want to know before installing which Secrets do not exist yet, so that the install does
not fail on a crash-looping pod.
- With a cluster target, every referenced Secret is checked for existence and for the keys the
  component reads.
- Findings appear on Review and in `NEXT_STEPS.md`, with the exact keys required.
- Without a cluster, the same list is produced from the configuration alone.

**US-8.11 — Identity material is not a password** · M · it1
As Dan, I want the DID key and the realm keystore treated as identity, so that nothing replaces
them by accident.
- Keys and certificates form their own class, never rotated as a side effect.
- Before any action that would replace one, the consequence at the Trust Anchor is stated.
- A development key can be generated; a production one has to come from cert-manager or from a
  Secret.

## Epic 9 — Changing a configuration later

**US-9.1 — Add a feature to a running deployment** · M · it1
As Pia, I want to enable consent enforcement six months after the install, so that the
connector grows with us.
- Opening a profile or a values file lands on Review.
- Turning a feature on produces a diff of the new keys only.
- The diff says what is added, what is untouched and which new Secrets are needed.
- With a cluster target, the diff is against the deployed release, not only the file.

**US-9.2 — Change a cross-cutting value** · M · it1
As Pia, I want to move the connector to a new domain, so that one edit is enough.
- Every dependent key is re-rendered and listed.
- Keys pinned by hand are listed as not updated, each resolvable individually.

**US-9.3 — Upgrade the chart version** · S · it2
As Pia, I want to move from chart 10.16.0 to 11.0.0, so that we get the new features without
re-configuring.
- The profile is migrated to the new model bundle.
- Renamed keys are migrated, removed keys reported, new features listed as available.
- Breaking changes are linked to the [release notes](../release-notes).
- The migration is shown before it is applied.

**US-9.4 — Detect hand edits** · M · it1
As Pia, I want to know when someone changed the file outside the tool, so that the tool does
not silently overwrite it.
- An owned key whose value differs from the render is reported as a conflict.
- Each conflict is resolved individually: keep the file's value (becomes an override) or take
  the profile's.
- Nothing is overwritten before the conflicts are resolved.

## Epic 10 — Installing into a cluster

**US-10.1 — Install from the tool** · M · it2
As Pia, I want to install without copying a command, so that the first deployment is one step.
- `helm upgrade --install` through the SDK into the selected context and namespace.
- Per-release progress, hook jobs' logs streamed, the failing resource named on failure.
- The equivalent command is always shown and copyable.

**US-10.2 — Upgrade an existing release** · M · it2
As Pia, I want to apply a change to what is running, so that I do not have to leave the tool.
- A diff against the deployed values before anything is applied.
- Immutable-field changes — such as the `priorityClassName` of a completed rotation Job — are
  detected and explained, with the way out, before the upgrade fails.

**US-10.3 — Verify afterwards** · S · it2
As Pia, I want the deployment checked, so that I know it works and not only that it started.
- `fdsc-verify --preflight-only` after the install, the full run on request.
- Findings shown in the tool, each linked back to the field it comes from where possible.

**US-10.4 — Uninstall what I just installed** · C · it2
As Eva, I want to remove a trial cleanly, so that my cluster is as it was.
- Only the releases the tool installed, named explicitly.
- PersistentVolumeClaims and cluster-wide resources are called out, never removed silently.

## Epic 11 — Non-functional

**US-11.1 — Three platforms, no runtime** · M · it1
One signed binary per platform for Linux, macOS and Windows; no installer, no interpreter, no
container required.

**US-11.2 — Headless** · M · it1
`dsc-config generate -p profile.yaml -o values.yaml` produces exactly what the GUI produces,
for use in CI.

**US-11.3 — Offline** · S · it1
With a chart and a model bundle on disk, the tool works with no network access; no telemetry
is sent in any mode.

**US-11.4 — Honest about the cluster** · M · it1
Until the user presses Install, no write reaches the cluster, and the UI says so where
detection happens.

**US-11.5 — Accessible** · S · it1
Keyboard-navigable throughout, labelled fields, contrast in light and dark, no meaning carried
by colour alone.

**US-11.6 — Versioned with the chart** · M · it1
The tool and its question model live in the chart's repository under `config-tool/`, the model as
data per chart version; a CI check in the same checkout fails when the model names a key the
chart does not have, or when the role presets disagree with the component matrix. The tool keeps
its own version, tagged `config-tool/vX.Y.Z`, so its releases are independent of the chart's.

## Release cut

| Milestone | Stories | Result |
|---|---|---|
| **M1 — Generate** | 1.1, 1.3, 2.1, 3.1–3.3, 5.1–5.3, 6.1, 7.1, 8.1–8.3, 8.9, 11.1, 11.2, 11.6 | A Consumer and a Provider configuration generated from scratch and installable by hand |
| **M2 — Integrate** | 1.2, 1.4, 4.1–4.3, 4.5, 5.4, 5.7, 6.2, 7.2–7.4, 8.10, 8.11 | Real clusters: existing databases, existing cert-manager, round-trip on an existing file |
| **M3 — Evolve** | 2.2, 2.3, 5.5, 5.6, 9.1, 9.2, 9.4, 6.3 | Features enabled later; cross-cutting changes safe |
| **M4 — Install** | 6.4, 8.4, 8.5, 9.3, 10.1–10.3, 4.4 | Installs and upgrades from the tool, verified |
| **M5 — Fit in** | 5.8, 7.5, 8.6–8.8, 10.4, 4.6, 3.6, 11.3, 11.5 | Operator role, GitOps, multi-participant clusters |
