package probing

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// kindSelector wraps a Probe object and only executes the probe when the probed object is of the right Group and Kind.
type kindSelector struct {
	Prober
	schema.GroupKind
}

func (kp *kindSelector) Probe(obj *unstructured.Unstructured) (success bool, message string) {
	gvk := obj.GetObjectKind().GroupVersionKind()
	if kp.Kind == gvk.Kind &&
		kp.Group == gvk.Group {
		return kp.Prober.Probe(obj)
	}

	// We want to _skip_ objects, that don't match.
	// So this probe succeeds by default.
	return true, ""
}

type selectorSelector struct {
	Prober
	labels.Selector
}

func (ss *selectorSelector) Probe(obj *unstructured.Unstructured) (success bool, message string) {
	if !ss.Selector.Matches(labels.Set(obj.GetLabels())) {
		// We want to _skip_ objects, that don't match.
		// So this probe succeeds by default.
		return true, ""
	}

	return ss.Prober.Probe(obj)
}
