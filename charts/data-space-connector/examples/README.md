# Reference profiles

Example values files to deploy each role of a data space in your own cluster: a trust anchor,
providers and consumers. Pick the file of your role, replace its placeholders, create the Secrets
it expects and install it, as described below.

| Profile | Role | Chart |
|---|---|---|
| [`provider.yaml`](provider.yaml) | Offers a data service, protected with Verifiable Credentials and ODRL policies, and its offers through the TM Forum API | `data-space-connector` |
| [`provider-marketplace.yaml`](provider-marketplace.yaml) | Adds a local marketplace portal to `provider.yaml`, where the provider publishes its offers and consumers order them | `data-space-connector` |
| [`consumer.yaml`](consumer.yaml) | Issues Verifiable Credentials to its users, to access the providers' services | `data-space-connector` |
| [`trust-anchor.yaml`](../../trust-anchor/examples/trust-anchor.yaml) | Keeps the registry of the data space's participants | `trust-anchor` |

An organization that both offers and consumes services uses `provider.yaml`: its Keycloak issues
credentials to its users as well. See [Consumer & Provider](../../../doc/deployment-integration/roles/consumer-provider/README.md).

The role guides explain each part of the profiles: [provider](../../../doc/deployment-integration/roles/provider/README.md),
[consumer](../../../doc/deployment-integration/roles/consumer/README.md), [trust anchor](../../../doc/deployment-integration/roles/operator/README.md),
[Keycloak](../../../doc/deployment-integration/roles/KEYCLOAK.md) and the [local marketplace](../../../doc/deployment-integration/roles/provider/LOCAL_MARKETPLACE.md).

## Prerequisites

In the cluster, installed once and shared by every release (see [Prerequisites](../../../doc/deployment-integration/roles/PREREQUISITES.md)):

