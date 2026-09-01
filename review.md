# Re-review — `wistefan/consent-owner-resolver` PR #1

**First pass:** `458f5b9` (+3363/-0, 34 files) — 28 findings.
**This pass:** `b0cf68b` (+6750/-0, 54 files) — 21 new commits, one per finding cluster.
**Verification:** the branch was rebuilt and re-probed locally. `go vet` clean;
`go test -race ./...` passes; coverage **76.9 % → 89.8 %**; `hack/license-header.sh check` passes.
Findings marked **[verified]** were reproduced by executing code, not read off the diff. Scratch
tests were deleted afterwards.

---

## Verdict

**All 28 findings from the first pass are addressed.** Not papered over — the fixes go at the cause,
they carry comments explaining *why*, and most come with a test that would catch a regression. A
few are better than what I asked for: the security scans now run twice (a non-blocking SARIF
reporting run plus a blocking verdict run), and the OpenAPI spec is enforced by a drift test that
validates real handler responses against it in both directions.

**One new issue is worth fixing before merge** (§N1): a caller can mint unbounded Prometheus label
values, with attacker-controlled text, through the unauthenticated `/metrics` endpoint. It was
introduced by the observability work — the one place where the redaction pattern applied everywhere
else has a gap. Four smaller residuals follow it.

---

## New findings

### N1. Unbounded, caller-controlled metric labels via `errorClass`  [verified] — fix before merge

`errorClass` (`internal/api/logging.go`) reduces an error to the part before the first `": "`, and
**falls back to the whole message when there is no separator**. Exactly one runtime error has no
separator, and it interpolates caller input:

```go
// internal/resolver/jsonpointer.go
return nil, badRequestf("unknown body encoding %q", b.Encoding)
```

`h.metrics.observeFailure(errorClass(err))` then uses that as a map key and a metric label. 25
crafted requests:

```
POST /resolve  {"body":{"encoding":"attacker-<n>"}}   x25
GET  /metrics
-> distinct failure-class series: 25
-> owner_resolver_resolve_failures_total{class="unknown body encoding \"attacker-0\""} 1
```

Two consequences: the `failures` map grows without bound (a slow memory-exhaustion DoS from a
single endpoint), and caller-supplied text lands in the metrics output — which `/metrics` serves
**without the shared secret**, on the stated grounds that *"labels deliberately carry no request
path and no owner"*. That guarantee does not currently hold. (The exposition format itself survives:
`%q` escaping nests correctly, so this is not an injection into the scrape.)

Worth fixing in both places, since they are independent:

* give the message a class prefix — `badRequestf("decode body: unknown encoding %q", b.Encoding)`;
* make `errorClass` return a fixed fallback (`"unspecified"`) rather than the whole message when no
  separator is found. That is the durable guard: it makes the label set closed by construction, so
  the next error message added without a prefix cannot reopen this.

A closed enum of classes would be stronger still, but the fallback gets the safety at a fraction of
the churn.

### N2. `cachingLookup` never evicts expired entries

`internal/resolver/contract_cache.go` — `load` treats an expired entry as a miss but leaves it in
the map, and `store` only ever overwrites. Nothing purges. The key is the contract id, so the map is
bounded by the number of distinct contracts the provider has ever served, which is fine in practice
and unbounded in principle. A sweep on write, or a size cap, closes it. Low priority — noting it
because the surrounding code is otherwise careful about bounds.

### N3. A prohibition in a *different* policy of the same contract is ignored

`findContractForTarget` checks permission and prohibition **within one policy**:

```
contract{ policy1: permission -> URI ; policy2: prohibition -> URI }  ->  governs = true   [verified]
```

The doc comment's reasoning — *"a policy that both permits and prohibits an asset is not a grant"* —
applies just as well one level up: a contract that prohibits the asset anywhere is not a grant for
it. Given the fail-closed posture everywhere else, hoisting the prohibition scan to the contract
level looks like the intended semantics. If the per-policy scope is deliberate, the comment should
say so.

