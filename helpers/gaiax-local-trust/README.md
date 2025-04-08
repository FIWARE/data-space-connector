# GAIA-X Local Trust

In order to have a [Gaia-X Registry Service](https://gitlab.com/gaia-x/lab/compliance/gx-registry) working, it needs to have access to the Trust Anchors Lists of Gaia-X. They are usually provided through [IPFS](https://gitlab.com/gaia-x/lab/compliance/gx-ipfs-pinning), thus its not trivial to insert a local CA for demo purposes. The ```quay.io/wi_stefan/gaiax-local-trust:0.0.1``` can be used to generate a compliant file and folder structure to be used by the registry service, with a custom ROOT-CA included.
Run the container and find the files int $(pwd)/out via:

```shell
    docker run -v $(pwd)/out:/out -ROOT_CA "<THE_CA>" quay.io/wi_stefan/gaiax-local-trust:0.0.1
```