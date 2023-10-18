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
	"errors"
	"fmt"
	"io"
	"path/filepath"

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

	afero.Fs
}

// NewBundleDeploymentStore returns a local file system abstraction rooted at the provided <base_unpack_path/bundledeployment_name>.
// It removes any pre-existing data at that path.
func NewBundleDeploymentStore(baseUnpackPath, bundledeploymentName string, fs afero.Fs) (*bundledeploymentStore, error) {
	if fs == nil {
		return nil, errors.New("filesystem not defined")
	}

	if err := fs.RemoveAll(filepath.Join(baseUnpackPath, bundledeploymentName)); err != nil {
		return nil, err
	}
	if err := fs.MkdirAll(filepath.Join(baseUnpackPath, bundledeploymentName), 0755); err != nil {
		return nil, err
	}

	return &bundledeploymentStore{bundledeploymentName, filepath.Join(baseUnpackPath, bundledeploymentName), afero.NewBasePathFs(fs, filepath.Join(baseUnpackPath, bundledeploymentName))}, nil
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
			return fmt.Errorf("%w: %v", ErrCopyContents, err)
		}

		if header.Typeflag == tar.TypeDir {
			if err := b.MkdirAll(filepath.Join(dst, filepath.Clean(header.Name)), 0755); err != nil {
				return fmt.Errorf("%w: %v", ErrCopyContents, err)
			}
		} else if header.Typeflag == tar.TypeReg {
			// If it is a regular file, create the path and copy data.
			// The header stream is not sorted to go over the directories and then
			// the files. In case, a file is encountered which does not have a parent
			// when we try to copy contents from the reader, we would error. So, verify if the
			// parent exists and then copy contents.

			if err := ensureParentDirExists(b, filepath.Join(dst, filepath.Clean(header.Name))); err != nil {
				return fmt.Errorf("%w: %v", ErrCopyContents, err)
			}

			file, err := b.Create(filepath.Join(dst, filepath.Clean(header.Name)))
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
