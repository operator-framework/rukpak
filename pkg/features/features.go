package features

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"
)

const (
	// Add new feature gates constants (strings)
	// Ex: SomeFeature featuregate.Feature = "SomeFeature"

	BundleDeploymentHealth                   featuregate.Feature = "BundleDeploymentHealth"
	BundleDeploymentCustomAvailabilityProbes featuregate.Feature = "BundleDeploymentCustomAvailabilityProbes"
)

var rukpakFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	// Add new feature gate definitions
	// Ex: SomeFeature: {...}
	BundleDeploymentHealth:                   {Default: false, PreRelease: featuregate.Alpha},
	BundleDeploymentCustomAvailabilityProbes: {Default: false, PreRelease: featuregate.Alpha},
}

var RukpakFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()

func init() {
	utilruntime.Must(RukpakFeatureGate.Add(rukpakFeatureGates))
}
