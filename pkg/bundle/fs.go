package bundle

import (
	"io/fs"
	"path/filepath"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FS is a File System that contains a bundle.
//
// A bundle is a directory that contains files that may or may not be
// Kubernetes manifests. When opening a manifest file, it will return an
// `ObjectFile` type that allows the consumer to access the typed objects
// without first opening/parsing the manifest itself. It still fully supports
// all file operations.
//
// TODO(ryantking): Should we support any other file system abstractions beyond `fs.FS`?
// `fs.FS` doesn't support any write operations. They can modify the objects in an `ObjectFile`, but that won't change
// the byte representation.
type FS struct {
	baseFS       fs.FS
	manifestDirs []string
	scheme       *runtime.Scheme
	strictTypes  bool

	// TODO(ryantking): This is a hack since there is no API for modifying files in an fs.FS
	storedObjs map[string]ObjectFile[client.Object]
}

// New returns a new FS wrapped around the given base FS.
func New(baseFS fs.FS, opts ...func(*FS)) FS {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(operatorsv1alpha1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(rukpakv1alpha1.AddToScheme(scheme))

	fsys := FS{
		baseFS:     baseFS,
		scheme:     scheme,
		storedObjs: make(map[string]ObjectFile[client.Object]),
	}
	for _, opt := range opts {
		opt(&fsys)
	}

	return fsys
}

// WithManifestDir adds a restriction that manifests are contained in a given directory.
//
// If this option is used multiple times, manifests will be looked for in all
// of the given directories.
func WithManifestDir(name string) func(*FS) {
	return func(fsys *FS) {
		fsys.manifestDirs = append(fsys.manifestDirs, name)
	}
}

// WithScheme uses the given scheme to add type information to objects.
func WithScheme(scheme *runtime.Scheme) func(*FS) {
	return func(fsys *FS) {
		fsys.scheme = scheme
	}
}

// WithStrictTypes will cause an error if `Open` is called on a manifest file
// that contains object types that the scheme does not recognize.
func WithStrictTypes() func(*FS) {
	return func(fsys *FS) {
		fsys.strictTypes = true
	}
}

// Open a file if it exists on the file system.
// If the file is in a directory that containes manifests and ends in .yaml,
// it will return a `File` with the manifests parsed to their underlying types.
func (fsys FS) Open(name string) (fs.File, error) {
	// TODO(ryantking): When we figure out how to store objects, this will change
	if objFile, ok := fsys.storedObjs[name]; ok {
		return objFile, nil
	}

	f, err := fsys.baseFS.Open(name)
	if err != nil {
		return nil, err
	}
	if !fsys.isManifestFile(name) {
		return f, nil
	}

	objFile, err := NewObjectFile[client.Object](f, fsys.scheme, fsys.strictTypes)
	if err != nil {
		f.Close() // close the file that's already open
		return nil, err
	}

	return objFile, nil
}

func (fsys FS) isManifestFile(name string) bool {
	if filepath.Ext(name) != ".yaml" {
		return false
	}
	if len(fsys.manifestDirs) == 0 {
		return true
	}
	for _, dir := range fsys.manifestDirs {
		if filepath.HasPrefix(name, dir) {
			return true
		}
	}

	return false
}

// Scheme returns the scheme used to convert between objects.
func (fsys FS) Scheme() *runtime.Scheme {
	return fsys.scheme
}

// Objects returns all the objects contained in the filesystem.
// Optionally, filter functions can be supplied that will filter out any objects that they return true for.
func (fsys FS) Objects(filterFns ...func(client.Object) bool) ([]client.Object, error) {
	var objs []client.Object
	filterFn := fsys.mergeFilterFns(filterFns)
	if err := fs.WalkDir(fsys.baseFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !fsys.isManifestFile(path) {
			return err
		}

		f, err := fsys.baseFS.Open(path)
		if err != nil {
			return err
		}
		file, err := NewObjectFile[client.Object](f, fsys.scheme, fsys.strictTypes)
		if err != nil {
			return err
		}
		for _, obj := range file.Objects {
			if filterFn(obj) {
				objs = append(objs, obj)
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	// TODO(ryantking): Remove when the stored objects go away with a better write abstraction
	for _, f := range fsys.storedObjs {
		for _, obj := range f.Objects {
			if filterFn(obj) {
				objs = append(objs, obj)
			}
		}
	}

	return objs, nil
}

// StoreObjects adds objects to the name filed.
func (fsys FS) StoreObjects(name string, objs ...client.Object) {
	objFile := fsys.storedObjs[name]
	objFile.Objects = append(objFile.Objects, objs...)
	fsys.storedObjs[name] = objFile
}

func (fsys FS) mergeFilterFns(filterFns []func(client.Object) bool) func(client.Object) bool {
	return func(obj client.Object) bool {
		for _, fn := range filterFns {
			if !fn(obj) {
				return false
			}
		}

		return true
	}
}
