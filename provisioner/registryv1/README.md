# registryv1 Provisioner

## Summary

The registryv1 provisioner is a core rukpak provisioner that knows how to interact with `registry+v1` format bundles. 
The `registry+v1` bundles are the typical OLM bundles that contain a CSV manifest as well as other supported manifest types, such as CRDs and RBAC.
They contain a manifests and metadata directory and can be built via standard OLM tooling such as the `opm` tool.
For more information on the `registry+v1` bundle format see the [documentation](https://olm.operatorframework.io/docs/tasks/creating-operator-bundle/#operator-bundle). 

The registryv1 provisioner is able to unpack a given `registry+v1` bundle onto a cluster and then instantiate it, making the
content of the bundle available in the cluster. It does so by reconciling the `Bundle` and `BundleInstance` types that 
have the spec.provisionerClassName field set to `core.rukpak.io/registry+v1`. This field must be set to the correct provisioner name 
in order for the registryv1 provisioner to see and interact with the bundle. 

Since this provisioner is responsible for creating OLM bundles on-cluster, it has a soft dependency on 
OLM and the existing set of APIs, specifically ClusterServiceVersion and OperatorGroup. Those APIs must exist on the cluster
in order to instantiate a `registry+v1` bundle successfully. The provisioner creates the CSV provided in the bundle, but delegates
additional handling of the CSV to OLM itself, therefore OLM should be installed on the cluster alongside this provisioner. 

## Use cases

### Install and apply a specific version of a bundle 

The registryv1 provisioner can install and make available a specific `registry+v1` bundle available in the cluster. This is in contrast to 
the existing OLM workflow which requires a Subscription and manual approval for the relevant InstallPlan to pin to a specific version.  

Simply create a `Bundle` resource pointing to a specific version of your bundle, and a `BundleInstance` which references that bundle.
The registryv1 provisioner will unpack the provided Bundle onto the cluster, and eventually make the content available on the cluster. 

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata:
  name: my-bundle
spec:
  image: my-bundle-image@sha256:xyz123
  provisionerClassName: core.rukpak.io/registry+v1
---
apiVersion: core.rukpak.io/v1alpha1
kind: BundleInstance
metadata:
  name: my-bundle-instance
spec:
  provisionerClassName: core.rukpak.io/registry+v1
  bundleName: my-bundle
``` 
### Make bundle content available but do not install it

There is a natural separation between sourcing of the content and application of that content via two separate RukPak APIs, `Bundle` and `BundleInstance`. 
A user can specify a particular `Bundle` to be available in the cluster for inspection before any application of the resources. 
Given a `Bundle` resource named `my-bundle`, the registryv1 provisioner will pull down and unpack the bundle to a variety of storage backends, such as ConfigMaps or PersistentVolumes.

Currently the ConfigMap backend is supported by the provisioner, and the content can be inspected by viewing the metadata ConfigMap, which holds
information on all ConfigMaps related to this bundle. 

`kubectl get cm bundle-metadata-my-bundle -o yaml` provides access to this metadata ConfigMap, which then links to all other ConfigMap objects
where the unpacked data is stored. 

Surfacing the content of a bundle in a more user-friendly way, via a plugin or additional API, is on the RukPak roadmap. 

### Pivot between bundle versions 

The `BundleInstance` API is meant to indicate the version of the bundle that should be active within the cluster. Given two unpacked bundles in the cluster,
`my-bundle-v0.0.1` and `my-bundle-v0.0.2`, the `spec.bundleName` field of the related `BundleInstance` can be updated from say, the older version,
to the newer version, and the registryv1 provisioner will be able to successfully create the new content on-cluster and remove the old content.  

## Running locally 

To experiment with the registryv1 provisioner locally, first setup a local cluster. It's recommended to have OLM installed on the cluster as well, 
which can be done easily by heading to the [releases](https://github.com/operator-framework/operator-lifecycle-manager/releases) section and installing via the install script. 

Once the cluster is setup and OLM is running, take the following steps from the root of the repository:
* Create the rukpak namespace via `kubectl create ns rukpak-system` by default
* Run `make bin/registryv1` to create the registryv1 provisioner binary locally
* Apply the RukPak core CRDs via `kubectl apply -f manifests/`
* Run `./bin/registryv1` to start the controller 

From there, creating some `Bundles` and `BundleInstance` types to see the provisioner in action. For an example bundle to use, 
the [combo operator](https://github.com/operator-framework/combo) is a good candidate. 

Create the bundle:
```yaml
---
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata:
  name: combo-0.0.1
spec:
  image: quay.io/operator-framework/combo-bundle:v0.0.1
  provisionerClassName: core.rukpak.io/registry+v1
```

Before creating the combo `BundleInstance` on-cluster, ensure that there is a namespace available with an OperatorGroup that 
supports the combo operator. 

Create the namespace and OperatorGroup:
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: combo-system
---
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: combo-system-og
  namespace: combo-system
spec: {}
```

Create the combo `BundleInstance` referencing the combo `Bundle` available in the cluster. 

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: BundleInstance
metadata:
  name: combo-system
spec:
  provisionerClassName: core.rukpak.io/registry+v1
  bundleName: combo-0.0.1
```

From there, check out the combo operator and ensure that the CSV is present on the cluster. 
