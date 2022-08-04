package source

import (
	"compress/gzip"
	"context"
	"fmt"
	"net/http"

	"github.com/nlepage/go-tarfs"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

// Upload is a bundle source that sources bundles from the rukpak upload service.
type Upload struct {
	baseDownloadURL string
	bearerToken     string
	client          http.Client
}

// Unpack unpacks an uploaded bundle by requesting the bundle contents from a web server hosted
// by rukpak's upload service.
func (b *Upload) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeUpload {
		return nil, fmt.Errorf("cannot unpack source type %q with %q unpacker", bundle.Spec.Source.Type, rukpakv1alpha1.SourceTypeUpload)
	}

	url := fmt.Sprintf("%s/uploads/%s.tgz", b.baseDownloadURL, bundle.Name)
	action := fmt.Sprintf("%s %s", http.MethodGet, url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create http request %q for bundle content: %v", action, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", b.bearerToken))
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: http request for bundle content failed: %v", action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return &Result{State: StatePending, Message: "waiting for bundle to be uploaded"}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: unexpected status %q", action, resp.Status)
	}
	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response as gzip: %v", err)
	}
	bundleFS, err := tarfs.New(gzipReader)
	if err != nil {
		return nil, fmt.Errorf("untar bundle contents from response: %v", err)
	}
	return &Result{Bundle: bundleFS, ResolvedSource: bundle.Spec.Source.DeepCopy(), State: StateUnpacked}, nil
}
