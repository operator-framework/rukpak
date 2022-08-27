# Helm Bundle Spec

## Overview

This document is meant to define the helm bundle format as a reference for those publishing helm bundles for use with
RukPak. A bundle is a collection of Kubernetes resources that are packaged together for the purposes of installing onto
a Kubernetes cluster. A bundle can be unpacked onto a running cluster, where controllers can then create the underlying
content embedded in the bundle. The bundle can be used as the underlying `spec.source` for
a [Bundle](https://github.com/operator-framework/rukpak#bundle) resource.

A helm bundle is a [helm chart](https://helm.sh/docs/chart_template_guide/getting_started/#charts).
The helm chart can be created by [helm create](https://helm.sh/docs/helm/helm_create/).

The current helm bundle format name is the `helm+v3` that
combines the type of bundle (helm) with the current helm format version (v3).

## Example

For example, below is some examples of a file tree in a "helm+v3" bundle.

```tree
<bundleRoot>
└── hello-world
    ├── .helmignore
    ├── Chart.yaml
    ├── README.md
    ├── templates
    │   ├── NOTES.txt
    │   ├── _helpers.tpl
    │   ├── deployment.yaml
    │   ├── service.yaml
    │   └── serviceaccount.yaml
    └── values.yaml
```

and

```tree
<bundleRoot>
├── .helmignore
├── Chart.yaml
├── README.md
├── templates
│   ├── NOTES.txt
│   ├── _helpers.tpl
│   ├── deployment.yaml
│   ├── service.yaml
│   └── serviceaccount.yaml
└── values.yaml
```
