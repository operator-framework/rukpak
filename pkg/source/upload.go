package source

import (
	"compress/gzip"
	"context"
	"fmt"
	"net/http"

	"github.com/nlepage/go-tarfs"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//TODO: Add comments to improve the godocs

type UploadUnpackerOption func(u *upload)

func WithBaseDownloadURL(url string) UploadUnpackerOption {
	return func(u *upload) {
		u.baseDownloadURL = url
	}
}

func WithBearerToken(token string) UploadUnpackerOption {
	return func(u *upload) {
		u.bearerToken = token
	}
}

// NewUploadUnpacker returns a new Unpacker for unpacking sources of type "upload"
func NewUploadUnpacker(cli http.Client, opts ...UploadUnpackerOption) Unpacker {
	u := &upload{
		client: cli,
	}

	for _, opt := range opts {
		opt(u)
	}

	return u
}

// upload is a source that sources from the rukpak upload service.
type upload struct {
	baseDownloadURL string
	bearerToken     string
	client          http.Client
}

// Unpack unpacks an uploaded source by requesting the source contents from a web server hosted
// by rukpak's upload service.
func (b *upload) Unpack(ctx context.Context, src *Source, obj client.Object) (*Result, error) {
	if src.Type != SourceTypeUpload {
		return nil, fmt.Errorf("cannot unpack source type %q with %q unpacker", src.Type, SourceTypeUpload)
	}

	url := fmt.Sprintf("%s/uploads/%s.tgz", b.baseDownloadURL, obj.GetName())
	action := fmt.Sprintf("%s %s", http.MethodGet, url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create http request %q for src content: %v", action, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", b.bearerToken))
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: http request for src content failed: %v", action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return &Result{State: StatePending, Message: "waiting for src to be uploaded"}, nil
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
		return nil, fmt.Errorf("untar src contents from response: %v", err)
	}

	message := generateMessage("upload")

	return &Result{FS: bundleFS, ResolvedSource: src.DeepCopy(), State: StateUnpacked, Message: message}, nil
}
