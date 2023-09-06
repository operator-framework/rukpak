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
	"errors"
	"fmt"
	"sync"

	"github.com/operator-framework/rukpak/api/v1alpha2"

	v1alpha2deployer "github.com/operator-framework/rukpak/internal/controllers/v1alpha2/deployer"
	v1alpha2source "github.com/operator-framework/rukpak/internal/controllers/v1alpha2/source"
	v1alpha2validators "github.com/operator-framework/rukpak/internal/controllers/v1alpha2/validator"
	helmpredicate "github.com/operator-framework/rukpak/internal/helm-operator-plugins/predicate"
	"github.com/spf13/afero"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// BundleDeploymentReconciler reconciles a BundleDeployment object
type bundleDeploymentReconciler struct {
	client.Client

	unpacker   v1alpha2source.Unpacker
	validators []v1alpha2validators.Validator
	deployer   v1alpha2deployer.Deployer

	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	controller        crcontroller.Controller
	dynamicWatchMutex sync.RWMutex
	dynamicWatchGVKs  map[schema.GroupVersionKind]struct{}
}

type Option func(bd *bundleDeploymentReconciler)

func WithUnpacker(u v1alpha2source.Unpacker) Option {
	return func(bd *bundleDeploymentReconciler) {
		bd.unpacker = u
	}
}

func WithValidators(u ...v1alpha2validators.Validator) Option {
	return func(bd *bundleDeploymentReconciler) {
		bd.validators = u
	}
}

func WithDeployer(u v1alpha2deployer.Deployer) Option {
	return func(bd *bundleDeploymentReconciler) {
		bd.deployer = u
	}
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
	fmt.Println("reconciling")
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

	reconciledBD := existingBD.DeepCopy()
	res, reconcileErr := b.reconcile(ctx, reconciledBD)
	// Update the status subresource before updating the main object. This is
	// necessary because, in many cases, the main object update will remove the
	// finalizer, which will cause the core Kubernetes deletion logic to
	// complete. Therefore, we need to make the status update prior to the main
	// object update to ensure that the status update can be processed before
	// a potential deletion.
	if !equality.Semantic.DeepEqual(existingBD.Status, reconciledBD.Status) {
		if updateErr := b.Status().Update(ctx, reconciledBD); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	existingBD.Status, reconciledBD.Status = v1alpha2.BundleDeploymentStatus{}, v1alpha2.BundleDeploymentStatus{}
	if !equality.Semantic.DeepEqual(existingBD, reconciledBD) {
		if updateErr := b.Update(ctx, reconciledBD); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	return res, reconcileErr
}

func (b *bundleDeploymentReconciler) reconcile(ctx context.Context, bd *v1alpha2.BundleDeployment) (ctrl.Result, error) {
	bundleDepFS, err := b.unpackContents(ctx, bd)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("error unpacking contents: %v", err)
	}

	if err = b.validateContents(ctx, bd, bundleDepFS); err != nil {
		return ctrl.Result{}, fmt.Errorf("error validating contents for bundle %s with format %s: %v", bd.Name, bd.Spec.Format, err)
	}

	var deployedObjects []client.Object
	if deployedObjects, err = b.deployContents(ctx, bd, bundleDepFS); err != nil {
		return ctrl.Result{}, fmt.Errorf("error deploying contents: %v", err)
	}

	for _, obj := range deployedObjects {
		uMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			setDynamicWatchFailed(&bd.Status.Conditions, err.Error(), bd.Generation)
			return ctrl.Result{}, err
		}

		unstructuredObj := &unstructured.Unstructured{Object: uMap}
		if err := func() error {
			b.dynamicWatchMutex.Lock()
			defer b.dynamicWatchMutex.Unlock()

			_, isWatched := b.dynamicWatchGVKs[unstructuredObj.GroupVersionKind()]
			if !isWatched {
				if err := b.controller.Watch(
					&source.Kind{Type: unstructuredObj},
					&handler.EnqueueRequestForOwner{OwnerType: bd, IsController: true},
					helmpredicate.DependentPredicateFuncs()); err != nil {
					return err
				}
				b.dynamicWatchGVKs[unstructuredObj.GroupVersionKind()] = struct{}{}
			}
			return nil
		}(); err != nil {
			setDynamicWatchFailed(&bd.Status.Conditions, err.Error(), bd.Generation)
			return ctrl.Result{}, err
		}
	}

	fmt.Println("deployed contents successfully")

	return ctrl.Result{}, nil
}

