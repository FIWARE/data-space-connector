#!/bin/bash

wget "https://get.helm.sh/helm-v3.15.2-linux-amd64.tar.gz"
tar zxf helm-v3.15.2-linux-amd64.tar.gz
mkdir bin
mv linux-amd64/helm ./bin/helm

go install github.com/yannh/kubeconform/cmd/kubeconform@latest
