package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	sshgit "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-logr/logr"
	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/otiai10/copy"
	"github.com/spf13/afero"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Git struct {
	client.Reader
	SecretNamespace string
	Log             logr.Logger
}

func (r *Git) Unpack(ctx context.Context, bundeDepName string, bundleSrc *v1alpha2.BundleDeplopymentSource, base afero.Fs) (*Result, error) {
	// Validate inputs
	if err := r.validate(bundleSrc); err != nil {
		return nil, fmt.Errorf("unpacking unsuccessful %v", err)
	}

	// Check if cached content exists for this bundle deployment.
	// If it exists copy the contents to the necessary local destination
	// and return the result.
	localcacheDir, err := getCachedContentPath(bundeDepName, bundleSrc, base)
	if err != nil {
		return nil, fmt.Errorf("error verifying if cached content exists %v", err)
	}

	// refers to the full local path where contents need to be stored.
	storagePath := filepath.Join(bundeDepName, filepath.Clean(bundleSrc.Destination))

	if localcacheDir != "" {
		// copy the contents into the destination speified in the source.
		if err := base.RemoveAll(filepath.Clean(bundleSrc.Destination)); err != nil {
			return nil, fmt.Errorf("error removing dir %v", err)
		}
		if err := copy.Copy(filepath.Join(bundeDepName, CacheDir, localcacheDir), storagePath); err != nil {
			return nil, fmt.Errorf("error fetching cached content %v", err)
		}
		// TODO: pass the resolved source that contains the resolved git reference.
		return &Result{ResolvedSource: &v1alpha2.BundleDeplopymentSource{
			Kind: v1alpha2.SourceTypeGit,
		}, State: StateUnpacked, Message: "Re-storing unpacked bundle from cache"}, nil
	}

	// Proceed with downloading content from git.
	gitsource := bundleSrc.Git
	if bundleSrc.Destination == "" {
		bundleSrc.Destination = "/manifests"
	}

	// Set options for clone
	progress := bytes.Buffer{}
	cloneOpts := git.CloneOptions{
		URL:             gitsource.Repository,
		Progress:        &progress,
		Tags:            git.NoTags,
		InsecureSkipTLS: bundleSrc.Git.Auth.InsecureSkipVerify,
	}

	if bundleSrc.Git.Auth.Secret.Name != "" {
		auth, err := r.configAuth(ctx, bundleSrc)
		if err != nil {
			return nil, fmt.Errorf("configuring Auth error: %w", err)
		}
		cloneOpts.Auth = auth
	}
	if gitsource.Ref.Branch != "" {
		cloneOpts.ReferenceName = plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", gitsource.Ref.Branch))
		cloneOpts.SingleBranch = true
		cloneOpts.Depth = 1
	} else if gitsource.Ref.Tag != "" {
		cloneOpts.ReferenceName = plumbing.ReferenceName(fmt.Sprintf("refs/tags/%s", gitsource.Ref.Tag))
		cloneOpts.SingleBranch = true
		cloneOpts.Depth = 1
	}

	// create a destination path to clone the repository to.
	// destination would be <bd-name>/bd.spec.sources.destination.
	// verify if path already exists if so, clean up.
	if err := base.RemoveAll(filepath.Clean(bundleSrc.Destination)); err != nil {
		return nil, fmt.Errorf("error removing contents from local destination %v", err)
	}
	if err := base.MkdirAll(storagePath, 0755); err != nil {
		return nil, fmt.Errorf("error creating storagepath %q", err)
	}

	// clone to local.
	repo, err := git.PlainCloneContext(ctx, storagePath, false, &cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("bundle unpack git clone error: %v - %s", err, progress.String())
	}

	commitHash, err := repo.ResolveRevision("HEAD")
	if err != nil {
		return nil, fmt.Errorf("resolve commit hash: %v", err)
	}

	resolvedGit := bundleSrc.Git.DeepCopy()
	resolvedGit.Ref = v1alpha2.GitRef{
		Commit: commitHash.String(),
	}

	resolvedSource := &v1alpha2.BundleDeplopymentSource{
		Kind: v1alpha2.SourceTypeGit,
		Git:  resolvedGit,
	}

	// cache the downloaded content.
	// TODO: create a glocal cache during setup of the controller
	// and pass it on unpackers. Each bundledeployment should not
	// have its own cache.
	if err := base.Mkdir(localcacheDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating local cached directory %s: %v", localcacheDir, err)
	}
	if err := copy.Copy(storagePath, filepath.Join(bundeDepName, CacheDir, localcacheDir)); err != nil {
		return nil, fmt.Errorf("error storing cached content %v", err)
	}

	return &Result{ResolvedSource: resolvedSource, State: StateUnpacked, Message: "Successfully unpacked git bundle"}, nil
}

func createFile(path string) error {
	fileName := fmt.Sprintf("example-%v", rand.Int())
	filePath := filepath.Join(path, fileName)
	_, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating file %v", err)
	}
	return nil
}

