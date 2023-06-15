package source

import (
	"context"
	"fmt"
	"runtime"

	"github.com/containers/image/v5/docker/reference"
	v1 "github.com/joelanford/olm-oci/api/v1"
	"github.com/joelanford/olm-oci/pkg/fetch"
	"github.com/joelanford/olm-oci/pkg/remote"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

const DefaultOCICacheDir = "/var/cache/oci"

type OCIArtifact struct {
	LocalStore *oci.Store
}

func (i *OCIArtifact) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeOCIArtifact {
		return nil, fmt.Errorf("bundle source type %q not supported", bundle.Spec.Source.Type)
	}
	if bundle.Spec.Source.OCIArtifact == nil {
		return nil, fmt.Errorf("bundle source artifact configuration is unset")
	}

	// TODO: support image pull secrets for OCI artifacts
	if bundle.Spec.Source.OCIArtifact.ImagePullSecretName != "" {
		return nil, fmt.Errorf("bundle source artifact image pull secret is not currently supported")
	}

	repo, ref, err := remote.ParseNameAndReference(bundle.Spec.Source.OCIArtifact.Ref)
	if err != nil {
		return nil, fmt.Errorf("parse reference: %v", err)
	}

	desc, err := repo.Resolve(ctx, ref.String())
	if err != nil {
		return nil, fmt.Errorf("resolve reference: %v", err)
	}
	if err := oras.CopyGraph(ctx, repo, i.LocalStore, desc, oras.CopyGraphOptions{
		Concurrency: runtime.NumCPU(),
	}); err != nil {
		return nil, fmt.Errorf("pull artifact to local storage: %v", err)
	}

	art, err := fetch.FetchArtifact(ctx, i.LocalStore, desc)
	if err != nil {
		return nil, fmt.Errorf("fetch artifact descriptor: %v", err)
	}

	if art.ArtifactType != v1.MediaTypeBundle {
		return nil, fmt.Errorf("unsupported artifact type %q, expected %q", art.ArtifactType, v1.MediaTypeBundle)
	}

	ociBundle, err := fetch.FetchBundle(ctx, i.LocalStore, art)
	if err != nil {
		return nil, fmt.Errorf("fetch artifact: %v", err)
	}

	digestRef, err := reference.WithDigest(ref, desc.Digest)
	if err != nil {
		return nil, fmt.Errorf("create digest reference: %v", err)
	}
	resolvedSource := &rukpakv1alpha1.BundleSource{
		Type:        rukpakv1alpha1.SourceTypeOCIArtifact,
		OCIArtifact: &rukpakv1alpha1.ImageSource{Ref: digestRef.String()},
	}
	message := generateMessage(bundle.Name)
	return &Result{Bundle: ociBundle.Content.FS, ResolvedSource: resolvedSource, State: StateUnpacked, Message: message}, nil
}
