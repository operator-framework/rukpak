package controller

import (
	"context"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/rukpak/api/v1alpha1"
)

// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=get;list;watch;create;update;patch

type bundleController struct {
	client.Client

	log logr.Logger
}

func (b *bundleController) manageWith(mgr ctrl.Manager) error {
	// Create multiple controllers for resource types that require automatic adoption
	return ctrl.NewControllerManagedBy(mgr).
		// TODO: Filter down to Bundles that reference a ProvisionerClass that references ProvisionerID
		For(&v1alpha1.Bundle{}).
		Complete(b)
}

func (b *bundleController) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	// Set up a convenient log object so we don't have to type request over and over again
	log := b.log.WithValues("request", req)
	log.V(1).Info("reconciling bundle")

	in := &v1alpha1.Bundle{}
	if err := b.Get(ctx, req.NamespacedName, in); err != nil {
		if apierrors.IsNotFound(err) {
			// If the instance is not found, we're likely reconciling because of a DELETE event.
			return reconcile.Result{}, nil
		}

		log.Error(err, "Error requesting Operator")
		return reconcile.Result{Requeue: true}, nil
	}

	return reconcile.Result{}, nil
}
