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

	"github.com/operator-framework/rukpak/pkg/source"
)

var (
	BundleGVK  = SchemeBuilder.GroupVersion.WithKind("Bundle")
	BundleKind = BundleGVK.Kind
)

const (
	TypeUnpacked = "Unpacked"

	ReasonUnpackPending             = "UnpackPending"
	ReasonUnpacking                 = "Unpacking"
	ReasonUnpackSuccessful          = "UnpackSuccessful"
	ReasonUnpackFailed              = "UnpackFailed"
	ReasonProcessingFinalizerFailed = "ProcessingFinalizerFailed"

	PhasePending   = "Pending"
	PhaseUnpacking = "Unpacking"
	PhaseFailing   = "Failing"
	PhaseUnpacked  = "Unpacked"
)

// BundleSpec defines the desired state of Bundle
type BundleSpec struct {
	//+kubebuilder:validation:Pattern:=^[a-z0-9]([-a-z0-9]*[a-z0-9])?$
	// ProvisionerClassName sets the name of the provisioner that should reconcile this BundleDeployment.
	ProvisionerClassName string `json:"provisionerClassName"`
	// Source defines the configuration for the underlying Bundle content.
	Source source.Source `json:"source"`
}

type ProvisionerID string

// BundleStatus defines the observed state of Bundle
type BundleStatus struct {
	Phase              string             `json:"phase,omitempty"`
	ResolvedSource     *source.Source     `json:"resolvedSource,omitempty"`
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	ContentURL         string             `json:"contentURL,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name=Type,type=string,JSONPath=`.spec.source.type`
//+kubebuilder:printcolumn:name=Phase,type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name=Age,type=date,JSONPath=`.metadata.creationTimestamp`
//+kubebuilder:printcolumn:name=Provisioner,type=string,JSONPath=`.spec.provisionerClassName`,priority=1
//+kubebuilder:printcolumn:name=Resolved Source,type=string,JSONPath=`.status.resolvedSource`,priority=1

// Bundle is the Schema for the bundles API
type Bundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BundleSpec   `json:"spec"`
	Status BundleStatus `json:"status,omitempty"`
}

func (b *Bundle) ProvisionerClassName() string {
	return b.Spec.ProvisionerClassName
}

//+kubebuilder:object:root=true

// BundleList contains a list of Bundle
type BundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bundle `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Bundle{}, &BundleList{})
}
