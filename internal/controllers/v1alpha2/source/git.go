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
	"github.com/operator-framework/rukpak/internal/controllers/v1alpha2/store"
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

const (
	unpackCachePath = "var/cache"
)

func (r *Git) Unpack(ctx context.Context, bundleSrc v1alpha2.BundleDeplopymentSource, store store.Store, opts UnpackOption) (*Result, error) {
	// Validate inputs
	if err := r.validate(bundleSrc); err != nil {
		return nil, fmt.Errorf("unpacking unsuccessful %v", err)
	}

	// Proceed with downloading content from git.
	// validation is already in place.
	gitsource := bundleSrc.Git

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
	if bundleSrc.Destination != "" {
		if err := store.RemoveAll(bundleSrc.Destination); err != nil {
			return nil, fmt.Errorf("error removing contents from local destination %v", err)
		}
		if err := store.MkdirAll(bundleSrc.Destination, 0755); err != nil {
			return nil, fmt.Errorf("error creating storagepath %q", err)
		}
	}

	// TODO: A temp dir is created to clone the git contents, and then copy them
	// into the local dir. This can be reworked to cache contents, so that we need
	// not download again when there is no change.
	tempDir, err := afero.TempDir(store, ".", "git")
	if err != nil {
		return &Result{State: StateUnpackFailed, Message: "Error creating temp dir"}, err
	}

	defer store.RemoveAll(tempDir)

	// refers to the full local path where contents need to be stored.
	// since we are using "github.com/otiai10/copy", we need to add afero's
	// basepath, and construct the full base path for copying.
	storagePath := filepath.Join(unpackCachePath, store.GetBundleDeploymentName(), filepath.Clean(bundleSrc.Destination))
	cacheSrcPath := filepath.Join(unpackCachePath, store.GetBundleDeploymentName(), tempDir)

	// clone to local but in a cache dir.
	repo, err := git.PlainCloneContext(ctx, cacheSrcPath, false, &cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("bundle unpack git clone error: %v - %s", err, progress.String())
	}

	if gitsource.Directory != "" {
		directory := filepath.Clean(gitsource.Directory)
		if directory[:3] == "../" || directory[0] == '/' {
			return nil, fmt.Errorf("get subdirectory %q for repository %q: %s", gitsource.Directory, gitsource.Repository, "directory can not start with '../' or '/'")
		}
		cacheSrcPath = filepath.Join(cacheSrcPath, directory)
	}

	if err := store.CopyDir(cacheSrcPath, storagePath); err != nil {
		return nil, fmt.Errorf("copying contents from cache to local dir: %v", err)
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
		Kind: v1alpha2.SourceKindGit,
		Git:  resolvedGit,
	}
	return &Result{ResolvedSource: resolvedSource, State: StateUnpacked, Message: "Successfully unpacked git bundle"}, nil
}

func (r *Git) validate(bundleSrc v1alpha2.BundleDeplopymentSource) error {
	if bundleSrc.Kind != v1alpha2.SourceKindGit {
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

func createFile(path string) error {
	fileName := fmt.Sprintf("example-%v", rand.Int())
	filePath := filepath.Join(path, fileName)
	_, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("error creating file %v", err)
	}
	return nil
}

func (r *Git) configAuth(ctx context.Context, bundleSrc v1alpha2.BundleDeplopymentSource) (transport.AuthMethod, error) {
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
