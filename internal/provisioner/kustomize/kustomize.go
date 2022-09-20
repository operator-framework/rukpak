package kustomize

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apimachyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/yaml"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
	"github.com/operator-framework/rukpak/internal/util"
)

const (
	// ProvisionerID is the unique kustomize provisioner ID
	ProvisionerID = "core-rukpak-io-kustomize"
)

func HandleBundle(ctx context.Context, fsys fs.FS, bundle *rukpakv1alpha1.Bundle) (fs.FS, error) {
	err := verifyObjects(".", fsys)
	if err != nil {
		return nil, err
	}
	return fsys, nil
}

func HandleBundleDeployment(ctx context.Context, fsys fs.FS, bd *rukpakv1alpha1.BundleDeployment) (*chart.Chart, chartutil.Values, error) {
	desiredObjects, err := loadBundle(fsys, bd)
	if err != nil {
		return nil, nil, err
	}

	chrt := &chart.Chart{
		Metadata: &chart.Metadata{},
	}
	for _, obj := range desiredObjects {
		jsonData, err := yaml.Marshal(obj)
		if err != nil {
			return nil, nil, err
		}
		hash := sha256.Sum256(jsonData)
		chrt.Templates = append(chrt.Templates, &chart.File{
			Name: fmt.Sprintf("object-%x.yaml", hash[0:8]),
			Data: jsonData,
		})
	}
	return chrt, nil, nil
}

func verifyObjects(path string, bundleFS fs.FS) error {
	entries, err := fs.ReadDir(bundleFS, path)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			// Symbolic link is skipped to avoid infinite loop
			if (e.Type() & fs.ModeSymlink) != 0 {
				continue
			}
			err := verifyObjects(filepath.Join(path, e.Name()), bundleFS)
			if err != nil {
				return err
			}
			continue
		}
		fileData, err := fs.ReadFile(bundleFS, filepath.Join(path, e.Name()))
		if err != nil {
			return err
		}
		var data interface{}
		err = yaml.Unmarshal(fileData, &data)
		if err != nil {
			return err
		}
	}
	return nil
}

func loadBundle(fsys fs.FS, bd *rukpakv1alpha1.BundleDeployment) ([]client.Object, error) {
	bundleFileSystem, err := fStoFileSystem(fsys)
	if err != nil {
		return nil, fmt.Errorf("kustomize file copy error: %v", err)
	}
	path, err := loadPath(bd)
	if err != nil {
		return nil, fmt.Errorf("path for kustomize is not specified: %v", err)
	}
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	res, err := kustomizer.Run(bundleFileSystem, path)
	if err != nil {
		return nil, fmt.Errorf("kustomize error: %v", err)
	}
	fileData, err := res.AsYaml()
	if err != nil {
		return nil, fmt.Errorf("kustomize asyaml error: %v", err)
	}

	var objects []client.Object
	dec := apimachyaml.NewYAMLOrJSONDecoder(bytes.NewReader(fileData), 1024)
	for {
		obj := unstructured.Unstructured{}
		err := dec.Decode(&obj)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read error: %v", err)
		}
		objects = append(objects, &obj)
	}

	objs := make([]client.Object, 0, len(objects))
	for _, obj := range objects {
		obj := obj
		obj.SetLabels(util.MergeMaps(obj.GetLabels(), map[string]string{
			util.CoreOwnerKindKey: rukpakv1alpha1.BundleDeploymentKind,
			util.CoreOwnerNameKey: bd.GetName(),
		}))
		objs = append(objs, obj)
	}
	return objs, nil
}

func loadPath(bd *rukpakv1alpha1.BundleDeployment) (string, error) {
	data, err := bd.Spec.Config.MarshalJSON()
	if err != nil {
		return "", err
	}
	var config map[string]string
	err = json.Unmarshal(data, &config)
	if err != nil {
		return "", err
	}
	path, ok := config["path"]
	// When path is not specified, bundle root directory is used as the path
	if !ok {
		path = "."
	}
	return path, nil
}

func fStoFileSystem(infs fs.FS) (filesys.FileSystem, error) {
	memfs := filesys.MakeFsInMemory()
	err := fs.WalkDir(infs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("kustomize copy file error: %v", err)
		}
		if d.IsDir() {
			return nil
		}
		file, err := memfs.Create(path)
		if err != nil {
			return fmt.Errorf("kustomize create file error: %v", err)
		}
		fileData, err := fs.ReadFile(infs, path)
		if err != nil {
			return fmt.Errorf("kustomize read file error: %v", err)
		}
		_, err = file.Write(fileData)
		if err != nil {
			return fmt.Errorf("kustomize write file error: %v", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return memfs, nil
}
