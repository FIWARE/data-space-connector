# 0008 - Ship the data space events routes as a validated operator snippet, not as chart routes

**Status:** Accepted
**Date:** 2026-11-16
**Context doc:** [`../eventing/participant-setup.md`](../eventing/participant-setup.md#2-gateway-routes)

## Context

A participant's contract-management (CM) receives data space events on `POST /events`. CM does not
verify tokens: the API gateway must run `openid-connect` (`bearer_only`, `set_userinfo_header`) and
the ODRL policy (`opa`), and drop a client-sent `X-Userinfo`. Publishers also need the unauthenticated
`OPTIONS` handshake and the OpenID discovery document of the events verifier service under the same
path. So three APISIX routes are needed, and all three depend on the deployment: the host of the
announced sink URL and the id of the verifier service.

The gateway is the bundled APISIX of odrl-authorization, configured through the subchart list
`decentralizedIam.odrlAuthorization.apisix.routes`:

- Helm replaces lists instead of merging them, so a default entry in the umbrella's list is lost as
  soon as an operator sets `routes`. The umbrella declares no `routes` default; the list is
  operator-owned and every existing DSC route lives there.
- odrl-authorization renders the list in one of three ways, by APISIX deployment mode: into the
  `apisix-routes` ConfigMap (default standalone yaml mode, no admin API), as admin-API PUTs under the
  ids `<release>-<index>` from a hook Job (etcd-backed modes, never deleted), or as one `ApisixRoute`
  CR per entry (ingress-controller mode, which drops `methods`, `hosts` and `priority`).
- An entry's `name` is carried through all three modes as a label, never as the route id.
- CI runs `helm unittest` without building dependencies, so only the umbrella's own templates are
  tested; Go-template logic about the subchart's rendering cannot be checked against it in CI.

## Decision

The chart ships no events route. Operators append the three named entries of
[`doc/eventing/examples/events-routes.yaml`](../eventing/examples/events-routes.yaml)
(`dataspace-events`, `dataspace-events-handshake`, `dataspace-events-discovery`) to their own
`routes` list and adapt host, verifier service and upstreams. `templates/eventing-validation.yaml`
finds the entries by `name` and `fail`s on the settings that weaken access control without breaking
delivery (no ingress-controller mode, handshake and discovery methods, the host binding, the
`openid-connect`/`opa`/`X-Userinfo` strip chain, and route fields that bypass it).
`eventingRouteValidation.enabled: false` switches the whole validation off.

## Consequences

- **Two switches.** The routes and `contract-management.dataspace.events.listener.enabled` are set
  separately and the chart does not check that they agree. Routes without a listener answer `404`;
  a listener without routes is allowed for operators with their own gateway.
- **Checks are opt-in by name.** A renamed or unnamed copy of a route is not checked at all; the
  example and the guide say to keep the names.
- **Only access-control checks.** Consistency between the three entries (matching `uri`s and hosts,
  the discovery rewrite, `methods: [POST]`, the listener flag) is a checklist in the guide. A mistake
  there breaks the handshake, discovery or delivery visibly.
- **New upgrade `fail` path.** An existing route that carries one of the three names and does not
  pass the checks fails `helm upgrade`; the fix is to rename it or switch the validation off.
- **Stale routes in etcd-backed modes.** Removing the entries leaves their `<release>-<index>`
  routes in etcd, as for any route removed from the list today; the guide shows the admin-API
  delete. In the default yaml mode the ConfigMap is rewritten and APISIX reloads it.
- **Ingress-controller mode is refused** while one of the events routes is in the list: the
  unauthenticated handshake route would match every method on `/events` there.

## Alternatives

- **An `apisix.extraRoutes` map in odrl-authorization.** It would only buy route defaults that
  survive a user-set `routes` list and a `DELETE` of disabled routes in etcd mode. The routes need a
  deployment-specific host and verifier service, so a default could not be enabled as-is, and the
  stale-route problem affects every route and belongs in odrl-authorization as a general fix. Not
  worth an upstream chart change and two releases.
- **An umbrella hook Job that PUTs the routes to the admin API** (as the OTEL global-rule Job does).
  The default yaml mode has no admin API, so the Job would fail the install; it cannot delete the
  route when the feature is turned off or on `helm uninstall`; and it does not cover the
  ingress-controller mode.
- **Also guarding unnamed routes to `/events` on the release's CM Service.** It needed a copy of the
  CM chart's fullname helper that CI cannot check against the subchart, so it would silently stop
  matching after a CM bump, and it added an upgrade `fail` for routes no operator named for this
  feature, while catching only a renamed copy with exactly that upstream and path.
- **Full consistency validation of the three entries** (about 20 rules). Every APISIX or
  odrl-authorization bump can invalidate Go-template logic CI cannot test against the subchart, and
  a wrong rule blocks install and upgrade, for mistakes that already fail visibly.
