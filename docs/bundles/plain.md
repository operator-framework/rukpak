# Plain Bundle Spec

## Overview

This document is meant to define the `plain` bundle format as a reference for those publishing `plain` bundles
for use with RukPak. For more information on the concept of a bundle, click [here](https://github.com/operator-framework/rukpak#bundle).

A plain bundle is simply a collection of static, arbitrary, Kubernetes YAML manifests in a given directory.

The currently implemented plain bundle format is the `plain+v0` format. The name of the bundle format, `plain+v0`
combines the type of bundle (plain) with the current schema version (v0).

> Note: the plain+v0 bundle format is at schema version v0, which means it's an experimental format that is subject
> to change.

## Example

For example, below is an example of a file tree in a `plain+v0` bundle. It must have a `manifests` directory contains
the Kubernetes resources required to deploy an application.

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

> Note: there must be at least one resource in the manifests directory in order for the bundle to be a valid
> plain+v0 bundle.

## Technical Details

* The static manifests must be located in the /manifests directory for the bundle to be a
valid `plain+v0` bundle that the provisioner can unpack. A plain bundle image without a /manifests directory is
invalid and will not be successfully unpacked onto the cluster.
* The manifests directory should be flat: all manifests should be at the top-level with no subdirectories.
* Including any content in the /manifests directory of a plain bundle that is not static manifests will result in
a failure when creating content on-cluster from that bundle. Essentially, any file that would not
successfully `kubectl apply` will result in an error, but multi-object YAML files, or JSON files, are fine. There will
be validation tooling provided that can determine whether a given artifact is a valid bundle.