package bundle

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crfinalizer "sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	crsource "sigs.k8s.io/controller-runtime/pkg/source"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/source"
	"github.com/operator-framework/rukpak/internal/storage"
	"github.com/operator-framework/rukpak/internal/util"
)

type Option func(*controller)

func WithHandler(h Handler) Option {
	return func(c *controller) {
		c.handler = h
	}
}

func WithProvisionerID(provisionerID string) Option {
	return func(c *controller) {
		c.provisionerID = provisionerID
	}
}

func WithStorage(s storage.Storage) Option {
	return func(c *controller) {
		c.storage = s
	}
}

func WithUnpacker(u source.Unpacker) Option {
	return func(c *controller) {
		c.unpacker = u
	}
}

func WithFinalizers(f crfinalizer.Finalizers) Option {
	return func(c *controller) {
		c.finalizers = f
	}
}

func SetupWithManager(mgr manager.Manager, systemNsCache cache.Cache, systemNamespace string, opts ...Option) error {
	c := &controller{
		cl: mgr.GetClient(),
	}

	for _, o := range opts {
		o(c)
	}

	c.setDefaults()

	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("invalid configuration: %v", err)
	}

	controllerName := fmt.Sprintf("controller.bundle.%s", c.provisionerID)
	l := mgr.GetLogger().WithName(controllerName)
	return ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(&rukpakv1alpha1.Bundle{}, builder.WithPredicates(
			util.BundleProvisionerFilter(c.provisionerID),
		)).
		// The default image source unpacker creates Pod's ownerref'd to its bundle, so
		// we need to watch pods to ensure we reconcile events coming from these
		// pods.
		Watches(crsource.NewKindWithCache(&corev1.Pod{}, systemNsCache), util.MapOwneeToOwnerProvisionerHandler(context.Background(), mgr.GetClient(), l, c.provisionerID, &rukpakv1alpha1.Bundle{})).
		Watches(crsource.NewKindWithCache(&corev1.ConfigMap{}, systemNsCache), util.MapConfigMapToBundlesHandler(context.Background(), mgr.GetClient(), systemNamespace, c.provisionerID)).
		Complete(c)
}

func (c *controller) setDefaults() {
	if c.handler == nil {
		c.handler = HandlerFunc(func(_ context.Context, fsys fs.FS, _ *rukpakv1alpha1.Bundle) (fs.FS, error) { return fsys, nil })
	}
}

func (c *controller) validateConfig() error {
	errs := []error{}
	if c.handler == nil {
		errs = append(errs, errors.New("converter is unset"))
	}
	if c.provisionerID == "" {
		errs = append(errs, errors.New("provisioner ID is unset"))
	}
	if c.unpacker == nil {
		errs = append(errs, errors.New("unpacker is unset"))
	}
	if c.storage == nil {
		errs = append(errs, errors.New("storage is unset"))
	}
	if c.finalizers == nil {
		errs = append(errs, errors.New("finalizer handler is unset"))
	}
	return apimacherrors.NewAggregate(errs)
}

