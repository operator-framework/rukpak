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
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	helmclient "github.com/operator-framework/helm-operator-plugins/pkg/client"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	helm "github.com/operator-framework/rukpak/internal/provisioner/helm/types"
	"github.com/operator-framework/rukpak/internal/storage"
	"github.com/operator-framework/rukpak/internal/util"
)

// BundleDeploymentReconciler reconciles a BundleDeployment object
type BundleDeploymentReconciler struct {
	client.Client
	Reader client.Reader

	Scheme     *runtime.Scheme
	Controller controller.Controller

	ActionClientGetter helmclient.ActionClientGetter
	BundleStorage      storage.Storage
	ReleaseNamespace   string
}

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundledeployments,verbs=list;watch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundledeployments/status,verbs=update;patch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundledeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *BundleDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(1).Info("starting reconciliation")
	defer l.V(1).Info("ending reconciliation")

	existingBD := &rukpakv1alpha1.BundleDeployment{}
	if err := r.Get(ctx, req.NamespacedName, existingBD); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reconciledBD := existingBD.DeepCopy()
	res, reconcileErr := r.reconcile(ctx, reconciledBD)

	if !equality.Semantic.DeepEqual(existingBD.Status, reconciledBD.Status) {
		if updateErr := r.Client.Status().Update(ctx, reconciledBD); updateErr != nil {
			return res, utilerrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	existingBD.Status, reconciledBD.Status = rukpakv1alpha1.BundleDeploymentStatus{}, rukpakv1alpha1.BundleDeploymentStatus{}
	if !equality.Semantic.DeepEqual(existingBD, reconciledBD) {
		if updateErr := r.Client.Update(ctx, reconciledBD); updateErr != nil {
			return res, utilerrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}

	return res, reconcileErr
}

func (r *BundleDeploymentReconciler) reconcile(ctx context.Context, bd *rukpakv1alpha1.BundleDeployment) (ctrl.Result, error) {
	bd.Status.ObservedGeneration = bd.Generation

	bundle, allBundles, err := util.ReconcileDesiredBundle(ctx, r.Client, bd)
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeHasValidBundle,
			Status:  metav1.ConditionUnknown,
			Reason:  rukpakv1alpha1.ReasonReconcileFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}
	if bundle.Status.Phase != rukpakv1alpha1.PhaseUnpacked {
		reason := rukpakv1alpha1.ReasonUnpackPending
		status := metav1.ConditionTrue
		message := fmt.Sprintf("Waiting for the %s Bundle to be unpacked", bundle.GetName())
		if bundle.Status.Phase == rukpakv1alpha1.PhaseFailing {
			reason = rukpakv1alpha1.ReasonUnpackFailed
			status = metav1.ConditionFalse
			message = fmt.Sprintf("Failed to unpack the %s Bundle", bundle.GetName())
			if c := meta.FindStatusCondition(bundle.Status.Conditions, rukpakv1alpha1.TypeUnpacked); c != nil {
				message = fmt.Sprintf("%s: %s", message, c.Message)
			}
		}
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeHasValidBundle,
			Status:  status,
			Reason:  reason,
			Message: message,
		})
		return ctrl.Result{}, nil
	}

	meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha1.TypeHasValidBundle,
		Status:  metav1.ConditionTrue,
		Reason:  rukpakv1alpha1.ReasonUnpackSuccessful,
		Message: fmt.Sprintf("Successfully unpacked the %s Bundle", bundle.GetName()),
	})

	values, err := loadValues(bd)
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeInstalled,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonInstallFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	chrt, err := r.loadChart(ctx, bundle)
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeHasValidBundle,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonReadingContentFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	bd.SetNamespace(r.ReleaseNamespace)
	cl, err := r.ActionClientGetter.ActionClientFor(bd)
	bd.SetNamespace("")
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeInstalled,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonErrorGettingClient,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	rel, state, err := r.getReleaseState(cl, bd, chrt, values)
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeInstalled,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonErrorGettingReleaseState,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	switch state {
	case stateNeedsInstall:
		_, err = cl.Install(bd.Name, r.ReleaseNamespace, chrt, values, func(install *action.Install) error {
			install.CreateNamespace = false
			return nil
		})
		if err != nil {
			if isResourceNotFoundErr(err) {
				err = errRequiredResourceNotFound{err}
			}
			meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
				Type:    rukpakv1alpha1.TypeInstalled,
				Status:  metav1.ConditionFalse,
				Reason:  rukpakv1alpha1.ReasonInstallFailed,
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}
	case stateNeedsUpgrade:
		_, err = cl.Upgrade(bd.Name, r.ReleaseNamespace, chrt, values)
		if err != nil {
			if isResourceNotFoundErr(err) {
				err = errRequiredResourceNotFound{err}
			}
			meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
				Type:    rukpakv1alpha1.TypeInstalled,
				Status:  metav1.ConditionFalse,
				Reason:  rukpakv1alpha1.ReasonUpgradeFailed,
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}
	case stateUnchanged:
		if err := cl.Reconcile(rel); err != nil {
			if isResourceNotFoundErr(err) {
				err = errRequiredResourceNotFound{err}
			}
			meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{

				Type:    rukpakv1alpha1.TypeInstalled,
				Status:  metav1.ConditionFalse,
				Reason:  rukpakv1alpha1.ReasonReconcileFailed,
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}
	default:
		return ctrl.Result{}, fmt.Errorf("unexpected release state %q", state)
	}

	meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{

		Type:    rukpakv1alpha1.TypeInstalled,
		Status:  metav1.ConditionTrue,
		Reason:  rukpakv1alpha1.ReasonInstallationSucceeded,
		Message: fmt.Sprintf("instantiated bundle %s successfully", bundle.GetName()),
	})
	bd.Status.ActiveBundle = bundle.GetName()

	if err := r.reconcileOldBundles(ctx, bundle, allBundles); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete old bundles: %v", err)
	}

	return ctrl.Result{}, nil
}

