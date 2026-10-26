# 0007 - Test the charts both rendered and applied, and installed as Helm releases

**Status:** Accepted
**Date:** 2026-10-14
**Context doc:** [`../deployment-integration/local-deployment/LOCAL.MD`](../deployment-integration/local-deployment/LOCAL.MD#deployment)

## Context

The local deployment and the integration tests used to render every chart with `helm template`
(helm-maven-plugin), patch the result and apply it with the k3s-maven-plugin. That is close to how
a GitOps tool deploys a chart, but `helm install` - the path the README documents and Helmfile
uses - was never tested. Template and apply hides problems that only show up with Helm:

- hooks are applied as ordinary resources, all at once, with no phases, weights or waits;
- the CRDs of a chart's `crds/` folder are not rendered unless asked for (the build downloaded the
  postgres-operator CRD by hand to compensate, and vc-operator's CRDs were missing);
- there is no release, so no upgrade, and `lookup` always returns nothing.

The reverse also holds. Some problems only show up when the manifests are applied without Helm, as
GitOps tools do: `lookup` returning nothing, hooks applied as plain resources, objects without a
namespace landing in the namespace of the kubectl context (the APISIX ingress). And Helm's
per-release waits can deadlock releases that depend on each other, which a plain apply converges
through (the consent scenario).

A first attempt replaced template and apply with `helm upgrade --install`; review pointed out that
it stopped testing the GitOps-style path.

## Decision

Keep both and run every integration-test scenario with both:

- **template and apply** stays the default: the helm-maven-plugin renders every release, with
  its CRDs (`--include-crds`), into `target/rendered`, `k3s/ci-post-render.sh` and the APISIX
  ingress patch adjust it, and the k3s-maven-plugin applies it in order;
- **helm install** is the `helm-install` profile: `k3s/deploy.sh` installs every chart with
  `helm upgrade --install`.

Both paths use the same pinned Helm binary (`helm.version`) and the same values files, selected by
the scenario profiles through the `deploy.*` properties. The CI matrix is scenarios × paths.

## Consequences

- The CI runs twice as many integration-test jobs.
- A chart change has to work both as a Helm release and as rendered manifests; a failure on only
  one path points at the difference between them (hooks, `lookup`, CRDs, namespaces, ordering).
- Template and apply is not exactly what ArgoCD does: ArgoCD passes the cluster's `--api-versions`
  and `--kube-version`, turns hooks into sync hooks and honours sync-waves. The path keeps
  `main`'s patches (deadlines, dropping upgrade-only hooks) and remains an approximation.
- The operators release is applied in two steps on the template path (subcharts first, then the
  chart's own CA and issuers), because the issuers are hooks that need cert-manager's webhook.

## Alternatives

- **Helm install only.** Loses the GitOps-style coverage that most deployments of this chart
  depend on.
- **Template and apply only.** Leaves `helm install`, the documented installation, untested.
- **Run ArgoCD in the test cluster.** The faithful GitOps path, but a larger change; it can replace
  the template path later without changing the values files.
