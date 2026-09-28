# Review — PR #212 "Improved eIDAS2.0 support"

Branch `ticket-62/work` → `main`, 22 files, +2320/-604.

Verified locally against:

* `decentralized-iam` 2.1.23 → `vc-authentication` 1.3.9 → `vcverifier` chart 4.13.0 (appVersion 6.22.0),
  unpacked from `~/.cache/helm/repository/decentralized-iam-2.1.23.tgz`
* VCVerifier source at `~/git/fiware/VCVerifier` (`04f0af9`, tag `6.22.0-PRE-122`)
* `helm lint`, `helm template provider -f k3s/provider.yaml -f k3s/provider-eidas.yaml`,
  `helm unittest charts/data-space-connector` (12 suites / 140 tests) — all pass
* `mvn -o -f it/pom.xml test-compile` — the new step definitions compile
* the committed certificate material (see [#2](#2-the-mock-trust-list-is-committed-twice-and-the-copy-the-docs-tell-you-to-edit-is-dead))

---

## Table of contents

* [Blocking](#blocking)
  * [1. eIDAS validation never actually runs in the demo or the integration test](#1-eidas-validation-never-actually-runs-in-the-demo-or-the-integration-test)
  * [2. The mock trust list is committed twice, and the copy the docs tell you to edit is dead](#2-the-mock-trust-list-is-committed-twice-and-the-copy-the-docs-tell-you-to-edit-is-dead)
  * [3. Verifier / trust-list-mock startup race, with a one-hour recovery time](#3-verifier--trust-list-mock-startup-race-with-a-one-hour-recovery-time)
  * [4. `lotUrl` is worked around in the umbrella instead of fixed upstream](#4-lotUrl-is-worked-around-in-the-umbrella-instead-of-fixed-upstream)
  * [5. The per-credential section of the new doc is factually wrong](#5-the-per-credential-section-of-the-new-doc-is-factually-wrong)
* [Release hygiene](#release-hygiene)
  * [6. No chart version bump](#6-no-chart-version-bump)
  * [7. No release notes, and the doc references a 10.7.0 that does not exist](#7-no-release-notes-and-the-doc-references-a-1070-that-does-not-exist)
  * [8. `-Plocal,elsi` → `-Plocal,eidas` is an undocumented breaking rename](#8--plocalelsi--plocaleidas-is-an-undocumented-breaking-rename)
  * [9. No helm-unittest coverage for the new values surface](#9-no-helm-unittest-coverage-for-the-new-values-surface)
* [Code quality](#code-quality)
* [Smaller things](#smaller-things)
* [What is good](#what-is-good)

---

## Blocking

### 1. eIDAS validation never actually runs in the demo or the integration test

This is the central problem: the feature the PR is built around is not exercised by anything it adds.

`EidasValidationService.ValidateVC` (`verifier/eidas_validation.go:110`) gates on **per-credential-type**
configuration, not on the global `eidas.enabled` flag:

```go
for _, credType := range verifiableCredential.Contents().Types {
    cfg, ok := eidasContext.PerType[credType]
    if ok && cfg != nil && cfg.Enabled { activeConfig = cfg; break }
}
if activeConfig == nil {
    logging.Log().Debug("EidasValidationService: no eIDAS config enabled for credential types, pass-through")
    return true, nil          // <-- every credential in this PR takes this branch
}

if verifiableCredential.Format() != common.FormatSDJWT {
    return false, ErrorEidasSDJWTRequired   // <-- and eIDAS is SD-JWT only
}
```

Neither condition is met by the overlays:

```console
$ grep -rn "eidasConfig" k3s/ charts/
(nothing)

$ grep -n "format:" k3s/provider-eidas.yaml k3s/consumer-eidas.yaml
k3s/provider-eidas.yaml:177:   format: "jwt_vc_json"
k3s/provider-eidas.yaml:199:   format: "jwt_vc_json"
k3s/consumer-eidas.yaml:271:   format: "jwt_vc_json"
k3s/consumer-eidas.yaml:309:   format: "jwt_vc_json"
k3s/consumer-eidas.yaml:351:   format: "jwt_vc_json"
```

Consequences:

* `decentralizedIam.vcAuthentication.vcverifier.deployment.eidas.enabled: true` only starts the
  `TrustListFetcher`. It builds the trust store and then nothing consults it.
* The whole `eidas-trust-list-mock` apparatus — LOTL, national list, static web server, `NO_PROXY`
  entry — is inert. Deleting it would not change a single test outcome.
* `it/src/test/resources/it/eidas.feature` would pass with an empty trust store, with the mock
  scaled to zero, or with a certificate from an entirely unrelated CA. What the feature actually
  proves is that the JAdES plugin emits an `x5c` header and that the credential is signed by a chain
  that is internally consistent — both worth testing, neither an eIDAS trust decision.
* The doc's own hedge in *Demo Flow* step 8 gives the game away: it cannot demonstrate eIDAS biting,
  so it falls back to presenting `verifiable-credential`, which is rejected by the
  **trusted-issuers list**, not by trust-list validation.

**Fix.** Two things have to change together, because eIDAS validation is SD-JWT only:

1. Switch the eIDAS credentials to `dc+sd-jwt` in `k3s/consumer-eidas.yaml` and the matching DCQL in
   `k3s/provider-eidas.yaml`. Per this PR's own JAdES table, Keycloak already writes `x5c` for
   SD-JWT, so the plugin is not strictly required for the chain — though keeping it gives the
   signing time.
2. Set `eidasConfig` on the credential in the verifier's registration block. The vcverifier chart
   passes `oidcScopes` through verbatim (`templates/registration-cm.yaml:57-63`,
   `$body | toJson`), so no upstream change is needed:

   ```yaml
   # k3s/provider-eidas.yaml
   registration:
     services:
       - id: data-service
         oidcScopes:
           "default":
             credentials:
               - type: UserCredential
                 trustedParticipantsLists: [ ... ]
                 trustedIssuersLists: [ ... ]
                 eidasConfig:
                   enabled: true
                   # the mock lists the test CA under DE
                   allowedCountries: [ "DE" ]
                   # defaults to true; the test leaf does carry a QcCompliance
                   # qcStatements extension, so true should hold - verify before pinning
                   requireQualified: true
   ```

   `requireQualified` defaults to **true** when absent (`config/configClient.go:152`), and it makes
   `requireQualifiedCertificate` demand a `QcCompliance` statement on the leaf. The committed test
   leaf does carry `qcStatements`, so this should pass, but it is worth asserting rather than
   assuming.
3. Once that is in place, add a negative scenario that actually isolates the trust list — e.g. point
   `lotlUrl` at a list that does not carry the test CA, or drop the `<TSPService>`, and assert the
   token exchange fails with `certificate does not chain to any trusted service`. Without a negative
   case the suite still cannot distinguish "validated" from "skipped".
4. `eidasConfig` also belongs in `doc/deployment-integration/eidas/README.md` as the **required**
   step, not as an optional refinement — see [#5](#5-the-per-credential-section-of-the-new-doc-is-factually-wrong).

### 2. The mock trust list is committed twice, and the copy the docs tell you to edit is dead

`k3s/eidas-mock/lotl.xml` and `k3s/eidas-mock/tl-de.xml` are added as standalone files, and the same
XML is inlined a second time into `k3s/provider-eidas.yaml`'s `extraManifests` ConfigMap
(lines ~516-683).

Nothing consumes the standalone files:

```console
$ grep -rn "eidas-mock" --include=*.xml --include=*.yaml --include=*.yml --include=*.md . \
    | grep -v '^./k3s/provider-eidas.yaml'
doc/deployment-integration/eidas/README.md:487:| [k3s/eidas-mock/lotl.xml](...) | List of Trusted Lists ...
doc/deployment-integration/eidas/README.md:488:| [k3s/eidas-mock/tl-de.xml](...) | National list (territory DE) ...
doc/deployment-integration/eidas/README.md:650:sed -n '/<X509Certificate>/,/<\/X509Certificate>/p' k3s/eidas-mock/tl-de.xml \
doc/deployment-integration/eidas/README.md:662:[k3s/eidas-mock/tl-de.xml](...). If they differ, ...
```

No `pom.xml` resource copy, no ConfigMap generator, no kustomize reference — only doc links. And the
doc instructs the reader to edit exactly the copy that has no effect:

> After regenerating the test material, the certificate in `tl-de.xml` has to be replaced with the
> new root CA

Doing that changes nothing about the deployment. The reader then hits the very failure the doc warns
about (`certificate does not chain to any trusted service`) with the doc's own remedy already
applied — a genuinely nasty trap.

Both copies currently agree (SHA-256 `7E:5C:E3:…:22:2B`), so this is latent, not broken today.

**Fix.** Pick one source of truth. The cleanest is to keep `k3s/eidas-mock/*.xml` as the source and
drop the inline copy, loading the files through the Maven overlay the way the other k3s assets are
loaded (`copy-resources-*` executions in `pom.xml`). If self-containment of the overlay is the
priority, delete `k3s/eidas-mock/` and repoint every doc reference at
`k3s/provider-eidas.yaml`. Either way, one copy.

### 3. Verifier / trust-list-mock startup race, with a one-hour recovery time

`eidas.NewTrustListFetcher(...).Start(ctx)` does one initial fetch. On failure it logs and then
simply waits for the next tick — there is no backoff and no early retry
(`eidas/fetcher.go:230-249`). `k3s/provider-eidas.yaml` pins `refreshInterval: 3600`, which the
overlay's own comment acknowledges is the floor VCVerifier will accept.

The verifier Deployment and the `eidas-trust-list-mock` Deployment are created by the same Helm
release and start concurrently. Nothing sequences them — the verifier's `initContainers` are
`add-root-ca` and `register-at-tir` only, and the `waitForHost` probe on the registration job waits
for the *verifier*, not for the mock.

So if the mock's pod is not serving by the time the verifier boots, the trust store stays empty for
a full hour. In CI that is not a flake, it is a hard failure with a confusing signature: the verifier
is `Ready`, `/health` is green, and credentials are silently rejected.

This is masked today by [#1](#1-eidas-validation-never-actually-runs-in-the-demo-or-the-integration-test)
— an empty trust store is never consulted. Fixing #1 makes this live.

**Fix.** Add a wait to the verifier's init containers in `k3s/provider-eidas.yaml`:

```yaml
- name: wait-for-trust-list
  image: curlimages/curl:8.18.0
  command: [ "/bin/sh" ]
  args:
    - -ec
    - |
      until curl -sf -o /dev/null http://eidas-trust-list-mock:3000/lotl.xml; do
        echo "waiting for the trust list mock"; sleep 2
      done
```

Note this must be excluded from the proxy, as the mock already is in `NO_PROXY`. Longer term,
a retry-with-backoff on the initial fetch belongs in VCVerifier upstream — worth an issue there.

### 4. `lotUrl` is worked around in the umbrella instead of fixed upstream

> **Resolved.** Fixed upstream in `decentralized-iam` 2.1.25 (vcverifier subchart 4.15.3), which
> renames the key to `lotlUrl`. Chart 10.9.0 pins 2.1.25 and the workaround notes are gone.
> Note there is no backward-compatible alias, so a values file still setting `lotUrl` is now
> silently ignored — recorded under Breaking changes in the release notes.


The vcverifier subchart declares the key as `lotUrl`:

```yaml
# vcverifier 4.13.0, values.yaml:151
  eidas:
    # -- URL of the List of Trusted Lists. Defaults to the official EU LOTL
    lotUrl:
```

VCVerifier reads `lotlUrl`:

```go
// config/config.go:274
LotlURL string `mapstructure:"lotlUrl"`
```

The subchart's configmap template just dumps the map (`{{- toYaml . | nindent 6 }}`), so anything
set under `eidas` reaches the config file verbatim. The PR exploits that by setting `lotlUrl` from
the umbrella and documenting the discrepancy in three places:

* `charts/data-space-connector/values.yaml` — *"VCVerifier reads `lotlUrl`, while the vcverifier
  subchart declares it as `lotUrl`, which the server ignores"*
* `k3s/provider-eidas.yaml:110` — *"spelled lotlUrl, not lotUrl - that is the key VCVerifier
  actually reads"*
* `doc/.../eidas/README.md` configuration reference — same note again

This is a workaround for an upstream chart bug, carried in the umbrella. The house rule is to fix
upstream instead.

(Mechanically it does work — `helm template` confirms only `lotlUrl` reaches the rendered configmap,
because Helm's coalescing drops the subchart's nil-valued `lotUrl`. That makes it a maintenance and
correctness-of-documentation problem, not a runtime one.)

**Fix.** Open a PR against `FIWARE/helm-charts` renaming `lotUrl` → `lotlUrl` in the vcverifier
chart (keeping `lotUrl` as a deprecated alias if anyone might already be setting it), bump the
dependency, and delete all three notes. If the bump cannot land in this PR, keep the workaround but
replace the three explanatory notes with a single `TODO` linking the upstream issue, so the
workaround has an expiry date.

### 5. The per-credential section of the new doc is factually wrong

`doc/deployment-integration/eidas/README.md`, *Per-Credential eIDAS Validation (SD-JWT)*:

> VCVerifier applies the eIDAS trust list validation specifically to that credential type's issuer
> certificate chain, **even if the global `eidas` block is disabled**.

The opposite is true. From `config/config.go:262`:

> When Enabled is false (the default), the trust list fetcher is not started (no background
> goroutines, no HTTP requests, no memory for the trust store), and **any per-credential eIDAS
> configuration is rejected at config validation time with HTTP 400**.

The section also reads as an optional refinement (*"This allows requiring eIDAS validation only for
specific credential types rather than applying it globally"*) when in fact `eidasConfig` is the
**only** way any eIDAS validation ever happens — there is no global mode. That framing is what
allowed [#1](#1-eidas-validation-never-actually-runs-in-the-demo-or-the-integration-test) through.

**Fix.** Rewrite the section as a required step:

* global `eidas.enabled: true` starts the trust-list fetcher and is a **precondition**;
* `eidasConfig.enabled: true` per credential type is what makes validation run;
* per-credential config with the global block disabled is rejected with HTTP 400;
* validation is **SD-JWT only** — a `jwt_vc_json` credential with `eidasConfig` enabled is rejected
  outright with `ErrorEidasSDJWTRequired`, it is not silently skipped. That constraint deserves to be
  called out at the top of the document, not buried in a section title.
* document `allowedCountries` and `requireQualified` (default `true`), which the reference table
  omits entirely.

Everything else I spot-checked in the document holds up:

| Claim | Verdict |
|---|---|
| `refreshInterval` clamped to `[3600, 604800]` | correct (`config/config.go:277`) |
| VCVerifier does not verify XMLDSig on trust lists | correct (`eidas/fetcher.go:132`, `eidas/trustlist.go:10`) |
| `statusEvaluation` current/issuance | correct and wired (`verifier/eidas_validation.go:277`) |
| Startup succeeds with an empty trust store | correct |
| Quoted `TrustListFetcher:` log lines | verbatim matches (`eidas/fetcher.go:230,290,296`) |
| Default LOTL URL | correct (`config/provider.go:14`) |

---

## Release hygiene

### 6. No chart version bump

`charts/data-space-connector/Chart.yaml` is `version: 10.8.0` on both `main` and this branch. The PR
adds a whole new values block and deprecates `decentralizedIam.vcAuthentication.dss` — additive
feature plus deprecation, which per the repo convention is a minor bump.

**Fix.** Bump to `10.9.0`.

### 7. No release notes, and the doc references a 10.7.0 that does not exist

`doc/release-notes/10-x.md` is untouched by this PR, and contains no eIDAS entry at all — the
10.7.0 section that commit `e9a9b2e` added was lost somewhere in the merge/`clean` history. Its
headings today go `## 10.8.0 …` then straight to `## 10.4.0 …`.

Meanwhile the new eIDAS document leans on that missing section:

* *"chart versions < 10.7.0"* (Migration section)
* the **Before / After** table is keyed `Before (< 10.7.0)` / `After (>= 10.7.0)`
* *"included in chart version 10.7.0 and later"*

All three are wrong regardless — the feature lands in 10.9.0 (see [#6](#6-no-chart-version-bump)).

**Fix.** Add a `## 10.9.0 — eIDAS 2.0 trust list validation` section to `doc/release-notes/10-x.md`
covering: the new `eidas` values block, the `dss` deprecation, the `elsi` → `eidas` profile and
overlay rename, and the `eidasConfig` + SD-JWT requirement from
[#1](#1-eidas-validation-never-actually-runs-in-the-demo-or-the-integration-test). Then replace every
`10.7.0` in the eIDAS doc with `10.9.0`.

Per the repo's documentation rule, the switch from an external DSS service to in-verifier PKIX
validation is also an architectural decision that warrants an ADR under `doc/adr/`.

### 8. `-Plocal,elsi` → `-Plocal,eidas` is an undocumented breaking rename

`pom.xml` renames the profile, `k3s/provider-elsi.yaml` is deleted, `k3s/consumer-elsi.yaml` is
renamed. Anyone with `-Plocal,elsi` in a script or CI job gets a silent no-op (Maven does not fail on
an unknown profile id by default), and anyone with `-f k3s/provider-elsi.yaml` gets a hard error.

**Fix.** Call it out in the release notes from [#7](#7-no-release-notes-and-the-doc-references-a-1070-that-does-not-exist).

### 9. No helm-unittest coverage for the new values surface

The chart ships 12 suites / 140 tests, including `tracing_test.yaml` as precedent for "new values
block gets its own suite". The new `eidas` block gets none.

**Fix.** Add `charts/data-space-connector/tests/eidas_test.yaml` asserting at minimum:

* `eidas.enabled: false` by default, so an existing release upgrades to a no-op;
* every documented key reaches the vcverifier configmap under `eidas:`;
* `lotlUrl` (not `lotUrl`) is the rendered key — this one would have caught
  [#4](#4-lotUrl-is-worked-around-in-the-umbrella-instead-of-fixed-upstream) regressing after an
  upstream bump;
* `revocationCheck: "off"` stays a string and does not get YAML-1.1'd into `false`.

---

## Code quality

**`it/.../RunCucumberTest.java` — stale profile in the javadoc.** Says
`mvn clean integration-test -Ptest,elsi,eidas-test`; the profile is `eidas` now (`pom.xml:718`).
`EidasStepDefinitions`'s own javadoc has it right.

```diff
-  *   <li>{@code @eidas} — Tests for the eIDAS deployment
-  *       ({@code mvn clean integration-test -Ptest,elsi,eidas-test})</li>
+  *   <li>{@code @eidas} — Tests for the eIDAS deployment
+  *       ({@code mvn clean integration-test -Ptest,eidas,eidas-test})</li>
```

**`EidasStepDefinitions.setup()` — `Thread.sleep(3001)`.** Magic constant, which the project rules
forbid outright. It is copied from `StandardStepDefinitions`, so the pre-existing one should get the
same treatment. The value being `3001` rather than `3000` suggests it was never deliberate. The
class already uses Awaitility elsewhere, which is the better tool here — poll the verifier's config
endpoint until the new trusted-issuer entry is visible, instead of sleeping.

```java
/** The verifier polls its credentials config on this cadence, so a new TIL entry needs one cycle. */
private static final Duration VERIFIER_CONFIG_REFRESH = Duration.ofSeconds(4);
```

**`registerElsiIssuerAtTil()` — the response code is logged, never asserted.**

```java
try (Response response = HTTP_CLIENT.newCall(create).execute()) {
    log.debug("Registered the did:elsi issuer - code {}", response.code());
}
```

A 4xx/5xx here surfaces much later as an opaque "no access token" in an unrelated scenario. Assert
`2xx` (or `2xx`/`409`, matching how the chart's own registration job treats an existing entry).

**Scenario name overpromises.** *"The certificate chain terminates in a CA the verifier trusts."*
maps to `certificateChainIsComplete`, which asserts the chain length is 3 and that the leaf verifies
against the intermediate. It never touches the trust list or the root. Either rename it to
*"The credential ships a complete certificate chain."* or extend the step to load the root from the
trust list and verify against it — the latter is the more valuable assertion, and the doc already
spells the shell equivalent out in *Demo Flow* step 4.

**Mock pod does not meet the chart's pod-security bar.** `k3s/provider-eidas.yaml:707-715` sets
`allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true` and drops all capabilities, but
omits `runAsNonRoot` / `runAsUser` and `seccompProfile`, which the project requires of any new
container. There are also no resource requests/limits and no probes, so on a loaded CI runner the
pod competes unbounded with the rest of the stack — which feeds
[#3](#3-verifier--trust-list-mock-startup-race-with-a-one-hour-recovery-time).

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  seccompProfile:
    type: RuntimeDefault
  capabilities:
    drop: [ ALL ]
resources:
  requests: { cpu: 10m, memory: 16Mi }
  limits:   { memory: 32Mi }
readinessProbe:
  httpGet: { path: /lotl.xml, port: 3000 }
```

(`lipanski/docker-static-website` runs as an unprivileged user by default, so this should be a
no-op at runtime — but it makes the intent explicit and survives an image bump.)

**`EidasStepDefinitions.setup()` skips `OBJECT_MAPPER.setSerializationInclusion(NON_EMPTY)`.**
`StandardStepDefinitions.setup()` sets it on the shared `protected static` mapper in
`StepDefintions`; the eIDAS hook does not, so `registerElsiIssuerAtTil` serializes
`{"credentialsType":"UserCredential","claims":[]}` rather than omitting the empty list. Harmless
against the current TIL, but the two hooks should agree — and mutating a shared static mapper from a
`@Before` is fragile in general. Worth setting the inclusion on the mapper once in `StepDefintions`.

**Leftover `dss:` blocks.** Both `k3s/provider-eidas.yaml:226` and `k3s/consumer-eidas.yaml:31`
still carry `dss: { enabled: false, crl: { enabled: false } }` under a comment saying it is no longer
needed. `enabled: false` is already the chart default — just delete the blocks and keep the comment,
or delete both.

---

## Smaller things

**Role docs still say ELSI.** `doc/.../roles/{consumer,provider}/README.md` keep the overlay labelled
**ELSI** and linked to `github.com/FIWARE/elsi` while pointing at `*-eidas.yaml`. Relabel to
**eIDAS / ELSI** now that the file name changed.

**Certificate-generation example does not match the committed material.** The doc's sample config is
`ORGANISATION="Test org"`, `COMMON_NAME="Test"`, and the sample subject output is
`O=Test org, CN=Test, …/organizationIdentifier=VATDE-1234567`. The keystore actually committed in
`k3s/consumer-eidas.yaml` has:

```
subject=C=DE, ST=Saxony, L=Lau..nitz, O=M&P Operations Inc., CN=M&P Ops,
        emailAddress=me@mp-operations.org, serialNumber=03,
        organizationIdentifier=VATDE-1234567
```

Only the `organizationIdentifier` matches. A reader who follows the doc literally regenerates a
different CA and then has to remember to re-embed it in the trust list — compounded by
[#2](#2-the-mock-trust-list-is-committed-twice-and-the-copy-the-docs-tell-you-to-edit-is-dead).
Either align the sample config with the committed material or say plainly that the values are
illustrative and only `ORGANISATION_IDENTIFIER` is load-bearing.

**The committed test leaf expires 2030-01-12** (intermediate 2032-04-16, root 2035-01-11). The CI
job will start failing then with no obvious pointer to the cause. A note next to the keystore in
`k3s/consumer-eidas.yaml` is cheap insurance.

**`EidasEnvironment` is an `abstract class` holding only static members.** `final` with a private
constructor expresses the intent better — though it does match `MPOperationsEnvironment` and
`TrustAnchorEnvironment`, so consistency arguably wins. Leave it if the others stay as they are.

**Unrelated changes to the Gaia-X overlays.** `k3s/provider-gaia-x.yaml` (+36/-13) and
`k3s/consumer-gaia-x.yaml` (+9/-4) carry a `did-provider.127.0.0.1.nip.io` → `mp-operations.org`
rename, `traefik.ingress.kubernetes.io/router.tls` annotations, an `frontendUrl`/`server.host` port
fix, the `requestMode`/`supportedModes` pinning, and the APISIX `routes: |-` → `routes:` fix. They
all look correct, and they align the overlay with the convention `main`'s `k3s/provider.yaml`
already uses — but they are drift repairs, not eIDAS work, and the CI matrix has no `gaia-x` entry,
so nothing verifies them. Either split them into their own PR or call them out explicitly in the
description so a reviewer knows to look.

**The Rollback section cannot work as written.** It says to *"Restore the `dss:` block and
`verifier.elsi.validationEndpoint`"*, but `validationEndpoint` no longer exists anywhere in the
bundled charts:

```console
$ grep -rn "validationEndpoint" charts/ k3s/          # nothing
$ grep -rn "validationEndpoint" <unpacked decentralized-iam 2.1.23>   # nothing
```

vcverifier 4.13.0 dropped the key, so rolling back to DSS requires pinning an older
`decentralized-iam` as well. Say that explicitly, or drop the Rollback section — an instruction that
silently does nothing is worse than none.

---

## What is good

Worth saying, because a lot here is careful work:

* The inline comments in the overlays are the useful kind — they record *why* (`frontendUrl` without
  the port, `NO_PROXY` for the trust-list host because squid re-terminates TLS, the init-container
  ordering and the `fsGroup` bit that forces it, `routes` as a list not a block scalar). That is
  exactly the knowledge that otherwise evaporates.
* The security warnings around the mock trust list are prominent and correctly placed — in the XML
  header, in the overlay, and in the doc.
* The `extraManifests` namespace note added to `values.yaml` is a good generalisation of a trap that
  bit this work, and it benefits every future user of that key.
* Chain material is internally consistent: the root in `k3s/eidas-mock/tl-de.xml` is byte-identical
  to the root of the keystore in `k3s/consumer-eidas.yaml` (SHA-256 `7E:5C:…:22:2B`), and
  `openssl verify -CAfile root -untrusted intermediate leaf` returns `OK`.
* Every factual claim I checked in the new document against VCVerifier's source held up, apart from
  [#5](#5-the-per-credential-section-of-the-new-doc-is-factually-wrong) — including the quoted log
  lines, the clamp bounds and the XMLDSig limitation.
* `helm lint`, `helm template` with the new overlay, `helm unittest` (140 tests) and
  `mvn test-compile` on the IT module all pass.

---

## Suggested order of work

1. [#1](#1-eidas-validation-never-actually-runs-in-the-demo-or-the-integration-test) —
   `eidasConfig` + SD-JWT + a negative scenario. Everything else is cosmetic until the feature is
   actually exercised.
2. [#3](#3-verifier--trust-list-mock-startup-race-with-a-one-hour-recovery-time) — the wait-for init
   container, which #1 turns from latent into load-bearing.
3. [#2](#2-the-mock-trust-list-is-committed-twice-and-the-copy-the-docs-tell-you-to-edit-is-dead) —
   collapse the duplicated trust list to one copy.
4. [#5](#5-the-per-credential-section-of-the-new-doc-is-factually-wrong),
   [#6](#6-no-chart-version-bump), [#7](#7-no-release-notes-and-the-doc-references-a-1070-that-does-not-exist),
   [#8](#8--plocalelsi--plocaleidas-is-an-undocumented-breaking-rename) — docs, version, release
   notes, ADR.
5. [#9](#9-no-helm-unittest-coverage-for-the-new-values-surface) and the code-quality items.
6. [#4](#4-lotUrl-is-worked-around-in-the-umbrella-instead-of-fixed-upstream) — upstream fix, can
   land in parallel and be picked up on the next dependency bump.
