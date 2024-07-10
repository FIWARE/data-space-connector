#! /bin/bash

CHARTS=$(pwd)/charts/*
RETURN_VAL=0
for chart in $CHARTS
do
 ./bin/helm dependency build ${chart}
 ./bin/helm template ${chart} | kubeconform -strict

 ret=$?
 if [ $ret -ne 0 ]; then
     RETURN_VAL=$ret
 fi
done

if [ $RETURN_VAL -eq 0 ]; then
    echo "Chart evaluation successful !!!"
fi

exit $RETURN_VAL
