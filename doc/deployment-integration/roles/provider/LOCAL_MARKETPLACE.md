# Running a local marketplace

A provider can run its own marketplace, the [Business API Ecosystem](https://github.com/FIWARE/business-api-ecosystem) (BAE), as part of its connector: a web portal where its offerings are published, browsed and ordered. This guide sets it up with the `data-space-connector` chart, for a provider that already follows the [provider guide](./README.md).

If the data space runs a shared marketplace instead, see [Participating in a Central Marketplace](./README.md#participating-in-a-central-marketplace). For the local demo with a mobile wallet, see [Marketplace integration](../../../MARKETPLACE_INTEGRATION.md).

<!-- ToC created with: https://github.com/thlorenz/doctoc -->

- [What gets deployed](#what-gets-deployed)
- [Prerequisites](#prerequisites)
  - [MongoDB](#mongodb)
- [Values](#values)
  - [TMForum API and contract management](#tmforum-api-and-contract-management)
  - [The chart's MongoDB](#the-charts-mongodb)
  - [The marketplace](#the-marketplace)
  - [Login with Verifiable Credentials](#login-with-verifiable-credentials)
  - [Trusting a private CA](#trusting-a-private-ca)
- [Common mistakes](#common-mistakes)
- [Security](#security)
- [Installing and checking it](#installing-and-checking-it)

## What gets deployed

| Component | Role |
|---|---|
| BAE logic proxy (`marketplace.bizEcosystemLogicProxy`) | The portal and the API the portal uses |
| BAE charging backend (`marketplace.bizEcosystemChargingBackend`) | Orders, billing and the assets attached to offerings |
| TMForum API (`tm-forum-api`) | Where offerings, orders and parties are stored; the BAE is a front end for it |
| MongoDB | The BAE's own data: an existing MongoDB, or one the chart deploys (`managedMongo`) |
| Contract management (`contract-management`) | Reacts to completed orders: grants the buyer access in the trusted issuers list and installs the policies of the ordered product |
| VCVerifier | Users log in to the portal by presenting a Verifiable Credential |

The data service itself, Keycloak, the DID and the VCVerifier are the ones of the provider deployment.

## Prerequisites

### MongoDB

The marketplace stores its data in MongoDB. Use the one you already have, or let the chart deploy one:

| Option | When | What it needs |
|---|---|---|
| **An existing MongoDB** | You already run one, or several connectors share one | `managedMongo.enabled: false`, and the marketplace's databases, users and passwords created in it. See [Using an existing MongoDB](#using-an-existing-mongodb) |
| **A MongoDB deployed by the chart** (`managedMongo`) | You have none | A MongoDB operator in the cluster. The chart creates a `MongoDBCommunity` named `mongodb` in the connector's namespace, with the marketplace's users |

#### The operator, for a MongoDB deployed by the chart

`managedMongo` needs an operator that handles `MongoDBCommunity` resources: the [MongoDB Community Operator](https://github.com/mongodb/mongodb-kubernetes-operator), whose chart the connector pins (`community-operator` in [Chart.yaml](../../../../charts/data-space-connector/Chart.yaml), 0.13.0 at the time of writing), or its successor [MongoDB Controllers for Kubernetes](https://github.com/mongodb/mongodb-kubernetes), which manages the same resource. The operator is cluster-wide: install it once, outside the connector, and leave `mongo-operator.enabled: false` (the default) in the connector's values.

The MongoDB pods run as a `mongodb-database` service account (with its Role and RoleBinding) in their own namespace. The Community Operator chart creates it only in the namespace given in `database.namespace`, so where to point it decides whether you have to create it yourself:

- **`database.namespace` set to the connector's namespace** - the simplest setup with a single connector; nothing else to create:

  ```shell
  helm repo add mongodb https://mongodb.github.io/helm-charts
  helm install mongo-operator mongodb/community-operator --version 0.13.0 \
    -n mongo-operator --create-namespace \
    --set operator.watchNamespace="*" \
    --set database.namespace=<your_namespace>
  ```

  `watchNamespace: "*"` lets the operator manage databases outside its own namespace. The service account then belongs to the operator's release: if the connector's namespace is deleted, it goes with it, and the operator has to be upgraded to create it again before the database can start.

- **The operator managing several namespaces**, with `database.namespace` left at its own namespace: create the service account in the connector's namespace before installing it:

  ```yaml
  apiVersion: v1
  kind: ServiceAccount
  metadata:
    name: mongodb-database
    namespace: <your_namespace>
  ---
  apiVersion: rbac.authorization.k8s.io/v1
  kind: Role
  metadata:
    name: mongodb-database
    namespace: <your_namespace>
  rules:
    - apiGroups: [""]
      resources: [secrets]
      verbs: [get]
    - apiGroups: [""]
      resources: [pods]
      verbs: [get, patch, delete]
  ---
  apiVersion: rbac.authorization.k8s.io/v1
  kind: RoleBinding
  metadata:
    name: mongodb-database
    namespace: <your_namespace>
  subjects:
    - kind: ServiceAccount
      name: mongodb-database
  roleRef:
    kind: Role
    name: mongodb-database
    apiGroup: rbac.authorization.k8s.io
  ```

  Without it the MongoDB pod never starts (`serviceaccounts "mongodb-database" not found`).

#### Database passwords

The chart's MongoDB has three users: `admin`, and one for each marketplace component (`charging`, `belp`). Their passwords come from three Secrets, each with a `password` key:

| Secret | User |
|---|---|
| `mongodb-admin-password` | `admin` |
| `mongodb-charging-password` | `charging` (charging backend) |
| `mongodb-belp-password` | `belp` (logic proxy) |

With `marketplace.generatePasswords: true` (the default), the chart generates them on the first install, with random values, and keeps them on upgrade and uninstall. That is convenient for a test environment. In production, create them yourself beforehand - from your secret store, External Secrets or Sealed Secrets - and set `generatePasswords: false`.

#### Using an existing MongoDB

Create a database and a user with `readWrite` on it for each marketplace component, store each password in a Secret of the connector's namespace, and point the components to them:

```yaml
managedMongo:
  enabled: false
marketplace:
  generatePasswords: false
  bizEcosystemChargingBackend:
    existingSecret: <charging_password_secret>   # the password under the key below
    db:
      host: <your_mongodb_host>
      database: <charging_database>
      user: <charging_user>
      secretKey: password
  bizEcosystemLogicProxy:
    existingSecret: <logic_proxy_password_secret>
    db:
      host: <your_mongodb_host>
      database: <logic_proxy_database>
      user: <logic_proxy_user>
      secretKey: password
```

## Values

The values below go into the provider's values file, next to those of the [provider guide](./README.md#using-the-fiware-dsc-helm-chart-and-values). Replace the placeholders; `<your_did>` is the provider's DID.

### TMForum API and contract management

Both are enabled by default; the marketplace needs them configured as follows:

```yaml
tm-forum-api:
  enabled: true
  allInOne:
    enabled: true          # all TMForum APIs in one deployment, served as tm-forum-api-svc
  apiProxy:
    enabled: false         # not needed with allInOne
  ingress:
    enabled: false         # reach it through APISIX only, see Security

contract-management:
  enabled: true
  did: <your_did>
  enableOdrlPap: true
  enableTrustedIssuersList: true
  organization:
    provider:
      role: seller         # the BAE records the provider of an offering with the role "seller"
  services:
    trusted-issuers-list:
      url: http://trusted-issuers-list:8080
    product-order:
      url: http://tm-forum-api-svc:8080
    party:
      url: http://tm-forum-api-svc:8080
    product-catalog:
      url: http://tm-forum-api-svc:8080
    service-catalog:
      url: http://tm-forum-api-svc:8080
    tmforum-agreement-api:
      url: http://tm-forum-api-svc:8080
    quote:
      url: http://tm-forum-api-svc:8080
    odrl:
      url: http://odrl-pap:8080
```

`organization.provider.role` is the `relatedParty` role by which contract management finds the provider of an ordered product. The chart default is `provider`, while the BAE records the provider as `seller`: without the setting, contract management does not find the provider of the orders placed in the BAE and activates nothing.

### The chart's MongoDB

When the chart deploys MongoDB:

```yaml
managedMongo:
  enabled: true
  spec:
    version: "8.0.12"      # the chart defaults to 6.0.5, which is out of support
    statefulSet:
      spec:
        volumeClaimTemplates:
          - metadata:
              name: data-volume
            spec:
              accessModes: [ReadWriteOnce]
              storageClassName: <your_storage_class>
              resources:
                requests:
                  storage: 5Gi
          - metadata:
              name: logs-volume
            spec:
              accessModes: [ReadWriteOnce]
              storageClassName: <your_storage_class>
              resources:
                requests:
                  storage: 1Gi
```

`managedMongo.spec` is passed as is to the `MongoDBCommunity` resource; resources for the `mongod` and `mongodb-agent` containers go under `statefulSet.spec.template.spec.containers`.

### The marketplace

```yaml
marketplace:
  enabled: true
  generatePasswords: false               # the Secrets above are created beforehand
  externalUrl: https://<your_marketplace_domain>
  tmForum:                               # the TMForum APIs, all served by tm-forum-api-svc
    catalog:           {host: tm-forum-api-svc, port: 8080, path: /tmf-api/productCatalogManagement/v4}
    inventory:         {host: tm-forum-api-svc, port: 8080, path: /tmf-api/productInventory/v4}
    ordering:          {host: tm-forum-api-svc, port: 8080, path: /tmf-api/productOrderingManagement/v4}
    billing:           {host: tm-forum-api-svc, port: 8080, path: /tmf-api/accountManagement/v4}
    usage:             {host: tm-forum-api-svc, port: 8080, path: /tmf-api/usageManagement/v4}
    party:             {host: tm-forum-api-svc, port: 8080, path: /tmf-api/party/v4}
    customer:          {host: tm-forum-api-svc, port: 8080, path: /tmf-api/customerManagement/v4}
    resources:         {host: tm-forum-api-svc, port: 8080, path: /tmf-api/resourceCatalog/v4}
    services:          {host: tm-forum-api-svc, port: 8080, path: /tmf-api/serviceCatalogManagement/v4}
    resourceInventory: {host: tm-forum-api-svc, port: 8080, path: /tmf-api/resourceInventoryManagement/v4}
    customerBill:      {host: tm-forum-api-svc, port: 8080, path: /tmf-api/customerBillManagement/v4}
  bizEcosystemChargingBackend:
    loglevel: info
    media:
      enabled: true                      # storage for the assets attached to offerings
      size: 2Gi
      storageClass: <your_storage_class>
  bizEcosystemLogicProxy:
    purchaseEnabled: true                # let consumers order offerings
    dataSpaceEnabled: true               # data space features in the portal
    ingress:
      enabled: true
      className: <your-ingress-class>    # add the annotations your ingress controller needs
      annotations:
        cert-manager.io/cluster-issuer: "prod"
      hosts:
        - host: <your_marketplace_domain>
          paths: ["/"]
      tls:
        - secretName: <your_marketplace_domain>-tls
          hosts:
            - <your_marketplace_domain>
```

To create offerings whose data is exchanged over the Dataspace Protocol, also add `BAE_LP_DSP_ENABLED=true` to the logic proxy's environment, see [Using the BAE Marketplace](../../../DSP_INTEGRATION.md#using-the-bae-marketplace).

### Login with Verifiable Credentials

Users log in to the portal by presenting a Verifiable Credential to the provider's VCVerifier, which acts as the OpenID Connect provider of the BAE. Three pieces have to agree: the credential Keycloak issues, the service registered at the verifier, and the BAE's OIDC client.

**1. The credential.** The user presents a `LegalPersonCredential` with their e-mail and names, and their marketplace roles in a `roles` claim. The BAE builds the user's profile from it: the e-mail is mandatory (the user name is derived from it), and the roles must be named as the BAE expects them - `seller` to publish offerings, `customer` to order them, `admin` - or as set in `marketplace.oauth.sellerrole`, `customerrole` and `adminrole`. In Keycloak this is a credential scope with these mappers, and the roles are client roles of the provider's DID client assigned to the user - see the [Keycloak guide](../KEYCLOAK.md) for the credential configuration:

```yaml
keycloak:
  realm:
    verifiableCredentials:
      legal-person-credential:
        attributes:
          format: "dc+sd-jwt"
          verifiable_credential_type: "LegalPersonCredential"
          credential_signing_alg: "ES256"
          credential_build_config.token_jws_type: "dc+sd-jwt"
          credential_build_config.sd_jwt.visible_claims: "iss,iat,nbf,exp,cnf,vct,status,roles,email"
        protocolMappers:
          - name: email-mapper
            protocol: oid4vc
            protocolMapper: oid4vc-user-attribute-mapper
            config:
              claim.name: email
              userAttribute: email
          - name: firstName-mapper
            protocol: oid4vc
            protocolMapper: oid4vc-user-attribute-mapper
            config:
              claim.name: firstName
              userAttribute: firstName
          - name: familyName-mapper
            protocol: oid4vc
            protocolMapper: oid4vc-user-attribute-mapper
            config:
              claim.name: familyName
              userAttribute: lastName
          - name: role-mapper
            protocol: oid4vc
            protocolMapper: oid4vc-target-role-mapper
            config:
              claim.name: roles
              clientId: <your_did>             # the client the marketplace roles are defined on
```

**2. The service at the verifier.** Register the marketplace as a service that requests that credential and passes its roles into the token:

```yaml
decentralizedIam:
  vcAuthentication:
    vcverifier:
      registration:
        enabled: true
        services:
          - id: <your_did>                     # the BAE's OIDC client id
            defaultOidcScope: "openid learcredential"
            authorizationType: "FRONTEND_V2"
            oidcScopes:
              "openid learcredential":
                credentials:
                  - type: LegalPersonCredential
                    trustedParticipantsLists:
                      - <trust_anchor_tir_url>
                    trustedIssuersLists:
                      - http://trusted-issuers-list:8080
                    jwtInclusion:
                      enabled: true
                      fullInclusion: true      # the BAE builds the user's profile from the credential
                dcql:
                  credentials:
                    - id: legal-person-query
                      format: "dc+sd-jwt"
                      multiple: false
                      claims:
                        - id: roles-claim
                          path: [roles]
                      meta:
                        vct_values: [LegalPersonCredential]
```

Add it to the same `registration.services` list as the provider's other services: Helm replaces lists instead of merging them. A service that is already registered is not updated on upgrade; change it through the verifier's configuration API.

**3. The BAE's OIDC client.** The logic proxy uses its `oidc-discover` login, which reads the verifier's discovery document for that service:

```yaml
marketplace:
  siop:
    enabled: false
  oauth:
    provider: oidc-discover
    oidc: true
    server: https://<your_verifier_domain>
    callbackPath: /auth/oidc-discover/callback
  bizEcosystemLogicProxy:
    statefulset:
      image:
        tag: 11.24.0           # 11.22.0 or later: earlier versions do not build the profile from the credential
    additionalEnvVars:
      # this list replaces the chart's default one, which only disables tracing
      - name: OTEL_SDK_DISABLED
        value: "true"
      - name: BAE_LP_OAUTH2_CLIENT_ID
        value: <your_did>      # the id of the service registered above
      - name: BAE_LP_OAUTH2_CLIENT_SECRET
        value: unused          # the verifier does not check it
      - name: BAE_LP_OIDC_DISCOVERY_URI
        value: https://<your_verifier_domain>/services/<your_did>/.well-known/openid-configuration
      - name: BAE_LP_OIDC_SCOPES
        value: "openid learcredential"
      - name: BAE_LP_OAUTH2_NONCE
        value: "true"
```

The client id and secret go into the environment directly: setting `oauth.clientId` makes the chart read the secret from an `oauthClientSecret` key of the logic proxy's database Secret.

The marketplace chart can also log users in with SIOP (`marketplace.siop`). In its redirection mode (`BAE_LP_SIOP_IS_REDIRECTION`) the logic proxy signs its token requests with its own key, using the client id as key id, and the verifier resolves that key id as a DID to check them: the client id has to be the provider's DID and the key the DID's key. The OIDC login above has none of these constraints.

### Trusting a private CA

The logic proxy is a Node.js application: it calls the verifier over HTTPS and only trusts the public CAs. If the verifier's certificate comes from a private CA, mount the CA and point `NODE_EXTRA_CA_CERTS` to it, adding to the `additionalEnvVars` above:

```yaml
marketplace:
  bizEcosystemLogicProxy:
    additionalEnvVars:
      # ... the variables of the previous section, and:
      - name: NODE_EXTRA_CA_CERTS
        value: /private-ca/ca.crt
    extraVolumeMounts:
      - name: private-ca
        mountPath: /private-ca
        readOnly: true
    extraContainerVolumes:
      - name: private-ca
        secret:
          secretName: <your_ca_secret>       # a Secret with the CA certificate under ca.crt
          items:
            - key: ca.crt
              path: ca.crt
```

Do not set `NODE_TLS_REJECT_UNAUTHORIZED=0` instead: it disables certificate verification for every call the logic proxy makes. The local deployment in [`k3s/`](../../../../k3s) uses it, but those values are meant for a test environment only.

## Common mistakes

| Symptom | Cause |
|---|---|
| The portal ignores the data space settings | `DATASPACE_ENABLED` is not read by the logic proxy; use `bizEcosystemLogicProxy.dataSpaceEnabled: true` |
| The same variable twice in the logic proxy, or an upgrade fails with `The order in patch list doesn't match $setElementOrder list` | A variable the chart already renders from a value - `BAE_LP_PURCHASE_ENABLED`, `BAE_LP_DATASPACE_ENABLED`, `BAE_LP_SIOP_*` - was also added to `additionalEnvVars`. Use the values |
| Tracing settings disappear | `additionalEnvVars` (logic proxy) and `extraEnvVars` (charging backend) replace the chart's lists, which hold the `OTEL_*` variables; keep `OTEL_SDK_DISABLED` |
| `bizEcosystemApis.tmForum` in older examples | The legacy name of `tmForum`; both work, `tmForum` wins |
| A change to the BAE's service at the verifier has no effect | The registration does not update a service that already exists; change it through the verifier's configuration API |
| Orders are completed but nothing happens | Contract management without `organization.provider.role: seller` |
| The chart's MongoDB pod does not start (`serviceaccounts "mongodb-database" not found`) | The operator did not create the service account in the provider's namespace, see [the operator](#the-operator-for-a-mongodb-deployed-by-the-chart) |

## Security

- **Do not publish the TMForum API or Scorpio.** The BAE reaches them inside the cluster. If other participants need the TMForum API, publish it through APISIX, protected by the VCVerifier and ODRL policies, as the provider guide does for the data service.
- **The logic proxy's Party API answers without authentication** (`/party/individual` returns the names and e-mail addresses of the registered individuals), and the catalog returns offerings that are still `In design` to anonymous users. Until the BAE restricts them, keep the portal's audience in mind and consider blocking those paths for anonymous callers at your ingress.
- The marketplace chart sets no resource limits on its containers, and its `wait-for-mongodb` init container uses the outdated `bitnamilegacy/mongodb:3.6.21` image.

## Installing and checking it

1. With the chart's MongoDB: install the MongoDB operator, once per cluster, and create the `mongodb-database` service account if the operator does not. With an existing MongoDB: create the marketplace's databases and users in it.
2. Create the password Secrets in the provider's namespace, unless `generatePasswords` is used.
3. Install or upgrade the connector with `--wait` and a generous `--timeout`, e.g. `30m`. On a fresh install the charging backend and contract management restart a few times until the TMForum API and MongoDB are up; that is expected.
4. Check that the logic proxy (`<release>-biz-ecosystem-logic-proxy-0`) and the charging backend are `Running`.
5. Open `https://<your_marketplace_domain>`, log in with a wallet holding the `LegalPersonCredential`, and check that a user with the `seller` role can create an offering.
