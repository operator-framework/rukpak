## OCI Artifact source

## Summary

The [OCI Artifact](https://github.com/opencontainers/artifacts) source provides the contents in an OCI Artifact image as the source of the bundle.  The `source.type` for the image source is `artifact`.
When creating an artifact source, a reference to an URL of the artifact image is specified. It is expected that a proper format of bundle contents is present
in the artifact image.

## Example 

The [oras](https://oras.land) tool can be used to push bundle mainfests to an image registry as OCI Artifacts. For example, given the following bundle manifests in the plain bundle format that represents the v0.0.3 release of your operator: 

```tree
manifests
├── namespace.yaml
├── cluster_role.yaml
├── role.yaml
├── serviceaccount.yaml
├── cluster_role_binding.yaml
├── role_binding.yaml
└── deployment.yaml
```
the v0.0.3 bundle can be published an artifact image in the following way: 

```bash
$ oras push <image-registry>/<namespace>/<my-operator>:v0.0.3 --artifact-type=olm.bundle ./manifests:olm.operator.bundle+tar 
```
>Note: The OCI artifact image MUST have the `--artifact-type` value set to `olm.bundle`, in order for rukpak to pull and save the conent of the image on cluster.

The content of the image can be pulled locally with any of the tools available for working with OCI Artifacts, eg

```bash
$ mkdir pulled-content
$ cd pulled-content
$ oras pull <image-registry>/<namespace>/<my-operator>:<tag>@sha256:85fc05f87a32c7609184b56dec5a10a7002647e47a7948f906924cb3243c04fd
Downloading b0fdf2b375a9 <my-operator>.<tag>
Downloaded  b0fdf2b375a9 <my-operator>.<tag>
Pulled <image-registry>/<namespace>/<my-operator>:<tag>@sha256:85fc05f87a32c7609184b56dec5a10a7002647e47a7948f906924cb3243c04fd
Digest: sha256:85fc05f87a32c7609184b56dec5a10a7002647e47a7948f906924cb3243c04fd
$ tree
my-operator
├── namespace.yaml
├── cluster_role.yaml
├── role.yaml
├── serviceaccount.yaml
├── cluster_role_binding.yaml
├── role_binding.yaml
└── deployment.yaml                       
```

The OCI artifact source can then be refered to as a bundle source in the `BundleDeployment` object: 

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
        type: artifact
        image:
          ref: quay.io/<namespace>/<package>:<version>
      provisionerClassName: core-rukpak-io-plain
```