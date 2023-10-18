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

package deployer

import (
	"context"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
)

// Deployer knows how apply objects on cluster.
type Deployer interface {
	// Deploy deploys the contents from fs onto the cluster and returns list of the deployed object.
	// Pass the clients specific to deployer with Options. Refer to Helm deployer for example.
	Deploy(ctx context.Context, store store.Store, bundleDeployment *v1alpha2.BundleDeployment) (*Result, error)
}

// Result conveys the progress information about deploying content.
// TODO: Refactor to use the same result struct for unpacking and deployment.
type Result struct {
	// State is the current state of deploying content on cluster.
	State State
	// AppliedObjects returns the list objects applied on cluster.
	AppliedObjects []client.Object
}

type State string

const (
	// StateInstallFailed indicates if the installation failed.
	StateIntallFailed State = "Install failed"

	// StateUnpgradeFailed indicates if the upgrade failed.
	StateUnpgradeFailed State = "Upgrade failed"

	// StateReconcileFailed indicates if the reconcile failed
	// in case the applied objects on the cluster need to be
	// patched.
	StateReconcileFailed State = "Reconcile failed"

	// StateObjectFetchFailed indicates if there was an error
	// while fetching the list of objects applied on cluster.
	StateObjectFetchFailed State = "Fetching list of applied objects failed"

	// StateDeploySuccessful indicates that the bundle
	// contents have been successfully applied on the cluster.
	StateDeploySuccessful State = "Deploy failed"
)
