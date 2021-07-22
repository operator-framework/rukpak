package manifests

import (
	"bytes"
	_ "embed"
	"text/template"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var (
	//go:embed job.yaml
	bundleUnpackJob string
)

type BundleUnpackJobConfig struct {
	JobName         string
	JobNamespace    string
	PVCName         string
	BundleImage     string
	UnpackOutputDir string
	UnpackImage     string
}

func NewJobManifest(config BundleUnpackJobConfig) (*batchv1.Job, error) {
	t, err := template.New("bundle").Parse(bundleUnpackJob)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, config)
	if err != nil {
		return nil, err
	}

	var job batchv1.Job
	if err := yaml.Unmarshal(buf.Bytes(), &job); err != nil {
		return nil, err
	}

	return &job, nil
}
