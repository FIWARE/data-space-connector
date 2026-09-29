# Response to the review of PR #209 (`policies-in-service`)

Thanks for the review — all nine remarks were reproduced against the branch and all nine are
valid. Below: what was verified, and the concrete fix per remark.

Two remarks share a root cause (the chart version), and the branch turned out to be **5 commits
behind `main`**, which changes the target version — see R1.

| # | File | Verdict | Fix |
|---|---|---|---|
| R1 | `charts/data-space-connector/values.yaml:1779` | valid | bump `Chart.yaml` to `10.8.0` |
| R2 | `doc/release-notes/10-x.md:10` | valid | retitle section to `10.8.0` |
| R3 | `doc/release-notes/10-x.md:61` | valid | drop the interim pin block + the matching sentence |
| R4 | `.../provider/COMPOSED_SPECIFICATIONS.md:12` | valid | already fixed |
| R5 | `RELEASE_NOTES_POLICIES_IN_SERVICE.md:1` | valid | remove the file from the PR |
| R6 | `it/.../policies/allowServiceSpec.json:1` | valid | drop the file + correct the release-note claim |
| R7 | `StandardStepDefinitions.java:1653` | valid | move the javadoc back to `createProductOffering` |
| R8 | `StandardStepDefinitions.java:1274` | valid | add `serviceSpecification` to `cleanUpTMForum()` |
| R9 | `charts/data-space-connector/values.yaml:1784` | valid | reword "below the limit" (3 places) |

---

## R1 — the chart version is not bumped, so the flag never ships

> These two values are added but `Chart.yaml` stays at `version: 10.6.0` [...] the release workflow
> runs chart-releaser with `CR_SKIP_EXISTING: true`.

**Agreed, and it moved on since the review.** Confirmed on the branch:

* `charts/data-space-connector/Chart.yaml:5` is `version: 10.6.0`, and `Chart.yaml` is not part of
  this PR's diff at all;
* `data-space-connector-10.6.0` is tagged, and `.github/workflows/release.yml:85` sets
  `CR_SKIP_EXISTING: true` — so the packaging step would skip the chart silently and
  `enableSpecificationComposition` would never reach an installable chart.

