# RukPak

RukPak runs in a Kubernetes cluster and defines an API for installing cloud native bundle content. 

## Introduction 

RukPak is a pluggable solution for the packaging and distribution of cloud-native content and supports advanced strategies for installation, updates, and policy.
The project provides a content ecosystem for installing a variety of artifacts, such as Git repositories, Helm charts, OLM bundles, and more onto a Kubernetes cluster. 
These artifacts can then be managed, scaled, and upgraded in a safe way to enable powerful cluster extensions. 

At its core, RukPak is a small set of APIs, packaged as Kubernetes CustomResourceDefinitions, and controllers that watch for those APIs. These APIs express what 
content is being installed on-cluster and how to create a running instance of the content. 

## Components 

RukPak is composed of a few primary APIs: `Bundle`, `Instance`, and `ProvisionerClass`

### Bundle
A `Bundle` represents content that needs to be made available to other consumers in the cluster.
Much like the contents of a container image need to be pulled and unpacked in order for Pods to start using them, 
`Bundles` are used to reference content that may need to be pulled and should be unpacked. 
In this sense, Bundle is a generalization of the image concept, and can be used to represent any type of content.

`Bundles` do nothing on their own - they require a `Provisioner` to unpack and make their content available in-cluster. They can be
unpacked to any arbitrary storage medium such as a PersistentVolume or ConfigMap. Each `Bundle` has an associated `spec.class` field which 
indicates the `Provisioner` that should be watching and unpacking that particular bundle type. 

Example Bundle
```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata: 
  name: example-operator.v0.9.3
spec:
  class: rukpack.io/k8s
  refs:
  - file://content
  volumeMounts:
  - mountPath: /content
    configMap:
      name: local 
      namespace: plumbus
```

### Instance 
The `Instance` API points to a Bundle and indicates that it should be “active”. This includes pivoting from older versions of an active bundle. 
`Instance` may also include an embedded spec for a desired Bundle.

Much like Pods stamp out instances of container images, `Instances` stamp out an instance of Bundles. `Instance` can be seen as a generalization of the Pod concept.

The specifics of how an `Instance` makes changes to a cluster based on a referenced `Bundle` is defined by the 
`ProvisionerClass` and the `Provisioner` that is configured to handle that `ProvisionerClass`.

Example Instance
```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: Instance
metadata:
  name: resolved-654adh
spec:
  selector: 
    matchLabels:
      subscription: etcd-operator
  bundle:
    name: resolved-654adh
    spec:
      class: rukpack.io/k8s
      refs:
      - file://content
      volumeMounts:
      - mountPath: /content
        configMap:
          name: resolved-654adh-content
          namespace: olm
```

### ProvisionerClass
`ProvisionerClass` defines a configuration for a `Provisioner`. Provisioners are controllers that understand `Instance`, `Bundle`, and `ProvisionerClass` APIs and take action.

Each `Provisioner` has a unique id. For example, a `Provisioner` that understands bundles composed of arbitrary Kubernetes YAML manifests can have the id: rukpack.io/k8s.

A `ProvisionerClass` specifies a specific configuration of provisioning (i.e. settings to interpret Instance and Bundle APIs). 
A `Provisioner` will only operate on Instance and Bundle objects that reference a `ProvisionerClass` that contain the provisioner’s unique id.

If this seems familiar, it is the same pattern that is used by StorageClass / PeristentVolume / PersistentVolumeClaim in Kubernetes. This design is meant to be extendable 
to support `Bundles` and `Instances` backed by different content sources. 

Example ProvisionerClass
```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: ProvisionerClass
metadata:
  name: <good name>
provisioner: <unique id>
# parameters has no schema, is provisioner-specific
parameters: {}
```

## Contributing 

RukPak is a community-based open source project, and all are welcome to get involved. 