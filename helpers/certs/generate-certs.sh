#!/bin/bash

cd "$(dirname "$0")"

# cleanup previous client certs
rm -r out/client > /dev/null

find ./out -maxdepth 1 -type f -delete

k3sFolder=$1

set -x
set -e

OUTPUT_FOLDER="${OUTPUT_FOLDER:-./out}"
CONFIG_FILE="${CONFIG_FILE:-./config}"

export OUTPUT_FOLDER=$OUTPUT_FOLDER

mkdir -p ${OUTPUT_FOLDER}
echo -n "" > ${OUTPUT_FOLDER}/index.txt
echo -n "01" > ${OUTPUT_FOLDER}/serial
echo -n "1000" > ${OUTPUT_FOLDER}/crlnumber

# Determine whether the installed OpenSSL supports the -copy_extensions option.
# LibreSSL on macOS doesn't support it, so disable when running under LibreSSL.
if openssl version 2>&1 | grep -qi libre; then
  COPY_EXTS=""
else
  COPY_EXTS="-copy_extensions=copyall"
fi

# Create temporary OpenSSL config files with the correct `dir` value so
# LibreSSL/OpenSSL both receive a concrete path (avoids $ENV::... issues).
TMP_OPENSSL_CNF=$(mktemp)
TMP_OPENSSL_INTERMEDIATE_CNF=$(mktemp)
sed "s|^dir[[:space:]]*=.*$|dir = ${OUTPUT_FOLDER}|" ${CONFIG_FILE}/openssl.cnf > ${TMP_OPENSSL_CNF}
sed "s|^dir[[:space:]]*=.*$|dir = ${OUTPUT_FOLDER}|" ${CONFIG_FILE}/openssl-intermediate.cnf > ${TMP_OPENSSL_INTERMEDIATE_CNF}

# Ensure temp files are removed on exit
trap 'rm -f "${TMP_OPENSSL_CNF}" "${TMP_OPENSSL_INTERMEDIATE_CNF}"' EXIT

# Only create ca if it does not exist. It allows to update the client certificates whitout having to update the ca everywhere
if [ ! -f ${OUTPUT_FOLDER}/ca/certs/cacert.pem ]; then

  mkdir -p ${OUTPUT_FOLDER}/ca/private
  mkdir -p ${OUTPUT_FOLDER}/ca/csr
  mkdir -p ${OUTPUT_FOLDER}/ca/certs

  # generate key
  openssl genrsa -out ${OUTPUT_FOLDER}/ca/private/cakey.pem 4096
  # create CA Request (use temp config with correct dir)
  openssl req -new -x509 -set_serial 01 -days 3650 \
    -config "${TMP_OPENSSL_CNF}" \
    -extensions v3_ca \
    -key ${OUTPUT_FOLDER}/ca/private/cakey.pem \
    -out ${OUTPUT_FOLDER}/ca/csr/cacert.pem \
    -subj "/C=DE/ST=Saxony/L=Dresden/O=FICODES CA/CN=FICODES-CA/serialNumber=01"

  ## Convert x509 CA cert
  openssl x509 -in ${OUTPUT_FOLDER}/ca/csr/cacert.pem -out ${OUTPUT_FOLDER}/ca/certs/cacert.pem -outform PEM

  openssl pkcs8 -topk8 -nocrypt -in ${OUTPUT_FOLDER}/ca/private/cakey.pem -out ${OUTPUT_FOLDER}/ca/private/cakey-pkcs8.pem

else
  echo "CA already exists, skipping generation."
fi

if [ ! -f "${OUTPUT_FOLDER}/intermediate/private/intermediate.cakey.pem" ] || [ ! -f "${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem" ]; then

  # Intermediate
  mkdir -p ${OUTPUT_FOLDER}/intermediate/private
  mkdir -p ${OUTPUT_FOLDER}/intermediate/csr
  mkdir -p ${OUTPUT_FOLDER}/intermediate/certs


  openssl genrsa -out ${OUTPUT_FOLDER}/intermediate/private/intermediate.cakey.pem 4096
  openssl req -new -sha256 -set_serial 02 -config "${TMP_OPENSSL_INTERMEDIATE_CNF}" \
    -subj "/C=DE/ST=Saxony/L=Dresden/O=FICODES CA/CN=FICODES-INTERMEDIATE/emailAddress=ca@ficodes.com/serialNumber=02" \
    -key ${OUTPUT_FOLDER}/intermediate/private/intermediate.cakey.pem \
    -out ${OUTPUT_FOLDER}/intermediate/csr/intermediate.csr.pem
  openssl ca -config "${TMP_OPENSSL_CNF}" -extensions v3_intermediate_ca -days 2650 -notext \
    -batch -in ${OUTPUT_FOLDER}/intermediate/csr/intermediate.csr.pem \
    -out ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem
  openssl x509 -in ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem -out ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem -outform PEM
  openssl x509 -noout -text -in ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem

  cat ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem ${OUTPUT_FOLDER}/ca/certs/cacert.pem > ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem

