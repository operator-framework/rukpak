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
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/afero"
)

func TestStore(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Store suite")
}

var _ = Describe("Store suite", func() {
	var (
		bdStore = bundledeploymentStore{}
		// Since there is no abstraction in spf13/afero
		// to mock afero.Fs, using an in-memory fs.
		baseUnpackPath = "var/cache/bundle"
		bdName         = "test-bd"
		testFs         = afero.NewBasePathFs(afero.NewMemMapFs(), filepath.Join(baseUnpackPath, bdName))
	)

	BeforeEach(func() {
		bdStore = bundledeploymentStore{
			bdName,
			baseUnpackPath,
			testFs,
		}
	})

	Describe("NewBundleDeploymentStore", func() {
		It("should create a new directory successfully", func() {
			store, err := NewBundleDeploymentStore(baseUnpackPath, bdName, testFs)
			Expect(err).NotTo(HaveOccurred())
			Expect(store).NotTo(BeNil())

			dirExists, _ := afero.DirExists(testFs, filepath.Join(baseUnpackPath, bdName))
			Expect(dirExists).To(BeTrue())
		})

		It("should create a new directory successfully when the old directory exists", func() {
			Expect(testFs.MkdirAll(filepath.Join(baseUnpackPath, bdName), 0755)).To(Succeed())

			store, err := NewBundleDeploymentStore(baseUnpackPath, bdName, testFs)
			Expect(err).NotTo(HaveOccurred())
			Expect(store).NotTo(BeNil())

			dirExists, _ := afero.DirExists(testFs, filepath.Join(baseUnpackPath, bdName))
			Expect(dirExists).To(BeTrue())
		})

		It("should error when a file system is not defined", func() {
			store, err := NewBundleDeploymentStore(baseUnpackPath, bdName, nil)
			Expect(err).To(HaveOccurred())
			Expect(store).To(BeNil())
		})
	})

	Describe("CopyTarArchive", func() {
		It("should copy directories and files", func() {
			By("creating a sample tar archive")
			tarData := createTestTarArchive()
			tarReader := tar.NewReader(bytes.NewReader(tarData))

			destination := "test-copy"
			Expect(bdStore.CopyTarArchive(tarReader, destination)).To(Succeed())

			By("check if the copied directory and file exists")
			dirExists, _ := afero.DirExists(bdStore, destination)
			Expect(dirExists).To(BeTrue())

			fileExists, _ := afero.Exists(bdStore, filepath.Join(destination, "test-file.txt"))
			Expect(fileExists).To(BeTrue())

			By("verifying if the contents are same")
			content, err := afero.ReadFile(bdStore, filepath.Join(destination, "test-file.txt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(content)).To(BeEquivalentTo("This is a test file."))
		})

		It("should error when a nil tar reader is passed", func() {
			err := bdStore.CopyTarArchive(nil, "test")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrCopyContents)).To(BeTrue())
		})

		It("should error when a tar reader contains unsupported header type", func() {
			By("creating an unsupported sample tar archive")
			tarData := createUnsupportedTarArchive()
			tarReader := tar.NewReader(bytes.NewReader(tarData))

			destination := "test-copy"
			err := bdStore.CopyTarArchive(tarReader, destination)
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, ErrCopyContents)).To(BeTrue())
			Expect(err.Error()).To(ContainSubstring("unsupported tar entry type"))
		})

	})

})

// Create a sample tar archive with a directory and a file.
func createTestTarArchive() []byte {
	var (
		buf bytes.Buffer
		tw  = tar.NewWriter(&buf)
	)

	// Directory
	header := &tar.Header{
		Name:     "test-dir/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
		Size:     int64(0),
	}
	Expect(tw.WriteHeader(header)).NotTo(HaveOccurred())

	content := "This is a test file."
	header = &tar.Header{
		Name:     "test-file.txt",
		Typeflag: tar.TypeReg,
		Mode:     0644,
		Size:     int64(len(content)),
	}
	Expect(tw.WriteHeader(header)).NotTo(HaveOccurred())

	_, err := tw.Write([]byte(content))
	Expect(err).NotTo(HaveOccurred())

	tw.Close()
	return buf.Bytes()
}

// Create a sample tar archive with unsupported
// header.
func createUnsupportedTarArchive() []byte {
	var (
		buf bytes.Buffer
		tw  = tar.NewWriter(&buf)
	)

	// unsupported header flag.
	header := &tar.Header{
		Name:     "error-file/",
		Typeflag: tar.TypeLink,
		Mode:     0644,
		Size:     int64(len("test")),
	}
	Expect(tw.WriteHeader(header)).To(Not(HaveOccurred()))
	_, err := tw.Write([]byte("test"))
	Expect(err).NotTo(HaveOccurred())

	tw.Close()
	return buf.Bytes()
}
