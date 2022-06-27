package bundle_test

import (
	"io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/bundle"
	"github.com/operator-framework/rukpak/test/testutil"
)

var _ = Describe("ObjectFile", func() {
	var (
		scheme *runtime.Scheme
		fsys   fs.FS
		path   = csvFname

		file fs.File
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		utilruntime.Must(operatorsv1alpha1.AddToScheme(scheme))
		utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
		utilruntime.Must(rukpakv1alpha1.AddToScheme(scheme))
		fsys = testutil.NewRegistryV1FS()
	})

	JustBeforeEach(func() {
		var err error
		file, err = fsys.Open(path)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		file.Close()
	})

	When("casting to the concrete type", func() {
		var objFile *bundle.ObjectFile[*operatorsv1alpha1.ClusterServiceVersion]

		JustBeforeEach(func() {
			var err error
			objFile, err = bundle.NewObjectFile[*operatorsv1alpha1.ClusterServiceVersion](file, scheme, false)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should not be nil", func() {
			Expect(objFile).ToNot(BeNil())
		})
	})

	When("casting to an interface type", func() {
		var (
			strictTypes = false

			objFile *bundle.ObjectFile[client.Object]
			err     error
		)

		JustBeforeEach(func() {
			objFile, err = bundle.NewObjectFile[client.Object](file, scheme, strictTypes)
		})

		When("the object is of a known type", func() {
			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return a file with an object that has its concrete type", func() {
				Expect(objFile.Objects).To(HaveLen(1))
				Expect(objFile.Objects[0]).To(BeAssignableToTypeOf(&operatorsv1alpha1.ClusterServiceVersion{}))
			})
		})

		When("the object is of an unknown type", func() {
			BeforeEach(func() {
				path = "manifests/memcached-operator-controller-manager-metrics-monitor_monitoring.coreos.com_v1_servicemonitor.yaml"
			})

			It("should not return an error", func() {
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return a file with an unstructured object", func() {
				Expect(objFile.Objects).To(HaveLen(1))
				Expect(objFile.Objects[0]).To(BeAssignableToTypeOf(&unstructured.Unstructured{}))
			})

			When("strict types are on", func() {
				BeforeEach(func() {
					strictTypes = true
				})

				It("should return an error", func() {
					Expect(err).To(MatchError("unrecognized object type: monitoring.coreos.com/v1, Kind=ServiceMonitor"))
				})
			})
		})
	})
})
