package bundle

import (
	"io/fs"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const manifestsDir = "manifests"

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(operatorsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(rukpakv1alpha1.AddToScheme(scheme))
}

// Bundle represents a rukpak bundle.
type Bundle interface {
	fs.FS

	// CSV returns the ClusterServiceVersion manifest that defines this bundle.
	CSV() *operatorsv1alpha1.ClusterServiceVersion

	// CRDs returns all the CRDs that this bundle
	CRDs() []apiextensionsv1.CustomResourceDefinition

	// Others returns all other manifests contained by this bundle.
	Others() []client.Object
}
