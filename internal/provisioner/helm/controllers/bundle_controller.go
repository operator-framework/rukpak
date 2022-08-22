/*
Copyright 2021.

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

package controllers

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	helm "github.com/operator-framework/rukpak/internal/provisioner/helm/types"
	"github.com/operator-framework/rukpak/internal/source"
	"github.com/operator-framework/rukpak/internal/storage"
	updater "github.com/operator-framework/rukpak/internal/updater/bundle"
	"github.com/operator-framework/rukpak/internal/util"
)

// BundleReconciler reconciles a Bundle object
type BundleReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	Storage    storage.Storage
	Finalizers finalizer.Finalizers
	Unpacker   source.Unpacker
}

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles,verbs=list;watch;update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/status,verbs=update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundles/finalizers,verbs=update
//+kubebuilder:rbac:verbs=get,urls=/bundles/*;/uploads/*
//+kubebuilder:rbac:groups=core,resources=pods,verbs=list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *BundleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(1).Info("starting reconciliation")
	defer l.V(1).Info("ending reconciliation")
	bundle := &rukpakv1alpha1.Bundle{}
	if err := r.Get(ctx, req.NamespacedName, bundle); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	u := updater.NewBundleUpdater(r.Client)
	defer func() {
		if err := u.Apply(ctx, bundle); err != nil {
			l.Error(err, "failed to update status")
		}
	}()
	u.UpdateStatus(updater.EnsureObservedGeneration(bundle.Generation))

	finalizerResult, err := r.Finalizers.Finalize(ctx, bundle)
	if err != nil {
		u.UpdateStatus(
			updater.EnsureResolvedSource(nil),
			updater.EnsureContentURL(""),
			updater.SetPhase(rukpakv1alpha1.PhaseFailing),
			updater.EnsureCondition(metav1.Condition{
				Type:    rukpakv1alpha1.TypeUnpacked,
				Status:  metav1.ConditionUnknown,
				Reason:  rukpakv1alpha1.ReasonProcessingFinalizerFailed,
				Message: err.Error(),
			}),
		)
		return ctrl.Result{}, err
	}
	var (
		finalizerUpdateErrs []error
	)
	// Update the status subresource before updating the main object. This is
	// necessary because, in many cases, the main object update will remove the
	// finalizer, which will cause the core Kubernetes deletion logic to
	// complete. Therefore, we need to make the status update prior to the main
	// object update to ensure that the status update can be processed before
	// a potential deletion.
	if finalizerResult.StatusUpdated {
		finalizerUpdateErrs = append(finalizerUpdateErrs, r.Status().Update(ctx, bundle))
	}
	if finalizerResult.Updated {
		finalizerUpdateErrs = append(finalizerUpdateErrs, r.Update(ctx, bundle))
	}
	if finalizerResult.Updated || finalizerResult.StatusUpdated || !bundle.GetDeletionTimestamp().IsZero() {
		err := apimacherrors.NewAggregate(finalizerUpdateErrs)
		if err != nil {
			u.UpdateStatus(
				updater.EnsureResolvedSource(nil),
				updater.EnsureContentURL(""),
				updater.SetPhase(rukpakv1alpha1.PhaseFailing),
				updater.EnsureCondition(metav1.Condition{
					Type:    rukpakv1alpha1.TypeUnpacked,
					Status:  metav1.ConditionUnknown,
					Reason:  rukpakv1alpha1.ReasonProcessingFinalizerFailed,
					Message: err.Error(),
				}),
			)
		}
		return ctrl.Result{}, err
	}

	unpackResult, err := r.Unpacker.Unpack(ctx, bundle)
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&u, fmt.Errorf("source bundle content: %v", err))
	}
	switch unpackResult.State {
	case source.StatePending:
		updateStatusUnpackPending(&u, unpackResult)
		return ctrl.Result{}, nil
	case source.StateUnpacking:
		updateStatusUnpacking(&u, unpackResult)
		return ctrl.Result{}, nil
	}

	// Helm expects an FS whose root contains a single chart directory. Depending on how
	// the bundle is sourced, the FS may or may not contain this single chart directory in
	// its root (e.g. charts uploaded via 'rukpakctl run <bdName> <chartDir>') would not.
	// This FS wrapper adds this base directory unless the FS already has a base directory.
	chartFS, err := util.EnsureBaseDirFS(unpackResult.Bundle, "chart")
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&u, err)
	}

	_, err = getChart(chartFS)
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&u, err)
	}
	if err := r.Storage.Store(ctx, bundle, chartFS); err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&u, fmt.Errorf("persist bundle objects: %v", err))
	}

	contentURL, err := r.Storage.URLFor(ctx, bundle)
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&u, fmt.Errorf("get content URL: %v", err))
	}

	updateStatusUnpacked(&u, unpackResult, contentURL)
	return ctrl.Result{}, nil
}

func updateStatusUnpackPending(u *updater.Updater, result *source.Result) {
	u.UpdateStatus(
		updater.EnsureResolvedSource(nil),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhasePending),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpackPending,
			Message: result.Message,
		}),
	)
}

func updateStatusUnpacking(u *updater.Updater, result *source.Result) {
	u.UpdateStatus(
		updater.EnsureResolvedSource(nil),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhaseUnpacking),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpacking,
			Message: result.Message,
		}),
	)
}

func updateStatusUnpacked(u *updater.Updater, result *source.Result, contentURL string) {
	u.UpdateStatus(
		updater.EnsureResolvedSource(result.ResolvedSource),
		updater.EnsureContentURL(contentURL),
		updater.SetPhase(rukpakv1alpha1.PhaseUnpacked),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionTrue,
			Reason:  rukpakv1alpha1.ReasonUnpackSuccessful,
			Message: result.Message,
		}),
	)
}

func updateStatusUnpackFailing(u *updater.Updater, err error) error {
	u.UpdateStatus(
		updater.EnsureResolvedSource(nil),
		updater.EnsureContentURL(""),
		updater.SetPhase(rukpakv1alpha1.PhaseFailing),
		updater.EnsureCondition(metav1.Condition{
			Type:    rukpakv1alpha1.TypeUnpacked,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonUnpackFailed,
			Message: err.Error(),
		}),
	)
	return err
}

// SetupWithManager sets up the controller with the Manager.
func (r *BundleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rukpakv1alpha1.Bundle{}, builder.WithPredicates(
			util.BundleProvisionerFilter(helm.ProvisionerID),
		)).
		Complete(r)
}
