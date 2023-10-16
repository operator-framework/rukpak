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
	helmpredicate "github.com/operator-framework/rukpak/internal/helm-operator-plugins/predicate"
	"github.com/operator-framework/rukpak/internal/v1alpha2/deployer"
	"github.com/operator-framework/rukpak/internal/v1alpha2/source"
	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
	"github.com/operator-framework/rukpak/internal/v1alpha2/validator"
	"github.com/spf13/afero"
	corev1 "k8s.io/api/core/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	crsource "sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	unpackpath = "/var/cache/bundles"
)

// BundleDeploymentReconciler reconciles a BundleDeployment object
type bundleDeploymentReconciler struct {
	client.Client

	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	controller crcontroller.Controller

	// unpacker knows how to unpack and store the contents locally
	// on filesystem for the bundle types which would be handled
	// by this reconciler.
	unpacker source.Unpacker
	// validators have specific rules defined, based on which
	// they validate the unpacked content.
	// Accepting validators as a list, in case custom validators
	// are needed to be added in future.
	validators []validator.Validator
	// deployer knows how to apply bundle objects into cluster.
	deployer deployer.Deployer

	dynamicWatchMutex sync.RWMutex
	dynamicWatchGVKs  map[schema.GroupVersionKind]struct{}
}

// Options to configure bundleDeploymentReconciler
type Option func(bd *bundleDeploymentReconciler)

