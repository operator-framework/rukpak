package source

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"

	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/controllers/v1alpha2/store"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
)

// UnpackOptions stores bundle deployment specific options
// that are passed to the unpacker.
// This is currently used to pass the bundle deployment UID
// for the use in image unpacker. But can further be expanded
// to pass other bundle specific options.
type UnpackOption struct {
	BundleDeploymentUID types.UID
}

type Unpacker interface {
	Unpack(ctx context.Context, bundleDeploymentSource v1alpha2.BundleDeplopymentSource, store store.Store, opts UnpackOption) (*Result, error)
}

// Result conveys progress information about unpacking bundle content.
type Result struct {
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

	// Bundle contains the full filesystem of a bundle's root directory.
	Bundle io.Reader

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
	StateUnpackPending State = "Pending"

	// StateUnpacking conveys that the source is currently unpacking a bundle.
	// This state should be used when the bundle contents are being downloaded
	// and processed.
	StateUnpacking State = "Unpacking"

	// StateUnpacked conveys that the bundle has been successfully unpacked.
	StateUnpacked State = "Unpacked"

	// StateUnpackFailed conveys that the unpacking of the bundle has failed.
	StateUnpackFailed State = "Unpack failed"
)

type defaultUnpacker struct {
	systemNsCluster      cluster.Cluster
	namespace            string
	unpackImage          string
	baseUploadManagerURL string
	rootCAs              *x509.CertPool
	sources              map[v1alpha2.SourceKind]Unpacker
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
	sources map[v1alpha2.SourceKind]Unpacker
	bd      *v1alpha2.BundleDeplopymentSource
}

// NewUnpacker returns a new composite Source that unpacks bundles using the source
// mapping provided by the configured sources.
func NewUnpacker(sources map[v1alpha2.SourceKind]Unpacker) Unpacker {
	return &unpacker{sources: sources}
}

// Unpack itrates over the sources specified in bundleDeployment object. Unpacking is done
// for each specified source, the bundle contents are stored in the specified destination.
func (s *unpacker) Unpack(ctx context.Context, bd v1alpha2.BundleDeplopymentSource, store store.Store, opts UnpackOption) (*Result, error) {
	source, ok := s.sources[bd.Kind]
	if !ok {
		return nil, fmt.Errorf("source type %q not supported", bd.Kind)
	}
	return source.Unpack(ctx, bd, store, opts)
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

	cfg := u.systemNsCluster.GetConfig()
	kubeclient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	httpTransport := http.DefaultTransport.(*http.Transport).Clone()
	if httpTransport.TLSClientConfig == nil {
		httpTransport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	httpTransport.TLSClientConfig.RootCAs = u.rootCAs
	return NewUnpacker(map[v1alpha2.SourceKind]Unpacker{
		v1alpha2.SourceKindGit: &Git{
			Reader:          u.systemNsCluster.GetClient(),
			SecretNamespace: u.namespace,
		},
		v1alpha2.SourceKindImage: &Image{
			Client:       u.systemNsCluster.GetClient(),
			KubeClient:   kubeclient,
			PodNamespace: u.namespace,
			UnpackImage:  u.unpackImage,
		},
		v1alpha2.SourceKindHTTP: &HTTP{
			Reader:          u.systemNsCluster.GetClient(),
			SecretNamespace: u.namespace,
		},
	}), nil
}
