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

package v1alpha2

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

// Add limit to the number of watchNamespaces allowed, as the estimated cost of this rule is linear per BD.
//+kubebuilder:validation:XValidation:rule="!has(self.watchNamespaces) || size(self.watchNamespaces) <= 1 || (size(self.watchNamespaces) > 1 && !self.watchNamespaces.exists(e, e == ''))",message="Empty string not accepted if length of watchNamespaces is more than 1."

// BundleDeploymentSpec defines the desired state of BundleDeployment
type BundleDeploymentSpec struct {
	//+kubebuilder:validation:Pattern:=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// ProvisionerClassName sets the name of the provisioner that should reconcile this BundleDeployment.
	ProvisionerClassName string `json:"provisionerClassName"`
	// Source defines the configuration for the underlying Bundle content.
	Source BundleSource `json:"source"`
	// Config is provisioner specific configurations
	// +kubebuilder:pruning:PreserveUnknownFields
	Config runtime.RawExtension `json:"config,omitempty"`
	// watchNamespaces indicates which namespaces the operator should watch.
	WatchNamespaces []string `json:"watchNamespaces,omitempty"`
}

// BundleDeploymentStatus defines the observed state of BundleDeployment
type BundleDeploymentStatus struct {
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ResolvedSource     *BundleSource      `json:"resolvedSource,omitempty"`
	ContentURL         string             `json:"contentURL,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster,shortName={"bd","bds"}
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

func (b *BundleDeployment) ProvisionerClassName() string {
	return b.Spec.ProvisionerClassName
}

//+kubebuilder:object:root=true

// BundleDeploymentList contains a list of BundleDeployment
type BundleDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BundleDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BundleDeployment{}, &BundleDeploymentList{})
}
