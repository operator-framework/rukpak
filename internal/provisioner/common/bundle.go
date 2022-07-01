package common

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/source"
	"github.com/operator-framework/rukpak/internal/storage"
	updater "github.com/operator-framework/rukpak/internal/updater/bundle"
)

type ParseUnpackStateFunc func(context.Context, *source.Result, *updater.Updater, *rukpakv1alpha1.Bundle, storage.Storage) (ctrl.Result, error)
type ProvisionerSetupFunc func(manager.Manager, BundleReconciler, BundleDeploymentReconciler) error

type BundleReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Storage          storage.Storage
	Finalizers       finalizer.Finalizers
	Unpacker         source.Unpacker
	ProvisionerID    string
	ParseUnpackState ParseUnpackStateFunc
}

func DefaultParseUnpackStateFunc(ctx context.Context, unpackResult *source.Result, u *updater.Updater, bundle *rukpakv1alpha1.Bundle, s storage.Storage) (ctrl.Result, error) {
	switch unpackResult.State {
	case source.StatePending:
		UpdateStatusUnpackPending(u, unpackResult)
		return ctrl.Result{}, nil
	case source.StateUnpacking:
		UpdateStatusUnpacking(u, unpackResult)
		return ctrl.Result{}, nil
	case source.StateUnpacked:
		objects, err := GetObjects(unpackResult.Bundle)
		if err != nil {
			return ctrl.Result{}, UpdateStatusUnpackFailing(u, fmt.Errorf("get objects from bundle manifests: %v", err))
		}
		if len(objects) == 0 {
			return ctrl.Result{}, UpdateStatusUnpackFailing(u, errors.New("invalid bundle: found zero objects"))
		}

		if err := s.Store(ctx, bundle, unpackResult.Bundle); err != nil {
			return ctrl.Result{}, UpdateStatusUnpackFailing(u, fmt.Errorf("persist bundle objects: %v", err))
		}

		contentURL, err := s.URLFor(ctx, bundle)
		if err != nil {
			return ctrl.Result{}, UpdateStatusUnpackFailing(u, fmt.Errorf("get content URL: %v", err))
		}

		UpdateStatusUnpacked(u, unpackResult, contentURL)
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{}, UpdateStatusUnpackFailing(u, fmt.Errorf("unknown unpack state %q", unpackResult.State))
	}
}

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
		return ctrl.Result{}, UpdateStatusUnpackFailing(&u, fmt.Errorf("source bundle content: %v", err))
	}

	return r.ParseUnpackState(ctx, unpackResult, &u, bundle, r.Storage)
}
