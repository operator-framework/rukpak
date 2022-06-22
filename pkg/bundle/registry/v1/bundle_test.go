package v1_test

import (
	"testing/fstest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	registryv1 "github.com/operator-framework/rukpak/pkg/bundle/registry/v1"
	"github.com/operator-framework/rukpak/test/testutil"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Bundle", func() {
	var fsys fstest.MapFS

	BeforeEach(func() {
		fsys = testutil.NewRegistryV1FS()
	})

	var bundle registryv1.Bundle

	JustBeforeEach(func() {
		bundle = registryv1.New(fsys)
	})

	Describe("reading the CSV manifest", func() {
		var (
			csv *operatorsv1alpha1.ClusterServiceVersion
			err error
		)

		JustBeforeEach(func() {
			csv, err = bundle.CSV()
		})

		It("should not return an error", func() {
			Expect(err).ToNot(HaveOccurred())
		})

		It("should return a non-nil and CSV", func() {
			Expect(csv).ToNot(BeNil())
		})

		When("the file system does not have a CSV manifest", func() {
			BeforeEach(func() {
				delete(fsys, "manifests/memcached-operator.clusterserviceversion.yaml")
			})

			It("should return an error", func() {
				Expect(err).To(MatchError("no CSV found"))
			})
		})

		When("the file system has multiple CSVs", func() {
			BeforeEach(func() {
				csv := fsys["manifests/memcached-operator.clusterserviceversion.yaml"]
				fsys["manifests/memcached-operator.clusterserviceversion.copy.yaml"] = csv
			})

			It("should return an error", func() {
				Expect(err).To(MatchError("more than one CSV found"))
			})
		})
	})

	Describe("reading all CRD manifests", func() {
		var (
			crds []apiextensionsv1.CustomResourceDefinition
			err  error
		)

		JustBeforeEach(func() {
			crds, err = bundle.CRDs()
		})

		It("should not return an error", func() {
			Expect(err).ToNot(HaveOccurred())
		})

		It("should have the expected CRD", func() {
			Expect(crds).To(HaveLen(1))
			Expect(crds[0].GetName()).To(Equal("memcacheds.cache.example.com"))
		})
	})

	Describe("reading all other manifests", func() {
		var (
			objs []client.Object
			err  error
		)

		JustBeforeEach(func() {
			objs, err = bundle.Others()
		})

		It("should not return an error", func() {
			Expect(err).ToNot(HaveOccurred())
		})

		It("should have the expected manifests", func() {
			Expect(objs).To(HaveLen(5))
		})
	})
})
