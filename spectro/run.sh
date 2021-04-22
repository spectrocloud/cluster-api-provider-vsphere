#!/bin/bash

rm generated/*

./kustomize build base > ./generated/core-base.yaml
./kustomize build global > ./generated/core-global.yaml
