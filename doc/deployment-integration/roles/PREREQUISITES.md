# Cluster Prerequisites

What a Kubernetes cluster needs before any role of the FIWARE Data Space Connector is installed in it: the operators the charts rely on, the network the components expect, and the resources they use. It applies to every role - [Consumer](consumer/README.md), [Provider](provider/README.md) and [Operator](operator/README.md) - and to the [local marketplace](provider/LOCAL_MARKETPLACE.md).

The [local deployment](../local-deployment/LOCAL.MD) sets all of this up on its own, for a test environment on a single machine; this page is for clusters that are set up by hand.

<!-- ToC -->
- [At a glance](#at-a-glance)
- [Kubernetes](#kubernetes)
- [Databases](#databases)
  - [PostgreSQL deployed by the charts: the Zalando Postgres Operator](#postgresql-deployed-by-the-charts-the-zalando-postgres-operator)
  - [MongoDB deployed by the charts](#mongodb-deployed-by-the-charts)
  - [Installing the operators once](#installing-the-operators-once)
- [cert-manager](#cert-manager)
- [Network](#network)
  - [Public names have to resolve inside the cluster](#public-names-have-to-resolve-inside-the-cluster)
  - [Outbound access](#outbound-access)
  - [Private CA](#private-ca)
- [Sizing](#sizing)

## At a glance

| Requirement | Needed | Used by |
|---|---|---|
| Kubernetes 1.26 or later (1.19 without the vc-operator) | Always | Every chart |
| A default StorageClass | When the charts deploy the databases, or other persistent components | PostgreSQL, MongoDB, Vault |
| An ingress controller (or a Gateway API implementation) and TLS certificates for the public hosts | Always | DID document, Keycloak, verifier, APISIX, marketplace, Trust Anchor |
| [PostgreSQL](#databases) | Always: an existing one, or one the charts deploy through the Zalando Postgres Operator | All roles |
| [MongoDB](#databases) | With the local marketplace or the Trust Anchor's consent management: an existing one, or one the charts deploy through a MongoDB operator | Provider, Operator |
| [cert-manager](#cert-manager) | Optional, recommended | Certificates of the public hosts and of the identity key |
| [Public names resolvable inside the cluster](#public-names-have-to-resolve-inside-the-cluster) | Always | Verifier, APISIX, marketplace, vc-operator, Keycloak |
| [CPU, memory and disk](#sizing) | Always | - |

## Kubernetes

- **Version:** the charts accept Kubernetes 1.19 or later, except the vc-operator, which needs 1.26. Use a Kubernetes release that is still supported upstream; the integration tests run on a current [k3s](https://k3s.io/).
- **Storage:** the databases the charts deploy and, when enabled, Vault and APISIX's etcd request PersistentVolumeClaims from the default StorageClass unless their values name one, so the cluster needs a default StorageClass or a class set for each of them.
- **Ingress:** the examples in these guides use Ingress resources with [Traefik](https://traefik.io/) annotations; any ingress controller works, adapting the annotations. Every public host needs a TLS certificate from a CA the other participants trust.

## Databases

What the components need is the databases, not a particular way of running them:

- **PostgreSQL**, for every role: Keycloak, the Trusted Issuers List, the verifier, the PAP and Scorpio in a connector, and the Trusted Issuers List in the Trust Anchor.
- **MongoDB**, only for the [local marketplace](provider/LOCAL_MARKETPLACE.md) (Business API Ecosystem) and the Trust Anchor's consent management.

Each can be **an existing database** - a managed service such as Amazon RDS, Azure Database or Cloud SQL, or a deployment of your own - or **one the charts deploy**:

| | An existing database | A database the charts deploy |
|---|---|---|
| What the cluster needs | Network access to it, and the databases and users created in it | The operator that creates it: the [Zalando Postgres Operator](#postgresql-deployed-by-the-charts-the-zalando-postgres-operator) for PostgreSQL, a [MongoDB operator](#mongodb-deployed-by-the-charts) for MongoDB |
| Values | The connection of each component, see [Databases](provider/README.md#databases) in the provider guide and [Using an existing MongoDB](provider/LOCAL_MARKETPLACE.md#using-an-existing-mongodb) for the marketplace. In the Trust Anchor, `managedPostgres.enabled: false` and `trusted-issuers-list.database.*` | The default for PostgreSQL (`managedPostgres`); `managedMongo` for MongoDB |

An existing PostgreSQL for a connector also needs the PostGIS extension in Scorpio's database.

### PostgreSQL deployed by the charts: the Zalando Postgres Operator

The databases the charts deploy are `postgresql` resources of the [Zalando Postgres Operator](https://github.com/zalando/postgres-operator), and **both the connector and the Trust Anchor charts install that operator by default**:

| Chart | Value that installs the operator | Operator chart version |
|---|---|---|
| data-space-connector | `decentralizedIam.vcAuthentication.postgres-operator.enabled` | 1.15.1 |
| trust-anchor | `postgres-operator.enabled` | 1.15.1 |

The versions are the ones the charts on `main` depend on; check `Chart.yaml` (and `Chart.lock`) of the chart version you install.

An operator is cluster-wide: its CustomResourceDefinitions, ClusterRoles and webhooks exist once per cluster, and Helm lets only one release own them.

- **A single release in the cluster:** keep the default.
- **More than one release in the cluster** - a connector next to the Trust Anchor, or several connectors: install the operator once, on its own, and set those values to `false` in every release. A second release that installs it fails on the resources the first one owns.
- **No PostgreSQL deployed by the charts:** set those values to `false`; the operator is not needed.

### MongoDB deployed by the charts

`managedMongo` creates a `MongoDBCommunity` resource, handled by the MongoDB Community Operator or its successor, MongoDB Controllers for Kubernetes, which has to be installed beforehand. The connector chart can also install the Community Operator (`mongo-operator.enabled`, chart 0.13.0, off by default). [Running a local marketplace](provider/LOCAL_MARKETPLACE.md#mongodb) explains the installation options, including the database namespace and its service account.

### Installing the operators once

Install each operator the charts need with its upstream chart, in its own namespace, before any participant, and disable it in every participant's release:

```shell
helm repo add postgres-operator-charts https://opensource.zalando.com/postgres-operator/charts/postgres-operator
helm install postgres-operator postgres-operator-charts/postgres-operator \
  --version 1.15.1 --namespace postgres-operator --create-namespace
```

```yaml
# in the values of every connector release
decentralizedIam:
  vcAuthentication:
    postgres-operator:
      enabled: false
```

```yaml
# in the values of the trust-anchor release
postgres-operator:
  enabled: false
```

The local deployment follows the same pattern with the connector chart itself: [`k3s/operators.yaml`](../../../k3s/operators.yaml) installs the Postgres and MongoDB operators and cert-manager as a release of their own, with every other component disabled, and the participants' profiles turn them off. It is a test setup; in a production cluster, the upstream charts keep the operators' lifecycle independent of the connector's.

## cert-manager

[cert-manager](https://cert-manager.io/) is not required, but the guides use it for the TLS certificates of the public hosts and for the identity key. The connector chart can install it (`cert-manager.enabled`, chart v1.21.2, off by default); like the operators, it can be installed only once per cluster.

Whatever issues the certificate that holds the DID's key has to keep that key on renewal: see [Identity key lifecycle](provider/README.md#identity-key-lifecycle).

## Network

### Public names have to resolve inside the cluster

Several components call other components through their **public** HTTPS names, also when both run in the same cluster:

| Component | Calls | Why |
|---|---|---|
| VCVerifier | The `did:web` of every credential issuer, and the Trust Anchor's TIR (`tirAddress`) | To resolve the issuers' keys and check that they are trusted |
| APISIX (`openid-connect` plugin) | The verifier's discovery document and its `jwks_uri` | To validate the access tokens; the discovery document announces the public `jwks_uri` even when `discovery` points inside the cluster |
| Marketplace logic proxy | The verifier's discovery document and token endpoint | For the login, see [Running a local marketplace](provider/LOCAL_MARKETPLACE.md) |
| vc-operator | Keycloak's credential issuer | To obtain the credentials it manages |
| Keycloak | The status list server, when `statusListServer` is enabled | For the revocation status of the credentials it issues |

From inside a pod those names have to reach the ingress, not something else. In a cluster behind a cloud load balancer they usually do: the name resolves to the load balancer's public address and the traffic comes back in (**hairpin NAT**). Check it, because some load balancers and most local or on-premises setups do not route traffic from the cluster back to its own public address. The alternatives:

- **Split-horizon DNS**: the organization's internal DNS, used by the nodes, resolves the public names to the ingress' internal address.
- **A CoreDNS rewrite**: the cluster DNS answers the public names with the ingress controller's Service. In k3s and RKE2, CoreDNS imports the `coredns-custom` ConfigMap, which leaves the CoreDNS configuration the distribution manages untouched:

  ```yaml
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: coredns-custom
    namespace: kube-system
  data:
    # resolve every name under <your_domain> to the ingress controller's Service
    public-names.override: |
      rewrite stop {
        name regex (.+)\.<your_domain_regex>\.$ <ingress_service>.<ingress_namespace>.svc.cluster.local.
        answer auto
      }
  ```

  `<your_domain_regex>` is the domain with its dots escaped (`example\.org`), and `<ingress_service>.<ingress_namespace>` the Service of the ingress controller, for example `ingress-nginx-controller.ingress-nginx`. Managed clusters have their own way to extend the cluster DNS ([EKS](https://docs.aws.amazon.com/eks/latest/userguide/managing-coredns.html), [GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/kube-dns), [AKS](https://learn.microsoft.com/azure/aks/coredns-custom)).

To check it, resolve and call a public name from a pod in the connector's namespace:

```shell
kubectl -n <your_namespace> run dns-check --rm -it --restart=Never --image=curlimages/curl -- \
  curl -sS https://<your_did_domain>/.well-known/did.json
```

### Outbound access

Besides the other participants' public names (their DIDs, the Trust Anchor's TIR, and a Central Marketplace if there is one), the cluster needs to reach:

- the image registries: `quay.io`, `docker.io` and `ghcr.io`, plus those of the operators' images;
- Maven Central (`repo1.maven.org`), when Keycloak loads the token status list plugin with the chart's helper: an init container downloads it on every start. `keycloak.tokenStatusList.plugin.mavenBaseUrl` can point to a mirror;
- the JSON-LD contexts the credentials reference (such as `https://www.w3.org/2018/credentials/v1`), for the verifier.

If outbound traffic goes through a proxy, give `HTTPS_PROXY` to the components that call out - the verifier's `deployment.additionalEnvVars`, Keycloak's `proxy.*`, the marketplace's and the vc-operator's environment, and `proxy_opts` in APISIX's `openid-connect` plugin - and keep the in-cluster services out of it with `NO_PROXY` (`.svc`, `.cluster.local`, and the Service names the component calls, such as `trusted-issuers-list` and `credentials-config-service` for the verifier).

### Private CA

If the public hosts have certificates from a private CA, every component that calls them has to trust it. Never turn the verification off instead.

| Component | How to trust the CA |
|---|---|
| APISIX | `apisix.ssl.existingCASecret`, see [Trusting a private CA](provider/README.md#trusting-a-private-ca) |
| VCVerifier | An init container that appends the CA to the image's bundle (`/etc/ssl/cert.pem`) in a shared volume mounted over it, as [`k3s/provider.yaml`](../../../k3s/provider.yaml) does. The chart's `serverCertificates.caCert` only takes effect when other init containers are configured too |
| Marketplace logic proxy | `NODE_EXTRA_CA_CERTS`, see [Running a local marketplace](provider/LOCAL_MARKETPLACE.md) |
| vc-operator | `SSL_CERT_FILE` pointing to the CA, mounted with `extraVolumes`/`extraVolumeMounts`, as in [`k3s/provider.yaml`](../../../k3s/provider.yaml). |
| Keycloak | `tls.truststoreEnabled` with a Java truststore in `tls.truststoreExistingSecret` |
| Contract Management | No option to add a CA: the hosts it calls (the Central Marketplace) need a certificate from a public CA |

## Sizing

The figures below were measured with chart 10.13.2 (trust-anchor 1.3.0) and the local deployment's profiles ([`k3s/`](../../../k3s/)), one release per role on a single k3s node, after the components had settled and **without load**. They give the floor a role needs, not its capacity: plan the headroom for your traffic on top of them.

| Role | Memory in use | CPU in use | Images (compressed) | Disk for images |
|---|---|---|---|---|
| Trust Anchor (TIR/TIL and PostgreSQL) | 0.3 GiB | 0.02 cores | 0.8 GB | ~2.5 GB |
| Consumer (Keycloak, DID, PostgreSQL) | 0.8 GiB | 0.05 cores | 1.0 GB | ~3 GB |
| Provider without the marketplace | 2.3 GiB | 0.2 cores | 2.2 GB | ~7 GB |
| Provider with the local marketplace | 2.6 GiB | 0.25 cores | 4.2 GB | ~13 GB |
| Operators (Postgres, MongoDB, cert-manager) | 0.1 GiB | 0.01 cores | 0.15 GB | ~0.5 GB |

- **Memory:** the largest consumers at rest are Keycloak (~0.6 GiB), Scorpio (~0.4 GiB), the TM Forum API (~0.35 GiB), PostgreSQL (~0.2 GiB per cluster), the Trusted Issuers List and contract-management (~0.2 GiB each) and APISIX (~0.2 GiB). The verifier, the PAP, OPA and did-helper use a few tens of MiB.
- **Disk:** the container runtime keeps both the downloaded and the unpacked layers, about three times the compressed size of the images, on every node that runs them. Add the PersistentVolumes: by default 1 GiB for each PostgreSQL cluster and 1 GiB + 1 GiB for the marketplace's MongoDB, which hold a few hundred MiB on a fresh installation.
- **CPU:** at rest, a fraction of a core per role. Startup needs more, mostly for the Java components (Keycloak, Scorpio, the TM Forum API, the Trusted Issuers List, contract-management).

### Set memory limits on the Java components

Most components declare no resources by default, so the scheduler reserves nothing for them and nothing bounds them. That matters most for the Java components: without a memory limit, the JVM sizes its heap from the memory of the **node**, not from what the component needs. On a long-running installation with 64 GiB nodes and no limits, Scorpio grew to ~17 GiB and the TM Forum API to ~8.5 GiB, against the ~0.4 GiB they use after a fresh start.

Give every component requests and limits through its chart's `resources` - in particular Scorpio, the TM Forum API, the Trusted Issuers List and contract-management - and size the limits with your own measurements under load (`kubectl top pods --containers`). The PostgreSQL clusters take their resources from the operator's defaults (100m CPU and 100 MiB requested, 1 CPU and 500 MiB limit) unless `resources` is set in `decentralizedIam.vcAuthentication.managedPostgres.config` (in the trust-anchor chart, `managedPostgres.config`), which is the `postgresql` resource's spec; 500 MiB is little for a database that several components share.