type controller struct {
	handler       Handler
	provisionerID string

	cl         client.Client
	storage    storage.Storage
	finalizers crfinalizer.Finalizers
	unpacker   source.Unpacker
}

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=list;watch;update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/finalizers,verbs=update
//+kubebuilder:rbac:verbs=get,urls=/bundles/*;/uploads/*
//+kubebuilder:rbac:groups=core,resources=pods,verbs=list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=list;watch
//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (c *controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(1).Info("starting reconciliation")
	defer l.V(1).Info("ending reconciliation")
	existingBundle := &rukpakv1alpha1.Bundle{}
	if err := c.cl.Get(ctx, req.NamespacedName, existingBundle); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reconciledBundle := existingBundle.DeepCopy()
	res, reconcileErr := c.reconcile(ctx, reconciledBundle)

	// Update the status subresource before updating the main object. This is
	// necessary because, in many cases, the main object update will remove the
	// finalizer, which will cause the core Kubernetes deletion logic to
	// complete. Therefore, we need to make the status update prior to the main
	// object update to ensure that the status update can be processed before
	// a potential deletion.
	if !equality.Semantic.DeepEqual(existingBundle.Status, reconciledBundle.Status) {
		if updateErr := c.cl.Status().Update(ctx, reconciledBundle); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	existingBundle.Status, reconciledBundle.Status = rukpakv1alpha1.BundleStatus{}, rukpakv1alpha1.BundleStatus{}
	if !equality.Semantic.DeepEqual(existingBundle, reconciledBundle) {
		if updateErr := c.cl.Update(ctx, reconciledBundle); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	return res, reconcileErr
}

func (c *controller) reconcile(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (ctrl.Result, error) {
	bundle.Status.ObservedGeneration = bundle.Generation

	finalizedBundle := bundle.DeepCopy()
	finalizerResult, err := c.finalizers.Finalize(ctx, finalizedBundle)
	if err != nil {
		bundle.Status.ResolvedSource = nil
		bundle.Status.ContentURL = ""
		bundle.Status.Phase = rukpakv1alpha1.PhaseFailing
		meta.SetStatusCondition(&bundle.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionUnknown,
			Reason:  rukpakv1alpha1.ReasonProcessingFinalizerFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}
	if finalizerResult.Updated {
		// The only thing outside the status that should ever change when handling finalizers
		// is the list of finalizers in the object's metadata. In particular, we'd expect
		// finalizers to be added or removed.
		bundle.ObjectMeta.Finalizers = finalizedBundle.ObjectMeta.Finalizers
	}
	if finalizerResult.StatusUpdated {
		bundle.Status = finalizedBundle.Status
	}
	if finalizerResult.Updated || finalizerResult.StatusUpdated || !bundle.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	unpackResult, err := c.unpacker.Unpack(ctx, bundle)
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&bundle.Status, fmt.Errorf("source bundle content: %v", err))
	}
	switch unpackResult.State {
	case source.StatePending:
		updateStatusUnpackPending(&bundle.Status, unpackResult)
		return ctrl.Result{}, nil
	case source.StateUnpacking:
		updateStatusUnpacking(&bundle.Status, unpackResult)
		return ctrl.Result{}, nil
	case source.StateUnpacked:
		storeFS, err := c.handler.Handle(ctx, unpackResult.Bundle, bundle)
		if err != nil {
			return ctrl.Result{}, updateStatusUnpackFailing(&bundle.Status, err)
		}

		if err := c.storage.Store(ctx, bundle, storeFS); err != nil {
			return ctrl.Result{}, updateStatusUnpackFailing(&bundle.Status, fmt.Errorf("persist bundle content: %v", err))
		}

		contentURL, err := c.storage.URLFor(ctx, bundle)
		if err != nil {
			return ctrl.Result{}, updateStatusUnpackFailing(&bundle.Status, fmt.Errorf("get content URL: %v", err))
		}

		updateStatusUnpacked(&bundle.Status, unpackResult, contentURL)
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, updateStatusUnpackFailing(&bundle.Status, fmt.Errorf("unknown unpack state %q: %v", unpackResult.State, err))
	}
}

func updateStatusUnpackPending(status *rukpakv1alpha1.BundleStatus, result *source.Result) {
	status.ResolvedSource = nil
	status.ContentURL = ""
	status.Phase = rukpakv1alpha1.PhasePending
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha1.TypeUnpacked,
		Status:  metav1.ConditionFalse,
		Reason:  rukpakv1alpha1.ReasonUnpackPending,
		Message: result.Message,
	})
}

func updateStatusUnpacking(status *rukpakv1alpha1.BundleStatus, result *source.Result) {
	status.ResolvedSource = nil
	status.ContentURL = ""
	status.Phase = rukpakv1alpha1.PhaseUnpacking
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha1.TypeUnpacked,
		Status:  metav1.ConditionFalse,
		Reason:  rukpakv1alpha1.ReasonUnpacking,
		Message: result.Message,
	})
}

func updateStatusUnpacked(status *rukpakv1alpha1.BundleStatus, result *source.Result, contentURL string) {
	status.ResolvedSource = result.ResolvedSource
	status.ContentURL = contentURL
	status.Phase = rukpakv1alpha1.PhaseUnpacked
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha1.TypeUnpacked,
		Status:  metav1.ConditionTrue,
		Reason:  rukpakv1alpha1.ReasonUnpackSuccessful,
		Message: result.Message,
	})
}

func updateStatusUnpackFailing(status *rukpakv1alpha1.BundleStatus, err error) error {
	status.ResolvedSource = nil
	status.ContentURL = ""
	status.Phase = rukpakv1alpha1.PhaseFailing
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha1.TypeUnpacked,
		Status:  metav1.ConditionFalse,
		Reason:  rukpakv1alpha1.ReasonUnpackFailed,
		Message: err.Error(),
	})
	return err
}
