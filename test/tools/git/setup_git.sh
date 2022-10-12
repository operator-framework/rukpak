#!/usr/bin/env bash

export GIT_NAME="local-git"
export GIT_NAMESPACE=rukpak-e2e
export DNS_NAME=$GIT_NAME.$GIT_NAMESPACE.svc.cluster.local
export KIND_CLUSTER_NAME=$1

kubectl create ns $GIT_NAMESPACE || true

mkdir -p test/tools/git/tmp
kubectl delete secret ssh-secret -n $GIT_NAMESPACE
kubectl delete secret gitsecret -n rukpak-system
ssh-keygen -t rsa -b 4096 -C "akuroda@us.ibm.com" -P "" -f test/tools/git/tmp/id_rsa
kubectl create secret generic ssh-secret --from-file=test/tools/git/tmp/id_rsa --from-file=test/tools/git/tmp/id_rsa.pub -n $GIT_NAMESPACE
kubectl create secret generic gitsecret --type "kubernetes.io/ssh-auth" --from-file=ssh-privatekey=test/tools/git/tmp/id_rsa --from-file=ssh-knownhosts=test/tools/git/ssh_knownhosts.txt -n rukpak-system
rm -rf test/tools/git/tmp

#
git clone https://github.com/operator-framework/combo.git test/tools/git/testdata/combo

# create docker image
docker build -t git:latest test/tools/git/
kind load docker-image git:latest --name $KIND_CLUSTER_NAME

# create image registry service
kubectl apply -f test/tools/git/service.yaml -n $GIT_NAMESPACE
# create image registry pod
kubectl apply -f test/tools/git/git.yaml -n $GIT_NAMESPACE

# clean up 
rm -rf test/tools/git/tmp/certs
kubectl wait --for=condition=ContainersReady --namespace=rukpak-e2e pod/local-git-pod --timeout=60s
rm -rf test/tools/git/testdata/combo
