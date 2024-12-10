# Charts

This directory provides the actual charts of the connector.


## Data Space Connector

The folder [data-space-connector](./data-space-connector) contains the actual FIWARE 
Data Space Connector as a [Helm Umbrella Chart](https://helm.sh/docs/howto/charts_tips_and_tricks/#complex-charts-with-many-dependencies). This includes the `Chart.yaml` with the different depending charts for the components, a `values.yaml` providing default values for the configuration parameters of the different components, and additional Helm templates. 


## Trust Anchor

The folder [trust-anchor](./trust-anchor) contains a minimal example of a Trust Anchor, provided as 
a [Helm Umbrella Chart](https://helm.sh/docs/howto/charts_tips_and_tricks/#complex-charts-with-many-dependencies). Basically it consists of a Trusted Issuers Registry with an attached database. This is also used in the local deployment of a Minimal Viable Dataspace described [here](../doc/deployment-integration/local-deployment/LOCAL.MD).

