# Implementation Plan: [config-tool] CT-5 The one-screen local trial

## Overview
Add the fourth S0 card, *Try it locally*. After a prerequisite screen, one choice produces the Quick Start's three
releases (trust-anchor, provider, consumer) and lands on Review with all three listed. Stages 1–6 are skipped.

The generic profile→values renderer (Epics 3/5) does not exist yet. Per the human answer on the ticket ("use 3 and
hard-code all production warnings"), the tool therefore ships bundled values byte-identical to `k3s/trust-anchor.yaml`,
`k3s/provider.yaml` and `k3s/consumer.yaml`. They go out through a new multi-release path: one embedded trial descriptor
→ three releases → three values files. A fixed list of production warnings is always shown and never suppressed.
`helm template` is verified for all three outputs, so the ticket can close without the cross-epic renderer.

Ground rules for every step (from `AGENTS.md` / earlier CT tickets):
- The core (`internal/*` except `internal/server`) never imports `net/http`; `internal/layering_test.go` enforces it.
- Nothing the tool did not create is ever overwritten. Multi-file writes refuse when any target exists and write nothing.
- `profile.Profile` (format 1, single `Target`, ADR 0015) stays untouched. The trial descriptor is not a `.dscprofile.yaml`.
- One-line GoDoc / JSDoc, named constants, table-driven Go tests, Vitest `it.each` where it fits.
- Out of scope: installing anything, detecting a cluster, the namespaces/operators/infra/ordering `k3s/deploy.sh` applies.

## Steps

### Step 1: ADR 0016 and the bundled trial assets
Record the decision and add the embedded data. There is no Go logic beyond embedding in this step.

- `doc/adr/0016-config-tool-local-trial.md`, in the format of ADR 0013–0015. It records:
  - The trial ships pre-rendered values (option 3 of the ticket discussion) rather than answer sets fed through
    `internal/render`. They are replaced by real answer sets once Epics 2/3/5 render.
  - The copies are byte-identical to `k3s/` and kept in sync by a test, because `go:embed` cannot reach above the
    module directory.
  - A multi-release *trial descriptor* is not a profile, so profile format 1 is unchanged.
  - The production warnings are a hard-coded list until Epic 6 validation, each backed by evidence in the values.
  - Outputs never overwrite an existing file.
  - Only the `data-space-connector` outputs (provider, consumer) can be reopened in the tool. The trust-anchor output
    is not meant to be, because there is no trust-anchor model bundle and CT-3 reads every file against
    `data-space-connector`. Its header therefore carries no chart version, and the banner names the chart.

  Add it to `doc/adr/README.md`.
- New directory `config-tool/trial/` (sibling of `config-tool/model/`):
  - `embed.go`: package `trialdata`, one-line package doc, `//go:embed trial.yaml values/*.yaml` into `var FS embed.FS`.
  - `values/trust-anchor.yaml`, `values/provider.yaml` and `values/consumer.yaml`: exact copies of the `k3s/` files.
    Do not normalise trailing newlines; some k3s files have none.
  - `trial.yaml` is the descriptor. It has a one-line header comment and an ordered `releases:` list. Each entry has
    `role`, `source` (repo path, e.g. `k3s/provider.yaml`) and `values` (embedded path). Its `target` uses the keys of
    `profile.Target`: `type: file`, `namespace`, `createNamespace: true`, `releaseName`, `chart`, `chartVersion` and
    `environment: development`. The releases, namespaces and charts match `k3s/deploy.sh`:
    - `trust-anchor`: namespace `trust-anchor`, chart `trust-anchor`, `chartVersion` 1.3.6.
    - `provider`: namespace `provider`, chart `data-space-connector`, `chartVersion` 10.16.0.
    - `consumer`: namespace `consumer`, chart `data-space-connector`, `chartVersion` 10.16.0.

    Read the versions from `charts/*/Chart.yaml`. Do not hard-code them from this plan if they have moved.
- `config-tool/Makefile`: a `trial-sync` target that copies the three `k3s/` files over `trial/values/`.
- `.github/workflows/config-tool.yaml`: add `charts/**` to both `paths:` filters. `k3s/**` is already there. Steps 2
  and 4 read `Chart.yaml` and render the charts.

Acceptance: `go build ./...` and `go vet ./...` pass; the ADR is linked from the ADR index.

### Step 2: Core package `internal/trial` — descriptor, render, warnings
New core package `config-tool/internal/trial`, with no `net/http` import.

- Types:
  - `Release`: `Role`, `Source`, `Values` (the embedded path) and `Target profile.Target`.
  - `Trial`: `Releases []Release`.
  - `Output`: `Release`, `File` (`<releaseName>.yaml`) and `Values []byte`.
  - `Warning`: `ID`, `Releases []string`, `Message`, `Evidence`. `Evidence` is a substring that must occur in each
    listed release's bundled values. It proves the warning applies.
- `Load(fsys fs.FS) (Trial, error)`:
  - Decode `trial.yaml` strictly (yaml.v3 `KnownFields(true)`, one document).
  - Require at least one release, unique release names and unique file names, and that each `values` file exists in
    `fsys`.
  - Run `Target.Validate()` on each release; it only checks the chart name is present. Do **not** call
    `profile.Profile.Validate`, because the `trust-anchor` chart has no model bundle.
  - Require `environment: development` for every release.
- `Render(t Trial, fsys fs.FS, toolVersion string) ([]Output, error)`. Each output is three parts:
  1. `roundtrip.Header{ToolVersion, ChartVersion, Profile: TrialProfileName}.Render()`, where the constant
     `TrialProfileName` is `"local-trial"`. `Header` has no chart name, and `load.chooseBundle` reads `ChartVersion`
     as a `load.DefaultChart` (`data-space-connector`) version. So `ChartVersion` is the release's `chartVersion` only
     when `Target.Chart == load.DefaultChart`, and empty otherwise (the trust-anchor release). The banner names the
     chart and its version for every release.
  2. A fixed development banner of 2–3 comment lines, held in a constant. It says the file is a development
     configuration from the local trial for chart `<chart>@<chartVersion>`, equivalent to the source `k3s/` file,
     with self-signed certificates and chart-deployed databases, and not for production.
  3. The bundled bytes, unchanged.

  Output order follows the descriptor.
- `Warnings() []Warning` returns a copy of a package-level hard-coded list. Before writing it, check each item against
  the bundled files, using `PREREQUISITES.md` and FEATURE_MODEL.md §validation for wording. The expected items are:
  - Self-signed certificates from `selfsigned-issuer` (all three releases).
  - Chart- or operator-deployed PostgreSQL / MongoDB with default sizes (all three).
  - `*.127.0.0.1.nip.io` host names that only resolve on this machine (all three).
  - `tlsTrustAll: true`, which trusts every certificate (wherever it occurs).
  - Example identities (`did:web:fancy-marketplace.biz`, `mp-operations.org`) and demo keystore passwords.
  - Outbound traffic through the local squid proxy (`squid-proxy.infra`).
  - Memory limits sized for a laptop. This one links to `PREREQUISITES.md#set-memory-limits-on-the-java-components`.

  Drop any item that has no evidence in the bundled values.
- Tests (`trial_test.go`, table-driven):
  - Each embedded `values/<x>.yaml` is byte-equal to `<repo>/k3s/<x>.yaml`. Use the `repoRoot` pattern of
    `internal/load/corpus_test.go`. On failure, print the hint "run `make trial-sync`".
  - Each release's `chartVersion` equals `version:` in `<repo>/charts/<chart>/Chart.yaml`.
  - `Load` refuses each broken descriptor, using an in-memory `fstest.MapFS`: unknown key, duplicate release, missing
    values file, production environment and empty releases.
  - `Render`:
    - Each output starts with the generated header and the banner.
    - The remainder equals the bundled bytes.
    - `roundtrip.Parse` on each output recognises the header, with `Profile` `local-trial`.
    - Provider and consumer can be opened back in the tool: `load.Load` on them picks the bundle of their
      `chartVersion` with `ChartVersionFrom == load.FromHeader` and no `ChartVersionNote`.
    - Trust-anchor has an empty header `ChartVersion`, and its banner names `trust-anchor@<chartVersion>`. It is not
      meant to be reopened (ADR 0016), because no trust-anchor model bundle exists; the test does not load it.
  - Warnings:
    - Every warning's `Evidence` occurs in each release it names.
    - Every named release exists.
    - IDs are unique.
    - A golden list of IDs pins that none is silently dropped.

Acceptance: `go test ./internal/trial/... ./internal/...` and `make lint` pass; `layering_test.go` stays green.

### Step 3: Multi-release write path and `dsc-config generate --trial`
Emit the three files without ever overwriting.

- `internal/trial`: `WriteAll(dir string, outs []Output) ([]string, error)`.
  - Join `dir` with each `File`.
  - Pre-flight with `os.Lstat`. If any target exists (symlinks included), return an error that matches `fs.ErrExist`,
    lists *all* existing paths, and writes nothing.
  - Otherwise create each file with `O_WRONLY|O_CREATE|O_EXCL`, so a race still cannot clobber.
  - On any failure, remove every file this call created.
  - Return the written paths.
  - Reuse the file mode the CLI's `writeOutput` uses: move `outputFileMode` to a shared exported constant rather than
    duplicating it.
- `internal/cli/cli.go`: extend `generate` with `--trial` (bool) and `--output-dir` (default `.`).
  - `--trial` excludes `--profile` and `--output`: use cobra `MarkFlagsMutuallyExclusive`, and make `--profile`
    required only without `--trial` (`MarkFlagsOneRequired(flagProfile, flagTrial)`).
  - The run is `trial.Load(trialdata.FS)` → `trial.Render(..., version.Version)` → `trial.WriteAll`.
  - Print one line per written file.
  - Print the warnings to stderr, prefixed `warning:`, never suppressed.
  - Print the Quick Start pointer
    `https://github.com/FIWARE/data-space-connector/blob/main/doc/deployment-integration/local-deployment/LOCAL.MD`
    as a named constant.
- Tests:
  - `internal/trial` `WriteAll`, table-driven over these cases, asserting directory contents afterwards:
    - empty dir writes three;
    - one target exists → nothing written and the error lists it;
    - a symlink target is refused;
    - a write failure in the middle removes the files already created.
  - `internal/cli/cli_test.go`, in the style of the existing generate tests:
    - `generate --trial --output-dir <tmp>` writes three files and prints the warnings;
    - `--trial --profile x` is refused;
    - a second run refuses and leaves the first files byte-identical.

Acceptance: `go test ./...` passes; `dsc-config generate --trial --output-dir /tmp/t` produces `trust-anchor.yaml`,
`provider.yaml` and `consumer.yaml`.

### Step 4: `helm template` verification of all three releases
Prove the acceptance criterion "renders with `helm template` without errors, for all three releases".

- `internal/trial/helm_test.go`, as `TestRenderedValuesTemplate`:
  - For each `Render` output, write it to `t.TempDir()` and run
    `helm template <releaseName> <repo>/charts/<chart> --namespace <namespace> -f <file>`, with `exec.CommandContext`
    and a named timeout constant.
  - Fail with helm's stderr.
  - Skip, with the reason in the message, when `helm` is not on `PATH` or the chart's dependencies are not fetched (no
    `charts/<chart>/charts/` directory). If the env var `DSC_CONFIG_REQUIRE_HELM=1` is set (a named constant), the skip
    becomes a failure.
