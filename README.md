# rukpak

Rukpak runs in a Kubernetes cluster and defines an API for installing cloud native bundle content

## Installation

### PreRequisites

Before proceeding with the installing rukpak, ensure the following packages are present:

- Go
- Git
- Make
- Kubectl

Clone the rukpak directory using the following command:

```bash
git clone https://github.com/operator-framework/rukpak
```

### Installing RukPak

Before installing the various RukPak resources, ensure you have an existing Kubernetes cluster installed locally.

In order to create a KinD cluster locally, run the following commands:

```bash
kind create cluster
kind export cluster
```

Once a cluster is present, install the RukPak stack:

```bash
make install
```

The `install` Makefile target is responsible for creating a `rukpak` namespace and applying any underlying Kubernetes resources that are needed for the RukPak stack.
