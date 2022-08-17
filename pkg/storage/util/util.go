/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/operator-framework/rukpak/internal/storage"
	"github.com/operator-framework/rukpak/internal/util"
	pkgstorage "github.com/operator-framework/rukpak/pkg/storage"
)

// DefaultBundleCacheDir is a default directory in the bundle pod that holds loaded bundles.
const DefaultBundleCacheDir = "/var/cache/bundles"

func NewBundleStorage(mgr ctrl.Manager, provisionerStorageDirectory, httpExternalAddr, bundlesPath string, opts ...HTTPOption) (pkgstorage.Storage, error) {
	storageURL, err := url.Parse(fmt.Sprintf("%s/%s/", httpExternalAddr, bundlesPath))
	if err != nil {
		return nil, err
	}
	// URL.JoinPath needs go1.19 or later
	// storageURL, err := url.Parse(httpExternalAddr)
	// if err != nil {
	// 	return nil, err
	// }
	// storageURL = storageURL.JoinPath("/", bundlesPath, "/")

	localStorage := &storage.LocalDirectory{
		RootDirectory: provisionerStorageDirectory,
		URL:           *storageURL,
	}

	httpLoader := &storage.HTTP{Client: http.Client{
		Timeout:   time.Minute,
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
	}}
	for _, f := range opts {
		f(httpLoader)
	}

	return pkgstorage.WithFallbackLoader(localStorage, httpLoader), nil
}

type HTTPOption func(*storage.HTTP)

func WithInsecureSkipVerify(v bool) HTTPOption {
	return func(s *storage.HTTP) {
		tr := s.Client.Transport.(*http.Transport)
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{}
		}
		tr.TLSClientConfig.InsecureSkipVerify = v
	}
}

func WithRootCAs(bundleCAFile io.Reader) HTTPOption {
	return func(s *storage.HTTP) {
		var rootCAs *x509.CertPool
		if bundleCAFile != nil {
			var err error
			if rootCAs, err = util.LoadCertPool(bundleCAFile); err != nil {
				return
			}
		}
		tr := s.Client.Transport.(*http.Transport)
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{}
		}
		tr.TLSClientConfig.RootCAs = rootCAs
	}
}

func WithBearerToken(token string) HTTPOption {
	return func(s *storage.HTTP) {
		s.RequestOpts = append(s.RequestOpts, func(request *http.Request) {
			request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		})
	}
}

func NewHTTP(opts ...HTTPOption) *storage.HTTP {
	s := &storage.HTTP{Client: http.Client{
		Timeout:   time.Minute,
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
	}}
	for _, f := range opts {
		f(s)
	}
	return s
}
