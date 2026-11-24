# Implementation Plan: [E3 Foundations] US-3.3 — The data space's parameters (TIR address, credential types, import)

## Overview
Add the Stage 3 *Data space* panel to `dsc-config`: three new foundations (`dataspace.tirAddress`,
`dataspace.credentialTypes`, `dataspace.autoRegistration`) in `model/10.16.0/bundle.yaml`, the L2 error rule
"credential verification on ⇒ TIR address set", a core package that parses a data-space descriptor into a preview
and exposes the credential types to later stories, and two user-triggered outbound calls in the server (TIR
reachability check, descriptor import). Nothing goes on the network unless the user presses a button.

## Decisions (recorded in ADR 0022, step 7; a reviewer can override them here)

1. **Only the TIR address renders** (confirmed by the maintainer on PR #36). Its one target is
   `decentralizedIam.vcAuthentication.vcverifier.deployment.verifier.tirAddress` (`k3s/provider.yaml` line 96:
   `https://tir.127.0.0.1.nip.io/`). The other TIR leaves in provider.yaml stay excluded:
   - `decentralizedIam.vcAuthentication.vcverifier.registration.services[*].oidcScopes.*.credentials[*]`
     `.trustedParticipantsLists[*]` belong to the verifier's service registration, which is a list with content no
     foundation owns. Re-attribute these rows to **US-5.6**, which pre-seeds that list from this story's answers.
   - the verifier `initContainers[*].args[*]` curl script is a k3s-local registration at the in-cluster TIR. The
     story stays **US-3.3**, with a new reason: no chart key, see ADR 0022.
   - `registration.issuer[*].did` is the participant's local TIL list, which names other participants. Re-attribute
     to **US-5.5** ("local TIL entries", FEATURE_MODEL).
2. **`autoRegistration` and `credentialTypes` are answers only**, for two different reasons:
   - `autoRegistration`: chart 10.16.0 has no key for "register at the Trust Anchor". The chart's `registration.*`
     registers issuers in the participant's *own* TIL, which is a different thing. No chart key is invented.
   - `credentialTypes`: the chart key exists, `...registration.services[*].oidcScopes.*.credentials[*].type`
     (`k3s/provider.yaml` line 174), next to the `trustedParticipantsLists` of Decision 1. Those elements belong to
     the service registration list, which US-5.6 owns. This story does not render them; it exposes the answer for
     US-5.6 to pre-seed each element's `type` and `trustedParticipantsLists` from.

   The core exposes both answers (step 4) for US-5.5, US-5.6 and NEXT_STEPS. ADR 0022 and the `AGENTS.md` line
   give these reasons; neither says the chart lacks a key for credential types.
3. **No `dataspace.id`/DID foundation.** No chart key reads the data space's identifier in 10.16.0, so the
   ticket's "where the registration job needs it" does not apply.
4. **Descriptor shape.** The chart's `dsconfig-cm.yaml` publishes only `supported_models`, `supported_protocols` and
   `authentication_protocols`. The tool reads three more **optional** top-level keys in the same snake_case style:
   - `tir_address`: string URL.
   - `credential_types`: a list. Each element is either a string or `{type, description?}`.
   - `participant_registration`: `"automatic"` or `"manual"`.

   Other keys are ignored and listed as "not used". Publishing these keys from the chart's own `dataSpaceConfig` is
   a follow-up and out of scope here.

   **The import is forward-looking.** No publisher emits these keys yet, the chart's `dsconfig-cm.yaml` included, so
   today every real data space yields an empty preview. ADR 0022 says so. The empty-preview message says so too, so
   the user does not read it as a failure: "This data space does not publish a TIR address, credential types or a
   registration mode yet. Enter them below." The unused keys are still listed, which shows the document was read.
5. **The reachability record is session state**, like `AppState.roleChanges` (ADR 0019). It is not a profile field,
   because a reachability result is tied to one moment.
6. **Rule grammar.** A `when` may address `features.<feature id>` with `equals: "on"` or `equals: "off"`, and may
   use `unset: true` instead of `equals`. `unset: true` holds when the resolved answer, given or derived, is absent.
   A given `when` has exactly one of `equals` and `unset`.
7. **A loaded TIR address is back-derived.** `internal/load` derives answers only from `recognize:` rules, so
   without one a loaded file that sets `verifier.tirAddress` leaves `dataspace.tirAddress` unanswered, and the
   Decision 6 rule would block `generate` on a correct file. Step 2 adds a `recognize:` rule (`kind: value`, stage
   `foundations`) on the target path, the first recognition rule for a foundation answer. The leaf then counts as
   recognized: it derives the answer, re-renders to the same bytes, and is not pinned (ADR 0020 §7). A loaded value
   that `TypeURL` refuses derives no answer and gets a load note, so the leaf stays pinned and the rule fires. That
   is correct, because the verifier would get an unusable address.

   **Derived foundation answers are seeded into the profile.** `load.Load` only lists derived answers in
   `Report.Answers`, and `check.Evaluate(b, p)` resolves answers from `p.Answers` and defaults alone, so the
   `recognize:` rule by itself does not reach the check. The core therefore gets
   `load.SeedAnswers(r Report, answers map[string]any) map[string]any`. It copies every derived answer of stage
   `foundations` whose id the profile does not already answer, and returns the merged map without mutating its input.
   - **A seeded answer counts as a given answer.** Profile answers carry no origin, and adding one would change the
     profile format. The value is the user's own, taken from their file, and it renders to the same bytes. A given
     answer always wins over a derived one, so seeding never overwrites a user's edit.
   - **Where it runs:**
     - `POST /api/profile/open` (`handleOpenProfile`, after `restoreSource`) seeds the returned profile.
     - `POST /api/load` keeps returning the report. The UI's `loadedAnswers` in `ui/src/state/reducer.ts` copies the
       report's `foundations`-stage answers into `answers.values`. `profileOpened` needs no change, because the server
       has already seeded the profile it receives.
   - Once saved, the answer is in the `.dscprofile.yaml`. The CLI (`renderProfile` in `internal/cli/cli.go`), which
     never restores the source, then sees it without further plumbing. There is one limit: a profile saved by an
     earlier build lacks the answer, so a CLI `generate` on it blocks until the profile is opened and saved once in
     the GUI. The error names the field, and the README says so (step 7).
   - Feature toggles are not affected. They keep their own `source: loaded` path (`loadedToggles`), and
     `SeedAnswers` ignores answers of other stages.

## Steps

### Step 1: Foundation types `url`, `bool`, `credentialTypes` and the `dataspace` group
Core model and profile only. No bundle content changes yet.

**Model** (`internal/model/foundations.go`):
- Add `TypeURL`: an absolute `http`/`https` URL with a host. It has no userinfo, fragment, whitespace or control
  characters, and none of the characters `plainLine` refuses. Use a named max-length constant.
- Add `TypeBool`: a Go `bool`.
- Add `TypeCredentialTypes`: a list of `{type, description?}`.
  - `type` matches a named pattern such as `^[A-Za-z][A-Za-z0-9._-]*$`, and types are unique.
  - `description` is optional and checked as `TypeString` when present.
  - There is a named cap on the list length.
  - `Check` accepts `[]any` of `map[string]any`, which is what both yaml.v3 and encoding/json decode to. It refuses
    unknown element keys.
- Add `GroupDataspace` to `knownFoundationGroups` in screen order: organization, dataspace, network.
- Validation at compile time:
  - A `tpl`/`join` arg may only reference foundations of a scalar string-like type.
  - A `bool` or list foundation may be referenced only by `copy`.
  - Neither a `bool` nor a list may have a non-constant `default`, `defaultWhen` or `prefix`.
  - Add the new types to `knownTypes`.

**Profile** (`internal/profile/format.go`):
- Update the `Answers` doc comment: values are scalars or a type's list.
- Make sure the strict decode, the deterministic encode and `validateAnswers` handle the list answer. Add a
  save → open round-trip test that keeps the list order.

**UI types** (`ui/src/api/foundations.ts`, `ui/src/state/answers.ts`):
- Widen the answer value type to `string | boolean | CredentialType[]`, so that existing code keeps compiling. No new
  UI yet.

**Tests:** table-driven cases in `internal/model/foundations_test.go` (valid and invalid for each new type, plus the
compile-time refusals) and in `internal/profile/format_test.go`, using fixtures in `testdata/`.

**Acceptance:** `go test ./...`, `npm test` and `make lint` are green.

### Step 2: `dataspace.*` foundations in the bundle and the TIR render target
- In `model/10.16.0/bundle.yaml`:
  - Append the three foundations with `stage: foundations` and `group: dataspace`, each with `label`, `help` and a
    `doc` link. Suggested links: `doc/deployment-integration/roles/provider/README.md` and the trust-anchor docs.
  - `dataspace.tirAddress` (`url`) has one target:
    `{path: 'decentralizedIam.vcAuthentication.vcverifier.deployment.verifier.tirAddress', fn: copy, args: ["{{ .dataspace.tirAddress }}"]}`.
  - `dataspace.credentialTypes` (`credentialTypes`) and `dataspace.autoRegistration` (`bool`) get no targets. Add a
    YAML comment above them that names their consumers (US-5.5, US-5.6) and ADR 0022.
  - Document the new types and the group in the header comment.
  - Add `{path: 'decentralizedIam.vcAuthentication.vcverifier.deployment.verifier.tirAddress', answer:
    dataspace.tirAddress, stage: foundations, kind: value}` to `recognize:` (Decision 7).
  - Bump `revision: 7` → `8`.
- Recognition of a foundation answer (`internal/model`, `internal/load`):
  - At compile time, a `recognize:` rule whose answer is a foundation id must have that foundation's stage, and
    `kind: value` only for a scalar string-like type (`toggle` for `bool`). A list type cannot be recognized.
  - `internal/load` runs the foundation's `Check` on a recognized value. A refused value derives nothing and gets a
    note ("does not match the type of %s").
  - Tests: `internal/load/load_test.go` rows for a loaded file with the TIR leaf set (answer derived, leaf
    recognized), with an invalid URL (no answer, note, leaf unknown), and without it (no answer). Also a
    `render_test.go` row: loading that file and rendering keeps the leaf unpinned and byte-identical.
- Seeding derived foundation answers (Decision 7):
  - `internal/load/seed.go`: `SeedAnswers`.
  - `internal/server/profile.go`: `handleOpenProfile` seeds after `restoreSource`.
  - `ui/src/state/reducer.ts`: `loadedAnswers` seeds the report's `foundations`-stage answers.
  - Tests:
    - `internal/load/seed_test.go`, table-driven: a derived foundation answer fills an empty slot; a given answer is
      kept; a `features`-stage toggle is ignored; an invalid value derived nothing, so nothing is seeded; the input
      map is not mutated.
    - `internal/server/profile_test.go`: opening a profile whose source sets `verifier.tirAddress`, with no given
      answer, returns a profile with `answers["dataspace.tirAddress"]`. A profile that already answers it keeps its
      own value.
    - `ui/src/state/reducer.test.ts`: `fileLoaded` with a report carrying a `foundations` answer and a `features`
      toggle seeds only the former into `answers.values`. Saving afterwards puts it into the profile body.
- In `internal/render/provider_test.go`:
  - Add a reference answer for `dataspace.tirAddress` (`https://tir.127.0.0.1.nip.io/`).
  - Make the domain-leaf search accept a leaf that a target of another foundation owns, when that foundation's
    reference value contains the domain.
  - Drop the `verifier.tirAddress` exclusion row and update the other three TIR/registration rows as in Decision 1.
  - Add the TIR answer to `internal/render/testdata/provider.dscprofile.yaml`.
- Update `internal/render/render_test.go` and `internal/server/foundations_test.go`:
  - An unanswered `dataspace.tirAddress` emits no key.
  - The answered address renders byte-identically.
  - `GET /api/foundations` lists the three entries with group `dataspace` and a count of 1 / 0 / 0.

**Acceptance:** the TIR address renders into the provider.yaml key, and `go test ./...` is green.

### Step 3: L2 rule — credential verification on ⇒ TIR address set
- In `internal/model/checkrules.go`:
  - Allow the `when` field `features.<id>`. The id must exist in the bundle's catalogue, and the only values are
    `on`/`off`.
  - Add `RuleWhen.Unset bool` (`yaml:"unset"`), refusing a `when` that has both or neither of `equals`/`unset`.
  - Add a constant `FeatureFieldPrefix = "features."`.
- In `internal/check/check.go`:
  - `features.<id>` resolves from `p.Features[id].Enabled`. Absent means `off`.
  - `unset` holds when the answer id is not in the resolved answers.
- Add to `rules:` in the bundle:
  `{id: tir-address-for-verification, severity: error, stage: foundations, field: answers.dataspace.tirAddress,
  when: [{field: features.access.verification, equals: "on"}, {field: answers.dataspace.tirAddress, unset: true}]}`
  with a message ("The verifier cannot check issuers without the data space's Trust Anchor…") and a `doc` link.
  Bump `revision` to 9, because every merged step that changes the bundle bumps it once.
- Tests:
  - `internal/model/checkrules_test.go`: table cases for the new grammar, including the refusals.
  - `internal/check/check_test.go`: a dedicated table-driven test for this rule.
    - It fires as **error** with verification on and the TIR unset. That is the failure it was written for.
    - It does not fire with the TIR set, with verification off, or with the feature absent.
    - It does not fire for a **loaded** file that sets `verifier.tirAddress` and has verification on, with no given
      answer in the profile. The row loads the fixture through `internal/load`, seeds with `load.SeedAnswers` and
      then calls `Evaluate`; it does not hand-write the answer. A companion row skips the seeding and expects the
      error, which pins that `Evaluate` itself reads nothing from the load result.
    - `Blocking` is true when it fires.
  - Use profile fixtures in `internal/check/testdata/`.
  - Proof that the derived answer reaches `Evaluate` on every front end:
    - **Server, load:** `internal/server/render_test.go` loads the fixture through `POST /api/load`, builds the
      profile the way the UI does (report answers of stage `foundations` in `answers`), turns verification on, and
      `POST /api/render` returns no `tir-address-for-verification` finding.
    - **Server, open:** `internal/server/profile_test.go` opens a `.dscprofile.yaml` whose source sets the TIR and
      whose answers do not, then renders the returned profile: no finding.
    - **CLI:** `internal/cli/cli_test.go`, table-driven over `internal/cli/testdata/`. `generate` succeeds on a
      profile saved after a load (the seeded answer present, verification on). The same profile without the answer
      fails with `errBlockingFindings` and names `answers.dataspace.tirAddress`, which is the documented limit of
      Decision 7.

**Acceptance:** `go test ./...` and `make lint` are green, and the existing `did-key-in-production` tests pass unchanged.

### Step 4: Core package `internal/dataspace` — descriptor preview and credential-type access
A new core package. It must not import `net/http`, so `internal/layering_test.go` keeps passing.

API:
- `MaxDescriptorBytes`: a named constant, e.g. 64 KiB. The server enforces it too.
- `ParseDescriptor(data []byte) (Descriptor, error)`:
  - Refuses an oversized document, non-JSON, and a JSON value that is not an object, each with a clear message
    that does not echo the body.
  - Reads the three optional keys of Decision 4, plus the chart's three known keys (shown, not applied).
  - Collects other keys as `Unused`.
- `Preview(b model.Bundle, p profile.Profile, d Descriptor) PreviewResult`:
  - Returns per-foundation `{ID, Current, Imported}`, for values that differ only.
  - Returns `Problems` for an imported value that the foundation's `Check` refuses. That value is not offered.
  - Maps `participant_registration` to the bool answer.
  - Returns an empty result plus the forward-looking message of Decision 4 when the document carries none of the
    three keys. It is a message, not an error.
- `CredentialTypes(b model.Bundle, p profile.Profile) []CredentialType`: the ordered answer, nil when unanswered.
  This is the seam US-5.5 and US-5.6 pre-seed from. Also add `AutoRegistration(b, p) (bool, bool)`.

Tests are table-driven over fixtures in `internal/dataspace/testdata/`:
- the full document;
- the chart's document only, which gives an empty preview;
- credential types as strings and as objects;
- a duplicate type;
- an invalid URL;
- `participant_registration` with a bad value;
- non-JSON, a JSON array, oversized input, and empty input;
- a preview against a profile that already holds the same values, which gives no changes.

**Acceptance:** malformed or oversized input gives an error message, never a panic, and `go test ./...` is green.

### Step 5: Server — TIR reachability check and `POST /api/dataspace/import`
In `internal/server`, add a new `dataspace.go`.

**Outbound client:**
- A `Server.outbound http.RoundTripper` field, which tests swap the same way they swap `models`.
- Default: an `http.Transport` clone whose `Proxy` comes from `httpproxy.FromEnvironment().ProxyFunc()`
  (`golang.org/x/net`, already a direct dependency). This honours `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY`, read when
  the client is built.
- A named timeout constant.

**`POST /api/dataspace/check` `{url}` → `{status: "reachable"|"unreachable", httpStatus?, error?}`:**
- Validate the URL with `model.TypeURL.Check`.
- Send exactly one GET and follow no redirect (`CheckRedirect` returns `http.ErrUseLastResponse`).
- Any HTTP response counts as reachable, with its status. A transport error counts as unreachable, with the error
  text. Do not read the body.
- Never block, never log the URL.

**`POST /api/dataspace/import` `{url, profile}` → `{preview, unused, message}`:**
- Accept a base URL or the full descriptor URL, and append `/.well-known/data-space-configuration` unless the path
  already ends with it.
- Use a `CheckRedirect` that refuses any redirect changing the scheme, with a named max-redirect constant.
- Refuse a non-2xx status with its code.
- Read through `io.LimitReader(MaxDescriptorBytes+1)` and refuse an oversized body.
- Parse and preview with `internal/dataspace`. Write nothing to the profile; the UI applies the preview.

**Routes:** register both in `api.go`. They sit behind the existing token, Origin and same-origin checks.

**Tests** in `internal/server/dataspace_test.go`. All are table-driven over a stub `RoundTripper` that serves
fixtures, so there is no live network.
- Check: reachable 200 and 404, a transport error, a redirect not followed, an invalid URL.
- Import:
  - the fixture document gives the right preview;
  - non-JSON, oversized, and non-2xx responses each give their message;
  - a redirect https → http is refused;
  - a same-scheme redirect is followed;
  - URL normalisation.
- Proxy: with `t.Setenv("HTTPS_PROXY", …)` and `NO_PROXY`, the built transport's `Proxy` returns the proxy for one
  host and nil for the excluded one.
- **No unasked network:** a server whose `outbound` calls `t.Fatal` handles every other API route (model,
  foundations, render, load, profile save/open, preset, features, trial, validate) without calling it.

**Acceptance:** `go test ./...` and `make lint` are green, and `internal/layering_test.go` still passes.

### Step 6: Stage 3 *Data space* panel (UI)
- `ui/src/api/dataspace.ts`: typed clients for the two endpoints, with tests in the style of the existing `*.test.ts`.
- `ui/src/stages/Foundations.tsx`: a new panel between Organization and Network (MOCKUPS §S3, wording from
  `mockups/prototype.html`). Extract it to `ui/src/stages/DataSpacePanel.tsx` if the file grows.
  - **Import from:** a URL field plus an *Import* button. The preview shows each value with current → imported,
    plus problems and unused keys, and offers *Apply* and *Discard*. Only *Apply* dispatches the existing
    set-answer actions. Errors go to an `aria-live` region. An empty preview shows the core's Decision 4 message
    as information, styled apart from errors, and keeps the unused keys visible.
  - **Trust Anchor (TIR):** a URL input plus a *Check* button, with the result shown as text and icon, never colour
    alone (US-11.5).
  - **Credential types:** an ordered table editor (type, description; add, remove, move up/down) bound to
    `answers.dataspace.credentialTypes`.
  - **Register at the Trust Anchor:** a checkbox bound to `answers.dataspace.autoRegistration`.
  - Put *Ask later* (`chrome/AskLater.tsx`) on all four answer fields and keep the existing open-field id
    `answers.<id>`, so Review findings link and focus them.
- Session state in `ui/src/state/reducer.ts`: add `tirCheck: {address, outcome: 'reachable'|'unreachable'|'skipped',
  detail?}`.
  - The outcome is `skipped` whenever an address is set and no check ran for it.
  - Changing the address resets it to `skipped`.
  - Review lists it as an information line, never a blocking finding. It is cleared on new or open, like
    `roleChanges`.
- Tests (vitest + Testing Library, table-driven where possible):
  - Typing a TIR address or editing any field makes **no** call to `/api/dataspace/*`. The test uses a fetch mock
    that fails when called.
  - *Check* calls the endpoint once and shows the result.
  - Import, then Apply, sets the answers.
  - Import, then Discard, changes nothing.
  - Table editor add, remove and reorder.
  - Ask later marks each field.
  - Reducer tests for `tirCheck`.
  - The L2 finding from step 3 shows on the TIR field and blocks generate on Review.

**Acceptance:** `make ui`, `npm test` and `make lint` are green.

### Step 7: ADR 0022 and documentation
- `doc/adr/0022-config-tool-data-space-parameters.md`: Decisions 1–7 above (including the statement that the import is
  forward-looking until a publisher emits its keys), the outbound-call policy (user-triggered
  only, proxy from the environment, size cap, no scheme-changing redirect, server layer only) and its link to US-11.3
  (offline). Add it to the `doc/adr/README.md` index.
- `config-tool/README.md`:
  - *Foundations (Stage 3)*: the data space panel, the new types, the import and its descriptor keys.
  - *Rules and findings*: the new `features.`/`unset` grammar and the rule.
  - *Loading*: derived foundation answers are seeded into the profile as given answers (Decision 7). A profile
    saved by an earlier build needs one open-and-save in the GUI before a CLI `generate`.
  - *Security model*: the only outbound HTTP requests are the two button-triggered ones.
  - *Package layout*: `internal/dataspace`.
- `doc/config-tool/FEATURE_MODEL.md`: update the `dataspace.*` rows (1 key shipped, the rest answers-only, with
  their consumers) and mark the L2 rule as shipped.
- `doc/config-tool/USER_JOURNEY.md` Stage 3: describe what shipped, including the descriptor keys.
- `AGENTS.md`: flip the US-3.3 line to shipped and correct its reason: no chart key exists for auto-registration;
  credential types do have one (`registration.services[*]...credentials[*].type`), owned by US-5.6's list. Mention
  the `recognize:` rule for the loaded TIR address and that `load.SeedAnswers` copies derived foundation answers
  into the profile as given answers.
- Run `.github/scripts/check-links.sh` if Docker is available.

**Acceptance:** the docs match the code, the links resolve, and `make test` and `make lint` are green.