### N4. The documented NetworkPolicy blocks the documented metrics scrape

README adds `GET /metrics` and says it is *"safe to scrape without the shared secret"*, but the
NetworkPolicy example immediately below admits ingress **only** from the APISIX pod on 8080 — so
Prometheus cannot reach it. Either add an ingress rule for the monitoring namespace to the example,
or serve metrics on a second port and say which. Small, but the two sections are ~40 lines apart and
contradict each other.

### N5. Stale bot message contradicts its own setting

`stale-issues.yml`: the message says *"open 30 days with no activity"*, `days-before-stale: 40`.
Pre-existing; I missed it the first time.

---

## First-pass findings — status

Every item verified against the current head.

### Blocking (4/4 resolved)

| # | Finding | Status |
|---|---|---|
| 1 | Release pipeline can never publish | **Fixed.** `main.yml` now grants `contents: write` + `pull-requests: read`, with a comment recording the reusable-workflow rule and the silent-skip failure mode. `build` added to the release `needs:`. |
| 2 | One facade round-trip per data item | **Fixed** [verified]. `perRequestResources` memoizes per request, `cachingLookup` adds a 30 s TTL across requests. 50 items now cost **1** `DataResources` call, down from 50. Contracts are deliberately *not* cached — they carry signature state — and the reasoning is in the comment. |
| 3 | `Contract.Status` parsed, never checked | **Fixed.** `Contract.IsSigned()` (case-insensitive, absent status = not signed), filtered in `SignedContracts`. `Prohibition` is now evaluated too; `Target` and `Action` carry a comment saying why they are not. See §N3 for the residual. |
| 4 | `/resolve` unauthenticated, no deployment guidance | **Addressed.** Optional `AUTH_TOKEN` bearer check with a constant-time compare and a required scheme; `/health` and `/metrics` deliberately exempt; a README section spelling out the oracle risk with a NetworkPolicy example; a startup log line when no token is set. |

### Should-fix (11/11 resolved)

| # | Finding | Status |
|---|---|---|
| 5 | Body decoded before rule selection | **Fixed** [verified]. `Payload` / `lazyPayload` decodes on first use; the `path` matcher now resolves cleanly through an undecodable base64 body. |
| 6 | Documented 400 vs. actual 422 | **Fixed.** `resolver.BadRequestError` + `errors.As` in the handler. |
| 7 | `contract` matcher undocumented | **Fixed.** README covers the matcher, `contractService`, `parties`, the signed-only rule and the cache TTL; `config/example-contract.json` added. |
| 8 | `containsPII` claim contradicted the code | **Fixed** [verified]. Comment corrected to "selects *which* resource, does not decide whether consent is checked", and `Parse` now *rejects* a `contract` rule with `consentRequired:false` rather than leaving the contradiction configurable. |
| 9 | `consentRequired:true` with zero claims | **Fixed** [verified]. A `static` matcher without an owner is a config error. |
| 10 | `claims` serialized as `null` | **Fixed** [verified]. `{"consentRequired":true,"scheme":"identifier","claims":[]}`. |
| 11 | EOL Go, EOL base image, gates that can't fail | **Fixed.** Go 1.26.7 (go.mod and Dockerfile pinned together, with a comment saying to bump both), runtime moved to `distroless/static-debian12:nonroot`. Each scanner now runs twice — non-blocking SARIF for the Security tab, then a blocking verdict (`govulncheck` reachable-only, `gosec`, Trivy CRITICAL). Tool versions pinned and confirmed to exist (`govulncheck@v1.7.0`, `gosec@v2.29.0`). |
| 12 | Archived / mutable actions | **Fixed.** Every third-party action pinned to a 40-char SHA with a trailing `# vN`; the three archived ones replaced; `.github/dependabot.yml` added for actions, gomod and docker. The only unpinned `uses:` left are local `./.github/workflows/*`, which is correct. |
| 13 | `pre-release.yml` red for fork PRs | **Fixed.** `if: github.event.pull_request.head.repo.full_name == github.repository` on all five secret-dependent jobs. |
| 14 | `.golangci.yml` referenced but absent | **Fixed.** Config added (revive, errorlint, nilerr, bodyclose, contextcheck, gosec, …, with test-file exclusions); linter version pinned. |
| 15 | `CONTRIBUTING.md` referenced but absent | **Fixed.** CONTRIBUTING, SECURITY, CODEOWNERS and CHANGELOG all added. |

