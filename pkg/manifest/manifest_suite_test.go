package manifest_test

import (
	"io/fs"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/manifest"
	"github.com/operator-framework/rukpak/test/testutil"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestManifest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Manifest Suite")
}

const csvFname = "memcached-operator.clusterserviceversion.yaml"

var (
	testFS manifest.FS

	testCSV operatorsv1alpha1.ClusterServiceVersion

	_ = BeforeSuite(func() {
		baseFS, err := fs.Sub(testutil.NewRegistryV1FS(), "manifests")
		Expect(err).ToNot(HaveOccurred())
		testFS, err = manifest.NewFS(baseFS)
		Expect(err).ToNot(HaveOccurred())

		err = yaml.Unmarshal(testutil.NewRegistryV1FS()[filepath.Join("manifests", csvFname)].Data, &testCSV)
		Expect(err).ToNot(HaveOccurred())
	})
)
