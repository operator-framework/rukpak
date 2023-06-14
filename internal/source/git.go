package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	sshgit "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

type Git struct {
	client.Reader
	SecretNamespace string
}

func (r *Git) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeGit {
		return nil, fmt.Errorf("bundle source type %q not supported", bundle.Spec.Source.Type)
	}
	if bundle.Spec.Source.Git == nil {
		return nil, fmt.Errorf("bundle source git configuration is unset")
	}
	gitsource := bundle.Spec.Source.Git
	if gitsource.Repository == "" {
		// This should never happen because the validation webhook rejects git bundles without repository
		return nil, errors.New("missing git source information: repository must be provided")
	}

	// Set options for clone
	progress := bytes.Buffer{}
	cloneOpts := git.CloneOptions{
		URL:             gitsource.Repository,
		Progress:        &progress,
		Tags:            git.NoTags,
		InsecureSkipTLS: bundle.Spec.Source.Git.Auth.InsecureSkipVerify,
	}

	if bundle.Spec.Source.Git.Auth.Secret.Name != "" {
		auth, err := r.configAuth(ctx, bundle)
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

	// Clone
	repo, err := git.CloneContext(ctx, memory.NewStorage(), memfs.New(), &cloneOpts)
	if err != nil {
		return nil, fmt.Errorf("bundle unpack git clone error: %v - %s", err, progress.String())
	}
	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("bundle unpack error: %v", err)
	}

	// Checkout commit
	if gitsource.Ref.Commit != "" {
		commitHash := plumbing.NewHash(gitsource.Ref.Commit)
		if err := wt.Reset(&git.ResetOptions{
			Commit: commitHash,
			Mode:   git.HardReset,
		}); err != nil {
			return nil, fmt.Errorf("checkout commit %q: %v", commitHash.String(), err)
		}
	}

	var bundleFS fs.FS = &billyFS{wt.Filesystem}

	// Subdirectory
	if gitsource.Directory != "" {
		directory := filepath.Clean(gitsource.Directory)
		if directory[:3] == "../" || directory[0] == '/' {
			return nil, fmt.Errorf("get subdirectory %q for repository %q: %s", gitsource.Directory, gitsource.Repository, "directory can not start with '../' or '/'")
		}
		sub, err := wt.Filesystem.Chroot(filepath.Clean(directory))
		if err != nil {
			return nil, fmt.Errorf("get subdirectory %q for repository %q: %v", gitsource.Directory, gitsource.Repository, err)
		}
		bundleFS = &billyFS{sub}
	}

	commitHash, err := repo.ResolveRevision("HEAD")
	if err != nil {
		return nil, fmt.Errorf("resolve commit hash: %v", err)
	}

	resolvedGit := bundle.Spec.Source.Git.DeepCopy()
	resolvedGit.Ref = rukpakv1alpha1.GitRef{
		Commit: commitHash.String(),
	}

	resolvedSource := &rukpakv1alpha1.BundleSource{
		Type: rukpakv1alpha1.SourceTypeGit,
		Git:  resolvedGit,
	}

	message := generateMessage("git")

	return &Result{Bundle: bundleFS, ResolvedSource: resolvedSource, State: StateUnpacked, Message: message}, nil
}

func (r *Git) configAuth(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (transport.AuthMethod, error) {
	var auth transport.AuthMethod
	if strings.HasPrefix(bundle.Spec.Source.Git.Repository, "http") {
		userName, password, err := r.getCredentials(ctx, bundle)
		if err != nil {
			return nil, err
		}
		return &http.BasicAuth{Username: userName, Password: password}, nil
	}
	privatekey, host, err := r.getCertificate(ctx, bundle)
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
	if bundle.Spec.Source.Git.Auth.InsecureSkipVerify {
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
func (r *Git) getCredentials(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (string, string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.SecretNamespace, Name: bundle.Spec.Source.Git.Auth.Secret.Name}, secret)
	if err != nil {
		return "", "", err
	}
	userName := string(secret.Data["username"])
	password := string(secret.Data["password"])

	return userName, password, nil
}

// getCertificate reads certificate from the secret specified in the bundle
// It returns the privatekey and the entry of the host in known_hosts when they are in the secret
func (r *Git) getCertificate(ctx context.Context, bundle *rukpakv1alpha1.Bundle) ([]byte, []byte, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Namespace: r.SecretNamespace, Name: bundle.Spec.Source.Git.Auth.Secret.Name}, secret)
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
