package bundle

import (
	"io/fs"
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoadPlainV0", func() {
	It("should fail if there is no manifests directory", func() {
		mockfs := fstest.MapFS{}
		_, err := LoadPlainV0(mockfs)
		Expect(err).To(MatchError(ContainSubstring("file does not exist")))
	})
	It("should fail if there is an empty manifests directory", func() {
		mockfs := fstest.MapFS{
			"manifests": {
				Mode: fs.ModeDir,
			},
		}
		_, err := LoadPlainV0(mockfs)
		Expect(err).To(MatchError(ContainSubstring("found zero files")))
	})
	It("should fail if there is a file, but no yaml objects", func() {
		mockfs := fstest.MapFS{
			"manifests/invalid.yaml": {
				Data: []byte(""),
			},
		}
		_, err := LoadPlainV0(mockfs)
		Expect(err).To(MatchError(ContainSubstring("required to contain at least one object")))
	})
	It("should fail if any of the files are invalid json or yaml", func() {
		mockfs := fstest.MapFS{
			"manifests/valid.yaml": {
				Data: []byte("kind: ofnecessary"),
			},
			"manifests/invalid.yaml": {
				Data: []byte("not;valid"),
			},
		}
		_, err := LoadPlainV0(mockfs)
		Expect(err).To(MatchError(ContainSubstring("cannot unmarshal string into Go value")))
	})
	It("should fail if the YAML or JSON objects do not specify `kind`", func() {
		mockfs := fstest.MapFS{
			"manifests/invalid.yaml": {
				Data: []byte("almost: valid"),
			},
		}
		_, err := LoadPlainV0(mockfs)
		Expect(err).To(MatchError(ContainSubstring("Object 'Kind' is missing")))
	})
	It("should pass if the YAML is valid and contains a `kind` element", func() {
		mockfs := fstest.MapFS{
			"manifests/valid.yaml": {
				Data: []byte("kind: ofnecessary"),
			},
		}
		_, err := LoadPlainV0(mockfs)
		Expect(err).To(BeNil())
	})
	It("should pass if the JSON is valid and contains a `kind` element", func() {
		mockfs := fstest.MapFS{
			"manifests/valid-json.yaml": {
				Data: []byte(`{"kind": "ofnecessary"}`),
			},
		}
		plainv0, err := LoadPlainV0(mockfs)
		Expect(len(plainv0.Objects)).To(Equal(1))
		Expect(err).To(BeNil())
	})
	It("should pass if some manifests are JSON and some are YAML", func() {
		mockfs := fstest.MapFS{
			"manifests/valid-json.yaml": {
				Data: []byte(`{"kind": "ofnecessary"}`),
			},
			"manifests/valid-yaml.yaml": {
				Data: []byte(`kind: ofnecessary`),
			},
		}
		plainv0, err := LoadPlainV0(mockfs)
		Expect(len(plainv0.Objects)).To(Equal(2))
		Expect(err).To(BeNil())
	})

	It("should fail if there are nested subdirectories", func() {
		mockfs := fstest.MapFS{
			"manifests/valid.yaml": {
				Data: []byte("kind: ofnecessary"),
			},
			"manifests/invalid/valid.yaml": {
				Data: []byte("kind: illegalsubdir"),
			},
		}
		_, err := LoadPlainV0(mockfs)
		Expect(err).To(MatchError(ContainSubstring("subdirectories are not allowed")))
	})
	It("should pass with multi-object yaml files", func() {
		mockfs := fstest.MapFS{
			"manifests/valid.yaml": {
				Data: []byte("kind: ofnecessary\n---\nkind: me"),
			},
		}
		plainv0, err := LoadPlainV0(mockfs)
		Expect(len(plainv0.Objects)).To(Equal(2))
		Expect(err).To(BeNil())
	})
	It("should flatten and pass with metav1.List yaml files", func() {
		mockfs := fstest.MapFS{
			"manifests/valid.yaml": {
				Data: []byte("kind: List\napiVersion: v1\nitems:\n  - kind: me\n    apiVersion: v1\n  - kind: you\n    apiVersion: v1"),
			},
		}
		plainv0, err := LoadPlainV0(mockfs)
		Expect(len(plainv0.Objects)).To(Equal(2))
		Expect(err).To(BeNil())
	})
})
