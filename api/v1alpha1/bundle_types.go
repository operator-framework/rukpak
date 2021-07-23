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

type BundleSourceType string

// BundleVolumeMount is the specification of a single
// volume that contains bundle manifest content and
// how to mount that content.
type BundleVolumeMount struct {
	// MountPath is the filesystem path to the bundle
	// manifest content
	MountPath string `json:"mountPath"`
}

// +union
type BundleSource struct {
	// +unionDiscriminator
	// +optional
	VolumeMounts []BundleVolumeMount `json:"volumeMounts,omitempty"`
	Ref          string              `json:"ref,omitempty"`
}

// BundleSpec defines the desired state of Bundle
type BundleSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Likely in the future, we'll have the immediate need to handle something
	// like the `olm.bundle` class which has embedded knowledge that understands
	// how to unpack registry+v1 bundle image(s).
	// Class specifies the name of the ProvisionerClass to use for unpacking the bundle.
	Class  ProvisionerID `json:"class,omitempty"`
	Source BundleSource  `json:"source"`
}

// BundleStatus defines the observed state of Bundle
type BundleStatus struct {
	Conditions []metav1.Condition           `json:"conditions,omitempty"`
	Unpacked   BundleUnpackStatusType       `json:"unpacked,omitempty"`
	Volume     *corev1.LocalObjectReference `json:"volume"`
	URI        string                       `json:"uri,omitempty"`
}

type BundleUnpackStatusType string

const (
	BundleUnpacked       BundleUnpackStatusType = "Unpacked"
	BundleNeedsUnpacking BundleUnpackStatusType = "NeedsUnpacking"
)

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=rukpak,scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Unpacked",type=string,JSONPath=`.status.unpacked`
// +kubebuilder:printcolumn:name="URI",type=string,JSONPath=`.status.uri`

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
	SchemeBuilder.Register(&Bundle{}, &BundleList{})
}
