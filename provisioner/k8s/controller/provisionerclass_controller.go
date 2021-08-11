package controller

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/rukpak/api/v1alpha1"
)

// +kubebuilder:rbac:groups=core.rukpak.io,resources=provisionerclasses,verbs=get;list;watch;create;update;patch;delete

type provisionerClassController struct {
	client.Client

	log logr.Logger
}

func (p *provisionerClassController) manageWith(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// TODO: Filter down to ProvisionerClasses that reference ProvisionerID
		For(&v1alpha1.ProvisionerClass{}).
		Complete(p)
}

func (p *provisionerClassController) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	// Set up a convenient log object so we don't have to type request over and over again
	log := p.log.WithValues("request", req)
	log.V(1).Info("reconciling provisionerclass")
	return reconcile.Result{}, nil
}
