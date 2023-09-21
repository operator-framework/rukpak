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
	"time"

	"github.com/go-logr/logr"
	"github.com/operator-framework/rukpak/api/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

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
	"k8s.io/apimachinery/pkg/types"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	crsource "sigs.k8s.io/controller-runtime/pkg/source"
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

	// Skip reconciling if `spec.paused` is set.
	if existingBD.Spec.Paused {
		log.Info("bundledeployment has been paused for reconciliation", "bundle deployment name", existingBD.Name)
		return ctrl.Result{}, nil
	}

	reconciledBD := existingBD.DeepCopy()
	res, reconcileErr := b.reconcile(ctx, reconciledBD)
	// Update the status subresource before updating the main object. This is
	// necessary because, in many cases, the main object update will remove the
	// finalizer, which will cause the core Kubernetes deletion logic to
	// complete. Therefore, we need to make the status update prior to the main
	// object update to ensure that the status update can be processed before
	// a potential deletion.
	// The controller is not updating spec, we only update the status. Hence sending
	// a status update should be enough.
	if !equality.Semantic.DeepEqual(existingBD.Status, reconciledBD.Status) {
		if updateErr := b.Status().Update(ctx, reconciledBD); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	return res, reconcileErr
}

func (b *bundleDeploymentReconciler) reconcile(ctx context.Context, bd *v1alpha2.BundleDeployment) (ctrl.Result, error) {
	// Unpack contents from the bundle deployment for each of the specified source and update
	// the status of the object.
	// TODO: In case of unpack pending and request is being requeued again indefinitely.
	bundleDepFS, res, err := b.unpackContents(ctx, bd)
	switch res.State {
	case v1alpha2source.StateUnpackPending:
		// Explicitely state that error is nil during phases when unpacking is preogressing.
		setUnpackStatusPending(&bd.Status.Conditions, fmt.Sprintf("pending unpack pod: err %v", err), bd.Generation)
		// Requeing after 5 sec for now since the average time to unpack an registry bundle locally
		// was around ~4sec. Also requeing the err, in case it exists.
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	case v1alpha2source.StateUnpacking:
		setUnpackStatusPending(&bd.Status.Conditions, fmt.Sprintf("unpacking pod: err %v", err), bd.Generation)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, err
	case v1alpha2source.StateUnpackFailed:
		setUnpackStatusFailing(&bd.Status.Conditions, fmt.Sprintf("unpacking failed: err %v", err), bd.Generation)
		return ctrl.Result{}, err
	case v1alpha2source.StateUnpacked:
		setUnpackStatusSuccess(&bd.Status.Conditions, fmt.Sprintf("unpacked %s", bd.GetName()), bd.Generation)
	default:
		return ctrl.Result{}, fmt.Errorf("unkown unpack state %q for bundle deployment %s: %v", res.State, bd.GetName(), bd.Generation)
	}

	// Unpacked contents from each source would now be availabe in the fs. Validate
	// if the contents together conform to the specified format.
	if err = b.validateContents(ctx, bd, bundleDepFS); err != nil {
		validateErr := fmt.Errorf("error validating contents for bundle %s with format %s: %v", bd.Name, bd.Spec.Format, err)
		setValidateFailing(&bd.Status.Conditions, validateErr.Error(), bd.Generation)
		return ctrl.Result{}, validateErr
	}
	setValidateSuccess(&bd.Status.Conditions, fmt.Sprintf("validate successful for bundle deployment %s", bd.GetName()), bd.Generation)

	// Deploy the validated contents onto the cluster.
	// The deployer should return the list of objects which have been deployed, so that
	// controller can be configured to set up watches for them.
	deployRes, err := b.deployContents(ctx, bd, bundleDepFS)
	switch deployRes.State {
	case v1alpha2deployer.StateIntallFailed:
		setInstallStatusFailed(&bd.Status.Conditions, err.Error(), bd.Generation)
		return ctrl.Result{}, err
	case v1alpha2deployer.StateUnpgradeFailed:
		setUnpackStatusFailing(&bd.Status.Conditions, err.Error(), bd.Generation)
		return ctrl.Result{}, err
	case v1alpha2deployer.StateReconcileFailed:
		setReconcileStatusFailed(&bd.Status.Conditions, err.Error(), bd.Generation)
		return ctrl.Result{}, err
	case v1alpha2deployer.StateObjectFetchFailed:
		setDynamicWatchFailed(&bd.Status.Conditions, err.Error(), bd.Generation)
		return ctrl.Result{}, err
	case v1alpha2deployer.StateDeploySuccessful:
		setInstallStatusSuccess(&bd.Status.Conditions, fmt.Sprintf("installed %s", bd.GetName()), bd.Generation)
	default:
		return ctrl.Result{}, fmt.Errorf("unkown deploy state %q for bundle deployment %s: %v", deployRes.State, bd.GetName(), bd.Generation)
	}

	for _, obj := range deployRes.AppliedObjects {
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
	return ctrl.Result{}, nil
}

// unpackContents unpacks contents from all the sources, and stores under a directory referenced by the bundle deployment name.
// It returns the consolidated state on whether contents from all the sources have been unpacked.
func (b *bundleDeploymentReconciler) unpackContents(ctx context.Context, bd *v1alpha2.BundleDeployment) (*afero.Fs, v1alpha2source.Result, error) {
	// set a base filesystem path and unpack contents under the root filepath defined by
	// bundledeployment name.
	bundleDepFs := afero.NewBasePathFs(afero.NewOsFs(), bd.GetName())

	errs := make([]error, 0)
	unpackResult := make([]v1alpha2source.Result, 0)

	// Unpack each of the sources individually, and consolidate all their results into one.
	for _, source := range bd.Spec.Sources {
		res, err := b.unpacker.Unpack(ctx, bd.Name, source, bundleDepFs, v1alpha2source.UnpackOption{
			BundleDeploymentUID: bd.GetUID(),
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("error unpacking from %s source: %q:  %v", source.Kind, res.Message, err))
		}
		unpackResult = append(unpackResult, *res)
	}

	// Even if one source has not unpacked, update Bundle Deployment status accordingly.
	// In this case the status will contain the result from the first source
	// which is still waiting to be unpacked.
	for _, res := range unpackResult {
		if res.State != v1alpha2source.StateUnpacked {
			return &bundleDepFs, res, apimacherrors.NewAggregate(errs)
		}
	}

	// TODO: capture the list of resolved sources for all the successful entry points.
	return &bundleDepFs, v1alpha2source.Result{State: v1alpha2source.StateUnpacked, Message: "Successfully unpacked"}, nil
}

// validateContents validates if the unpacked bundle contents are of the right format.
func (b *bundleDeploymentReconciler) validateContents(ctx context.Context, bd *v1alpha2.BundleDeployment, fs *afero.Fs) error {
	errs := make([]error, 0)
	for _, validator := range b.validators {
		if err := validator.Validate(ctx, *fs, *bd); err != nil {
			errs = append(errs, err)
		}
	}
	return apimacherrors.NewAggregate(errs)
}

// deployContents calls the registered deployer to apply the bundle contents onto the cluster.
func (b *bundleDeploymentReconciler) deployContents(ctx context.Context, bd *v1alpha2.BundleDeployment, fs *afero.Fs) (*v1alpha2deployer.Result, error) {
	return b.deployer.Deploy(ctx, *fs, bd)
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

func SetupWithManager(mgr manager.Manager, systemNsCache cache.Cache, opts ...Option) error {
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
	l := mgr.GetLogger().WithName(controllerName)
	controller, err := ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(&v1alpha2.BundleDeployment{}).
		Watches(crsource.NewKindWithCache(&corev1.Pod{}, systemNsCache), MapOwnerToBundleDeploymentHandler(context.Background(), mgr.GetClient(), l, &v1alpha2.BundleDeployment{})).
		Build(bd)
	if err != nil {
		return err
	}
	bd.controller = controller
	return nil
}

// MapOwnerToBundleDeploymentHandler is a handler implementation that finds an owner reference in the event object that
// references the provided owner. If a reference for the provided owner is found this handler enqueues a request for that owner to be reconciled.
func MapOwnerToBundleDeploymentHandler(ctx context.Context, cl client.Client, log logr.Logger, owner client.Object) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		ownerGVK, err := apiutil.GVKForObject(owner, cl.Scheme())
		if err != nil {
			log.Error(err, "map ownee to owner: lookup GVK for owner")
			return nil
		}
		type ownerInfo struct {
			key types.NamespacedName
			gvk schema.GroupVersionKind
		}
		var oi *ownerInfo

		for _, ref := range obj.GetOwnerReferences() {
			gv, err := schema.ParseGroupVersion(ref.APIVersion)
			if err != nil {
				log.Error(err, fmt.Sprintf("map ownee to owner: parse ownee's owner reference group version %q", ref.APIVersion))
				return nil
			}
			refGVK := gv.WithKind(ref.Kind)
			if refGVK == ownerGVK && ref.Controller != nil && *ref.Controller {
				oi = &ownerInfo{
					key: types.NamespacedName{Name: ref.Name},
					gvk: ownerGVK,
				}
				break
			}
		}
		if oi == nil {
			return nil
		}
		return []reconcile.Request{{NamespacedName: oi.key}}
	})
}
