#!/usr/bin/env bash

export GIT_NAME="local-git"
export GIT_NAMESPACE=rukpak-e2e
export DNS_NAME=$GIT_NAME.$GIT_NAMESPACE.svc.cluster.local
export KIND_CLUSTER_NAME=$1

kubectl create ns $GIT_NAMESPACE || true

mkdir -p tools/git/tmp
kubectl delete secret ssh-secret -n $GIT_NAMESPACE
kubectl delete secret gitsecret -n rukpak-system
ssh-keygen -t rsa -b 4096 -C "akuroda@us.ibm.com" -P "" -f tools/git/tmp/id_rsa
kubectl create secret generic ssh-secret --from-file=tools/git/tmp/id_rsa --from-file=tools/git/tmp/id_rsa.pub -n $GIT_NAMESPACE
kubectl create secret generic gitsecret --type "kubernetes.io/ssh-auth" --from-file=ssh-privatekey=tools/git/tmp/id_rsa --from-file=ssh-knownhosts=tools/git/ssh_knownhosts.txt -n rukpak-system
rm -rf tools/git/tmp

#
git clone https://github.com/operator-framework/combo.git tools/git/testdata/combo

# create docker image
docker build -t git:latest tools/git/
kind load docker-image git:latest --name $KIND_CLUSTER_NAME

# create image registry service
kubectl apply -f tools/git/service.yaml -n $GIT_NAMESPACE
# create image registry pod
kubectl apply -f tools/git/git.yaml -n $GIT_NAMESPACE

# clean up 
rm -rf tools/git/tmp/certs
kubectl wait --for=condition=ContainersReady --namespace=rukpak-e2e pod/local-git-pod --timeout=60s
rm -rf tools/git/testdata/combo
