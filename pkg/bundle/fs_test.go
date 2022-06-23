package bundle_test

import (
	"io/fs"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/pkg/bundle"
	"github.com/operator-framework/rukpak/test/testutil"
)

const csvFname = "manifests/memcached-operator.clusterserviceversion.yaml"

var _ = Describe("FS", func() {
	var (
		baseFS fs.FS
		fsys   bundle.FS
	)

	BeforeEach(func() {
		baseFS = testutil.NewRegistryV1FS()
	})

	JustBeforeEach(func() {
		fsys = bundle.New(baseFS, bundle.WithManifestDirs("manifests"))
	})

	Describe("opening a file", func() {
		var (
			name string

			f   fs.File
			err error
		)

		JustBeforeEach(func() {
			f, err = fsys.Open(name)
		})

		When("it is a normal file", func() {
			BeforeEach(func() {
				baseFS = testutil.NewPlainV0FS()
				name = "Dockerfile"
			})

			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return a normal file", func() {
				Expect(f).ToNot(BeNil())
				Expect(f).ToNot(BeAssignableToTypeOf(bundle.ObjectFile[client.Object]{}))
			})
		})

		When("it is a manifest file", func() {
			BeforeEach(func() {
				name = csvFname
			})

			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return a manifest file", func() {
				Expect(f).ToNot(BeNil())
				Expect(f).To(BeAssignableToTypeOf(&bundle.ObjectFile[client.Object]{}))

				By("asserting the file to a manifest file type")
				objFile := f.(*bundle.ObjectFile[client.Object])
				Expect(objFile.Objects).To(HaveLen(1), "it should contain a parsed object")
			})
		})

		When("the file does not exists", func() {
			BeforeEach(func() {
				name = "a-non-existent-file.yaml"
			})

			It("should return a nil file and not found error", func() {
				Expect(err).To(MatchError(os.ErrNotExist))
				Expect(f).To(BeNil())
			})
		})
	})

	Describe("retrieving objects from the filesystem", func() {
		var objs []client.Object

		Context("without any filters", func() {
			BeforeEach(func() {
				var err error
				objs, err = fsys.Objects()
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return every object", func() {
				Expect(objs).To(HaveLen(7))
			})

			It("the objects should have data", func() {
				for _, obj := range objs {
					Expect(obj).ToNot(BeZero())
				}
			})
		})
	})
})
