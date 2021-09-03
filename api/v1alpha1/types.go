/*


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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ProvisionerID string

// ProvisionerClassSpec defines the desired state of ProvisionerClass
type ProvisionerClassSpec struct {
	Provisioner ProvisionerID `json:"provisioner"`

	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=rukpak,scope=Cluster
// +kubebuilder:subresource:status

// ProvisionerClass is the Schema for the provisionerclasses API
type ProvisionerClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ProvisionerClassSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// ProvisionerClassList contains a list of ProvisionerClass
type ProvisionerClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProvisionerClass `json:"items"`
}

// Source locates content to stage.
type Source string

func (s Source) String() string {
	return string(s)
}

// BundleSpec defines the desired state of Bundle
type BundleSpec struct {
	// Class specifies the name of the provisioner that should manage the Bundle.
	// +optional
	Class string `json:"class,omitempty"`

	// Source locates all remote content to be staged by the Bundle.
	Source Source `json:"source"`
}

// Content surfaces staged content for clients to access.
type Content string

type BundleUnpackStatusType string

const (
	BundleUnpacked       BundleUnpackStatusType = "Unpacked"
	BundleNeedsUnpacking BundleUnpackStatusType = "NeedsUnpacking"
)

// BundleStatus defines the observed state of Bundle
type BundleStatus struct {
	// Provisioner is the ID of the provisioner managing this Bundle.
	// +optional
	Provisioner ProvisionerID `json:"provisioner,omitempty"`
	// Volume is a reference to a local volume
	Volume *corev1.LocalObjectReference `json:"volume,omitempty"`
	// Unpacked reflects whether the bundle needs unpacking or has already been unpacked
	Unpacked BundleUnpackStatusType `json:"unpacked,omitempty"`
	// Contents provides access to all staged bundle content.
	// +optional
	Contents []Content `json:"contents,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (b *Bundle) SetUnpackedStatus(unpacked BundleUnpackStatusType) {
	b.Status.Unpacked = unpacked
}

func (b *Bundle) SetVolumeRef(ref *corev1.LocalObjectReference) {
	b.Status.Volume = ref
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=rukpak,scope=Cluster
// +kubebuilder:subresource:status

// Bundle is the Schema for the bundles API
type Bundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BundleSpec   `json:"spec,omitempty"`
	Status BundleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BundleList contains a list of Bundle
type BundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Bundle `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProvisionerClass{}, &ProvisionerClassList{}, &Bundle{}, &BundleList{})
}
