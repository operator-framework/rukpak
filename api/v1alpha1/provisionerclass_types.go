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

	Spec ProvisionerClassSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// ProvisionerClassList contains a list of ProvisionerClass
type ProvisionerClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProvisionerClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProvisionerClass{}, &ProvisionerClassList{})
}
