package source

import (
	"context"
	"fmt"
	"path/filepath"
	"testing/fstest"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
)

type ConfigMaps struct {
	Reader             client.Reader
	ConfigMapNamespace string
}

func (o *ConfigMaps) Unpack(ctx context.Context, bundle *rukpakv1alpha1.Bundle) (*Result, error) {
	if bundle.Spec.Source.Type != rukpakv1alpha1.SourceTypeConfigMaps {
		return nil, fmt.Errorf("bundle source type %q not supported", bundle.Spec.Source.Type)
	}
	if bundle.Spec.Source.ConfigMaps == nil {
		return nil, fmt.Errorf("bundle source configmaps configuration is unset")
	}

	configMapSources := bundle.Spec.Source.ConfigMaps

	bundleFS := fstest.MapFS{}
	for _, cmSource := range configMapSources {
		cmName := cmSource.ConfigMap.Name
		dir := filepath.Clean(cmSource.Path)

		// Check for paths outside the bundle root is handled in the bundle validation webhook
		// if strings.HasPrefix("../", dir) { ... }

		var cm corev1.ConfigMap
		if err := o.Reader.Get(ctx, client.ObjectKey{Name: cmName, Namespace: o.ConfigMapNamespace}, &cm); err != nil {
			return nil, fmt.Errorf("get configmap %s/%s: %v", o.ConfigMapNamespace, cmName, err)
		}

		// TODO: move configmaps immutability check to webhooks
		//   This would require the webhook to lookup referenced configmaps.
		//   We would need to implement this in two places:
		//     1. During bundle create:
		//         - if referenced configmap already exists, ensure it is immutable
		//         - if referenced configmap does not exist, allow the bundle to be created anyway
		//     2. During configmap create:
		//         - if the configmap is referenced by a bundle, ensure it is immutable
		//         - if not referenced by a bundle, allow the configmap to be created.
		if cm.Immutable == nil || *cm.Immutable == false {
			return nil, fmt.Errorf("configmap %s/%s is mutable: all bundle configmaps must be immutable", o.ConfigMapNamespace, cmName)
		}

		files := map[string][]byte{}
		for filename, data := range cm.Data {
			files[filename] = []byte(data)
		}
		for filename, data := range cm.BinaryData {
			files[filename] = data
		}

		seenFilepaths := map[string]string{}
		for filename, data := range files {
			filepath := filepath.Join(dir, filename)

			// forbid multiple configmaps in the list from referencing the same destination file.
			if existingCmName, ok := seenFilepaths[filepath]; ok {
				return nil, fmt.Errorf("configmap %s/%s contains path %q which is already referenced by configmap %s/%s",
					o.ConfigMapNamespace, cmName, filepath, o.ConfigMapNamespace, existingCmName)
			}
			seenFilepaths[filepath] = cmName
			bundleFS[filepath] = &fstest.MapFile{
				Data: data,
			}
		}
	}

	resolvedSource := &rukpakv1alpha1.BundleSource{
		Type:       rukpakv1alpha1.SourceTypeConfigMaps,
		ConfigMaps: bundle.Spec.Source.DeepCopy().ConfigMaps,
	}

	message := generateMessage("configMaps")
	return &Result{Bundle: bundleFS, ResolvedSource: resolvedSource, State: StateUnpacked, Message: message}, nil
}
