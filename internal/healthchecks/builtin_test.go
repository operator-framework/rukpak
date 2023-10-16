package healthchecks

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAreObjectsHealthy(t *testing.T) {
	for _, tt := range []struct {
		name        string
		resources   []client.Object
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
						Replicas:          1,
						UpdatedReplicas:   1,
						AvailableReplicas: 1,
						ReadyReplicas:     1,
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
						ReadyReplicas:   1,
						Replicas:        1,
						CurrentReplicas: 1,
					},
				},
				&appsv1.DaemonSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DaemonSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "MyDaemonSet",
						Generation: 1,
					},
					Status: appsv1.DaemonSetStatus{
						NumberAvailable:        1,
						DesiredNumberScheduled: 1,
						ObservedGeneration:     1,
						CurrentNumberScheduled: 1,
						UpdatedNumberScheduled: 1,
						NumberReady:            1,
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
						AvailableReplicas:    1,
						Replicas:             1,
						FullyLabeledReplicas: 1,
						ReadyReplicas:        1,
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
			expectedErr: false,
		},
		{
			name: "multiple resources are healthy, only one is not, return error",
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
						Replicas:          1,
						UpdatedReplicas:   1,
						AvailableReplicas: 1,
						ReadyReplicas:     1,
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
						ReadyReplicas:   1,
						Replicas:        1,
						CurrentReplicas: 1,
					},
				},
				&appsv1.DaemonSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DaemonSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "MyDaemonSet",
						Generation: 1,
					},
					Status: appsv1.DaemonSetStatus{
						NumberAvailable:        0,
						DesiredNumberScheduled: 1,
						ObservedGeneration:     1,
						CurrentNumberScheduled: 1,
						UpdatedNumberScheduled: 1,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "All resources are unhealthy, return error",
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
								Type:    appsv1.DeploymentAvailable,
								Status:  "False",
								Message: "Something went wrong",
							},
						},
						Replicas:          1,
						UpdatedReplicas:   1,
						AvailableReplicas: 1,
						ReadyReplicas:     1,
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
						ReadyReplicas:   0,
						Replicas:        1,
						CurrentReplicas: 1,
					},
				},
				&appsv1.DaemonSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DaemonSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "MyDaemonSet",
						Generation: 1,
					},
					Status: appsv1.DaemonSetStatus{
						NumberAvailable:        0,
						DesiredNumberScheduled: 1,
						ObservedGeneration:     1,
						CurrentNumberScheduled: 1,
						UpdatedNumberScheduled: 1,
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
						AvailableReplicas:    0,
						Replicas:             1,
						FullyLabeledReplicas: 1,
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
						Phase: corev1.PodRunning,
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
								Type:    apiregistrationv1.Available,
								Status:  "False",
								Message: "Something went wrong",
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
			expectedErr: true,
		},
		{
			name: "unknown resource, no error",
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
			expectedErr: false,
		},
		{
			name: "Pod: valid resource with no conditions doesn't return error",
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
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodReady,
								Status: corev1.ConditionTrue,
							},
						},
						Phase: corev1.PodRunning,
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "APIService: resource with no conditions, return error",
			resources: []client.Object{
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
			},
			expectedErr: true,
		},
		{
			name: "CustomResourceDefinition: resource with no conditions return error",
			resources: []client.Object{
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
			},
			expectedErr: true,
		},
		{
			name: "Deployment: resource with no conditions return error",
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
						Conditions:        nil,
						Replicas:          1,
						UpdatedReplicas:   1,
						AvailableReplicas: 1,
						ReadyReplicas:     1,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "StatefulSet: valid resource with no conditions, doesn't return error",
			resources: []client.Object{
				&appsv1.StatefulSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "MyStatefulSet",
						Generation: 1,
					},
					Spec: appsv1.StatefulSetSpec{
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.RollingUpdateStatefulSetStrategyType,
						},
						Replicas: pointer.Int32(1),
					},
					Status: appsv1.StatefulSetStatus{
						Conditions:         nil,
						ObservedGeneration: 1,
						ReadyReplicas:      1,
						Replicas:           1,
						CurrentReplicas:    1,
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "DaemonSet: valid resource with no conditions, doesn't return error",
			resources: []client.Object{
				&appsv1.DaemonSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DaemonSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "MyDaemonSet",
						Generation: 1,
					},
					Status: appsv1.DaemonSetStatus{
						DesiredNumberScheduled: 1,
						NumberAvailable:        1,
						Conditions:             nil,
						ObservedGeneration:     1,
						CurrentNumberScheduled: 1,
						UpdatedNumberScheduled: 1,
						NumberReady:            1,
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "ReplicaSet: valid resource with no conditions, doesn't return error",
			resources: []client.Object{
				&appsv1.ReplicaSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ReplicaSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyReplicaSet",
					},
					Status: appsv1.ReplicaSetStatus{
						Replicas:             1,
						AvailableReplicas:    1,
						Conditions:           nil,
						FullyLabeledReplicas: 1,
						ReadyReplicas:        1,
					},
				},
			},
			expectedErr: false,
		},
		{
			name: "APIService: resource with conditions but not the one we are looking for, return error",
			resources: []client.Object{

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
								Type:   "testing",
								Status: "True",
							},
						},
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "CustomResourceDefinition: resource with conditions but not the one we are looking for, return error",
			resources: []client.Object{
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
								Type:    apiextensionsv1.NamesAccepted,
								Status:  "True",
								Message: "CustomResourceDefinition names have been accepted",
							},
						},
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "Deployment: resource with conditions but not the one we are looking for, return error",
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
								Type:   appsv1.DeploymentProgressing,
								Status: "True",
							},
						},
						Replicas:          1,
						UpdatedReplicas:   1,
						AvailableReplicas: 1,
						ReadyReplicas:     1,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "Pod: resource with conditions but not the one we are looking for, return error",
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
						Phase: corev1.PodRunning,
						Conditions: []corev1.PodCondition{
							{
								Type:   corev1.PodInitialized,
								Status: "True",
							},
						},
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "APIService: resource with conditions but not the one we are looking for, return error",
			resources: []client.Object{
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
								Type:   "testing",
								Status: "True",
							},
						},
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "StatefulSet is not ready as observedGeneration doesn't match, return error",
			resources: []client.Object{
				&appsv1.StatefulSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "MyStatefulSet",
						Generation: 2,
					},
					Spec: appsv1.StatefulSetSpec{
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.RollingUpdateStatefulSetStrategyType,
						},
						Replicas: pointer.Int32(1),
					},
					Status: appsv1.StatefulSetStatus{
						ObservedGeneration: 1,
						ReadyReplicas:      1,
						Replicas:           1,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "StatefulSet is not ready as replicas are not ready, return error",
			resources: []client.Object{
				&appsv1.StatefulSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "MyStatefulSet",
						Generation: 2,
					},
					Spec: appsv1.StatefulSetSpec{
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.RollingUpdateStatefulSetStrategyType,
						},
						Replicas: pointer.Int32(1),
					},
					Status: appsv1.StatefulSetStatus{
						ObservedGeneration: 2,
						ReadyReplicas:      0,
						UpdatedReplicas:    1,
						Replicas:           1,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "StatefulSet is not ready as Revisions don't match, return error",
			resources: []client.Object{
				&appsv1.StatefulSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:       "MyStatefulSet",
						Generation: 2,
					},
					Spec: appsv1.StatefulSetSpec{
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.RollingUpdateStatefulSetStrategyType,
						},
						Replicas: pointer.Int32(1),
					},
					Status: appsv1.StatefulSetStatus{
						CurrentRevision:    "revision2",
						UpdateRevision:     "revision1",
						ObservedGeneration: 2,
						ReadyReplicas:      1,
						UpdatedReplicas:    1,
						Replicas:           1,
						CurrentReplicas:    1,
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "StatefulSet is not using the RollingUpdate strategy, return no error",
			resources: []client.Object{
				&appsv1.StatefulSet{
					TypeMeta: metav1.TypeMeta{
						Kind:       "StatefulSet",
						APIVersion: "apps/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: "MyStatefulSet",
					},
					Spec: appsv1.StatefulSetSpec{
						UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
							Type: appsv1.OnDeleteStatefulSetStrategyType,
						},
					},
					Status: appsv1.StatefulSetStatus{},
				},
			},
			expectedErr: false,
		},
	} {
		ctx := context.Background()
		client := fakeClient{}
		t.Run(tt.name, func(t *testing.T) {
			// Instantiate a fake client.
			client.setResources(tt.resources)
			err := AreObjectsHealthy(ctx, client, tt.resources)
			if (err != nil) != tt.expectedErr {
				t.Errorf("AreRelObjectsHealthy() testName=%q  error = %v, expectedErr %v", tt.name, err, tt.expectedErr)
			}
		})
	}
}

// Fake client for testing, implementing the client.Client interface.
type fakeClient struct {
	client.Client
	resources []client.Object
}

// setResources is used to populate the fake client with the resources we want to test.
func (f *fakeClient) setResources(resources []client.Object) {
	f.resources = resources
}

// Get is a fake implementation of the client.Client.Get method, the generic healthcheck only requires the Get method.
func (f fakeClient) Get(_ context.Context, objectKey types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
	for _, resource := range f.resources {
		if resource.GetNamespace() == objectKey.Namespace && resource.GetName() == objectKey.Name && resource.GetObjectKind().GroupVersionKind() == obj.GetObjectKind().GroupVersionKind() {
			// copy the resource into the obj, obj is unstructured.Unstructured
			// and resource is a typed object, so we need to convert it.
			u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(resource)
			if err != nil {
				return err
			}
			return runtime.DefaultUnstructuredConverter.FromUnstructured(u, obj)
		}
	}
	return errors.New("resource not found")
}

func (f fakeClient) Scheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(apiregistrationv1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
	utilruntime.Must(appsv1.AddToScheme(s))
	utilruntime.Must(corev1.AddToScheme(s))
	return s
}