func WithUnpacker(u *source.Unpacker) Option {
	return func(bd *bundleDeploymentReconciler) {
		bd.unpacker = *u
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
	err := b.Client.Get(ctx, req.NamespacedName, existingBD)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// TODO: if bundledeployment is deleted, remove the unpacked bundle present locally.
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

	// Establish a filesystem view on the local system, similar to 'chroot', with the root directory
	// determined by the bundle deployment name. All contents from various sources will be extracted and
	// placed within this specified root directory.
	bundledeploymentStore, err := store.NewBundleDeploymentStore(unpackpath, reconciledBD.GetName(), afero.NewOsFs())
	if err != nil {
		return ctrl.Result{}, err
	}

	res, reconcileErr := b.reconcile(ctx, reconciledBD, bundledeploymentStore)

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
func (b *bundleDeploymentReconciler) reconcile(ctx context.Context, bundleDeployment *v1alpha2.BundleDeployment, bundledeploymentStore store.Store) (ctrl.Result, error) {

	res, err := b.unpackContents(ctx, bundleDeployment, bundledeploymentStore)
	// result can be nil, when there is an error during unpacking. This indicates
	// that unpacking was failed. Update the status accordingly.
	if res == nil || err != nil {
		setUnpackStatusFailing(&bundleDeployment.Status.Conditions, fmt.Sprintf("unpack unsuccessful %v", err), bundleDeployment.Generation)
		return ctrl.Result{}, err
	}

	switch res.State {
	case source.StateUnpackPending:
		setUnpackStatusPending(&bundleDeployment.Status.Conditions, fmt.Sprintf("pending unpack"), bundleDeployment.Generation)
		// Requeing after 5 sec for now since the average time to unpack an registry bundle locally
		// was around ~4sec.
		// Warning: This could end up requeing indefinitely, till an error has occured.
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	case source.StateUnpacking:
		setUnpackStatusPending(&bundleDeployment.Status.Conditions, fmt.Sprintf("unpacking in progress"), bundleDeployment.Generation)
	case source.StateUnpacked:
		setUnpackStatusSuccessful(&bundleDeployment.Status.Conditions, fmt.Sprintf("unpacked successfully"), bundleDeployment.Generation)
	default:
		return ctrl.Result{}, fmt.Errorf("unkown unpack state %q for bundle deployment %s: %v", res.State, bundleDeployment.GetName(), bundleDeployment.Generation)
	}

	// Unpacked contents from each source would now be availabe in the fs. Validate
	// if the contents together conform to the specified format.
	if err = b.validateContents(ctx, bundleDeployment.Spec.Format, bundledeploymentStore); err != nil {
		validateErr := fmt.Errorf("validating contents for bundle %s with format %s: %v", bundleDeployment.Name, bundleDeployment.Spec.Format, err)
		setValidateStatusFailing(&bundleDeployment.Status.Conditions, validateErr.Error(), bundleDeployment.Generation)
		return ctrl.Result{}, validateErr
	}
	setValidateStatusSuccess(&bundleDeployment.Status.Conditions, fmt.Sprintf("unpacked successfully"), bundleDeployment.Generation)

	// Deploy the validated contents onto the cluster.
	// The deployer should return the list of objects which have been deployed, so that
	// controller can be configured to set up watches for them.
	deployResult, err := b.deployContents(ctx, bundledeploymentStore, bundleDeployment)
	switch deployResult.State {
	case deployer.StateIntallFailed:
		setInstallStatusFailed(&bundleDeployment.Status.Conditions, err.Error(), bundleDeployment.Generation)
		return ctrl.Result{}, err
	case deployer.StateUnpgradeFailed:
		setUnpgradeStatusFailed(&bundleDeployment.Status.Conditions, err.Error(), bundleDeployment.Generation)
		return ctrl.Result{}, err
	case deployer.StateReconcileFailed:
		setReconcileStatusFailed(&bundleDeployment.Status.Conditions, err.Error(), bundleDeployment.Generation)
		return ctrl.Result{}, err
	case deployer.StateObjectFetchFailed:
		setDynamicWatchFailed(&bundleDeployment.Status.Conditions, err.Error(), bundleDeployment.Generation)
		return ctrl.Result{}, err
	case deployer.StateDeploySuccessful:
		setInstallStatusSuccess(&bundleDeployment.Status.Conditions, fmt.Sprintf("installed %s", bundleDeployment.GetName()), bundleDeployment.Generation)
	default:
		return ctrl.Result{}, fmt.Errorf("unkown deploy state %q for bundle deployment %s: %v", deployResult.State, bundleDeployment.GetName(), bundleDeployment.Generation)
	}

	// for the objects returned from the deployer, set watches on them.
	// TODO(brainstorm): any event coming from the dependent object will trigger the entire reconcile,
	// making it to unpack again. Introduce a caching mechanism to skip unpacking when the source has not
	// changed.
	for _, obj := range deployResult.AppliedObjects {
		uMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			setDynamicWatchFailed(&bundleDeployment.Status.Conditions, err.Error(), bundleDeployment.Generation)
			return ctrl.Result{}, err
		}

		unstructuredObj := &unstructured.Unstructured{Object: uMap}
		if err := func() error {
			b.dynamicWatchMutex.Lock()
			defer b.dynamicWatchMutex.Unlock()

			_, isWatched := b.dynamicWatchGVKs[unstructuredObj.GroupVersionKind()]
			if !isWatched {
				if err := b.controller.Watch(
					&crsource.Kind{Type: unstructuredObj},
					&handler.EnqueueRequestForOwner{OwnerType: bundleDeployment, IsController: true},
					helmpredicate.DependentPredicateFuncs()); err != nil {
					return err
				}
				b.dynamicWatchGVKs[unstructuredObj.GroupVersionKind()] = struct{}{}
			}
			return nil
		}(); err != nil {
			setDynamicWatchFailed(&bundleDeployment.Status.Conditions, err.Error(), bundleDeployment.Generation)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// unpackContents unpacks contents from all the sources, and stores under a directory referenced by the bundle deployment name.
// It returns the consolidated state on whether contents from all the sources have been unpacked.
func (b *bundleDeploymentReconciler) unpackContents(ctx context.Context, bundledeployment *v1alpha2.BundleDeployment, store store.Store) (*source.Result, error) {
	unpackResult := make([]source.Result, 0)

	// unpack each of the sources individually, and consolidate all their results into one.
	for _, src := range bundledeployment.Spec.Sources {
		res, err := b.unpacker.Unpack(ctx, &src, store, source.UnpackOption{
			BundleDeploymentUID: bundledeployment.UID,
		})
		if err != nil {
			return nil, err
		}
		unpackResult = append(unpackResult, *res)
	}
	// Even if one source has not unpacked, update Bundle Deployment status accordingly.
	// In this case the status will contain the result from the first source
	// which is still waiting to be unpacked.
	for _, res := range unpackResult {
		if res.State != source.StateUnpacked {
			return &res, nil
		}
	}
	// TODO: capture the list of resolved sources for all the successful entry points.
	return &source.Result{State: source.StateUnpacked, Message: "Successfully unpacked"}, nil
}

// validateContents validates if the unpacked bundle contents are of the right format.
func (b *bundleDeploymentReconciler) validateContents(ctx context.Context, format v1alpha2.FormatType, store store.Store) error {
	errs := make([]error, 0)
	for _, validator := range b.validators {
		if err := validator.Validate(ctx, format, store); err != nil {
			errs = append(errs, err)
		}
	}
	return apimacherrors.NewAggregate(errs)
}

// deployContents calls the registered deployer to apply the bundle contents onto the cluster.
func (b *bundleDeploymentReconciler) deployContents(ctx context.Context, store store.Store, bd *v1alpha2.BundleDeployment) (*deployer.Result, error) {
	return b.deployer.Deploy(ctx, store, bd)
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
