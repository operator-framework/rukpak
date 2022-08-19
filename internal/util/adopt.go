package util

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/api/v1alpha1"
)

// AdoptObject sets metadata on an object to associate that object with a bundle
// deployment, such that it could be adopted by that bundle deployment.
//
// The systemNamespace is the namespace in which the provisioner managing the objects
// is running. And the bundleDeployment name is the name of the bundle deployment that
// should adopt the provided object.
//
// This function does _not_ apply the changes to a cluster, so callers must apply the
// updates themselves.
//
// NOTE: This function is designed specifically for the current helm-based
// implementation of the plain provisioner, and will track the plain provisioner's
// implementation. Should the plain provisioner change it's underlying mechanism
// for associating bundle deployments to managed objects, this implementation will
// also change.
func AdoptObject(obj client.Object, systemNamespace, bundleDeploymentName string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations["meta.helm.sh/release-name"] = bundleDeploymentName
	annotations["meta.helm.sh/release-namespace"] = systemNamespace
	obj.SetAnnotations(annotations)

	labels := obj.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	labels["app.kubernetes.io/managed-by"] = "Helm"
	labels[CoreOwnerKindKey] = v1alpha1.BundleDeploymentKind
	labels[CoreOwnerNameKey] = bundleDeploymentName
	obj.SetLabels(labels)
}
