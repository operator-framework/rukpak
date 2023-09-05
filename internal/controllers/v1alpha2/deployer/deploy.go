package deployer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/postrender"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apimachyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"

	helmclient "github.com/operator-framework/helm-operator-plugins/pkg/client"
	"github.com/operator-framework/rukpak/api/v1alpha2"
	v1alpha2util "github.com/operator-framework/rukpak/internal/controllers/v1alpha2/controllers/util"
	"github.com/operator-framework/rukpak/internal/convert"
	"github.com/operator-framework/rukpak/internal/util"
	"golang.org/x/sync/errgroup"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/spf13/afero"
)

type Deployer interface {
	Deploy(ctx context.Context, fs afero.Fs, bundleDeployment *v1alpha2.BundleDeployment) error
}

type helmDeployer struct {
	actionClientGetter helmclient.ActionClientGetter
	dynamicWatchGVKs   map[schema.GroupVersionKind]struct{}
	dynamicWatchMutex  sync.RWMutex
	releaseNamespace   string
}

type DeployerOption func(*helmDeployer)

func WithActionClientGetter(cl helmclient.ActionClientGetter) DeployerOption {
	return func(hd *helmDeployer) {
		hd.actionClientGetter = cl
	}
}

func WithReleaseNamespace(ns string) DeployerOption {
	return func(hd *helmDeployer) {
		hd.releaseNamespace = ns
	}
}

// Pass kubeconfig and client by default to be able to apply
// manifests directly on cluster
func NewDefaultHelmDeployerWithOpts(opts ...DeployerOption) Deployer {
	dep := &helmDeployer{}
	for _, opt := range opts {
		opt(dep)
	}
	return dep
}