// unpackContents unpacks contents from all the sources, and stores under a directory referenced by the bundle deployment name.
func (b *bundleDeploymentReconciler) unpackContents(ctx context.Context, bd *v1alpha2.BundleDeployment) (*afero.Fs, error) {
	// add status to mention that the contents are being unpacked.
	setUnpackStatusPending(&bd.Status.Conditions, fmt.Sprintf("unpacking bundledeployment %q", bd.GetName()), bd.Generation)

	// set a base filesystem path and unpack contents under the root filepath defined by
	// bundledeployment name.
	bundleDepFs := afero.NewBasePathFs(afero.NewOsFs(), bd.GetName())
	errs := make([]error, 0)

	for _, source := range bd.Spec.Sources {
		res, err := b.unpacker.Unpack(ctx, bd.Name, &source, bundleDepFs)
		if err != nil {
			errs = append(errs, fmt.Errorf("error unpacking from %s source: %q:  %v", source.Kind, res.Message, err))
		}
	}

	if len(errs) != 0 {
		setUnpackStatusFailing(&bd.Status.Conditions, fmt.Sprintf("unpacking failure %q", bd.GetName()), bd.Generation)
	}

	setUnpackStatusSuccess(&bd.Status.Conditions, fmt.Sprintf("unpacking successful %q", bd.GetName()), bd.Generation)
	return &bundleDepFs, apimacherrors.NewAggregate(errs)
}

// validateContents validates if the unpacked bundle contents are of the right format.
func (b *bundleDeploymentReconciler) validateContents(ctx context.Context, bd *v1alpha2.BundleDeployment, fs *afero.Fs) error {
	setValidatePending(&bd.Status.Conditions, fmt.Sprintf("validating bundledeployment %q", bd.GetName()), bd.Generation)

	errs := make([]error, 0)
	for _, validator := range b.validators {
		if err := validator.Validate(ctx, *fs, bd); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) != 0 {
		setValidateFailing(&bd.Status.Conditions, fmt.Sprintf("validating failure %q", bd.GetName()), bd.Generation)
	}

	setValidateSuccess(&bd.Status.Conditions, fmt.Sprintf("validating successful %q", bd.GetName()), bd.Generation)
	return apimacherrors.NewAggregate(errs)
}

func (b *bundleDeploymentReconciler) deployContents(ctx context.Context, bd *v1alpha2.BundleDeployment, fs *afero.Fs) ([]client.Object, error) {
	deployedObjects, err := b.deployer.Deploy(ctx, *fs, bd)
	if err != nil {
		return nil, fmt.Errorf("error deploying contents: %v", err)
	}
	return deployedObjects, nil
}

func (b *bundleDeploymentReconciler) validateConfig() error {
	errs := []error{}
	if b.unpacker == nil {
		errs = append(errs, errors.New("unpacker is unset"))
	}
	if b.validators == nil || len(b.validators) == 0 {
		errs = append(errs, errors.New("validators not provided"))
	}
	if b.deployer == nil {
		errs = append(errs, errors.New("deployer is unset"))
	}
	return utilerrors.NewAggregate(errs)
}

func SetupWithManager(mgr manager.Manager, opts ...Option) error {
	bd := &bundleDeploymentReconciler{
		Client:           mgr.GetClient(),
		dynamicWatchGVKs: map[schema.GroupVersionKind]struct{}{},
	}
	for _, o := range opts {
		o(bd)
	}

	if err := bd.validateConfig(); err != nil {
		return fmt.Errorf("invalid configuration: %v", err)
	}

	controllerName := fmt.Sprintf("controller-bundledeployment.%s", v1alpha2.BundleDeploymentGVK.Version)
	controller, err := ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(&v1alpha2.BundleDeployment{}).
		Build(bd)
	if err != nil {
		return err
	}
	bd.controller = controller
	return nil
}
