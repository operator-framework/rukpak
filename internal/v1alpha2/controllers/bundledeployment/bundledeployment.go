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

package bundledeployment

import (
	"context"
	"sync"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
	"github.com/spf13/afero"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	unpackpath = "/var/cache/bundles"
)

// BundleDeploymentReconciler reconciles a BundleDeployment object
type bundleDeploymentReconciler struct {
	client.Client

	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	controller        crcontroller.Controller
	dynamicWatchMutex sync.RWMutex
	dynamicWatchGVKs  map[schema.GroupVersionKind]struct{}
}

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundledeployments,verbs=list;watch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundledeployments/status,verbs=update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundledeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (b *bundleDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	existingBD := &v1alpha2.BundleDeployment{}
	err := b.Get(ctx, req.NamespacedName, existingBD)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("bundledeployment resource not found. Ignoring since object must be deleted.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get bundledeployment")
		return ctrl.Result{}, err
	}

	// Skip reconciling if `spec.Paused` is set.
	if existingBD.Spec.Paused {
		log.Info("bundledeployment has been paused for reconciliation", "name", existingBD.Name)
		return ctrl.Result{}, nil
	}

	reconciledBD := existingBD.DeepCopy()
	res, reconcileErr := b.reconcile(ctx, reconciledBD)

	// The controller is not updating spec, we only update the status. Hence sending
	// a status update should be enough.
	if !equality.Semantic.DeepEqual(existingBD.Status, reconciledBD.Status) {
		if updateErr := b.Status().Update(ctx, reconciledBD); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	return res, reconcileErr
}

// reconcile reconciles the bundle deployment object by unpacking the contents specified in its spec.
// It further validates if the unpacked content conforms to the format specified in the bundledeployment.
// If the validation is successful, it further deploys the objects on cluster. If there is an error
// encountered in this process, an appropriate result is returned.
func (b *bundleDeploymentReconciler) reconcile(ctx context.Context, bundleDeployment *v1alpha2.BundleDeployment) (ctrl.Result, error) {

	// Establish a filesystem view on the local system, similar to 'chroot', with the root directory
	// determined by the bundle deployment name. All contents from various sources will be extracted and
	// placed within this specified root directory.
	_, err := store.NewBundleDeploymentStore(unpackpath, bundleDeployment.GetName(), afero.NewOsFs())
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
