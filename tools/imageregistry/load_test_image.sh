#!/usr/bin/env bash

export REGISTRY_NAME="docker-registry"
export REGISTRY_NAMESPACE=rukpak-e2e
export DNS_NAME=$REGISTRY_NAME.$REGISTRY_NAMESPACE.svc.cluster.local
KIND=$1
KIND_CLUSTER_NAME=$2

# push test bundle image into in-cluster docker registry
kubectl exec nerdctl -n $REGISTRY_NAMESPACE -- sh -c "nerdctl login -u myuser -p mypasswd $DNS_NAME:5000 --insecure-registry"

for x in $(docker images --format "{{.Repository}}:{{.Tag}}" | grep $DNS_NAME); do
echo $x
    $KIND load docker-image $x --name $KIND_CLUSTER_NAME
    kubectl exec nerdctl -n $REGISTRY_NAMESPACE -- sh -c "nerdctl -n k8s.io push $x --insecure-registry"
    kubectl exec nerdctl -n $REGISTRY_NAMESPACE -- sh -c "nerdctl -n k8s.io rmi $x --insecure-registry"
done

