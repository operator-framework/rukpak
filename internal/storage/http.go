package storage

import (
	"compress/gzip"
	"context"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/nlepage/go-tarfs"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

type HTTP struct {
	Client      http.Client
	RequestOpts []func(*http.Request)
}

func (s *HTTP) Load(ctx context.Context, owner client.Object) (fs.FS, error) {
	bundle := owner.(*rukpakv1alpha1.Bundle)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bundle.Status.ContentURL, nil)
	if err != nil {
		return nil, err
	}
	for _, f := range s.RequestOpts {
		f(req)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response status %q", resp.Status)
	}
	tarReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	return tarfs.New(tarReader)
}
