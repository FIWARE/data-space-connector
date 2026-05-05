# Charts

This directory provides the actual charts of the connector.


## Data Space Connector

The folder [data-space-connector](./data-space-connector) contains the actual FIWARE 
Data Space Connector as a [Helm Umbrella Chart](https://helm.sh/docs/howto/charts_tips_and_tricks/#complex-charts-with-many-dependencies). This includes the `Chart.yaml` with the different depending charts for the components, a `values.yaml` providing default values for the configuration parameters of the different components, and additional Helm templates. 

### Secret management

A few components (Keycloak issuance, MongoDB users for the marketplace) require Secrets that hold credentials. The chart can bootstrap these for you, but in production you usually do not want the chart to own them.

The chart-controlled paths are:

- `issuance.generatePasswords.enabled` (default `true`): generates `issuance-secret` on first install and reuses the existing value on subsequent renders via `lookup`.
- `marketplace.generatePasswords` (default `true`): on `helm install` only, generates `mongodb-admin-password`, `mongodb-charging-password` and `mongodb-belp-password` with random values. These Secrets are annotated with `helm.sh/resource-policy: keep`, so Helm will never update or delete them, the passwords are not rotated, and they survive `helm upgrade` and `helm uninstall`.

For production deployments — and for any environment driven by GitOps tools (ArgoCD, Flux) where the Helm `lookup` function is not available by default — set both flags to `false` and provide the Secrets externally before installing the chart. Recommended options:

- **External Secrets Operator** (Vault, AWS Secrets Manager, GCP Secret Manager, 1Password, …): the operator materialises the required Secrets from your secret store. Independent of the deploy tool.
- **Sealed Secrets / SOPS**: keep the encrypted Secrets in Git and let the controller/decryptor in the cluster materialise them.
- **Plain `kubectl apply` from a previous CI step**, a helmfile `presync` hook, or an ArgoCD App with a lower sync wave.

The Secret names the components expect (when `*.generatePasswords` is `false`) are:

- `issuance-secret` — keys: `keycloak-admin` (and any additional component-specific keys you want to override).
- `mongodb-admin-password` — key: `password`.
- `mongodb-charging-password` — key matches `marketplace.bizEcosystemChargingBackend.db.secretKey` (default `password`).
- `mongodb-belp-password` — key matches `marketplace.bizEcosystemLogicProxy.db.secretKey` (default `password`).



## Trust Anchor

The folder [trust-anchor](./trust-anchor) contains a minimal example of a Trust Anchor, provided as 
a [Helm Umbrella Chart](https://helm.sh/docs/howto/charts_tips_and_tricks/#complex-charts-with-many-dependencies). Basically it consists of a Trusted Issuers Registry with an attached database. This is also used in the local deployment of a Minimum Viable Dataspace described [here](../doc/deployment-integration/local-deployment/LOCAL.MD).
