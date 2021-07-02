package provisioner

import (
	"context"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/rukpak/api/v1alpha1"
)

var (
	localSchemeBuilder = runtime.NewSchemeBuilder(
		kscheme.AddToScheme,
		v1alpha1.AddToScheme,
	)

	// AddToScheme adds all types necessary for the controller to operate.
	AddToScheme = localSchemeBuilder.AddToScheme
)

const (
	ID v1alpha1.ProvisionerID = "rukpack.io/k8s"
)

type Reconciler struct {
	client.Client

	log logr.Logger
}

// +kubebuilder:rbac:groups=core.rukpak.io,resources=provisionerclasses,verbs=create;update;patch;delete
// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=create;update;patch;delete
// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=update;patch
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch

// SetupWithManager adds the operator reconciler to the given controller manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create multiple controllers for resource types that require automatic adoption
	err := ctrl.NewControllerManagedBy(mgr).
		// TODO: Filter down to bundles that reference a ProvisionerClass that references ID
		For(&v1alpha1.Bundle{}).
		Complete(reconcile.Func(r.ReconcileBundle))
	if err != nil {
		return err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		// TODO: Filter down to provisionerclass.spec.id = ID
		For(&v1alpha1.ProvisionerClass{}).
		Complete(reconcile.Func(r.ReconcileProvisionerClass))
	if err != nil {
		return err
	}

	return nil
}

// NewReconciler constructs and returns an BundleReconciler.
// As a side effect, the given scheme has operator discovery types added to it.
func NewReconciler(cli client.Client, log logr.Logger, scheme *runtime.Scheme) (*Reconciler, error) {
	// Add watched types to scheme.
	if err := AddToScheme(scheme); err != nil {
		return nil, err
	}

	return &Reconciler{
		Client: cli,

		log: log,
	}, nil
}

func (r *Reconciler) ReconcileBundle(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	// Set up a convenient log object so we don't have to type request over and over again
	log := r.log.WithValues("request", req)
	log.V(1).Info("reconciling bundle")

	in := &v1alpha1.Bundle{}
	if err := r.Get(ctx, req.NamespacedName, in); err != nil {
		if apierrors.IsNotFound(err) {
			// If the Operator instance is not found, we're likely reconciling because
			// of a DELETE event.
			return reconcile.Result{}, nil
		} else {
			log.Error(err, "Error requesting Operator")
			return reconcile.Result{Requeue: true}, nil
		}
	}

	// Want:
	// Content oracle -- Content(bundle.Spec.Source) io.Reader

	// Open Question:
	// 1. Check bundle state
	// - look for unpack results
	// - is there anything missing
	// - does the digest match
	// 2. Unpack bundle (if not) -- Asynchronous (maybe a Job)?
	// - take the bundlesource
	// - fire job based on type to unpack
	// 3. Update status

	return reconcile.Result{}, nil
}

func (r *Reconciler) ReconcileProvisionerClass(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	// Set up a convenient log object so we don't have to type request over and over again
	log := r.log.WithValues("request", req)
	log.V(1).Info("reconciling provisionerclass")
	// TODO
	return reconcile.Result{}, nil
}
