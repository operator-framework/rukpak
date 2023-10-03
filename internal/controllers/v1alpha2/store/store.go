package store

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	cp "github.com/otiai10/copy"

	"github.com/spf13/afero"
)

type Store interface {
	afero.Fs
	GetBundleDirectory() string
	GetBundleDeploymentName() string
	CopyDir(source, destination string) error
	CopyTarArchive(reader *tar.Reader, destination string) error
}

var (
	ErrCopyContents = errors.New("error copying contents")
)

type bundleDeploymentStore struct {
	bundleDeploymentName string

	baseDirectory string

	fs afero.Fs
}

var _ Store = &bundleDeploymentStore{}

func NewBundleDeploymentStore(basepath, bundleDeploymentName string) (*bundleDeploymentStore, error) {
	if err := os.RemoveAll(filepath.Join(basepath, bundleDeploymentName)); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(basepath, bundleDeploymentName), 0755); err != nil {
		return nil, err
	}

	return &bundleDeploymentStore{
		bundleDeploymentName: bundleDeploymentName,
		baseDirectory:        filepath.Join(basepath, bundleDeploymentName),
		fs:                   afero.NewBasePathFs(afero.NewOsFs(), filepath.Join(basepath, bundleDeploymentName)),
	}, nil
}

func (b *bundleDeploymentStore) CopyTarArchive(reader *tar.Reader, destination string) error {
	if reader == nil {
		return fmt.Errorf("%w for bundle deployment %q", ErrCopyContents, b.GetBundleDeploymentName())
	}

	dst := filepath.Clean(destination)
	if dst == "." || dst == "/" {
		dst = ""
	}

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}

		if err != nil {
			return err
		}

		if header.Typeflag == tar.TypeDir {
			if err := b.fs.MkdirAll(filepath.Join(dst, header.Name), 0755); err != nil {
				return fmt.Errorf("%w: %v", ErrCopyContents, err)
			}
		} else if header.Typeflag == tar.TypeReg {
			// If it is a regular file, create the path and copy data.
			// The header stream is not sorted to go over the directories and then
			// the files. In case, a file is encountered which does not have a parent
			// when we try to copy contents from the reader, we would error. So, verify if the
			// parent exists and then copy contents.

			if err := ensureParentDirExists(b.fs, filepath.Join(dst, header.Name)); err != nil {
				return fmt.Errorf("%w: %v", ErrCopyContents, err)
			}

			file, err := b.fs.Create(filepath.Join(header.Name))
			if err != nil {
				return fmt.Errorf("%w: %v", ErrCopyContents, err)
			}

			if _, err := io.Copy(file, reader); err != nil {
				return fmt.Errorf("%w: %v", ErrCopyContents, err)
			}
			file.Close()
		} else {
			return fmt.Errorf("%w: unsupported tar entry type %q: %q", ErrCopyContents, header.Name, header.Typeflag)
		}
	}
	return nil
}

func ensureParentDirExists(fs afero.Fs, header string) error {
	parent := filepath.Dir(header)

	// indicates that the file is in cwd
	if parent == "." || parent == "" {
		return nil
	}
	return fs.MkdirAll(parent, 0755)
}

func (b *bundleDeploymentStore) CopyDir(source, destination string) error {
	return cp.Copy(source, destination, cp.Options{
		OnDirExists: func(src, dest string) cp.DirExistsAction {
			return cp.Merge
		},
	})
}

func (b *bundleDeploymentStore) GetBundleDeploymentName() string {
	return b.bundleDeploymentName
}

func (b *bundleDeploymentStore) GetBundleDirectory() string {
	return b.baseDirectory
}

func (b *bundleDeploymentStore) Create(name string) (afero.File, error) {
	return b.fs.Create(name)
}

func (b *bundleDeploymentStore) Mkdir(name string, perm os.FileMode) error {
	return b.fs.Mkdir(name, perm)
}

func (b *bundleDeploymentStore) MkdirAll(path string, perm os.FileMode) error {
	return b.fs.MkdirAll(path, perm)
}

func (b *bundleDeploymentStore) Open(name string) (afero.File, error) {
	return b.fs.Open(name)
}

func (b *bundleDeploymentStore) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return b.fs.OpenFile(name, flag, perm)
}

func (b *bundleDeploymentStore) Remove(name string) error {
	return b.fs.Remove(name)
}

func (b *bundleDeploymentStore) RemoveAll(path string) error {
	return b.fs.RemoveAll(path)
}

func (b *bundleDeploymentStore) Rename(oldname, newname string) error {
	return b.fs.Rename(oldname, newname)
}

func (b *bundleDeploymentStore) Stat(name string) (os.FileInfo, error) {
	return b.fs.Stat(name)
}

func (b *bundleDeploymentStore) Name() string {
	return b.fs.Name()
}

func (b *bundleDeploymentStore) Chmod(name string, mode os.FileMode) error {
	return b.fs.Chmod(name, mode)
}

func (b *bundleDeploymentStore) Chown(name string, uid, gid int) error {
	return b.fs.Chown(name, uid, gid)
}

func (b *bundleDeploymentStore) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return b.fs.Chtimes(name, atime, mtime)
}
