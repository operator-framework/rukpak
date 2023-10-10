package probing

import (
	"context"
	"fmt"
	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Parse takes a list of ObjectSetProbes (commonly defined within a ObjectSetPhaseSpec)
// and compiles a single Prober to test objects with.
func Parse(ctx context.Context, bdProbes []rukpakv1alpha1.BundleDeploymentProbe) (Prober, error) {
	probeList := make(list, len(bdProbes))
	for i, bdProbe := range bdProbes {
		var (
			probe Prober
			err   error
		)
		probe, err = ParseProbes(ctx, bdProbe.Probes)
		if err != nil {
			return nil, fmt.Errorf("parsing probe #%d: %w", i, err)
		}
		probe, err = ParseSelector(ctx, bdProbe.Selector, probe)
		if err != nil {
			return nil, fmt.Errorf("parsing selector of probe #%d: %w", i, err)
		}
		probeList[i] = probe
	}
	return probeList, nil
}

// ParseSelector reads a corev1alpha1.ProbeSelector and wraps a Prober,
// only executing the Prober when the selector criteria match.
func ParseSelector(_ context.Context, selector rukpakv1alpha1.ProbeSelector, probe Prober) (Prober, error) {
	if selector.Kind != nil {
		probe = &kindSelector{
			Prober: probe,
			GroupKind: schema.GroupKind{
				Group: selector.Kind.Group,
				Kind:  selector.Kind.Kind,
			},
		}
	}
	if selector.Selector != nil {
		s, err := metav1.LabelSelectorAsSelector(selector.Selector)
		if err != nil {
			return nil, err
		}
		probe = &selectorSelector{
			Prober:   probe,
			Selector: s,
		}
	}
	return probe, nil
}

// ParseProbes takes a []corev1alpha1.Probe and compiles it into a Prober.
func ParseProbes(_ context.Context, probeSpecs []rukpakv1alpha1.Probe) (Prober, error) {
	var probeList list
	for _, probeSpec := range probeSpecs {
		var (
			probe Prober
			err   error
		)

		switch {
		case probeSpec.FieldsEqual != nil:
			probe = &fieldsEqualProbe{
				FieldA: probeSpec.FieldsEqual.FieldA,
				FieldB: probeSpec.FieldsEqual.FieldB,
			}

		case probeSpec.Condition != nil:
			probe = NewConditionProbe(
				probeSpec.Condition.Type,
				probeSpec.Condition.Status,
			)

		case probeSpec.CEL != nil:
			probe, err = newCELProbe(
				probeSpec.CEL.Rule,
				probeSpec.CEL.Message,
			)
			if err != nil {
				return nil, err
			}

		default:
			// probe has no known config
			continue
		}
		probeList = append(probeList, probe)
	}

	// Always check .status.observedCondition, if present.
	return &statusObservedGenerationProbe{Prober: probeList}, nil
}