// reconcileOldBundles is responsible for garbage collecting any Bundles
// that no longer match the desired Bundle template.
func (r *BundleDeploymentReconciler) reconcileOldBundles(ctx context.Context, currBundle *rukpakv1alpha1.Bundle, allBundles *rukpakv1alpha1.BundleList) error {
	var (
		errors []error
	)
	for _, b := range allBundles.Items {
		if b.GetName() == currBundle.GetName() {
			continue
		}
		if err := r.Delete(ctx, &b); err != nil {
			errors = append(errors, err)
			continue
		}
	}
	return utilerrors.NewAggregate(errors)
}

type releaseState string

const (
	stateNeedsInstall releaseState = "NeedsInstall"
	stateNeedsUpgrade releaseState = "NeedsUpgrade"
	stateUnchanged    releaseState = "Unchanged"
	stateError        releaseState = "Error"
)

func (r *BundleDeploymentReconciler) getReleaseState(cl helmclient.ActionInterface, obj metav1.Object, chrt *chart.Chart, values chartutil.Values) (*release.Release, releaseState, error) {
	currentRelease, err := cl.Get(obj.GetName())
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, stateError, err
	}
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, stateNeedsInstall, nil
	}
	desiredRelease, err := cl.Upgrade(obj.GetName(), r.ReleaseNamespace, chrt, values, func(upgrade *action.Upgrade) error {
		upgrade.DryRun = true
		return nil
	})
	if err != nil {
		return currentRelease, stateError, err
	}
	if desiredRelease.Manifest != currentRelease.Manifest ||
		currentRelease.Info.Status == release.StatusFailed ||
		currentRelease.Info.Status == release.StatusSuperseded {
		return currentRelease, stateNeedsUpgrade, nil
	}
	return currentRelease, stateUnchanged, nil
}

func loadValues(bd *rukpakv1alpha1.BundleDeployment) (chartutil.Values, error) {
	data, err := bd.Spec.Config.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var config map[string]string
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	valuesString := config["values"]

	var values chartutil.Values
	if valuesString != "" {
		values, err = chartutil.ReadValues([]byte(valuesString))
		if err != nil {
			return nil, err
		}
		return values, nil
	}
	return nil, nil
}

func (r *BundleDeploymentReconciler) loadChart(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*chart.Chart, error) {
	chartfs, err := r.BundleStorage.Load(ctx, bundle)
	if err != nil {
		return nil, err
	}
	return getChart(chartfs)
}

type errRequiredResourceNotFound struct {
	error
}

func (err errRequiredResourceNotFound) Error() string {
	return fmt.Sprintf("required resource not found: %v", err.error)
}

func isResourceNotFoundErr(err error) bool {
	var agg utilerrors.Aggregate
	if errors.As(err, &agg) {
		for _, err := range agg.Errors() {
			return isResourceNotFoundErr(err)
		}
	}

	nkme := &meta.NoKindMatchError{}
	if errors.As(err, &nkme) {
		return true
	}
	if apierrors.IsNotFound(err) {
		return true
	}

	// TODO: improve NoKindMatchError matching
	//   An error that is bubbled up from the k8s.io/cli-runtime library
	//   does not wrap meta.NoKindMatchError, so we need to fallback to
	//   the use of string comparisons for now.
	return strings.Contains(err.Error(), "no matches for kind")
}

// SetupWithManager sets up the controller with the Manager.
func (r *BundleDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&rukpakv1alpha1.BundleDeployment{}, builder.WithPredicates(
			util.BundleDeploymentProvisionerFilter(helm.ProvisionerID)),
		).
		Watches(&source.Kind{Type: &rukpakv1alpha1.Bundle{}}, handler.EnqueueRequestsFromMapFunc(
			util.MapBundleToBundleDeploymentHandler(context.Background(), mgr.GetClient(), mgr.GetLogger(), helm.ProvisionerID),
		)).
		Build(r)
	if err != nil {
		return err
	}
	r.Controller = controller
	return nil
}
