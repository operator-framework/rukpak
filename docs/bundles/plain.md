# Plain Bundle Spec

## Overview

This document is meant to define the plain bundle format as a reference for those publishing plain bundles for use with
RukPak. A bundle is a collection of Kubernetes resources that are packaged together for the purposes of installing onto
a Kubernetes cluster.

Users can specify a resource bundle to unpack or install via the
[`Bundle`](https://github.com/operator-framework/rukpak#bundle)
and [`BundleDeployment`](https://github.com/operator-framework/rukpak#bundledeployment) resources.
Controllers, called provisioners, consume the bundle referenced in the `Bundle(Deployment)` resource and store or apply the embedded manifests

A plain bundle is a static collection of arbitrary Kubernetes YAML manifests. These manifests are contained in a directory
that can be packaged in a container image layer , a `git` repository or any other content source that the
[plain bundle provisioner](../provisioners/plain.md) supports.

Supported source types for a plain bundle currently include the following:

* A directory in a [container image](../sources/image.md)
* A directory in a [`git` repository](../sources/git.md)
* A set of keys in a [`ConfigMap`](../sources/local.md)
* A `.tgz` file returned by a [http endpoint](../sources/http.md)


The currently implemented plain bundle format is the `plain+v0` format. The name of the bundle format, `plain+v0`
combines the type of bundle (plain) with the current schema version (v0). The
[plain bundle provisioner](../provisioners/plain.md) is able to source
`plain+v0` bundles and install them onto a Kubernetes cluster.

> Note: the `plain+v0` bundle format is at schema version v0, which means it's an experimental format that is subject
> to change.

## Common Terminology

* *bundle* is a collection of Kubernetes manifests that define content to be deployed to a cluster
* *bundle image* is a container image that contains a bundle within its filesystem
* *bundle git repo* is a git repository that contains a bundle within a directory

## Technical Details

* The [plain bundle provisioner](../provisioners/plain.md) is opinionated and expects the
plain bundle to be located in the root-level `/manifests` directory.
* The manifests directory should be flat: all manifests should be at the top-level with no subdirectories.
* It is required that `kubectl apply` is able to process all .yaml files in the directory that make up a plain bundle. For example,
multi-object YAML files are acceptable, but Ansible playbooks would not be.

## Building a plain bundle
### Prerequisites

In order to create a plain bundle for RukPak, ensure your Kubernetes manifests are in a flat directory at the root of
your project called `manifests/`. This allows the contents to be sourced and unpacked by the
[plain provisioner](..provisioners/plain.md). It should look similar to:

```bash
$ tree manifests
manifests
├── namespace.yaml
├── service_account.yaml
├── cluster_role.yaml
├── cluster_role_binding.yaml
└── deployment.yaml
```

> Note: there must be at least one resource in the manifests directory in order for the bundle to be a valid
> `plain+v0` bundle.

If you are using [kustomize](https://kustomize.io/) for building your manifests from templates, redirect the output into a single file under the `manifests/` directory. For example:
```bash
$ tree templates
templates
├── namespace.yaml
├── service_account.yaml
├── cluster_role.yaml
├── cluster_role_binding.yaml
├── deployment.yaml.yaml
└── kustomization.yaml
```

```bash
kustomize build templates > manifests/manifests.yaml
```

### Git source

For using the bundle with a [`git` source](../sources/git.md), commit your `manifests/` directory to your `git` repository.

### HTTP source

For using the bundle with a [HTTP source](../sources/http.md), create a `.tgz` file that contains the `manifests/` directory and its manifests. This `.tgz` file should be served when hitting the HTTP endpoint.

### Image source

For using the bundle with an [image source](../sources/image.md), follow the below steps:

1. Create a Dockerfile at the root of the project
```bash
touch Dockerfile.plainbundle
```

2. Edit the Dockerfile to include the following:
```bash
cat <<EOF >Dockerfile.plainbundle
FROM scratch
COPY manifests /manifests
EOF
```

> Note: The Dockerfile can have any `FROM ...` directive, but it is recommended to use the `FROM scratch`
> directive to keep the resulting image size minimal

3. Build an OCI-compliant image using any build tooling you prefer. Use an image tag that references a repository that you have push access to. For example,

```bash
docker build -f Dockerfile.plainbundle -t quay.io/operator-framework/rukpak:example .
```

4. Push the image to the remote registry

```bash
docker push quay.io/operator-framework/rukpak:example
```

If you are looking to use a private image registry for sourcing your bundle content, see the ["Private image registries" section of the image source documentation](../sources/image.md#private-image-registries)

### ConfigMap source

For using the bundle with a [`ConfigMap` source (also known as a local source)](../sources/local.md), follow the below steps:

1. Ensure RukPak is running. For more info on how to install RukPak on your cluster see the [Installation section of the README](../../README.md#installation)

2. Create a `ConfigMap` from the `manifests/` directory
```bash
kubectl create configmap my-configmap --from-file=manifests -n rukpak-system
```
