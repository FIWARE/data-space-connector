#! /bin/bash

CHARTS=$(pwd)/charts/*
RETURN_VAL=0
for chart in $CHARTS
do
 [ ! -d "${chart}" ] && continue
 ./bin/helm dependency build ${chart}
 ./bin/helm template ${chart} | kubeconform -strict \
   -schema-location default \
   -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

 ret=$?
 if [ $ret -ne 0 ]; then
     RETURN_VAL=$ret
 fi

 # Rendering with default values only covers what is enabled by default, which leaves whole
 # feature areas (identityhub, vault, fdsc-edc, vc-operator resources) never validated. Each
 # ci/*-values.yaml turns one of those on.
 for values in ${chart}/ci/*-values.yaml
 do
   [ ! -f "${values}" ] && continue
   echo "Evaluating ${chart} with $(basename ${values})"
   # -ignore-missing-schemas because these values render custom resources whose CRDs are not in any
   # public catalog - the vc-operator's CredentialIssuer and VerifiableCredentialRequest. Everything
   # with a published schema is still validated strictly.
   ./bin/helm template ${chart} -f ${values} | kubeconform -strict -ignore-missing-schemas \
     -schema-location default \
     -schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json'

   ret=$?
   if [ $ret -ne 0 ]; then
       RETURN_VAL=$ret
   fi
 done
done

if [ $RETURN_VAL -eq 0 ]; then
    echo "Chart evaluation successful !!!"
fi

exit $RETURN_VAL
