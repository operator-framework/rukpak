package source

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

type Unpacker interface {
	Unpack(ctx context.Context, bundleDeploymentName string, bundleDeploymentSource *v1alpha2.BundleDeplopymentSource, fs afero.Fs) (*Result, error)
}

// Result conveys progress information about unpacking bundle content.
type Result struct {
	// Bundle contains the full filesystem of a bundle's root directory.
	Bundle fs.FS

	// ResolvedSource is a reproducible view of a Bundle's Source.
	// When possible, source implementations should return a ResolvedSource
	// that pins the Source such that future fetches of the bundle content can
	// be guaranteed to fetch the exact same bundle content as the original
	// unpack.
	//
	// For example, resolved image sources should reference a container image
	// digest rather than an image tag, and git sources should reference a
	// commit hash rather than a branch or tag.
	ResolvedSource *v1alpha2.BundleDeplopymentSource

	// State is the current state of unpacking the bundle content.
	State State

	// Message is contextual information about the progress of unpacking the
	// bundle content.
	Message string
}

type State string

const (
	// StatePending conveys that a request for unpacking a bundle has been
	// acknowledged, but not yet started.
	StatePending State = "Pending"

	// StateUnpacking conveys that the source is currently unpacking a bundle.
	// This state should be used when the bundle contents are being downloaded
	// and processed.
	StateUnpacking State = "Unpacking"

	// StateUnpacked conveys that the bundle has been successfully unpacked.
	StateUnpacked State = "Unpacked"

	// CacheDir is where the unpacked bundle contents are cached locally.
	CacheDir string = "cache"
)

type defaultUnpacker struct {
	systemNsCluster      cluster.Cluster
	namespace            string
	unpackImage          string
	baseUploadManagerURL string
	rootCAs              *x509.CertPool
	sources              map[v1alpha2.SourceType]Unpacker
}

type UnpackerOption func(*defaultUnpacker)

func WithUnpackImage(image string) UnpackerOption {
	return func(du *defaultUnpacker) {
		du.unpackImage = image
	}
}

func WithBaseUploadManagerURL(url string) UnpackerOption {
	return func(du *defaultUnpacker) {
		du.baseUploadManagerURL = url
	}
}

func WithRootCAs(rootCAs *x509.CertPool) UnpackerOption {
	return func(du *defaultUnpacker) {
		du.rootCAs = rootCAs
	}
}

type unpacker struct {
	sources map[v1alpha2.SourceType]Unpacker
	bd      *v1alpha2.BundleDeplopymentSource
}

// NewUnpacker returns a new composite Source that unpacks bundles using the source
// mapping provided by the configured sources.
func NewUnpacker(sources map[v1alpha2.SourceType]Unpacker) Unpacker {
	return &unpacker{sources: sources}
}

// Unpack itrates over the sources specified in bundleDeployment object. Unpacking is done
// for each specified source, the bundle contents are stored in the specified destination.
func (s *unpacker) Unpack(ctx context.Context, bdDepName string, bd *v1alpha2.BundleDeplopymentSource, fs afero.Fs) (*Result, error) {
	source, ok := s.sources[bd.Kind]
	if !ok {
		return nil, fmt.Errorf("source type %q not supported", bd.Kind)
	}
	return source.Unpack(ctx, bdDepName, bd, fs)
}

func NewDefaultUnpackerWithOpts(systemNsCluster cluster.Cluster, namespace string, opts ...UnpackerOption) (Unpacker, error) {
	unpacker := &defaultUnpacker{
		systemNsCluster: systemNsCluster,
		namespace:       namespace,
	}
	for _, opt := range opts {
		opt(unpacker)
	}
	return unpacker.initialize()

}

func (u *defaultUnpacker) initialize() (Unpacker, error) {
	if u.systemNsCluster == nil {
		return nil, fmt.Errorf("systemNsCluster cannot be empty, cannot initialize")
	}
	// cfg := u.systemNsCluster.GetConfig()
	// kubeClient, err := kubernetes.NewForConfig(cfg)
	// if err != nil {
	// 	return nil, err
	// }

	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	if httpTransport.TLSClientConfig == nil {
		httpTransport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	httpTransport.TLSClientConfig.RootCAs = u.rootCAs
	return NewUnpacker(map[v1alpha2.SourceType]Unpacker{
		v1alpha2.SourceTypeGit: &Git{
			Reader:          u.systemNsCluster.GetClient(),
			SecretNamespace: u.namespace,
		},
	}), nil
}
