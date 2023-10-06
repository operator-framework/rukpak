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

package source

import (
	"context"
	"errors"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// UnpackOptions stores bundle deployment specific options
// that are passed to the unpacker.
// This is currently used to pass the bundle deployment UID
// for the use in image unpacker. But can further be expanded
// to pass other bundle specific options.
type UnpackOption struct {
	BundleDeploymentUID types.UID
}

// Unpacker unpacks bundle content, either synchronously or asynchronously and
// returns a Result, which conveys information about the progress of unpacking
// the bundle content.
//
// NOTE: A source is meant to be agnostic to specific bundle formats and
// specifications. A source should treat a bundle root directory as an opaque
// file tree and delegate bundle format concerns to bundle parsers.
type Unpacker interface {
	// Unpack unpacks the bundle content. Unpack should not mutate the bundle deployment source being passed.
	Unpack(ctx context.Context, bundledeploymentSource *v1alpha2.BundleDeplopymentSource, store store.Store, opts UnpackOption) (*Result, error)
}

// Result conveys progress information about unpacking bundle content.
type Result struct {
	// ResolvedSource is a reproducible view of a Bundle's Source.
	// When possible, source implementations should return a ResolvedSource
	// that pins the Source such that future fetches of the bundle content can
	// be guaranteed to fetch the exact same bundle content as the original
	// unpack.
	//
	// For example, resolved image sources should reference a container image
	// digest rather than an image tag, and git sources should reference a
	// commit hash rather than a branch or tag.
	ResolvedSource *v1alpha2.BundleDeplopymentSource

	// State is the current state of unpacking the bundle content.
	State State

	// Message is contextual information about the progress of unpacking the
	// bundle content.
	Message string
}

type State string

const (
	// StatePending conveys that a request for unpacking a bundle has been
	// acknowledged, but not yet started.
	StateUnpackPending State = "Pending"

	// StateUnpacking conveys that the source is currently unpacking a bundle.
	// This state should be used when the bundle contents are being downloaded
	// and processed.
	StateUnpacking State = "Unpacking"

	// StateUnpacked conveys that the bundle has been successfully unpacked.
	StateUnpacked State = "Unpacked"

	// StateUnpackFailed conveys that the unpacking of the bundle has failed.
	StateUnpackFailed State = "Unpack failed"
)

type defaultUnpacker struct {
	systemNsCluster cluster.Cluster
	namespace       string
	unpackImage     string
}

// sourceKind refers to the kind of source being
// used to unpack contents.
type sourceKind string

const (
	sourceTypeImage sourceKind = "image"
)

type UnpackerOption func(*defaultUnpacker)

func WithUnpackImage(image string) UnpackerOption {
	return func(du *defaultUnpacker) {
		du.unpackImage = image
	}
}

type unpacker struct {
	sources map[sourceKind]Unpacker
}

// NewUnpacker returns a new composite Source that unpacks bundles using the source
// mapping provided by the configured sources.
func NewUnpacker(sources map[sourceKind]Unpacker) Unpacker {
	return &unpacker{sources: sources}
}

// Unpack itrates over the sources specified in bundleDeployment object. Unpacking is done
// for each specified source, the bundle contents are stored in the specified destination.
func (u *unpacker) Unpack(ctx context.Context, bundledeploymentSource *v1alpha2.BundleDeplopymentSource, store store.Store, opts UnpackOption) (*Result, error) {
	if bundledeploymentSource == nil {
		return nil, errors.New("bundledeployment source emtpy.")
	}

	if store == nil {
		return nil, errors.New("file system to unpack contents empty.")
	}

	// TODO: aggregate the result for multiple resolved resources when other source
	// types are added.
	if bundledeploymentSource.Image != nil {
		return u.sources[sourceTypeImage].Unpack(ctx, bundledeploymentSource, store, opts)
	}

	return nil, errors.New("unable to unpack.")
}

// NewDefaultUnpackerWithOpts returns the default unpacker, configured to unpack contents from in-built source types.
func NewDefaultUnpackerWithOpts(systemNsCluster cluster.Cluster, namespace string, opts ...UnpackerOption) (Unpacker, error) {
	unpacker := &defaultUnpacker{
		systemNsCluster: systemNsCluster,
		namespace:       namespace,
	}
	for _, opt := range opts {
		opt(unpacker)
	}
	return unpacker.initialize()
}

func (u *defaultUnpacker) initialize() (Unpacker, error) {
	if u.systemNsCluster == nil {
		return nil, errors.New("systemNsCluster cannot be empty, cannot initialize")
	}

	cfg := u.systemNsCluster.GetConfig()
	kubeclient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return NewUnpacker(map[sourceKind]Unpacker{
		sourceTypeImage: &image{
			Client:       u.systemNsCluster.GetClient(),
			KubeClient:   kubeclient,
			PodNamespace: u.namespace,
			UnpackImage:  u.unpackImage,
		},
	}), nil
}
