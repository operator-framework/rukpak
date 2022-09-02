# Local Bundles

## Summary

A Bundle can reference content from an in-cluster resource instead of a remote container image or a git repository by
using the`local` source type in the Bundle manifest. This enables one to easily source content locally without external
repositories/registries. Sourcing from a local configmap is currently supported, with support for additional content
sources like a PV is on the roadmap.

For example, for a plain+v0 bundle, the local configmap backing the bundle, or "bundle configmap", must have a certain
data structure in order to produce a valid Bundle that works with the plain provisioner. It should have a map of
manifest file contents with its filename as the key in the `data` of the configmap. The name and namespace of the
configmap must be specified in `spec.source.local.configmap.name` and `spec.source.local.configmap.namespace`
respectively.

The configmap can be created with the command:

```bash
kubectl create configmap <configmap name> --from-file=<manifests directory>
```

> Note: Once a configmap is referenced by a Bundle, the configmap will have an owner reference placed on it that points
> to the Bundle. As a result, when the Bundle is removed, the configmap will also be removed. This ensures that the cluster is
> in the same state before and after installing content.

### Example

1. Create the configmap

``` bash
kubectl create configmap my-configmap --from-file=../testdata/bundles/plain-v0/valid/manifests -n rukpak-system
```

2. Create a bundle

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata:
  name: combo-v0.1.0
spec:
  source:
    type: local
    local:
      configmap:
        name: my-configmay
        namespace: rukpak-system
  provisionerClassName: core-rukpak-io-plain
EOF
```
