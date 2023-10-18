/*
Copyright 2021.

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

package source

import (
	"archive/tar"
	"os"
	"time"

	"github.com/spf13/afero"

	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
)

// MockStore is intended to be used for testing purposes. It implements store.Store.
type MockStore struct {
	copyTarArchiveFunc func(tr *tar.Reader, destination string) error
	copyDirFSFunc      func(source, destination string) error
	bundleDirectory    string
	bundleDeployment   string
	Fs                 afero.Fs
}

var _ store.Store = &MockStore{}

// Copies contents from a tar reader to the destination on the filesystem
func (m *MockStore) CopyTarArchive(reader *tar.Reader, destination string) error {
	return m.copyTarArchiveFunc(reader, destination)
}

func (m *MockStore) CopyDirFS(source, destination string, _ afero.Fs) error {
	return m.copyDirFSFunc(source, destination)
}

func (m *MockStore) GetBundleDeploymentName() string {
	return m.bundleDeployment
}

func (m *MockStore) GetBundleDirectory() string {
	return m.bundleDirectory
}

func (m *MockStore) Create(name string) (afero.File, error) {
	return m.Fs.Create(name)
}

func (m *MockStore) Mkdir(name string, perm os.FileMode) error {
	return m.Fs.Mkdir(name, perm)
}

func (m *MockStore) MkdirAll(path string, perm os.FileMode) error {
	return m.Fs.MkdirAll(path, perm)
}

func (m *MockStore) Open(name string) (afero.File, error) {
	return m.Fs.Open(name)
}

func (m *MockStore) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return m.Fs.OpenFile(name, flag, perm)
}

func (m *MockStore) Remove(name string) error {
	return m.Fs.Remove(name)
}

func (m *MockStore) RemoveAll(path string) error {
	return m.Fs.RemoveAll(path)
}

func (m *MockStore) Rename(oldname, newname string) error {
	return m.Fs.Rename(oldname, newname)
}

func (m *MockStore) Stat(name string) (os.FileInfo, error) {
	return m.Fs.Stat(name)
}

func (m *MockStore) Name() string {
	return m.Fs.Name()
}

func (m *MockStore) Chmod(name string, mode os.FileMode) error {
	return m.Fs.Chmod(name, mode)
}

func (m *MockStore) Chown(name string, uid, gid int) error {
	return m.Fs.Chown(name, uid, gid)
}

func (m *MockStore) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return m.Fs.Chtimes(name, atime, mtime)
}
