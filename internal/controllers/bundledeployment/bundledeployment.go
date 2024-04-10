package bundledeployment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/postrender"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crcontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	crfinalizer "sigs.k8s.io/controller-runtime/pkg/finalizer"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"

	helmclient "github.com/operator-framework/helm-operator-plugins/pkg/client"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/healthchecks"
	helmpredicate "github.com/operator-framework/rukpak/internal/helm-operator-plugins/predicate"
	unpackersource "github.com/operator-framework/rukpak/internal/source"
	"github.com/operator-framework/rukpak/internal/storage"
	"github.com/operator-framework/rukpak/internal/util"
	"github.com/operator-framework/rukpak/pkg/features"
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

func WithFinalizers(f crfinalizer.Finalizers) Option {
	return func(c *controller) {
		c.finalizers = f
	}
}

func WithStorage(s storage.Storage) Option {
	return func(c *controller) {
		c.storage = s
	}
}

func WithUnpacker(u unpackersource.Unpacker) Option {
	return func(c *controller) {
		c.unpacker = u
	}
}

func WithActionClientGetter(acg helmclient.ActionClientGetter) Option {
	return func(c *controller) {
		c.acg = acg
	}
}

func SetupWithManager(mgr manager.Manager, systemNamespace string, opts ...Option) error {
	c := &controller{
		cl:               mgr.GetClient(),
		cache:            mgr.GetCache(),
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
		For(&rukpakv1alpha2.BundleDeployment{}, builder.WithPredicates(
			util.BundleDeploymentProvisionerFilter(c.provisionerID)),
		).
		Watches(&corev1.Pod{}, util.MapOwneeToOwnerProvisionerHandler(mgr.GetClient(), l, c.provisionerID, &rukpakv1alpha2.BundleDeployment{})).
		Watches(&corev1.ConfigMap{}, util.MapConfigMapToBundleDeploymentHandler(mgr.GetClient(), systemNamespace, c.provisionerID)).
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
	if c.unpacker == nil {
		errs = append(errs, errors.New("unpacker is unset"))
	}
	if c.finalizers == nil {
		errs = append(errs, errors.New("finalizer handler is unset"))
	}
	return utilerrors.NewAggregate(errs)
}

// controller reconciles a BundleDeployment object
type controller struct {
	cl    client.Client
	cache cache.Cache

	handler       Handler
	provisionerID string
	acg           helmclient.ActionClientGetter
	storage       storage.Storage

	unpacker          unpackersource.Unpacker
	controller        crcontroller.Controller
	finalizers        crfinalizer.Finalizers
	dynamicWatchMutex sync.RWMutex
	dynamicWatchGVKs  map[schema.GroupVersionKind]struct{}
}

//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundledeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundledeployments,verbs=list;watch
//+kubebuilder:rbac:groups=core.rukpak.io,resources=bundledeployments/status,verbs=update;patch
//+kubebuilder:rbac:verbs=get,urls=/bundles/*
//+kubebuilder:rbac:groups=core,resources=pods,verbs=list;watch;create;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=list;watch
//+kubebuilder:rbac:groups=core,resources=pods/log,verbs=get
//+kubebuilder:rbac:groups=*,resources=*,verbs=*
//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
//+kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (c *controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(1).Info("starting reconciliation")
	defer l.V(1).Info("ending reconciliation")

	existingBD := &rukpakv1alpha2.BundleDeployment{}
	if err := c.cl.Get(ctx, req.NamespacedName, existingBD); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	reconciledBD := existingBD.DeepCopy()
	res, reconcileErr := c.reconcile(ctx, reconciledBD)

	// Do checks before any Update()s, as Update() may modify the resource structure!
	updateStatus := !equality.Semantic.DeepEqual(existingBD.Status, reconciledBD.Status)
	updateFinalizers := !equality.Semantic.DeepEqual(existingBD.Finalizers, reconciledBD.Finalizers)
	unexpectedFieldsChanged := checkForUnexpectedFieldChange(*existingBD, *reconciledBD)

	if updateStatus {
		if updateErr := c.cl.Status().Update(ctx, reconciledBD); updateErr != nil {
			return res, utilerrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}

	if unexpectedFieldsChanged {
		panic("spec or metadata changed by reconciler")
	}

	if updateFinalizers {
		if updateErr := c.cl.Update(ctx, reconciledBD); updateErr != nil {
			return res, utilerrors.NewAggregate([]error{reconcileErr, updateErr})
		}
	}

	return res, reconcileErr
}

// nolint:unparam
// Today we always return ctrl.Result{} and an error.
// But in the future we might update this function
// to return different results (e.g. requeue).
func (c *controller) reconcile(ctx context.Context, bd *rukpakv1alpha2.BundleDeployment) (ctrl.Result, error) {
	bd.Status.ObservedGeneration = bd.Generation

	// handle finalizers.
	_, err := c.finalizers.Finalize(ctx, bd)

	if err != nil {
		bd.Status.ResolvedSource = nil
		bd.Status.ContentURL = ""
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha2.TypeUnpacked,
			Status:  metav1.ConditionUnknown,
			Reason:  rukpakv1alpha2.ReasonProcessingFinalizerFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	unpackResult, err := c.unpacker.Unpack(ctx, bd)
	if err != nil {
		return ctrl.Result{}, updateStatusUnpackFailing(&bd.Status, fmt.Errorf("source bundle content: %v", err))
	}

	switch unpackResult.State {
	case unpackersource.StatePending:
		updateStatusUnpackPending(&bd.Status, unpackResult)
		// There must a limit to number of retries if status is stuck at
		// unpack pending.
		return ctrl.Result{}, nil
	case unpackersource.StateUnpacking:
		updateStatusUnpacking(&bd.Status, unpackResult)
		return ctrl.Result{}, nil
	case unpackersource.StateUnpacked:
		if err := c.storage.Store(ctx, bd, unpackResult.Bundle); err != nil {
			return ctrl.Result{}, updateStatusUnpackFailing(&bd.Status, fmt.Errorf("persist bundle content: %v", err))
		}
		contentURL, err := c.storage.URLFor(ctx, bd)
		if err != nil {
			return ctrl.Result{}, updateStatusUnpackFailing(&bd.Status, fmt.Errorf("get content URL: %v", err))
		}
		updateStatusUnpacked(&bd.Status, unpackResult, contentURL)
	default:
		return ctrl.Result{}, updateStatusUnpackFailing(&bd.Status, fmt.Errorf("unknown unpack state %q: %v", unpackResult.State, err))
	}

	bundleFS, err := c.storage.Load(ctx, bd)
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha2.TypeHasValidBundle,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha2.ReasonBundleLoadFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	chrt, values, err := c.handler.Handle(ctx, bundleFS, bd)
	if err != nil {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha2.TypeInstalled,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha2.ReasonInstallFailed,
			Message: err.Error(),
		})
		return ctrl.Result{}, err
	}

	cl, err := c.acg.ActionClientFor(ctx, bd)
	if err != nil {
		setInstalledAndHealthyFalse(bd, rukpakv1alpha2.ReasonErrorGettingClient, err.Error())
		return ctrl.Result{}, err
	}

	post := &postrenderer{
		labels: map[string]string{
			util.CoreOwnerKindKey: rukpakv1alpha2.BundleDeploymentKind,
			util.CoreOwnerNameKey: bd.GetName(),
		},
	}

	rel, state, err := c.getReleaseState(cl, bd, chrt, values, post)
	if err != nil {
		setInstalledAndHealthyFalse(bd, rukpakv1alpha2.ReasonErrorGettingReleaseState, err.Error())
		return ctrl.Result{}, err
	}

	switch state {
	case stateNeedsInstall:
		rel, err = cl.Install(bd.Name, bd.Spec.InstallNamespace, chrt, values, func(install *action.Install) error {
			install.CreateNamespace = false
			return nil
		}, helmclient.AppendInstallPostRenderer(post))
		if err != nil {
			if isResourceNotFoundErr(err) {
				err = errRequiredResourceNotFound{err}
			}
			setInstalledAndHealthyFalse(bd, rukpakv1alpha2.ReasonInstallFailed, err.Error())
			return ctrl.Result{}, err
		}
	case stateNeedsUpgrade:
		rel, err = cl.Upgrade(bd.Name, bd.Spec.InstallNamespace, chrt, values, helmclient.AppendUpgradePostRenderer(post))
		if err != nil {
			if isResourceNotFoundErr(err) {
				err = errRequiredResourceNotFound{err}
			}
			setInstalledAndHealthyFalse(bd, rukpakv1alpha2.ReasonUpgradeFailed, err.Error())
			return ctrl.Result{}, err
		}
	case stateUnchanged:
		if err := cl.Reconcile(rel); err != nil {
			if isResourceNotFoundErr(err) {
				err = errRequiredResourceNotFound{err}
			}
			setInstalledAndHealthyFalse(bd, rukpakv1alpha2.ReasonReconcileFailed, err.Error())
			return ctrl.Result{}, err
		}
	default:
		return ctrl.Result{}, fmt.Errorf("unexpected release state %q", state)
	}

	relObjects, err := util.ManifestObjects(strings.NewReader(rel.Manifest), fmt.Sprintf("%s-release-manifest", rel.Name))
	if err != nil {
		setInstalledAndHealthyFalse(bd, rukpakv1alpha2.ReasonCreateDynamicWatchFailed, err.Error())
		return ctrl.Result{}, err
	}

	for _, obj := range relObjects {
		uMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			setInstalledAndHealthyFalse(bd, rukpakv1alpha2.ReasonCreateDynamicWatchFailed, err.Error())
			return ctrl.Result{}, err
		}

		unstructuredObj := &unstructured.Unstructured{Object: uMap}
		if err := func() error {
			c.dynamicWatchMutex.Lock()
			defer c.dynamicWatchMutex.Unlock()

			_, isWatched := c.dynamicWatchGVKs[unstructuredObj.GroupVersionKind()]
			if !isWatched {
				if err := c.controller.Watch(
					source.Kind(c.cache, unstructuredObj),
					handler.EnqueueRequestForOwner(c.cl.Scheme(), c.cl.RESTMapper(), bd, handler.OnlyControllerOwner()),
					helmpredicate.DependentPredicateFuncs()); err != nil {
					return err
				}
				c.dynamicWatchGVKs[unstructuredObj.GroupVersionKind()] = struct{}{}
			}
			return nil
		}(); err != nil {
			setInstalledAndHealthyFalse(bd, rukpakv1alpha2.ReasonCreateDynamicWatchFailed, err.Error())
			return ctrl.Result{}, err
		}
	}
	meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha2.TypeInstalled,
		Status:  metav1.ConditionTrue,
		Reason:  rukpakv1alpha2.ReasonInstallationSucceeded,
		Message: fmt.Sprintf("Instantiated bundle %s successfully", bd.GetName()),
	})

	if features.RukpakFeatureGate.Enabled(features.BundleDeploymentHealth) {
		if err = healthchecks.AreObjectsHealthy(ctx, c.cl, relObjects); err != nil {
			meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
				Type:    rukpakv1alpha2.TypeHealthy,
				Status:  metav1.ConditionFalse,
				Reason:  rukpakv1alpha2.ReasonUnhealthy,
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha2.TypeHealthy,
			Status:  metav1.ConditionTrue,
			Reason:  rukpakv1alpha2.ReasonHealthy,
			Message: "BundleDeployment is healthy",
		})
	}

	return ctrl.Result{}, nil
}

