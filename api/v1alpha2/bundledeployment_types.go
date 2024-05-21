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

// BundleDeploymentSpec defines the desired state of BundleDeployment
type BundleDeploymentSpec struct {
	//+kubebuilder:validation:Pattern:=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	//+kubebuilder:validation:MaxLength:=63
	//
	// installNamespace is the namespace where the bundle should be installed. However, note that
	// the bundle may contain resources that are cluster-scoped or that are
	// installed in a different namespace. This namespace is expected to exist.
	InstallNamespace string `json:"installNamespace"`

	//+kubebuilder:validation:Pattern:=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	//
	// provisionerClassName sets the name of the provisioner that should reconcile this BundleDeployment.
	ProvisionerClassName string `json:"provisionerClassName"`

	// source defines the configuration for the underlying Bundle content.
	Source BundleSource `json:"source"`

	//+kubebuilder:pruning:PreserveUnknownFields
	//
	// config is provisioner specific configurations
	Config runtime.RawExtension `json:"config,omitempty"`

	//+kubebuilder:Optional
	// Preflight defines the configuration of preflight checks.
	Preflight *PreflightConfig `json:"preflight,omitempty"`
}

// PreflightConfig holds the configuration for the preflight checks.
type PreflightConfig struct {
	//+kubebuilder:Required
	// CRDUpgradeSafety holds necessary configuration for the CRD Upgrade Safety preflight checks.
	CRDUpgradeSafety *CRDUpgradeSafetyPreflightConfig `json:"crdUpgradeSafety,omitempty"`
}

// CRDUpgradeSafetyPreflightConfig is the configuration for CRD upgrade safety preflight check.
type CRDUpgradeSafetyPreflightConfig struct {
	//+kubebuilder:Required
	// Disabled represents the state of the CRD upgrade safety preflight check being disabled/enabled.
	Disabled bool `json:"disabled,omitempty"`
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
