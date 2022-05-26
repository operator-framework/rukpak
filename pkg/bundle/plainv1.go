package bundle

import (
	"io/fs"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/manifest"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ Bundle = PlainV1{}

// PlainV1 holds a plain v1 bundle.
type PlainV1 struct {
	manifest.FS
}

func NewPlainV1(fsys fs.FS)

// CSV returns the ClusterServiceVersion manifest.
func (r PlainV1) CSV() *operatorsv1alpha1.ClusterServiceVersion {
	return nil
}

// CSV returns the ClusterServiceVersion manifest.
func (r PlainV1) CRDs() []apiextensionsv1.CustomResourceDefinition {
	return nil
}

// Others returns all other manifest files.
func (r PlainV1) Others() []client.Object {
	return r.objs
}