func (r *Git) validate(bundleSrc *v1alpha2.BundleDeplopymentSource) error {
	if bundleSrc.Kind != v1alpha2.SourceTypeGit {
		return fmt.Errorf("bundle source type %q not supported", bundleSrc.Kind)
	}
	if bundleSrc.Git == nil {
		return fmt.Errorf("bundle source git configuration is unset")
	}
	if bundleSrc.Git.Repository == "" {
		return errors.New("missing git source information: repository must be provided")
	}
	return nil
}

func (r *Git) configAuth(ctx context.Context, bundleSrc *v1alpha2.BundleDeplopymentSource) (transport.AuthMethod, error) {
	var auth transport.AuthMethod
	if strings.HasPrefix(bundleSrc.Git.Repository, "http") {
		userName, password, err := r.getCredentials(ctx, bundleSrc.Git.Auth.Secret.Name)
		if err != nil {
			return nil, err
		}
		return &http.BasicAuth{Username: userName, Password: password}, nil
	}

	privatekey, host, err := r.getCertificate(ctx, bundleSrc.Git.Auth.Secret.Name)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(privatekey)
	if err != nil {
		return nil, err
	}
	auth = &sshgit.PublicKeys{
		User:   "git",
		Signer: signer,
	}

	if bundleSrc.Git.Auth.InsecureSkipVerify {
		auth = &sshgit.PublicKeys{
			User:   "git",
			Signer: signer,
			HostKeyCallbackHelper: sshgit.HostKeyCallbackHelper{
				HostKeyCallback: ssh.InsecureIgnoreHostKey(), // nolint:gosec
			},
		}
	} else if host != nil {
		_, _, pubKey, _, _, err := ssh.ParseKnownHosts(host)
		if err != nil {
			return nil, err
		}
		auth = &sshgit.PublicKeys{
			User:   "git",
			Signer: signer,
			HostKeyCallbackHelper: sshgit.HostKeyCallbackHelper{
				HostKeyCallback: ssh.FixedHostKey(pubKey),
			},
		}
	}
	return auth, nil
}

// getCredentials reads credentials from the secret specified in the bundle
// It returns the username ane password when they are in the secret
func (r *Git) getCredentials(ctx context.Context, secretName string) (string, string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.SecretNamespace, Name: secretName}, secret)
	if err != nil {
		return "", "", err
	}
	userName := string(secret.Data["username"])
	password := string(secret.Data["password"])

	return userName, password, nil
}

// getCertificate reads certificate from the secret specified in the bundle
// It returns the privatekey and the entry of the host in known_hosts when they are in the secret
func (r *Git) getCertificate(ctx context.Context, secretName string) ([]byte, []byte, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.SecretNamespace, Name: secretName}, secret)
	if err != nil {
		return nil, nil, err
	}
	return secret.Data["ssh-privatekey"], secret.Data["ssh-knownhosts"], nil
}

// billy.Filesysten -> fs.FS
var (
	_ fs.FS         = &billyFS{}
	_ fs.ReadDirFS  = &billyFS{}
	_ fs.ReadFileFS = &billyFS{}
	_ fs.StatFS     = &billyFS{}
	_ fs.File       = &billyFile{}
)

type billyFS struct {
	billy.Filesystem
}

func (f *billyFS) ReadFile(name string) ([]byte, error) {
	file, err := f.Filesystem.Open(name)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}

func (f *billyFS) Open(path string) (fs.File, error) {
	fi, err := f.Filesystem.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return &billyDirFile{billyFile{nil, fi}, f, path}, nil
	}
	file, err := f.Filesystem.Open(path)
	if err != nil {
		return nil, err
	}
	return &billyFile{file, fi}, nil
}

func (f *billyFS) ReadDir(name string) ([]fs.DirEntry, error) {
	fis, err := f.Filesystem.ReadDir(name)
	if err != nil {
		return nil, err
	}
	entries := make([]fs.DirEntry, 0, len(fis))
	for _, fi := range fis {
		entries = append(entries, fs.FileInfoToDirEntry(fi))
	}
	return entries, nil
}

type billyFile struct {
	billy.File
	fi os.FileInfo
}

func (b billyFile) Stat() (fs.FileInfo, error) {
	return b.fi, nil
}

func (b billyFile) Close() error {
	if b.File == nil {
		return nil
	}
	return b.File.Close()
}

type billyDirFile struct {
	billyFile
	fs   *billyFS
	path string
}

func (d *billyDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	entries, err := d.fs.ReadDir(d.path)
	if n <= 0 || n > len(entries) {
		n = len(entries)
	}
	return entries[:n], err
}

func (d billyDirFile) Read(_ []byte) (int, error) {
	return 0, &fs.PathError{Op: "read", Path: d.path, Err: syscall.EISDIR}
}