One correction to the suggested target: `10.7.0` was taken in the meantime — `main` is at
`version: 10.7.0` (commit `aa27415`, tag `data-space-connector-10.7.0`, released 2026-09-21
12:51 UTC, via PR #211). This branch is 5 commits behind `main`.

**Fix**

1. merge `origin/main` into `policies-in-service`;
2. bump `charts/data-space-connector/Chart.yaml` to `version: 10.8.0` (minor — additive feature,
   per `CLAUDE.md`).

## R2 — the release-note section documents a version that does not contain the feature

> `10.5.0` and `10.6.0` are already tagged [...] Retitle it to the version this actually ships in.

**Agreed** — same root cause as R1. The heading at `doc/release-notes/10-x.md:10` reads
`## 10.5.0 — policies on the ServiceSpecifications a product is composed of`, and `10.5.0` is
tagged since before this branch existed.

**Fix**: retitle to `## 10.8.0 — policies on the ServiceSpecifications a product is composed of`
(not `10.7.0`, see R1). The same version string also appears in
`RELEASE_NOTES_POLICIES_IN_SERVICE.md` ("### `data-space-connector` — `10.5.0`"), which R5 removes
from the PR anyway.

## R3 — the interim `trusted-issuers-list` pin describes something that is not there

> `values.yaml` contains no such pin, and `Chart.yaml` already depends on `decentralized-iam` 2.1.22.

**Agreed, the block is stale.** Verified:

* no `tag: "0.9.1"` (nor any `0.9.1`) anywhere in `charts/data-space-connector/values.yaml`;
* `Chart.yaml:10` pins `decentralized-iam` at `2.1.22`, while the block claims `2.1.20` /
  `vc-authentication 1.3.6`;
* `doc/release-notes/10-x.md:1003` — an earlier section of the *same file* — already records
  `decentralized-iam 2.1.19 -> 2.1.21` delivering app `0.9.1` via `vc-authentication 1.3.7`.

So the block contradicts both the chart and the file it lives in. Dropping it is right; adding the
pin would be a regression (it would hold `trusted-issuers-list` at an image the chain already
delivers).

**Fix**

* delete `doc/release-notes/10-x.md` lines 62–76 (the "The `trusted-issuers-list` image is pinned in
  `values.yaml` for now" paragraph, the YAML block and the "Remove the override once…" sentence);
* in `doc/deployment-integration/roles/provider/COMPOSED_SPECIFICATIONS.md:29-30`, replace

  > The connector chart pins the `trusted-issuers-list` image until the chart chain catches up; see
  > the [10.5.0 release note](../../../release-notes/10-x.md).

  with a statement of fact:

  > The chart chain delivers this since `decentralized-iam` 2.1.21 (`vc-authentication` 1.3.7); no
  > image override is needed.

* keep the *Component / Minimum version* table rows in both files — `contract-management` chart
  `3.5.36` (app `3.3.12`) and `trusted-issuers-list` app `0.9.1` are still the correct minimums, and
  `Chart.yaml:30` already pins `contract-management` at `3.5.36`.

## R4 — the `doc/tmforum/` links are broken

> `doc/tmforum/` does not exist on this branch.

**Agreed.** `doc/tmforum/` is neither on the branch nor in the PR diff (the diff touches 9 files, none
under `doc/tmforum/`). Three live references:

* `doc/deployment-integration/roles/provider/COMPOSED_SPECIFICATIONS.md:12`
* `doc/release-notes/10-x.md:19`
* `RELEASE_NOTES_POLICIES_IN_SERVICE.md:11` and `:78` (removed by R5)

Those nine documents are working notes of the topic (the analysis and the decision register); they
were never prepared for publication in this repo, and shipping them would widen this PR from a flag
plus a test into a documentation drop that deserves its own review.

**Fix**: drop the links rather than add the directory.

* `COMPOSED_SPECIFICATIONS.md:11-12` — remove the "The analysis […] are in `doc/tmforum/…`"
  blockquote; the guide is self-contained without it.
* `10-x.md:18-19` — replace the sentence with a pointer to the guide that *is* in this PR:

  > The provider-facing guide is
  > [`COMPOSED_SPECIFICATIONS.md`](../deployment-integration/roles/provider/COMPOSED_SPECIFICATIONS.md).

If you would rather have the analysis in the repo, I'm happy to open a follow-up PR that adds
`doc/tmforum/` on its own and restores the links — but not in this one.

## R5 — `RELEASE_NOTES_POLICIES_IN_SERVICE.md` is a working note

> Release notes live in `doc/release-notes/` — suggest dropping this file from the PR.

**Agreed.** The file is an artifact of a personal topic-branch convention, not of this repository's:
it names the branch in its first line, restates the `doc/release-notes/10-x.md` section, carries the
broken `doc/tmforum/` links (R4) and the stale `10.5.0` version (R2) and the interim pin (R3). It is
also the only `RELEASE_NOTES_*.md` in the repo root.

**Fix**: `git rm RELEASE_NOTES_POLICIES_IN_SERVICE.md`. Nothing in it is lost — the "what was
decided" table is the only content not already in `10-x.md`, and it belongs with the analysis
documents discussed in R4.

## R6 — `allowServiceSpec.json` is not loaded by anything

> `createPolicyAtMP` is never called with `allowServiceSpec`, and the new scenario posts the service
> specification to `TMF_DIRECT_ADDRESS`, which bypasses the OID4VP-protected API.

**Agreed on both halves.** Verified:

* the nine `createPolicyAtMP("…")` call sites cover `allowCatalogRead`, `allowProductOffering`,
  `allowProductOrder`, `allowSelfRegistration`, `allowTMFAgreementRead`, `clusterCreate`,
  `energyReport`, `transferRequest`, `uptimeReport` — `allowServiceSpec` is not among them;
* `createSmallServiceSpec()` posts to `MPOperationsEnvironment.TMF_DIRECT_ADDRESS
  + "/tmf-api/serviceCatalogManagement/v4/serviceSpecification"`, i.e. straight at the TMForum API,
  so no policy is evaluated for that call at all.

The release note's claim ("so a seller may author service specifications through the OID4VP-protected
API") is therefore unbacked by this PR.

(For context, not as a defence: `allowAgreementRead.json` is likewise unreferenced on `main`, so the
file would not be the first orphan in that directory — the remark still stands.)

**Fix (recommended)**: drop the file and the claim.

* `git rm it/src/test/resources/policies/allowServiceSpec.json`;
* remove the corresponding bullet from the release note (it only survives in
  `RELEASE_NOTES_POLICIES_IN_SERVICE.md`, which R5 deletes).

**Alternative, if the policy should be proven**: keep the file and make the scenario author the
service specification through the protected API — post to `TMF_ADDRESS` with the seller's access
token and add `createPolicyAtMP("allowServiceSpec")` to a new
`@Given("M&P Operations allows to author service specifications.")` step used by the new scenario.
That is a larger change (the step currently has no token plumbing) and I would rather do it in a
follow-up than grow this PR; say the word if you prefer it here.

## R7 — the dangling `createProductOffering` javadoc

> `buildClusterPolicy` was inserted between `createProductOffering`'s javadoc and its signature.

**Agreed, verified.** At `StandardStepDefinitions.java:1650-1696` the block

```java
/**
 * Creates a product offering at the provider's TMForum API referencing the given specification.
 *
 * @param offeringName the name of the product offering
 * @param specId       the product specification ID to reference
 */
/**
 * Builds the ODRL policy that allows an OperatorCredential holder to act on K8S clusters,
 * ...
 */
private Map<String, Object> buildClusterPolicy(...)
```

leaves two javadoc comments stacked on `buildClusterPolicy`, and `createProductOffering` at line
~1698 undocumented.

**Fix**: move the first comment down so it sits directly above
`private void createProductOffering(String offeringName, String specId)`. No other change.

## R8 — `cleanUpTMForum()` leaks service specifications

> service specs created here survive every scenario and pile up when the suite is re-run.

**Agreed, verified.** `cleanUpTMForum()` (line 223) clears `productOffering`,
`productSpecification`, `productOrder`, `agreement` and `party/organization` — nothing else. The new
`createSmallServiceSpec()` step creates a `serviceSpecification` on every run of the new scenario,
and it is never removed.

**Fix**: add the list to the cleanup, after the product specification so the referencing resources go
first:

```java
cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
        "/tmf-api/productCatalogManagement/v4/productSpecification", "Standard specifications");
cleanUpTMForumResourceList(TMF_DIRECT_ADDRESS,
        "/tmf-api/serviceCatalogManagement/v4/serviceSpecification", "Standard service specifications");
```

`cleanUpTMForumResourceList` already tolerates an empty or absent list, so this is safe for the
scenarios that create no service specification.

## R9 — "below the limit" reads as the opposite of what happens

> the comment uses "below" for both the resolved levels and the skipped ones.

**Agreed** — and the same ambiguity is in two more places, so all three should move together.

**Fix**

* `charts/data-space-connector/values.yaml:1784-1785` — take the suggested wording:

  > References deeper than the limit are skipped with a warning naming them.

* `doc/release-notes/10-x.md`, the **Depth** row of the "What it does when enabled" table — reads
  "a deeper or cyclic composition is truncated"; that one is already unambiguous, no change needed.
* `doc/deployment-integration/roles/provider/COMPOSED_SPECIFICATIONS.md:197` — "references below the
  limit were **not** applied" has the same problem; change to "references deeper than the limit were
  **not** applied".

The first sentence of the same values comment ("how many specification levels below the ordered
`ProductSpecification` are resolved") uses "below" in the other sense and is correct there — I would
leave it, since the two readings no longer collide once the second sentence is reworded.

---

## Summary of the changes to make

1. merge `main`, bump `Chart.yaml` to `10.8.0` (R1) and retitle the release-note section (R2);
2. drop the interim `trusted-issuers-list` pin block and its companion sentence (R3);
3. drop the three `doc/tmforum/` links (R4);
4. `git rm RELEASE_NOTES_POLICIES_IN_SERVICE.md` (R5);
5. `git rm it/src/test/resources/policies/allowServiceSpec.json` and the claim about it (R6);
6. move the `createProductOffering` javadoc back to its method (R7);
7. clean `serviceCatalogManagement/v4/serviceSpecification` in `cleanUpTMForum()` (R8);
8. reword "below the limit" in `values.yaml` and `COMPOSED_SPECIFICATIONS.md` (R9).

Nothing here changes the chart templates or the contract-management behaviour, so
`helm unittest charts/data-space-connector` should stay green; the integration suite needs one run to
confirm R8.
