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
)

type ConfigMapUnpackerOption func(c *configMaps)

// WithConfigMapNamespace configures the namespace used by the
// ConfigMap Unpacker to look for ConfigMaps
func WithConfigMapNamespace(ns string) ConfigMapUnpackerOption {
	return func(c *configMaps) {
		c.ConfigMapNamespace = ns
	}
}

// NewConfigMapUnpacker returns a new Unpacker for unpacking sources of type "configmap"
func NewConfigMapUnpacker(reader client.Reader, opts ...ConfigMapUnpackerOption) Unpacker {
	cm := &configMaps{
		Reader:             reader,
		ConfigMapNamespace: "default",
	}

	for _, opt := range opts {
		opt(cm)
	}

	return cm
}

type configMaps struct {
	Reader             client.Reader
	ConfigMapNamespace string
}

func (o *configMaps) Unpack(ctx context.Context, src *Source, _ client.Object) (*Result, error) {
	if src.Type != SourceTypeConfigMaps {
		return nil, fmt.Errorf("source type %q not supported", src.Type)
	}
	if src.ConfigMaps == nil {
		return nil, fmt.Errorf("source configmaps configuration is unset")
	}

	configMapSources := src.ConfigMaps

	bundleFS := fstest.MapFS{}
	seenFilepaths := map[string]sets.Set[string]{}

	for _, cmSource := range configMapSources {
		cmName := cmSource.ConfigMap.Name
		dir := filepath.Clean(cmSource.Path)

		// Validating admission webhook handles validation for:
		//  - paths outside the src root
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

	resolvedSource := &Source{
		Type:       SourceTypeConfigMaps,
		ConfigMaps: src.DeepCopy().ConfigMaps,
	}

	message := generateMessage("configMaps")
	return &Result{FS: bundleFS, ResolvedSource: resolvedSource, State: StateUnpacked, Message: message}, nil
}
