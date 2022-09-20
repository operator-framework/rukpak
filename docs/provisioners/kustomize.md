# kustomize Provisioner

## Summary

The `kustomize` provisioner is one of the [provisioners](https://github.com/operator-framework/rukpak/tree/main/internal/provisioner) of RukPak.
It instantiates a given `kustomize+v0` bundle onto a cluster and then install the generated resources
on the cluster.  The `kustomize+v0` bundle is a [Kustomization](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/).
It does so by reconciling `Bundle` and `BundleDeployment` types that have
the `spec.provisionerClassName` field set to `core-rukpak-io-kustomize`. This field must be set to the correct provisioner
name in order for the `kustomize` provisioner to see and interact with the bundle.

Supported source types for a kustomize bundle currently include the following:

* A container image
* A directory in a git repository
* An [http](../sources/http.md) url
* An [upload](../uploading-bundles.md) artifact

Additional source types, such as a local volume are on the roadmap. These source types
all present the same content, a directory containing a kustomize, in a different ways.

### Install and apply a specific version of a `kustomize+v0` bundle

> :warning: Anyone with the ability to create or update BundleDeployment objects can become a cluster admin. It's important
> to limit access to this API via RBAC to only those that explicitly require access, as well as audit your bundles to
> ensure the content being installed on-cluster is as-expected and secure.

The `kustomize` provisioner can install and make available a specific `kustomize+v0` bundle in the cluster.

Simply create a `BundleDeployment` resource that contains the desired specification of a Bundle resource.
The `kustomize` provisioner will retrieve the referred kustomization files and instantiate the Bundle onto the cluster, and eventually install the generated resources
 on the cluster.

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-kustomize
spec:
  provisionerClassName: core-rukpak-io-kustomize
  config:
    path: dev
  template:
    metadata:
      labels:
        app: my-kustomize
    spec:
      provisionerClassName: core-rukpak-io-kustomize
      source:
        git:
          ref:
            branch: main
          repository: https://github.com/akihikokuroda/kustomize
          directory: ./manifests
        type: git
```

For the target purpose, the path to the target purpose directory is specified in the `config`.  It is used during the kustomization of the manifest files.

> Note: the generated Bundle will contain the BundleDeployment's metadata.Name as a prefix, followed by
> the hash of the provided template.

As the bundle content is retrieved and stored onto the cluster via the defined storage mechanism, the bundle status
will be updated to Unpacked, indicating that all its contents have been stored on-cluster.

```console
$ kubectl get bundle -l app=my-kustomize
NAME                  TYPE   PHASE      AGE
my-kustomize-fd5xcx   git    Unpacked   17s
```

Now that the bundle has been unpacked, the provisioner is able to create the resources in the bundle on the cluster.
These resources will be owned by the corresponding BundleDeployment. Creating the BundleDeployment on-cluster results in an
InstallationSucceeded Phase if the application of resources to the cluster was successful.

```console
$ kubectl get bundledeployment my-kustomize
NAME           ACTIVE BUNDLE         INSTALL STATE           AGE
my-kustomize   my-kustomize-fd5xcx   InstallationSucceeded   68s
```

> Note: Creation of more than one BundleDeployment from the same Bundle will likely result in an error.

## Quick Start

### Setup

#### Install RukPak for expertiment

To experiment with the `kustomize` provisioner locally, take the following steps to
create a local [kind](https://kind.sigs.k8s.io/) cluster and deploy the provisioner onto it:

```bash
# Clone the repository
git clone https://github.com/operator-framework/rukpak

# Navigate to the repository
cd rukpak

# Start a local kind cluster then build and deploy the provisioner onto it
make run
```

### Installing the sample

From there, create some Bundles and BundleDeployment types to see the provisioner in action. For an example kustomize to
use, the [akihikokuroda/kustomize](https://github.com/akihikokuroda/kustomize) is a good example.

Create the sample BundleDeployment referencing the desired sample kustomize in Bundle configuration:

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-kustomize
spec:
  provisionerClassName: core-rukpak-io-kustomize
  config:
    path: dev
  template:
    metadata:
      labels:
        app: my-kustomize
    spec:
      provisionerClassName: core-rukpak-io-kustomize
      source:
        git:
          ref:
            branch: main
          repository: https://github.com/akihikokuroda/kustomize
          directory: ./manifests
        type: git
EOF
```

A message saying that the BundleDeployment is created should be returned:

```console
$ kubectl apply -f -<<EOF
...
EOF
bundledeployment.core.rukpak.io/my-kustomize created
```

Next, check the Bundle status via:

```bash
kubectl get bundle -l app=my-kustomize
```

Eventually the Bundle should show up as Unpacked:

```console
$ kubectl get bundle -l app=my-kustomize
NAME                  TYPE   PHASE      AGE
my-kustomize-fd5xcx   git    Unpacked   44s
```

Check the BundleDeployment status to ensure that the installation was successful:

```bash
kubectl get bundledeployment my-kustomize
```

A successful installation will show InstallationSucceeded as the `INSTALL STATE`:

```console
$ kubectl get bundledeployment my-kustomize
NAME           ACTIVE BUNDLE         INSTALL STATE           AGE
my-kustomize   my-kustomize-fd5xcx   InstallationSucceeded   3m15s
```

From there, ensure that the generated resources are installed and the operator is present on the cluster:

```bash
# Check the dev-myapp-pod pod
kubectl get pod dev-myapp-pod
```

The pod should show ready and available:

```console
$ kubectl get pod dev-myapp-pod
NAME            READY   STATUS    RESTARTS   AGE
dev-myapp-pod   1/1     Running   0          6m2s
```

This means the operator should be successfully installed.

Delete BundleDeployment for the next step
```bash
kubectl delete bundledeployment my-kustomize
```

```console
bundledeployment.core.rukpak.io "my-kustomize" deleted
```

### Update the target purpose path

Change this a value in the values file.  Update the `dev` to `staging`

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-kustomize
spec:
  provisionerClassName: core-rukpak-io-kustomize
  config:
    path: staging
  template:
    metadata:
      labels:
        app: my-kustomize
    spec:
      provisionerClassName: core-rukpak-io-kustomize
      source:
        git:
          ref:
            branch: main
          repository: https://github.com/akihikokuroda/kustomize
          directory: ./manifests
        type: git
EOF
```

```console
bundledeployment.core.rukpak.io/my-kustomize created
```

Check the name of the pod

```bash
kubectl get pod
```

```console
NAME                                    READY   STATUS    RESTARTS   AGE
core-5659bb87b6-6vsnn                   2/2     Running   0          5h46m
helm-provisioner-ff7c54d6-29rwh         2/2     Running   0          5h46m
kustomize-provisioner-f56d7695b-l26nj   2/2     Running   0          4h3m
rukpak-webhooks-59c548fcd6-6phc5        1/1     Running   0          5h46m
stag-myapp-pod                          1/1     Running   0          62s
```

Now the name of the pod is `stag-myapp-pod`

### Upgrade the kustomize resorces

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-kustomize
spec:
  provisionerClassName: core-rukpak-io-kustomize
  config:
    path: staging
  template:
    metadata:
      labels:
        app: my-kustomize
    spec:
      provisionerClassName: core-rukpak-io-kustomize
      source:
        git:
          ref:
            branch: v1
          repository: https://github.com/akihikokuroda/kustomize
          directory: ./manifests
        type: git
EOF
```

```console
bundledeployment.core.rukpak.io/my-kustomize configured
```

Check the name of the pod

```bash
kubectl get pod
```

```console
$ kubectl get pod
NAME                                    READY   STATUS    RESTARTS   AGE
core-5659bb87b6-6vsnn                   2/2     Running   0          5h58m
helm-provisioner-ff7c54d6-29rwh         2/2     Running   0          5h58m
kustomize-provisioner-f56d7695b-l26nj   2/2     Running   0          4h15m
rukpak-webhooks-59c548fcd6-6phc5        1/1     Running   0          5h58m
stag-myapp-pod-v1                       1/1     Running   0          48s
```

Now the name of the pod is `stag-myapp-pod-v1`

### Reconcile deployed resources

The `kustomize` provisioner continually reconciles BundleDeployment resources. Next, let's try deleting the stag-myapp-pod-v1 pod:

```bash
kubectl delete pod stag-myapp-pod-v1
```

A message saying the deployment was deleted should be returned:

```console
$ kubectl delete pod stag-myapp-pod-v1
pod "stag-myapp-pod-v1" deleted
```

The provisioner ensures that all resources required for the BundleDeployment to run are accounted for on-cluster.
So if we check for the pod again, it will be back on the cluster:


```bash
kubectl get pod stag-myapp-pod-v1
```

```console
$ kubectl get pod   stag-myapp-pod-v1
NAME                READY   STATUS    RESTARTS   AGE
stag-myapp-pod-v1   1/1     Running   0          64s
```

### Deleting the sample chart and Local Kind Cluster

To clean up from the installation, simply remove the BundleDeployment from the cluster. This will remove all references
resources including the deployment, RBAC, and the operator namespace.

> Note: There's no need to manually clean up the Bundles that were generated from a BundleDeployment resource. The plain provisioner places owner references on any Bundle that's generated from an individual BundleDeployment resource.

```bash
# Delete the combo BundleDeployment
kubectl delete bundledeployments.core.rukpak.io my-kustomize
```

A message should show that the BundleDeployment was deleted and now the cluster state is the same as it was
prior to installing the operator.

```console
$ kubectl delete bundledeployments.core.rukpak.io my-kustomize
bundledeployment.core.rukpak.io "my-kustomize" deleted
```

To stop and clean up the kind cluster, delete it:

```bash
# Clean up kind cluster
kind delete clusters rukpak
```
