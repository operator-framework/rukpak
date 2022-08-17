package util

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	bundlesource "github.com/operator-framework/rukpak/internal/source"
	pkgsource "github.com/operator-framework/rukpak/pkg/source"
)

const (
	// uploadClientTimeout is the timeout to be used with http connections to upload manager.
	uploadClientTimeout = time.Second * 10
)

type unpacker struct {
	sources map[rukpakv1alpha1.SourceType]pkgsource.Unpacker
}

// NewUnpacker returns a new composite Source that unpacks bundles using the source
// mapping provided by the configured sources.
func NewUnpacker(sources map[rukpakv1alpha1.SourceType]pkgsource.Unpacker) pkgsource.Unpacker {
	return &unpacker{sources: sources}
}

func (s *unpacker) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*pkgsource.Result, error) {
	source, ok := s.sources[bundle.Spec.Source.Type]
	if !ok {
		return nil, fmt.Errorf("source type %q not supported", bundle.Spec.Source.Type)
	}
	return source.Unpack(ctx, bundle)
}

// NewDefaultUnpacker returns a new composite Source that unpacks bundles using
// a default source mapping with built-in implementations of all of the supported
// source types.
func NewDefaultUnpacker(mgr ctrl.Manager, namespace, unpackImage string, baseUploadManagerURL string, rootCAs *x509.CertPool) (pkgsource.Unpacker, error) {
	cfg := mgr.GetConfig()
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	if httpTransport.TLSClientConfig == nil {
		httpTransport.TLSClientConfig = &tls.Config{}
	}
	httpTransport.TLSClientConfig.RootCAs = rootCAs
	return NewUnpacker(map[rukpakv1alpha1.SourceType]pkgsource.Unpacker{
		rukpakv1alpha1.SourceTypeImage: &bundlesource.Image{
			Client:       mgr.GetClient(),
			KubeClient:   kubeClient,
			PodNamespace: namespace,
			UnpackImage:  unpackImage,
		},
		rukpakv1alpha1.SourceTypeGit: &bundlesource.Git{
			Reader:          mgr.GetAPIReader(),
			SecretNamespace: namespace,
		},
		rukpakv1alpha1.SourceTypeLocal: &bundlesource.Local{
			Client: mgr.GetClient(),
			Reader: mgr.GetAPIReader(),
		},
		rukpakv1alpha1.SourceTypeUpload: bundlesource.NewUpload(
			http.Client{Timeout: uploadClientTimeout, Transport: httpTransport},
			baseUploadManagerURL,
			mgr.GetConfig().BearerToken),
		rukpakv1alpha1.SourceTypeHTTP: &bundlesource.HTTP{
			Reader:          mgr.GetAPIReader(),
			SecretNamespace: namespace,
		},
	}), nil
}
