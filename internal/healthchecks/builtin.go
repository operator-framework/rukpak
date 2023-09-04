package healthchecks

import (
	"context"
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AreObjectsHealthy checks if the given resources are healthy.
// It returns a nil error if all the resources are healthy, if any resource is not healthy, the error will
// contain the GVK + namespace/resourceName and the error message of each unhealthy resource.
//
// The current list of supported resources is:
// - Deployments
// - StatefulSets
// - DaemonSets
// - ReplicaSets
// - Pods
// - APIServices
// - CustomResourceDefinitions
//
// If the resource is not supported, it is assumed to be healthy.
func AreObjectsHealthy(ctx context.Context, client client.Client, objects []client.Object) error {
	var gvkErrors []error

	for _, object := range objects {
		objectKey := types.NamespacedName{
			Name:      object.GetName(),
			Namespace: object.GetNamespace(),
		}

		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(object.GetObjectKind().GroupVersionKind())
		if err := client.Get(ctx, objectKey, u); err != nil {
			gvkErrors = appendResourceError(gvkErrors, object, err.Error())
			continue
		}

		switch u.GroupVersionKind() {
		case appsv1.SchemeGroupVersion.WithKind("Deployment"):
			// Check if the deployment is available.
			obj := &appsv1.Deployment{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				gvkErrors = appendResourceError(gvkErrors, obj, err.Error())
				continue
			}
			conditionExists := false
			for _, condition := range obj.Status.Conditions {
				if condition.Type == appsv1.DeploymentAvailable {
					if condition.Status != "True" {
						gvkErrors = appendResourceError(gvkErrors, obj, condition.Message)
					}
					conditionExists = true
					break
				}
			}
			if conditionExists {
				continue
			}
			gvkErrors = appendResourceError(gvkErrors, obj, "DeploymentAvailable condition not found")
		case appsv1.SchemeGroupVersion.WithKind("StatefulSet"):
			// This logic has been adapted from the helm codebase.
			obj := &appsv1.StatefulSet{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				gvkErrors = appendResourceError(gvkErrors, obj, err.Error())
				continue
			}
			// This logic has been adapted from the helm codebase.
			// - https://github.com/helm/helm/blob/e7bb860d9a32e8739c944b8e7b7f7031d752411a/pkg/kube/ready.go#L357-L410

			// If the statefulset is not using the RollingUpdate strategy, we assume it's healthy.
			if obj.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
				continue
			}
			if obj.Status.ObservedGeneration < obj.Generation {
				gvkErrors = appendResourceError(gvkErrors, obj, "StatefulSet is not ready (update has not yet been observed)")
			}

			var partition int
			var replicas = 1
			if obj.Spec.UpdateStrategy.RollingUpdate != nil && obj.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
				partition = int(*obj.Spec.UpdateStrategy.RollingUpdate.Partition)
			}
			if obj.Spec.Replicas != nil {
				replicas = int(*obj.Spec.Replicas)
			}
			expectedReplicas := replicas - partition

			if obj.Status.UpdatedReplicas < int32(expectedReplicas) {
				gvkErrors = appendResourceError(gvkErrors, obj, fmt.Sprintf("StatefulSet is not ready (expected %d replicas, got %d)", expectedReplicas, obj.Status.UpdatedReplicas))
				continue
			}
			if int(obj.Status.ReadyReplicas) != replicas {
				gvkErrors = appendResourceError(gvkErrors, obj, fmt.Sprintf("StatefulSet is not ready (expected %d replicas, got %d)", replicas, obj.Status.ReadyReplicas))
				continue
			}
			if partition == 0 && obj.Status.CurrentRevision != obj.Status.UpdateRevision {
				gvkErrors = appendResourceError(gvkErrors, obj, fmt.Sprintf("StatefulSet is not ready (expected revision %s, got %s)", obj.Status.CurrentRevision, obj.Status.UpdateRevision))
				continue
			}
		case appsv1.SchemeGroupVersion.WithKind("DaemonSet"):
			// Check if the daemonset is ready.
			obj := &appsv1.DaemonSet{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				gvkErrors = appendResourceError(gvkErrors, obj, err.Error())
				continue
			}
			if obj.Status.NumberAvailable != obj.Status.DesiredNumberScheduled {
				gvkErrors = appendResourceError(gvkErrors, obj, "DaemonSet is not ready")
			}
		case appsv1.SchemeGroupVersion.WithKind("ReplicaSet"):
			// Check if the replicaset is ready.
			obj := &appsv1.ReplicaSet{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				gvkErrors = appendResourceError(gvkErrors, obj, err.Error())
				continue
			}
			if obj.Status.AvailableReplicas != obj.Status.Replicas {
				gvkErrors = appendResourceError(gvkErrors, obj, "ReplicaSet is not ready")
			}
		case corev1.SchemeGroupVersion.WithKind("Pod"):
			// Check if the pod is running or succeeded.
			obj := &corev1.Pod{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				gvkErrors = appendResourceError(gvkErrors, obj, err.Error())
				continue
			}
			if obj.Status.Phase != corev1.PodRunning && obj.Status.Phase != corev1.PodSucceeded {
				gvkErrors = appendResourceError(gvkErrors, obj, "Pod is not Running or Succeeded")
			}
		case apiregistrationv1.SchemeGroupVersion.WithKind("APIService"):
			// Check if the APIService is available.
			obj := &apiregistrationv1.APIService{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				gvkErrors = appendResourceError(gvkErrors, obj, err.Error())
				continue
			}
			conditionExists := false
			for _, condition := range obj.Status.Conditions {
				if condition.Type == apiregistrationv1.Available {
					if condition.Status != "True" {
						gvkErrors = appendResourceError(gvkErrors, obj, condition.Message)
					}
					conditionExists = true
					break
				}
			}
			if conditionExists {
				continue
			}
			// If we are here we didn't find the "Available" condition, so we assume the APIService is non healthy.
			gvkErrors = appendResourceError(gvkErrors, obj, "Available condition not found")
		case apiextensionsv1.SchemeGroupVersion.WithKind("CustomResourceDefinition"):
			// Check if the CRD is established.
			obj := &apiextensionsv1.CustomResourceDefinition{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				gvkErrors = appendResourceError(gvkErrors, obj, err.Error())
				continue
			}
			conditionExists := false
			for _, condition := range obj.Status.Conditions {
				if condition.Type == apiextensionsv1.Established {
					if condition.Status != "True" {
						gvkErrors = appendResourceError(gvkErrors, obj, condition.Message)
					}
					conditionExists = true
					break
				}
			}
			if conditionExists {
				continue
			}
			gvkErrors = appendResourceError(gvkErrors, obj, "Established condition not found")
		default:
			// If we don't know how to check the health of the object, we assume it's healthy.
			continue
		}
	}

	return errors.Join(gvkErrors...)
}

// toErrKey returns a string that identifies a resource based on its GVK and namespace/name. This key is used
// to identify the resource in the error message.
func toErrKey(resource client.Object) string {
	// If the resource is namespaced, include the namespace in the key.
	if resource.GetNamespace() != "" {
		return fmt.Sprintf("(%s)(%s/%s)", resource.GetObjectKind().GroupVersionKind().String(), resource.GetNamespace(), resource.GetName())
	}

	return fmt.Sprintf("(%s)(%s)", resource.GetObjectKind().GroupVersionKind().String(), resource.GetName())
}

// appendResourceError appends a new error to the given slice of errors and returns it.
func appendResourceError(gvkErrors []error, resource client.Object, message string) []error {
	return append(gvkErrors, errors.New(toErrKey(resource)+": "+message))
}
