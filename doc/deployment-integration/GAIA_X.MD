```shell
  docker run -v $(pwd):/cert quay.io/wi_stefan/did-helper:0.1.1
  export USER_CREDENTIAL=$(./doc/scripts/get_credential_for_consumer.sh http://keycloak-consumer.127.0.0.1.nip.io:8080 user-credential); echo ${USER_CREDENTIAL}
  export ACCESS_TOKEN=$(./doc/scripts/get_access_token_oid4vp.sh http://mp-data-service.127.0.0.1.nip.io:8080 $USER_CREDENTIAL default); echo ${ACCESS_TOKEN}
```


```shell
  curl -k -x http://localhost:8888 -X POST 'https://registry.127.0.0.1.nip.io/v2/api/trustAnchor/chain/file' \
  --header 'Accept: */*' \
  --header 'Content-Type: application/json' \
  --data-raw '{ "uri": "https://consumer.127.0.0.1.nip.io/.well-known/tls.crt"}'
```

```shell
  curl -k -x http://localhost:8888 -v https://consumer.127.0.0.1.nip.io/.well-known/did.json
```