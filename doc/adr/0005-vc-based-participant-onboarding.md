# 0005 - Onboard participants with a verifiable credential, not client credentials

**Status:** Proposed (design only - not implemented)
**Date:** 2026-08-26
**Applies to:** `FIWARE/data-space-connector`, Prometheus-X/Visions consent-manager
**Context doc:** [`../CONSENT_MANAGEMENT.md`](../CONSENT_MANAGEMENT.md)

## Context

[ADR-0001](0001-oid4vp-for-participant-authentication.md) made participants authenticate to the
consent-manager over OID4VP, and [ADR-0004](0004-map-oid4vp-tokens-to-participants-by-did.md) made
the token subject resolve to a `Participant` by its `did`. Both rest on an assumption that onboarding
does not currently justify: that the `did` on the participant record belongs to that participant.

`POST /participants` requires `clientID` and `clientSecret`. That requirement is inherited, not
needed here: they are the participant's service credentials in the **VisionsTrust PDI network**, used
*outbound* during onboarding when `dataspaceEndpoint` is set (`registerParticipant` logs into the
participant's own dataspace service with them to publish its consent-signature key). The DSC never
sets `dataspaceEndpoint`, so that path is dead - yet the fields remain mandatory in both the Mongoose
and the Joi schema, while `did` is optional in Joi.

Three properties of the endpoint as it stands:

* It has **no authentication middleware** at all. Anyone who can reach it may onboard a participant
  claiming any `did`, `legalName` and `selfDescriptionURL`.
* The secret is stored **in plaintext**; login is a direct equality match.
* The secret doubles as an **HMAC signing key** in the `serviceKey` branch of
  `verifyParticipantJWT`.

The consequence that matters: the `did` uniqueness guard added for ADR-0004 stops two participants
*sharing* a DID, but nothing stops the first one claiming a DID that is not theirs. Identity is
asserted at onboarding and trusted at authentication.

## Decision

Onboarding **proves** the identity it registers, rather than accepting it as body fields:

* `POST /participants` is authenticated over OID4VP, requiring a credential that authorizes joining
  (a `LegalPersonCredential` or dataspace-membership credential) whose issuer is trusted via the TIR.
* The participant's identity is **derived from the presented credential** - `did` from the holder
  DID / `credentialSubject.id`, `legalName` from the credential - not from the request body.
* `clientID`/`clientSecret` become optional and unused; `did` becomes required.

This is the same shift ADR-0004 made one step earlier in the chain: stop treating an identifier as a
value someone hands you, and start treating it as something a presentation proves.

## Consequences

**Positive**

* Closes the claim-any-DID hole, which is the weakest link in the participant auth chain: an
  unauthenticated onboarding endpoint undermines an otherwise cryptographic path.
* Makes the `did` uniqueness guard structurally redundant - a DID you cannot prove is a DID you
  cannot register (the guard stays as defence in depth).
* Removes the last participant secret from the system, and with it the plaintext storage and the
  HMAC-key double duty.
* One trust model end to end: the credential that gets you in is the credential you authenticate
  with.

**Negative**

* Requires upstream changes in the Prometheus-X consent-manager - schema relaxation is easy, but
  removing `/participants/login` and the `serviceKey` branch may break other deployments that still
  use PDI federation.
* Onboarding gains a dependency on the verifier and the TIR, so a participant cannot be created
  while either is down.
* The demo flow's step 3a gets longer: a credential must be obtained before onboarding, where today
  two literals suffice.

**Bootstrap ordering (not a drawback, but a limit)**

To present a membership credential you must already hold one, so this proves *"the authority already
vouches for this DID"*, not *"this stranger should be admitted"*. Admission remains a governance
decision, expressed as who gets issued a credential. In the DSC that ordering already holds: the
participant's own keycloak issues the credential and TIR registration is an earlier, separate step.

## Alternatives considered

- **Keep client credentials, just authenticate the endpoint** (e.g. an authority admin token).
  Cheaper, and it does close the open-endpoint hole. Rejected as the target state because the body
  would still *assert* the `did`: an authenticated operator could still onboard a participant under
  someone else's DID, and the credentials would remain as a second trust model. Worth doing as an
  interim step if the upstream change stalls.
- **Validate the claimed `did` out of band** - resolve the DID document and check for a proof of
  control at onboarding time. Achieves proof-of-control without a credential, but it is a second
  bespoke verification mechanism next to the OID4VP one already in place, and it says nothing about
  whether the DID's owner is entitled to join.
- **Leave onboarding as an operator task outside the API.** Defensible for a small dataspace and
  effectively what the demo does. Rejected as a design: it does not scale to self-service onboarding
  and leaves the unauthenticated endpoint reachable regardless.
- **Drop the fields but keep the endpoint open.** Tidier schema, same hole. The schema is the small
  part of this decision; the authentication is the point.
