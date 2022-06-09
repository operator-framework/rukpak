package manifest_test

import (
	"io/fs"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("FS", func() {
	Describe("opening a file", func() {
		var (
			name string

			f   fs.File
			err error
		)

		JustBeforeEach(func() {
			f, err = testFS.Open(name)
		})

		When("The file exists", func() {
			BeforeEach(func() {
				name = csvFname
			})

			It("should return a file and no error", func() {
				Expect(f).ToNot(BeNil())
				Expect(err).ToNot(HaveOccurred())
			})
		})

		When("The file exists", func() {
			BeforeEach(func() {
				name = "a-non-existent-file.yaml"
			})

			It("should return a nil file and not found error", func() {
				Expect(f).To(BeNil())
				Expect(err).To(MatchError(os.ErrNotExist))
			})
		})
	})
})
