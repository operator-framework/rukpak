package v0

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apimacherrors "k8s.io/apimachinery/pkg/util/errors"
	apimachyaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/rukpak/internal/bundle"
)

const manifestsPath = "manifests"

var GVK = schema.GroupVersionKind{Group: "bundles.rukpak.io", Version: "v0", Kind: "Plain"}

type Bundle struct {
	bundle.Metadata
	Objects []client.Object
}

func (b Bundle) Validate() error {
	var errs []error
	if b.GroupVersionKind() != GVK {
		errs = append(errs, fmt.Errorf("unexpected bundle metadata: expected %q, got %q", GVK, b.GroupVersionKind()))
	}
	if len(b.Objects) == 0 {
		errs = append(errs, fmt.Errorf("found zero objects: bundles of type %q must have at least one object", GVK))
	}
	return apimacherrors.NewAggregate(errs)
}

func LoadFS(bundleFS fs.FS) (*Bundle, error) {
	metadata, err := bundle.MetadataFromFS(bundleFS)
	if err != nil {
		return nil, fmt.Errorf("load metadata: %w", err)
	}
	objects, err := getObjects(bundleFS)
	if err != nil {
		return nil, fmt.Errorf("load objects: %w", err)
	}
	b := &Bundle{Metadata: *metadata, Objects: objects}
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("invalid bundle: %w", err)
	}
	return b, nil
}

func getObjects(bundleFS fs.FS) ([]client.Object, error) {
	var objects []client.Object
	entries, err := fs.ReadDir(bundleFS, manifestsPath)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fileData, err := fs.ReadFile(bundleFS, filepath.Join(manifestsPath, e.Name()))
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
				return nil, err
			}
			objects = append(objects, &obj)
		}
	}
	return objects, nil
}
