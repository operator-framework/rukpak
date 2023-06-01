package bundledeployment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	helmclient "github.com/operator-framework/helm-operator-plugins/pkg/client"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/postrender"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	apimachyaml "k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	helmpredicate "github.com/operator-framework/rukpak/internal/helm-operator-plugins/predicate"
	"github.com/operator-framework/rukpak/internal/storage"
	"github.com/operator-framework/rukpak/internal/util"
)

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

type Option func(bd *controller)

func WithHandler(h Handler) Option {
	return func(c *controller) {
		c.handler = h
	}
}

func WithProvisionerID(provisionerID string) Option {
	return func(c *controller) {
		c.provisionerID = provisionerID
	}
}

func WithStorage(s storage.Storage) Option {
	return func(c *controller) {
		c.storage = s
	}
}

func WithActionClientGetter(acg helmclient.ActionClientGetter) Option {
	return func(c *controller) {
		c.acg = acg
	}
}

func WithReleaseNamespace(releaseNamespace string) Option {
	return func(c *controller) {
		c.releaseNamespace = releaseNamespace
	}
}

func SetupWithManager(mgr manager.Manager, opts ...Option) error {
	c := &controller{
		cl:               mgr.GetClient(),
		dynamicWatchGVKs: map[schema.GroupVersionKind]struct{}{},
	}

	for _, o := range opts {
		o(c)
	}

	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("invalid configuration: %v", err)
	}

	controllerName := fmt.Sprintf("controller.bundledeployment.%s", c.provisionerID)
	l := mgr.GetLogger().WithName(controllerName)
	controller, err := ctrl.NewControllerManagedBy(mgr).
		Named(controllerName).
		For(&rukpakv1alpha1.BundleDeployment{}, builder.WithPredicates(
			util.BundleDeploymentProvisionerFilter(c.provisionerID)),
		).
		Watches(&source.Kind{Type: &rukpakv1alpha1.Bundle{}}, handler.EnqueueRequestsFromMapFunc(
			util.MapBundleToBundleDeploymentHandler(context.Background(), mgr.GetClient(), l, c.provisionerID)),
		).
		Build(c)
	if err != nil {
		return err
	}
	c.controller = controller
	return nil
}

func (c *controller) validateConfig() error {
	errs := []error{}
	if c.handler == nil {
		errs = append(errs, errors.New("converter is unset"))
	}
	if c.provisionerID == "" {
		errs = append(errs, errors.New("provisioner ID is unset"))
	}
	if c.acg == nil {
		errs = append(errs, errors.New("action client getter is unset"))
	}
	if c.storage == nil {
		errs = append(errs, errors.New("storage is unset"))
	}
	if c.releaseNamespace == "" {
		errs = append(errs, errors.New("release namespace is unset"))
	}
	return utilerrors.NewAggregate(errs)
}

