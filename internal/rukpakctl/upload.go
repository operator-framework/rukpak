package rukpakctl

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"

	"github.com/operator-framework/rukpak/internal/util"
)

// BundleUploader uploads bundle filesystems to rukpak's upload service.
type BundleUploader struct {
	UploadServiceName      string
	UploadServiceNamespace string

	Cfg     *rest.Config
	RootCAs *x509.CertPool
}

// Upload uploads the contents of a bundle to the bundle upload service configured on the
// BundleUploader.
//
// To perform the upload, Upload utilizes a Kubernetes API port-forward to forward a port from
// the uploader service to the local machine. Once the port has been forwarded, Upload
// uploads the bundleFS as the content for the bundle named by the provided bundleName.
//
// Upload returns a boolean value indicating if the bundle content was modified on the server and
// an error value that will convey any errors that occurred during the upload.
//
// Uploads of content that is identical to the existing bundle's content will not result in an error,
// which means this function is idempotent. Running this function multiple times with the same input
// does not result in any change to the cluster state after the initial upload.
func (bu *BundleUploader) Upload(ctx context.Context, bundleName string, bundleFS fs.FS) (bool, error) {
	pf, err := NewServicePortForwarder(bu.Cfg, types.NamespacedName{Namespace: bu.UploadServiceNamespace, Name: bu.UploadServiceName}, intstr.FromString("https"))
	if err != nil {
		return false, err
	}

	// cancel is called by the upload goroutine before it returns, thus ensuring
	// the port-forwarding goroutine exits, which allows the errgroup.Wait() call
	// to unblock.
	ctx, cancel := context.WithCancel(ctx)

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return pf.Start(ctx)
	})

	// Create a pipe to conserve memory. We don't need to buffer the entire bundle
	// tar.gz prior to sending it. To use a pipe, we start a writer goroutine and
	// a reader goroutine such that the reader reads as soon as the writer writes.
	// The reader continues reading until it receives io.EOF, so we need to close
	// writer (triggering the io.EOF) as soon as we finish writing. We close the
	// writer with `bundleWriter.CloseWithError` so that an error encountered
	// writing the FS to a tar.gz stream can be processed by the reader.
	bundleReader, bundleWriter := io.Pipe()
	eg.Go(func() error {
		return bundleWriter.CloseWithError(util.FSToTarGZ(bundleWriter, bundleFS))
	})

	var bundleModified bool
	eg.Go(func() error {
		defer cancel()

		// get the local port. this will wait until the port forwarder is ready.
		localPort, err := pf.LocalPort(ctx)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, proxyBundleURL(bundleName, localPort), bundleReader)
		if err != nil {
			return err
		}

		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig, err = rest.TLSConfigFor(bu.Cfg)
		if err != nil {
			return err
		}
		if bu.RootCAs != nil {
			transport.TLSClientConfig.RootCAs = bu.RootCAs
		}
		if bu.Cfg.BearerToken != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", bu.Cfg.BearerToken))
		}

		httpClient := http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusCreated:
			bundleModified = true
		case http.StatusNoContent:
			bundleModified = false
		default:
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if len(string(body)) > 0 {
				return errors.New(string(body))
			}
			return fmt.Errorf("unexpected response %q", resp.Status)
		}
		return nil
	})
	if err := eg.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return false, err
	}
	return bundleModified, nil
}

func proxyBundleURL(bundleName string, port uint16) string {
	return fmt.Sprintf("https://localhost:%d/uploads/%s", port, fmt.Sprintf("%s.tgz", bundleName))
}