- **An ingress controller**, whose class goes into `<ingress-class>`.
- **cert-manager with a ClusterIssuer** that issues publicly trusted certificates, such as Let's
  Encrypt; its name goes into `<cluster-issuer>`. The connector chart can create one
  (`certManagerResources`, see the [provider guide](../../../doc/deployment-integration/roles/provider/README.md#using-the-fiware-dsc-helm-chart-and-values)).
  The participants resolve each other's `did:web` and check each other's certificates: with a
  private CA, every component that calls another one has to trust it (see "Trusting a private CA"
  in the [provider guide](../../../doc/deployment-integration/roles/provider/README.md#trusting-a-private-ca)
  and in [LOCAL_MARKETPLACE.md](../../../doc/deployment-integration/roles/provider/LOCAL_MARKETPLACE.md#trusting-a-private-ca)).
- **The Postgres operator** (Zalando): the profiles do not install it, see
  [Installing the operators once](../../../doc/deployment-integration/roles/PREREQUISITES.md#installing-the-operators-once).
- **The MongoDB operator**, for the marketplace only: see [MongoDB](../../../doc/deployment-integration/roles/provider/LOCAL_MARKETPLACE.md#mongodb).
- **A default StorageClass**, or set one where the profiles mention `<storage-class>`.

Outside the cluster: a DNS name for each public endpoint, pointing to the ingress controller.

## Placeholders

Replace every `<placeholder>` of the files you use. A placeholder in a comment marks an optional
setting.

| Placeholder | Profiles | Value |
|---|---|---|
| `<ingress-class>` | all | the class of your ingress controller, e.g. `nginx` |
| `<cluster-issuer>` | all | the ClusterIssuer of cert-manager |
| `<tir-domain>` | trust anchor | public host of the trust anchor's registry |
| `<tir-url>` | provider | the trust anchor's registry, `https://<tir-domain>` |
| `<did-domain>` | provider, consumer | host of the organization's `did:web` |
| `<did>` | provider, consumer | the organization's DID: `did:web:<did-domain>` |
| `<keycloak-domain>` | provider, consumer | public host of Keycloak, the credential issuer |
| `<realm>` | provider, consumer | name of the Keycloak realm that issues the credentials |
| `<verifier-domain>` | provider | public host of the verifier |
| `<data-service-domain>` | provider | public host of the data service, served by APISIX |
| `<data-service-upstream>` | provider | the data service inside the cluster, as `host:port` |
| `<tmf-domain>` | provider | public host of the TM Forum API, served by APISIX |
| `<consumer-did>` | provider | a consumer whose users may use the provider's services |
| `<provider-did>` | consumer | the provider whose roles the consumer's users hold |
| `<marketplace-domain>` | provider with marketplace | public host of the marketplace portal |

Every placeholder of a file has to be replaced before installing it. To check that none is left
outside the comments:

```shell
grep -n -v '^[[:space:]]*#' provider.yaml | grep '<[a-z][a-z-]*>'
```

## Secrets to create first

The charts generate the passwords of the databases. These have to exist before the install, in
the namespace of the release - create them from your secret store, with External Secrets or
Sealed Secrets in a GitOps setup:

| Secret | Key | Profiles | Used for |
|---|---|---|---|
| `keystore-password` | `password` | provider, consumer | the keystore Keycloak signs the credentials with |
| `mongodb-admin-password` | `password` | provider with marketplace | MongoDB's administrator |
| `mongodb-charging-password` | `password` | provider with marketplace | the marketplace's charging backend |
| `mongodb-belp-password` | `password` | provider with marketplace | the marketplace's logic proxy |

For example:

```shell
kubectl create namespace provider
kubectl -n provider create secret generic keystore-password --from-literal=password="$(openssl rand -hex 24)"
```

## Installation

Install the trust anchor first, then the participants. Pin the version of the charts (see the
[release notes](../../../doc/release-notes)):

```shell
helm install trust-anchor oci://quay.io/fiware/helm-charts/trust-anchor --version <chart-version> \
  --namespace trust-anchor --create-namespace -f trust-anchor.yaml

helm install provider oci://quay.io/fiware/helm-charts/data-space-connector --version <chart-version> \
  --namespace provider -f provider.yaml            # add -f provider-marketplace.yaml for the marketplace

helm install consumer oci://quay.io/fiware/helm-charts/data-space-connector --version <chart-version> \
  --namespace consumer --create-namespace -f consumer.yaml
```

## Registering the participants at the trust anchor

The trust anchor only publishes its registry. Participants are added through its trusted issuers
list API, which stays inside the cluster; the operator of the trust anchor registers each DID:

```shell
kubectl -n trust-anchor port-forward svc/tir 8080:8080 &
curl -X POST http://localhost:8080/issuer -H 'Content-Type: application/json' \
  -d '{"did": "<did of the participant>", "credentials": []}'
```

The participant then appears at `https://<tir-domain>/v4/issuers/<did>`.

## Users and credentials

The profiles import no users into Keycloak. Create them in the realm, or federate it with your
identity provider, and give each user the client roles of the credentials it needs: in the
consumer, the roles at `<provider-did>`. The admin console is not published; reach it with
`kubectl -n consumer port-forward svc/consumer-keycloak 8080:8080` and log in with the user
`keycloak-admin` and the password in the Secret `issuance-secret` (key `keycloak-admin`).

A user then gets a credential with a wallet, from `https://<keycloak-domain>/realms/<realm>`, and
presents it to the provider's services. The access to each service is decided by the provider's
ODRL policies, managed through its PAP (see the [provider guide](../../../doc/deployment-integration/roles/provider/README.md)).

## Checking the deployment

[fdsc-verify](https://github.com/SEAMWARE/fdsc-verify) checks that a connector is built and wired
as expected. By hand:

- the DID document: `https://<did-domain>/.well-known/did.json`;
- the credential issuer: `https://<keycloak-domain>/realms/<realm>/.well-known/openid-credential-issuer`;
- the data service asks for a credential: `https://<data-service-domain>/.well-known/openid-configuration`
  answers, and a request without a token gets a `401`.

## Before going to production

The profiles leave out what depends on each installation:

- **Sizing and availability:** one replica of each component, the charts' default resources. See
  [Sizing](../../../doc/deployment-integration/roles/PREREQUISITES.md#sizing) in the prerequisites.
- **Backups** of the databases (PostgreSQL, and MongoDB for the marketplace).
- **TLS between the components and PostgreSQL**, which the charts do not require yet.
- **An existing PostgreSQL** instead of the one the chart deploys: see Databases in the
  [provider guide](../../../doc/deployment-integration/roles/provider/README.md#databases).
- **The DID's key**: the profiles keep it in the certificate of the DID's ingress, which
  cert-manager never rotates. Keep a backup of it outside the cluster, see
  [Identity key lifecycle](../../../doc/deployment-integration/roles/provider/README.md#identity-key-lifecycle).
