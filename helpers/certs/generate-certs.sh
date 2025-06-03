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

# Only create ca if it does not exist. It allows to update the client certificates whitout having to update the ca everywhere
if [ ! -f ${OUTPUT_FOLDER}/ca/certs/cacert.pem ]; then

  mkdir -p ${OUTPUT_FOLDER}/ca/private
  mkdir -p ${OUTPUT_FOLDER}/ca/csr
  mkdir -p ${OUTPUT_FOLDER}/ca/certs

  # generate key
  openssl genrsa -out ${OUTPUT_FOLDER}/ca/private/cakey.pem 4096
  # create CA Request
  openssl req -new -x509 -set_serial 01 -days 3650 \
    -config ./config/openssl.cnf \
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
  openssl req -new -sha256 -set_serial 02 -config ./config/openssl-intermediate.cnf \
    -subj "/C=DE/ST=Saxony/L=Dresden/O=FICODES CA/CN=FICODES-INTERMEDIATE/emailAddress=ca@ficodes.com/serialNumber=02" \
    -key ${OUTPUT_FOLDER}/intermediate/private/intermediate.cakey.pem \
    -out ${OUTPUT_FOLDER}/intermediate/csr/intermediate.csr.pem
  openssl ca -config ./config/openssl.cnf -extensions v3_intermediate_ca -days 2650 -notext \
    -batch -in ${OUTPUT_FOLDER}/intermediate/csr/intermediate.csr.pem \
    -out ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem
  openssl x509 -in ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem -out ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem -outform PEM
  openssl x509 -noout -text -in ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem

  cat ${OUTPUT_FOLDER}/intermediate/certs/intermediate.cacert.pem ${OUTPUT_FOLDER}/ca/certs/cacert.pem > ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem

else
  echo "Intermediate CA already exists, skipping generation."
fi

# client
mkdir -p ${OUTPUT_FOLDER}/client/private
mkdir -p ${OUTPUT_FOLDER}/client/csr
mkdir -p ${OUTPUT_FOLDER}/client/certs

openssl ecparam -name prime256v1 -genkey -noout -out ${OUTPUT_FOLDER}/client/private/client.key.pem
#openssl genrsa -out ${OUTPUT_FOLDER}/client/private/client.key.pem 4096
openssl req -new -set_serial 03 -key ${OUTPUT_FOLDER}/client/private/client.key.pem -out ${OUTPUT_FOLDER}/client/csr/client.csr \
  -config ./config/openssl-client.cnf
openssl x509 -req -in ${OUTPUT_FOLDER}/client/csr/client.csr -CA ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem \
  -CAkey ${OUTPUT_FOLDER}/intermediate/private/intermediate.cakey.pem -out ${OUTPUT_FOLDER}/client/certs/client.cert.pem \
  -CAcreateserial -days 1825 -sha256 -extfile ./config/openssl-client.cnf \
  -copy_extensions=copyall \
  -extensions v3_req

openssl x509 -in ${OUTPUT_FOLDER}/client/certs/client.cert.pem -out ${OUTPUT_FOLDER}/client/certs/client.cert.pem -outform PEM

cat ${OUTPUT_FOLDER}/client/certs/client.cert.pem ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem > ${OUTPUT_FOLDER}/client/certs/client-chain-bundle.cert.pem

# create keystore to be used by keycloak
openssl pkcs12 -export -password pass:password -in ${OUTPUT_FOLDER}/client/certs/client-chain-bundle.cert.pem -inkey ${OUTPUT_FOLDER}/client/private/client.key.pem -out ${OUTPUT_FOLDER}/certificate.p12 -name "certificate"
keytool -importkeystore -srckeystore ${OUTPUT_FOLDER}/certificate.p12 -srcstoretype pkcs12 -destkeystore ${OUTPUT_FOLDER}/cert.jks -srcstorepass password -deststorepass password -destkeypass password

kubectl create configmap consumer-keystore --from-file=${OUTPUT_FOLDER}/cert.jks --namespace consumer --dry-run=client -oyaml > ${k3sFolder}/consumer/keystore-cm.yaml
kubectl create secret tls local-wildcard --cert=${OUTPUT_FOLDER}/client/certs/client-chain-bundle.cert.pem --key=${OUTPUT_FOLDER}/client/private/client.key.pem --namespace infra -o yaml --dry-run=client > ${k3sFolder}/certs/local-wildcard.yaml
kubectl create secret generic gx-registry-keypair --from-file=PRIVATE_KEY=${OUTPUT_FOLDER}/ca/private/cakey-pkcs8.pem --from-file=X509_CERTIFICATE=${OUTPUT_FOLDER}/ca/certs/cacert.pem --namespace infra -o yaml --dry-run=client > ${k3sFolder}/infra/gx-registry/secret.yaml
kubectl create secret generic root-ca --from-file=${OUTPUT_FOLDER}/ca/certs/cacert.pem --namespace provider -o yaml --dry-run=client > ${k3sFolder}/provider/root-ca.yaml
kubectl create secret generic cert-chain --from-file=${OUTPUT_FOLDER}/client/certs/client-chain-bundle.cert.pem --namespace consumer -o yaml --dry-run=client > ${k3sFolder}/consumer/cert-chain.yaml
kubectl create secret generic cert-chain --from-file=${OUTPUT_FOLDER}/client/certs/client-chain-bundle.cert.pem --namespace provider -o yaml --dry-run=client > ${k3sFolder}/provider/cert-chain.yaml
kubectl create secret generic signing-key --from-file=${OUTPUT_FOLDER}/client/private/client.key.pem --namespace provider -o yaml --dry-run=client > ${k3sFolder}/provider/signing-key.yaml

ca=$(cat ${OUTPUT_FOLDER}/ca/certs/cacert.pem | sed '/-----BEGIN CERTIFICATE-----/d' | sed '/-----END CERTIFICATE-----/d' | tr -d '\n')
yq -i "(.spec.template.spec.initContainers[] | select(.name == \"local-trust\") | .env[] | select(.name == \"ROOT_CA\")).value = \"$ca\"" ${k3sFolder}/infra/gx-registry/deployment-registry.yaml


openssl x509 -in ${OUTPUT_FOLDER}/client/certs/client.cert.pem -noout -pubkey > ${OUTPUT_FOLDER}/client/certs/public_key.pem

n=$(openssl rsa -in ${OUTPUT_FOLDER}/client/certs/public_key.pem -pubin -modulus -noout | cut -d'=' -f2 | xxd -r -p | base64 -w 0 | tr -d '=' | tr '/+' '_-')
e_dec=$(openssl rsa -in ${OUTPUT_FOLDER}/client/certs/public_key.pem -pubin -text -noout | grep "Exponent" | awk '{print $2}')
if [ "$e_dec" -eq 65537 ]; then
    e="AQAB"
else
    e=$(printf "%%x" "$e_dec" | xxd -r -p | base64 | tr -d '=' | tr '/+' '_-')
fi

chain=$(cat ${OUTPUT_FOLDER}/client/certs/client-chain-bundle.cert.pem)

yq -i ".didJson.key.modulus = \"${n}\"" ${k3sFolder}/consumer-gaia-x.yaml
yq -i ".didJson.key.exponent = \"${e}\"" ${k3sFolder}/consumer-gaia-x.yaml