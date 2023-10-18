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

package validator

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/spf13/afero"

	"github.com/operator-framework/rukpak/internal/v1alpha2/source"
	"github.com/operator-framework/rukpak/internal/v1alpha2/store"
)

func TestValidator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Validator Suite")
}

var _ = Describe("Test Validator", func() {
	var (
		testValidator Validator
		ctx           context.Context
		fs            afero.Fs
	)

	BeforeEach(func() {
		testValidator = NewDefaultValidator()
		ctx = context.Background()
	})

	When("invalid format is provided", func() {
		It("expect error to have occurred with invalid format", func() {
			fs = afero.NewMemMapFs()
			store, err := store.NewBundleDeploymentStore("test", "testbundedeployment", fs)
			Expect(err).NotTo(HaveOccurred())
			err = testValidator.Validate(ctx, "invalid format", store)
			Expect(err).To(HaveOccurred())
		})
	})

	When("valid plain bundle formats are provided", func() {
		It("expect no error to have occurred when valid plain bundle is provided", func() {
			store := source.MockStore{
				Fs: afero.NewBasePathFs(afero.NewOsFs(), "valid_testdata/plain"),
			}
			err := testValidator.Validate(ctx, v1alpha2.FormatPlain, &store)
			Expect(err).NotTo(HaveOccurred())
		})

		It("expect no error to have occurred when valid registryV1 bundle is provided", func() {
			store := source.MockStore{
				Fs: afero.NewBasePathFs(afero.NewOsFs(), "valid_testdata/registryV1"),
			}
			err := testValidator.Validate(ctx, v1alpha2.FormatRegistryV1, &store)
			Expect(err).NotTo(HaveOccurred())
		})

		It("expect no error to have occurred when valid helm bundle is provided", func() {
			store := source.MockStore{
				Fs: afero.NewBasePathFs(afero.NewOsFs(), "valid_testdata/helm"),
			}
			err := testValidator.Validate(ctx, v1alpha2.FormatHelm, &store)
			Expect(err).NotTo(HaveOccurred())
		})

		It("expect no error to have occurred when valid helm bundle with chart.yaml in cwd is provided", func() {
			store := source.MockStore{
				Fs: afero.NewBasePathFs(afero.NewOsFs(), "valid_testdata/helm/test"),
			}
			err := testValidator.Validate(ctx, v1alpha2.FormatHelm, &store)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("valid bundle formats with invalid bundles are provided", func() {
		// Tests specific to invalid registryV1 bundles are convered during plain to registry conversion.
		When("invalid plain bundle is provided", func() {
			It("expect error to have occurred when plain bundle with no objects is provided", func() {
				fs = afero.NewMemMapFs()
				Expect(fs.MkdirAll("manifests", 0755)).NotTo(HaveOccurred())
				store := source.MockStore{
					Fs: fs,
				}
				err := testValidator.Validate(ctx, v1alpha2.FormatPlain, &store)
				Expect(err).To(HaveOccurred())

			})
			It("expect error when plain bundle has subdirectories", func() {
				fs = afero.NewMemMapFs()
				Expect(fs.MkdirAll("manifests/testpath", 0755)).NotTo(HaveOccurred())
				store := source.MockStore{
					Fs: fs,
				}
				err := testValidator.Validate(ctx, v1alpha2.FormatPlain, &store)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("subdirectories are not allowed"))
			})
		})

		When("invalid helm bundle is provided", func() {
			It("expecy error when helm bundle when empty folder is provided", func() {
				fs = afero.NewMemMapFs()
				Expect(fs.MkdirAll("templates", 0755)).NotTo(HaveOccurred())
				Expect(fs.MkdirAll("charts", 0755)).NotTo(HaveOccurred())
				store := source.MockStore{
					Fs: fs,
				}
				err := testValidator.Validate(ctx, v1alpha2.FormatHelm, &store)
				Expect(err).To(HaveOccurred())
			})
		})

	})
})
