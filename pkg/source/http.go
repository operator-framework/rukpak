package source

import (
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/nlepage/go-tarfs"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HTTPUnpackerOption func(h *hTTP)

// WithHTTPSecretNamespace configures the namespace that the
// HTTP Unpacker uses to find the Secret used for authorization
// if authorization is specified in the HTTPSource
func WithHTTPSecretNamespace(ns string) HTTPUnpackerOption {
	return func(h *hTTP) {
		h.SecretNamespace = ns
	}
}

// NewHTTPUnpacker returns a new Unpacker for unpacking sources of type "http"
func NewHTTPUnpacker(reader client.Reader, opts ...HTTPUnpackerOption) Unpacker {
	h := &hTTP{
		Reader:          reader,
		SecretNamespace: "default",
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

// http is a src source that sources bundles from the specified url.
type hTTP struct {
	Reader          client.Reader
	SecretNamespace string
}

// Unpack unpacks a src by requesting the src contents from a specified URL
func (b *hTTP) Unpack(ctx context.Context, src *Source, _ client.Object) (*Result, error) {
	if src.Type != SourceTypeHTTP {
		return nil, fmt.Errorf("cannot unpack source type %q with %q unpacker", src.Type, SourceTypeHTTP)
	}

	url := src.HTTP.URL
	action := fmt.Sprintf("%s %s", http.MethodGet, url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create http request %q for source content: %v", action, err)
	}
	var userName, password string
	if src.HTTP.Auth.Secret.Name != "" {
		userName, password, err = b.getCredentials(ctx, src)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(userName, password)
	}

	httpClient := http.Client{Timeout: 10 * time.Second}
	if src.HTTP.Auth.InsecureSkipVerify {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // nolint:gosec
		httpClient.Transport = tr
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: http request for source content failed: %v", action, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: unexpected status %q", action, resp.Status)
	}

	tarReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, err
	}
	fs, err := tarfs.New(tarReader)
	if err != nil {
		return nil, fmt.Errorf("error creating FS: %s", err)
	}

	message := generateMessage("http")

	return &Result{FS: fs, ResolvedSource: src.DeepCopy(), State: StateUnpacked, Message: message}, nil
}

// getCredentials reads credentials from the secret specified in the src
// It returns the username ane password when they are in the secret
func (b *hTTP) getCredentials(ctx context.Context, src *Source) (string, string, error) {
	secret := &corev1.Secret{}
	err := b.Reader.Get(ctx, client.ObjectKey{Namespace: b.SecretNamespace, Name: src.HTTP.Auth.Secret.Name}, secret)
	if err != nil {
		return "", "", err
	}
	userName := string(secret.Data["username"])
	password := string(secret.Data["password"])

	return userName, password, nil
}
