## Postgres Operator

The postgres-operator can be installed in your cluster as described [here](https://github.com/zalando/postgres-operator/blob/master/docs/quickstart.md#deployment-options). As ```helm template``` does not normally render the necessary CRDs, include the ```--include-crds``` option when updating this operator