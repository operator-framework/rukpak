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

	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
	"github.com/spf13/afero"
)

// mockStore is intended to be used for testing purposes. It implements store.Store.
type mockStore struct {
	copyTarArchiveFunc func(tr *tar.Reader, destination string) error
	copyDirFSFunc      func(source, destination string) error
	bundleDirectory    string
	bundleDeployment   string
	fs                 afero.Fs
}

var _ store.Store = &mockStore{}

// Copies contents from a tar reader to the destination on the filesystem
func (m *mockStore) CopyTarArchive(reader *tar.Reader, destination string) error {
	return m.copyTarArchiveFunc(reader, destination)
}

func (m *mockStore) CopyDirFS(source, destination string, _ afero.Fs) error {
	return m.copyDirFSFunc(source, destination)
}

func (m *mockStore) GetBundleDeploymentName() string {
	return m.bundleDeployment
}

func (m *mockStore) GetBundleDirectory() string {
	return m.bundleDirectory
}

func (m *mockStore) Create(name string) (afero.File, error) {
	return m.fs.Create(name)
}

func (m *mockStore) Mkdir(name string, perm os.FileMode) error {
	return m.fs.Mkdir(name, perm)
}

func (m *mockStore) MkdirAll(path string, perm os.FileMode) error {
	return m.fs.MkdirAll(path, perm)
}

func (m *mockStore) Open(name string) (afero.File, error) {
	return m.fs.Open(name)
}

func (m *mockStore) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	return m.fs.OpenFile(name, flag, perm)
}

func (m *mockStore) Remove(name string) error {
	return m.fs.Remove(name)
}

func (m *mockStore) RemoveAll(path string) error {
	return m.fs.RemoveAll(path)
}

func (m *mockStore) Rename(oldname, newname string) error {
	return m.fs.Rename(oldname, newname)
}

func (m *mockStore) Stat(name string) (os.FileInfo, error) {
	return m.fs.Stat(name)
}

func (m *mockStore) Name() string {
	return m.fs.Name()
}

func (m *mockStore) Chmod(name string, mode os.FileMode) error {
	return m.fs.Chmod(name, mode)
}

func (m *mockStore) Chown(name string, uid, gid int) error {
	return m.fs.Chown(name, uid, gid)
}

func (m *mockStore) Chtimes(name string, atime time.Time, mtime time.Time) error {
	return m.fs.Chtimes(name, atime, mtime)
}
