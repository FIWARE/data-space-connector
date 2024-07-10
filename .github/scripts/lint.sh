#! /bin/bash

CHARTS=./charts/*
for chart in $CHARTS
do
 docker run --rm -v $(pwd):/apps alpine/helm:2.9.0 lint $chart
done
