package source

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/adrg/xdg"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/pkg/docker/config"
	"github.com/docker/docker/pkg/jsonmessage"
	dockerprogress "github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/mattn/go-isatty"
	"github.com/nlepage/go-tarfs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

type ArtifactImage struct {
}

func (i *ArtifactImage) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeOCIArtifacts {
		return nil, fmt.Errorf("bundle source type %q not supported", bundle.Spec.Source.Type)
	}
	if bundle.Spec.Source.Image == nil {
		return nil, fmt.Errorf("bundle source image configuration is unset")
	}
	src, ref, desc, err := ResolveNameAndReference(ctx, bundle.Spec.Source.Image.Ref)
	if err != nil {
		log.Fatal(err)
	}
	storeDir := filepath.Join(xdg.CacheHome, "rukpak", "store")
	dst, err := oci.NewWithContext(ctx, storeDir)
	if err != nil {
		log.Fatal(err)
	}

	if err := CopyGraphWithProgress(ctx, src, dst, *desc); err != nil {
		return nil, err
	}

	rc, err := dst.Fetch(ctx, *desc)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var a ocispec.Artifact
	if err := json.NewDecoder(rc).Decode(&a); err != nil {
		return nil, err
	}
	var innerRc io.ReadCloser
	for _, blob := range a.Blobs {
		if blob.MediaType != "application/vnd.cncf.operatorframework.olm.bundle.content.v1.tar+gzip" {
			continue
		}
		reg, err := remote.NewRepository(ref.String())
		if err != nil {
			return nil, err
		}
		innerRc, err = reg.Fetch(ctx, ocispec.Descriptor{Digest: blob.Digest, Size: blob.Size})
		if err != nil {
			return nil, err
		}
		defer innerRc.Close()
	}
	resolvedSource := &rukpakv1alpha1.BundleSource{
		Type:     rukpakv1alpha1.SourceTypeOCIArtifacts,
		Artifact: &rukpakv1alpha1.OCIArtifactSource{Ref: bundle.Spec.Source.Image.Ref},
	}
	gzr, err := gzip.NewReader(innerRc)
	if err != nil {
		return nil, fmt.Errorf("read bundle content gzip: %v", err)
	}
	fs, err := tarfs.New(gzr)
	if err != nil {
		return nil, err
	}
	return &Result{Bundle: fs, ResolvedSource: resolvedSource, State: StateUnpacked}, nil
}

func TagOrDigest(ref reference.Reference) (string, error) {
	switch r := ref.(type) {
	case reference.Digested:
		return r.Digest().String(), nil
	case reference.Tagged:
		return r.Tag(), nil
	}
	return "", fmt.Errorf("reference is not tagged or digested")
}

func ParseNameAndReference(nameAndReference string) (*remote.Repository, reference.Named, error) {
	ref, err := reference.ParseNamed(nameAndReference)
	if err != nil {
		return nil, nil, err
	}

	repo, err := NewRepository(ref.Name())
	if err != nil {
		return nil, nil, err
	}
	return repo, ref, nil
}

func NewRepository(repoName string) (*remote.Repository, error) {
	repo, err := remote.NewRepository(repoName)
	if err != nil {
		return nil, err
	}
	repo.Client = &auth.Client{Credential: getCredentials(repoName)}
	return repo, nil
}

func getCredentials(repoName string) func(context.Context, string) (auth.Credential, error) {
	return func(ctx context.Context, _ string) (auth.Credential, error) {
		ref, err := reference.ParseNamed(repoName)
		if err != nil {
			return auth.Credential{}, err
		}
		authConfig, err := config.GetCredentialsForRef(nil, ref)
		if err != nil {
			return auth.Credential{}, err
		}
		return auth.Credential{
			Username: authConfig.Username,
			Password: authConfig.Password,
		}, nil
	}
}

func ResolveNameAndReference(ctx context.Context, nameAndReference string) (*remote.Repository, reference.Reference, *ocispec.Descriptor, error) {
	repo, ref, err := ParseNameAndReference(nameAndReference)
	if err != nil {
		return nil, nil, nil, err
	}

	tagOrDigest, err := TagOrDigest(ref)
	if err != nil {
		return nil, nil, nil, err
	}

	desc, err := repo.Resolve(ctx, tagOrDigest)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to resolve %s: %v", nameAndReference, err)
	}
	return repo, ref, &desc, nil
}

func CopyGraphWithProgress(ctx context.Context, src oras.Target, dst oras.Target, desc ocispec.Descriptor) error {
	pr, pw := io.Pipe()
	fd := os.Stdout.Fd()
	isTTY := isatty.IsTerminal(fd)
	out := streamformatter.NewJSONProgressOutput(pw, !isTTY)
	ps := NewStore(src, out)
	errChan := make(chan error, 1)
	go func() {
		errChan <- jsonmessage.DisplayJSONMessagesStream(pr, os.Stdout, fd, isTTY, nil)
	}()
	opts := oras.CopyGraphOptions{
		Concurrency: runtime.NumCPU(),
		OnCopySkipped: func(ctx context.Context, desc ocispec.Descriptor) error {
			return out.WriteProgress(dockerprogress.Progress{
				ID:     IDForDesc(desc),
				Action: "Artifact is up to date",
			})
		},
		PostCopy: func(_ context.Context, desc ocispec.Descriptor) error {
			return out.WriteProgress(dockerprogress.Progress{
				ID:      IDForDesc(desc),
				Action:  "Complete",
				Current: desc.Size,
				Total:   desc.Size,
			})
		},
	}
	if err := oras.CopyGraph(ctx, ps, dst, desc, opts); err != nil {
		return fmt.Errorf("copy artifact graph: %v", err)
	}
	if err := pw.Close(); err != nil {
		return fmt.Errorf("close progress writer: %v", err)
	}
	if err := <-errChan; err != nil {
		return fmt.Errorf("display progress: %v", err)
	}
	return nil
}

func IDForDesc(desc ocispec.Descriptor) string {
	return desc.Digest.String()[7:19]
}

type Store struct {
	base content.ReadOnlyStorage
	out  dockerprogress.Output
}

func NewStore(base content.ReadOnlyStorage, out dockerprogress.Output) content.ReadOnlyStorage {
	return &Store{
		base: base,
		out:  out,
	}
}

func (s *Store) Exists(ctx context.Context, desc ocispec.Descriptor) (bool, error) {
	return s.base.Exists(ctx, desc)
}

func (s *Store) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	rc, err := s.base.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}

	return dockerprogress.NewProgressReader(rc, s.out, desc.Size, IDForDesc(desc), "Pushing "), nil
}
