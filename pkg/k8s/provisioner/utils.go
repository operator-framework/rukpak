package provisioner

import "k8s.io/apimachinery/pkg/types"

func getNonNamespacedName(name string) types.NamespacedName {
	return types.NamespacedName{Name: name}
}
