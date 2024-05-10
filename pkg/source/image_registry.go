package source

import (
	"archive/tar"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd/archive"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	gcrkube "github.com/google/go-containerregistry/pkg/authn/kubernetes"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	rukpakerrors "github.com/operator-framework/rukpak/pkg/errors"
)

// TODO: Make asynchronous

type ImageRegistry struct {
	BaseCachePath string
	AuthNamespace string
}

func (i *ImageRegistry) Unpack(ctx context.Context, bundle *rukpakv1alpha2.BundleDeployment) (*Result, error) {
	l := log.FromContext(ctx)
	if bundle.Spec.Source.Type != rukpakv1alpha2.SourceTypeImage {
		panic(fmt.Sprintf("programmer error: source type %q is unable to handle specified bundle source type %q", rukpakv1alpha2.SourceTypeImage, bundle.Spec.Source.Type))
	}

	if bundle.Spec.Source.Image == nil {
		return nil, rukpakerrors.NewUnrecoverable(fmt.Errorf("error parsing bundle, bundle %s has a nil image source", bundle.Name))
	}

	imgRef, err := name.ParseReference(bundle.Spec.Source.Image.Ref)
	if err != nil {
		return nil, rukpakerrors.NewUnrecoverable(fmt.Errorf("error parsing image reference: %w", err))
	}

	remoteOpts := []remote.Option{}
	if bundle.Spec.Source.Image.ImagePullSecretName != "" {
		chainOpts := k8schain.Options{
			ImagePullSecrets: []string{bundle.Spec.Source.Image.ImagePullSecretName},
			Namespace:        i.AuthNamespace,
			// TODO: Do we want to use any secrets that are included in the rukpak service account?
			// If so, we will need to add the permission to get service accounts and specify
			// the rukpak service account name here.
			ServiceAccountName: gcrkube.NoServiceAccount,
		}
		authChain, err := k8schain.NewInCluster(ctx, chainOpts)
		if err != nil {
			return nil, fmt.Errorf("error getting auth keychain: %w", err)
		}

		remoteOpts = append(remoteOpts, remote.WithAuthFromKeychain(authChain))
	}

	if bundle.Spec.Source.Image.InsecureSkipTLSVerify {
		insecureTransport := remote.DefaultTransport.(*http.Transport).Clone()
		if insecureTransport.TLSClientConfig == nil {
			insecureTransport.TLSClientConfig = &tls.Config{} // nolint:gosec
		}
		insecureTransport.TLSClientConfig.InsecureSkipVerify = true // nolint:gosec
		remoteOpts = append(remoteOpts, remote.WithTransport(insecureTransport))
	}

	digest, isDigest := imgRef.(name.Digest)
	if isDigest {
		hexVal := strings.TrimPrefix(digest.DigestStr(), "sha256:")
		unpackPath := filepath.Join(i.BaseCachePath, bundle.Name, hexVal)
		if stat, err := os.Stat(unpackPath); err == nil && stat.IsDir() {
			l.V(1).Info("found image in filesystem cache", "digest", hexVal)
			return unpackedResult(os.DirFS(unpackPath), bundle, digest.String()), nil
		}
	}

	// always fetch the hash
	imgDesc, err := remote.Head(imgRef, remoteOpts...)
	if err != nil {
		return nil, fmt.Errorf("error fetching image descriptor: %w", err)
	}
	l.V(1).Info("resolved image descriptor", "digest", imgDesc.Digest.String())

	unpackPath := filepath.Join(i.BaseCachePath, bundle.Name, imgDesc.Digest.Hex)
	if _, err = os.Stat(unpackPath); errors.Is(err, os.ErrNotExist) { //nolint: nestif
		// Ensure any previous unpacked bundle is cleaned up before unpacking the new catalog.
		if err := i.Cleanup(ctx, bundle); err != nil {
			return nil, fmt.Errorf("error cleaning up bundle cache: %w", err)
		}

		if err = os.MkdirAll(unpackPath, 0700); err != nil {
			return nil, fmt.Errorf("error creating unpack path: %w", err)
		}

		if err = unpackImage(ctx, imgRef, unpackPath, remoteOpts...); err != nil {
			cleanupErr := os.RemoveAll(unpackPath)
			if cleanupErr != nil {
				err = apimacherrors.NewAggregate(
					[]error{
						err,
						fmt.Errorf("error cleaning up unpack path after unpack failed: %w", cleanupErr),
					},
				)
			}
			return nil, wrapUnrecoverable(fmt.Errorf("error unpacking image: %w", err), isDigest)
		}
	} else if err != nil {
		return nil, fmt.Errorf("error checking if image is in filesystem cache: %w", err)
	}

	resolvedRef := fmt.Sprintf("%s@sha256:%s", imgRef.Context().Name(), imgDesc.Digest.Hex)
	return unpackedResult(os.DirFS(unpackPath), bundle, resolvedRef), nil
}

func wrapUnrecoverable(err error, isUnrecoverable bool) error {
	if isUnrecoverable {
		return rukpakerrors.NewUnrecoverable(err)
	}
	return err
}

func (i *ImageRegistry) Cleanup(_ context.Context, bundle *rukpakv1alpha2.BundleDeployment) error {
	return os.RemoveAll(filepath.Join(i.BaseCachePath, bundle.Name))
}

func unpackedResult(fsys fs.FS, bundle *rukpakv1alpha2.BundleDeployment, ref string) *Result {
	return &Result{
		Bundle: fsys,
		ResolvedSource: &rukpakv1alpha2.BundleSource{
			Type: rukpakv1alpha2.SourceTypeImage,
			Image: &rukpakv1alpha2.ImageSource{
				Ref:                   ref,
				ImagePullSecretName:   bundle.Spec.Source.Image.ImagePullSecretName,
				InsecureSkipTLSVerify: bundle.Spec.Source.Image.InsecureSkipTLSVerify,
			},
		},
		State: StateUnpacked,
	}
}

// unpackImage unpacks a bundle image reference to the provided unpackPath,
// returning an error if any errors are encountered along the way.
func unpackImage(ctx context.Context, imgRef name.Reference, unpackPath string, remoteOpts ...remote.Option) error {
	img, err := remote.Image(imgRef, remoteOpts...)
	if err != nil {
		return fmt.Errorf("error fetching remote image %q: %w", imgRef.Name(), err)
	}

	layers, err := img.Layers()
	if err != nil {
		return fmt.Errorf("error getting image layers: %w", err)
	}

	for _, layer := range layers {
		layerRc, err := layer.Uncompressed()
		if err != nil {
			return fmt.Errorf("error getting uncompressed layer data: %w", err)
		}

		// This filter ensures that the files created have the proper UID and GID
		// for the filesystem they will be stored on to ensure no permission errors occur when attempting to create the
		// files.
		_, err = archive.Apply(ctx, unpackPath, layerRc, archive.WithFilter(func(th *tar.Header) (bool, error) {
			th.Uid = os.Getuid()
			th.Gid = os.Getgid()
			return true, nil
		}))
		if err != nil {
			return fmt.Errorf("error applying layer to archive: %w", err)
		}
	}

	return nil
}