else
  echo "Intermediate CA already exists, skipping generation."
fi

# consumer client
mkdir -p ${OUTPUT_FOLDER}/client-consumer/private
mkdir -p ${OUTPUT_FOLDER}/client-consumer/csr
mkdir -p ${OUTPUT_FOLDER}/client-consumer/certs

openssl ecparam -name prime256v1 -genkey -noout -out ${OUTPUT_FOLDER}/client-consumer/private/client.key.pem

openssl req -new -set_serial 03 -key ${OUTPUT_FOLDER}/client-consumer/private/client.key.pem -out ${OUTPUT_FOLDER}/client-consumer/csr/client.csr \
  -config ./config/openssl-client-consumer.cnf
openssl x509 -req -in ${OUTPUT_FOLDER}/client-consumer/csr/client.csr -CA ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem \
  -CAkey ${OUTPUT_FOLDER}/intermediate/private/intermediate.cakey.pem -out ${OUTPUT_FOLDER}/client-consumer/certs/client.cert.pem \
  -CAcreateserial -days 1825 -sha256 -extfile ./config/openssl-client-consumer.cnf \
  ${COPY_EXTS} \
  -extensions v3_req

openssl x509 -in ${OUTPUT_FOLDER}/client-consumer/certs/client.cert.pem -out ${OUTPUT_FOLDER}/client-consumer/certs/client.cert.pem -outform PEM

cat ${OUTPUT_FOLDER}/client-consumer/certs/client.cert.pem ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem > ${OUTPUT_FOLDER}/client-consumer/certs/client-chain-bundle.cert.pem

# provider client
mkdir -p ${OUTPUT_FOLDER}/client-provider/private
mkdir -p ${OUTPUT_FOLDER}/client-provider/csr
mkdir -p ${OUTPUT_FOLDER}/client-provider/certs

openssl ecparam -name prime256v1 -genkey -noout -out ${OUTPUT_FOLDER}/client-provider/private/client.key.pem

openssl req -new -set_serial 03 -key ${OUTPUT_FOLDER}/client-provider/private/client.key.pem -out ${OUTPUT_FOLDER}/client-provider/csr/client.csr \
  -config ./config/openssl-client-provider.cnf
openssl x509 -req -in ${OUTPUT_FOLDER}/client-provider/csr/client.csr -CA ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem \
  -CAkey ${OUTPUT_FOLDER}/intermediate/private/intermediate.cakey.pem -out ${OUTPUT_FOLDER}/client-provider/certs/client.cert.pem \
  -CAcreateserial -days 1825 -sha256 -extfile ./config/openssl-client-provider.cnf \
  ${COPY_EXTS} \
  -extensions v3_req

openssl x509 -in ${OUTPUT_FOLDER}/client-provider/certs/client.cert.pem -out ${OUTPUT_FOLDER}/client-provider/certs/client.cert.pem -outform PEM

cat ${OUTPUT_FOLDER}/client-provider/certs/client.cert.pem ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem > ${OUTPUT_FOLDER}/client-provider/certs/client-chain-bundle.cert.pem

## create keystore to be used by keycloak
# consumer
openssl pkcs12 -export -password pass:password -in ${OUTPUT_FOLDER}/client-consumer/certs/client-chain-bundle.cert.pem -inkey ${OUTPUT_FOLDER}/client-consumer/private/client.key.pem -out ${OUTPUT_FOLDER}/client-consumer/certificate.p12 -name "certificate"
openssl pkcs12 -export -password pass:password -in ${OUTPUT_FOLDER}/client-consumer/certs/client.cert.pem -inkey ${OUTPUT_FOLDER}/client-consumer/private/client.key.pem -out ${OUTPUT_FOLDER}/client-consumer/keystore.pfx -name "certificate"

# provider
openssl pkcs12 -export -password pass:password -in ${OUTPUT_FOLDER}/client-provider/certs/client-chain-bundle.cert.pem -inkey ${OUTPUT_FOLDER}/client-provider/private/client.key.pem -out ${OUTPUT_FOLDER}/client-provider/certificate.p12 -name "certificate"
openssl pkcs12 -export -password pass:password -in ${OUTPUT_FOLDER}/client-provider/certs/client.cert.pem -inkey ${OUTPUT_FOLDER}/client-provider/private/client.key.pem -out ${OUTPUT_FOLDER}/client-provider/keystore.pfx -name "certificate"

