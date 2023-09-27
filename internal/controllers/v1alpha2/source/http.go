package source

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/spf13/afero"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// http is a bundle source that sources bundles from the specified utrl.
type HTTP struct {
	client.Reader
	SecretNamespace string
}

// Unpack unpacks a bundle by requesting the bundle contents from the specified URL
func (h *HTTP) Unpack(ctx context.Context, bdName string, bdSrc v1alpha2.BundleDeplopymentSource, base afero.Fs, opts UnpackOption) (*Result, error) {
	// Validate inputs
	if err := h.validate(bdSrc); err != nil {
		return nil, fmt.Errorf("validating inputs for bundle deployment source %v", err)
	}

	url := bdSrc.HTTP.URL
	action := fmt.Sprintf("%s %s", http.MethodGet, url)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create http request %q for bundle content: %v", action, err)
	}

	var userName, password string
	if bdSrc.HTTP.Auth.Secret.Name != "" {
		userName, password, err = h.getCredentials(ctx, &bdSrc)
		if err != nil {
			return nil, err
		}
		req.SetBasicAuth(userName, password)
	}

	httpClient := http.Client{Timeout: 10 * time.Second}
	if bdSrc.HTTP.Auth.InsecureSkipVerify {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // noline:gosec
		httpClient.Transport = tr
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: http request for bundle content failed: %v", action, err)
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: unexpected status %q", action, resp.Status)
	}

	if err := (base).RemoveAll(filepath.Clean(bdSrc.Destination)); err != nil {
		return nil, fmt.Errorf("removing dir %v", err)
	}

	if err := base.MkdirAll(bdSrc.Destination, 0755); err != nil {
		return nil, fmt.Errorf("creating storagepath %q", err)
	}

	gzr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading bundle content gzip: %v", err)
	}

	// create a tar reader to read the compressed data
	tr := tar.NewReader(gzr)

	// TODO: if chart.yaml exists in cwd, append a parent.
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("storing content locally: %v", err)
		}

		// create file or directory in the file system
		if header.Typeflag == tar.TypeDir {
			if err := base.MkdirAll(filepath.Join(header.Name), 0755); err != nil {
				return nil, fmt.Errorf("creating directory for storing bundle contents: %v", err)
			}
		} else if header.Typeflag == tar.TypeReg {
			// If it is a regular file, create the path and copy data.
			// The header stream is not sorted to go over the directories and then
			// the files. In case, a file is encountered which does not have a parent
			// when we try to copy contents from the reader, we would error. So, verify if the
			// parent exists and then copy contents.

			if err := ensureParentDirExists(base, header.Name); err != nil {
				return nil, fmt.Errorf("creating parent directory: %v", err)
			}

			file, err := base.Create(filepath.Join(header.Name))
			if err != nil {
				return nil, fmt.Errorf("creating file for storing bundle contents: %v", err)
			}

			if _, err := io.Copy(file, tr); err != nil {
				return nil, fmt.Errorf("copying contents: %v", err)
			}
			file.Close()
		} else {
			return nil, fmt.Errorf("unsupported tar entry type for %s: %v while unpacking", header.Name, header.Typeflag)
		}
	}
	return &Result{ResolvedSource: bdSrc.DeepCopy(), State: StateUnpacked, Message: "Successfully unpacked the http Bundle"}, nil
}

func ensureParentDirExists(fs afero.Fs, header string) error {
	parent := filepath.Dir(header)

	// indicates that the file is in cwd
	if parent == "." || parent == "" {
		return nil
	}
	return fs.MkdirAll(parent, 0755)
}

func (h *HTTP) validate(bundleSrc v1alpha2.BundleDeplopymentSource) error {
	if bundleSrc.Kind != v1alpha2.SourceKindHTTP {
		return fmt.Errorf("bundle source type %q not supported", bundleSrc.Kind)
	}

	if bundleSrc.HTTP.URL == "" {
		return errors.New("URL needs to be specified for HTTP source")
	}
	return nil
}

// getCredentials reads credentials from the secret specified in the bundle.
// It returns the username and password when they are in the secret
func (h *HTTP) getCredentials(ctx context.Context, bundleSrc *v1alpha2.BundleDeplopymentSource) (string, string, error) {
	secret := &corev1.Secret{}
	err := h.Get(ctx, client.ObjectKey{Namespace: h.SecretNamespace, Name: bundleSrc.HTTP.Auth.Secret.Name}, secret)
	if err != nil {
		return "", "", err
	}

	userName := string(secret.Data["username"])
	password := string(secret.Data["password"])

	return userName, password, nil
}
