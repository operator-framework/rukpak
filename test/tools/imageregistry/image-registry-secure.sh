#! /bin/bash

set -o errexit
set -o nounset
set -o pipefail

help="
image-registry-secure.sh is a script to stand up an image registry within a cluster with authentication.
Usage:
  image-registry-secure.sh [NAMESPACE] [NAME]

Argument Descriptions:
  - NAMESPACE is the namespace that should be created and is the namespace in which the image registry will be created
  - NAME is the name that should be used for the image registry Deployment and Service
"

if [[ "$#" -ne 2 ]]; then
  echo "Illegal number of arguments passed"
  echo "${help}"
  exit 1
fi

namespace=$1
name=$2

kubectl apply -f - << EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${namespace}
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: selfsigned-issuer
  namespace: ${namespace}
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: ${namespace}-registry-secure
  namespace: ${namespace}
spec:
  secretName: ${namespace}-registry-secure
  isCA: true
  dnsNames:
    - ${name}-secure.${namespace}.svc
  privateKey:
    algorithm: ECDSA
    size: 256
  issuerRef:
    name: selfsigned-issuer
    kind: Issuer
    group: cert-manager.io
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${name}-secure
  namespace: ${namespace}
  labels:
    app: registry-secure
spec:
  replicas: 1
  selector:
    matchLabels:
      app: registry-secure
  template:
    metadata:
      labels:
        app: registry-secure
    spec:
      initContainers:
      - name: auth
        image: registry:2.6.2
        command:
        - "sh"
        - "-c"
        - "htpasswd -Bbn myuser mypasswd >> /auth/htpasswd"
        terminationMessagePolicy: FallbackToLogsOnError
        volumeMounts:
        - name: auth-vol
          mountPath: "/auth"
      containers:
      - name: registry
        image: registry:2
        volumeMounts:
        - name: certs-vol
          mountPath: "/certs"
        - name: auth-vol
          mountPath: "/auth"
          readOnly: true
        env:
        - name: REGISTRY_HTTP_TLS_CERTIFICATE
          value: "/certs/tls.crt"
        - name: REGISTRY_HTTP_TLS_KEY
          value: "/certs/tls.key"
        - name: REGISTRY_AUTH
          value: "htpasswd"
        - name: REGISTRY_AUTH_HTPASSWD_REALM
          value: "Registry Realm"
        - name: REGISTRY_AUTH_HTPASSWD_PATH
          value: "/auth/htpasswd"
      volumes:
        - name: certs-vol
          secret:
            secretName: ${namespace}-registry-secure
        - name: auth-vol
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: ${name}-secure
  namespace: ${namespace}
spec:
  selector:
    app: registry-secure
  ports:
  - port: 5000
    targetPort: 5000
EOF

kubectl create secret docker-registry "secureregistrysecret" --docker-server=${name}-secure.${namespace}.svc.cluster.local:5000 --docker-username="myuser" --docker-password="mypasswd" --docker-email="email@foo.com" -n rukpak-system
kubectl create secret docker-registry "secureregistrysecret" --docker-server=${name}-secure.${namespace}.svc.cluster.local:5000 --docker-username="myuser" --docker-password="mypasswd" --docker-email="email@foo.com" -n rukpak-e2e
kubectl wait --for=condition=Available -n "${namespace}" "deploy/${name}-secure" --timeout=60s
