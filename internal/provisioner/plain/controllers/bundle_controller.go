package controllers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	crsource "sigs.k8s.io/controller-runtime/pkg/source"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/provisioner/common"
	plain "github.com/operator-framework/rukpak/internal/provisioner/plain/types"
	"github.com/operator-framework/rukpak/internal/util"
)

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=list;watch;update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/finalizers,verbs=update
//+kubebuilder:rbac:verbs=get,urls=/bundles/*
//+kubebuilder:rbac:groups=core,resources=pods,verbs=list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

type BundleReconciler struct {
	common.BundleReconciler
}

func (r *BundleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.BundleReconciler.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BundleReconciler) SetupWithManager(mgr manager.Manager, predicates ...predicate.Predicate) error {
	l := mgr.GetLogger().WithName("controller.bundle")

	// r.ProvisionerBundleReconciler = bundleReconciler
	r.ProvisionerID = plain.ProvisionerID
	r.ParseUnpackState = common.DefaultParseUnpackStateFunc

	return ctrl.NewControllerManagedBy(mgr).
		For(&rukpakv1alpha1.Bundle{}, builder.WithPredicates(
			append(predicates, util.BundleProvisionerFilter(r.ProvisionerID))...,
		)).
		// The default unpacker creates Pod's ownerref'd to its bundle, so
		// we need to watch pods to ensure we reconcile events coming from these
		// pods.
		Watches(&crsource.Kind{Type: &corev1.Pod{}}, util.MapOwneeToOwnerProvisionerHandler(context.Background(), mgr.GetClient(), l, plain.ProvisionerID, &rukpakv1alpha1.Bundle{})).
		Complete(r)
}