// setInstalledAndHealthyFalse sets the Installed and if the feature gate is enabled, the Healthy conditions to False,
// and allows to set the Installed condition reason and message.
func setInstalledAndHealthyFalse(bd *rukpakv1alpha2.BundleDeployment, installedConditionReason, installedConditionMessage string) {
	meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha2.TypeInstalled,
		Status:  metav1.ConditionFalse,
		Reason:  installedConditionReason,
		Message: installedConditionMessage,
	})

	if features.RukpakFeatureGate.Enabled(features.BundleDeploymentHealth) {
		meta.SetStatusCondition(&bd.Status.Conditions, metav1.Condition{
			Type:    rukpakv1alpha2.TypeHealthy,
			Status:  metav1.ConditionFalse,
			Reason:  rukpakv1alpha2.ReasonInstallationStatusFalse,
			Message: "Installed condition is false",
		})
	}
}

type releaseState string

const (
	stateNeedsInstall releaseState = "NeedsInstall"
	stateNeedsUpgrade releaseState = "NeedsUpgrade"
	stateUnchanged    releaseState = "Unchanged"
	stateError        releaseState = "Error"
)

func (c *controller) getReleaseState(cl helmclient.ActionInterface, bd *rukpakv1alpha2.BundleDeployment, chrt *chart.Chart, values chartutil.Values, post *postrenderer) (*release.Release, releaseState, error) {
	currentRelease, err := cl.Get(bd.GetName())
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, stateError, err
	}
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, stateNeedsInstall, nil
	}
	desiredRelease, err := cl.Upgrade(bd.GetName(), bd.Spec.InstallNamespace, chrt, values, func(upgrade *action.Upgrade) error {
		upgrade.DryRun = true
		return nil
	}, helmclient.AppendUpgradePostRenderer(post))
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
	if strings.Contains(err.Error(), "no matches for kind") {
		return true
	}
	return strings.Contains(err.Error(), "the server could not find the requested resource")
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
		obj.SetLabels(util.MergeMaps(obj.GetLabels(), p.labels))
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

