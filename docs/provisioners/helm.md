# helm Provisioner

## Summary

The `helm` provisioner is one of the [provisioners](https://github.com/operator-framework/rukpak/tree/main/internal/provisioner) of RukPak.
It is able to instantiate a given `helm+v3` bundle with a specified helm chart onto a cluster where it then installs the content. It does so by reconciling `Bundle` and `BundleDeployment` types that have
the `spec.provisionerClassName` field set to `core-rukpak-io-helm`. This field must be set to the correct provisioner
name in order for the `helm` provisioner to see and interact with the bundle.

Supported source types for a helm bundle currently include the following:

* A container image
* A directory in a git repository
* A [http](../sources/http.md)
* An [upload](../uploading-bundles.md)

Additional source types, such as a local volume are on the roadmap. These source types
all present the same content, a directory containing a helm chart, in a different ways.

### Install and apply a specific version of a `helm+v3` bundle

> :warning: Anyone with the ability to create or update BundleDeployment objects can become a cluster admin. It's important
> to limit access to this API via RBAC to only those that explicitly require access, as well as audit your bundles to
> ensure the content being installed on-cluster is as-expected and secure.

The `helm` provisioner can install and make available a specific `helm+v3` bundle in the cluster. To do this, the `helm` provisioner will retrieve the configured helm chart, instantiate a Bundle onto the cluster, and eventually install the desired helm chart on the cluster.

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

## Quick Start

### Setup

#### Prepare sample github repository

Fork `https://github.com/helm/examples` github repository into any origanization that you have write access.

Set a YOUR_ORG environment variable for convenience.
```bash
export YOUR_ORG="your goranization name"
```

Edit `https://github.com/$YOUR_ORG/examples/blob/main/charts/hello-world/Chart.yaml` and update `version: 0.1.0` to `version: 0.1.1` and
commit the change into `v0.1.1` branch.

#### Install RukPak for expertiment

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
        git:
          ref:
            branch: main
          repository: https://github.com/akihikokuroda/examples
          directory: ./charts
        type: git
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
my-ahoy-5764594dc8   git    Unpacked   33s
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

From there, ensure that the helm chart is installed and the operator is present on the cluster:

```bash
# Check the installed helm chart
helm list

# Check the my-ahoy deployment
kubectl get deployments.apps my-ahoy-hello-world

# Check that the operator is present
kubectl get deployments.apps my-ahoy-hello-world -o yaml | grep 'image:' | xargs
```

The deployment should show ready and available:

```console
$ helm list
NAME    NAMESPACE       REVISION        UPDATED                                 STATUS          CHART                   APP VERSION
my-ahoy rukpak-system   1               2022-08-27 22:08:23.310271084 +0000 UTC deployed        hello-world-0.1.0       1.16.0

$ kubectl get deployments.apps my-ahoy-hello-world
NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
my-ahoy-hello-world   1/1     1            1           102s

$ kubectl get deployments.apps my-ahoy-hello-world -o yaml | grep 'image:' | xargs
- image: nginx:1.16.0
```

This means the operator should be successfully installed.

Delete BundleDeployment for the next step
```bash
kubectl delete bundledeployment my-ahoy
```

```console
bundledeployment.core.rukpak.io "my-ahoy" deleted
```

### Add a values file

Add a values file into the BundleDeployment to override some default values:

```bash
kubectl apply -f -<<EOF
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
      nameOverride: "fromvalues"
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
        git:
          ref:
            branch: main
          repository: https://github.com/akihikokuroda/examples
          directory: ./charts
        type: git
EOF
```

This values file is updating the `nameOverride`

```console
bundledeployment.core.rukpak.io/my-ahoy created
```

Check the name of the deployment:

```bash
kubectl get deployment
```

```console
NAME                 READY   UP-TO-DATE   AVAILABLE   AGE
core                 1/1     1            1           20h
helm-provisioner     1/1     1            1           20h
my-ahoy-fromvalues   1/1     1            1           36s
rukpak-webhooks      1/1     1            1           20h
```

The name of the deployment is `my-ahoy-fromvalues` now

### Update the values file

Change this a value in the values file.  Update the `nameOverride` to `name1`

```bash
kubectl apply -f -<<EOF
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
      nameOverride: "name1"
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
        git:
          ref:
            branch: main
          repository: https://github.com/akihikokuroda/examples
          directory: ./charts
        type: git
EOF
```

```console
bundledeployment.core.rukpak.io/my-ahoy configured
```

Check the name of the deployment

```bash
kubectl get deployment
```

```console
NAME               READY   UP-TO-DATE   AVAILABLE   AGE
core               1/1     1            1           20h
helm-provisioner   1/1     1            1           20h
my-ahoy-name1      1/1     1            1           56s
rukpak-webhooks    1/1     1            1           20h
```

Now the name of the deployment is `my-ahoy-name1`

### Upgrade the helm chart

Check the helm chart version install now

```bash
# Check the installed helm chart
helm list
```

```console
NAME    NAMESPACE       REVISION        UPDATED                                 STATUS          CHART                   APP VERSION
my-ahoy rukpak-system   2               2022-08-27 22:14:50.56889877 +0000 UTC  deployed        hello-world-0.1.0       1.16.0
```

Now the helm chart version is `hello-world-0.1.0`.  Change the git branch to `v0.1.1` from `main`

```bash
kubectl apply -f -<<EOF
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
      nameOverride: "name1"
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
        git:
          ref:
            branch: v0.1.1
          repository: https://github.com/akihikokuroda/examples
          directory: ./charts
        type: git
EOF
```

```console
bundledeployment.core.rukpak.io/my-ahoy configured
```

Check the helm chart version, again

```bash
# Check the installed helm chart
helm list
```

```console
NAME    NAMESPACE       REVISION        UPDATED                                 STATUS          CHART                   APP VERSION
my-ahoy rukpak-system   3               2022-08-27 22:12:51.056782394 +0000 UTC deployed        hello-world-0.1.1       1.16.0
```

Now the helm chart version is `hello-world-0.1.1`.

### Reconcile deployed resources

The `helm` provisioner continually reconciles BundleDeployment resources. Next, let's try deleting the my-ahoy-name1 deployment:

```bash
kubectl delete deployments.apps my-ahoy-name1
```

A message saying the deployment was deleted should be returned:

```console
deployment.apps "my-ahoy-name1" deleted
```

The provisioner ensures that all resources required for the BundleDeployment to run are accounted for on-cluster.
So if we check for the deployment again, it will be back on the cluster:

```console
NAME                  READY   UP-TO-DATE   AVAILABLE   AGE
my-ahoy-name1         1/1     1            1           7s
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
