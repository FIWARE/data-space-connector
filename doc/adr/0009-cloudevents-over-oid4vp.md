# 0009 - Carry data space events as CloudEvents authenticated over OID4VP

**Status:** Accepted
**Date:** 2026-11-20
**Context doc:** [`../eventing/specification.md`](../eventing/specification.md)

## Context

When the data space authority removes a member from the Trusted Issuers Registry (TIR), every
participant should learn about it and clean up its local Trusted Issuers List (TIL). That needs an
event mechanism between the entities of a data space. The participants and the authority already
authenticate to each other with verifiable credentials over OID4VP: the central-marketplace
notification works that way, and so does contract-management's `/order` callback. Some data spaces
include parties that do not run FIWARE components, so the wire contract has to be implementable
without FIWARE code.

## Decision

- **Envelope.** An event is a **CloudEvents 1.0** event with a reverse-DNS `type`
  (`org.fiware.dataspace.<entity>.<past-tense-verb>`) and the publisher's participant identifier as
  `source`. The extension attribute `dataspaceid` is optional: most data spaces have no identifier of
  their own, so the publisher's `source` is what tells data spaces apart.
- **Use the specified protocols.** Events are sent in the structured mode of the CloudEvents
  HTTP Protocol Binding 1.0.2. Callbacks are confirmed with the CloudEvents webhook handshake
  ([ADR 0013](0013-cloudevents-webhook-handshake.md)). The subscription request borrows the shape of
  the CloudEvents Subscriptions API without claiming conformance to it. The webhook document is
  `1.0.3-wip`, so the specification pins it to a commit.
- **Authentication.** The sender presents a credential it holds over OID4VP at the **receiver's**
  verifier and sends the returned access token as a bearer, as for the central-marketplace
  notification. The receiver's PEP verifies the token.
- **Component.** Both ends live in **contract-management**, in a publisher and a listener mode.
  It already receives authenticated events, already sends them over OID4VP, and already registers
  callbacks at start-up with retry and a health indicator.
- **Configuration namespace.** The configuration lives under `dataspace.events`, not under
  the component's name.

## Consequences

- The [interoperability specification](../eventing/specification.md) is short: its normative
  references are released or pinned CloudEvents and OID4VP documents, and only the
  FIWARE-specific parts (types, the authentication binding, the authorization rule) are its own.
- A receiver needs no new trust mechanism: the events path is one more service at the verifier it
  already runs, with an ODRL policy at the PEP.
- contract-management now carries more than contract management, so it may have to be **renamed**
  one day. The `dataspace.events` namespace keeps that cheap: the deployed configuration does not
  change with the component's name.
- Delivery is best-effort with leased in-memory subscriptions, so contract-management stays without
  a database ([ADR 0015](0015-best-effort-stateless-event-delivery.md)).

## Alternatives

- **A TMForum hub/listener-shaped API, or a bespoke JSON body.** Either would be a FIWARE-local
  wire protocol, where specifications for `application/cloudevents+json` exchange already exist.
- **A mandatory data space id.** Most data spaces have none.
- **A dedicated `dataspace-events` service.** It would duplicate what contract-management already
  does (OID4VP client, authenticated callbacks, registration with retry).
- **Extending the trusted-issuers-list.** It is only one TIR implementation among several
  ([ADR 0010](0010-detect-removal-by-diffing-the-registry.md)).
- **`contract-management.events.*`** as configuration namespace, or renaming the component along
  with the feature.
