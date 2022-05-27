package bundle

import (
	"errors"
	"io/fs"
	"strings"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/rukpak/pkg/manifest"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RegistryV1 holds the contents of a registry+v1 bundle.
type RegistryV1 struct {
	manifest.FS
	csv    *operatorsv1alpha1.ClusterServiceVersion
	crds   []*apiextensionsv1.CustomResourceDefinition
	others []client.Object
}

// NewRegistryV1 reads in a filesystem that containes a bundle.
//
// If the filesystem is itself another known bundle format, it will convert it to the registry+v1 format.
// Otherwise it will treat the filesystem as a valid registry+v1 layout and parse its contents into manifests.
func NewRegistryV1(fsys fs.FS, opts ...Option) (*RegistryV1, error) {
	var allOpts options
	for _, opt := range opts {
		opt.apply(&allOpts)
	}
	switch bundle := fsys.(type) {
	case PlainV0:
		return newRegistryV1FromPlainV0(bundle, allOpts)
	default:
		return newRegistryV1FromFS(fsys, allOpts)
	}
}

func newRegistryV1FromFS(baseFS fs.FS, opts options) (*RegistryV1, error) {
	fsys, err := newManifestFS(baseFS)
	if err != nil {
		return nil, err
	}

	bundle := RegistryV1{FS: fsys}
	for _, file := range fsys {
		for _, obj := range file.Objects {
			switch typedObj := obj.(type) {
			case *operatorsv1alpha1.ClusterServiceVersion:
				bundle.csv = typedObj
			case *apiextensionsv1.CustomResourceDefinition:
				bundle.crds = append(bundle.crds, typedObj)
			default:
				bundle.others = append(bundle.others, typedObj)
			}
		}
	}

	return &bundle, nil
}

func newRegistryV1FromPlainV0(bundle PlainV0, opts options) (*RegistryV1, error) {
	// TODO(ryantking): un-unimplement
	return nil, errors.New("unimplemented: converting plain+v0 to registry+v1")
}

// Open opens a manifest.
func (b RegistryV1) Open(name string) (fs.File, error) {
	if !strings.HasPrefix(name, "/manifests") {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	return b.FS.Open(strings.TrimPrefix(name, "/manifests"))
}

// CSV returns the ClusterServiceVersion manifest.
func (r RegistryV1) CSV() operatorsv1alpha1.ClusterServiceVersion {
	return *r.csv
}

// CSV returns the ClusterServiceVersion manifest.
func (r RegistryV1) CRDs() []apiextensionsv1.CustomResourceDefinition {
	crds := make([]apiextensionsv1.CustomResourceDefinition, len(r.crds))
	for i, crd := range r.crds {
		crds[i] = *crd
	}

	return crds
}

// Others returns all other manifest files.
func (r RegistryV1) Others() []client.Object {
	return r.others
}