// Compare resources - ignoring status & metadata.finalizers
func checkForUnexpectedFieldChange(a, b rukpakv1alpha2.BundleDeployment) bool {
	a.Status, b.Status = rukpakv1alpha2.BundleDeploymentStatus{}, rukpakv1alpha2.BundleDeploymentStatus{}
	a.Finalizers, b.Finalizers = []string{}, []string{}
	return !equality.Semantic.DeepEqual(a, b)
}

func updateStatusUnpackFailing(status *rukpakv1alpha2.BundleDeploymentStatus, err error) error {
	status.ResolvedSource = nil
	status.ContentURL = ""
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha2.TypeUnpacked,
		Status:  metav1.ConditionFalse,
		Reason:  rukpakv1alpha2.ReasonUnpackFailed,
		Message: err.Error(),
	})
	return err
}

func updateStatusUnpackPending(status *rukpakv1alpha2.BundleDeploymentStatus, result *unpackersource.Result) {
	status.ResolvedSource = nil
	status.ContentURL = ""
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha2.TypeUnpacked,
		Status:  metav1.ConditionFalse,
		Reason:  rukpakv1alpha2.ReasonUnpackPending,
		Message: result.Message,
	})
}

func updateStatusUnpacking(status *rukpakv1alpha2.BundleDeploymentStatus, result *unpackersource.Result) {
	status.ResolvedSource = nil
	status.ContentURL = ""
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha2.TypeUnpacked,
		Status:  metav1.ConditionFalse,
		Reason:  rukpakv1alpha2.ReasonUnpacking,
		Message: result.Message,
	})
}

func updateStatusUnpacked(status *rukpakv1alpha2.BundleDeploymentStatus, result *unpackersource.Result, contentURL string) {
	status.ResolvedSource = result.ResolvedSource
	status.ContentURL = contentURL
	meta.SetStatusCondition(&status.Conditions, metav1.Condition{
		Type:    rukpakv1alpha2.TypeUnpacked,
		Status:  metav1.ConditionTrue,
		Reason:  rukpakv1alpha2.ReasonUnpackSuccessful,
		Message: result.Message,
	})
}
