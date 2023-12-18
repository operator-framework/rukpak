package bundledeployment

import (
	"context"
	"io/fs"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

type Handler interface {
	Handle(context.Context, fs.FS, *rukpakv1alpha1.BundleDeployment) (*chart.Chart, chartutil.Values, error)
}

type HandlerFunc func(context.Context, fs.FS, *rukpakv1alpha1.BundleDeployment) (*chart.Chart, chartutil.Values, error)

func (f HandlerFunc) Handle(ctx context.Context, fsys fs.FS, bd *rukpakv1alpha1.BundleDeployment) (*chart.Chart, chartutil.Values, error) {
	return f(ctx, fsys, bd)
}

// TODO: Having two interfaces with same parameters seems unnecessary. This should ideally
// be moved to HandleBundleDeployment. With it, we can remove the additional step of loading from
// store. 
type BundleDeploymentProcessor interface {
	ProcessBundleDeployment(context.Context, fs.FS, *rukpakv1alpha1.BundleDeployment) (fs.FS, error)
}

type BundleDeploymentProcessorFunc func(context.Context, fs.FS, *rukpakv1alpha1.BundleDeployment) (fs.FS, error)

func (f BundleDeploymentProcessorFunc) ProcessBundleDeployment(ctx context.Context, fsys fs.FS, b *rukpakv1alpha1.BundleDeployment) (fs.FS, error) {
	return f(ctx, fsys, b)
}
