package generic

import (
	"context"
	"errors"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AreObjectsHealthy checks if the given resources are healthy.
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
func AreObjectsHealthy(ctx context.Context, client client.Client, objects []client.Object) (bool, error) {
	gvkErrorsMap := make(map[string]string)

	// Create a new scheme to be able to convert the objects to their correct version.
	s := runtime.NewScheme()
	apiregistrationv1.AddToScheme(s)
	apiextensionsv1.AddToScheme(s)
	appsv1.AddToScheme(s)
	corev1.AddToScheme(s)

	for _, object := range objects {
		// Convert the object to its proper type, deepCopy it to avoid mutating the original object.
		obj, err := runtime.ObjectConvertor(s).ConvertToVersion(object.DeepCopyObject(), object.GetObjectKind().GroupVersionKind().GroupVersion())
		if err != nil {
			gvkErrorsMap[toErrKey(object)] = err.Error()
			continue
		}
		switch obj.(type) {
		case *appsv1.Deployment:
			deployment := obj.(*appsv1.Deployment)

			if err := client.Get(ctx, types.NamespacedName{
				Name:      deployment.GetName(),
				Namespace: deployment.GetNamespace(),
			}, deployment); err != nil {
				gvkErrorsMap[toErrKey(deployment)] = err.Error()
				continue
			}

			if deployment.Status.Conditions == nil {
				gvkErrorsMap[toErrKey(deployment)] = "Deployment has no conditions"
				continue
			}

			for _, condition := range deployment.Status.Conditions {
				if condition.Type == appsv1.DeploymentAvailable && condition.Status != "True" {
					gvkErrorsMap[toErrKey(deployment)] = condition.Message
				}
			}
		case *appsv1.StatefulSet:
			statefulSet := obj.(*appsv1.StatefulSet)
			if err := client.Get(ctx, types.NamespacedName{
				Name:      statefulSet.GetName(),
				Namespace: statefulSet.GetNamespace(),
			}, statefulSet); err != nil {
				gvkErrorsMap[toErrKey(statefulSet)] = err.Error()
				continue
			}
			if statefulSet.Status.ReadyReplicas != statefulSet.Status.Replicas {
				gvkErrorsMap[toErrKey(statefulSet)] = "StatefulSet is not ready"
			}
		case *appsv1.DaemonSet:
			daemonSet := obj.(*appsv1.DaemonSet)
			if err := client.Get(ctx, types.NamespacedName{
				Name:      daemonSet.GetName(),
				Namespace: daemonSet.GetNamespace(),
			}, daemonSet); err != nil {
				gvkErrorsMap[toErrKey(daemonSet)] = err.Error()
				continue
			}
			if daemonSet.Status.NumberAvailable != daemonSet.Status.DesiredNumberScheduled {
				gvkErrorsMap[toErrKey(daemonSet)] = "DaemonSet is not ready"
			}
		case *appsv1.ReplicaSet:
			replicaSet := obj.(*appsv1.ReplicaSet)
			if err := client.Get(ctx, types.NamespacedName{
				Name:      replicaSet.GetName(),
				Namespace: replicaSet.GetNamespace(),
			}, replicaSet); err != nil {
				gvkErrorsMap[toErrKey(replicaSet)] = err.Error()
				continue
			}
			if replicaSet.Status.AvailableReplicas != replicaSet.Status.Replicas {
				gvkErrorsMap[toErrKey(replicaSet)] = "ReplicaSet is not ready"
			}
		case *corev1.Pod:
			pod := obj.(*corev1.Pod)
			if err := client.Get(ctx, types.NamespacedName{
				Name:      pod.GetName(),
				Namespace: pod.GetNamespace(),
			}, pod); err != nil {
				gvkErrorsMap[toErrKey(pod)] = err.Error()
				continue
			}
			if pod.Status.Conditions == nil {
				gvkErrorsMap[toErrKey(pod)] = "Pod has no conditions"
				continue
			}
			if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
				gvkErrorsMap[toErrKey(pod)] = pod.Status.Message
			}
		case *apiregistrationv1.APIService:
			apiService := obj.(*apiregistrationv1.APIService)
			if err := client.Get(ctx, types.NamespacedName{
				Name:      apiService.GetName(),
				Namespace: apiService.GetNamespace(),
			}, apiService); err != nil {
				gvkErrorsMap[toErrKey(apiService)] = err.Error()
				continue
			}
			if apiService.Status.Conditions == nil {
				gvkErrorsMap[toErrKey(apiService)] = "APIService has no conditions"
				continue
			}
			for _, condition := range apiService.Status.Conditions {
				if condition.Type == apiregistrationv1.Available && condition.Status != "True" {
					gvkErrorsMap[toErrKey(apiService)] = condition.Message
				}
			}
		case *apiextensionsv1.CustomResourceDefinition:
			crd := obj.(*apiextensionsv1.CustomResourceDefinition)
			if err := client.Get(ctx, types.NamespacedName{
				Name:      crd.GetName(),
				Namespace: crd.GetNamespace(),
			}, crd); err != nil {
				gvkErrorsMap[toErrKey(crd)] = err.Error()
				continue
			}
			if crd.Status.Conditions == nil {
				gvkErrorsMap[toErrKey(crd)] = "CustomResourceDefinition has no conditions"
				continue
			}
			for _, condition := range crd.Status.Conditions {
				if condition.Type == apiextensionsv1.Established && condition.Status != "True" {
					gvkErrorsMap[toErrKey(crd)] = condition.Message
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

// toErrKey returns a string that uniquely identifies a resource based on its GVK and name.
// This is used to create a map of "GVK + Resource Name" to error message.
func toErrKey(resource client.Object) string {
	return fmt.Sprintf("%s.%s.%s/%s", resource.GetObjectKind().GroupVersionKind().Kind, resource.GetObjectKind().GroupVersionKind().Group, resource.GetObjectKind().GroupVersionKind().Version, resource.GetName())
}

// gvkErrorsMapToErr converts a map of "GVKs + Resource Name" to a single error.
func gvkErrorsMapToErr(gvkErrorsMap map[string]string) error {
	// Sort the keys to make the error message deterministic.
	var gvkKeys []string
	for gvk := range gvkErrorsMap {
		gvkKeys = append(gvkKeys, gvk)
	}

	// Sort the keys to make the error message deterministic.
	sort.Strings(gvkKeys)

	// Compose the error message, avoid trailing comma.
	var errStr string
	for i, gvk := range gvkKeys {
		errStr += gvk + ": " + gvkErrorsMap[gvk]
		if i != len(gvkKeys)-1 {
			errStr += ", "
		}
	}
	return errors.New(errStr)
}
