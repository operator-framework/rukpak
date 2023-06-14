package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"

	"golang.org/x/sync/errgroup"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/util"
)

const (
	// ProvisionerID is the unique helm provisioner ID
	ProvisionerID = "core-rukpak-io-helm"
)

func HandleBundle(_ context.Context, fsys fs.FS, _ *rukpakv1alpha1.Bundle) (fs.FS, error) {
	// Helm expects an FS whose root contains a single chart directory. Depending on how
	// the bundle is sourced, the FS may or may not contain this single chart directory in
	// its root (e.g. charts uploaded via 'rukpakctl run <bdName> <chartDir>') would not.
	// This FS wrapper adds this base directory unless the FS already has a base directory.
	chartFS, err := util.EnsureBaseDirFS(fsys, "chart")
	if err != nil {
		return nil, err
	}

	if _, err = getChart(chartFS); err != nil {
		return nil, err
	}
	return chartFS, nil
}

func HandleBundleDeployment(_ context.Context, fsys fs.FS, bd *rukpakv1alpha1.BundleDeployment) (*chart.Chart, chartutil.Values, error) {
	values, err := loadValues(bd)
	if err != nil {
		return nil, nil, err
	}
	chart, err := getChart(fsys)
	if err != nil {
		return nil, nil, err
	}
	return chart, values, nil
}

func loadValues(bd *rukpakv1alpha1.BundleDeployment) (chartutil.Values, error) {
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

func getChart(chartfs fs.FS) (*chart.Chart, error) {
	pr, pw := io.Pipe()
	var eg errgroup.Group
	eg.Go(func() error {
		return pw.CloseWithError(util.FSToTarGZ(pw, chartfs))
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
		return nil, err
	}
	return chrt, nil
}
