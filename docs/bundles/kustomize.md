# Kustomize Bundle Spec

## Overview

This document is meant to define the `Kustomize` bundle format as a reference for those publishing `Kustomize` bundles
for use with RukPak. For more information on the concept of a bundle, click [here](https://github.com/operator-framework/rukpak#bundle).

A Kustomize bundle is a [Kustomize](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/).
The Kustomize can be created by [Kustomize create](https://kubectl.docs.kubernetes.io/references/kustomize/cmd/create/).

The current Kustomize bundle format name is the `Kustomize+v0` that
combines the type of bundle (Kustomize) with the current format version (v0).

## Example

For example, below is an example of a file tree in a "Kustomize+v0" bundle.

```tree
<bundleRoot> 
├── kustomization.yaml
├── staging
│   └── kustomization.yaml
├── dev
│   └── kustomization.yaml
├── production
│   └── kustomization.yaml
└── base
    ├── kustomization.yaml
    └── pod.yaml
```
