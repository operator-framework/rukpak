# Provisioner Spec [DRAFT]

## Overview

A provisioner is a controller responsible for reconciling `Bundle` and/or `BundleDeployment` objects using
provisioner-specific logic, but with a consistent API. This provisioner concept is inspired by and therefore very
similar to native Kubernetes's `Ingress` API and the ecosystem of ingress controllers.

The idea of a provisioner is to unpack bundle content and install that bundle content onto a cluster, and the
provisioner abstraction enables variations of bundle format and install/upgrade techniques to be implemented under a
single API.

In order for bundle consumers and producers to be able to treat bundles and bundle deployments homogeneously, all
provisioners must include certain functionality and capabilities.

## Terminology
| Term              | Description                                                |
|-------------------|------------------------------------------------------------|
| **Bundle Source** | A protocol that provisioners use to fetch bundle contents. |
| **Bundle Format** | A schema that describes the contents of a bundle.          |


## Requirements

1. A provisioner _must_ define one or more globally unique names for the `Bundle` and `BundleDeployment` controllers it
runs. For the purposes of provisioner class name references, this name must also conform to the
[DNS label schema](https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-label-names) as defined
in RFC 1123.
2. A provisioner _should_ use its unique controller names when configuring its watch predicates so that it only
reconciles bundles and bundle deployments that use its name.
3. A provisioner is not required to implement controllers for both bundles and bundle deployments.
   - There may be use cases where a `Bundle` provisioner fetches a bundle in one format and converts it to another
     format such that a different `BundleDeployment` provisioner can be used to install it.
   - There may also be use cases where different provisioners exist for to provide implementation variations for the
     same bundle formats. For example, two different provisioners that handle plain manifest bundles: one that performs
     "atomic" upgrades and one tha performs eventually consistent upgrades.
4. A provisioner _must_ reuse the defined condition types and phases when updating the status of bundles and bundle deployments.
5. A provisioner _should_ populate all condition types during every reconciliation, even if that means setting
   `condition.status = "Unknown"`. This enables consumers to avoid making false assumptions about the status of the
   object.
6. A bundle provisioner _must_ populate and update the observed generation in the bundle status such that it reflects
   the `metadata.generation` value.
7. A bundle provisioner _must_ populate the `contentURL` field and host a webserver at which the bundle can be fetched.
   - The webserver _must_ deny unauthorized access to the bundle content.
   - The webserver _must_ allow access to the bundle content via the `bundle-reader` cluster role provided by rukpak.
8. A bundle deployment provisioner _must_ populate and update the `activeBundle` field in the status to reflect the
   currently installed bundle. It is up to the provisioner implementation to define what "installed" means for its
   implementation.
9. A bundle provisioner _should_ implement _all_ concrete source types in the bundle spec. In the event that it does
   not implement a concrete source type, it _must_ populate the `Unpacked` condition with status `False` and reason
  `ReasonUnpackFailed` with a message explaining that the provisioner does not implement the desired source.
10. A bundle deployment provisioner _must_ reconcile embedded bundle objects by:
    - Ensuring that the desired bundle template exists as a `Bundle`
    - Ensuring that the desired bundle has successfully unpacked prior to triggering a pivot to it.
    - Ensuring that previous bundles associated with the bundle deployment are cleaned up as soon as possible after the
      desired bundle has been successfully installed.
