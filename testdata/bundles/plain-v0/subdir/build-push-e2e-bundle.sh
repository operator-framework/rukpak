#! /bin/bash

set -o errexit
set -o nounset
set -o pipefail

help="
build-push-e2e-bundle.sh is a script to build and push the e2e bundle image using kaniko.
Usage:
  build-push-e2e-bundle.sh [NAMESPACE] [TAG] [BUNDLE_DIR] [BUNDLE_NAME]

Argument Descriptions:
  - NAMESPACE is the namespace the kaniko Job should be created in
  - TAG is the full tag used to build and push the catalog image
"

if [[ "$#" -ne 2 ]]; then
  echo "Illegal number of arguments passed"
  echo "${help}"
  exit 1
fi

namespace=$1
tag=$2
bundle_dir="testdata/bundles/plain-v0/subdir"
bundle_name="plain-subdir"

echo "${namespace}" "${tag}"

kubectl create configmap -n "${namespace}" --from-file="${bundle_dir}/" rukpak-e2e-${bundle_name}.root
kubectl create configmap -n "${namespace}" --from-file="${bundle_dir}/manifests" rukpak-e2e-${bundle_name}.manifests
kubectl create configmap -n "${namespace}" --from-file="${bundle_dir}/manifests/emptydir" rukpak-e2e-${bundle_name}.manifests.emptydir
kubectl create configmap -n "${namespace}" --from-file="${bundle_dir}/manifests/subdir" rukpak-e2e-${bundle_name}.manifests.subdir

kubectl apply -f - << EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: "kaniko-${bundle_name}"
  namespace: "${namespace}"
spec:
  template:
    spec:
      initContainers:
        - name: copy-manifests
          image: busybox
          command: ['sh', '-c', 'cp -r /manifests-data/* /manifests']
          volumeMounts:
            - name: manifests
              mountPath: /manifests
            - name: manifests-data
              mountPath: /manifests-data
            - name: emptydir-data
              mountPath: /manifests-data/emptydir
            - name: subdir-data
              mountPath: /manifests-data/subdir
      containers:
      - name: kaniko
        image: gcr.io/kaniko-project/executor:latest
        args: ["--dockerfile=/workspace/Dockerfile",
                "--context=/workspace/",
                "--destination=${tag}",
                "--skip-tls-verify"]
        volumeMounts:
          - name: dockerfile
            mountPath: /workspace/
          - name: manifests
            mountPath: /workspace/manifests/
      restartPolicy: Never
      volumes:
        - name: dockerfile
          configMap:
            name: rukpak-e2e-${bundle_name}.root
        - name: manifests
          emptyDir: {}
        - name: manifests-data
          configMap:
            name: rukpak-e2e-${bundle_name}.manifests
        - name: emptydir-data
          configMap:
            name: rukpak-e2e-${bundle_name}.manifests.emptydir
        - name: subdir-data
          configMap:
            name: rukpak-e2e-${bundle_name}.manifests.subdir
EOF

kubectl wait --for=condition=Complete -n "${namespace}" jobs/kaniko-${bundle_name} --timeout=60s
