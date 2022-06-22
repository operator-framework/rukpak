package bundle_test

import (
	"io/fs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

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

	Context("casting to the direct implementation", func() {
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
})
