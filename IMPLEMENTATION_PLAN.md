# Implementation Plan: [DSC] Interoperability specification for data space eventing

## Overview
Write `doc/eventing/specification.md`: the interoperability contract that a second, non-FIWARE
implementation needs in order to publish events to, or subscribe at, a FIWARE participant. Link it
from `doc/README.md`. Steps 1–3 are docs-only: they touch no chart, values or schema files. (The
plan PR itself also carries the intended CLAUDE.md → AGENTS.md rename and the three comment lines
that point to it; that is repository housekeeping, not part of the eventing steps.)
The spec is normative and kept short. The rationale stays in `doc/eventing/architecture.md`, and the
wire details follow FIWARE/contract-management `api/dataspace-events.yaml` (main @ `c7da3c4`).

**Sources the step agents must read** (none of them may be edited, apart from the files named in a step):
- Design: `doc/eventing/architecture.md` §4 (event model), §5.1/§5.2 (APIs, handshake),
  §6 (trust and authorization), §7 (delivery), §12 D-2/D-5/D-7/D-8/D-9; `doc/eventing/questions.md`
  (Q10: no ODRL left operand for the issuer); `doc/eventing/ticket.md` section "DSC-3".
- API of record: `git clone https://github.com/FIWARE/contract-management /tmp/cm` (read-only, outside
  the repo). Use `api/dataspace-events.yaml` and these classes in `src/main/java/org/fiware/iam/dataspace/`:
  `WebhookHeaders`, `WebhookHandshake` (accepts any 2xx; `WebHook-Allowed-Origin` must equal the
  origin, case-insensitive, or `*`; parses `WebHook-Allowed-Rate`, where `*` means unlimited),
  `EventListener`, `EventAuthorizer`, `EventDeduplicator`, `EventDelivery` (sends
  `WebHook-Request-Origin` on every delivery; retries on 5xx/408/429; a `410 Gone` from the sink drops
  the subscription), `SubscriptionController`, `ParticipantRemovedHandler`, `TirWatcher`,
  `ParticipantResolver` (CM does not verify the token: the API gateway passes the verified claims in
  a header, `X-Userinfo` by default, and CM reads the caller's id from the top-level claim
  `dataspace.events.listener.identity.participant-claim`, `participantId` by default).
