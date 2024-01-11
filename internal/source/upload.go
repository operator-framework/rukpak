package source

import (
	"compress/gzip"
	"context"
	"fmt"
	"net/http"

	"github.com/nlepage/go-tarfs"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
)

// Upload is a bundle source that sources bundles from the rukpak upload service.
type Upload struct {
	baseDownloadURL string
	bearerToken     string
	client          http.Client
}

// Unpack unpacks an uploaded bundle by requesting the bundle contents from a web server hosted
// by rukpak's upload service.
func (b *Upload) Unpack(ctx context.Context, bundle *rukpakv1alpha2.BundleDeployment) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha2.SourceTypeUpload {
		return nil, fmt.Errorf("cannot unpack source type %q with %q unpacker", bundle.Spec.Source.Type, rukpakv1alpha2.SourceTypeUpload)
	}

	// Proceed with fetching contents from a web server, only if the bundle upload was successful.
	// If upload is a failure, we have "TypeUploadState" explicitly set to false.
	if !isBundleContentUploaded(bundle) {
		return &Result{State: StatePending, Message: "pending unpacking contents from uploaded bundle"}, nil
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

	message := generateMessage("upload")

	return &Result{Bundle: bundleFS, ResolvedSource: bundle.Spec.Source.DeepCopy(), State: StateUnpacked, Message: message}, nil
}

func isBundleContentUploaded(bd *rukpakv1alpha2.BundleDeployment) bool {
	if bd == nil {
		return false
	}

	condition := meta.FindStatusCondition(bd.Status.Conditions, rukpakv1alpha2.TypeUploadStatus)
	return condition != nil && condition.Status == metav1.ConditionTrue
}