func (hd *helmDeployer) Deploy(ctx context.Context, fs afero.Fs, bundleDeployment *v1alpha2.BundleDeployment) error {
	chrt, values, err := hd.fetchChart(fs, bundleDeployment)
	if err != nil {
		return fmt.Errorf("error creating chart from bundle contents: %v", err)
	}

	if hd.releaseNamespace != "" {
		bundleDeployment.SetNamespace(hd.releaseNamespace)
	}

	cl, err := hd.actionClientGetter.ActionClientFor(bundleDeployment)
	bundleDeployment.SetNamespace("")
	if err != nil {
		return fmt.Errorf("error fetching client for bundle deployment %v", err)
	}

	post := &postrenderer{
		labels: map[string]string{
			util.CoreOwnerKindKey: v1alpha2.BundleDeploymentKind,
			util.CoreOwnerNameKey: bundleDeployment.GetName(),
		},
	}

	rel, state, err := hd.getReleaseState(cl, bundleDeployment, chrt, values, post)
	if err != nil {
		return fmt.Errorf("error fetching release state: %v", err)
	}

	switch state {
	case stateNeedsInstall:
		rel, err = cl.Install(bundleDeployment.Name, hd.releaseNamespace, chrt, values, func(install *action.Install) error {
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
			meta.SetStatusCondition(&bundleDeployment.Status.Conditions, metav1.Condition{
				Type:    v1alpha2.TypeInstalled,
				Status:  metav1.ConditionFalse,
				Reason:  v1alpha2.ReasonInstallFailed,
				Message: err.Error(),
			})
			return err
		}
	case stateNeedsUpgrade:
		rel, err = cl.Upgrade(bundleDeployment.Name, hd.releaseNamespace, chrt, values,
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
			meta.SetStatusCondition(&bundleDeployment.Status.Conditions, metav1.Condition{
				Type:    v1alpha2.TypeInstalled,
				Status:  metav1.ConditionFalse,
				Reason:  v1alpha2.ReasonUpgradeFailed,
				Message: err.Error(),
			})
			return err
		}
	case stateUnchanged:
		if err := cl.Reconcile(rel); err != nil {
			if isResourceNotFoundErr(err) {
				err = errRequiredResourceNotFound{err}
			}
			meta.SetStatusCondition(&bundleDeployment.Status.Conditions, metav1.Condition{
				Type:    v1alpha2.TypeInstalled,
				Status:  metav1.ConditionFalse,
				Reason:  v1alpha2.ReasonReconcileFailed,
				Message: err.Error(),
			})
			return err
		}
	default:
		return fmt.Errorf("unexpected release state %q", state)
	}

	meta.SetStatusCondition(&bundleDeployment.Status.Conditions, metav1.Condition{
		Type:    v1alpha2.TypeInstalled,
		Status:  metav1.ConditionTrue,
		Reason:  v1alpha2.ReasonInstallationSucceeded,
		Message: fmt.Sprintf("Instantiated bundle deployment %s successfully", bundleDeployment.GetName()),
	})
	return nil
}

type releaseState string

const (
	stateNeedsInstall releaseState = "NeedsInstall"
	stateNeedsUpgrade releaseState = "NeedsUpgrade"
	stateUnchanged    releaseState = "Unchanged"
	stateError        releaseState = "Error"
)

func (hd *helmDeployer) getReleaseState(cl helmclient.ActionInterface, obj metav1.Object, chrt *chart.Chart, values chartutil.Values, post *postrenderer) (*release.Release, releaseState, error) {
	currentRelease, err := cl.Get(obj.GetName())
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, stateError, err
	}
	if errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, stateNeedsInstall, nil
	}
	desiredRelease, err := cl.Upgrade(obj.GetName(), hd.releaseNamespace, chrt, values, func(upgrade *action.Upgrade) error {
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

func (bd *helmDeployer) fetchChart(fs afero.Fs, bundleDeployment *v1alpha2.BundleDeployment) (*chart.Chart, chartutil.Values, error) {
	format := bundleDeployment.Spec.Format
	switch format {
	case v1alpha2.FormatHelm:
		return getChartFromHelmBundle(fs, bundleDeployment)
	case v1alpha2.FormatRegistryV1:
		return getChartFromRegistryBundle(fs, *bundleDeployment)
	case v1alpha2.FormatPlain:
		return getChartFromPlainBundle()
	default:
		return nil, nil, errors.New("unknown format to convert into chart")
	}
}

func getChartFromHelmBundle(chartfs afero.Fs, bundleDeployment *v1alpha2.BundleDeployment) (*chart.Chart, chartutil.Values, error) {
	values, err := loadValues(bundleDeployment)
	if err != nil {
		return nil, nil, err
	}

	pr, pw := io.Pipe()
	var eg errgroup.Group
	eg.Go(func() error {
		return pw.CloseWithError(util.AferoFSToTarGZ(pw, chartfs))
	})

	var chrt *chart.Chart
	eg.Go(func() error {
		var err error
		chrt, err = loader.LoadArchive(pr)
		if err != nil {
			return err
		}
		return chrt.Validate()
	})
	if err := eg.Wait(); err != nil {
		return nil, nil, err
	}
	return chrt, values, nil
}

func loadValues(bd *v1alpha2.BundleDeployment) (chartutil.Values, error) {
	data, err := json.Marshal(bd.Spec.Config)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON for deployment config: %v", err)
	}
	var config map[string]string
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse deployment config: %v", err)
	}
	valuesString := config["values"]

	var values chartutil.Values
	if valuesString == "" {
		return nil, nil
	}

	values, err = chartutil.ReadValues([]byte(valuesString))
	if err != nil {
		return nil, fmt.Errorf("read chart values: %v", err)
	}
	return values, nil
}

func getChartFromRegistryBundle(chartfs afero.Fs, bd v1alpha2.BundleDeployment) (*chart.Chart, chartutil.Values, error) {
	plainFS, err := convert.RegistryV1ToPlain(chartfs)
	if err != nil {
		return nil, nil, fmt.Errorf("error converting registry+v1 bundle to plain+v0 bundle: %v", err)
	}

	objects, err := v1alpha2util.GetBundleObjects(plainFS)
	if err != nil {
		return nil, nil, fmt.Errorf("error fetching objects from bundle manifests: %v", err)
	}

	chrt := &chart.Chart{
		Metadata: &chart.Metadata{},
	}

	for _, obj := range objects {
		obj.SetLabels(util.MergeMaps(obj.GetLabels(), map[string]string{
			util.CoreOwnerKindKey: v1alpha2.BundleDeploymentKind,
			util.CoreOwnerNameKey: bd.Name,
		}))
		yamlData, err := yaml.Marshal(obj)
		if err != nil {
			return nil, nil, err
		}
		hash := sha256.Sum256(yamlData)
		chrt.Templates = append(chrt.Templates, &chart.File{
			Name: fmt.Sprintf("object-%x.yaml", hash[0:8]),
			Data: yamlData,
		})
	}
	return chrt, nil, nil
}

func getChartFromPlainBundle() (*chart.Chart, chartutil.Values, error) {
	return nil, nil, nil
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
