package registry

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	apimachyaml "k8s.io/apimachinery/pkg/util/yaml"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	rukpakv1alpha2 "github.com/operator-framework/rukpak/api/v1alpha2"
	"github.com/operator-framework/rukpak/pkg/convert"
	"github.com/operator-framework/rukpak/pkg/provisioner/plain"
)

const (
	// ProvisionerID is the unique registry provisioner ID
	ProvisionerID = "core-rukpak-io-registry"
)

func HandleBundleDeployment(ctx context.Context, fsys fs.FS, bd *rukpakv1alpha2.BundleDeployment) (*chart.Chart, chartutil.Values, error) {
	plainFS, err := convert.RegistryV1ToPlain(fsys, bd.Spec.InstallNamespace, []string{metav1.NamespaceAll})
	if err != nil {
		return nil, nil, fmt.Errorf("convert registry+v1 bundle to plain+v0 bundle: %v", err)
	}
	csv, err := registryV1ExtractCSV(fsys)
	if err != nil {
		return nil, nil, err
	}
	chart, vals, err := plain.HandleBundleDeployment(ctx, plainFS, bd)
	if err != nil {
		return nil, nil, err
	}
	// Append CSV Annotations to Chart Metadata Annotations
	for k, v := range csv.GetAnnotations() {
		chart.Metadata.Annotations[k] = v
	}
	return chart, vals, err
}

func registryV1ExtractCSV(fsys fs.FS) (*v1alpha1.ClusterServiceVersion, error) {
	var objects []*unstructured.Unstructured
	const manifestsDir = "manifests"

	entries, err := fs.ReadDir(fsys, manifestsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			return nil, fmt.Errorf("subdirectories are not allowed within the %q directory of the bundle image filesystem: found %q", manifestsDir, filepath.Join(manifestsDir, e.Name()))
		}
		fileData, err := fs.ReadFile(fsys, filepath.Join(manifestsDir, e.Name()))
		if err != nil {
			return nil, err
		}

		dec := apimachyaml.NewYAMLOrJSONDecoder(bytes.NewReader(fileData), 1024)
		for {
			obj := unstructured.Unstructured{}
			err := dec.Decode(&obj)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("read %q: %v", e.Name(), err)
			}
			objects = append(objects, &obj)
		}
	}

	for _, obj := range objects {
		obj := obj
		if obj.GetObjectKind().GroupVersionKind().Kind == "ClusterServiceVersion" {
			csv := v1alpha1.ClusterServiceVersion{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &csv); err != nil {
				return nil, err
			}
			return &csv, nil
		}
	}

	return nil, fmt.Errorf("no csv found within the %q directory of the bundle image filesystem", manifestsDir)
}