# consumer
kubectl create secret tls tls-secret --cert=${OUTPUT_FOLDER}/client-consumer/certs/client-chain-bundle.cert.pem --key=${OUTPUT_FOLDER}/client-consumer/private/client.key.pem --namespace consumer -o yaml --dry-run=client > ${k3sFolder}/consumer/tls-secret.yaml
kubectl create secret generic consumer-keystore --from-file=keystore.pfx=${OUTPUT_FOLDER}/client-consumer/keystore.pfx --from-literal=password="password" --namespace=consumer --dry-run=client -oyaml > ${k3sFolder}/consumer/keystore-secret.yaml
kubectl create secret generic cert-chain --from-file=${OUTPUT_FOLDER}/client-consumer/certs/client-chain-bundle.cert.pem --namespace consumer -o yaml --dry-run=client > ${k3sFolder}/consumer/cert-chain.yaml

consumer_key_env=$(openssl ec -in ${OUTPUT_FOLDER}/client-consumer/private/client.key.pem -noout -text | grep 'priv:' -A 3 | tail -n +2 | tr -d ':\n ')
openssl pkcs8 -topk8 -nocrypt -in ${OUTPUT_FOLDER}/client-consumer/private/client.key.pem -out ${OUTPUT_FOLDER}/client-consumer/private/client-pkcs8.key.pem

kubectl create secret generic signing-key --from-file=${OUTPUT_FOLDER}/client-consumer/private/client.key.pem --from-file=${OUTPUT_FOLDER}/client-consumer/private/client-pkcs8.key.pem --namespace consumer -o yaml --dry-run=client > ${k3sFolder}/consumer/signing-key.yaml
kubectl create secret generic signing-key-env --from-literal=key="${consumer_key_env}" --namespace consumer -o yaml --dry-run=client > ${k3sFolder}/consumer/signing-key-env.yaml 


# provider
kubectl create secret tls tls-secret --cert=${OUTPUT_FOLDER}/client-provider/certs/client-chain-bundle.cert.pem --key=${OUTPUT_FOLDER}/client-provider/private/client.key.pem --namespace provider -o yaml --dry-run=client > ${k3sFolder}/provider/tls-secret.yaml
kubectl create secret generic provider-keystore --from-file=keystore.pfx=${OUTPUT_FOLDER}/client-provider/keystore.pfx --from-literal=password="password" --namespace=provider --dry-run=client -oyaml > ${k3sFolder}/provider/keystore-secret.yaml
kubectl create secret generic cert-chain --from-file=${OUTPUT_FOLDER}/client-provider/certs/client-chain-bundle.cert.pem --namespace provider -o yaml --dry-run=client > ${k3sFolder}/provider/cert-chain.yaml

provider_key_env=$(openssl ec -in ${OUTPUT_FOLDER}/client-provider/private/client.key.pem -noout -text | grep 'priv:' -A 3 | tail -n +2 | tr -d ':\n ')
openssl pkcs8 -topk8 -nocrypt -in ${OUTPUT_FOLDER}/client-provider/private/client.key.pem -out ${OUTPUT_FOLDER}/client-provider/private/client-pkcs8.key.pem

kubectl create secret generic signing-key --from-file=${OUTPUT_FOLDER}/client-provider/private/client.key.pem --from-file=${OUTPUT_FOLDER}/client-provider/private/client-pkcs8.key.pem --namespace provider -o yaml --dry-run=client > ${k3sFolder}/provider/signing-key.yaml
kubectl create secret generic signing-key-env --from-literal=key="${provider_key_env}" --namespace provider -o yaml --dry-run=client > ${k3sFolder}/provider/signing-key-env.yaml 

# infra
kubectl create secret tls local-wildcard --cert=${OUTPUT_FOLDER}/client-consumer/certs/client-chain-bundle.cert.pem --key=${OUTPUT_FOLDER}/client-consumer/private/client.key.pem --namespace infra -o yaml --dry-run=client > ${k3sFolder}/certs/local-wildcard.yaml
kubectl create secret generic gx-registry-keypair --from-file=PRIVATE_KEY=${OUTPUT_FOLDER}/ca/private/cakey-pkcs8.pem --from-file=X509_CERTIFICATE=${OUTPUT_FOLDER}/ca/certs/cacert.pem --namespace infra -o yaml --dry-run=client > ${k3sFolder}/infra/gx-registry/secret.yaml

# root ca is required to enable trust for the verifier
kubectl create secret generic root-ca --from-file=${OUTPUT_FOLDER}/ca/certs/cacert.pem --namespace provider -o yaml --dry-run=client > ${k3sFolder}/provider/root-ca.yaml
kubectl create secret generic root-ca --from-file=${OUTPUT_FOLDER}/ca/certs/cacert.pem --namespace consumer -o yaml --dry-run=client > ${k3sFolder}/consumer/root-ca.yaml


ca=$(cat ${OUTPUT_FOLDER}/ca/certs/cacert.pem | sed '/-----BEGIN CERTIFICATE-----/d' | sed '/-----END CERTIFICATE-----/d' | tr -d '\n')
yq -i "(.spec.template.spec.initContainers[] | select(.name == \"local-trust\") | .env[] | select(.name == \"ROOT_CA\")).value = \"$ca\"" ${k3sFolder}/infra/gx-registry/deployment-registry.yaml