- Verifier claim mapping: `jwtInclusion` in the credentials-config-service API
  (https://github.com/FIWARE/credentials-config-service, `api/credentials-config-service.yaml`) is
  what puts a credential field into a top-level claim of the verifier's access token.
- **Where architecture.md and the API differ, the API wins.** Known differences:
  - status codes: the API has 415, 500 and 503 + `Retry-After`, and no 422;
  - the API returns 202 for duplicates, unknown types and self-events;
  - the defaults differ (dedup 1h, lease 24h, cap 10 per subscriber). Defaults are implementation
    choices and must not be specified as normative values.

**Link check in the agent container.** Every step's acceptance requires
`.github/scripts/check-links.sh` to pass. That script runs lychee in Docker with a bind mount of
`$(pwd)`; the Docker sidecar cannot see the workspace, so it fails with "lychee.toml not found". That
is a tooling failure, not a doc defect: do not report it as one. Run the same check with the static
musl lychee 0.24.2 binary instead (the gnu build needs glibc 2.38+):
`lychee --config lychee.toml --root-dir "$(pwd)" '**/*.md'`. It must report 0 errors.

## Steps

### Step 1: Bring the eventing design documents onto the work branch
`ticket-97/work` was cut from `main` and does not contain `doc/eventing/`. The spec links into
`architecture.md` §-anchors, and the lychee link check (`.github/scripts/check-links.sh`, offline,
`include_fragments = "anchor-only"`) fails on dangling relative links. Step 2 therefore needs those files on the branch.

- If `doc/eventing/architecture.md` already exists on `ticket-97/work` (design PR #2 has landed
  upstream and the branch has been updated), this step has nothing to do. Report that and stop.
- Otherwise run `git fetch http://gitea-http.gitea.svc.cluster.local:3001/local-claude/data-space-connector.git step-1-eventing-design`
  and `git merge --no-ff FETCH_HEAD`. Merge, do not cherry-pick: that keeps the commit SHAs, so a
  later merge of PR #2 is a no-op, but only if PR #2 lands with a merge commit and gets no further
  commits after this step. If it is squash- or rebase-merged, or changes again, the final ticket PR
  must first re-sync `doc/eventing/{architecture,ticket,questions}.md` from upstream (see below). Its base `topic/participant-eventing` is already an ancestor of
  `main`, so the merge adds only `doc/eventing/{architecture,ticket,questions}.md`. Check that with
  `git diff --stat HEAD~1`. Do not edit their content.
- Acceptance: the three files are present and byte-identical to `step-1-eventing-design`, no other
  file changes, and the link check passes (see "Link check in the agent container"). If lychee
  itself reports broken links, they must come from the design docs. Report them and do not fix them
  in this step.
- Before the final `ticket-97/work` PR goes upstream: if PR #2 has landed in a way that changed the
  SHAs (squash/rebase) or its content after this step, replace the three files with the upstream
  version (`git checkout <upstream main> -- doc/eventing/{architecture,ticket,questions}.md`), so
  the ticket diff carries no design-doc changes of its own.

### Step 2: Write `doc/eventing/specification.md`
Create the spec. It is normative, uses RFC 2119 keywords (MUST/SHOULD/MAY, with a one-line
conventions note), and has a doctoc ToC like `architecture.md`. Aim for about 200–300 lines. Explain
nothing that is not needed for interoperability; for the reasons, link to `architecture.md#…`.
Sections:

1. **Status and scope.** Version `1.0` (draft), date, and the source API version (`dataspace-events.yaml`
   `0.0.1`, contract-management `c7da3c4`). Say that the document is not a submission to a governance
   body, that DSBA, Gaia-X and IDSA are unaffected, and that it is the document to hand over if one
   ever asks. Name the roles: *publisher* and *receiver/subscriber*.
2. **Normative references.**
   - CloudEvents 1.0.2 core.
   - HTTP Protocol Binding 1.0.2, structured mode.
   - JSON Event Format.
   - HTTP 1.1 Web Hooks for Event Delivery, `1.0.3-wip`: pinned, and only §4 (abuse protection) is used.
   - CloudEvents Subscriptions API: the shape is borrowed, conformance is **not** claimed.
   - OID4VP.
   Link the cloudevents/spec documents at their tagged versions (`v1.0.2`). External URLs are not
   link-checked, so get them right by hand.
3. **Event format.** CloudEvents attributes table:
   - `specversion`, `id`, `source` and `type` are REQUIRED. `subject`, `time`, `datacontenttype` and
     `dataschema` are optional.
   - `id` is unique per `source`.
   - `source` is the publisher's participant identifier. Any DID method is allowed.
   - `dataspaceid` is an OPTIONAL extension. If the event carries it and the receiver has a data
     space id configured, the two MUST be equal; otherwise the receiver answers `403`
     (`EventAuthorizer.isOtherDataspace`). If either side leaves it blank, it is not checked.
   - Transport: structured mode only, `Content-Type: application/cloudevents+json`. Binary mode → 415.
   - Type naming: `org.fiware.dataspace.<entity>.<past-tense-verb>`. A breaking payload change is a new type.
   - A receiver MUST accept and ignore unknown types (`202`), after authentication and the
     `source`/`dataspaceid` checks of §6, but before the publisher×type check. The `202` applies
     only to a complete event: an incomplete event of an unknown type gets `400` (§6).
4. **Event types.**
   - `org.fiware.dataspace.participant.removed`:
     - `subject` is the removed participant's identifier.
     - `data.participantId` (string, equal to `subject`) is the only defined `data` member, and the
       publisher MUST send it. The reference publisher (`TirWatcher.java:233`, `c7da3c4`) sends
       `data = {participantId}` and nothing else. The receiver identifies the participant from
       `subject`, and falls back to `data.participantId` (`ParticipantRemovedHandler`).
     - architecture §4 also lists `observedAt` and `registry`. The API does not have them, so they are
       OPTIONAL at most: a publisher MAY send them, a receiver MUST ignore unknown `data` members and
       MUST NOT depend on these two. Put this gap in the "Relation to the architecture" note.
     - There is deliberately no `reason`.
     - Give a JSON example that matches what the reference publisher sends (§4's example minus
       `observedAt`/`registry`), not the §4 example verbatim.
     - Rule for any other mismatch between architecture §4 and the code: the code wins. A field the
       reference publisher does not emit is never REQUIRED.
   - Reserved names, not specified: `participant.registered`, `participant.credentials.changed`,
     `participant.suspended`, `service.configuration.provided`.
5. **Authentication binding.**
   - The publisher obtains an access token over OID4VP at the **receiver's** verifier, i.e. the
     verifier that protects the receiver's events path. It sends the token as `Authorization: Bearer`.
   - It presents a verifiable credential of the type the receiver admits on that path. The type is
     local configuration of the receiver, not a data space-wide parameter.
   - A receiver answers `401` when the token is missing or invalid, and the publisher then runs the
     flow.
   - The same binding applies to subscription calls, in the opposite direction: the subscriber
     authenticates at the publisher's verifier. For delivery, the publisher uses the subscriber's
     `protocolsettings.clientId`/`scopes`.
   - **Publisher identity.** State normatively which value is compared with `source`: the
     `credentialSubject.id` of the presented credential (the holder's DID). The publisher MUST present
     a credential whose `credentialSubject.id` equals the `source` it sends. The receiver MUST configure
     its verifier so that exactly this value reaches the comparison. How it gets there is receiver-local
     and informative only: in a FIWARE receiver, `jwtInclusion` maps `credentialSubject.id` to the
     top-level token claim `participantId`, the gateway passes the claims in `X-Userinfo`, and CM reads
     `participant-claim`. Before writing this, check in the credentials-config-service API that
     `jwtInclusion` can map `credentialSubject.id` to a top-level claim. If it cannot, do not invent
     a mapping: say that the claim is receiver-local and that the publisher learns it from the
     receiver's verifier metadata (the presentation definition / DCQL query of the events scope).
   - Describe how a client discovers the verifier exactly as contract-management's OID4VP client
     does; do not invent a new discovery mechanism. `doc/CENTRAL_MARKETPLACE.md` step 4 only says
     that the client authenticates at the verifier, so take the mechanism from the code:
     `org.fiware.iam.http.Oid4VpAuthHandler` reacts to a `401` by calling the OID4VP client with the
     called URL's `scheme://host[:port]`, the request path, `clientId` and `scope`. Read the FIWARE
     OID4VP client library it uses to find which well-known metadata URL it fetches from those
     values, and specify that URL.
6. **Authorization rule** (architecture §6, D-5). The receiver admits only tokens issued by its
   own verifier, then applies the checks below **in this order**; the first that matches decides the
   answer. The order is normative, because it decides which status a publisher gets when several
   checks fail. Copy it from `EventAuthorizer.authorize` + `EventListener.process` (c7da3c4) and
   re-check it there:
   1. no authenticated identifier (missing/invalid token) → `401`;
   2. authenticated identifier (§5, the credential's `credentialSubject.id`) ≠ `source` → `403`;
   3. `dataspaceid` present on both sides and different (§3) → `403`;
   4. `type` known to the receiver and publisher not configured for that `type` → `403`
      (skipped for an unknown type);
   5. `specversion`, `id`, `source` or `type` missing → `400`;
   6. unknown `type` (the event is complete) → `202`, dropped, not deduplicated;
   7. deduplication on (`source`, `id`): a repeat → `202`; first copy still in flight → `503` +
      `Retry-After`;
   8. `subject` is the receiver's own identifier → `202`, no action (counted for dedup);
   9. otherwise dispatch: `202`; a handler rejecting the event as invalid → `400`, a downstream
      failure → `502`, any other failure → `500`.

   Items 1–4 are `EventAuthorizer.authorize`; `EventListener.process` returns their rejection
   before it checks completeness, then returns the unknown-type `202`. Spell out the consequences in
   the spec:
   - an unconfigured publisher that sends a complete event of an unknown type gets `202`, not `403`;
   - an incomplete event of an unknown type gets `400`, not `202`;
   - a malformed event whose `source` differs from the token gets `403`, not `400` (a missing
     `source` is a mismatch too);
   - a malformed event of a known type from a publisher not configured for it gets `403`, not `400`.

   Note that an ODRL left operand for the credential **issuer** does not exist today. It would only
   add defence in depth (a possible PAP contribution, Q10), and it is not a dependency.
7. **Receiving endpoint** `POST /events`.
   - List the request headers: `Authorization`, `Content-Type`, `WebHook-Request-Origin`.
   - Give the full status-code table, exactly as in `dataspace-events.yaml`:
     202/400/401/403/415/500/502/503.
   - Say which codes the publisher retries (5xx, 408, 429) and that `410 Gone` ends the subscription.
   - Delivery is at-least-once and best-effort, with no ordering. Receivers MUST be idempotent.
   - The path is relative to a base URL that the receiver announces (the `sink`).
8. **Subscription profile.**
   - `POST /subscriptions`:
     - The request fields are `subscriberId`, `sink`, `types` (minItems 1), `protocol` (`HTTP` only),
       and `protocolsettings` with `clientId` (required) and `scopes`.
     - The `201` response is `Subscription` with `id` and `expiresAt`.
     - The `400` response is `SubscriptionError {reason}`.
     - Give the check order from the schema description: 401 → missing `subscriberId` 400 →
       mismatch 403 → other fields 400. Also list 503.
     - The same subscriber + sink renews the lease in place. Subscribers MUST renew before `expiresAt`
       and MUST re-subscribe after a publisher restart, because leases are not durable.
   - `DELETE /subscriptions/{subscriptionId}`: 204/401/404.
   - A table maps the Subscriptions API fields that are used against those that are not: no
     `filters`, `config`, `source`, `subscriptionId` in the request, and no `GET`/`PUT`/discovery.
9. **Webhook validation handshake**, pinned because the source document is `1.0.3-wip`:
   - Before it activates a subscription, the publisher sends `OPTIONS <sink>` with
     `WebHook-Request-Origin: <publisher DNS name>`.
   - The sink MUST answer 2xx with `WebHook-Allowed-Origin` equal to that origin (case-insensitive)
     or `*`, and MAY add `WebHook-Allowed-Rate` (an integer, or `*`); also `Allow`.
   - Receiver side: the `OPTIONS` call is unauthenticated. The sink MUST include
     `WebHook-Allowed-Origin` only when the requesting origin belongs to a configured publisher, and
     otherwise answers without it (`EventListener.validateWebhook`, `dataspace-events.yaml`
     `validateWebhook`). Put this rule into the receiver conformance list as well.
   - Any other answer leaves the subscription inactive (400 to the subscriber).
   - Every delivery repeats `WebHook-Request-Origin`.
   - The publisher honours `WebHook-Allowed-Rate`.
   - Callback-request validation (`WebHook-Request-Callback`) is NOT used.
   - The origin is configured together with the identifier and never derived from it (D-8).
10. **Not specified.**
    - The receiver's reaction to an event: local policy (architecture §9).
    - How a publisher detects a removal (§8).
    - Reconciliation (§10).
    - The publisher list configuration and the chart values.
    - Payload signatures (D-7; a future `signature` extension would be additive).
    - A catch-up feed and brokers (§5.3).
11. **Conformance checklist.** Two short bullet lists, one for a publisher and one for a receiver.

Acceptance:
- Every status code, header, field name and required flag matches `dataspace-events.yaml` and the
  classes listed above. List any remaining deviation from architecture.md in one "Relation to the
  architecture" note, not inline.
- Every `architecture.md#…` anchor resolves.
- The link check passes (`.github/scripts/check-links.sh`, or the lychee fallback above).
- There is no chart change and no ADR. D-9 already records the decision; the ADR is the separate
  DSC ticket.

### Step 3: Link the specification from `doc/README.md`
- `doc/README.md`:
  - Add a `### Data Space Eventing` subsection under `## Details about flows and interfaces`,
    after `### Consent Management`. It gets bullet links to
    `[Architecture](./eventing/architecture.md)` and
    `[Interoperability specification](./eventing/specification.md)`, with a one-line description each.
  - Add the matching ToC entry by hand, between the doctoc markers, in the existing indentation
    style. If `doctoc` is available, run it instead.
- Do **not** edit `doc/eventing/architecture.md`. It arrives by merge in Step 1, and while design
  PR #2 is still open any edit here would conflict with its review changes. The back-links from
  §5.2 and the D-9 row to `specification.md` belong in PR #2 or in a follow-up after it has landed;
  note that in the Step 3 PR description.
- Acceptance: the link check passes (`.github/scripts/check-links.sh`, or the lychee fallback
  above), and the ToC entry anchor resolves.
