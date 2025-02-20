#!/bin/bash


#rm generated/*

kustomize build --load-restrictor LoadRestrictionsNone core/global > generated/core-global.yaml
kustomize build --load-restrictor LoadRestrictionsNone core/base > generated/core-base.yaml
