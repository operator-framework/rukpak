package controllers

import (
	"io"
	"io/fs"

	"golang.org/x/sync/errgroup"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"

	"github.com/operator-framework/rukpak/internal/util"
)

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
