# Plain Provisioner

## Summary

The `plain` provisioner is one of core RukPak [provisioners](https://github.com/operator-framework/rukpak/tree/main/internal/provisioner)
that knows how to interact with bundles of a particular format.
These `plain+v0` bundles, or plain bundles, are simply container images containing a set of static Kubernetes YAML
manifests in a given directory. For more information on the `plain+v0` format, see
the [plain+v0 bundle spec](/docs/bundles/plain.md).

The `plain` provisioner is able to unpack a given `plain+v0` bundle onto a cluster and then instantiate it, making the
content of the bundle available in the cluster. It does so by reconciling `BundleDeployment` types that have
the `spec.provisionerClassName` field set to `core-rukpak-io-plain`. This field must be set to the correct provisioner
name in order for the `plain` provisioner to see and interact with the bundle.

Supported source types for a plain bundle currently include the following:

* A container image
* A directory in a git repository
* A [http](../sources/http.md)
* A [configmap](local-bundles.md)

Additional source types, such as a local volume are on the roadmap. These source types
all present the same content, a directory containing a plain bundle, in a different ways.

### Install and apply a specific version of a `plain+v0` bundle

> :warning: Anyone with the ability to create or update BundleDeployment objects can become cluster admin. It's important
> to limit access to this API via RBAC to only those that explicitly require access, as well as audit your bundles to
> ensure the content being installed on-cluster is as-expected and secure.

The `plain` provisioner can install and make available a specific `plain+v0` bundle in the cluster.

Simply create a `BundleDeployment` resource that contains the desired specification of a Bundle resource.
The `plain` provisioner will unpack the provided Bundle onto the cluster, and eventually make the content
available on the cluster.

```yaml
apiVersion: core.rukpak.io/v1alpha2
kind: BundleDeployment
metadata:
  name: my-bundle-deployment
spec:
  provisionerClassName: core-rukpak-io-plain
  source:
    type: image
    image:
      ref: my-bundle@sha256:xyz123
  provisionerClassName: core-rukpak-io-plain
```

As the bundle content is retrieved and stored onto the cluster via the defined storage mechanism, the bundledeployment's `Install State` moves from `UnpackSuccessful` to `InstallationSucceeded`.
The install succeeded state indicates that the provisioner has created the resources in the bundle on the cluster. These resources will be owned by the corresponding BundleDeployment.

```console
$ kubectl get bundledeployment my-bundle-deployment
NAME                       INSTALL STATE           AGE
my-bundle-deployment       InstallationSucceeded   11s
```

## Running locally

### Setup

To experiment with the `plain` provisioner locally, take the following steps to
create a local [kind](https://kind.sigs.k8s.io/) cluster and deploy the provisioner onto it:

```bash
# Clone the repository
git clone https://github.com/operator-framework/rukpak

# Navigate to the repository
cd rukpak

# Start a local kind cluster then build and deploy the provisioner onto it
make run
```

### Installing the Combo Operator

From there, create some BundleDeployment types to see the provisioner in action. For an example bundle to
use, the [combo operator](https://github.com/operator-framework/combo) is a good candidate.

Create the combo BundleDeployment referencing the desired combo Bundle configuration:

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha2
kind: BundleDeployment
metadata:
  name: combo
spec:
  provisionerClassName: core-rukpak-io-plain
  source:
    image:
      ref: quay.io/operator-framework/combo-bundle:v0.0.1
    type: image
EOF
```

A message saying that the BundleDeployment is created should be returned:

```console
$ kubectl apply -f -<<EOF
...
EOF
bundledeployment.core.rukpak.io/combo created
```

Check the BundleDeployment status to ensure that the installation was successful:

```bash
kubectl get bundledeployment combo
```

A successful installation will show InstallationSucceeded as the `INSTALL STATE`:

```console
$ kubectl get bundledeployment combo
NAME      INSTALL STATE           AGE
combo     InstallationSucceeded   10s
```

From there, check out the combo operator deployment and ensure that the operator is present on the cluster:

```bash
# Check the combo operator deployment
kubectl -n combo get deployments.apps combo-operator

# Check that the operator is present
kubectl -n combo get deployments.apps combo-operator -o yaml | grep 'image:' | xargs
```

The deployment should show that the operator is ready and available:

```console
$ kubectl -n combo get deployments.apps combo-operator
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
combo-operator   1/1     1            1           10s

$ kubectl -n combo get deployments.apps combo-operator -o yaml | grep 'image:' | xargs
image: quay.io/operator-framework/combo-operator:v0.0.1
```

This means the operator should be successfully installed.

The `plain` provisioner continually reconciles BundleDeployment resources. Next, let's try deleting the combo deployment:

```bash
kubectl -n combo delete deployments.apps combo-operator
```

A message saying the deployment was deleted should be returned:

```console
$ kubectl -n combo delete deployments.apps combo-operator
deployment.apps "combo-operator" deleted
```

The provisioner ensures that all resources required for the BundleDeployment to run are accounted for on-cluster.
So if we check for the deployment again, it will be back on the cluster:

```console
$ kubectl -n combo get deployments.apps combo-operator
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
combo-operator   1/1     1            1           15s
```

### Upgrading the Combo Operator

Let's say the combo operator released a new patch version, and we want to upgrade to that version.

Update the existing `combo` BundleDeployment resource and update the container image being referenced:

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha2
kind: BundleDeployment
metadata:
  name: combo
spec:
  provisionerClassName: core-rukpak-io-plain
  source:
    image:
      ref: quay.io/operator-framework/combo-bundle:v0.0.2
    type: image
EOF
```

```console
$ kubectl get bundledeployment combo
NAME       INSTALL STATE           AGE
combo      InstallationSucceeded   10s

$ kubectl -n combo get deployment
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
combo-operator   1/1     1            1           10s

$ kubectl -n combo get deployments.apps combo-operator -o yaml | grep 'image:' | xargs
image: quay.io/operator-framework/combo-operator:v0.0.2
```

Notice that the container image has changed to `v0.0.2` since we first installed the combo operator.

### Deleting the Combo Operator and Local Kind Cluster

To clean up from the installation, simply remove the BundleDeployment from the cluster. This will remove all references
resources including the deployment, RBAC, and the operator namespace.

> Note: There's no need to manually clean up the Bundles that were generated from a BundleDeployment resource. The plain provisioner places owner references on any Bundle that's generated from an individual BundleDeployment resource.

```bash
# Delete the combo BundleDeployment
kubectl delete bundledeployments.core.rukpak.io combo
```

A message should show that the BundleDeployment was deleted and now the cluster state is the same as it was
prior to installing the operator.

```console
$ kubectl delete bundledeployments.core.rukpak.io combo
bundledeployment.core.rukpak.io "combo" deleted
```

To stop and clean up the kind cluster, delete it:

```bash
# Clean up kind cluster
make kind-cluster-cleanup
```
