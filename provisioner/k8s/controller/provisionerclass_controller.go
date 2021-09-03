package controller

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/operator-framework/rukpak/api/v1alpha1"
)

const (
	// ID is the rukpak provisioner's unique ID. Only ProvisionerClass(es) that specify
	// this unique ID will be managed by this provisioner controller.
	ID v1alpha1.ProvisionerID = "rukpak.io/k8s"
)

// +kubebuilder:rbac:groups=core.rukpak.io,resources=provisionerclasses,verbs=get;list;watch;create;update;patch;delete

type provisionerClassController struct {
	client.Client

	log logr.Logger
}

func (p *provisionerClassController) manageWith(mgr ctrl.Manager) error {
	// filter out events for any ProvisionerClass resources that don't specify
	// the unique k8s bundle provisioner ID.
	predicateProvisionerIDFilter := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pc, ok := obj.(*v1alpha1.ProvisionerClass)
		if !ok {
			return false
		}
		return pc.Spec.Provisioner == ID
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProvisionerClass{}, builder.WithPredicates(predicateProvisionerIDFilter)).
		Complete(p)
}

func (p *provisionerClassController) Reconcile(ctx context.Context, req ctrl.Request) (reconcile.Result, error) {
	// Set up a convenient log object so we don't have to type request over and over again
	log := p.log.WithValues("request", req)
	log.Info("reconciling provisionerclass")
	return reconcile.Result{}, nil
}
