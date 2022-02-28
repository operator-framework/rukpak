package controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/rukpak/api/v1alpha1"
)

// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=get;list;watch;create;update;patch

type BundleController struct {
	client.Client
}

func (b *BundleController) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	l := log.FromContext(ctx).WithValues("request", req)
	l.V(1).Info("reconciling bundle")

	in := &v1alpha1.Bundle{}
	if err := b.Get(ctx, req.NamespacedName, in); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	return reconcile.Result{}, nil
}

func (b *BundleController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// TODO: Filter down to Bundles that reference a ProvisionerClass that references ProvisionerID
		For(&v1alpha1.Bundle{}).
		Complete(b)
}
