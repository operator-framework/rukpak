# helm Provisioner

## Summary

The `helm` provisioner is able to instantiate a given `helm+v3` bundle with a specified helm chart onto a cluster and then install the helm chart,
in the cluster. It does so by reconciling `Bundle` and `BundleDeployment` types that have
the `spec.provisionerClassName` field set to `core-rukpak-io-helm`. This field must be set to the correct provisioner
name in order for the `helm` provisioner to see and interact with the bundle.

### Install and apply a specific version of a `helm+v3` bundle

> :warning: Anyone with the ability to create or update BundleDeployment objects can become a cluster admin. It's important
> to limit access to this API via RBAC to only those that explicitly require access, as well as audit your bundles to
> ensure the content being installed on-cluster is as-expected and secure.

The `helm` provisioner can install and make available a specific `helm+v3` bundle in the cluster.

Simply create a `BundleDeployment` resource that contains the desired specification of a Bundle resource.
The `helm` provisioner will retrieve the specific helm chart and instantiate the Bundle onto the cluster, and eventually install the helm chart
 on the cluster.

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-ahoy
spec:
  provisionerClassName: core-rukpak-io-helm
  config:
    values: |
      # Default values for hello-world.
      # This is a YAML-formatted file.
      # Declare variables to be passed into your templates.
      replicaCount: 1
      image:
        repository: nginx
        pullPolicy: IfNotPresent
        # Overrides the image tag whose default is the chart appVersion.
        tag: ""
      nameOverride: "formvalues"
      fullnameOverride: ""
      serviceAccount:
        # Specifies whether a service account should be created
        create: true
        # Annotations to add to the service account
        annotations: {}
        # The name of the service account to use.
        # If not set and create is true, a name is generated using the fullname template
        name: ""
      service:
        type: ClusterIP
        port: 80
  template:
    metadata:
      labels:
        app: my-ahoy
    spec:
      provisionerClassName: core-rukpak-io-helm
      source:
        http:
          url: https://github.com/helm/examples/releases/download/hello-world-0.1.0/hello-world-0.1.0.tgz  
        type: http
```

For the helm chart, the values file embedded in the `config` can be applied during the installation of the chart.

> Note: the generated Bundle will contain the BundleDeployment's metadata.Name as a prefix, followed by
> the hash of the provided template.

As the bundle content is retrieved and stored onto the cluster via the defined storage mechanism, the bundle status
will be updated to Unpacked, indicating that all its contents have been stored on-cluster.

```console
$ kubectl get bundle -l app=my-ahoy
NAME                 TYPE   PHASE      AGE
my-ahoy-5764594dc8   http   Unpacked   33s
```

Now that the bundle has been unpacked, the provisioner is able to create the resources in the bundle on the cluster.
These resources will be owned by the corresponding BundleDeployment. Creating the BundleDeployment on-cluster results in an
InstallationSucceeded Phase if the application of resources to the cluster was successful.

```console
$ kubectl get bundledeployment my-ahoy
NAME      ACTIVE BUNDLE        INSTALL STATE           AGE
my-ahoy   my-ahoy-5764594dc8   InstallationSucceeded   48s
```

> Note: Creation of more than one BundleDeployment from the same Bundle will likely result in an error.

## Running locally

### Setup

To experiment with the `helm` provisioner locally, take the following steps to
create a local [kind](https://kind.sigs.k8s.io/) cluster and deploy the provisioner onto it:

```bash
# Clone the repository
git clone https://github.com/operator-framework/rukpak

# Navigate to the repository
cd rukpak

# Start a local kind cluster then build and deploy the provisioner onto it
make run
```

### Installing the sample chart

From there, create some Bundles and BundleDeployment types to see the provisioner in action. For an example helm chart to
use, the [helm/examples](https://github.com/helm/examples) is a good example.

Create the sample BundleDeployment referencing the desired sample helm chart in Bundle configuration:

```bash
kubectl apply -f -<<EOF
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-ahoy
spec:
  provisionerClassName: core-rukpak-io-helm
  template:
    metadata:
      labels:
        app: my-ahoy
    spec:
      provisionerClassName: core-rukpak-io-helm
      source:
        http:
          url: https://github.com/helm/examples/releases/download/hello-world-0.1.0/hello-world-0.1.0.tgz
        type: http
