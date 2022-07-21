package rukpakctl

import (
	"bytes"
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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/internal/util"
)

type BundleUploader struct {
	UploadServiceName      string
	UploadServiceNamespace string

	Cfg       *rest.Config
	RootCAs   *x509.CertPool
	APIReader client.Reader
}

func (bu *BundleUploader) Upload(ctx context.Context, bundleName string, bundleFS fs.FS) error {
	ctx, cancel := context.WithCancel(ctx)
	eg, ctx := errgroup.WithContext(ctx)
	pf := NewServicePortForwarder(bu.Cfg, bu.APIReader, types.NamespacedName{Namespace: bu.UploadServiceNamespace, Name: bu.UploadServiceName}, intstr.FromString("https"))
	eg.Go(func() error {
		return pf.Start(ctx)
	})

	eg.Go(func() error {
		// get the local port. this will wait until the port forwarder is ready.
		localPort, err := pf.LocalPort(ctx)
		if err != nil {
			return ctx.Err()
		}

		bundleTgz := &bytes.Buffer{}
		if err := util.FSToTarGZ(bundleTgz, bundleFS); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, proxyBundleURL(bundleName, localPort), bundleTgz)
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

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			if len(string(body)) > 0 {
				return errors.New(string(body))
			}
			return fmt.Errorf("unexpected response %q", resp.Status)
		}
		cancel()
		return nil
	})
	if err := eg.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func proxyBundleURL(bundleName string, port uint16) string {
	return fmt.Sprintf("https://localhost:%d/bundles/%s", port, fmt.Sprintf("%s.tgz", bundleName))
}