- `.github/workflows/config-tool.yaml`: new job `trial-render`.
  - Install Helm with `azure/setup-helm@v4` and the pinned `HELM_VERSION` from `.github/workflows/test.yaml` (Helm
    4.x); `check.yaml` installs no Helm.
  - Add the repos with the `helm repo add` list of `.github/workflows/pre-release.yml` (step "Add Helm
    repositories"), then run `helm dependency update` for `charts/data-space-connector` and `charts/trust-anchor`, as
    `pre-release.yml` does. Use `update`, not `build`: no `Chart.lock` is committed, so `build` would fall back to
    `update` anyway. The `oci://` dependencies need no repo add.
  - Then run `DSC_CONFIG_REQUIRE_HELM=1 go test ./internal/trial -run TestRenderedValuesTemplate`.
  - Add one short "why" comment above the job.
- Run it locally first: install helm if missing (`sudo` is available), run `helm dependency update` for both charts
  and run the test.
  - If helm refuses a bundled file, for example over duplicate keys (`roundtrip` notes them in `k3s/consumer.yaml`),
    do not edit the copy, because equivalence must hold.
  - Check how `k3s/deploy.sh` passes the same file. If the problem is real, fix it in `k3s/` in this step and re-sync,
    and say so in the PR.

Acceptance: the test passes locally with helm, and the new CI job is green.

### Step 5: HTTP API for the trial
Two endpoints in `internal/server`, on the existing mux, token guard and body cap, in a new file `trial.go`.

- `GET /api/trial` returns JSON with:
  - `profile` (`"local-trial"`) and `environment` (`"development"`);
  - `releases`: `[{role, releaseName, namespace, chart, chartVersion, source, file, values}]`, where `values` is the
    rendered text including header and banner;
  - `warnings`: `[{id, releases, message}]`.

  Load and render once at server construction, or lazily with `sync.Once`. Any load/render error is a 500 with
  `errorBody`.
- `POST /api/trial/write` takes `{"dir": "<absolute path>"}`.
  - The path must be absolute and an existing directory, otherwise 422. Reuse the checks and messages style of
    `checkProfilePath`.
  - On success, call `trial.WriteAll` and return 201 with `{dir, files}`.
  - When files exist, return 409 with `errorBody` naming them, the same status profile save uses for an existing file.
  - No endpoint writes to a cluster.
- Tests in `internal/server/trial_test.go`, table-driven like `profile_test.go`:
  - GET returns three releases in order with non-empty values and every warning;
  - write to an empty temp dir → 201 and three files;
  - a second write → 409 with the files unchanged;
  - a relative dir or a missing dir → 422;
  - a request without the token → refused.

Acceptance: `go test ./internal/server/...` passes.

### Step 6: UI — S0 card, prerequisite screen and trial state
- `ui/src/links.ts`:
  - `QUICK_START_URL` →
    `${REPOSITORY}/blob/main/doc/deployment-integration/local-deployment/LOCAL.MD#quick-start`.
  - `SIZING_URL` → `${REPOSITORY}/blob/main/doc/deployment-integration/roles/PREREQUISITES.md#sizing`.
  - Extend `links.test.ts`.
- `ui/src/api/trial.ts`: types mirroring Step 5, `fetchTrial(signal?)` and `writeTrial(dir)`. Tests in
  `api/trial.test.ts` use the existing `src/test/api.ts` fetch mock.
- State (`state/reducer.ts`):
  - Add `trial?: Trial` to `AppState`.
  - A new action `trialStarted { trial }` calls `startOver(state, StageId.Review, …)`. It sets
    `answers.target.environment = development`, clears `loaded`/`profilePath`, and stores `trial`. `visited` is
    `[Review]` only.
  - `newConfiguration`, `fileLoaded` and `profileOpened` clear `trial`.
  - Cover each transition in `reducer.test.ts`.
- `stages/Start.tsx`:
  - Replace the `COMING` "Try it locally" entry with a live card, wording from MOCKUPS S0.
  - Clicking the card shows `TrialPrerequisites`, a new component in `stages/TrialPrerequisites.tsx`, in place of the
    cards. It is still S0, so the chrome is not entered. The screen:
    - Is headed "Try it locally".
    - Lists what is needed: a local Kubernetes cluster as in the Quick Start (k3s); `kubectl` and Helm to install the
      releases yourself; the namespaces, operators, CA and infrastructure the Quick Start applies before the
      releases, linking to `QUICK_START_URL`; and enough memory and disk for three roles.
    - Links to `SIZING_URL` and **does not** restate any figure.
    - States that the result is a development configuration: self-signed certificates and chart-deployed databases.
    - Has the buttons **Back** (back to the cards) and **Generate the three configurations**.
  - Only Generate calls `fetchTrial`. On success it dispatches `trialStarted`. On failure it shows an alert and stays
    on the screen. Unmounting aborts the request, the same pattern as `OpenValuesCard`.
- Tests:
  - `Start.test.tsx`: the card opens the prerequisite screen without any fetch; Back returns; Generate fetches and
    dispatches; a failure shows an alert.
  - `TrialPrerequisites.test.tsx`: both links are present, and no sizing figure is shown (assert no `/GiB|cores|GB/`
    text).

Acceptance: `npm run lint`, `npm run typecheck` and `npm test` pass in `config-tool/ui`.

### Step 7: UI — Review lists the three releases; stages 1–6 skipped; Playwright
- `stages/Review.tsx`: when `state.trial` is set, render a `TrialReview` instead of the report or placeholder, in a new
  file `stages/TrialReview.tsx`. It shows:
  - The heading "Local trial" with a **Development** badge, plus a one-line note: "equivalent to the Quick Start; the
    tool emits values, you run the install", with a link to `QUICK_START_URL`.
  - One entry per release: release name, namespace, `chart@chartVersion`, output file name and source `k3s/…` file.
    The rendered values sit in a collapsed `<details>` with a `<pre>`.
  - "Production warnings (N)": every warning with the releases it applies to. There is no dismiss or hide control.
  - **Write files**: a directory path field (default `defaultDir` from `fetchRecent`, as in the save dialog) and a
    button that calls `writeTrial`. On success it lists the written paths. A 409 shows the server's message, and nothing
    was written.
- Skipped stages:
  - `state/issues.ts` gains a `skipped` status, with glyph and legend text, returned for Target…Configuration while
    `state.trial` is set.
  - Choosing a skipped stage shows a short placeholder: "Skipped in the local trial", with a **Start a new
    configuration** button that dispatches `newConfiguration`.
  - Install keeps its Epic 10 placeholder.
- Save profile: the trial is fixed, so the rail's ⌘S button and the Review action are disabled while `state.trial` is
  set. The title names the reason. This guards `saveContext` too, so ⌘S does nothing.
- Tests:
  - `TrialReview.test.tsx`: three releases listed, badge shown, the warning count equals the payload, no
    dismiss control, write success and 409 paths.
  - `Chrome.test.tsx`/`Rail`: stages 1–6 are shown as skipped, and save is disabled in the trial.
- `ui/e2e/smoke.spec.ts`, a new test "try it locally lands on Review with three releases":
  - S0 → card → prerequisites visible, Sizing link present → Generate → Review shows `trust-anchor`, `provider`,
    `consumer`, the Development badge and the warnings.
  - Then write to a temp dir (create it in the test with `fs.mkdtemp`) and assert the three files exist.

Acceptance: `npm test` and `npm run e2e` (after `make build`) pass, and so does `make test`.

### Step 8: Documentation
- `config-tool/README.md`:
  - A "Try it locally" section covering the S0 card and the prerequisite screen, the three releases and their file
    names, the `generate --trial --output-dir` CLI, the never-overwrite rule, the development banner and the
    hard-coded warnings.
  - `make trial-sync` and the `helm template` test, with how to run it: helm plus `helm dependency update`
    (repos as in `pre-release.yml`), and `DSC_CONFIG_REQUIRE_HELM`.
  - Link to the Quick Start instead of copying it.
- `doc/config-tool/USER_JOURNEY.md` Stage 0, the *Try it locally* row: note the prerequisite screen and that Review
  lists the three releases. Note that values are bundled until the renderer exists (ADR 0016).
- `doc/config-tool/MOCKUPS.md`: add a short "S0 — Try it locally: prerequisites" box and the Review trial listing, in
  the existing ASCII style.
- `AGENTS.md`, "Config tool" section: replace the CT-5 "planned" bullet with the final package and endpoint names.
  Step agents may not edit `AGENTS.md`, so this is handed to the plan agent (see the step 8 Taiga comment).
- Run `.github/scripts/check-links.sh` on the touched docs, if docker is available.

Acceptance: links resolve, and the docs describe exactly what Steps 1–7 shipped.
