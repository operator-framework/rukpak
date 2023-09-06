package util

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/operator-framework/rukpak/internal/util"
	"github.com/spf13/afero"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	manifestsDir = "manifests"
)

func GetBundleObjects(bundleFS afero.Fs) ([]client.Object, error) {
	entries, err := afero.ReadDir(bundleFS, manifestsDir)
	if err != nil {
		return nil, err
	}

	var bundleObjects []client.Object
	for _, e := range entries {
		if e.IsDir() {
			return nil, fmt.Errorf("subdirectories are not allowed within the %q directory of the bundle image filesystem: found %q", manifestsDir, filepath.Join(manifestsDir, e.Name()))
		}

		manifestObjects, err := getObjects(bundleFS, e)
		if err != nil {
			return nil, err
		}
		bundleObjects = append(bundleObjects, manifestObjects...)
	}
	return bundleObjects, nil
}

func getObjects(bundle afero.Fs, manifest fs.FileInfo) ([]client.Object, error) {
	manifestPath := filepath.Join(manifestsDir, manifest.Name())
	manifestReader, err := bundle.Open(manifestPath)
	if err != nil {
		return nil, err
	}
	defer manifestReader.Close()
	return util.ManifestObjects(manifestReader, manifestPath)
}
