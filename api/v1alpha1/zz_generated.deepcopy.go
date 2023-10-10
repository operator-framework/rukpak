//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Authorization) DeepCopyInto(out *Authorization) {
	*out = *in
	out.Secret = in.Secret
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Authorization.
func (in *Authorization) DeepCopy() *Authorization {
	if in == nil {
		return nil
	}
	out := new(Authorization)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Bundle) DeepCopyInto(out *Bundle) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Bundle.
func (in *Bundle) DeepCopy() *Bundle {
	if in == nil {
		return nil
	}
	out := new(Bundle)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Bundle) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleDeployment) DeepCopyInto(out *BundleDeployment) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleDeployment.
func (in *BundleDeployment) DeepCopy() *BundleDeployment {
	if in == nil {
		return nil
	}
	out := new(BundleDeployment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BundleDeployment) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleDeploymentList) DeepCopyInto(out *BundleDeploymentList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]BundleDeployment, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleDeploymentList.
func (in *BundleDeploymentList) DeepCopy() *BundleDeploymentList {
	if in == nil {
		return nil
	}
	out := new(BundleDeploymentList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BundleDeploymentList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleDeploymentProbe) DeepCopyInto(out *BundleDeploymentProbe) {
	*out = *in
	if in.Probes != nil {
		in, out := &in.Probes, &out.Probes
		*out = make([]Probe, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	in.Selector.DeepCopyInto(&out.Selector)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleDeploymentProbe.
func (in *BundleDeploymentProbe) DeepCopy() *BundleDeploymentProbe {
	if in == nil {
		return nil
	}
	out := new(BundleDeploymentProbe)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleDeploymentProbeKindSpec) DeepCopyInto(out *BundleDeploymentProbeKindSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleDeploymentProbeKindSpec.
func (in *BundleDeploymentProbeKindSpec) DeepCopy() *BundleDeploymentProbeKindSpec {
	if in == nil {
		return nil
	}
	out := new(BundleDeploymentProbeKindSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleDeploymentSpec) DeepCopyInto(out *BundleDeploymentSpec) {
	*out = *in
	if in.Template != nil {
		in, out := &in.Template, &out.Template
		*out = new(BundleTemplate)
		(*in).DeepCopyInto(*out)
	}
	in.Config.DeepCopyInto(&out.Config)
	if in.AvailabilityProbes != nil {
		in, out := &in.AvailabilityProbes, &out.AvailabilityProbes
		*out = make([]BundleDeploymentProbe, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleDeploymentSpec.
func (in *BundleDeploymentSpec) DeepCopy() *BundleDeploymentSpec {
	if in == nil {
		return nil
	}
	out := new(BundleDeploymentSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleDeploymentStatus) DeepCopyInto(out *BundleDeploymentStatus) {
	*out = *in
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleDeploymentStatus.
func (in *BundleDeploymentStatus) DeepCopy() *BundleDeploymentStatus {
	if in == nil {
		return nil
	}
	out := new(BundleDeploymentStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleList) DeepCopyInto(out *BundleList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Bundle, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleList.
func (in *BundleList) DeepCopy() *BundleList {
	if in == nil {
		return nil
	}
	out := new(BundleList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *BundleList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleSource) DeepCopyInto(out *BundleSource) {
	*out = *in
	if in.Image != nil {
		in, out := &in.Image, &out.Image
		*out = new(ImageSource)
		**out = **in
	}
	if in.Git != nil {
		in, out := &in.Git, &out.Git
		*out = new(GitSource)
		**out = **in
	}
	if in.ConfigMaps != nil {
		in, out := &in.ConfigMaps, &out.ConfigMaps
		*out = make([]ConfigMapSource, len(*in))
		copy(*out, *in)
	}
	if in.Upload != nil {
		in, out := &in.Upload, &out.Upload
		*out = new(UploadSource)
		**out = **in
	}
	if in.HTTP != nil {
		in, out := &in.HTTP, &out.HTTP
		*out = new(HTTPSource)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleSource.
func (in *BundleSource) DeepCopy() *BundleSource {
	if in == nil {
		return nil
	}
	out := new(BundleSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleSpec) DeepCopyInto(out *BundleSpec) {
	*out = *in
	in.Source.DeepCopyInto(&out.Source)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleSpec.
func (in *BundleSpec) DeepCopy() *BundleSpec {
	if in == nil {
		return nil
	}
	out := new(BundleSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleStatus) DeepCopyInto(out *BundleStatus) {
	*out = *in
	if in.ResolvedSource != nil {
		in, out := &in.ResolvedSource, &out.ResolvedSource
		*out = new(BundleSource)
		(*in).DeepCopyInto(*out)
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]v1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleStatus.
func (in *BundleStatus) DeepCopy() *BundleStatus {
	if in == nil {
		return nil
	}
	out := new(BundleStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BundleTemplate) DeepCopyInto(out *BundleTemplate) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BundleTemplate.
func (in *BundleTemplate) DeepCopy() *BundleTemplate {
	if in == nil {
		return nil
	}
	out := new(BundleTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConditionMapping) DeepCopyInto(out *ConditionMapping) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConditionMapping.
func (in *ConditionMapping) DeepCopy() *ConditionMapping {
	if in == nil {
		return nil
	}
	out := new(ConditionMapping)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConfigMapSource) DeepCopyInto(out *ConfigMapSource) {
	*out = *in
	out.ConfigMap = in.ConfigMap
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConfigMapSource.
func (in *ConfigMapSource) DeepCopy() *ConfigMapSource {
	if in == nil {
		return nil
	}
	out := new(ConfigMapSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitRef) DeepCopyInto(out *GitRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitRef.
func (in *GitRef) DeepCopy() *GitRef {
	if in == nil {
		return nil
	}
	out := new(GitRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitSource) DeepCopyInto(out *GitSource) {
	*out = *in
	out.Ref = in.Ref
	out.Auth = in.Auth
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitSource.
func (in *GitSource) DeepCopy() *GitSource {
	if in == nil {
		return nil
	}
	out := new(GitSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *HTTPSource) DeepCopyInto(out *HTTPSource) {
	*out = *in
	out.Auth = in.Auth
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HTTPSource.
func (in *HTTPSource) DeepCopy() *HTTPSource {
	if in == nil {
		return nil
	}
	out := new(HTTPSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ImageSource) DeepCopyInto(out *ImageSource) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ImageSource.
func (in *ImageSource) DeepCopy() *ImageSource {
	if in == nil {
		return nil
	}
	out := new(ImageSource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Probe) DeepCopyInto(out *Probe) {
	*out = *in
	if in.Condition != nil {
		in, out := &in.Condition, &out.Condition
		*out = new(ProbeConditionSpec)
		**out = **in
	}
	if in.FieldsEqual != nil {
		in, out := &in.FieldsEqual, &out.FieldsEqual
		*out = new(ProbeFieldsEqualSpec)
		**out = **in
	}
	if in.CEL != nil {
		in, out := &in.CEL, &out.CEL
		*out = new(ProbeCELSpec)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Probe.
func (in *Probe) DeepCopy() *Probe {
	if in == nil {
		return nil
	}
	out := new(Probe)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProbeCELSpec) DeepCopyInto(out *ProbeCELSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProbeCELSpec.
func (in *ProbeCELSpec) DeepCopy() *ProbeCELSpec {
	if in == nil {
		return nil
	}
	out := new(ProbeCELSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProbeConditionSpec) DeepCopyInto(out *ProbeConditionSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProbeConditionSpec.
func (in *ProbeConditionSpec) DeepCopy() *ProbeConditionSpec {
	if in == nil {
		return nil
	}
	out := new(ProbeConditionSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProbeFieldsEqualSpec) DeepCopyInto(out *ProbeFieldsEqualSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProbeFieldsEqualSpec.
func (in *ProbeFieldsEqualSpec) DeepCopy() *ProbeFieldsEqualSpec {
	if in == nil {
		return nil
	}
	out := new(ProbeFieldsEqualSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProbeSelector) DeepCopyInto(out *ProbeSelector) {
	*out = *in
	if in.Kind != nil {
		in, out := &in.Kind, &out.Kind
		*out = new(BundleDeploymentProbeKindSpec)
		**out = **in
	}
	if in.Selector != nil {
		in, out := &in.Selector, &out.Selector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProbeSelector.
func (in *ProbeSelector) DeepCopy() *ProbeSelector {
	if in == nil {
		return nil
	}
	out := new(ProbeSelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *UploadSource) DeepCopyInto(out *UploadSource) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new UploadSource.
func (in *UploadSource) DeepCopy() *UploadSource {
	if in == nil {
		return nil
	}
	out := new(UploadSource)
	in.DeepCopyInto(out)
	return out
}