EOF
```

A message saying that the BundleDeployment is created should be returned:

```console
$ kubectl apply -f -<<EOF
...
EOF
bundledeployment.core.rukpak.io/my-ahoy created
```

Next, check the Bundle status via:

```bash
kubectl get bundle -l app=my-ahoy
```

Eventually the Bundle should show up as Unpacked:

```console
$ kubectl get bundle -l app=my-ahoy
NAME                 TYPE   PHASE      AGE
my-ahoy-5764594dc8   http   Unpacked   33s
```

Check the BundleDeployment status to ensure that the installation was successful:

```bash
kubectl get bundledeployment my-ahoy
```

A successful installation will show InstallationSucceeded as the `INSTALL STATE`:

```console
$ kubectl get bundledeployment my-ahoy
NAME      ACTIVE BUNDLE        INSTALL STATE           AGE
my-ahoy   my-ahoy-5764594dc8   InstallationSucceeded   48s
```

From there, check out the my-ahoy deployment and ensure that the operator is present on the cluster:

```bash
# Check the my-ahoy deployment
kubectl get deployments.apps my-ahoy-hello-world

# Check that the operator is present
kubectl get deployments.apps my-ahoy-hello-world -o yaml | grep 'image:' | xargs
```

The deployment should show ready and available:

```console
$ kubectl get deployments.apps my-ahoy-hello-world
NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
my-ahoy-hello-world   1/1     1            1           102s

$ kubectl get deployments.apps my-ahoy-hello-world -o yaml | grep 'image:' | xargs
- image: nginx:1.16.0
```

This means the operator should be successfully installed.

The `helm` provisioner continually reconciles BundleDeployment resources. Next, let's try deleting the my-ahoy-hello-world deployment:

```bash
kubectl delete deployments.apps my-ahoy-hello-world
```

A message saying the deployment was deleted should be returned:

```console
$ kubectl delete deployments.apps my-ahoy-hello-world
deployment.apps "my-ahoy-hello-world" deleted
```

The provisioner ensures that all resources required for the BundleDeployment to run are accounted for on-cluster.
So if we check for the deployment again, it will be back on the cluster:

```console
$ kubectl get deployment my-ahoy-hello-world
NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
my-ahoy-hello-world   1/1     1            1           7s
```

The values file for the chart can be applied during install.  The values can be embeded in the `config` section of BundleDeployment.

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-ahoy
spec:
  provisionerClassName: core-rukpak-io-helm
  config:
    values: |
      # Default values for hello-world.
      # This is a YAML-formatted file.
      # Declare variables to be passed into your templates.
      replicaCount: 1
      image:
        repository: nginx
        pullPolicy: IfNotPresent
        # Overrides the image tag whose default is the chart appVersion.
        tag: ""
      nameOverride: "formvalues"
      fullnameOverride: ""
      serviceAccount:
        # Specifies whether a service account should be created
        create: true
        # Annotations to add to the service account
        annotations: {}
        # The name of the service account to use.
        # If not set and create is true, a name is generated using the fullname template
        name: ""
      service:
        type: ClusterIP
        port: 80
  template:
    metadata:
      labels:
        app: my-ahoy
    spec:
      provisionerClassName: core-rukpak-io-helm
      source:
        http:
          url: https://github.com/helm/examples/releases/download/hello-world-0.1.0/hello-world-0.1.0.tgz  
        type: http
```

### Deleting the sample chart and Local Kind Cluster

To clean up from the installation, simply remove the BundleDeployment from the cluster. This will remove all references
resources including the deployment, RBAC, and the operator namespace.

> Note: There's no need to manually clean up the Bundles that were generated from a BundleDeployment resource. The plain provisioner places owner references on any Bundle that's generated from an individual BundleDeployment resource.

```bash
# Delete the combo BundleDeployment
kubectl delete bundledeployments.core.rukpak.io my-ahoy
```

A message should show that the BundleDeployment was deleted and now the cluster state is the same as it was
prior to installing the operator.

```console
$ kubectl delete bundledeployments.core.rukpak.io my-ahoy
bundledeployment.core.rukpak.io "my-ahoy" deleted
```

To stop and clean up the kind cluster, delete it:

```bash
# Clean up kind cluster
kind delete clusters rukpak
```
