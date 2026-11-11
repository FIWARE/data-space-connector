# Implementation Plan: [DSC] Interoperability specification for data space eventing

## Overview
Write `doc/eventing/specification.md`: the interoperability contract that a second, non-FIWARE
implementation needs in order to publish events to, or subscribe at, a FIWARE participant. Link it
from `doc/README.md`. This is a docs-only change. No chart, values or schema files are touched.
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

## Steps

### Step 1: Bring the eventing design documents onto the work branch
`ticket-97/work` was cut from `main` and does not contain `doc/eventing/`. The spec links into
`architecture.md` §-anchors, and the lychee link check (`.github/scripts/check-links.sh`, offline,
`include_fragments = "anchor-only"`) fails on dangling relative links. Step 2 therefore needs those files on the branch.

- If `doc/eventing/architecture.md` already exists on `ticket-97/work` (design PR #2 has landed
  upstream and the branch has been updated), this step has nothing to do. Report that and stop.
- Otherwise run `git fetch http://gitea-http.gitea.svc.cluster.local:3001/local-claude/data-space-connector.git step-1-eventing-design`
  and `git merge --no-ff FETCH_HEAD`. Merge, do not cherry-pick: that keeps the commit SHAs, so the
  later merge of PR #2 is a no-op. Its base `topic/participant-eventing` is already an ancestor of
  `main`, so the merge adds only `doc/eventing/{architecture,ticket,questions}.md`. Check that with
  `git diff --stat HEAD~1`. Do not edit their content.
- Acceptance: the three files are present and byte-identical to `step-1-eventing-design`, no other
  file changes, and `.github/scripts/check-links.sh` passes. If it fails, the failure must be caused
  by the design docs themselves. Report it and do not fix it in this step.

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
   - `dataspaceid` is an OPTIONAL extension. No receiver behaviour depends on it.
   - Transport: structured mode only, `Content-Type: application/cloudevents+json`. Binary mode → 415.
   - Type naming: `org.fiware.dataspace.<entity>.<past-tense-verb>`. A breaking payload change is a new type.
   - A receiver MUST accept and ignore unknown types.
4. **Event types.**
   - `org.fiware.dataspace.participant.removed`:
     - `subject` is the removed participant's identifier.
     - `data` has `participantId` (string, equal to `subject`), `observedAt` (RFC 3339) and `registry`
       (the URL of the trusted issuers registry it was removed from).
     - There is deliberately no `reason`.
     - Include the §4 JSON example.
     - Check the field names and the `subject`/`data.participantId` fallback against
       `ParticipantRemovedHandler` and `TirWatcher`.
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
   - Describe how a client discovers the verifier exactly as the central-marketplace flow does
     (`doc/CENTRAL_MARKETPLACE.md` step 4; check the contract-management OID4VP client path). Do not
     invent a new discovery mechanism.
6. **Authorization rule** (architecture §6, D-5). The receiver MUST, in this order:
   - (a) admit only tokens issued by its own verifier;
   - (b) reject with `403` if the authenticated identifier (§5, the credential's
     `credentialSubject.id`) ≠ `source`, or if that publisher is not
     configured for that `type`;
   - (c) ignore events whose `subject` is the receiver's own identifier: return `202` and do not act.

   Deduplicate on (`source`, `id`): a repeat gets `202`, and `503` + `Retry-After` while the first
   copy is still in flight.

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
- `.github/scripts/check-links.sh` passes.
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
- Acceptance: `.github/scripts/check-links.sh` passes, and the ToC entry anchor resolves.
