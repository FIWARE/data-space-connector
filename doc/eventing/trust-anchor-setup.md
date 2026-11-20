# Data Space Eventing — Trust Anchor Setup

The data space authority keeps the list of the data space's members, the Trusted Issuers Registry
(TIR). When it removes a member there, the participants should learn about it and stop trusting the
removed organization. **Data space eventing** delivers that news: a *publisher* at the trust anchor
watches the TIR and sends a `org.fiware.dataspace.participant.removed` event to every participant
that subscribed.

This guide covers the publisher. The participant side is described in the
[participant setup](participant-setup.md); the mechanism in [how it works](architecture.md) and the
[interoperability specification](specification.md); the local demo in [EVENTING.md](../EVENTING.md).

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [What the publisher does](#what-the-publisher-does)
- [Where it runs](#where-it-runs)
- [Requirements](#requirements)
- [Enabling](#enabling)
  - [1. A publisher-only contract-management](#1-a-publisher-only-contract-management)
  - [2. Present the authority's credential](#2-present-the-authoritys-credential)
  - [3. Register the subscription service at the verifier](#3-register-the-subscription-service-at-the-verifier)
  - [4. Expose `/subscriptions` at the gateway](#4-expose-subscriptions-at-the-gateway)
  - [5. Tell the participants](#5-tell-the-participants)
- [Who a subscriber is](#who-a-subscriber-is)
- [The TIR watcher](#the-tir-watcher)
- [Timing: production defaults and the k3s values](#timing-production-defaults-and-the-k3s-values)
- [Removing a participant](#removing-a-participant)
- [Checking the publisher](#checking-the-publisher)
- [contract-management pitfalls](#contract-management-pitfalls)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## What the publisher does

The publisher is a contract-management (CM) instance with two features on:

- the **publisher** accepts subscriptions (`POST /subscriptions`, `DELETE /subscriptions/{id}`) and
  delivers events to the subscribers' sinks. Before delivering, it authenticates at each
  participant's verifier with the authority's credential (OID4VP);
- the **TIR watcher** reads the TIR periodically, compares it with the previous read and announces
  every DID that disappeared.

An operator can also announce a removal at once with a manual publish
([Removing a participant](#removing-a-participant)).

## Where it runs

The publisher is the `contract-management` of the authority's own DSC release, the one that already
holds the authority's identity, verifier and API gateway. In k3s this is the `consent-authority`
release in namespace `trust-anchor`, extended by
[`k3s/eventing-trust-anchor.yaml`](../../k3s/eventing-trust-anchor.yaml). The `trust-anchor` chart
(TIR and TIL) is not changed.

The authority release brings everything the publisher needs:

- the identity `did:web:dataspace-authority.org`, its key (secret `dataspace-authority.org-tls`) and
  its OperatorCredential (secret `vc-operator-credential`, kept by vc-operator). The publisher
  presents this credential to the participants. No new key or credential is created at the trust
  anchor, but each participant's TIL must list the authority as issuer of `OperatorCredential` with
  `scope: ""` ([participant setup](participant-setup.md#3-register-the-verifier-service));
- a verifier, at which subscribers get their token for `/subscriptions`;
- an APISIX gateway, which checks that token in front of CM.

A dedicated release would need a verifier and a gateway of its own; a dependency in the
`trust-anchor` chart would mix the publisher's identity into the registry's chart
([ADR 0014](../adr/0014-publisher-in-the-authority-release.md)).

In k3s the authority release also runs the consent-manager stack, because the eventing demo builds on
the consent deployment. A production publisher does not need it.

## Requirements

- contract-management ≥ 3.5.4 (chart 10.16.0 ships it).
- A TIR that CM can enumerate with `GET /v4/issuers`, the EBSI-compatible read API of the FIWARE
  trusted-issuers-list.
- An identity for the authority: a DID, its key and a credential the participants admit on their
  `/events` route (`OperatorCredential` in k3s, see
  [participant setup, step 4](participant-setup.md#4-create-the-odrl-policy)).
- A verifier and an APISIX gateway in the same release, as in the authority release.

## Enabling

### 1. A publisher-only contract-management

The publisher needs none of CM's integrations, so switch them off. The values, as in
[`k3s/eventing-trust-anchor.yaml`](../../k3s/eventing-trust-anchor.yaml):

```yaml
contract-management:
  enabled: true
  did: did:web:dataspace-authority.org
  enableTmForum: false
  enableRainbow: false
  enableOdrlPap: false
  enableTrustedIssuersList: false
  enableCentralMarketplace: false
  deployment:
    # all state is in memory, see the pitfalls below
    replicaCount: 1
  additionalEnvVars:
    # ... a copy of the chart's default OTEL_* entries (Helm replaces lists) ...
    - name: NOTIFICATION_ENABLED
      value: "false"
  # the TIR the watcher reads; Helm merges maps, so this adds to the chart's services
  services:
    trusted-issuers-registry:
      path: "/"
      url: http://tir.trust-anchor.svc.cluster.local:8080
  dataspace:
    events:
      publisher:
        enabled: true
        # the gateway host of step 4; sent as WebHook-Request-Origin in the handshake
        origin: dataspace-events.127.0.0.1.nip.io
      tir-watcher:
        enabled: true
```

- `origin` is the host the participants configure as `publishers[].origin`. Their handshake answers
  only that origin, so use the same value in both places; in production it is your own gateway host.
- `tir-watcher` and `publisher.lease-ttl` keep CM's defaults in production; see
  [Timing](#timing-production-defaults-and-the-k3s-values).
- Under `dataspace.events`, the `enabled` flags and `publisher.sink-*-timeout` are read at start-up;
  the rest reloads at runtime. The keys outside `dataspace.events` (`did`, `enable*`, `services`,
  `additionalEnvVars`) take effect only on a new rollout
  ([participant setup](participant-setup.md#when-configuration-changes-take-effect)).

### 2. Present the authority's credential

To deliver an event, the publisher gets a token from the participant's verifier
([specification §5.1](specification.md#51-token-flow-fiware-m2m-profile-of-oid4vp)). It does so with
CM's OID4VP client:

```yaml
contract-management:
  oid4vp:
    enabled: true
    credentialsFolder: /credential-repo
    trustAnchors:
      - /etc/ssl/cacert.pem
    holder:
      holderId: did:web:dataspace-authority.org
      keyType: EC
      keyPath: /signing-key/tls.key
      signatureAlgorithm: ECDH-ES
```

The chart has no value to mount a key or a credential, so mount them with
`deployment.additionalVolumes` and `deployment.additionalVolumeMounts`:

- the credential into `credentialsFolder` (in k3s: key `credential` of secret `vc-operator-credential`
  as `operator-credential.jwt`);
- the key into `keyPath`. The client reads a PKCS#8 key; k3s converts the cert-manager key with an
  `openssl pkcs8` init container;
- the CA of the participants' verifiers into `trustAnchors` and into the Java truststore, when it is
  not a public CA. k3s imports its `dsc-ca` with an init container.

Set `oid4vp.proxyConfig` when the participants' verifiers are reached through a proxy (k3s: Squid).

### 3. Register the subscription service at the verifier

Subscribers authenticate at the authority's verifier with their MembershipCredential. Add a service
for it to a **full copy** of your `decentralizedIam.vcAuthentication.vcverifier.registration.services`
(Helm replaces lists):

```yaml
- id: dataspace-subscriptions
  defaultOidcScope: "default"
  authorizationType: "DEEPLINK"
  oidcScopes:
    "default":
      flatClaims: true
      credentials:
        - type: MembershipCredential
          # the credential's issuer must be a data space member
          trustedParticipantsLists:
            - https://tir.dataspace-authority.org
          trustedIssuersLists:
            - "*"
          jwtInclusion:
            enabled: true
            # fullInclusion: true would make the verifier ignore claimsToInclude
            fullInclusion: false
            claimsToInclude:
              - originalKey: issuer
                newKey: participantId
      dcql:
        credentials:
          - id: membership-query
            format: "jwt_vc_json"
            multiple: true
            meta:
              type_values:
                - ["MembershipCredential"]
```

`participantId` is mapped from the credential's **issuer**; [Who a subscriber is](#who-a-subscriber-is)
explains why and what it requires.

### 4. Expose `/subscriptions` at the gateway

CM does not verify tokens. It takes the subscriber from the `participantId` claim of the
`X-Userinfo` header, which the gateway sets after checking the token, exactly as on a participant's
`/events` route ([participant setup](participant-setup.md#who-may-send-events)). Three routes go to a
**full copy** of `decentralizedIam.odrlAuthorization.apisix.routes`, all bound to the publisher's host:

| Route `name` | Request | Upstream | Authentication |
|---|---|---|---|
| `dataspace-subscriptions` | `POST /subscriptions` | `contract-management:8080` | token of `dataspace-subscriptions` |
| `dataspace-subscriptions-delete` | `DELETE /subscriptions/*` | `contract-management:8080` | token of `dataspace-subscriptions` |
| `dataspace-subscriptions-discovery` | `GET` `/.well-known/openid-configuration`, `/subscriptions/.well-known/openid-configuration` and `/subscriptions/<id>/.well-known/openid-configuration` | `verifier:3000`, rewritten to `/services/dataspace-subscriptions/.well-known/openid-configuration` | none |

The two authenticated routes carry:

- `openid-connect` with `bearer_only: true`, `set_userinfo_header: true`, `client_id:
  dataspace-subscriptions` and an `aud` check (`claim_validator.audience` with
  `match_with_client_id: true`), so a token issued for another service of the verifier is refused;
- `proxy-rewrite` with `headers.remove: ["X-Userinfo"]` at `_meta.priority: 3000`, above
  `openid-connect` (2599), so a client-sent header is removed before the real one is set.

There is no `opa` plugin: the authority runs no PDP, and the verifier service is the authorization.
Do **not** name these routes `dataspace-events*`: the chart validates routes of those names against
the rules for a participant's `/events` ([participant setup](participant-setup.md#what-the-chart-checks-on-the-routes)).
The k3s overlay holds the complete routes. Add the publisher's host to the gateway's Ingress
(`ingress.hosts`, `ingress.tls[].hosts` and the certificate) as well.

### 5. Tell the participants

Each participant configures the publisher under `contract-management.dataspace.events.publishers`
([participant setup, step 1](participant-setup.md#1-configure-contract-management)). Give them:

| Participant value | k3s value |
|---|---|
| `id` | `did:web:dataspace-authority.org` |
| `origin` | the `publisher.origin` of step 1 (`dataspace-events.127.0.0.1.nip.io`) |
| `subscriptions-url` | `https://<origin>/subscriptions` |
| `client-id` / `scopes` | `dataspace-subscriptions` / `["default"]` |
| `on-participant-removed.registry-url` | the TIR |
| the credential to admit on `/events` | `OperatorCredential` issued by `did:web:dataspace-authority.org` |

## Who a subscriber is

The publisher compares the `subscriberId` of a subscription with the caller's `participantId`. The
k3s service takes that claim from the MembershipCredential's **issuer**, not from its
`credentialSubject.id`:

- the verifier's TIR check (`trustedParticipantsLists`) binds the issuer, nothing else. The subject is
  a value the issuer writes freely, so with a subject mapping any member could mint a
  MembershipCredential for another member and subscribe, or cancel a subscription, in its name;
- in k3s every participant issues its own MembershipCredential with its own Keycloak, so the issuer
  is the participant's DID (`did:web:mp-operations.org`, `did:web:fancy-marketplace.biz`).

The contract this sets: **a subscriber is identified by the DID that issued its MembershipCredential**.
A data space whose memberships are issued by a third party (one membership issuer for everyone)
cannot use this mapping: every subscriber would get the issuer's DID. It needs a subject mapping
together with a trust check that binds the subject, for example `trustedIssuersLists` limited to the
membership issuer and holder binding on the credential. The
[specification §5.2](specification.md#52-caller-identity) allows either binding, as long as the value
that reaches the comparison is the one the trust check binds.

:warning: The k3s MembershipCredentials are issued with `holderBinding.enabled: false`. They carry
no confirmation claim, so whoever obtains a participant's credential JWT can present it and act as that
participant at `/subscriptions`. Enable holder binding in production.

## The TIR watcher

The watcher diffs the TIR, so the registry needs no hook
([how it works §6](architecture.md#6-detecting-the-removal-at-the-trust-anchor),
[ADR 0010](../adr/0010-detect-removal-by-diffing-the-registry.md)). A diff turns any
failure that makes the registry look empty into a mass removal, so CM applies these safeguards:

- **The first cycle after a start only takes a snapshot.** A fresh publisher announces nothing, and
  a DID counts as removed only if an earlier snapshot held it. A DID that is added and removed
  between two cycles is never announced; use a [manual publish](#removing-a-participant) for it.
- **Only a complete read becomes a snapshot.** A failed or partial read keeps the previous snapshot.
- **`confirmations`:** a DID must be missing in that many consecutive cycles before it is announced.
- **`max-removals-per-cycle`:** more disappearances at once announce nothing and need an operator.

A removal is announced about `confirmations × interval` after it happens. The next cycle is
scheduled `interval` after the previous one ends.

## Timing: production defaults and the k3s values

The k3s overlay shortens CM's clocks so the integration test runs in minutes. Keep the defaults in
production unless the data space promises faster propagation.

| Key | CM default | k3s | Why k3s differs |
|---|---|---|---|
| `tir-watcher.initial-delay` | `PT30S` | `PT10S` | the first snapshot is taken sooner after a start |
| `tir-watcher.interval` | `PT1M` | `PT10S` | a removal reaches the participants in ~30 s |
| `tir-watcher.confirmations` | `2` | `2` | unchanged |
| `tir-watcher.max-removals-per-cycle` | `5` | `5` | unchanged |
| `publisher.lease-ttl` | `PT24H` | `PT2M` | see below |

Subscriptions live in the publisher's memory. A restart of the publisher drops all of them, and a
subscriber only notices at its next renewal, which it schedules at half of the remaining lease: with
the default lease up to ~12 h after the restart. The k3s lease of two minutes brings them back within
a minute, which the test needs after it scales the publisher down. A short lease costs one
subscription request per participant per renewal; pick it from how long the data space may go without
events after a publisher restart. The participants' `subscriber.retry` (default `PT5S` doubling up to
`PT5M`) adds to it when the publisher is still down at the renewal.

Events published while a participant has no subscription are lost for it; its reconciliation sweep
catches up ([participant setup](participant-setup.md#reconciliation)).

## Removing a participant

Remove the participant from the registry, as without eventing:

```shell
# TIL: the trust anchor's trusted-issuers-list management API, e.g. https://til.127.0.0.1.nip.io in k3s
curl -X DELETE "$TIL/issuer/did:web:fancy-marketplace.biz"
```

The TIR the watcher reads is the read-only view of the same list. The watcher announces
the removal after `confirmations × interval`.

To announce it at once, publish it by hand on CM's management port 9090. The port is
cluster-internal, so port-forward to the Deployment (the Service exposes 8080 only):

```shell
kubectl port-forward -n trust-anchor deploy/contract-management 9090:9090 &
curl -X POST http://localhost:9090/events -H 'Content-Type: application/json' \
  -d '{"type": "org.fiware.dataspace.participant.removed", "subject": "did:web:fancy-marketplace.biz"}'
# 202 {"id": "...", "subscriptions": 2}
```

`subscriptions` is the number of subscriptions the event was sent to. In k3s,
[`doc/scripts/dataspace-eventing-demo.sh`](../scripts/dataspace-eventing-demo.sh) wraps both calls
(`remove`, `publish`).

## Checking the publisher

- `GET :9090/health` lists the `dataspace-tir-watcher` indicator with `state`, `lastSuccessfulCycle`,
  `firstCycleDue`, `snapshotSize`, `pendingSize` and `undeliveredSize`. A `snapshotSize` > 0 and a
  recent `lastSuccessfulCycle` show that the watcher reads the TIR. CM only shows these details to
  anonymous callers with `ENDPOINTS_HEALTH_DETAILS_VISIBLE=ANONYMOUS`, which k3s sets.
- `POST https://<origin>/subscriptions` without a token answers `401`.
- `GET https://<origin>/subscriptions/.well-known/openid-configuration` returns the metadata of the
  `dataspace-subscriptions` service.
- A manual publish of an unknown DID returns the number of active subscriptions, one per subscribed
  participant.

## contract-management pitfalls

- **`NOTIFICATION_ENABLED=false`.** The CM chart's `notification.enabled: false` only omits the
  block, and CM then defaults to `true`. Without TM Forum the health check goes `DOWN`. Set the env
  var in `contract-management.additionalEnvVars`. That value is a list and Helm replaces lists, so
  copy the chart's default `OTEL_*` entries into it.
- **The TIR URL is a Micronaut service.** The watcher has no URL key: it reads the HTTP service
  `trusted-issuers-registry`, configured in the CM chart's `services:` map.
- **One replica.** CM has no database: subscriptions, the TIR snapshot and pending removals are in
  memory. A second replica would announce every removal twice and know only part of the
  subscriptions.
- **Port 9090 stays cluster-internal.** It serves the unauthenticated `POST /events` (manual
  publish), `POST /refresh` and the health details. No Service port, Ingress or route for it, and a
  NetworkPolicy where pod IPs are reachable.
