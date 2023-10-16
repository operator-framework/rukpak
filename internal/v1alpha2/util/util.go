/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
