package features

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"
)

const (
// Add new feature gates constants (strings)
// Ex: SomeFeature featuregate.Feature = "SomeFeature"
)

var rukpakFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	// Add new feature gate definitions
	// Ex: SomeFeature: {...}
}

var RukpakFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()

func init() {
	utilruntime.Must(RukpakFeatureGate.Add(rukpakFeatureGates))
}
