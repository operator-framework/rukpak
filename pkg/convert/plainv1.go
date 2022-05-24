package convert

import "sigs.k8s.io/controller-runtime/pkg/client"

// PlainV1 holds a plain v1 bundle.
type PlainV1 struct {
	Objects []client.Object
}
