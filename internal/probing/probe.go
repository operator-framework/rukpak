package probing

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type Prober interface {
	Probe(obj *unstructured.Unstructured) (success bool, message string)
}

type list []Prober

var _ Prober = (list)(nil)

func (p list) Probe(obj *unstructured.Unstructured) (success bool, message string) {
	var messages []string
	for _, probe := range p {
		if success, message := probe.Probe(obj); !success {
			messages = append(messages, message)
		}
	}
	if len(messages) > 0 {
		return false, strings.Join(messages, ", ")
	}
	return true, ""
}

// conditionProbe checks if the object's condition is set and in a certain status.
type conditionProbe struct {
	Type, Status string
}

func NewConditionProbe(typeName, status string) Prober {
	return &conditionProbe{
		Type:   typeName,
		Status: status,
	}
}

var _ Prober = (*conditionProbe)(nil)

func (cp *conditionProbe) Probe(obj *unstructured.Unstructured) (success bool, message string) {
	defer func() {
		if success {
			return
		}
		// add probed condition type and status as context to error message.
		message = fmt.Sprintf("condition %q == %q: %s", cp.Type, cp.Status, message)
	}()

	rawConditions, exist, err := unstructured.NestedFieldNoCopy(
		obj.Object, "status", "conditions")
	conditions, ok := rawConditions.([]interface{})
	if err != nil || !exist {
		return false, "missing .status.conditions"
	}
	if !ok {
		return false, "malformed"
	}

	for _, condI := range conditions {
		cond, ok := condI.(map[string]interface{})
		if !ok {
			// no idea what this is supposed to be
			return false, "malformed"
		}

		if cond["type"] != cp.Type {
			// not the type we are probing for
			continue
		}

		// Check the condition's observed generation, if set
		if observedGeneration, ok, err := unstructured.NestedInt64(
			cond, "observedGeneration",
		); err == nil && ok && observedGeneration != obj.GetGeneration() {
			return false, "outdated"
		}

		if cond["status"] == cp.Status {
			return true, ""
		}
		return false, "wrong status"
	}
	return false, "not reported"
}

// fieldsEqualProbe checks if the values of the fields under the given json paths are equal.
type fieldsEqualProbe struct {
	FieldA, FieldB string
}

var _ Prober = (*fieldsEqualProbe)(nil)

func (fe *fieldsEqualProbe) Probe(obj *unstructured.Unstructured) (success bool, message string) {
	fieldAPath := strings.Split(strings.Trim(fe.FieldA, "."), ".")
	fieldBPath := strings.Split(strings.Trim(fe.FieldB, "."), ".")

	defer func() {
		if success {
			return
		}
		// add probed field paths as context to error message.
		message = fmt.Sprintf(`"%v" == "%v": %s`, fe.FieldA, fe.FieldB, message)
	}()

	fieldAVal, ok, err := unstructured.NestedFieldCopy(obj.Object, fieldAPath...)
	if err != nil || !ok {
		return false, fmt.Sprintf(`"%v" missing`, fe.FieldA)
	}
	fieldBVal, ok, err := unstructured.NestedFieldCopy(obj.Object, fieldBPath...)
	if err != nil || !ok {
		return false, fmt.Sprintf(`"%v" missing`, fe.FieldB)
	}

	if !equality.Semantic.DeepEqual(fieldAVal, fieldBVal) {
		return false, fmt.Sprintf(`"%v" != "%v"`, fieldAVal, fieldBVal)
	}
	return true, ""
}

// statusObservedGenerationProbe wraps the given Prober and ensures that .status.observedGeneration is equal to .metadata.generation,
// before running the given probe. If the probed object does not contain the .status.observedGeneration field,
// the given prober is executed directly.
type statusObservedGenerationProbe struct {
	Prober
}

var _ Prober = (*statusObservedGenerationProbe)(nil)

func (cg *statusObservedGenerationProbe) Probe(obj *unstructured.Unstructured) (success bool, message string) {
	if observedGeneration, ok, err := unstructured.NestedInt64(
		obj.Object, "status", "observedGeneration",
	); err == nil && ok && observedGeneration != obj.GetGeneration() {
		return false, ".status outdated"
	}
	return cg.Prober.Probe(obj)
}
