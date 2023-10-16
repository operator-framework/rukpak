package healthchecks

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/cli-utils/pkg/kstatus/status"
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
// - Jobs
// - Services
// - PersistentVolumeClaims
// - PodDisruptionBudgets
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

		if object.GetObjectKind().GroupVersionKind() == apiregistrationv1.SchemeGroupVersion.WithKind("APIService") {
			obj := &apiregistrationv1.APIService{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
				gvkErrors = appendResourceError(gvkErrors, obj, err.Error())
				continue
			}

			// Check if the APIService is available.
			var isAvailable *apiregistrationv1.APIServiceCondition
			for i, condition := range obj.Status.Conditions {
				if condition.Type == apiregistrationv1.Available {
					isAvailable = &obj.Status.Conditions[i]
					break
				}
			}
			if isAvailable == nil {
				gvkErrors = appendResourceError(gvkErrors, obj, "Available condition not found")
			} else if isAvailable.Status == apiregistrationv1.ConditionFalse {
				gvkErrors = appendResourceError(gvkErrors, obj, isAvailable.Message)
			}
			continue
		}

		result, err := status.Compute(u)
		if err != nil {
			gvkErrors = appendResourceError(gvkErrors, object, err.Error())
		}

		if result.Status != status.CurrentStatus {
			gvkErrors = appendResourceError(gvkErrors, object, fmt.Sprintf("object %s: %s", result.Status, result.Message))
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
