package healthchecks

import (
	"errors"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetObjectsHealth checks if the given resources are healthy.
// It returns true if all resources are healthy with nil error.
// It returns false if any of the resources are not healthy, the error contains the GVK + resourceName and the error message of
// each unhealthy resource.
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
func GetObjectsHealth(resources []client.Object) (bool, error) {
	gvkErrorsMap := make(map[string]string, 0)
	for _, resource := range resources {
		switch resource := resource.(type) {
		case *appsv1.Deployment:
			if resource.Status.Conditions == nil {
				gvkErrorsMap[toErrKey(resource)] = "Deployment has no conditions"
				continue
			}
			for _, condition := range resource.Status.Conditions {
				if condition.Type == appsv1.DeploymentAvailable && condition.Status != "True" {
					gvkErrorsMap[toErrKey(resource)] = condition.Message
				}
			}
		case *appsv1.StatefulSet:
			if resource.Status.ReadyReplicas != resource.Status.Replicas {
				gvkErrorsMap[toErrKey(resource)] = "StatefulSet is not ready"
			}
		case *appsv1.DaemonSet:
			if resource.Status.NumberAvailable != resource.Status.DesiredNumberScheduled {
				gvkErrorsMap[toErrKey(resource)] = "DaemonSet is not ready"
			}
		case *appsv1.ReplicaSet:
			if resource.Status.AvailableReplicas != resource.Status.Replicas {
				gvkErrorsMap[toErrKey(resource)] = "ReplicaSet is not ready"
			}
		case *corev1.Pod:
			if resource.Status.Conditions == nil {
				gvkErrorsMap[toErrKey(resource)] = "Pod has no conditions"
				continue
			}
			if resource.Status.Phase != corev1.PodRunning && resource.Status.Phase != corev1.PodSucceeded {
				gvkErrorsMap[toErrKey(resource)] = resource.Status.Message
			}
		case *apiregistrationv1.APIService:
			if resource.Status.Conditions == nil {
				gvkErrorsMap[toErrKey(resource)] = "APIService has no conditions"
				continue
			}
			for _, condition := range resource.Status.Conditions {
				if condition.Type == apiregistrationv1.Available && condition.Status != "True" {
					gvkErrorsMap[toErrKey(resource)] = condition.Message
				}
			}
		case *apiextensionsv1.CustomResourceDefinition:
			if resource.Status.Conditions == nil {
				gvkErrorsMap[toErrKey(resource)] = "CustomResourceDefinition has no conditions"
				continue
			}
			for _, condition := range resource.Status.Conditions {
				if condition.Type == apiextensionsv1.Established && condition.Status != "True" {
					gvkErrorsMap[toErrKey(resource)] = condition.Message
				}
			}
		default:
			// If we don't know how to check the health of the object, we assume it's healthy.
			continue
		}
	}

	if len(gvkErrorsMap) == 0 {
		return true, nil
	}
	return false, gvkErrorsMapToErr(gvkErrorsMap)
}

func toErrKey(resource client.Object) string {
	return fmt.Sprintf("%s.%s.%s/%s", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetObjectKind().GroupVersionKind().Group, resource.GetObjectKind().GroupVersionKind().Version, resource.GetName())
}

func gvkErrorsMapToErr(gvkErrorsMap map[string]string) error {
	var errStr string

	// Sort the keys to make the error message deterministic.
	var gvkKeys []string
	for gvk := range gvkErrorsMap {
		gvkKeys = append(gvkKeys, gvk)
	}
	sort.Strings(gvkKeys)
	for i, gvr := range gvkKeys {
		errStr += gvr + ": " + gvkErrorsMap[gvr]
		if i != len(gvkKeys)-1 {
			errStr += ", "
		}
	}
	return errors.New(errStr)
}
