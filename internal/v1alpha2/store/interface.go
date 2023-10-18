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

	"github.com/spf13/afero"
)

// The Store interface defines the required methods that must be implemented
// by the bundle deployment store. These methods facilitate the unpacking and storage
// of contents from all the sources.
type Store interface {
	afero.Fs

	// GetBundleDirectory returns the path where all the bundle contents
	// are unpacked and stored.
	GetBundleDirectory() string

	// GetBundleDeploymentName returns the name of the bundle deployment
	// whose contents are unpacked.
	GetBundleDeploymentName() string

	// CopyDirFS copies contents from source to the destination
	// in the same filesystem.
	CopyDirFS(source, destination string, fs afero.Fs) error

	// CopyTarArchive copies contents from a tar reader to the
	// destination on the filesystem
	CopyTarArchive(reader *tar.Reader, destination string) error
}

var (
	// Any error occurred during copying contents on the bundle deployment
	// store will be wrapped along with this error.
	ErrCopyContents = errors.New("error copying contents")
)
