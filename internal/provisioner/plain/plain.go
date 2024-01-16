package plain

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/internal/util"
)

const (
	// ProvisionerID is the unique plain provisioner ID
	ProvisionerID = "core-rukpak-io-plain"

	manifestsDir = "manifests"
)

func HandleBundleDeployment(_ context.Context, fsys fs.FS, bd *rukpakv1alpha2.BundleDeployment) (*chart.Chart, chartutil.Values, error) {
	if err := ValidateBundle(fsys); err != nil {
		return nil, nil, err
	}

	chrt, err := chartFromBundle(fsys, bd)
	if err != nil {
		return nil, nil, err
	}
	return chrt, nil, nil
}

func ValidateBundle(fsys fs.FS) error {
	objects, err := getBundleObjects(fsys)
	if err != nil {
		return fmt.Errorf("get objects from bundle manifests: %v", err)
	}
	if len(objects) == 0 {
		return errors.New("invalid bundle: found zero objects: plain+v0 bundles are required to contain at least one object")
	}
	return nil
}

func getBundleObjects(bundleFS fs.FS) ([]client.Object, error) {
	entries, err := fs.ReadDir(bundleFS, manifestsDir)
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

func getObjects(bundle fs.FS, manifest fs.DirEntry) ([]client.Object, error) {
	manifestPath := filepath.Join(manifestsDir, manifest.Name())
	manifestReader, err := bundle.Open(manifestPath)
	if err != nil {
		return nil, err
	}
	defer manifestReader.Close()
	return util.ManifestObjects(manifestReader, manifestPath)
}

func chartFromBundle(fsys fs.FS, bd *rukpakv1alpha2.BundleDeployment) (*chart.Chart, error) {
	objects, err := getBundleObjects(fsys)
	if err != nil {
		return nil, fmt.Errorf("read bundle objects from bundle: %v", err)
	}

	chrt := &chart.Chart{
		Metadata: &chart.Metadata{},
	}
	for _, obj := range objects {
		obj.SetLabels(util.MergeMaps(obj.GetLabels(), map[string]string{
			util.CoreOwnerKindKey: rukpakv1alpha2.BundleDeploymentKind,
			util.CoreOwnerNameKey: bd.Name,
		}))
		yamlData, err := yaml.Marshal(obj)
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(yamlData)
		chrt.Templates = append(chrt.Templates, &chart.File{
			Name: fmt.Sprintf("object-%x.yaml", hash[0:8]),
			Data: yamlData,
		})
	}
	return chrt, nil
}
