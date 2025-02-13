set -x
set -e

OUTPUT_FOLDER="${OUTPUT_FOLDER:-./out}"
CONFIG_FILE="${CONFIG_FILE:-./config}"

export OUTPUT_FOLDER=$OUTPUT_FOLDER

mkdir -p ${OUTPUT_FOLDER}
echo -n "" > ${OUTPUT_FOLDER}/index.txt
echo -n "01" > ${OUTPUT_FOLDER}/serial
echo -n "1000" > ${OUTPUT_FOLDER}/crlnumber

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


# client
mkdir -p ${OUTPUT_FOLDER}/client/private
mkdir -p ${OUTPUT_FOLDER}/client/csr
mkdir -p ${OUTPUT_FOLDER}/client/certs

openssl genrsa -out ${OUTPUT_FOLDER}/client/private/client.key.pem 4096
openssl req -new -set_serial 03 -key ${OUTPUT_FOLDER}/client/private/client.key.pem -out ${OUTPUT_FOLDER}/client/csr/client.csr \
  -subj "/C=BE/ST=BRUSSELS/L=Brussels/O=Fancy Marketplace Co. CA/CN=*.127.0.0.1.nip.io/serialNumber=03" \
  -config ./config/openssl-client.cnf
openssl x509 -req -in ${OUTPUT_FOLDER}/client/csr/client.csr -CA ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem \
  -CAkey ${OUTPUT_FOLDER}/intermediate/private/intermediate.cakey.pem -out ${OUTPUT_FOLDER}/client/certs/client.cert.pem \
  -CAcreateserial -days 1825 -sha256 -extfile ./config/openssl-client.cnf \
  -copy_extensions=copyall

openssl x509 -in ${OUTPUT_FOLDER}/client/certs/client.cert.pem -out ${OUTPUT_FOLDER}/client/certs/client.cert.pem -outform PEM

cat ${OUTPUT_FOLDER}/client/certs/client.cert.pem ${OUTPUT_FOLDER}/intermediate/certs/ca-chain-bundle.cert.pem > ${OUTPUT_FOLDER}/client/certs/client-chain-bundle.cert.pem


kubectl create secret tls local-wildcard --cert=${OUTPUT_FOLDER}/client/certs/client-chain-bundle.cert.pem --key=${OUTPUT_FOLDER}/client/private/client.key.pem --namespace infra -o yaml --dry-run > ${OUTPUT_FOLDER}/local-wildcard.yaml
kubectl create secret generic gx-registry-keypair --from-file=PRIVATE_KEY=${OUTPUT_FOLDER}/ca/private/cakey-pkcs8.pem --from-file=X509_CERTIFICATE=${OUTPUT_FOLDER}/ca/certs/cacert.pem --namespace infra -o yaml --dry-run > ${OUTPUT_FOLDER}/secret.yaml
