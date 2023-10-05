/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package store

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	cp "github.com/otiai10/copy"
	"github.com/spf13/afero"
)

var _ Store = &bundledeploymentStore{}

// bundledeploymentStore implements the Store interface
// to handle storing of bundle deployment content
type bundledeploymentStore struct {
	// name of the bundle deployment.
	bundledeploymentName string

	// path of the root dir that contains bundle
	// content for the bundledeployment.
	baseDirectory string

	fs afero.Fs
}

// NewBundleDeploymentStore returns a local file system abstraction rooted at the provided <base_unpack_path/bundledeployment_name>.
// It removes any pre-existing data at that path.
func NewBundleDeploymentStore(baseUnpackPath, bundledeploymentName string, fs afero.Fs) (*bundledeploymentStore, error) {
	if err := fs.RemoveAll(filepath.Join(baseUnpackPath, bundledeploymentName)); err != nil {
		return nil, err
	}
	if err := fs.MkdirAll(filepath.Join(baseUnpackPath, bundledeploymentName), 0755); err != nil {
		return nil, err
	}

	return &bundledeploymentStore{
		bundledeploymentName: bundledeploymentName,
		baseDirectory:        filepath.Join(baseUnpackPath, bundledeploymentName),
		fs:                   afero.NewBasePathFs(fs, filepath.Join(baseUnpackPath, bundledeploymentName)),
	}, nil
}

// Copies contents from a tar reader to the destination on the filesystem
func (b *bundledeploymentStore) CopyTarArchive(reader *tar.Reader, destination string) error {
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

			file, err := b.fs.Create(filepath.Join(dst, header.Name))
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

// ensureParentDirExists makes sure that the parent directory for the
// passed header path exists.
func ensureParentDirExists(fs afero.Fs, header string) error {
	parent := filepath.Dir(header)

	// indicates that the file is in cwd
	if parent == "." || parent == "" {
		return nil
	}
	return fs.MkdirAll(parent, 0755)
}

func (b *bundledeploymentStore) CopyDirFS(source, destination string, _ afero.Fs) error {
	return cp.Copy(source, destination, cp.Options{
		OnDirExists: func(src, dest string) cp.DirExistsAction {
			return cp.Merge
		},
	})
}

func (b *bundledeploymentStore) GetBundleDeploymentName() string {
	return b.bundledeploymentName
}

func (b *bundledeploymentStore) GetBundleDirectory() string {
	return b.baseDirectory
}

func (b *bundledeploymentStore) Create(name string) (afero.File, error) {
	return b.fs.Create(name)
}

func (b *bundledeploymentStore) Mkdir(name string, perm os.FileMode) error {
	return b.fs.Mkdir(name, perm)
}

func (b *bundledeploymentStore) MkdirAll(path string, perm os.FileMode) error {
	return b.fs.MkdirAll(path, perm)
}

func (b *bundledeploymentStore) Open(name string) (afero.File, error) {
	return b.fs.Open(name)
}

func (b *bundledeploymentStore) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return b.fs.OpenFile(name, flag, perm)
}

func (b *bundledeploymentStore) Remove(name string) error {
	return b.fs.Remove(name)
}

func (b *bundledeploymentStore) RemoveAll(path string) error {
	return b.fs.RemoveAll(path)
}

func (b *bundledeploymentStore) Rename(oldname, newname string) error {
	return b.fs.Rename(oldname, newname)
}

func (b *bundledeploymentStore) Stat(name string) (os.FileInfo, error) {
	return b.fs.Stat(name)
}

func (b *bundledeploymentStore) Name() string {
	return b.fs.Name()
}

func (b *bundledeploymentStore) Chmod(name string, mode os.FileMode) error {
	return b.fs.Chmod(name, mode)
}

func (b *bundledeploymentStore) Chown(name string, uid, gid int) error {
	return b.fs.Chown(name, uid, gid)
}

func (b *bundledeploymentStore) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return b.fs.Chtimes(name, atime, mtime)
}