// controller reconciles a BundleDeployment object
type controller struct {
	cl client.Client

	handler          Handler
	provisionerID    string
	acg              helmclient.ActionClientGetter
	storage          storage.Storage
	releaseNamespace string

	controller        crcontroller.Controller
	dynamicWatchMutex sync.RWMutex
	dynamicWatchGVKs  map[schema.GroupVersionKind]struct{}
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
func (c *controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(1).Info("starting reconciliation")
	defer l.V(1).Info("ending reconciliation")

	existingBD := &rukpakv1alpha1.BundleDeployment{}
	if err := c.cl.Get(ctx, req.NamespacedName, existingBD); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reconciledBD := existingBD.DeepCopy()
	res, reconcileErr := c.reconcile(ctx, reconciledBD)

	if !equality.Semantic.DeepEqual(existingBD.Status, reconciledBD.Status) {
		if updateErr := c.cl.Status().Update(ctx, reconciledBD); updateErr != nil {
			return res, utilerrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}
	existingBD.Status, reconciledBD.Status = rukpakv1alpha1.BundleDeploymentStatus{}, rukpakv1alpha1.BundleDeploymentStatus{}
	if !equality.Semantic.DeepEqual(existingBD, reconciledBD) {
		if updateErr := c.cl.Update(ctx, reconciledBD); updateErr != nil {
			return res, utilerrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}

	return res, reconcileErr
}

func (c *controller) reconcile(ctx context.Context, bd *rukpakv1alpha1.BundleDeployment) (ctrl.Result, error) {
	bd.Status.ObservedGeneration = bd.Generation

	bundle, allBundles, err := util.ReconcileDesiredBundle(ctx, c.cl, bd)
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

	bundleFS, err := c.storage.Load(ctx, bundle)
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeHasValidBundle,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonBundleLoadFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	chrt, values, err := c.handler.Handle(ctx, bundleFS, bd)
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeHasValidBundle,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonBundleLoadFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	bd.SetNamespace(c.releaseNamespace)
	cl, err := c.acg.ActionClientFor(bd)
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

	post := &postrenderer{
		labels: map[string]string{
			util.CoreOwnerKindKey: rukpakv1alpha1.BundleDeploymentKind,
			util.CoreOwnerNameKey: bd.GetName(),
		},
	}

	rel, state, err := c.getReleaseState(cl, bd, chrt, values, post)
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
		rel, err = cl.Install(bd.Name, c.releaseNamespace, chrt, values, func(install *action.Install) error {
			install.CreateNamespace = false
			return nil
		},
			// To be refactored issue https://github.com/operator-framework/rukpak/issues/534
			func(install *action.Install) error {
				post.cascade = install.PostRenderer
				install.PostRenderer = post
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
		rel, err = cl.Upgrade(bd.Name, c.releaseNamespace, chrt, values,
			// To be refactored issue https://github.com/operator-framework/rukpak/issues/534
			func(upgrade *action.Upgrade) error {
				post.cascade = upgrade.PostRenderer
				upgrade.PostRenderer = post
				return nil
			})
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

	relObjects, err := util.ManifestObjects(strings.NewReader(rel.Manifest), fmt.Sprintf("%s-release-manifest", rel.Name))
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha1.TypeInstalled,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha1.ReasonCreateDynamicWatchFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	for _, obj := range relObjects {
		uMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
				Type:    rukpakv1alpha1.TypeInstalled,
				Status:  metav1.ConditionFalse,
				Reason:  rukpakv1alpha1.ReasonCreateDynamicWatchFailed,
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}

		unstructuredObj := &unstructured.Unstructured{Object: uMap}
		if err := func() error {
			c.dynamicWatchMutex.Lock()
			defer c.dynamicWatchMutex.Unlock()

			_, isWatched := c.dynamicWatchGVKs[unstructuredObj.GroupVersionKind()]
			if !isWatched {
				if err := c.controller.Watch(
					&source.Kind{Type: unstructuredObj},
					&handler.EnqueueRequestForOwner{OwnerType: bd, IsController: true},
					helmpredicate.DependentPredicateFuncs()); err != nil {
					return err
				}
				c.dynamicWatchGVKs[unstructuredObj.GroupVersionKind()] = struct{}{}
			}
			return nil
		}(); err != nil {
			meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
				Type:    rukpakv1alpha1.TypeInstalled,
				Status:  metav1.ConditionFalse,
				Reason:  rukpakv1alpha1.ReasonCreateDynamicWatchFailed,
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}
	}
	meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha1.TypeInstalled,
		Status:  metav1.ConditionTrue,
		Reason:  rukpakv1alpha1.ReasonInstallationSucceeded,
		Message: fmt.Sprintf("Instantiated bundle %s successfully", bundle.GetName()),
	})
	bd.Status.ActiveBundle = bundle.GetName()

	if err := c.reconcileOldBundles(ctx, bundle, allBundles); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to delete old bundles: %v", err)
	}

	return ctrl.Result{}, nil
}

// reconcileOldBundles is responsible for garbage collecting any Bundles
// that no longer match the desired Bundle template.
func (c *controller) reconcileOldBundles(ctx context.Context, currBundle *rukpakv1alpha1.Bundle, allBundles *rukpakv1alpha1.BundleList) error {
	var (
		errors []error
	)
	for i := range allBundles.Items {
		if allBundles.Items[i].GetName() == currBundle.GetName() {
			continue
		}
		if err := c.cl.Delete(ctx, &allBundles.Items[i]); err != nil {
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

func (c *controller) getReleaseState(cl helmclient.ActionInterface, obj metav1.Object, chrt *chart.Chart, values chartutil.Values, post *postrenderer) (*release.Release, releaseState, error) {
	currentRelease, err := cl.Get(obj.GetName())
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, stateError, err
	}
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, stateNeedsInstall, nil
	}
	desiredRelease, err := cl.Upgrade(obj.GetName(), c.releaseNamespace, chrt, values, func(upgrade *action.Upgrade) error {
		upgrade.DryRun = true
		return nil
	},
		// To be refactored issue https://github.com/operator-framework/rukpak/issues/534
		func(upgrade *action.Upgrade) error {
			post.cascade = upgrade.PostRenderer
			upgrade.PostRenderer = post
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

type postrenderer struct {
	labels  map[string]string
	cascade postrender.PostRenderer
}

func (p *postrenderer) Run(renderedManifests *bytes.Buffer) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	dec := apimachyaml.NewYAMLOrJSONDecoder(renderedManifests, 1024)
	for {
		obj := unstructured.Unstructured{}
		err := dec.Decode(&obj)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		obj.SetLabels(p.labels)
		b, err := obj.MarshalJSON()
		if err != nil {
			return nil, err
		}
		buf.Write(b)
	}
	if p.cascade != nil {
		return p.cascade.Run(&buf)
	}
	return &buf, nil
}
