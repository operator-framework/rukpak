package source

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"

	"github.com/go-git/go-billy/v5/memfs"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Local struct {
	client.Reader
}

func (o *Local) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeLocal {
		return nil, fmt.Errorf("bundle source type %q not supported", bundle.Spec.Source.Type)
	}
	if bundle.Spec.Source.Local == nil {
		return nil, fmt.Errorf("bundle source local configuration is unset")
	}

	configMapRef := bundle.Spec.Source.Local.ConfigMapRef

	var cm corev1.ConfigMap
	if err := o.Get(ctx, client.ObjectKey{Name: configMapRef.Name, Namespace: configMapRef.Namespace}, &cm); err != nil {
		return nil, fmt.Errorf("could not find configmap %s/%s on the cluster", configMapRef.Namespace, configMapRef.Name)
	}

	// If the configmap is empty, return early
	if len(cm.Data) == 0 {
		return nil, fmt.Errorf("configmap %s/%s is empty: at least one object is required", configMapRef.Namespace, configMapRef.Name)
	}

	var memFS = memfs.New()
	for filename, contents := range cm.Data {
		file, err := memFS.Create(filepath.Join("manifests", filename))
		if err != nil {
			return nil, fmt.Errorf("creating filesystem from configmap: %s", err)
		}
		_, err = file.Write([]byte(contents))
		if err != nil {
			return nil, fmt.Errorf("creating filesystem from configmap: %s", err)
		}
	}

	var bundleFS fs.FS = &billyFS{memFS}
	resolvedSource := &rukpakv1alpha1.BundleSource{
		Type:  rukpakv1alpha1.SourceTypeLocal,
		Local: bundle.Spec.Source.Local.DeepCopy(),
	}

	return &Result{Bundle: bundleFS, ResolvedSource: resolvedSource, State: StateUnpacked}, nil
}
