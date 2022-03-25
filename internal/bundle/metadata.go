package bundle

import (
	"io/fs"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

const metadataFile = "bundle.yaml"

type Metadata struct {
	metav1.TypeMeta `json:",inline"`
}

func MetadataFromFS(bundleFS fs.FS) (*Metadata, error) {
	bundleFileData, err := fs.ReadFile(bundleFS, metadataFile)
	if err != nil {
		return nil, err
	}
	obj := &Metadata{}
	if err := yaml.Unmarshal(bundleFileData, obj); err != nil {
		return nil, err
	}
	return obj, nil
}
