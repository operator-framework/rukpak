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
	corev1 "k8s.io/api/core/v1"
)

type SourceType string

const (
	SourceTypeImage      SourceType = "image"
	SourceTypeGit        SourceType = "git"
	SourceTypeConfigMaps SourceType = "configMaps"
	SourceTypeHTTP       SourceType = "http"

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

type BundleSource struct {
	// Type defines the kind of Bundle content being sourced.
	Type SourceType `json:"type"`
	// Image is the bundle image that backs the content of this bundle.
	Image *ImageSource `json:"image,omitempty"`
	// Git is the git repository that backs the content of this Bundle.
	Git *GitSource `json:"git,omitempty"`
	// ConfigMaps is a list of config map references and their relative
	// directory paths that represent a bundle filesystem.
	ConfigMaps []ConfigMapSource `json:"configMaps,omitempty"`
	//  HTTP is the remote location that backs the content of this Bundle.
	HTTP *HTTPSource `json:"http,omitempty"`
}

type ImageSource struct {
	// Ref contains the reference to a container image containing Bundle contents.
	Ref string `json:"ref"`
	// ImagePullSecretName contains the name of the image pull secret in the namespace that the provisioner is deployed.
	ImagePullSecretName string `json:"pullSecret,omitempty"`
}

type GitSource struct {
	// Repository is a URL link to the git repository containing the bundle.
	// Repository is required and the URL should be parsable by a standard git tool.
	Repository string `json:"repository"`
	// Directory refers to the location of the bundle within the git repository.
	// Directory is optional and if not set defaults to ./manifests.
	Directory string `json:"directory,omitempty"`
	// Ref configures the git source to clone a specific branch, tag, or commit
	// from the specified repo. Ref is required, and exactly one field within Ref
	// is required. Setting more than one field or zero fields will result in an
	// error.
	Ref GitRef `json:"ref"`
	// Auth configures the authorization method if necessary.
	Auth Authorization `json:"auth,omitempty"`
}

type ConfigMapSource struct {
	// ConfigMap is a reference to a configmap in the rukpak system namespace
	ConfigMap corev1.LocalObjectReference `json:"configMap"`
	// Path is the relative directory path within the bundle where the files
	// from the configmap will be present when the bundle is unpacked.
	Path string `json:"path,omitempty"`
}

type HTTPSource struct {
	// URL is where the bundle contents is.
	URL string `json:"url"`
	// Auth configures the authorization method if necessary.
	Auth Authorization `json:"auth,omitempty"`
}

type GitRef struct {
	// Branch refers to the branch to checkout from the repository.
	// The Branch should contain the bundle manifests in the specified directory.
	Branch string `json:"branch,omitempty"`
	// Tag refers to the tag to checkout from the repository.
	// The Tag should contain the bundle manifests in the specified directory.
	Tag string `json:"tag,omitempty"`
	// Commit refers to the commit to checkout from the repository.
	// The Commit should contain the bundle manifests in the specified directory.
	Commit string `json:"commit,omitempty"`
}

type Authorization struct {
	// Secret contains reference to the secret that has authorization information and is in the namespace that the provisioner is deployed.
	// The secret is expected to contain `data.username` and `data.password` for the username and password, respectively for http(s) scheme.
	// Refer to https://kubernetes.io/docs/concepts/configuration/secret/#basic-authentication-secret
	// For the ssh authorization of the GitSource, the secret is expected to contain `data.ssh-privatekey` and `data.ssh-knownhosts` for the ssh privatekey and the host entry in the known_hosts file respectively.
	// Refer to https://kubernetes.io/docs/concepts/configuration/secret/#ssh-authentication-secrets
	Secret corev1.LocalObjectReference `json:"secret,omitempty"`
	// InsecureSkipVerify controls whether a client verifies the server's certificate chain and host name. If InsecureSkipVerify
	// is true, the clone operation will accept any certificate presented by the server and any host name in that
	// certificate. In this mode, TLS is susceptible to machine-in-the-middle attacks unless custom verification is
	// used. This should be used only for testing.
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

type ProvisionerID string
