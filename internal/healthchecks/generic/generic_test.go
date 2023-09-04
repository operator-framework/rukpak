package healthchecks

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestGetObjectsHealth(t *testing.T) {
	for _, tt := range []struct {
		name        string
		resources   []client.Object
		healthy     bool
		expectedErr bool
	}{
		{
			name: "Return true, all resources are healthy",
			resources: []client.Object{
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyDeployment",
					},
					Status: appsv1.DeploymentStatus{
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: "True",
							},
						},
					},
				},
				&appsv1.StatefulSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyStatefulSet",
					},
					Status: appsv1.StatefulSetStatus{
						ReadyReplicas: 1,
						Replicas:      1,
					},
				},
				&appsv1.DaemonSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DaemonSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyDaemonSet",
					},
					Status: appsv1.DaemonSetStatus{
						NumberAvailable:        1,
						DesiredNumberScheduled: 1,
					},
				},
				&appsv1.ReplicaSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyReplicatSet",
					},
					Status: appsv1.ReplicaSetStatus{
						AvailableReplicas: 1,
						Replicas:          1,
					},
				},
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Pod",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyPod",
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: "True",
							},
						},
						Phase: corev1.PodRunning,
					},
				},
				&apiregistrationv1.APIService{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIService",
						APIVersion: "apiregistration.k8s.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyAPIService",
					},
					Status: apiregistrationv1.APIServiceStatus{
						Conditions: []apiregistrationv1.APIServiceCondition{
							{
								Type:   apiregistrationv1.Available,
								Status: "True",
							},
						},
					},
				},
				&apiextensionsv1.CustomResourceDefinition{
					TypeMeta: metav1.TypeMeta{
						Kind:       "CustomResourceDefinition",
						APIVersion: "apiextensions.k8s.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyCustomResourceDefinition",
					},
					Status: apiextensionsv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
							{
								Type:    apiextensionsv1.Established,
								Status:  "True",
								Message: "CustomResourceDefinition is established",
							},
						},
					},
				},
			},
			healthy:     true,
			expectedErr: false,
		},
		{
			name: "multiple resources are healthy, only on is not, return false and error",
			resources: []client.Object{
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyDeployment",
					},
					Status: appsv1.DeploymentStatus{
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: "True",
							},
						},
					},
				},
				&appsv1.StatefulSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyStatefulSet",
					},
					Status: appsv1.StatefulSetStatus{
						ReadyReplicas: 1,
						Replicas:      1,
					},
				},
				&appsv1.DaemonSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DaemonSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyDaemonSet",
					},
					Status: appsv1.DaemonSetStatus{
						NumberAvailable:        0,
						DesiredNumberScheduled: 1,
					},
				},
			},
			healthy:     false,
			expectedErr: true,
		},
		{
			name: "All resources are unhealthy, return false and error",
			resources: []client.Object{
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyDeployment",
					},
					Status: appsv1.DeploymentStatus{
						Conditions: []appsv1.DeploymentCondition{
							{
								Type:   appsv1.DeploymentAvailable,
								Status: "False",
							},
						},
					},
				},
				&appsv1.StatefulSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyStatefulSet",
					},
					Status: appsv1.StatefulSetStatus{
						ReadyReplicas: 0,
						Replicas:      1,
					},
				},
				&appsv1.DaemonSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DaemonSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyDaemonSet",
					},
					Status: appsv1.DaemonSetStatus{
						NumberAvailable:        0,
						DesiredNumberScheduled: 1,
					},
				},
				&appsv1.ReplicaSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyReplicatSet",
					},
					Status: appsv1.ReplicaSetStatus{
						AvailableReplicas: 0,
						Replicas:          1,
					},
				},
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Pod",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyPod",
					},
					Status: corev1.PodStatus{
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: "False",
							},
						},
					},
				},
				&apiregistrationv1.APIService{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIService",
						APIVersion: "apiregistration.k8s.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyAPIService",
					},
					Status: apiregistrationv1.APIServiceStatus{
						Conditions: []apiregistrationv1.APIServiceCondition{
							{
								Type:   apiregistrationv1.Available,
								Status: "False",
							},
						},
					},
				},
				&apiextensionsv1.CustomResourceDefinition{
					TypeMeta: metav1.TypeMeta{
						Kind:       "CustomResourceDefinition",
						APIVersion: "apiextensions.k8s.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyCustomResourceDefinition",
					},
					Status: apiextensionsv1.CustomResourceDefinitionStatus{
						Conditions: []apiextensionsv1.CustomResourceDefinitionCondition{
							{
								Type:    apiextensionsv1.Established,
								Status:  "False",
								Message: "CustomResourceDefinition is not established",
							},
						},
					},
				},
			},
			healthy:     false,
			expectedErr: true,
		},
		{
			name: "unknown resource, return true and no error",
			resources: []client.Object{
				&corev1.Service{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Service",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyService",
					},
				},
			},
			healthy:     true,
			expectedErr: false,
		},
		{
			name: "resource with no conditions, return false and error",
			resources: []client.Object{
				&corev1.Pod{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Pod",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyPod",
					},
					Status: corev1.PodStatus{
						Conditions: nil,
					},
				},
				&apiregistrationv1.APIService{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIService",
						APIVersion: "apiregistration.k8s.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyAPIService",
					},
					Status: apiregistrationv1.APIServiceStatus{
						Conditions: nil,
					},
				},
				&apiextensionsv1.CustomResourceDefinition{
					TypeMeta: metav1.TypeMeta{
						Kind:       "CustomResourceDefinition",
						APIVersion: "apiextensions.k8s.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyCustomResourceDefinition",
					},
					Status: apiextensionsv1.CustomResourceDefinitionStatus{
						Conditions: nil,
					},
				},
				&appsv1.Deployment{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Deployment",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyDeployment",
					},
					Status: appsv1.DeploymentStatus{
						Conditions: nil,
					},
				},
			},
			healthy:     false,
			expectedErr: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetObjectsHealth(tt.resources)
			if (err != nil) != tt.expectedErr {
				t.Errorf("AreRelObjectsHealthy() testName=%q  error = %v, expectedErr %v", tt.name, err, tt.expectedErr)
				return
			}
			if got != tt.healthy {
				t.Errorf("AreRelObjectsHealthy() testName=%q got %v, healthy %v", tt.name, got, tt.healthy)
			}
		})
	}
}
