package bundle

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PlainV0 struct {
	Objects []client.Object
}

const plainV0ManifestsDir = "manifests"

func LoadPlainV0(bundle fs.FS) (*PlainV0, error) {
	var objects []client.Object
	dirEntries, err := fs.ReadDir(bundle, plainV0ManifestsDir)
	if err != nil {
		return nil, err
	}
	if len(dirEntries) == 0 {
		return nil, errors.New("invalid bundle: found zero files")
	}
	for _, manifest := range dirEntries {
		if manifest.IsDir() {
			return nil, fmt.Errorf("subdirectories are not allowed within the %q directory "+
				"of the bundle image filesystem: found %q", plainV0ManifestsDir, filepath.Join(plainV0ManifestsDir, manifest.Name()))
		}
		theseObjects, err := getObjects(bundle, manifest)
		if err != nil {
			return nil, err
		}
		objects = append(objects, theseObjects...)
	}
	if len(objects) == 0 {
		return nil, errors.New("invalid bundle: found zero objects: " +
			"plain+v0 bundles are required to contain at least one object")
	}
	return &PlainV0{Objects: objects}, nil
}

func getObjects(bundle fs.FS, manifest fs.DirEntry) ([]client.Object, error) {
	manifestPath := filepath.Join(plainV0ManifestsDir, manifest.Name())
	manifestReader, err := bundle.Open(manifestPath)
	if err != nil {
		return nil, err
	}
	defer manifestReader.Close()
	// Stream().Do() visits and serializes each object
	result := resource.NewLocalBuilder().Flatten().Unstructured().Stream(manifestReader, manifestPath).Do()
	if err := result.Err(); err != nil {
		return nil, err
	}
	infos, err := result.Infos()
	if err != nil {
		return nil, err
	}
	return infosToObjects(infos), nil
}

func infosToObjects(infos []*resource.Info) []client.Object {
	objects := make([]client.Object, 0, len(infos))
	for _, info := range infos {
		clientObject := info.Object.(client.Object)
		objects = append(objects, clientObject)
	}
	return objects
}
