package source

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-git/go-billy/v5/memfs"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

// http is a bundle source that sources bundles from the specified url.
type HTTP struct {
	client.Reader
	SecretNamespace string
}

// Unpack unpacks a bundle by requesting the bundle contents from a specified URL
func (b *HTTP) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeHTTP {
		return nil, fmt.Errorf("cannot unpack source type %q with %q unpacker", bundle.Spec.Source.Type, rukpakv1alpha1.SourceTypeHTTP)
	}

	url := bundle.Spec.Source.HTTP.URL
	action := fmt.Sprintf("%s %s", http.MethodGet, url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create http request %q for bundle content: %v", action, err)
	}
	var userName, password string
	if bundle.Spec.Source.HTTP.Auth.Secret.Name != "" {
		userName, password, err = b.getCredentials(ctx, bundle)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(userName, password)
	}

	httpClient := http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
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

	var memFS = memfs.New()
	file, err := memFS.Create(filepath.Join("manifests", "chart"))
	if err != nil {
		return nil, fmt.Errorf("creating filesystem: %s", err)
	}
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return nil, fmt.Errorf("writing chart file: %s", err)
	}
	err = file.Close()
	if err != nil {
		return nil, fmt.Errorf("closing chart file: %s", err)
	}

	var bundleFS fs.FS = &billyFS{memFS}
	return &Result{Bundle: bundleFS, ResolvedSource: bundle.Spec.Source.DeepCopy(), State: StateUnpacked}, nil
}

// getCredentials reads credentials from the secret specified in the bundle
// It returns the username ane password when they are in the secret
func (b *HTTP) getCredentials(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (string, string, error) {
	secret := &corev1.Secret{}
	err := b.Get(ctx, client.ObjectKey{Namespace: b.SecretNamespace, Name: bundle.Spec.Source.HTTP.Auth.Secret.Name}, secret)
	if err != nil {
		return "", "", err
	}
	userName := string(secret.Data["username"])
	password := string(secret.Data["password"])

	return userName, password, nil
}
