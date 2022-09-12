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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/log"
	crsource "sigs.k8s.io/controller-runtime/pkg/source"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	helm "github.com/operator-framework/rukpak/internal/provisioner/helm/types"
	"github.com/operator-framework/rukpak/internal/source"
	"github.com/operator-framework/rukpak/internal/storage"
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
	existingBundle := &rukpakv1alpha1.Bundle{}
	if err := r.Get(ctx, req.NamespacedName, existingBundle); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reconciledBundle := existingBundle.DeepCopy()
	res, reconcileErr := r.reconcile(ctx, reconciledBundle)

	// Update the status subresource before updating the main object. This is
	// necessary because, in many cases, the main object update will remove the
	// finalizer, which will cause the core Kubernetes deletion logic to
	// complete. Therefore, we need to make the status update prior to the main
	// object update to ensure that the status update can be processed before
	// a potential deletion.
	if !equality.Semantic.DeepEqual(existingBundle.Status, reconciledBundle.Status) {
		if updateErr := r.Client.Status().Update(ctx, reconciledBundle); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	existingBundle.Status, reconciledBundle.Status = rukpakv1alpha1.BundleStatus{}, rukpakv1alpha1.BundleStatus{}
	if !equality.Semantic.DeepEqual(existingBundle, reconciledBundle) {
		if updateErr := r.Client.Update(ctx, reconciledBundle); updateErr != nil {
			return res, apimacherrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	return res, reconcileErr
}

func (r *BundleReconciler) reconcile(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (ctrl.Result, error) {
	bundle.Status.ObservedGeneration = bundle.Generation

	finalizedBundle := bundle.DeepCopy()
	finalizerResult, err := r.Finalizers.Finalize(ctx, finalizedBundle)
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

	unpackResult, err := r.Unpacker.Unpack(ctx, bundle)
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
	}

	// Helm expects an FS whose root contains a single chart directory. Depending on how
	// the bundle is sourced, the FS may or may not contain this single chart directory in
	// its root (e.g. charts uploaded via 'rukpakctl run <bdName> <chartDir>') would not.
	// This FS wrapper adds this base directory unless the FS already has a base directory.
	chartFS, err := util.EnsureBaseDirFS(unpackResult.Bundle, "chart")
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&bundle.Status, err)
	}

	_, err = getChart(chartFS)
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&bundle.Status, err)
	}
	if err := r.Storage.Store(ctx, bundle, chartFS); err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&bundle.Status, fmt.Errorf("persist bundle objects: %v", err))
	}

	contentURL, err := r.Storage.URLFor(ctx, bundle)
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&bundle.Status, fmt.Errorf("get content URL: %v", err))
	}

	updateStatusUnpacked(&bundle.Status, unpackResult, contentURL)
	return ctrl.Result{}, nil
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

// SetupWithManager sets up the controller with the Manager.
func (r *BundleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	l := mgr.GetLogger().WithName("controller.bundle")
	return ctrl.NewControllerManagedBy(mgr).
		For(&rukpakv1alpha1.Bundle{}, builder.WithPredicates(
			util.BundleProvisionerFilter(helm.ProvisionerID),
		)).
		// The default image source unpacker creates Pod's ownerref'd to its bundle, so
		// we need to watch pods to ensure we reconcile events coming from these
		// pods.
		Watches(&crsource.Kind{Type: &corev1.Pod{}}, util.MapOwneeToOwnerProvisionerHandler(context.Background(), mgr.GetClient(), l, helm.ProvisionerID, &rukpakv1alpha1.Bundle{})).
		Complete(r)
}
