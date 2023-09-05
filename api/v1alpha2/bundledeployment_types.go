/*
Copyright 2023.

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
)

var (
	BundleDeploymentGVK  = SchemeBuilder.GroupVersion.WithKind("BundleDeployment")
	BundleDeploymentKind = BundleDeploymentGVK.Kind
)

type FormatType string

const (
	FormatPlain      = "plain"
	FormatRegistryV1 = "registry"
	FormatHelm       = "helm"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName={"bd","bds"}
//+kubebuilder:storageversion

// BundleDeployment is the Schema for the bundledeployments API
// +kubebuilder:storageversion
type BundleDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BundleDeploymentSpec   `json:"spec"`
	Status BundleDeploymentStatus `json:"status,omitempty"`
}

// BundleDeploymentSpec defines the desired state of BundleDeployment
type BundleDeploymentSpec struct {
	// Source configures how to pull the bundle content.

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems:=1
	Sources []BundleDeplopymentSource `json:"sources"`
	// Format refers to the bundle type which is being passed through
	// the bundle deployment API.

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=plain;helm;registry
	Format FormatType `json:"format"`
	// Paused is used to configure whether we want the
	// bundle deployment to reconcile, or remmain in the
	// last observed state.

	// +kubebuilder:default:=false
	// +optional
	Paused bool `json:"paused"`
}

// FormatType refers to the allowed bundle formats that
// are being accepted in the APIs.
type FormatType string

// For more details on how each format looks like,
// refer: https://github.com/operator-framework/rukpak/tree/main/docs/bundles.
const (
	FormatPlain      = "plain"
	FormatRegistryV1 = "registry"
	FormatHelm       = "helm"
)

// BundleDeploymentStatus defines the observed state of BundleDeployment
type BundleDeploymentStatus struct {
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	CachedBundles      []string           `json:"cachedbundles,omitempty"`
}

// BundleDeploymentList contains a list of BundleDeployment

// +kubebuilder:object:root=true
type BundleDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BundleDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BundleDeployment{}, &BundleDeploymentList{})
}
