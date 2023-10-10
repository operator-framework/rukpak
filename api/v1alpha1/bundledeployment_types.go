/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	BundleDeploymentGVK  = SchemeBuilder.GroupVersion.WithKind("BundleDeployment")
	BundleDeploymentKind = BundleDeploymentGVK.Kind
)

const (
	TypeHasValidBundle = "HasValidBundle"
	TypeHealthy        = "Healthy"
	TypeInstalled      = "Installed"

	ReasonBundleLoadFailed          = "BundleLoadFailed"
	ReasonCreateDynamicWatchFailed  = "CreateDynamicWatchFailed"
	ReasonErrorGettingClient        = "ErrorGettingClient"
	ReasonErrorGettingReleaseState  = "ErrorGettingReleaseState"
	ReasonHealthy                   = "Healthy"
	ReasonInstallationStatusFalse   = "InstallationStatusFalse"
	ReasonInstallationStatusUnknown = "InstallationStatusUnknown"
	ReasonInstallationSucceeded     = "InstallationSucceeded"
	ReasonInstallFailed             = "InstallFailed"
	ReasonObjectLookupFailure       = "ObjectLookupFailure"
	ReasonReadingContentFailed      = "ReadingContentFailed"
	ReasonReconcileFailed           = "ReconcileFailed"
	ReasonUnhealthy                 = "Unhealthy"
	ReasonUpgradeFailed             = "UpgradeFailed"
)

// BundleDeploymentSpec defines the desired state of BundleDeployment
type BundleDeploymentSpec struct {
	//+kubebuilder:validation:Pattern:=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// ProvisionerClassName sets the name of the provisioner that should reconcile this BundleDeployment.
	ProvisionerClassName string `json:"provisionerClassName"`
	// Template describes the generated Bundle that this deployment will manage.
	Template *BundleTemplate `json:"template"`
	// Config is provisioner specific configurations
	// +kubebuilder:pruning:PreserveUnknownFields
	Config runtime.RawExtension `json:"config,omitempty"`
	// Availability Probes check objects that are part of the bundle deployment
	// +optional
	AvailabilityProbes []BundleDeploymentProbe `json:"availabilityProbes,omitempty"`
}

// BundleTemplate defines the desired state of a Bundle resource
type BundleTemplate struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the desired behavior of the Bundle.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	Spec BundleSpec `json:"spec"`
}

// BundleDeploymentStatus defines the observed state of BundleDeployment
type BundleDeploymentStatus struct {
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ActiveBundle       string             `json:"activeBundle,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName={"bd","bds"}
//+kubebuilder:printcolumn:name="Active Bundle",type=string,JSONPath=`.status.activeBundle`
//+kubebuilder:printcolumn:name="Install State",type=string,JSONPath=`.status.conditions[?(.type=="Installed")].reason`
//+kubebuilder:printcolumn:name=Age,type=date,JSONPath=`.metadata.creationTimestamp`
//+kubebuilder:printcolumn:name=Provisioner,type=string,JSONPath=`.spec.provisionerClassName`,priority=1

// BundleDeployment is the Schema for the bundledeployments API
type BundleDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BundleDeploymentSpec   `json:"spec"`
	Status BundleDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BundleDeploymentList contains a list of BundleDeployment
type BundleDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BundleDeployment `json:"items"`
}

// BundleDeploymentProbe define how BundleDeployments check their children for their status.
type BundleDeploymentProbe struct {
	// Probe configuration parameters.
	Probes []Probe `json:"probes"`
	// Selector specifies which objects this probe should target.
	Selector ProbeSelector `json:"selector"`
}

type ConditionMapping struct {
	// Source condition type.
	SourceType string `json:"sourceType"`
	// Destination condition type to report into Package Operator APIs.
	// +kubebuilder:validation:Pattern=`[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]`
	DestinationType string `json:"destinationType"`
}

// Selects a subset of objects to apply probes to.
// e.g. ensures that probes defined for apps/Deployments are not checked against ConfigMaps.
type ProbeSelector struct {
	// Kind and API Group of the object to probe.
	Kind *BundleDeploymentProbeKindSpec `json:"kind"`
	// Further sub-selects objects based on a Label Selector.
	// +example={matchLabels: {app.kubernetes.io/name: example-operator}}
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// Kind package probe parameters.
// selects objects based on Kind and API Group.
type BundleDeploymentProbeKindSpec struct {
	// Object Group to apply a probe to.
	// +example=apps
	Group string `json:"group"`
	// Object Kind to apply a probe to.
	// +example=Deployment
	Kind string `json:"kind"`
}

// Defines probe parameters. Only one can be filled.
type Probe struct {
	Condition   *ProbeConditionSpec   `json:"condition,omitempty"`
	FieldsEqual *ProbeFieldsEqualSpec `json:"fieldsEqual,omitempty"`
	CEL         *ProbeCELSpec         `json:"cel,omitempty"`
}

// Checks whether or not the object reports a condition with given type and status.
type ProbeConditionSpec struct {
	// Condition type to probe for.
	// +example=Available
	Type string `json:"type"`
	// Condition status to probe for.
	// +kubebuilder:default="True"
	Status string `json:"status"`
}

// Compares two fields specified by JSON Paths.
type ProbeFieldsEqualSpec struct {
	// First field for comparison.
	// +example=.spec.fieldA
	FieldA string `json:"fieldA"`
	// Second field for comparison.
	// +example=.status.fieldB
	FieldB string `json:"fieldB"`
}

// Uses Common Expression Language (CEL) to probe an object.
// CEL rules have to evaluate to a boolean to be valid.
// See:
// https://kubernetes.io/docs/reference/using-api/cel
// https://github.com/google/cel-go
type ProbeCELSpec struct {
	// CEL rule to evaluate.
	// +example=self.metadata.name == "Hans"
	Rule string `json:"rule"`
	// Error message to output if rule evaluates to false.
	// +example=Object must be named Hans
	Message string `json:"message"`
}

func init() {
	SchemeBuilder.Register(&BundleDeployment{}, &BundleDeploymentList{})
}
