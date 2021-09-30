## Bundles

Bundles are the API at the heart of RukPak, and represent discrete packages of content that can be made available within
the cluster. Bundles are used to reference content outside the cluster that needs to be pulled and unpacked within the cluster.  
A Bundle can represent a particular operator version, a Helm chart, a series of arbitrary manifests, or other content types.

Bundles do nothing on their own - they require a Provisioner to unpack and make their content available in-cluster. 
A Bundle can be unpacked to any arbitrary storage medium such as a PersistentVolume or ConfigMap. 
Each Bundle has an associated `spec.class` field which indicates the Provisioner that should be watching and unpacking that particular bundle type.

The Bundle spec is simple and reflects the need to unpack a series of manifests, from a given source, via a provisioner. 
```go
// BundleSpec defines the desired state of Bundle
type BundleSpec struct {
	// Class specifies the name of the provisioner that should manage the Bundle.
	// +optional
	Class string `json:"class,omitempty"`

	// Source locates all remote content to be staged by the Bundle.
	Source Source `json:"source"`
}
``` 

For example, 
```yaml
apiVersion: core.rukpak.io/v1alpha1
kind: Bundle
metadata: 
  name: example-operator.v0.9.3
spec:
  class: rukpack.io/k8s
  source: localhost:443/example/exampleoperator:v0.9.3
```

This Bundle represents the intent to install example-operator.v0.9.3, which is sourced from container image localhost:443/example/exampleoperator:v0.9.3,
from a local registry available on-cluster. 