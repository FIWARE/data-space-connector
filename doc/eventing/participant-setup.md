# Data Space Eventing — Participant Setup

Operator guide for the receiving side of data space eventing in the Data Space Connector chart:
contract-management (CM) subscribes at the data space's publisher and receives its events on
`POST /events`. The reasons behind the protocol are in the [architecture](architecture.md) and the
[interoperability specification](specification.md); the reasons behind the chart's packaging are in
[ADR 0008](../adr/0008-events-routes-as-validated-operator-snippet.md).

Everything below is off by default. An upgrade with default values creates no eventing ConfigMap,
route, env entry or volume.

<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Requirements](#requirements)
- [1. Enable the listener and the subscriber](#1-enable-the-listener-and-the-subscriber)
  - [Startup-only and refreshed keys](#startup-only-and-refreshed-keys)
- [2. Gateway routes](#2-gateway-routes)
  - [Add the routes](#add-the-routes)
  - [Why the routes look like this](#why-the-routes-look-like-this)
  - [What the chart enforces](#what-the-chart-enforces)
  - [Checklist: what the chart does not check](#checklist-what-the-chart-does-not-check)
  - [When a route change takes effect, and removing the routes](#when-a-route-change-takes-effect-and-removing-the-routes)
  - [Own gateway, and switching the checks off](#own-gateway-and-switching-the-checks-off)
- [3. Verifier service](#3-verifier-service)
- [4. ODRL policy](#4-odrl-policy)
- [5. Scoped membership grants](#5-scoped-membership-grants)
- [6. Management port 9090](#6-management-port-9090)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Requirements

- contract-management ≥ 3.5.3: the chart ships it (CM chart 3.6.6) since 10.16.0.
- trusted-issuers-list (TIL) ≥ 0.10.1 for scoped grants: the chart ships 0.10.3 (decentralizedIam
  2.3.0) since 10.16.0. A TIL outside the release (`registration.til`) must be upgraded by its operator.
- The bundled APISIX gateway (odrl-authorization) in its default yaml mode or an etcd-backed mode, or
  an own gateway (see [Own gateway](#own-gateway-and-switching-the-checks-off)).

## 1. Enable the listener and the subscriber

The configuration lives under `contract-management.dataspace.events` and is passed verbatim to CM as
its Micronaut property tree, so keys are kebab-case. The CM README documents every key. A minimal
participant that subscribes at the data space publisher and revokes trust of removed participants:

```yaml
contract-management:
  dataspace:
    events:
      listener:
        enabled: true
      subscriber:
        enabled: true
        # public URL of the own POST /events; its host is the host of the gateway routes (section 2)
        sink: "https://events.example.org/events"
        protocol-settings:
          # the verifier service of section 3; publishers request their token for it on a 401
          client-id: "<events-service>"
          scopes: [ "default" ]
      publishers:
        - id: "did:web:dataspace-authority.org"
          # the publisher's DNS name, as it sends it in WebHook-Request-Origin
          origin: "cm.dataspace-authority.org"
          types: [ "org.fiware.dataspace.participant.removed" ]
          subscriptions-url: "https://cm.dataspace-authority.org/subscriptions"
          # how CM authenticates at the publisher's subscriptions API
          client-id: "contract-management"
          scopes: [ "default" ]
          on-participant-removed:
            # report writes nothing; dataspace, granted and full revoke TIL grants
            mode: report
            registry-url: "https://tir.dataspace-authority.org"
```

Start with `mode: report` and read `GET :9090/participants-removed` before switching to a writing
mode. Do not leave `dataspace:` empty (null): the CM chart then fails the render; remove the key
instead.

With the listener or the subscriber on, the CM chart renders the tree into the ConfigMap
`contract-management-dataspace-events`, mounts it as a directory (no `subPath`) and points CM's
config watch at it.

### Startup-only and refreshed keys

| Keys | Takes effect |
|---|---|
| `listener.enabled`, `subscriber.enabled`, `publisher.enabled`, `tir-watcher.enabled`, `listener.watch.*`, `publisher.sink-connect-timeout`, `publisher.sink-read-timeout` | at startup only. The CM chart keeps a checksum of these keys on the pod, so changing one rolls the pod. |
| everything else, `publishers` included | at runtime: the kubelet updates the mounted file (within its sync period, about a minute), CM's watch sees it and re-reads the configuration. No restart. |

## 2. Gateway routes

CM does not verify tokens. It trusts the `X-Userinfo` header that the gateway sets after verifying
the token, and reads the caller from the claim `participantId` in it. The chart ships **no** events
route: the routes are ordinary entries of your own
`decentralizedIam.odrlAuthorization.apisix.routes` list, copied from
[`examples/events-routes.yaml`](examples/events-routes.yaml). The chart finds them by `name` and
refuses settings that weaken access control. [ADR 0008](../adr/0008-events-routes-as-validated-operator-snippet.md)
explains why.

### Add the routes

1. **Append** the three entries of [`examples/events-routes.yaml`](examples/events-routes.yaml) to
   your own `decentralizedIam.odrlAuthorization.apisix.routes` list. Helm replaces lists instead of
   merging them: passing the example with `-f` next to a values file that has its own `routes` list
   keeps only one of the two lists.
2. **Keep the three `name`s** (`dataspace-events`, `dataspace-events-handshake`,
   `dataspace-events-discovery`). The checks find the routes by name; a renamed or unnamed copy is
   not checked at all.
3. **Bind all three to the host of the sink URL** you announce (`subscriber.sink`), with `host` or
   `hosts`. Never leave the binding empty (APISIX drops a route with `hosts: []`) and never set both
   (APISIX rejects the route).
4. Set `openid-connect.discovery` of `dataspace-events` and the `proxy-rewrite.uri` of
   `dataspace-events-discovery` to the verifier service of [section 3](#3-verifier-service)
   (`<events-service>`), and `client_id` to the same id.
5. Override `upstream.nodes` if CM is not reachable as `contract-management:8080`
   (`contract-management.fullnameOverride` changed, or CM runs outside the release) or the verifier
   not as `verifier:3000`. Change the `uri` of all three entries if the sink path is not `/events`.

### Why the routes look like this

- **The host is required.** APISIX's default router (`radixtree_host_uri`) tries host-bound routes
  first and uses host-less routes only when no host-bound route matches. DSC deployments bind their
  routes to hosts, with `/*` catch-alls, so a host-less `/events` loses to the catch-all of the host
  the publisher delivers to, which applies another verifier scope and policy.
- **Use the host whose catch-all already routes to CM** (in k3s the `contract-management` host). The
  exact `/events` routes then shadow that catch-all. Any other route whose upstream is CM (a `/*` on
  another host) also forwards `POST /events`, with that route's verifier scope and policy and without
  the `X-Userinfo` strip. The events scope and policy do not apply there, and unless that route's
  `openid-connect` sets `X-Userinfo` itself, a client-sent header reaches CM's source == identity
  check. Such routes must exclude `/events`: a `vars` condition, or an ODRL policy that denies it.
- **The discovery route** serves `<sink>/.well-known/openid-configuration`. On a `401` the publisher
  derives the verifier's metadata URL from the called URL
  ([specification §5.1](specification.md#51-token-flow-fiware-m2m-profile-of-oid4vp), step 1), so
  without this route it cannot find out how to get a token.
- **`discovery` uses the verifier's external URL.** The verifier builds `issuer` and `jwks_uri` in its
  metadata from its configured `server.host`, not from the URL the metadata was fetched from, and
  `openid-connect` checks the token's `iss` against that `issuer`. So the gateway fetches the keys
  from the external host whichever discovery URL it uses. An internal
  `http://verifier:3000/services/<events-service>/...` URL works as well when the gateway reaches the
  advertised `jwks_uri`. Set `openid-connect.proxy_opts` where the gateway needs a proxy for it.
- **The `proxy-rewrite` priority.** APISIX runs a route's plugins in descending priority.
  `openid-connect` runs at 2599 and sets `X-Userinfo`; `proxy-rewrite` defaults to 1008, so at its
  default it would remove the header `openid-connect` just set and CM would answer every delivery
  with `401`. With `_meta.priority: 2600` it removes a client-sent header *before* `openid-connect`
  runs. APISIX 3.19.0's `openid-connect` already clears a client-sent `X-Userinfo` and sets its own
  (verified for this release), so the strip is defence in depth, as the CM README asks for: it keeps
  holding if a later APISIX stops doing that.
- **Two switches.** The routes and `contract-management.dataspace.events.listener.enabled` are set
  separately: a subchart value cannot depend on another value, and host and verifier service are
  deployment-specific. The chart does not check that the two agree. Routes without a listener answer
  `404`, which the first delivery shows at once; a listener without the routes is allowed for
  operators who run their own gateway.

### What the chart enforces

`templates/eventing-validation.yaml` fails `helm install`/`helm upgrade` when one of these does not
hold. Each would weaken access control without breaking delivery, so nothing else would show it.

1. The APISIX ingress-controller mode (`apisix.ingress-controller.enabled`) is off. odrl-authorization
   renders one `ApisixRoute` per entry there and drops `methods` and `hosts`, so the unauthenticated
   handshake route would match every method on `/events`. Use the yaml or an etcd-backed mode.
2. `dataspace-events-handshake` has `methods: [OPTIONS]` exactly; it carries no authentication.
3. `dataspace-events-discovery` has `methods: [GET]` exactly; it carries no authentication.
4. `dataspace-events` is bound to a host (`host` or a non-empty `hosts`).
5. `dataspace-events` has `openid-connect` with `bearer_only: true` (otherwise a request without a
   token passes) and `set_userinfo_header: true` (CM reads the caller from `X-Userinfo`).
6. `dataspace-events` has the `opa` plugin, so the ODRL policy applies.
7. `dataspace-events` removes `X-Userinfo` with `proxy-rewrite.headers.remove`, at a
   `proxy-rewrite._meta.priority` above `openid-connect`'s effective priority (its `_meta.priority`,
   default 2599).
8. None of `openid-connect`, `opa` and `proxy-rewrite` on `dataspace-events` is switched off with
   `_meta.disable` or made conditional with `_meta.filter`: APISIX skips such a plugin and the
   presence checks above would pass.
9. `dataspace-events` sets none of `script`, `service_id`, `filter_func`, `vars`, `remote_addrs`,
   `remote_addr`, and no `status` other than `1`. A script replaces the route's plugins, a service can
   bring one in, and the others let `/events` fall through to the host's catch-all.

### Checklist: what the chart does not check

A mistake here breaks the handshake, discovery or delivery visibly instead of weakening access
control, so it is not validated. Go through it once:

- [ ] All three entries are in the list.
- [ ] `dataspace-events` has `methods: [POST]`. Without it the route may shadow the handshake, which
      then answers `401`.
- [ ] The handshake `uri` equals the events `uri`; the discovery `uri` is the events `uri` +
      `/.well-known/openid-configuration`.
- [ ] All three entries bind the same hosts, and the sink URL's host is one of them.
- [ ] The discovery route's rewrite `uri` is `/services/<id>/.well-known/openid-configuration` for
      the same verifier service as the `discovery` URL of `dataspace-events`. A gateway path prefix in
      front of the verifier in the `discovery` URL is fine; the discovery route's upstream must reach
      the same verifier.
- [ ] `contract-management.dataspace.events.listener.enabled: true` when CM runs in this release.

Then test from outside the cluster:

```shell
# handshake: 2xx, and WebHook-Allowed-Origin for a configured publisher origin
curl -i -X OPTIONS https://events.example.org/events -H 'WebHook-Request-Origin: cm.dataspace-authority.org'
# delivery without a token: 401
curl -i -X POST https://events.example.org/events -H 'Content-Type: application/cloudevents+json' -d '{}'
# discovery: the events service's metadata, with the scope the publishers request
curl -s https://events.example.org/events/.well-known/openid-configuration
```

and let the publisher deliver one event. The handshake (CM's subscription stays inactive), the token
fetch (the publisher logs it) and the delivery (`401`/`403` at the gateway or in CM's log) each fail
visibly when an item is wrong.

### When a route change takes effect, and removing the routes

- **Default yaml mode** (`standalone`, `config_provider: yaml`): odrl-authorization renders the routes
  into the ConfigMap `apisix-routes`, mounted as a directory. The kubelet updates the file within its
  sync period (about a minute) and APISIX reloads it when its modification time changes, with no
  restart. Removing the entries removes the routes.
- **etcd-backed modes** (`traditional` + etcd, as in `k3s/provider.yaml`): a hook Job PUTs each entry to
  the admin API on install and upgrade, under the id `<release>-<index>`; `name` is only a field of
  the route body. The Job never deletes. Removing the entries, or shortening the list, leaves the
  trailing ids behind, as for any route removed from the list. Delete them through the admin API:

  ```shell
  curl -s http://<release>-apisix-admin:9180/apisix/admin/routes -H "X-API-KEY: $ADMIN_KEY"
  curl -X DELETE http://<release>-apisix-admin:9180/apisix/admin/routes/<release>-<index> -H "X-API-KEY: $ADMIN_KEY"
  ```

  These modes need `decentralizedIam.odrlAuthorization.apisix.etcd.enabled: true` (or an external
  etcd); odrl-authorization fails the render otherwise.
- **Ingress-controller mode**: refused by the chart (check 1).

### Own gateway, and switching the checks off

The route checks only run when `decentralizedIam` deploys odrl-authorization with its bundled
APISIX, i.e. none of `decentralizedIam.enabled`, `decentralizedIam.odrlAuthorization.enabled` and
`decentralizedIam.odrlAuthorization.apisix.enabled` is the boolean `false`. The ingress-controller
refusal still runs with `apisix.enabled: false`, because odrl-authorization then still renders
`ApisixRoute` CRs that an external controller may serve.

An own gateway must reproduce the same three routes and the same plugin chain: host binding, the
unauthenticated `OPTIONS` handshake, the discovery route, token verification with the claims passed
on in `X-Userinfo`, the ODRL policy, and removing a client-sent `X-Userinfo`.

If the checks refuse a setup that is correct (another auth chain, an `X-Userinfo` strip through a
global rule, an APISIX upgrade that moves `openid-connect`'s priority), set
`eventingRouteValidation.enabled: false`. That switches off every check; verify the list in
[What the chart enforces](#what-the-chart-enforces) by hand.

## 3. Verifier service

Register the events service at the verifier under
`decentralizedIam.vcAuthentication.vcverifier.registration.services`. Its `id` is the
`<events-service>` of the `dataspace-events` and `dataspace-events-discovery` routes and the
`protocol-settings.client-id` of the subscriber; its scope is the one the publishers request
(`protocol-settings.scopes`).

The token must carry two things:

- the top-level claim `participantId` = the credential's `credentialSubject.id`, which CM compares
  with the event's `source` (`dataspace.events.listener.identity.participant-claim`);
- the credential's `type` and `issuer` under `verifiableCredential`, where the ODRL policy of
  [section 4](#4-odrl-policy) reads them.

`fullInclusion: true`, as on the other k3s services, does not work here: the verifier then ignores
`claimsToInclude`, and the token has no `participantId`. Map the claims instead and set `flatClaims`
on the scope (a field of the scope entry, next to `credentials`, in the credentials-config-service
API). A `.` in `newKey` creates a nested claim:

```yaml
decentralizedIam:
  vcAuthentication:
    vcverifier:
      registration:
        services:
          # ... your other services ...
          - id: <events-service>
            defaultOidcScope: "default"
            authorizationType: "DEEPLINK"
            oidcScopes:
              "default":
                flatClaims: true
                credentials:
                  - type: OperatorCredential
                    # trust only the issuer(s) of the publisher's credential
                    trustedParticipantsLists:
                      - https://tir.dataspace-authority.org
                    trustedIssuersLists:
                      - http://trusted-issuers-list:8080
                    jwtInclusion:
                      enabled: true
                      claimsToInclude:
                        - originalKey: credentialSubject.id
                          newKey: participantId
                        - originalKey: type
                          newKey: verifiableCredential.type
                        - originalKey: issuer
                          newKey: verifiableCredential.issuer
                dcql:
                  credentials:
                    - id: operator-query
                      format: "jwt_vc_json"
                      multiple: true
                      meta:
                        type_values:
                          - [OperatorCredential]
```

The claim mapping is read from the VCVerifier 6.31.0 source (`buildInclusion`, `generateJWT`) and
has not run in an integration test yet.

## 4. ODRL policy

[`allowDataSpaceEvents.json`](../../it/src/test/resources/policies/allowDataSpaceEvents.json) is an
example policy for the events path. Create it at the ODRL-PAP (`POST <odrl-pap>/policy`). It admits
`POST` (`tmf:create`, a plain method check in ODRL-PAP) on `/events` for an `OperatorCredential` whose
**issuer** (`vc:currentParty`, the credential's `issuer`, not its holder) is
`did:web:dataspace-authority.org`, the k3s authority that issues the publisher's credential to itself.

- **Change the issuer DID.** Every participant's Keycloak issues `OperatorCredential`, so the type
  alone would admit any participant's operator. Set the right operand of `vc:currentParty` to the DID
  that issues the publisher's credential (the publisher's own id only when it is self-issued, as in
  k3s), or use a dedicated credential type. A wrong DID makes OPA answer `403` to every delivery.
- **The issuer must be a string.** ODRL-PAP's `current_party` returns the credential's `issuer`
  unchanged, so an object issuer (`{"id": "did:..."}`, allowed by the VC data model) never matches
  `odrl:eq`, and OPA answers `403` to every delivery.
- **The holder is checked by CM**, not by the policy: CM compares `publishers[].id` and the event's
  `source` with `participantId` ([specification §6](specification.md#6-authorization-rule)).
- **Three lines of defence.** The verifier service must still trust only the publisher's issuer
  (`trustedIssuersLists`/`trustedParticipantsLists`, [section 3](#3-verifier-service)); the policy is
  the second line, CM's source == identity check on `X-Userinfo` the third.

## 5. Scoped membership grants

The trusted issuers that `registration.issuer` registers at the TIL are now granted with
`scope` = the data space publisher's id and `scopeType` = `dataspace-membership`, so that CM's
`on-participant-removed.mode: dataspace` can revoke them when the publisher announces a removal.
The scope defaults to the `id` of the one `publishers` entry whose `types` contain
`org.fiware.dataspace.participant.removed`; with no such entry nothing changes.

- The TIL must be ≥ 0.10.1. An older TIL accepts the request and silently stores an unscoped grant.
- Set `scope` per issuer entry, or `scope: ""` for an unscoped grant, when the default is wrong:
  - the grant does not come from data space membership (e.g. an eIDAS `UserCredential` issuer, which
    a `participant.removed` for that DID would revoke);
  - the grant lands in a TIL whose CM subscribes to another publisher.
- Several `participant.removed` publishers without an explicit `scope` fail the render.
- An issuer that is already registered keeps its unscoped grant (the TIL answers `409`).
  `on-participant-removed.adopt-unattributed` covers those.

## 6. Management port 9090

CM's management port 9090 serves `POST /refresh` (re-reads the configuration) and
`GET /participants-removed` (removal reports) without authentication. It must stay cluster-internal:
no Service port, Ingress or route for it, and a NetworkPolicy where pod IPs are reachable.
`/refresh` is open on every CM ≥ 3.5.3, eventing on or off; see the
[10.16.0 release notes](../release-notes/10-x.md#10160--participant-side-of-data-space-eventing) for
the opt-out.
