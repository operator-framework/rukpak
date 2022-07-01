package controllers

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	crsource "sigs.k8s.io/controller-runtime/pkg/source"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/convert"
	"github.com/operator-framework/rukpak/internal/provisioner/common"
	registry "github.com/operator-framework/rukpak/internal/provisioner/registry/types"
	"github.com/operator-framework/rukpak/internal/source"
	"github.com/operator-framework/rukpak/internal/storage"
	updater "github.com/operator-framework/rukpak/internal/updater/bundle"
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

func registryParseUnpackState(ctx context.Context, unpackResult *source.Result, u *updater.Updater, bundle *rukpakv1alpha1.Bundle, s storage.Storage) (ctrl.Result, error) {
	switch unpackResult.State {
	case source.StatePending:
		common.UpdateStatusUnpackPending(u, unpackResult)
		return ctrl.Result{}, nil
	case source.StateUnpacking:
		common.UpdateStatusUnpacking(u, unpackResult)
		return ctrl.Result{}, nil
	case source.StateUnpacked:
		plainFS, err := convert.RegistryV1ToPlain(unpackResult.Bundle)
		if err != nil {
			return ctrl.Result{}, common.UpdateStatusUnpackFailing(u, fmt.Errorf("convert registry+v1 bundle to plain+v0 bundle: %v", err))
		}

		objects, err := common.GetObjects(plainFS)
		if err != nil {
			return ctrl.Result{}, common.UpdateStatusUnpackFailing(u, fmt.Errorf("get objects from bundle manifests: %v", err))
		}
		if len(objects) == 0 {
			return ctrl.Result{}, common.UpdateStatusUnpackFailing(u, errors.New("invalid bundle: found zero objects"))
		}

		if err := s.Store(ctx, bundle, plainFS); err != nil {
			return ctrl.Result{}, common.UpdateStatusUnpackFailing(u, fmt.Errorf("persist bundle objects: %v", err))
		}

		contentURL, err := s.URLFor(ctx, bundle)
		if err != nil {
			return ctrl.Result{}, common.UpdateStatusUnpackFailing(u, fmt.Errorf("get content URL: %v", err))
		}

		common.UpdateStatusUnpacked(u, unpackResult, contentURL)
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, common.UpdateStatusUnpackFailing(u, fmt.Errorf("unknown unpack state %q", unpackResult.State))
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *BundleReconciler) SetupWithManager(mgr manager.Manager, predicates ...predicate.Predicate) error {
	l := mgr.GetLogger().WithName("controller.bundle")

	r.ProvisionerID = registry.ProvisionerID
	r.ParseUnpackState = registryParseUnpackState

	return ctrl.NewControllerManagedBy(mgr).
		For(&rukpakv1alpha1.Bundle{}, builder.WithPredicates(
			append(predicates, util.BundleProvisionerFilter(r.ProvisionerID))...,
		)).
		// The default unpacker creates Pod's ownerref'd to its bundle, so
		// we need to watch pods to ensure we reconcile events coming from these
		// pods.
		Watches(&crsource.Kind{Type: &corev1.Pod{}}, util.MapOwneeToOwnerProvisionerHandler(context.Background(), mgr.GetClient(), l, registry.ProvisionerID, &rukpakv1alpha1.Bundle{})).
		Complete(r)
}
