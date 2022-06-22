package v1

import (
	"errors"
	"io/fs"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/bundle"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Bundle holds the contents of a registry+v1 bundle.
type Bundle struct {
	bundle.FS
}

// New creates a new registry+v1 bundle at the root of the given filesystem.
func New(fsys fs.FS) Bundle {
	// If the source is another bundle, use the FS
	if bundleFS, ok := fsys.(bundle.FS); ok {
		return Bundle{bundleFS}
	}

	return Bundle{bundle.New(fsys, bundle.WithManifestDir("manifests"))}
}

// CSV returns the ClusterServiceVersion manifest if one exists in the bundle.
// Changeing the CSV on the underlying filesystem
func (b Bundle) CSV() (*operatorsv1alpha1.ClusterServiceVersion, error) {
	csvs, err := b.Objects(func(obj client.Object) bool {
		_, ok := obj.(*operatorsv1alpha1.ClusterServiceVersion)
		return ok // filter out objects that don't assert to CSV
	})

	if err != nil {
		return nil, err
	} else if len(csvs) == 0 {
		return nil, errors.New("no CSV found")
	} else if len(csvs) > 1 {
		return nil, errors.New("more than one CSV found")
	}

	return csvs[0].DeepCopyObject().(*operatorsv1alpha1.ClusterServiceVersion), nil
}

// CRDs returns all the CRDs defined in the bundle.
func (b Bundle) CRDs() ([]apiextensionsv1.CustomResourceDefinition, error) {
	objs, err := b.Objects(func(obj client.Object) bool {
		_, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
		return ok // filter out objects that don't assert to CRD
	})
	if err != nil {
		return nil, err
	}

	crds := make([]apiextensionsv1.CustomResourceDefinition, len(objs))
	for i, obj := range objs {
		crd := obj.(*apiextensionsv1.CustomResourceDefinition)
		crds[i] = *crd
	}

	return crds, nil
}

// Others returns all other manifest files.
func (b Bundle) Others() ([]client.Object, error) {
	return b.Objects(func(obj client.Object) bool {
		// filter out the CSV and CRDs
		switch obj.(type) {
		case *operatorsv1alpha1.ClusterServiceVersion,
			*apiextensionsv1.CustomResourceDefinition:
			return false
		default:
			return true
		}
	})
}