### Lower priority (13/13 resolved)

| # | Finding | Status |
|---|---|---|
| 16 | Owner identifiers in logs | **Fixed.** `redactPath` logs `sha256:<8 hex>` — stable for correlation, not reversible. Verbatim paths only under `LOG_LEVEL=debug`, documented as temporary. |
| 17 | Error bodies leaked pointer config | **Fixed** [verified]. Responses are now `{"error":"cannot resolve owner"}` / `{"error":"invalid request body"}`; detail goes to the log. |
| 18 | `localize` could fetch an arbitrary host | **Fixed.** Always rebuilds from `baseURL + path` or returns an error; the empty-path case is called out by name. |
| 19 | Unbounded facade response reads | **Fixed.** Status checked first with a bounded drain for connection reuse, then `io.LimitReader(cap+1)` so an oversized body is reported rather than truncated into a confusing parse error. |
| 20 | Standard-alphabet base64 in path params | **Fixed.** `base64.RawURLEncoding`, pinned by `TestEncodeParticipant`, with a comment recording that this is a wire contract and why the standard alphabet was a hazard. |
| 21 | No scheme validation | **Fixed** [verified]. `defaultScheme:"identifer"` → `unknown scheme "identifer" (expected one of [identifier email did])`. |
| 22 | Empty `match` silently shadowed later rules | **Fixed** [verified]. Now a load error naming both rules and how to fix it. An empty match in the *last* rule is still allowed, which is right. |
| 23 | Duplicated item iteration | **Fixed.** `claimsPerItem` in `items.go`, used by both matchers. |
| 24 | `DisallowUnknownFields` on the request | **Fixed.** Request decoding is lenient, config decoding stays strict, and the comment explains the asymmetry via the separate release cycles. |
| 25 | Coverage gaps | **Fixed.** 76.9 % → 89.8 %; `main` refactored into a testable `run(ctx, settings, listener)`. |
| 26 | No observability | **Fixed.** Prometheus counters/histogram and `X-Request-Id` correlation (echoed, minted when absent, validated for length and control characters before it reaches a log). Hand-rolled exposition rather than a client library — the right call for a zero-dependency service. See §N1. |
| 27 | No OpenAPI spec | **Fixed.** `api/openapi.json` plus `openapi_test.go`, which checks real responses against the schema, flags undocumented properties *and* undocumented status codes, and asserts route coverage both ways. |
| 28 | CI nits | **Fixed.** `build` in the release `needs:`, `actions/stale` → v11 with a scoped token, `govulncheck-action` → v1.1.0, dependabot added. |

---

## Notes on the rework itself

* The fixes address causes rather than symptoms — `Payload` for §5 instead of reordering the decode,
  a config-load rejection for §8/§9 instead of a runtime guard, `errorClass`/`redactPath` as
  reusable primitives for §16/§17.
* Comments now record the *reasoning*, not just the change: why contracts are not cached but
  catalog resources are, why raw-URL base64 rather than standard, why request decoding is lenient
  while config decoding is strict, why the security scans run twice. That is the kind of comment
  that survives the next refactor.
* §N1 is a fair illustration of how these things go: the redaction discipline is applied
  consistently in `redactPath`, the metric key design, and the error bodies — and then one error
  message added elsewhere slips through the one code path that has a permissive fallback. The
  fallback, not the message, is the thing to fix.

## Merge recommendation

Fix **§N1** (two small edits) and merge. §N2–§N5 are fine as follow-up issues.
