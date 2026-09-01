# 0004 - Map OID4VP tokens to participants by DID

**Status:** Accepted (implemented)
**Date:** 2026-08-26
**Applies to:** `FIWARE/data-space-connector`, Prometheus-X/Visions consent-manager
**Context doc:** [`../CONSENT_MANAGEMENT.md`](../CONSENT_MANAGEMENT.md)

## Context

Once the authority's gateway verifies an OID4VP access token
([ADR-0001](0001-oid4vp-for-participant-authentication.md)), the consent-manager still has to know
*which* `Participant` is calling: its handlers resolve the caller to a `Participant` document
(`verifyParticipantJWT` reads the participant id from the token it issued itself).

Two relevant facts:

- The consent-manager **already** verifies externally-issued OID4VP tokens and maps them to a local
  identity - that is how the *data subject* path works (`consentManager.externalIdp` →
  `src/libs/jwt/externalVerifier.ts`, mapping the token `sub` to a `User`).
- `Participant` **already carries a `did`** field: a live `GET /participants/me` returns
  `"did":"did:web:mp-operations.org"`.

## Decision

A trusted external OID4VP token resolves to a participant by **DID**:
`Participant.findOne({ did: token.sub })`, reusing the existing external-token verification (issuer
allow-list, OIDC discovery, JWKS). The self-issued HS256 branch is removed once nothing issues those
tokens.

`did` is the participant analogue of the subject's `UserIdentifier.identifier`. There is no separate
identifier collection for participants and none is needed: a subject has *many* identifiers (one per
participant it is registered at, which is what the identifier matcher links), whereas a participant
has one identity, so its DID lives directly on the record.

Because that field is what authentication resolves through, a DID must not be claimable twice - two
participants sharing one would make the lookup arbitrary, i.e. one participant authenticated as
another. That is enforced **at write time**, in `registerParticipant`, beside the `clientID` check
that already returns `409`; only non-empty DIDs are checked, since `""` is the field's default.

## Consequences

**Positive**

- Symmetric with the subject path that already works in production here - same verification code,
  different collection and claim target.
- The DID is the identifier that is actually key-bound and verifiable, so the mapping rests on
  something the presentation proves rather than on a name or a URL.
- No new identifier has to be minted, distributed or kept in sync.

**Negative**

- Onboarding must populate `did` - it defaults to `""` today, and a participant without one simply
  cannot use this path.
- The uniqueness guard is a controller check, not a database constraint: a direct DB write or two
  racing registrations can still create duplicates. Accepted for consistency with the rest of the
  codebase (see the alternatives) and because the write path is single and administrative.
- The token's `aud` must be validated, not just its signature, or a token minted for another service
  of the same verifier would be accepted.

## Alternatives considered

- **Gateway mints a consent-manager JWT after verifying the OID4VP token.** Needs no upstream
  change, which is its whole appeal. Rejected: the gateway would have to hold the consent-manager's
  `jwtSecret` and mint identities, which recreates the shared-signing-secret problem
  [ADR-0001](0001-oid4vp-for-participant-authentication.md) sets out to remove, and hides the real
  caller identity from the application that makes authorization decisions.
- **A separate token-exchange service** doing the same mapping. Same objection, plus a component.
- **A partial-unique index on `did`** (`partialFilterExpression: { did: { $gt: "" } }`), instead of
  the controller guard. Strictly stronger - it also stops direct DB writes and racing inserts.
  Rejected on consistency and migration risk: the codebase declares **no** `unique:` or `.index()`
  in any model, the subject analogue `UserIdentifier.identifier` is likewise unindexed, and a unique
  index fails to build on a database that already holds duplicate DIDs, whereas a guard degrades
  gracefully. Revisit if participants ever become self-registering or high-volume.
- **Match on `selfDescriptionURL` instead of DID.** It is already the consent-manager's join key for
  participants, so it is tempting. Rejected: it is a URL, not key-bound - the presentation proves
  control of the DID, not of a hostname - and it re-creates the DID↔SD translation that the
  consent-plugin already has to do in the other direction (`ParticipantSelfDescriptionByDID`).
  Recorded as a related upstream concern: the consent receipt's `partyId` has the same
  identifier-as-URL problem.
