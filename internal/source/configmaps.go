package source

import (
	"context"
	"fmt"
	"path/filepath"
	"testing/fstest"

	corev1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
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
	seenFilepaths := map[string]sets.Set[string]{}

	for _, cmSource := range configMapSources {
		cmName := cmSource.ConfigMap.Name
		dir := filepath.Clean(cmSource.Path)

		// Validating admission webhook handles validation for:
		//  - paths outside the bundle root
		//  - configmaps referenced by bundles must be immutable

		var cm corev1.ConfigMap
		if err := o.Reader.Get(ctx, client.ObjectKey{Name: cmName, Namespace: o.ConfigMapNamespace}, &cm); err != nil {
			return nil, fmt.Errorf("get configmap %s/%s: %v", o.ConfigMapNamespace, cmName, err)
		}

		addToBundle := func(configMapName, filename string, data []byte) {
			filepath := filepath.Join(dir, filename)
			if _, ok := seenFilepaths[filepath]; !ok {
				seenFilepaths[filepath] = sets.New[string]()
			}
			seenFilepaths[filepath].Insert(configMapName)
			bundleFS[filepath] = &fstest.MapFile{
				Data: data,
			}
		}
		for filename, data := range cm.Data {
			addToBundle(cmName, filename, []byte(data))
		}
		for filename, data := range cm.BinaryData {
			addToBundle(cmName, filename, data)
		}
	}

	errs := []error{}
	for filepath, cmNames := range seenFilepaths {
		if len(cmNames) > 1 {
			errs = append(errs, fmt.Errorf("duplicate path %q found in configmaps %v", filepath, sets.List(cmNames)))
			continue
		}
	}
	if len(errs) > 0 {
		return nil, utilerrors.NewAggregate(errs)
	}

	resolvedSource := &rukpakv1alpha1.BundleSource{
		Type:       rukpakv1alpha1.SourceTypeConfigMaps,
		ConfigMaps: bundle.Spec.Source.DeepCopy().ConfigMaps,
	}

	message := generateMessage("configMaps")
	return &Result{Bundle: bundleFS, ResolvedSource: resolvedSource, State: StateUnpacked, Message: message}, nil
}
