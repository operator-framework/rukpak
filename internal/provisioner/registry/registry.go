package registry

import (
	"context"
	"fmt"
	"io/fs"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/convert"
	"github.com/operator-framework/rukpak/internal/provisioner/plain"
)

const (
	// ProvisionerID is the unique registry provisioner ID
	ProvisionerID = "core-rukpak-io-registry"
)

func HandleBundleDeployment(ctx context.Context, fsys fs.FS, bd *rukpakv1alpha2.BundleDeployment) (*chart.Chart, chartutil.Values, error) {
	plainFS, err := convert.RegistryV1ToPlain(fsys, bd.Spec.WatchNamespaces)
	if err != nil {
		return nil, nil, fmt.Errorf("convert registry+v1 bundle to plain+v0 bundle: %v", err)
	}
	return plain.HandleBundleDeployment(ctx, plainFS, bd)
}
