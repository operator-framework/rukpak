package manifest_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/rukpak/pkg/manifest"
	. "github.com/operator-framework/rukpak/test/testutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("File", func() {
	var (
		name = csvFname

		file manifest.File
	)

	JustBeforeEach(func() {
		file = testFS[name]
	})

	It("should have an object", func() {
		Expect(file.Objects).To(HaveLen(1))
	})

	Describe("the object", func() {
		var obj client.Object

		JustBeforeEach(func() {
			Expect(file.Objects).To(HaveLen(1))
			obj = file.Objects[0]
		})

		When("its a recognized object type", func() {
			It("should be typed", func() {
				Expect(obj).To(BeAssignableToTypeOf(&testCSV))
			})

			It("should be equivalent to the expected CSV", func() {
				Expect(obj).To(EqualObject(&testCSV))
			})
		})

		When("its an unrecognized object type", func() {
			BeforeEach(func() {
				name = "memcached-operator-controller-manager-metrics-monitor_monitoring.coreos.com_v1_servicemonitor.yaml"
			})

			It("should contain unstructured data", func() {
				Expect(obj).To(BeAssignableToTypeOf(&unstructured.Unstructured{}))
				Expect(obj).NotTo(BeNil())
			})
		})
	})
})
