# Image source

## Summary

The image source provides the contents in a container image as the source of the bundle.  The `source.type` for the image source is `image`.
When creating a image source, a reference to an URL of the image is specified. It is expected that a proper format of bundle contents is present
in the image.

## Example

For example, below is a minimal example of a Dockerfile that builds a bundle for image source from a directory
containing bundle contents.

```dockerfile
FROM scratch
COPY "Bundle contents directory" "File location for the bundle format" 
```

Build the image using a container tool like docker or podman. Use an image tag that references a repository that you
have push access to. For example,

```bash
docker build -f Dockerfile.example -t quay.io/operator-framework/rukpak:example .
```

Push the image to the remote registry

```bash
docker push quay.io/operator-framework/rukpak:example
```

The bundle source is referred in the following Bundle Deployment example.  
```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-bundle
spec:
  provisionerClassName: core-rukpak-io-plain
  template:
    metadata:
      labels:
        app: my-bundle
    spec:
      source:
        type: image
        image:
          ref: quay.io/operator-framework/rukpak:example
      provisionerClassName: core-rukpak-io-plain
```

## Private image registries

A Bundle can reference content in a private image registry by creating an `pullSecret` in the namespace that the provisioner is deployed.

### Methods

Create the secret for quay.io registry

```bash
kubectl create secret docker-registry mysecret --docker-server=quay.io --docker-username="your user name" --docker-password="your password" --docker-email="your e-mail adress" -n rukpak-system
```

Use the secret to pull the private image

#### Method 1:  Create a Bundle referencing a private image registry

```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: BundleDeployment
metadata:
  name: my-bundle
spec:
  provisionerClassName: core-rukpak-io-plain
  template:
    metadata:
      labels:
        app: my-bundle
    spec:
      source:
        type: image
        image:
          ref: quay.io/my-registry/rukpak:example
          pullSecret: mysecret
      provisionerClassName: core-rukpak-io-plain
```

#### Method 2: Add the secret to the `imagePullSecrets` in the `default` service account in the provisioner deployed namespace

```bash
kubectl patch serviceaccount default -p '{"imagePullSecrets": [{"name": "mysecret"}]}' -n rukpak-system
```
* This command replaces the secrets already in the `imagePullSecrets`.  To add the secret to the existing secrets, add the secret in the imagePullSecrets array of the existing secrets like `imagePullSecrets": [{"name": "mysecret"}, {"name": "existing_secret1"}, {"name": "existing_secret2"}]`

## Technical Details

* The root-level / directory in the container image is a bundle root directory of the bundle.

* The image source launches a pod to ensure that the bundle image is pulled using the CRI, which is beneficial because it means bundle
images are pulled with the exact same set of credentials, proxies, and other configurations that would be accessible to kubelet for any other workload images.

* The pod must be schedulable in the namespace in which the provisioner using the image source is running. There are implications with PSA which can cause
bundle images to fail to unpack. To avoid unpack failures and ensure widest compatibility with various provisioners, bundle image authors should ensure that
bundle images can be scheduled in a namespace with the restricted mode enforced. Bundle directory hierarchies in images should be traversable/readable by arbitrary users.